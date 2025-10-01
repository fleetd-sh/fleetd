package observability

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	globalLogger *Logger
	once         sync.Once
)

type Logger struct {
	*zap.Logger
	fields []zap.Field
}

type LogConfig struct {
	Level       string // debug, info, warn, error
	Format      string // json, console
	OutputPath  string // stdout, stderr, or file path
	ServiceName string
	Environment string
	Version     string
}

// InitLogger initializes the global logger
func InitLogger(config LogConfig) *Logger {
	once.Do(func() {
		globalLogger = NewLogger(config)
	})
	return globalLogger
}

// GetLogger returns the global logger instance
func GetLogger() *Logger {
	if globalLogger == nil {
		// Initialize with defaults if not already initialized
		globalLogger = NewLogger(LogConfig{
			Level:       "info",
			Format:      "json",
			OutputPath:  "stdout",
			ServiceName: "fleetd",
			Environment: "development",
			Version:     "unknown",
		})
	}
	return globalLogger
}

// NewLogger creates a new logger instance
func NewLogger(config LogConfig) *Logger {
	// Parse log level
	level := zapcore.InfoLevel
	switch strings.ToLower(config.Level) {
	case "debug":
		level = zapcore.DebugLevel
	case "info":
		level = zapcore.InfoLevel
	case "warn", "warning":
		level = zapcore.WarnLevel
	case "error":
		level = zapcore.ErrorLevel
	}

	// Create encoder config
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "timestamp",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		FunctionKey:    "function",
		MessageKey:     "message",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	// Create encoder based on format
	var encoder zapcore.Encoder
	if config.Format == "console" {
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	} else {
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	}

	// Setup output
	var output zapcore.WriteSyncer
	switch config.OutputPath {
	case "stdout":
		output = zapcore.AddSync(os.Stdout)
	case "stderr":
		output = zapcore.AddSync(os.Stderr)
	default:
		file, err := os.OpenFile(config.OutputPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			output = zapcore.AddSync(os.Stderr)
		} else {
			output = zapcore.AddSync(file)
		}
	}

	// Create core
	core := zapcore.NewCore(encoder, output, level)

	// Create logger with options
	logger := zap.New(core,
		zap.AddCaller(),
		zap.AddStacktrace(zapcore.ErrorLevel),
		zap.AddCallerSkip(1),
	)

	// Add default fields
	defaultFields := []zap.Field{
		zap.String("service", config.ServiceName),
		zap.String("environment", config.Environment),
		zap.String("version", config.Version),
		zap.String("host", getHostname()),
		zap.Int("pid", os.Getpid()),
	}

	return &Logger{
		Logger: logger.With(defaultFields...),
		fields: defaultFields,
	}
}

// With creates a child logger with additional fields
func (l *Logger) With(fields ...zap.Field) *Logger {
	return &Logger{
		Logger: l.Logger.With(fields...),
		fields: append(l.fields, fields...),
	}
}

// WithContext creates a child logger with context fields
func (l *Logger) WithContext(ctx context.Context) *Logger {
	fields := []zap.Field{}

	// Extract common context values
	if traceID := ctx.Value("trace_id"); traceID != nil {
		fields = append(fields, zap.String("trace_id", fmt.Sprintf("%v", traceID)))
	}
	if spanID := ctx.Value("span_id"); spanID != nil {
		fields = append(fields, zap.String("span_id", fmt.Sprintf("%v", spanID)))
	}
	if userID := ctx.Value("user_id"); userID != nil {
		fields = append(fields, zap.String("user_id", fmt.Sprintf("%v", userID)))
	}
	if deviceID := ctx.Value("device_id"); deviceID != nil {
		fields = append(fields, zap.String("device_id", fmt.Sprintf("%v", deviceID)))
	}
	if fleetID := ctx.Value("fleet_id"); fleetID != nil {
		fields = append(fields, zap.String("fleet_id", fmt.Sprintf("%v", fleetID)))
	}

	return l.With(fields...)
}

// WithError adds an error field to the logger
func (l *Logger) WithError(err error) *Logger {
	return l.With(zap.Error(err))
}

// WithHTTPRequest adds HTTP request fields
func (l *Logger) WithHTTPRequest(method, path string, statusCode int, duration time.Duration) *Logger {
	return l.With(
		zap.String("http_method", method),
		zap.String("http_path", path),
		zap.Int("http_status", statusCode),
		zap.Duration("http_duration", duration),
	)
}

// WithDevice adds device-related fields
func (l *Logger) WithDevice(deviceID, deviceType string) *Logger {
	return l.With(
		zap.String("device_id", deviceID),
		zap.String("device_type", deviceType),
	)
}

// WithUpdate adds update-related fields
func (l *Logger) WithUpdate(updateID, updateType, version string) *Logger {
	return l.With(
		zap.String("update_id", updateID),
		zap.String("update_type", updateType),
		zap.String("update_version", version),
	)
}

// WithOperation adds operation tracking fields
func (l *Logger) WithOperation(operation string, startTime time.Time) *Logger {
	return l.With(
		zap.String("operation", operation),
		zap.Time("operation_start", startTime),
		zap.Duration("operation_duration", time.Since(startTime)),
	)
}

// LogPanic logs panic information and recovers
func (l *Logger) LogPanic() {
	if r := recover(); r != nil {
		// Get stack trace
		buf := make([]byte, 1<<16)
		stackSize := runtime.Stack(buf, true)

		l.Error("Panic recovered",
			zap.Any("panic", r),
			zap.String("stack", string(buf[:stackSize])),
		)

		// Re-panic if needed
		panic(r)
	}
}

// Audit logs security-sensitive operations
func (l *Logger) Audit(action string, actor string, resource string, result string, metadata map[string]interface{}) {
	fields := []zap.Field{
		zap.String("audit_action", action),
		zap.String("audit_actor", actor),
		zap.String("audit_resource", resource),
		zap.String("audit_result", result),
		zap.Any("audit_metadata", metadata),
		zap.Time("audit_timestamp", time.Now()),
	}

	l.With(fields...).Info("Audit log")
}

// Performance logs performance metrics
func (l *Logger) Performance(operation string, duration time.Duration, metadata map[string]interface{}) {
	fields := []zap.Field{
		zap.String("perf_operation", operation),
		zap.Duration("perf_duration", duration),
		zap.Any("perf_metadata", metadata),
	}

	// Log as warning if operation took too long
	if duration > 5*time.Second {
		l.With(fields...).Warn("Slow operation detected")
	} else {
		l.With(fields...).Debug("Performance metric")
	}
}

// getHostname returns the hostname
func getHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return hostname
}

// LoggerMiddleware provides HTTP middleware for request logging
func LoggerMiddleware(logger *Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Create request-scoped logger
			reqLogger := logger.With(
				zap.String("request_id", generateRequestID()),
				zap.String("remote_addr", r.RemoteAddr),
				zap.String("user_agent", r.UserAgent()),
			)

			// Log request start
			reqLogger.Info("Request started",
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.String("query", r.URL.RawQuery),
			)

			// Wrap response writer
			wrapped := &loggingResponseWriter{ResponseWriter: w}

			// Add logger to context
			ctx := context.WithValue(r.Context(), "logger", reqLogger)
			r = r.WithContext(ctx)

			// Handle request
			next.ServeHTTP(wrapped, r)

			// Log request completion
			duration := time.Since(start)
			reqLogger.Info("Request completed",
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.Int("status", wrapped.statusCode),
				zap.Int("bytes", wrapped.bytes),
				zap.Duration("duration", duration),
			)

			// Log slow requests as warnings
			if duration > 1*time.Second {
				reqLogger.Warn("Slow request",
					zap.Duration("duration", duration),
				)
			}
		})
	}
}

type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
	bytes      int
}

func (lrw *loggingResponseWriter) WriteHeader(code int) {
	lrw.statusCode = code
	lrw.ResponseWriter.WriteHeader(code)
}

func (lrw *loggingResponseWriter) Write(b []byte) (int, error) {
	n, err := lrw.ResponseWriter.Write(b)
	lrw.bytes += n
	return n, err
}

func generateRequestID() string {
	return fmt.Sprintf("%d-%d", time.Now().UnixNano(), os.Getpid())
}

// ContextLogger extracts logger from context
func ContextLogger(ctx context.Context) *Logger {
	if logger, ok := ctx.Value("logger").(*Logger); ok {
		return logger
	}
	return GetLogger()
}