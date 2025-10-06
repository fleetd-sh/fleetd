package middleware

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"connectrpc.com/connect"
)

// ErrorRecoveryInterceptor creates a Connect interceptor for error handling and recovery
func ErrorRecoveryInterceptor() connect.UnaryInterceptorFunc {
	interceptor := func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			// Extract request ID from headers or generate new one
			requestID := req.Header().Get("X-Request-ID")
			if requestID == "" {
				requestID = generateRequestID()
			}

			// Add request ID to context
			ctx = context.WithValue(ctx, "request_id", requestID)

			// Recover from panics
			defer func() {
				if r := recover(); r != nil {
					stack := string(debug.Stack())
					slog.Error("API panic",
						"request_id", requestID,
						"recovered", r,
						"stack", stack,
						"procedure", req.Spec().Procedure,
					)
				}
			}()

			// Call the next handler
			resp, err := next(ctx, req)

			return resp, err
		}
	}
	return connect.UnaryInterceptorFunc(interceptor)
}

// StreamingErrorRecoveryInterceptor is currently unavailable due to Connect v2 API changes
// TODO: Update for Connect v2 streaming API when available

// LoggingMiddleware logs HTTP requests
func LoggingMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Wrap response writer to capture status code
			wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			// Call next handler
			next.ServeHTTP(wrapped, r)

			// Log request
			duration := time.Since(start)
			logger.Info("HTTP request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", wrapped.statusCode,
				"duration_ms", duration.Milliseconds(),
				"remote_addr", r.RemoteAddr,
			)
		})
	}
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	written    bool
}

func (w *responseWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *responseWriter) Write(b []byte) (int, error) {
	w.written = true
	return w.ResponseWriter.Write(b)
}

func generateRequestID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
