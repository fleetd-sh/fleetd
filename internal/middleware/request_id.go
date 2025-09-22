package middleware

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

const (
	// RequestIDKey is the context key for request IDs
	RequestIDKey contextKey = "request-id"

	// RequestIDHeader is the HTTP header for request IDs
	RequestIDHeader = "X-Request-ID"

	// TraceIDHeader is the HTTP header for trace IDs
	TraceIDHeader = "X-Trace-ID"
)

// RequestIDMiddleware adds a unique request ID to each request
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if request already has an ID (from client or upstream service)
		requestID := r.Header.Get(RequestIDHeader)
		if requestID == "" {
			// Also check for trace ID from distributed tracing
			requestID = r.Header.Get(TraceIDHeader)
		}
		if requestID == "" {
			// Generate a new request ID
			requestID = uuid.New().String()
		}

		// Add request ID to context
		ctx := context.WithValue(r.Context(), RequestIDKey, requestID)

		// Add request ID to response headers
		w.Header().Set(RequestIDHeader, requestID)

		// Continue with the request
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetRequestID extracts the request ID from context
func GetRequestID(ctx context.Context) string {
	if id, ok := ctx.Value(RequestIDKey).(string); ok {
		return id
	}
	return ""
}

// WithRequestID adds a request ID to the context
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, RequestIDKey, requestID)
}