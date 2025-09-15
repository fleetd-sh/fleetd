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
func HTTPMiddleware(provider *Provider) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract trace context from headers
			ctx := propagation.TraceContext{}.Extract(r.Context(), propagation.HeaderCarrier(r.Header))

			// Start span
			ctx, span := provider.Tracer("http").Start(ctx, fmt.Sprintf("%s %s", r.Method, r.URL.Path),
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

			// Track request in flight
			provider.metrics.HTTPRequestsInFlight.Add(ctx, 1)
			defer provider.metrics.HTTPRequestsInFlight.Add(ctx, -1)

			// Wrap response writer to capture status and size
			wrapped := &responseWriter{
				ResponseWriter: w,
				statusCode:     http.StatusOK,
				written:        0,
			}

			// Inject trace context into response headers
			propagation.TraceContext{}.Inject(ctx, propagation.HeaderCarrier(w.Header()))

			// Add trace ID to response headers
			if traceID := GetTraceID(ctx); traceID != "" {
				w.Header().Set("X-Trace-ID", traceID)
			}

			// Record start time
			startTime := time.Now()

			// Call next handler
			next.ServeHTTP(wrapped, r.WithContext(ctx))

			// Record metrics
			duration := time.Since(startTime)
			provider.metrics.RecordHTTPRequest(ctx, r.Method, r.URL.Path, wrapped.statusCode, duration, wrapped.written)

			// Set span status based on HTTP status code
			span.SetAttributes(
				attribute.Int("http.status_code", wrapped.statusCode),
				attribute.Int64("http.response_size", wrapped.written),
			)

			if wrapped.statusCode >= 400 {
				span.SetStatus(codes.Error, http.StatusText(wrapped.statusCode))
			} else {
				span.SetStatus(codes.Ok, "")
			}
		})
	}
}

// ConnectUnaryInterceptor creates a Connect unary interceptor with tracing and metrics
func ConnectUnaryInterceptor(provider *Provider) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			// Extract trace context from headers
			ctx = propagation.TraceContext{}.Extract(ctx, propagation.HeaderCarrier(req.Header()))

			// Start span
			procedure := req.Spec().Procedure
			ctx, span := provider.Tracer("connect").Start(ctx, procedure,
				trace.WithSpanKind(trace.SpanKindServer),
				trace.WithAttributes(
					attribute.String("rpc.system", "connect"),
					attribute.String("rpc.service", req.Spec().Procedure),
					attribute.String("rpc.method", procedure),
				),
			)
			defer span.End()

			// Record start time
			startTime := time.Now()

			// Call next handler
			resp, err := next(ctx, req)

			// Record metrics
			duration := time.Since(startTime)
			success := err == nil
			provider.metrics.RecordRPCRequest(ctx, procedure, success, duration)

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

				// Add trace ID to response headers
				if traceID := GetTraceID(ctx); traceID != "" {
					resp.Header().Set("X-Trace-ID", traceID)
				}
			}

			return resp, err
		}
	}
}

// DatabaseMiddleware wraps database operations with tracing
func DatabaseMiddleware(provider *Provider) func(ctx context.Context, query string, fn func() error) error {
	return func(ctx context.Context, query string, fn func() error) error {
		// Start span
		ctx, span := provider.Tracer("database").Start(ctx, "db.query",
			trace.WithAttributes(
				attribute.String("db.statement", truncateQuery(query)),
				attribute.String("db.system", "postgres"),
			),
		)
		defer span.End()

		// Record start time
		startTime := time.Now()

		// Execute query
		err := fn()

		// Record metrics
		duration := time.Since(startTime)
		success := err == nil
		provider.metrics.RecordDBQuery(ctx, query, success, duration)

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

// responseWriter wraps http.ResponseWriter to capture status and size
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	written    int64
}

func (w *responseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *responseWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.written += int64(n)
	return n, err
}

// truncateQuery truncates long queries for attributes
func truncateQuery(query string) string {
	const maxLen = 1000
	if len(query) > maxLen {
		return query[:maxLen] + "..."
	}
	return query
}

// TracedHandler wraps an HTTP handler with tracing
func TracedHandler(provider *Provider, name string, handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := provider.Tracer("http").Start(r.Context(), name,
			trace.WithSpanKind(trace.SpanKindInternal),
		)
		defer span.End()

		handler(w, r.WithContext(ctx))
	}
}

// TraceFunction wraps a function with tracing
func TraceFunction(ctx context.Context, provider *Provider, name string, fn func(context.Context) error) error {
	ctx, span := provider.Tracer("function").Start(ctx, name,
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

// WithRetry wraps a function with retry logic and tracing
func WithRetry(ctx context.Context, provider *Provider, name string, maxAttempts int, fn func(context.Context) error) error {
	ctx, span := provider.Tracer("retry").Start(ctx, name,
		trace.WithAttributes(
			attribute.Int("retry.max_attempts", maxAttempts),
		),
	)
	defer span.End()

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// Create span for each attempt
		attemptCtx, attemptSpan := provider.Tracer("retry").Start(ctx, fmt.Sprintf("%s.attempt.%d", name, attempt),
			trace.WithAttributes(
				attribute.Int("retry.attempt", attempt),
			),
		)

		// Execute function
		lastErr = fn(attemptCtx)

		if lastErr == nil {
			attemptSpan.SetStatus(codes.Ok, "")
			attemptSpan.End()
			span.SetStatus(codes.Ok, "")
			return nil
		}

		// Record error
		attemptSpan.RecordError(lastErr)
		attemptSpan.SetStatus(codes.Error, lastErr.Error())
		attemptSpan.End()

		// Add retry event
		span.AddEvent("retry_attempt_failed", trace.WithAttributes(
			attribute.Int("attempt", attempt),
			attribute.String("error", lastErr.Error()),
		))

		// Wait before retry (exponential backoff)
		if attempt < maxAttempts {
			backoff := time.Duration(attempt) * time.Second
			time.Sleep(backoff)
		}
	}

	// All attempts failed
	span.RecordError(lastErr)
	span.SetStatus(codes.Error, fmt.Sprintf("all %d attempts failed", maxAttempts))
	return lastErr
}
