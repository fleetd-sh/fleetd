package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"fleetd.sh/internal/security"
)

// contextKey is a custom type for context keys
type contextKey string

const (
	// ClaimsContextKey is the context key for JWT claims
	ClaimsContextKey contextKey = "claims"
	// APIKeyContextKey is the context key for API key info
	APIKeyContextKey contextKey = "api_key"
)

// AuthConfig contains authentication configuration
type AuthConfig struct {
	// JWTSecretKey is the secret key for JWT signing
	JWTSecretKey string
	// PublicPaths are paths that don't require authentication
	PublicPaths []string
	// EnableAPIKeys enables API key authentication
	EnableAPIKeys bool
	// APIKeyService is the service for validating API keys
	APIKeyService APIKeyValidator
	// RequireAuth forces authentication even in development
	RequireAuth bool
	// Logger for authentication events
	Logger *slog.Logger
}

// AuthMiddleware provides JWT authentication middleware
type AuthMiddleware struct {
	jwtManager    *security.JWTManager
	apiKeyService APIKeyValidator
	logger        *slog.Logger
	publicPaths   []string
	enableAPIKeys bool
	requireAuth   bool
}

// APIKeyValidator interface for validating API keys
type APIKeyValidator interface {
	ValidateAPIKeyWithClaims(ctx context.Context, apiKey string) (*security.Claims, error)
}

// NewAuthMiddleware creates a new auth middleware
func NewAuthMiddleware(config AuthConfig) func(http.Handler) http.Handler {
	if config.JWTSecretKey == "" {
		// This should fail fast in production
		panic("JWT secret key is required for authentication")
	}

	jwtConfig := &security.JWTConfig{
		SigningKey:    []byte(config.JWTSecretKey),
		SigningMethod: security.DefaultJWTConfig().SigningMethod,
		Issuer:        security.DefaultJWTConfig().Issuer,
		Audience:      security.DefaultJWTConfig().Audience,
	}

	jwtManager, err := security.NewJWTManager(jwtConfig)
	if err != nil {
		panic("Failed to create JWT manager: " + err.Error())
	}

	logger := config.Logger
	if logger == nil {
		logger = slog.Default().With("component", "auth-middleware")
	}

	// Default public paths if not specified
	publicPaths := config.PublicPaths
	if len(publicPaths) == 0 {
		publicPaths = []string{
			"/health",
			"/health/live",
			"/health/ready",
			"/metrics",
			"/api/v1/auth/login",
			"/api/v1/auth/refresh",
			"/api/v1/device/register", // Devices need to register without auth
		}
	}

	am := &AuthMiddleware{
		jwtManager:    jwtManager,
		apiKeyService: config.APIKeyService,
		logger:        logger,
		publicPaths:   publicPaths,
		enableAPIKeys: config.EnableAPIKeys,
		requireAuth:   config.RequireAuth,
	}

	return am.Middleware
}

// isPublicPath checks if the path is public
func (am *AuthMiddleware) isPublicPath(path string) bool {
	for _, publicPath := range am.publicPaths {
		if path == publicPath || strings.HasPrefix(path, publicPath) {
			return true
		}
	}
	return false
}

// validateAPIKey validates an API key
func (am *AuthMiddleware) validateAPIKey(ctx context.Context, apiKey string) (*security.Claims, error) {
	if am.apiKeyService == nil {
		return nil, nil // API key service not configured
	}

	// Validate the API key and get claims
	claims, err := am.apiKeyService.ValidateAPIKeyWithClaims(ctx, apiKey)
	if err != nil {
		return nil, err
	}

	return claims, nil
}

// Middleware is the auth middleware function
func (am *AuthMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth for public endpoints
		if am.isPublicPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		// Try Bearer token authentication first
		auth := r.Header.Get("Authorization")
		if auth != "" && strings.HasPrefix(auth, "Bearer ") {
			token := strings.TrimPrefix(auth, "Bearer ")
			claims, err := am.jwtManager.ValidateToken(token)
			if err != nil {
				am.logger.Debug("Token validation failed",
					"error", err,
					"path", r.URL.Path,
					"remote", r.RemoteAddr)
				http.Error(w, "Invalid or expired token", http.StatusUnauthorized)
				return
			}

			// Token is valid, add claims to context
			ctx := context.WithValue(r.Context(), ClaimsContextKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// Try API key authentication if enabled
		if am.enableAPIKeys {
			apiKey := r.Header.Get("X-API-Key")
			if apiKey != "" {
				claims, err := am.validateAPIKey(r.Context(), apiKey)
				if err != nil {
					am.logger.Debug("API key validation failed",
						"error", err,
						"path", r.URL.Path,
						"remote", r.RemoteAddr)
					http.Error(w, "Invalid API key", http.StatusUnauthorized)
					return
				}

				if claims != nil {
					// API key is valid
					ctx := context.WithValue(r.Context(), ClaimsContextKey, claims)
					ctx = context.WithValue(ctx, APIKeyContextKey, apiKey)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}

				// API key validation not implemented yet
				am.logger.Warn("API key authentication not yet implemented",
					"path", r.URL.Path,
					"remote", r.RemoteAddr)
			}
		}

		// No valid authentication provided
		am.logger.Debug("No authentication provided",
			"path", r.URL.Path,
			"method", r.Method,
			"remote", r.RemoteAddr)

		w.Header().Set("WWW-Authenticate", `Bearer realm="fleetd"`)
		http.Error(w, "Authentication required", http.StatusUnauthorized)
	})
}

// GetClaims retrieves JWT claims from the request context
func GetClaims(ctx context.Context) (*security.Claims, bool) {
	claims, ok := ctx.Value(ClaimsContextKey).(*security.Claims)
	return claims, ok
}

// GetAPIKey retrieves API key from the request context
func GetAPIKey(ctx context.Context) (string, bool) {
	apiKey, ok := ctx.Value(APIKeyContextKey).(string)
	return apiKey, ok
}

// loggingResponseWriter wraps http.ResponseWriter to capture status code for logging
type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
	written    bool
}

func (rw *loggingResponseWriter) WriteHeader(code int) {
	if !rw.written {
		rw.statusCode = code
		rw.ResponseWriter.WriteHeader(code)
		rw.written = true
	}
}

func (rw *loggingResponseWriter) Write(b []byte) (int, error) {
	if !rw.written {
		rw.WriteHeader(http.StatusOK)
	}
	return rw.ResponseWriter.Write(b)
}

// NewLoggingMiddleware creates logging middleware
func NewLoggingMiddleware() func(http.Handler) http.Handler {
	logger := slog.Default().With("component", "http")

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Wrap response writer to capture status code
			rw := &loggingResponseWriter{
				ResponseWriter: w,
				statusCode:     http.StatusOK,
				written:        false,
			}

			// Process request
			next.ServeHTTP(rw, r)

			// Log request details
			duration := time.Since(start)

			// Use appropriate log level based on status code
			logLevel := slog.LevelInfo
			if rw.statusCode >= 400 && rw.statusCode < 500 {
				logLevel = slog.LevelWarn
			} else if rw.statusCode >= 500 {
				logLevel = slog.LevelError
			}

			logger.Log(r.Context(), logLevel, "HTTP request",
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