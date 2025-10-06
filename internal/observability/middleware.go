package observability

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// HTTPMiddleware creates HTTP middleware with tracing and metrics
func HTTPMiddleware(provider *Observability) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract trace context from headers
			ctx := propagation.TraceContext{}.Extract(r.Context(), propagation.HeaderCarrier(r.Header))

			// Start span
			ctx, span := provider.Tracer.tracer.Start(ctx, fmt.Sprintf("%s %s", r.Method, r.URL.Path),
				trace.WithSpanKind(trace.SpanKindServer),
				trace.WithAttributes(
					attribute.String("http.method", r.Method),
					attribute.String("http.url", r.URL.String()),
					attribute.String("http.scheme", r.URL.Scheme),
					attribute.String("http.host", r.Host),
					attribute.String("http.user_agent", r.UserAgent()),
					attribute.String("http.remote_addr", r.RemoteAddr),
				),
			)
			defer span.End()

			// TODO: Add HTTPRequestsInFlight metric when available
			// provider.Metrics.HTTPRequestsInFlight.Add(ctx, 1)
			// defer provider.Metrics.HTTPRequestsInFlight.Add(ctx, -1)

			// Wrap response writer to capture status and size
			wrapped := &responseWriter{
				ResponseWriter: w,
				status:         http.StatusOK,
				bytes:          0,
			}

			// Inject trace context into response headers
			propagation.TraceContext{}.Inject(ctx, propagation.HeaderCarrier(w.Header()))

			// TODO: Add GetTraceID function or remove this
			// if traceID := GetTraceID(ctx); traceID != "" {
			// 	w.Header().Set("X-Trace-ID", traceID)
			// }

			// Record start time
			startTime := time.Now()

			// Call next handler
			next.ServeHTTP(wrapped, r.WithContext(ctx))

			// Record metrics
			duration := time.Since(startTime)
			statusStr := http.StatusText(wrapped.status)
			provider.Metrics.RecordHTTPRequest(r.Method, r.URL.Path, statusStr, duration, wrapped.bytes)

			// Set span status based on HTTP status code
			span.SetAttributes(
				attribute.Int("http.status_code", wrapped.status),
				attribute.Int64("http.response_size", int64(wrapped.bytes)),
			)

			if wrapped.status >= 400 {
				span.SetStatus(codes.Error, http.StatusText(wrapped.status))
			} else {
				span.SetStatus(codes.Ok, "")
			}
		})
	}
}

// ConnectUnaryInterceptor creates a Connect unary interceptor with tracing and metrics
func ConnectUnaryInterceptor(provider *Observability) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			// Extract trace context from headers
			ctx = propagation.TraceContext{}.Extract(ctx, propagation.HeaderCarrier(req.Header()))

			// Start span
			procedure := req.Spec().Procedure
			ctx, span := provider.Tracer.tracer.Start(ctx, procedure,
				trace.WithSpanKind(trace.SpanKindServer),
				trace.WithAttributes(
					attribute.String("rpc.system", "connect"),
					attribute.String("rpc.service", req.Spec().Procedure),
					attribute.String("rpc.method", procedure),
				),
			)
			defer span.End()

			// TODO: Add RecordRPCRequest method when available
			// startTime := time.Now()

			// Call next handler
			resp, err := next(ctx, req)

			// Record metrics when available
			// duration := time.Since(startTime)
			// success := err == nil
			// provider.Metrics.RecordRPCRequest(ctx, procedure, success, duration)

			// Set span status
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())

				// Add error details
				if connectErr, ok := err.(*connect.Error); ok {
					span.SetAttributes(
						attribute.String("rpc.connect.error_code", connectErr.Code().String()),
						attribute.String("rpc.connect.error_message", connectErr.Message()),
					)
				}
			} else {
				span.SetStatus(codes.Ok, "")
			}

			// Inject trace context into response headers if available
			if resp != nil {
				propagation.TraceContext{}.Inject(ctx, propagation.HeaderCarrier(resp.Header()))

				// TODO: Add GetTraceID function or remove this
				// if traceID := GetTraceID(ctx); traceID != "" {
				// 	resp.Header().Set("X-Trace-ID", traceID)
				// }
			}

			return resp, err
		}
	}
}

// DatabaseMiddleware wraps database operations with tracing
func DatabaseMiddleware(provider *Observability) func(ctx context.Context, query string, fn func() error) error {
	return func(ctx context.Context, query string, fn func() error) error {
		// Start span
		ctx, span := provider.Tracer.tracer.Start(ctx, "db.query",
			trace.WithAttributes(
				attribute.String("db.statement", truncateQuery(query)),
				attribute.String("db.system", "postgres"),
			),
		)
		defer span.End()

		// TODO: Add RecordDBQuery method when available
		// startTime := time.Now()

		// Execute query
		err := fn()

		// Record metrics when available
		// duration := time.Since(startTime)
		// success := err == nil
		// provider.Metrics.RecordDBQuery(ctx, query, success, duration)

		// Set span status
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		} else {
			span.SetStatus(codes.Ok, "")
		}

		return err
	}
}

// responseWriter is defined in metrics.go

// truncateQuery truncates long queries for attributes
func truncateQuery(query string) string {
	const maxLen = 1000
	if len(query) > maxLen {
		return query[:maxLen] + "..."
	}
	return query
}

// TracedHandler wraps an HTTP handler with tracing
func TracedHandler(provider *Observability, name string, handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := provider.Tracer.tracer.Start(r.Context(), name,
			trace.WithSpanKind(trace.SpanKindInternal),
		)
		defer span.End()

		handler(w, r.WithContext(ctx))
	}
}

// TraceFunction wraps a function with tracing
func TraceFunction(ctx context.Context, provider *Observability, name string, fn func(context.Context) error) error {
	ctx, span := provider.Tracer.tracer.Start(ctx, name,
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()

	err := fn(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	} else {
		span.SetStatus(codes.Ok, "")
	}

	return err
}
