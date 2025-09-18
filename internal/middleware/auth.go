package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"fleetd.sh/internal/security"
)

// contextKey is a custom type for context keys
type contextKey string

const (
	// ClaimsContextKey is the context key for JWT claims
	ClaimsContextKey contextKey = "claims"
)

// AuthMiddleware provides JWT authentication middleware
type AuthMiddleware struct {
	jwtManager *security.JWTManager
	logger     *slog.Logger
	devMode    bool
}

// NewAuthMiddleware creates a new auth middleware
func NewAuthMiddleware(secretKey string) func(http.Handler) http.Handler {
	jwtConfig := &security.JWTConfig{
		SigningKey:    []byte(secretKey),
		SigningMethod: security.DefaultJWTConfig().SigningMethod,
		Issuer:        security.DefaultJWTConfig().Issuer,
		Audience:      security.DefaultJWTConfig().Audience,
	}

	jwtManager, err := security.NewJWTManager(jwtConfig)
	if err != nil {
		// Log error but continue with a basic auth check
		slog.Error("Failed to create JWT manager", "error", err)
		return func(next http.Handler) http.Handler {
			return next
		}
	}

	// Check if we're in development/insecure mode
	devMode := os.Getenv("FLEETD_AUTH_MODE") == "development" ||
		os.Getenv("FLEETD_INSECURE") == "true" ||
		os.Getenv("NODE_ENV") == "development"

	logger := slog.Default().With("component", "auth-middleware")

	if devMode {
		logger.Warn("\n" +
			"╔══════════════════════════════════════════════════════════════╗\n" +
			"║                      SECURITY WARNING                       ║\n" +
			"║                                                              ║\n" +
			"║  Authentication is running in DEVELOPMENT/INSECURE mode!    ║\n" +
			"║  Unauthenticated requests will be allowed.                  ║\n" +
			"║                                                              ║\n" +
			"║  DO NOT use this configuration in production!               ║\n" +
			"║                                                              ║\n" +
			"║  To enable authentication:                                  ║\n" +
			"║  - Set FLEETD_AUTH_MODE=production                          ║\n" +
			"║  - Or remove FLEETD_INSECURE=true                           ║\n" +
			"╚══════════════════════════════════════════════════════════════╝")
	}

	am := &AuthMiddleware{
		jwtManager: jwtManager,
		logger:     logger,
		devMode:    devMode,
	}
	return am.Middleware
}

// isDevelopmentMode checks if we're running in development/insecure mode
func (am *AuthMiddleware) isDevelopmentMode() bool {
	return am.devMode
}

// Middleware is the auth middleware function
func (am *AuthMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth for health check
		if r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}

		// Skip auth for public endpoints
		publicPaths := []string{"/health", "/metrics", "/api/v1/register"}
		for _, path := range publicPaths {
			if strings.HasPrefix(r.URL.Path, path) {
				next.ServeHTTP(w, r)
				return
			}
		}

		// Check for Authorization header
		auth := r.Header.Get("Authorization")
		if auth == "" {
			// Check for API key in header
			apiKey := r.Header.Get("X-API-Key")
			if apiKey != "" {
				// TODO: Validate API key against database
				am.logger.Debug("API key authentication not yet implemented")
				next.ServeHTTP(w, r)
				return
			}

			// Check if we're in development/insecure mode
			if am.isDevelopmentMode() {
				am.logger.Warn("SECURITY WARNING: Authentication bypassed - running in INSECURE development mode!",
					"path", r.URL.Path,
					"method", r.Method,
					"remote", r.RemoteAddr)
				next.ServeHTTP(w, r)
				return
			}

			// Production mode - require authentication
			am.logger.Debug("No authentication provided", "path", r.URL.Path)
			http.Error(w, "Authentication required", http.StatusUnauthorized)
			return
		}

		// Validate Bearer token
		if !strings.HasPrefix(auth, "Bearer ") {
			http.Error(w, "Invalid authorization header format", http.StatusUnauthorized)
			return
		}

		// Extract and validate JWT token
		token := strings.TrimPrefix(auth, "Bearer ")
		claims, err := am.jwtManager.ValidateToken(token)
		if err != nil {
			am.logger.Debug("Token validation failed", "error", err)
			http.Error(w, "Invalid or expired token", http.StatusUnauthorized)
			return
		}

		// Add claims to context
		ctx := context.WithValue(r.Context(), ClaimsContextKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetClaims retrieves JWT claims from the request context
func GetClaims(ctx context.Context) (*security.Claims, bool) {
	claims, ok := ctx.Value(ClaimsContextKey).(*security.Claims)
	return claims, ok
}

// loggingResponseWriter wraps http.ResponseWriter to capture status code for logging
type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *loggingResponseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// NewLoggingMiddleware creates logging middleware
func NewLoggingMiddleware() func(http.Handler) http.Handler {
	logger := slog.Default().With("component", "http")

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Wrap response writer to capture status code
			rw := &loggingResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			// Process request
			next.ServeHTTP(rw, r)

			// Log request details
			duration := time.Since(start)
			logger.Info("HTTP request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", rw.statusCode,
				"duration_ms", duration.Milliseconds(),
				"remote_addr", r.RemoteAddr,
				"user_agent", r.UserAgent(),
			)
		})
	}
}
