package observability

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
)

type TracingConfig struct {
	ServiceName    string
	ServiceVersion string
	Environment    string
	Endpoint       string // OTLP endpoint (e.g., "localhost:4317" for gRPC or "localhost:4318" for HTTP)
	Protocol       string // "grpc" or "http"
	Insecure       bool   // Use insecure connection (for development)
	SampleRate     float64
	Enabled        bool
}

type Tracer struct {
	tracer   trace.Tracer
	provider *sdktrace.TracerProvider
}

// InitTracing initializes OpenTelemetry tracing
func InitTracing(ctx context.Context, config TracingConfig) (*Tracer, error) {
	if !config.Enabled {
		return &Tracer{tracer: otel.Tracer(config.ServiceName)}, nil
	}

	// Create resource
	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(config.ServiceName),
			semconv.ServiceVersionKey.String(config.ServiceVersion),
			semconv.DeploymentEnvironmentKey.String(config.Environment),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Create exporter based on protocol
	var exporter *otlptrace.Exporter
	if config.Protocol == "http" {
		opts := []otlptracehttp.Option{
			otlptracehttp.WithEndpoint(config.Endpoint),
		}
		if config.Insecure {
			opts = append(opts, otlptracehttp.WithInsecure())
		}
		exporter, err = otlptracehttp.New(ctx, opts...)
	} else {
		opts := []otlptracegrpc.Option{
			otlptracegrpc.WithEndpoint(config.Endpoint),
		}
		if config.Insecure {
			opts = append(opts, otlptracegrpc.WithInsecure())
		}
		exporter, err = otlptracegrpc.New(ctx, opts...)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create exporter: %w", err)
	}

	// Create sampler
	sampler := sdktrace.TraceIDRatioBased(config.SampleRate)

	// Create trace provider
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)

	// Set global tracer provider
	otel.SetTracerProvider(provider)

	// Set global propagator
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return &Tracer{
		tracer:   otel.Tracer(config.ServiceName),
		provider: provider,
	}, nil
}

// Shutdown shuts down the tracer provider
func (t *Tracer) Shutdown(ctx context.Context) error {
	if t.provider != nil {
		return t.provider.Shutdown(ctx)
	}
	return nil
}

// Start starts a new span
func (t *Tracer) Start(ctx context.Context, spanName string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return t.tracer.Start(ctx, spanName, opts...)
}

// StartSpan starts a new span with common attributes
func (t *Tracer) StartSpan(ctx context.Context, operation string, attributes map[string]interface{}) (context.Context, trace.Span) {
	opts := []trace.SpanStartOption{
		trace.WithSpanKind(trace.SpanKindInternal),
	}

	// Convert attributes
	var attrs []attribute.KeyValue
	for k, v := range attributes {
		attrs = append(attrs, attributeFromValue(k, v))
	}
	opts = append(opts, trace.WithAttributes(attrs...))

	return t.Start(ctx, operation, opts...)
}

// TraceHTTPRequest traces an HTTP request
func (t *Tracer) TraceHTTPRequest(ctx context.Context, req *http.Request) (context.Context, trace.Span) {
	spanName := fmt.Sprintf("%s %s", req.Method, req.URL.Path)
	ctx, span := t.Start(ctx, spanName,
		trace.WithSpanKind(trace.SpanKindServer),
		trace.WithAttributes(
			semconv.HTTPMethodKey.String(req.Method),
			semconv.HTTPTargetKey.String(req.URL.Path),
			semconv.HTTPURLKey.String(req.URL.String()),
			semconv.HTTPUserAgentKey.String(req.UserAgent()),
			semconv.HTTPHostKey.String(req.Host),
		),
	)

	return ctx, span
}

// TraceDatabase traces a database operation
func (t *Tracer) TraceDatabase(ctx context.Context, operation, query string) (context.Context, trace.Span) {
	return t.Start(ctx, fmt.Sprintf("db.%s", operation),
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("db.operation", operation),
			attribute.String("db.statement", query),
		),
	)
}

// TraceUpdate traces an update operation
func (t *Tracer) TraceUpdate(ctx context.Context, deviceID, updateType, version string) (context.Context, trace.Span) {
	return t.Start(ctx, fmt.Sprintf("update.%s", updateType),
		trace.WithAttributes(
			attribute.String("device.id", deviceID),
			attribute.String("update.type", updateType),
			attribute.String("update.version", version),
		),
	)
}

// TraceDevice traces a device operation
func (t *Tracer) TraceDevice(ctx context.Context, operation, deviceID string) (context.Context, trace.Span) {
	return t.Start(ctx, fmt.Sprintf("device.%s", operation),
		trace.WithAttributes(
			attribute.String("device.id", deviceID),
			attribute.String("device.operation", operation),
		),
	)
}

// RecordError records an error on the current span
func RecordError(ctx context.Context, err error) {
	span := trace.SpanFromContext(ctx)
	if span != nil && err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
}

// SetStatus sets the status of the current span
func SetStatus(ctx context.Context, code codes.Code, description string) {
	span := trace.SpanFromContext(ctx)
	if span != nil {
		span.SetStatus(code, description)
	}
}

// AddEvent adds an event to the current span
func AddEvent(ctx context.Context, name string, attributes map[string]interface{}) {
	span := trace.SpanFromContext(ctx)
	if span != nil {
		var attrs []attribute.KeyValue
		for k, v := range attributes {
			attrs = append(attrs, attributeFromValue(k, v))
		}
		span.AddEvent(name, trace.WithAttributes(attrs...))
	}
}

// SetAttributes sets attributes on the current span
func SetAttributes(ctx context.Context, attributes map[string]interface{}) {
	span := trace.SpanFromContext(ctx)
	if span != nil {
		var attrs []attribute.KeyValue
		for k, v := range attributes {
			attrs = append(attrs, attributeFromValue(k, v))
		}
		span.SetAttributes(attrs...)
	}
}

// HTTPMiddleware creates HTTP middleware for tracing
func (t *Tracer) HTTPMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return otelhttp.NewHandler(next, "",
			otelhttp.WithTracerProvider(otel.GetTracerProvider()),
			otelhttp.WithPropagators(otel.GetTextMapPropagator()),
		)
	}
}

// GRPCUnaryInterceptor creates a gRPC unary interceptor for tracing
func (t *Tracer) GRPCUnaryInterceptor() grpc.UnaryServerInterceptor {
	return otelgrpc.UnaryServerInterceptor(
		otelgrpc.WithTracerProvider(otel.GetTracerProvider()),
		otelgrpc.WithPropagators(otel.GetTextMapPropagator()),
	)
}

// GRPCStreamInterceptor creates a gRPC stream interceptor for tracing
func (t *Tracer) GRPCStreamInterceptor() grpc.StreamServerInterceptor {
	return otelgrpc.StreamServerInterceptor(
		otelgrpc.WithTracerProvider(otel.GetTracerProvider()),
		otelgrpc.WithPropagators(otel.GetTextMapPropagator()),
	)
}

// TraceFunc wraps a function with tracing
func (t *Tracer) TraceFunc(ctx context.Context, name string, fn func(context.Context) error) error {
	ctx, span := t.Start(ctx, name)
	defer span.End()

	err := fn(ctx)
	if err != nil {
		RecordError(ctx, err)
	}
	return err
}

// TraceFuncWithResult wraps a function with tracing and result
func (t *Tracer) TraceFuncWithResult(ctx context.Context, name string, fn func(context.Context) (interface{}, error)) (interface{}, error) {
	ctx, span := t.Start(ctx, name)
	defer span.End()

	result, err := fn(ctx)
	if err != nil {
		RecordError(ctx, err)
	}
	return result, err
}

// MeasureLatency measures the latency of an operation
func MeasureLatency(ctx context.Context, operation string) func() {
	start := time.Now()
	return func() {
		duration := time.Since(start)
		SetAttributes(ctx, map[string]interface{}{
			fmt.Sprintf("%s.duration_ms", operation): duration.Milliseconds(),
		})
	}
}

// attributeFromValue converts a value to an attribute
func attributeFromValue(key string, value interface{}) attribute.KeyValue {
	switch v := value.(type) {
	case string:
		return attribute.String(key, v)
	case int:
		return attribute.Int(key, v)
	case int64:
		return attribute.Int64(key, v)
	case float64:
		return attribute.Float64(key, v)
	case bool:
		return attribute.Bool(key, v)
	case []string:
		return attribute.StringSlice(key, v)
	case []int:
		return attribute.IntSlice(key, v)
	case []int64:
		return attribute.Int64Slice(key, v)
	case []float64:
		return attribute.Float64Slice(key, v)
	case []bool:
		return attribute.BoolSlice(key, v)
	default:
		return attribute.String(key, fmt.Sprintf("%v", v))
	}
}

// SpanLogger creates a logger with span context
func SpanLogger(ctx context.Context, logger *Logger) *Logger {
	span := trace.SpanFromContext(ctx)
	if span != nil {
		spanCtx := span.SpanContext()
		return logger.With(
			zap.String("trace_id", spanCtx.TraceID().String()),
			zap.String("span_id", spanCtx.SpanID().String()),
		)
	}
	return logger
}