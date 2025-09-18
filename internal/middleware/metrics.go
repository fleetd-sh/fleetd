package middleware

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"fleetd.sh/internal/metrics"
)

// metricsResponseWriter wraps http.ResponseWriter to capture status code and size
type metricsResponseWriter struct {
	http.ResponseWriter
	statusCode int
	size       int
}

func (rw *metricsResponseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *metricsResponseWriter) Write(b []byte) (int, error) {
	size, err := rw.ResponseWriter.Write(b)
	rw.size += size
	return size, err
}

// NewMetricsMiddleware creates a new metrics middleware
func NewMetricsMiddleware(serviceName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Wrap response writer to capture metrics
			wrapped := &metricsResponseWriter{
				ResponseWriter: w,
				statusCode:     200,
			}

			// Get request size
			reqSize := float64(r.ContentLength)
			if reqSize < 0 {
				reqSize = 0
			}

			// Process request
			next.ServeHTTP(wrapped, r)

			// Record metrics
			duration := time.Since(start).Seconds()
			endpoint := cleanPath(r.URL.Path)
			statusStr := strconv.Itoa(wrapped.statusCode)

			metrics.RecordHTTPRequest(
				serviceName,
				r.Method,
				endpoint,
				statusStr,
				duration,
				reqSize,
				float64(wrapped.size),
			)
		})
	}
}

// cleanPath removes IDs and dynamic segments from paths for metric labels
func cleanPath(path string) string {
	parts := strings.Split(path, "/")
	cleaned := make([]string, len(parts))

	for i, part := range parts {
		// Replace UUIDs with placeholder
		if len(part) == 36 && strings.Count(part, "-") == 4 {
			cleaned[i] = "{id}"
			continue
		}

		// Replace numeric IDs with placeholder
		if _, err := strconv.Atoi(part); err == nil && part != "" {
			cleaned[i] = "{id}"
			continue
		}

		cleaned[i] = part
	}

	return strings.Join(cleaned, "/")
}
