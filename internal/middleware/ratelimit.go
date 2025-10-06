package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

// RateLimiter provides request rate limiting
type RateLimiter struct {
	visitors map[string]*visitor
	mu       sync.RWMutex
	config   RateLimitConfig
	logger   *zap.Logger
}

// RateLimitConfig configures rate limiting behavior
type RateLimitConfig struct {
	// Global limits
	RequestsPerSecond int
	BurstSize         int

	// Per-endpoint limits
	EndpointLimits map[string]EndpointLimit

	// Per-API key limits
	APIKeyLimits map[string]APIKeyLimit

	// Device-specific limits
	DeviceRequestsPerMinute int
	DeviceBurstSize         int

	// DDoS protection
	MaxConnectionsPerIP    int
	MaxRequestsPerIPPerMin int
	BanDuration            time.Duration

	// Circuit breaker
	ErrorThreshold  int
	ErrorWindow     time.Duration
	RecoveryTimeout time.Duration

	// Cleanup
	CleanupInterval time.Duration
	VisitorTimeout  time.Duration
}

// EndpointLimit defines rate limits for specific endpoints
type EndpointLimit struct {
	Path              string
	RequestsPerSecond int
	BurstSize         int
	Methods           []string
}

// APIKeyLimit defines rate limits for API keys
type APIKeyLimit struct {
	Key               string
	RequestsPerSecond int
	BurstSize         int
	Role              string // "admin", "operator", "service", "device"
}

// visitor tracks rate limiting state per client
type visitor struct {
	limiter        *rate.Limiter
	lastSeen       time.Time
	requestCount   int
	errorCount     int
	banned         bool
	banExpiry      time.Time
	circuitBreaker *CircuitBreaker
}

// CircuitBreaker implements the circuit breaker pattern
type CircuitBreaker struct {
	mu              sync.Mutex
	state           string // "closed", "open", "half-open"
	failures        int
	lastFailureTime time.Time
	recoveryTimeout time.Duration
	errorThreshold  int
	errorWindow     time.Duration
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(config RateLimitConfig, logger *zap.Logger) *RateLimiter {
	if config.RequestsPerSecond == 0 {
		config.RequestsPerSecond = 100
	}
	if config.BurstSize == 0 {
		config.BurstSize = 200
	}
	if config.CleanupInterval == 0 {
		config.CleanupInterval = 1 * time.Minute
	}
	if config.VisitorTimeout == 0 {
		config.VisitorTimeout = 3 * time.Minute
	}
	if config.BanDuration == 0 {
		config.BanDuration = 15 * time.Minute
	}

	rl := &RateLimiter{
		visitors: make(map[string]*visitor),
		config:   config,
		logger:   logger,
	}

	// Start cleanup goroutine
	go rl.cleanupVisitors()

	return rl
}

// Middleware returns HTTP middleware for rate limiting
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract client identifier
		clientID := rl.getClientID(r)

		// Check if client is banned
		if rl.isBanned(clientID) {
			rl.handleRateLimitExceeded(w, r, "Client is temporarily banned")
			return
		}

		// Get or create visitor
		v := rl.getVisitor(clientID)

		// Check circuit breaker
		if !v.circuitBreaker.Allow() {
			rl.handleCircuitOpen(w, r)
			return
		}

		// Determine rate limit based on context
		limiter := rl.getLimiterForRequest(r, v, clientID)

		// Check rate limit
		if !limiter.Allow() {
			rl.handleRateLimitExceeded(w, r, "Rate limit exceeded")

			// Check if client should be banned
			v.requestCount++
			if v.requestCount > rl.config.MaxRequestsPerIPPerMin {
				rl.banClient(clientID)
			}
			return
		}

		// Track successful request
		v.requestCount = 0
		v.errorCount = 0
		v.lastSeen = time.Now()

		// Proceed with request
		next.ServeHTTP(w, r)
	})
}

// DeviceRateLimiter provides rate limiting specifically for device endpoints
func (rl *RateLimiter) DeviceRateLimiter(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract device ID from request
		deviceID := rl.getDeviceID(r)
		if deviceID == "" {
			http.Error(w, "Device ID required", http.StatusBadRequest)
			return
		}

		// Create device-specific key
		key := fmt.Sprintf("device:%s", deviceID)

		// Get or create visitor for device
		v := rl.getVisitor(key)

		// Use device-specific limits
		if v.limiter == nil {
			limit := rate.Limit(float64(rl.config.DeviceRequestsPerMinute) / 60.0)
			v.limiter = rate.NewLimiter(limit, rl.config.DeviceBurstSize)
		}

		// Check rate limit
		if !v.limiter.Allow() {
			rl.handleRateLimitExceeded(w, r, "Device rate limit exceeded")
			return
		}

		v.lastSeen = time.Now()
		next.ServeHTTP(w, r)
	})
}

// APIKeyRateLimiter provides rate limiting based on API keys
func (rl *RateLimiter) APIKeyRateLimiter(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract API key
		apiKey := rl.getAPIKey(r)
		if apiKey == "" {
			http.Error(w, "API key required", http.StatusUnauthorized)
			return
		}

		// Look up API key limits
		keyLimit, exists := rl.config.APIKeyLimits[apiKey]
		if !exists {
			// Use default limits for unknown keys
			keyLimit = APIKeyLimit{
				Key:               apiKey,
				RequestsPerSecond: 10,
				BurstSize:         20,
				Role:              "device",
			}
		}

		// Create key-specific identifier
		key := fmt.Sprintf("apikey:%s", apiKey)
		v := rl.getVisitor(key)

		// Create limiter with API key limits
		if v.limiter == nil {
			v.limiter = rate.NewLimiter(rate.Limit(keyLimit.RequestsPerSecond), keyLimit.BurstSize)
		}

		// Check rate limit
		if !v.limiter.Allow() {
			rl.handleRateLimitExceeded(w, r, "API key rate limit exceeded")
			return
		}

		// Add API key role to context for logging
		ctx := context.WithValue(r.Context(), "api_role", keyLimit.Role)
		r = r.WithContext(ctx)

		v.lastSeen = time.Now()
		next.ServeHTTP(w, r)
	})
}

// DDoSProtection provides enhanced DDoS protection
func (rl *RateLimiter) DDoSProtection(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientIP := rl.getClientIP(r)

		// Check connection count per IP
		rl.mu.RLock()
		connectionCount := 0
		for key := range rl.visitors {
			if strings.HasPrefix(key, clientIP) {
				connectionCount++
			}
		}
		rl.mu.RUnlock()

		if connectionCount > rl.config.MaxConnectionsPerIP {
			rl.logger.Warn("Too many connections from IP",
				zap.String("ip", clientIP),
				zap.Int("count", connectionCount),
			)
			http.Error(w, "Too many connections", http.StatusTooManyRequests)
			return
		}

		// Check for suspicious patterns
		if rl.detectSuspiciousActivity(r) {
			rl.banClient(clientIP)
			http.Error(w, "Suspicious activity detected", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// getVisitor retrieves or creates a visitor
func (rl *RateLimiter) getVisitor(key string) *visitor {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	v, exists := rl.visitors[key]
	if !exists {
		limiter := rate.NewLimiter(rate.Limit(rl.config.RequestsPerSecond), rl.config.BurstSize)
		v = &visitor{
			limiter:  limiter,
			lastSeen: time.Now(),
			circuitBreaker: &CircuitBreaker{
				state:           "closed",
				recoveryTimeout: rl.config.RecoveryTimeout,
				errorThreshold:  rl.config.ErrorThreshold,
				errorWindow:     rl.config.ErrorWindow,
			},
		}
		rl.visitors[key] = v
	}

	return v
}

// getLimiterForRequest determines the appropriate rate limiter for a request
func (rl *RateLimiter) getLimiterForRequest(r *http.Request, v *visitor, clientID string) *rate.Limiter {
	// Check API key-specific limits first (highest priority)
	if apiKey := rl.getAPIKey(r); apiKey != "" {
		if keyLimit, exists := rl.config.APIKeyLimits[apiKey]; exists {
			// Update visitor's limiter if not already set with API key limits
			if v.limiter == nil || v.limiter.Limit() != rate.Limit(keyLimit.RequestsPerSecond) {
				v.limiter = rate.NewLimiter(rate.Limit(keyLimit.RequestsPerSecond), keyLimit.BurstSize)
			}
			return v.limiter
		}
	}

	// Check endpoint-specific limits
	path := r.URL.Path
	for _, limit := range rl.config.EndpointLimits {
		if strings.HasPrefix(path, limit.Path) {
			// Check if method is allowed
			if len(limit.Methods) > 0 {
				allowed := false
				for _, method := range limit.Methods {
					if r.Method == method {
						allowed = true
						break
					}
				}
				if !allowed {
					continue
				}
			}

			// Create endpoint-specific limiter
			key := fmt.Sprintf("%s:%s", clientID, limit.Path)
			rl.mu.Lock()
			ev, exists := rl.visitors[key]
			if !exists {
				ev = &visitor{
					limiter:  rate.NewLimiter(rate.Limit(limit.RequestsPerSecond), limit.BurstSize),
					lastSeen: time.Now(),
				}
				rl.visitors[key] = ev
			}
			rl.mu.Unlock()
			return ev.limiter
		}
	}

	// Use default limiter
	return v.limiter
}

// getClientID extracts a client identifier from the request
func (rl *RateLimiter) getClientID(r *http.Request) string {
	// Try API key first
	if apiKey := rl.getAPIKey(r); apiKey != "" {
		return fmt.Sprintf("api:%s", apiKey)
	}

	// Fall back to IP address
	return rl.getClientIP(r)
}

// getClientIP extracts the client IP address
func (rl *RateLimiter) getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	return ip
}

// getAPIKey extracts the API key from the request
func (rl *RateLimiter) getAPIKey(r *http.Request) string {
	// Check Authorization header
	if auth := r.Header.Get("Authorization"); auth != "" {
		parts := strings.Split(auth, " ")
		if len(parts) == 2 && strings.ToLower(parts[0]) == "bearer" {
			return parts[1]
		}
	}

	// Check X-API-Key header
	if key := r.Header.Get("X-API-Key"); key != "" {
		return key
	}

	// Check query parameter
	return r.URL.Query().Get("api_key")
}

// getDeviceID extracts the device ID from the request
func (rl *RateLimiter) getDeviceID(r *http.Request) string {
	// Check header
	if deviceID := r.Header.Get("X-Device-ID"); deviceID != "" {
		return deviceID
	}

	// Check path parameter (assumes /api/devices/{id}/...)
	parts := strings.Split(r.URL.Path, "/")
	for i, part := range parts {
		if part == "devices" && i+1 < len(parts) {
			return parts[i+1]
		}
	}

	return ""
}

// isBanned checks if a client is banned
func (rl *RateLimiter) isBanned(clientID string) bool {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	if v, exists := rl.visitors[clientID]; exists {
		if v.banned && time.Now().Before(v.banExpiry) {
			return true
		}
		// Clear expired ban
		if v.banned && time.Now().After(v.banExpiry) {
			v.banned = false
		}
	}

	return false
}

// banClient bans a client for a period
func (rl *RateLimiter) banClient(clientID string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	v, exists := rl.visitors[clientID]
	if !exists {
		v = &visitor{lastSeen: time.Now()}
		rl.visitors[clientID] = v
	}

	v.banned = true
	v.banExpiry = time.Now().Add(rl.config.BanDuration)

	rl.logger.Warn("Client banned",
		zap.String("client_id", clientID),
		zap.Duration("duration", rl.config.BanDuration),
	)
}

// detectSuspiciousActivity checks for suspicious request patterns
func (rl *RateLimiter) detectSuspiciousActivity(r *http.Request) bool {
	// Check for common attack patterns
	suspicious := []string{
		"../",         // Path traversal
		"<script>",    // XSS attempt
		"';",          // SQL injection
		"${jndi:",     // Log4Shell
		"{{",          // Template injection
		"%00",         // Null byte
		"cmd=",        // Command injection
		"/etc/passwd", // Sensitive file access
	}

	url := r.URL.String()
	body := make([]byte, 1024)
	r.Body.Read(body)

	for _, pattern := range suspicious {
		if strings.Contains(url, pattern) || strings.Contains(string(body), pattern) {
			rl.logger.Warn("Suspicious pattern detected",
				zap.String("pattern", pattern),
				zap.String("url", url),
			)
			return true
		}
	}

	// Check for unusually large headers
	totalHeaderSize := 0
	for key, values := range r.Header {
		totalHeaderSize += len(key)
		for _, value := range values {
			totalHeaderSize += len(value)
		}
	}

	if totalHeaderSize > 100000 { // 100KB of headers is suspicious
		rl.logger.Warn("Unusually large headers detected",
			zap.Int("size", totalHeaderSize),
		)
		return true
	}

	return false
}

// handleRateLimitExceeded responds to rate limit violations
func (rl *RateLimiter) handleRateLimitExceeded(w http.ResponseWriter, r *http.Request, reason string) {
	rl.logger.Debug("Rate limit exceeded",
		zap.String("path", r.URL.Path),
		zap.String("reason", reason),
		zap.String("ip", rl.getClientIP(r)),
	)

	// Add rate limit headers
	w.Header().Set("X-RateLimit-Limit", strconv.Itoa(rl.config.RequestsPerSecond))
	w.Header().Set("X-RateLimit-Remaining", "0")
	w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(time.Minute).Unix(), 10))
	w.Header().Set("Retry-After", "60")

	// Return error response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusTooManyRequests)

	response := map[string]interface{}{
		"error":       "rate_limit_exceeded",
		"message":     reason,
		"retry_after": 60,
	}

	json.NewEncoder(w).Encode(response)
}

// handleCircuitOpen responds when circuit breaker is open
func (rl *RateLimiter) handleCircuitOpen(w http.ResponseWriter, r *http.Request) {
	rl.logger.Warn("Circuit breaker open",
		zap.String("path", r.URL.Path),
		zap.String("ip", rl.getClientIP(r)),
	)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusServiceUnavailable)

	response := map[string]interface{}{
		"error":       "service_unavailable",
		"message":     "Service temporarily unavailable due to high error rate",
		"retry_after": 30,
	}

	json.NewEncoder(w).Encode(response)
}

// cleanupVisitors removes old visitor entries
func (rl *RateLimiter) cleanupVisitors() {
	ticker := time.NewTicker(rl.config.CleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for key, v := range rl.visitors {
			if now.Sub(v.lastSeen) > rl.config.VisitorTimeout {
				delete(rl.visitors, key)
			}
		}
		rl.mu.Unlock()
	}
}

// CircuitBreaker methods

// Allow checks if request should be allowed
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()

	switch cb.state {
	case "open":
		// Check if we should transition to half-open
		if now.Sub(cb.lastFailureTime) > cb.recoveryTimeout {
			cb.state = "half-open"
			cb.failures = 0
			return true
		}
		return false

	case "half-open":
		// Allow limited requests to test recovery
		return true

	default: // closed
		// Check if we should open the circuit
		if cb.failures >= cb.errorThreshold {
			if now.Sub(cb.lastFailureTime) <= cb.errorWindow {
				cb.state = "open"
				return false
			}
			// Reset if outside error window
			cb.failures = 0
		}
		return true
	}
}

// RecordSuccess records a successful request
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == "half-open" {
		cb.state = "closed"
		cb.failures = 0
	}
}

// RecordFailure records a failed request
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures++
	cb.lastFailureTime = time.Now()

	if cb.state == "half-open" {
		cb.state = "open"
	}
}
