package middleware

import (
	"context"
	"fmt"
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// NewTracingMiddleware creates a tracing middleware for HTTP handlers
func NewTracingMiddleware(serviceName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		// Use otelhttp for automatic instrumentation
		handler := otelhttp.NewHandler(next, "",
			otelhttp.WithTracerProvider(otel.GetTracerProvider()),
			otelhttp.WithPropagators(otel.GetTextMapPropagator()),
			otelhttp.WithSpanNameFormatter(func(operation string, r *http.Request) string {
				return fmt.Sprintf("%s %s", r.Method, cleanPath(r.URL.Path))
			}),
			otelhttp.WithSpanOptions(
				trace.WithAttributes(
					attribute.String("service.name", serviceName),
				),
			),
			otelhttp.WithMessageEvents(otelhttp.ReadEvents, otelhttp.WriteEvents),
		)

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handler.ServeHTTP(w, r)
		})
	}
}

// ExtractTraceContext extracts trace context from HTTP request
func ExtractTraceContext(r *http.Request) (trace.SpanContext, bool) {
	ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
	spanCtx := trace.SpanContextFromContext(ctx)
	return spanCtx, spanCtx.IsValid()
}

// InjectTraceContext injects trace context into HTTP headers
func InjectTraceContext(spanCtx trace.SpanContext, headers http.Header) {
	carrier := propagation.HeaderCarrier(headers)
	ctx := trace.ContextWithSpanContext(context.Background(), spanCtx)
	otel.GetTextMapPropagator().Inject(ctx, carrier)
}

// TracingRoundTripper wraps an http.RoundTripper with tracing
type TracingRoundTripper struct {
	transport http.RoundTripper
}

// NewTracingRoundTripper creates a new tracing round tripper
func NewTracingRoundTripper(transport http.RoundTripper) *TracingRoundTripper {
	if transport == nil {
		transport = http.DefaultTransport
	}
	return &TracingRoundTripper{
		transport: transport,
	}
}

// RoundTrip implements http.RoundTripper with tracing
func (t *TracingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// Use otelhttp transport for automatic instrumentation
	transport := otelhttp.NewTransport(t.transport,
		otelhttp.WithTracerProvider(otel.GetTracerProvider()),
		otelhttp.WithPropagators(otel.GetTextMapPropagator()),
		otelhttp.WithSpanOptions(
			trace.WithAttributes(
				attribute.String("http.method", req.Method),
				attribute.String("http.url", req.URL.String()),
			),
		),
	)

	return transport.RoundTrip(req)
}

// NewTracingHTTPClient creates an HTTP client with tracing enabled
func NewTracingHTTPClient() *http.Client {
	return &http.Client{
		Transport: NewTracingRoundTripper(http.DefaultTransport),
	}
}
