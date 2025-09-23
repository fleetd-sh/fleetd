package middleware

import (
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/rs/cors"
)

// CORSConfig defines CORS configuration
type CORSConfig struct {
	// AllowedOrigins is a list of origins that are allowed.
	// Use ["*"] to allow any origin (not recommended for production)
	AllowedOrigins []string

	// AllowedMethods is a list of methods the client is allowed to use
	AllowedMethods []string

	// AllowedHeaders is a list of headers the client is allowed to use
	AllowedHeaders []string

	// ExposedHeaders indicates which headers are safe to expose to the API
	ExposedHeaders []string

	// AllowCredentials indicates whether the request can include user credentials
	AllowCredentials bool

	// MaxAge indicates how long the results of a preflight request can be cached (in seconds)
	MaxAge int

	// Debug enables debug logging
	Debug bool

	// AllowPrivateNetwork allows requests from private network ranges
	AllowPrivateNetwork bool
}

// DefaultCORSConfig returns a secure default CORS configuration
func DefaultCORSConfig() *CORSConfig {
	return &CORSConfig{
		AllowedOrigins: []string{}, // Same-origin only by default
		AllowedMethods: []string{
			http.MethodGet,
			http.MethodPost,
			http.MethodPut,
			http.MethodPatch,
			http.MethodDelete,
			http.MethodOptions,
		},
		AllowedHeaders: []string{"*"}, // Accept all headers, filter at API level
		ExposedHeaders: []string{
			"X-Request-ID",
			"X-RateLimit-Limit",
			"X-RateLimit-Remaining",
			"X-RateLimit-Reset",
		},
		AllowCredentials:    true,
		MaxAge:              3600, // 1 hour
		Debug:               false,
		AllowPrivateNetwork: false,
	}
}

// ProductionCORSConfig returns a production-ready CORS configuration
func ProductionCORSConfig(allowedOrigins []string) *CORSConfig {
	config := DefaultCORSConfig()

	// Validate and sanitize origins
	config.AllowedOrigins = sanitizeOrigins(allowedOrigins)

	// Stricter settings for production
	config.AllowCredentials = true // Only if needed
	config.MaxAge = 600            // 10 minutes
	config.Debug = false
	config.AllowPrivateNetwork = false

	return config
}

// DevelopmentCORSConfig returns a more permissive CORS configuration for development
func DevelopmentCORSConfig() *CORSConfig {
	config := DefaultCORSConfig()

	// More permissive for development
	config.AllowedOrigins = []string{
		"http://localhost:3000",
		"http://localhost:3001",
		"http://localhost:8080",
		"http://127.0.0.1:3000",
		"http://127.0.0.1:3001",
		"http://127.0.0.1:8080",
	}
	config.AllowedHeaders = []string{"*"}
	config.Debug = true
	config.AllowPrivateNetwork = true

	return config
}

// NewCORS creates a new CORS middleware
func NewCORS(config *CORSConfig) *cors.Cors {
	if config == nil {
		config = DefaultCORSConfig()
	}

	// Check if we need a custom validator
	needsCustomValidator := false
	for _, origin := range config.AllowedOrigins {
		if strings.Contains(origin, "*") && origin != "*" {
			// Pattern with wildcard (not just "*")
			needsCustomValidator = true
			break
		}
	}

	if config.AllowPrivateNetwork {
		needsCustomValidator = true
	}

	options := cors.Options{
		AllowedMethods:   config.AllowedMethods,
		AllowedHeaders:   config.AllowedHeaders,
		ExposedHeaders:   config.ExposedHeaders,
		AllowCredentials: config.AllowCredentials,
		MaxAge:           config.MaxAge,
		Debug:            config.Debug,
	}

	if needsCustomValidator {
		// Use custom origin validator for complex patterns
		options.AllowOriginFunc = createOriginValidator(config)
	} else {
		// Use simple allowed origins list
		options.AllowedOrigins = config.AllowedOrigins
	}

	// Add logging if debug is enabled
	if config.Debug {
		options.Logger = &corsLogger{}
		slog.Debug("CORS options",
			"AllowedOrigins", options.AllowedOrigins,
			"AllowedMethods", options.AllowedMethods,
			"AllowedHeaders", options.AllowedHeaders,
			"AllowCredentials", options.AllowCredentials,
			"needsCustomValidator", needsCustomValidator,
		)
	}

	return cors.New(options)
}

// createOriginValidator creates a custom origin validation function
func createOriginValidator(config *CORSConfig) func(origin string) bool {
	allowedOrigins := make(map[string]bool)
	allowedPatterns := []string{}

	for _, origin := range config.AllowedOrigins {
		if strings.Contains(origin, "*") {
			// Convert wildcard to pattern
			pattern := strings.ReplaceAll(origin, "*", "")
			allowedPatterns = append(allowedPatterns, pattern)
		} else {
			allowedOrigins[origin] = true
		}
	}

	return func(origin string) bool {
		// Check exact match
		if allowedOrigins[origin] {
			return true
		}

		// Check wildcard patterns
		for _, pattern := range allowedPatterns {
			if strings.Contains(origin, pattern) {
				return true
			}
		}

		// Check if it's a private network request
		if config.AllowPrivateNetwork {
			if isPrivateNetwork(origin) {
				return true
			}
		}

		return false
	}
}

// sanitizeOrigins validates and sanitizes origin URLs
func sanitizeOrigins(origins []string) []string {
	sanitized := []string{}

	for _, origin := range origins {
		origin = strings.TrimSpace(origin)

		// Skip empty origins
		if origin == "" {
			continue
		}

		// Allow wildcard
		if origin == "*" {
			slog.Warn("Using wildcard (*) for CORS origins is not recommended in production")
			return []string{"*"}
		}

		// Validate URL format
		if !strings.HasPrefix(origin, "http://") && !strings.HasPrefix(origin, "https://") {
			slog.Warn("Invalid CORS origin format, skipping", "origin", origin)
			continue
		}

		// Parse URL to validate
		u, err := url.Parse(origin)
		if err != nil {
			slog.Warn("Invalid CORS origin URL, skipping", "origin", origin, "error", err)
			continue
		}

		// Rebuild origin without path
		cleanOrigin := u.Scheme + "://" + u.Host
		sanitized = append(sanitized, cleanOrigin)
	}

	return sanitized
}

// isPrivateNetwork checks if the origin is from a private network
func isPrivateNetwork(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}

	host := u.Hostname()

	// Special handling for bare IPv6 addresses without brackets
	// URL parser incorrectly parses http://::1 as host=: port=:1
	if u.Host == "::1" {
		return true
	}

	// Check for localhost (including IPv6)
	if host == "localhost" || host == "127.0.0.1" || host == "::1" || host == "[::1]" {
		return true
	}

	// Check for private IP ranges (RFC 1918)
	// 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16
	if strings.HasPrefix(host, "10.") ||
		strings.HasPrefix(host, "172.16.") ||
		strings.HasPrefix(host, "172.17.") ||
		strings.HasPrefix(host, "172.18.") ||
		strings.HasPrefix(host, "172.19.") ||
		strings.HasPrefix(host, "172.20.") ||
		strings.HasPrefix(host, "172.21.") ||
		strings.HasPrefix(host, "172.22.") ||
		strings.HasPrefix(host, "172.23.") ||
		strings.HasPrefix(host, "172.24.") ||
		strings.HasPrefix(host, "172.25.") ||
		strings.HasPrefix(host, "172.26.") ||
		strings.HasPrefix(host, "172.27.") ||
		strings.HasPrefix(host, "172.28.") ||
		strings.HasPrefix(host, "172.29.") ||
		strings.HasPrefix(host, "172.30.") ||
		strings.HasPrefix(host, "172.31.") ||
		strings.HasPrefix(host, "192.168.") {
		return true
	}

	return false
}

// corsLogger implements the cors.Logger interface
type corsLogger struct{}

func (l *corsLogger) Printf(format string, v ...interface{}) {
	slog.Debug("CORS", "message", strings.TrimSpace(strings.TrimSuffix(format, "\n")), "args", v)
}

// CORSMiddleware creates an HTTP middleware for CORS
func CORSMiddleware(config *CORSConfig) func(http.Handler) http.Handler {
	c := NewCORS(config)
	return func(next http.Handler) http.Handler {
		return c.Handler(next)
	}
}

// ValidateCORSConfig validates CORS configuration
func ValidateCORSConfig(config *CORSConfig) error {
	if config == nil {
		return nil // Will use defaults
	}

	// Warn about security issues
	if len(config.AllowedOrigins) == 1 && config.AllowedOrigins[0] == "*" {
		if config.AllowCredentials {
			return ErrInsecureCORS
		}
		slog.Warn("Using wildcard (*) for CORS origins reduces security")
	}

	// Validate methods
	for _, method := range config.AllowedMethods {
		switch method {
		case http.MethodGet, http.MethodPost, http.MethodPut,
			http.MethodPatch, http.MethodDelete, http.MethodOptions,
			http.MethodHead:
			// Valid methods
		default:
			slog.Warn("Unusual HTTP method in CORS config", "method", method)
		}
	}

	return nil
}

// ErrInsecureCORS is returned when CORS configuration is insecure
var ErrInsecureCORS = &corsError{msg: "insecure CORS configuration: cannot use wildcard origin with credentials"}

type corsError struct {
	msg string
}

func (e *corsError) Error() string {
	return e.msg
}

// ApplyCORSHeaders manually applies CORS headers to a response
// Useful for specific endpoints that need custom CORS handling
func ApplyCORSHeaders(w http.ResponseWriter, r *http.Request, config *CORSConfig) {
	origin := r.Header.Get("Origin")

	// Check if origin is allowed
	allowed := false
	for _, allowedOrigin := range config.AllowedOrigins {
		if allowedOrigin == "*" || allowedOrigin == origin {
			allowed = true
			break
		}
	}

	if !allowed {
		return
	}

	// Set CORS headers
	w.Header().Set("Access-Control-Allow-Origin", origin)

	if config.AllowCredentials {
		w.Header().Set("Access-Control-Allow-Credentials", "true")
	}

	if len(config.ExposedHeaders) > 0 {
		w.Header().Set("Access-Control-Expose-Headers", strings.Join(config.ExposedHeaders, ", "))
	}

	// Handle preflight requests
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Methods", strings.Join(config.AllowedMethods, ", "))
		w.Header().Set("Access-Control-Allow-Headers", strings.Join(config.AllowedHeaders, ", "))
		w.Header().Set("Access-Control-Max-Age", string(rune(config.MaxAge)))
		w.WriteHeader(http.StatusNoContent)
	}
}
