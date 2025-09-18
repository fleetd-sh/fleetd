package tracing

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// Config holds tracing configuration
type Config struct {
	Enabled        bool
	ServiceName    string
	ServiceVersion string
	Environment    string
	Endpoint       string
	Protocol       string // "grpc" or "http"
	Headers        map[string]string
	Insecure       bool
	SampleRate     float64
}

// DefaultConfig returns default tracing configuration
func DefaultConfig(serviceName string) *Config {
	return &Config{
		Enabled:        false,
		ServiceName:    serviceName,
		ServiceVersion: "unknown",
		Environment:    "development",
		Protocol:       "grpc",
		SampleRate:     1.0, // 100% sampling by default
	}
}

// LoadFromEnvironment loads tracing config from environment
func LoadFromEnvironment(serviceName string) *Config {
	config := DefaultConfig(serviceName)

	if os.Getenv("OTEL_ENABLED") == "true" || os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" {
		config.Enabled = true
	}

	if endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"); endpoint != "" {
		config.Endpoint = endpoint
	} else if endpoint := os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"); endpoint != "" {
		config.Endpoint = endpoint
	}

	if protocol := os.Getenv("OTEL_EXPORTER_OTLP_PROTOCOL"); protocol != "" {
		config.Protocol = protocol
	}

	if env := os.Getenv("DEPLOYMENT_ENV"); env != "" {
		config.Environment = env
	} else if env := os.Getenv("ENVIRONMENT"); env != "" {
		config.Environment = env
	} else if env := os.Getenv("NODE_ENV"); env != "" {
		config.Environment = env
	}

	if os.Getenv("OTEL_EXPORTER_OTLP_INSECURE") == "true" {
		config.Insecure = true
	}

	// Parse headers from environment
	if headers := os.Getenv("OTEL_EXPORTER_OTLP_HEADERS"); headers != "" {
		config.Headers = parseHeaders(headers)
	}

	// Sample rate from environment (0.0 to 1.0)
	if rate := os.Getenv("OTEL_TRACES_SAMPLER_ARG"); rate != "" {
		var sampleRate float64
		fmt.Sscanf(rate, "%f", &sampleRate)
		if sampleRate >= 0 && sampleRate <= 1 {
			config.SampleRate = sampleRate
		}
	}

	return config
}

// Initialize sets up OpenTelemetry tracing
func Initialize(config *Config) (trace.TracerProvider, func(), error) {
	if !config.Enabled {
		slog.Info("OpenTelemetry tracing disabled")
		return otel.GetTracerProvider(), func() {}, nil
	}

	ctx := context.Background()

	// Create resource with service information
	res, err := resource.New(ctx,
		resource.WithAttributes(
			attribute.String("service.name", config.ServiceName),
			attribute.String("service.version", config.ServiceVersion),
			attribute.String("deployment.environment", config.Environment),
		),
		resource.WithHost(),
		resource.WithOS(),
		resource.WithProcess(),
		resource.WithContainer(),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create resource: %w", err)
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
		if len(config.Headers) > 0 {
			opts = append(opts, otlptracehttp.WithHeaders(config.Headers))
		}
		exporter, err = otlptracehttp.New(ctx, opts...)
	} else {
		opts := []otlptracegrpc.Option{
			otlptracegrpc.WithEndpoint(config.Endpoint),
		}
		if config.Insecure {
			opts = append(opts, otlptracegrpc.WithInsecure())
		}
		if len(config.Headers) > 0 {
			opts = append(opts, otlptracegrpc.WithHeaders(config.Headers))
		}
		exporter, err = otlptracegrpc.New(ctx, opts...)
	}

	if err != nil {
		return nil, nil, fmt.Errorf("failed to create exporter: %w", err)
	}

	// Create sampler based on sample rate
	var sampler sdktrace.Sampler
	if config.SampleRate >= 1.0 {
		sampler = sdktrace.AlwaysSample()
	} else if config.SampleRate <= 0 {
		sampler = sdktrace.NeverSample()
	} else {
		sampler = sdktrace.TraceIDRatioBased(config.SampleRate)
	}

	// Create tracer provider
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)

	// Set global tracer provider
	otel.SetTracerProvider(tp)

	// Set global propagator for context propagation
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	slog.Info("OpenTelemetry tracing initialized",
		"service", config.ServiceName,
		"environment", config.Environment,
		"endpoint", config.Endpoint,
		"protocol", config.Protocol,
		"sample_rate", config.SampleRate)

	// Return shutdown function
	shutdown := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := tp.Shutdown(ctx); err != nil {
			slog.Error("Failed to shutdown tracer provider", "error", err)
		}
	}

	return tp, shutdown, nil
}

// parseHeaders parses header string in format "key1=value1,key2=value2"
func parseHeaders(headerStr string) map[string]string {
	headers := make(map[string]string)
	for _, pair := range splitByComma(headerStr) {
		if kv := splitByEqual(pair); len(kv) == 2 {
			headers[kv[0]] = kv[1]
		}
	}
	return headers
}

func splitByComma(s string) []string {
	var result []string
	var current string
	for i, r := range s {
		if r == ',' && (i == 0 || s[i-1] != '\\') {
			if current != "" {
				result = append(result, current)
				current = ""
			}
		} else {
			current += string(r)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}

func splitByEqual(s string) []string {
	for i, r := range s {
		if r == '=' {
			return []string{s[:i], s[i+1:]}
		}
	}
	return []string{s}
}

// StartSpan starts a new span with the given name
func StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	tr := otel.Tracer("fleetd")
	return tr.Start(ctx, name, opts...)
}

// SpanFromContext returns the span from context
func SpanFromContext(ctx context.Context) trace.Span {
	return trace.SpanFromContext(ctx)
}

// AddEvent adds an event to the current span
func AddEvent(ctx context.Context, name string, attrs ...attribute.KeyValue) {
	span := trace.SpanFromContext(ctx)
	span.AddEvent(name, trace.WithAttributes(attrs...))
}

// SetStatusError marks the span as having an error
func SetStatusError(ctx context.Context, description string) {
	span := trace.SpanFromContext(ctx)
	span.SetStatus(codes.Error, description)
}

// SetStatusOK marks the span as successful
func SetStatusOK(ctx context.Context) {
	span := trace.SpanFromContext(ctx)
	span.SetStatus(codes.Ok, "")
}

// RecordError records an error on the current span
func RecordError(ctx context.Context, err error, opts ...trace.EventOption) {
	span := trace.SpanFromContext(ctx)
	span.RecordError(err, opts...)
}
