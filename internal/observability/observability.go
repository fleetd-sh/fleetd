package observability

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"go.opentelemetry.io/otel/codes"
	"go.uber.org/zap"
)

// Observability provides unified observability with metrics, logging, and tracing
type Observability struct {
	Metrics *MetricsCollector
	Logger  *Logger
	Tracer  *Tracer
	config  Config
}

type Config struct {
	ServiceName    string
	ServiceVersion string
	Environment    string

	// Metrics
	MetricsEnabled bool
	MetricsAddr    string // Address for Prometheus metrics endpoint

	// Logging
	LogLevel  string
	LogFormat string
	LogOutput string

	// Tracing
	TracingEnabled    bool
	TracingEndpoint   string
	TracingProtocol   string
	TracingInsecure   bool
	TracingSampleRate float64
}

// New creates a new observability instance
func New(ctx context.Context, config Config) (*Observability, error) {
	// Initialize metrics
	var metrics *MetricsCollector
	if config.MetricsEnabled {
		metrics = NewMetricsCollector()

		// Start metrics server
		go func() {
			if err := metrics.StartMetricsServer(ctx, config.MetricsAddr); err != nil {
				fmt.Printf("Failed to start metrics server: %v\n", err)
			}
		}()
	}

	// Initialize logging
	logger := InitLogger(LogConfig{
		Level:       config.LogLevel,
		Format:      config.LogFormat,
		OutputPath:  config.LogOutput,
		ServiceName: config.ServiceName,
		Environment: config.Environment,
		Version:     config.ServiceVersion,
	})

	// Initialize tracing
	tracer, err := InitTracing(ctx, TracingConfig{
		ServiceName:    config.ServiceName,
		ServiceVersion: config.ServiceVersion,
		Environment:    config.Environment,
		Endpoint:       config.TracingEndpoint,
		Protocol:       config.TracingProtocol,
		Insecure:       config.TracingInsecure,
		SampleRate:     config.TracingSampleRate,
		Enabled:        config.TracingEnabled,
	})
	if err != nil {
		logger.Error("Failed to initialize tracing", zap.Error(err))
	}

	return &Observability{
		Metrics: metrics,
		Logger:  logger,
		Tracer:  tracer,
		config:  config,
	}, nil
}

// Shutdown gracefully shuts down observability components
func (o *Observability) Shutdown(ctx context.Context) error {
	if o.Tracer != nil {
		if err := o.Tracer.Shutdown(ctx); err != nil {
			o.Logger.Error("Failed to shutdown tracer", zap.Error(err))
			return err
		}
	}
	o.Logger.Info("Observability shutdown complete")
	o.Logger.Sync()
	return nil
}

// HTTPMiddleware combines all observability middleware
func (o *Observability) HTTPMiddleware(next http.Handler) http.Handler {
	handler := next

	// Add metrics middleware
	if o.Metrics != nil {
		handler = o.Metrics.MetricsMiddleware(handler)
	}

	// Add logging middleware
	handler = LoggerMiddleware(o.Logger)(handler)

	// Add tracing middleware
	if o.Tracer != nil {
		handler = o.Tracer.HTTPMiddleware()(handler)
	}

	return handler
}

// WrapDatabase wraps database operations with observability
func (o *Observability) WrapDatabase(db *sql.DB) *ObservableDB {
	return &ObservableDB{
		DB:            db,
		observability: o,
	}
}

// ObservableDB wraps a database with observability
type ObservableDB struct {
	*sql.DB
	observability *Observability
}

// QueryContext executes a query with observability
func (db *ObservableDB) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	start := time.Now()

	// Start trace
	ctx, span := db.observability.Tracer.TraceDatabase(ctx, "query", query)
	defer span.End()

	// Log query
	logger := SpanLogger(ctx, db.observability.Logger)
	logger.Debug("Executing query", zap.String("query", query))

	// Execute query
	rows, err := db.DB.QueryContext(ctx, query, args...)

	// Record metrics
	duration := time.Since(start)
	if db.observability.Metrics != nil {
		db.observability.Metrics.RecordDatabaseQuery("query", extractTableName(query), duration, err)
	}

	// Handle error
	if err != nil {
		RecordError(ctx, err)
		logger.Error("Query failed", zap.Error(err), zap.Duration("duration", duration))
		return nil, err
	}

	// Log success
	logger.Debug("Query completed", zap.Duration("duration", duration))
	return rows, nil
}

// ExecContext executes a statement with observability
func (db *ObservableDB) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	start := time.Now()

	// Start trace
	ctx, span := db.observability.Tracer.TraceDatabase(ctx, "exec", query)
	defer span.End()

	// Log execution
	logger := SpanLogger(ctx, db.observability.Logger)
	logger.Debug("Executing statement", zap.String("query", query))

	// Execute statement
	result, err := db.DB.ExecContext(ctx, query, args...)

	// Record metrics
	duration := time.Since(start)
	if db.observability.Metrics != nil {
		db.observability.Metrics.RecordDatabaseQuery("exec", extractTableName(query), duration, err)
	}

	// Handle error
	if err != nil {
		RecordError(ctx, err)
		logger.Error("Statement execution failed", zap.Error(err), zap.Duration("duration", duration))
		return nil, err
	}

	// Log success
	logger.Debug("Statement executed", zap.Duration("duration", duration))
	return result, nil
}

// ObserveHTTPHandler wraps an HTTP handler with observability
func (o *Observability) ObserveHTTPHandler(pattern string, handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Apply all middleware
		o.HTTPMiddleware(http.HandlerFunc(handler)).ServeHTTP(w, r)
	}
}

// ObserveDeviceOperation observes a device operation
func (o *Observability) ObserveDeviceOperation(ctx context.Context, operation, deviceID string, fn func(context.Context) error) error {
	start := time.Now()

	// Start trace
	ctx, span := o.Tracer.TraceDevice(ctx, operation, deviceID)
	defer span.End()

	// Get logger with context
	logger := SpanLogger(ctx, o.Logger).WithDevice(deviceID, "")
	logger.Info("Device operation started", zap.String("operation", operation))

	// Execute operation
	err := fn(ctx)

	// Record metrics
	duration := time.Since(start)
	if o.Metrics != nil {
		if err != nil {
			o.Metrics.deviceErrors.WithLabelValues("", deviceID, operation).Inc()
		}
	}

	// Handle result
	if err != nil {
		RecordError(ctx, err)
		logger.Error("Device operation failed",
			zap.String("operation", operation),
			zap.Error(err),
			zap.Duration("duration", duration),
		)
		return err
	}

	logger.Info("Device operation completed",
		zap.String("operation", operation),
		zap.Duration("duration", duration),
	)
	return nil
}

// ObserveUpdateOperation observes an update operation
func (o *Observability) ObserveUpdateOperation(ctx context.Context, deviceID, updateType, version string, fn func(context.Context) error) error {
	start := time.Now()

	// Start trace
	ctx, span := o.Tracer.TraceUpdate(ctx, deviceID, updateType, version)
	defer span.End()

	// Get logger with context
	logger := SpanLogger(ctx, o.Logger).WithUpdate("", updateType, version)
	logger.Info("Update operation started")

	// Record update start
	if o.Metrics != nil {
		o.Metrics.RecordUpdateStart("", updateType, version)
	}

	// Execute operation
	err := fn(ctx)

	// Record metrics
	duration := time.Since(start)
	success := err == nil
	reason := ""
	if err != nil {
		reason = err.Error()
	}

	if o.Metrics != nil {
		o.Metrics.RecordUpdateComplete("", updateType, version, success, duration, reason)
	}

	// Handle result
	if err != nil {
		RecordError(ctx, err)
		logger.Error("Update operation failed",
			zap.Error(err),
			zap.Duration("duration", duration),
		)
		return err
	}

	SetStatus(ctx, codes.Ok, "Update completed successfully")
	logger.Info("Update operation completed",
		zap.Duration("duration", duration),
	)
	return nil
}

// RecordDeviceMetrics records device metrics
func (o *Observability) RecordDeviceMetrics(ctx context.Context, deviceID string, metrics map[string]interface{}) {
	// Log metrics
	logger := o.Logger.WithDevice(deviceID, "")
	logger.Debug("Recording device metrics", zap.Any("metrics", metrics))

	// Record in Prometheus
	if o.Metrics != nil {
		o.Metrics.metricsIngested.WithLabelValues(deviceID, "system").Inc()
	}

	// Add tracing event
	AddEvent(ctx, "metrics.recorded", map[string]interface{}{
		"device_id": deviceID,
		"count":     len(metrics),
	})
}

// RecordAuditLog records an audit log entry
func (o *Observability) RecordAuditLog(ctx context.Context, action, actor, resource, result string, metadata map[string]interface{}) {
	// Log audit event
	o.Logger.Audit(action, actor, resource, result, metadata)

	// Add tracing event
	AddEvent(ctx, "audit.log", map[string]interface{}{
		"action":   action,
		"actor":    actor,
		"resource": resource,
		"result":   result,
	})
}

// MonitorHealth monitors system health
func (o *Observability) MonitorHealth(ctx context.Context, interval time.Duration, healthCheck func() error) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			err := healthCheck()
			if err != nil {
				o.Logger.Warn("Health check failed", zap.Error(err))
				if o.Metrics != nil {
					o.Metrics.deviceErrors.WithLabelValues("", "system", "health_check").Inc()
				}
			}
		}
	}
}

// extractTableName attempts to extract table name from SQL query
func extractTableName(query string) string {
	// Simple extraction - would need more sophisticated parsing for production
	if len(query) > 50 {
		query = query[:50]
	}
	return "unknown"
}
