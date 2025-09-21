package observability

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	promClient "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	runtimemetrics "go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Config holds observability configuration
type Config struct {
	ServiceName     string
	ServiceVersion  string
	Environment     string
	OTLPEndpoint    string
	MetricsPort     int
	EnableTracing   bool
	EnableMetrics   bool
	EnableProfiling bool
	SampleRate      float64
}

// DefaultConfig returns default observability configuration
func DefaultConfig() *Config {
	return &Config{
		ServiceName:    "fleetd",
		ServiceVersion: "1.0.0",
		Environment:    "development",
		OTLPEndpoint:   "localhost:4317",
		MetricsPort:    9090,
		EnableTracing:  true,
		EnableMetrics:  true,
		SampleRate:     1.0,
	}
}

// Provider manages observability resources
type Provider struct {
	config         *Config
	tracerProvider *sdktrace.TracerProvider
	meterProvider  *sdkmetric.MeterProvider
	logger         *slog.Logger
	metrics        *Metrics
	shutdown       []func(context.Context) error
}

// Metrics holds application metrics
type Metrics struct {
	// HTTP metrics
	HTTPRequestDuration   metric.Float64Histogram
	HTTPRequestsTotal     metric.Int64Counter
	HTTPRequestsInFlight  metric.Int64UpDownCounter
	HTTPResponseSizeBytes metric.Int64Histogram

	// RPC metrics
	RPCDuration      metric.Float64Histogram
	RPCRequestsTotal metric.Int64Counter
	RPCErrorsTotal   metric.Int64Counter

	// Device metrics
	DevicesTotal        metric.Int64UpDownCounter
	DevicesOnline       metric.Int64UpDownCounter
	DeviceRegistrations metric.Int64Counter
	DeviceHeartbeats    metric.Int64Counter

	// Process metrics
	ProcessesRunning metric.Int64UpDownCounter
	ProcessRestarts  metric.Int64Counter
	ProcessCrashes   metric.Int64Counter

	// Database metrics
	DBConnectionsOpen metric.Int64UpDownCounter
	DBQueriesTotal    metric.Int64Counter
	DBQueryDuration   metric.Float64Histogram
	DBErrorsTotal     metric.Int64Counter

	// Circuit breaker metrics
	CircuitBreakerState metric.Int64UpDownCounter
	CircuitBreakerTrips metric.Int64Counter

	// Business metrics
	DeploymentsTotal  metric.Int64Counter
	DeploymentsFailed metric.Int64Counter
	UpdatesDownloaded metric.Int64Counter
	UpdatesApplied    metric.Int64Counter
}

// NewProvider creates a new observability provider
func NewProvider(ctx context.Context, config *Config) (*Provider, error) {
	if config == nil {
		config = DefaultConfig()
	}

	provider := &Provider{
		config:   config,
		logger:   slog.Default().With("component", "observability"),
		shutdown: make([]func(context.Context) error, 0),
	}

	// Setup resource
	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(config.ServiceName),
			semconv.ServiceVersion(config.ServiceVersion),
			semconv.DeploymentEnvironment(config.Environment),
			attribute.String("service.namespace", "fleetd"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Setup tracing
	if config.EnableTracing {
		if err := provider.setupTracing(ctx, res); err != nil {
			return nil, fmt.Errorf("failed to setup tracing: %w", err)
		}
	}

	// Setup metrics
	if config.EnableMetrics {
		if err := provider.setupMetrics(ctx, res); err != nil {
			return nil, fmt.Errorf("failed to setup metrics: %w", err)
		}
	}

	// Setup runtime metrics
	if err := runtimemetrics.Start(runtimemetrics.WithMinimumReadMemStatsInterval(time.Second)); err != nil {
		provider.logger.Warn("Failed to start runtime metrics", "error", err)
	}

	provider.logger.Info("Observability provider initialized",
		"tracing", config.EnableTracing,
		"metrics", config.EnableMetrics,
		"endpoint", config.OTLPEndpoint,
	)

	return provider, nil
}

func (p *Provider) setupTracing(ctx context.Context, res *resource.Resource) error {
	// Create OTLP exporter
	conn, err := grpc.DialContext(ctx, p.config.OTLPEndpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return fmt.Errorf("failed to create gRPC connection: %w", err)
	}

	exporter, err := otlptrace.New(ctx, otlptracegrpc.NewClient(
		otlptracegrpc.WithGRPCConn(conn),
	))
	if err != nil {
		return fmt.Errorf("failed to create trace exporter: %w", err)
	}

	// Create tracer provider
	p.tracerProvider = sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.TraceIDRatioBased(p.config.SampleRate)),
	)

	// Register as global provider
	otel.SetTracerProvider(p.tracerProvider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	p.shutdown = append(p.shutdown, p.tracerProvider.Shutdown)
	return nil
}

func (p *Provider) setupMetrics(ctx context.Context, res *resource.Resource) error {
	// Create Prometheus exporter
	promExporter, err := prometheus.New()
	if err != nil {
		return fmt.Errorf("failed to create prometheus exporter: %w", err)
	}

	// Create meter provider
	p.meterProvider = sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(promExporter),
		sdkmetric.WithResource(res),
	)

	// Register as global provider
	otel.SetMeterProvider(p.meterProvider)

	// Create metrics
	if err := p.createMetrics(); err != nil {
		return fmt.Errorf("failed to create metrics: %w", err)
	}

	// Start metrics server
	go p.serveMetrics()

	p.shutdown = append(p.shutdown, p.meterProvider.Shutdown)
	return nil
}

func (p *Provider) createMetrics() error {
	meter := p.meterProvider.Meter("fleetd")
	m := &Metrics{}

	var err error

	// HTTP metrics
	m.HTTPRequestDuration, err = meter.Float64Histogram(
		"http_request_duration_seconds",
		metric.WithDescription("HTTP request duration in seconds"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return err
	}

	m.HTTPRequestsTotal, err = meter.Int64Counter(
		"http_requests_total",
		metric.WithDescription("Total number of HTTP requests"),
	)
	if err != nil {
		return err
	}

	m.HTTPRequestsInFlight, err = meter.Int64UpDownCounter(
		"http_requests_in_flight",
		metric.WithDescription("Number of HTTP requests currently being processed"),
	)
	if err != nil {
		return err
	}

	m.HTTPResponseSizeBytes, err = meter.Int64Histogram(
		"http_response_size_bytes",
		metric.WithDescription("HTTP response size in bytes"),
		metric.WithUnit("By"),
	)
	if err != nil {
		return err
	}

	// RPC metrics
	m.RPCDuration, err = meter.Float64Histogram(
		"rpc_duration_seconds",
		metric.WithDescription("RPC request duration in seconds"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return err
	}

	m.RPCRequestsTotal, err = meter.Int64Counter(
		"rpc_requests_total",
		metric.WithDescription("Total number of RPC requests"),
	)
	if err != nil {
		return err
	}

	m.RPCErrorsTotal, err = meter.Int64Counter(
		"rpc_errors_total",
		metric.WithDescription("Total number of RPC errors"),
	)
	if err != nil {
		return err
	}

	// Device metrics
	m.DevicesTotal, err = meter.Int64UpDownCounter(
		"devices_total",
		metric.WithDescription("Total number of registered devices"),
	)
	if err != nil {
		return err
	}

	m.DevicesOnline, err = meter.Int64UpDownCounter(
		"devices_online",
		metric.WithDescription("Number of online devices"),
	)
	if err != nil {
		return err
	}

	m.DeviceRegistrations, err = meter.Int64Counter(
		"device_registrations_total",
		metric.WithDescription("Total number of device registrations"),
	)
	if err != nil {
		return err
	}

	m.DeviceHeartbeats, err = meter.Int64Counter(
		"device_heartbeats_total",
		metric.WithDescription("Total number of device heartbeats"),
	)
	if err != nil {
		return err
	}

	// Process metrics
	m.ProcessesRunning, err = meter.Int64UpDownCounter(
		"processes_running",
		metric.WithDescription("Number of running processes"),
	)
	if err != nil {
		return err
	}

	m.ProcessRestarts, err = meter.Int64Counter(
		"process_restarts_total",
		metric.WithDescription("Total number of process restarts"),
	)
	if err != nil {
		return err
	}

	m.ProcessCrashes, err = meter.Int64Counter(
		"process_crashes_total",
		metric.WithDescription("Total number of process crashes"),
	)
	if err != nil {
		return err
	}

	// Database metrics
	m.DBConnectionsOpen, err = meter.Int64UpDownCounter(
		"db_connections_open",
		metric.WithDescription("Number of open database connections"),
	)
	if err != nil {
		return err
	}

	m.DBQueriesTotal, err = meter.Int64Counter(
		"db_queries_total",
		metric.WithDescription("Total number of database queries"),
	)
	if err != nil {
		return err
	}

	m.DBQueryDuration, err = meter.Float64Histogram(
		"db_query_duration_seconds",
		metric.WithDescription("Database query duration in seconds"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return err
	}

	m.DBErrorsTotal, err = meter.Int64Counter(
		"db_errors_total",
		metric.WithDescription("Total number of database errors"),
	)
	if err != nil {
		return err
	}

	// Circuit breaker metrics
	m.CircuitBreakerState, err = meter.Int64UpDownCounter(
		"circuit_breaker_state",
		metric.WithDescription("Circuit breaker state (0=closed, 1=open, 2=half-open)"),
	)
	if err != nil {
		return err
	}

	m.CircuitBreakerTrips, err = meter.Int64Counter(
		"circuit_breaker_trips_total",
		metric.WithDescription("Total number of circuit breaker trips"),
	)
	if err != nil {
		return err
	}

	// Business metrics
	m.DeploymentsTotal, err = meter.Int64Counter(
		"deployments_total",
		metric.WithDescription("Total number of deployments"),
	)
	if err != nil {
		return err
	}

	m.DeploymentsFailed, err = meter.Int64Counter(
		"deployments_failed_total",
		metric.WithDescription("Total number of failed deployments"),
	)
	if err != nil {
		return err
	}

	m.UpdatesDownloaded, err = meter.Int64Counter(
		"updates_downloaded_total",
		metric.WithDescription("Total number of updates downloaded"),
	)
	if err != nil {
		return err
	}

	m.UpdatesApplied, err = meter.Int64Counter(
		"updates_applied_total",
		metric.WithDescription("Total number of updates applied"),
	)
	if err != nil {
		return err
	}

	p.metrics = m
	return nil
}

func (p *Provider) serveMetrics() {
	mux := http.NewServeMux()

	// Prometheus metrics endpoint
	mux.Handle("/metrics", promhttp.HandlerFor(
		promClient.DefaultGatherer,
		promhttp.HandlerOpts{
			EnableOpenMetrics: true,
		},
	))

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Ready check endpoint
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		// Check if metrics are being collected
		if p.meterProvider == nil && p.config.EnableMetrics {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("Metrics not ready"))
			return
		}

		// Check if tracing is initialized
		if p.tracerProvider == nil && p.config.EnableTracing {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("Tracing not ready"))
			return
		}

		// All checks passed
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Ready"))
	})

	addr := fmt.Sprintf(":%d", p.config.MetricsPort)
	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	p.logger.Info("Metrics server started", "port", p.config.MetricsPort)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		p.logger.Error("Metrics server failed", "error", err)
	}
}

// Shutdown gracefully shuts down the provider
func (p *Provider) Shutdown(ctx context.Context) error {
	for _, fn := range p.shutdown {
		if err := fn(ctx); err != nil {
			p.logger.Error("Shutdown error", "error", err)
		}
	}
	return nil
}

// Tracer returns a tracer for the given name
func (p *Provider) Tracer(name string) trace.Tracer {
	return otel.Tracer(name)
}

// Meter returns a meter for the given name
func (p *Provider) Meter(name string) metric.Meter {
	return otel.Meter(name)
}

// Metrics returns the metrics instance
func (p *Provider) Metrics() *Metrics {
	return p.metrics
}

// RecordHTTPRequest records HTTP request metrics
func (m *Metrics) RecordHTTPRequest(ctx context.Context, method, path string, statusCode int, duration time.Duration, responseSize int64) {
	attrs := []attribute.KeyValue{
		attribute.String("method", method),
		attribute.String("path", path),
		attribute.Int("status_code", statusCode),
	}

	m.HTTPRequestsTotal.Add(ctx, 1, metric.WithAttributes(attrs...))
	m.HTTPRequestDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(attrs...))
	m.HTTPResponseSizeBytes.Record(ctx, responseSize, metric.WithAttributes(attrs...))
}

// RecordRPCRequest records RPC request metrics
func (m *Metrics) RecordRPCRequest(ctx context.Context, method string, success bool, duration time.Duration) {
	attrs := []attribute.KeyValue{
		attribute.String("method", method),
		attribute.Bool("success", success),
	}

	m.RPCRequestsTotal.Add(ctx, 1, metric.WithAttributes(attrs...))
	m.RPCDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(attrs...))

	if !success {
		m.RPCErrorsTotal.Add(ctx, 1, metric.WithAttributes(attrs...))
	}
}

// RecordDBQuery records database query metrics
func (m *Metrics) RecordDBQuery(ctx context.Context, query string, success bool, duration time.Duration) {
	attrs := []attribute.KeyValue{
		attribute.String("query_type", extractQueryType(query)),
		attribute.Bool("success", success),
	}

	m.DBQueriesTotal.Add(ctx, 1, metric.WithAttributes(attrs...))
	m.DBQueryDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(attrs...))

	if !success {
		m.DBErrorsTotal.Add(ctx, 1, metric.WithAttributes(attrs...))
	}
}

// extractQueryType extracts the query type from SQL
func extractQueryType(query string) string {
	if len(query) > 6 {
		return query[:6]
	}
	return "unknown"
}

// StartSpan starts a new span
func StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return otel.Tracer("fleetd").Start(ctx, name, opts...)
}

// AddEvent adds an event to the current span
func AddEvent(ctx context.Context, name string, attrs ...attribute.KeyValue) {
	span := trace.SpanFromContext(ctx)
	span.AddEvent(name, trace.WithAttributes(attrs...))
}

// RecordError records an error on the current span
func RecordError(ctx context.Context, err error, attrs ...attribute.KeyValue) {
	span := trace.SpanFromContext(ctx)
	span.RecordError(err, trace.WithAttributes(attrs...))
}

// SetAttributes sets attributes on the current span
func SetAttributes(ctx context.Context, attrs ...attribute.KeyValue) {
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attrs...)
}

// GetTraceID returns the trace ID from the context
func GetTraceID(ctx context.Context) string {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().HasTraceID() {
		return span.SpanContext().TraceID().String()
	}
	return ""
}

// GetSpanID returns the span ID from the context
func GetSpanID(ctx context.Context) string {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().HasSpanID() {
		return span.SpanContext().SpanID().String()
	}
	return ""
}
