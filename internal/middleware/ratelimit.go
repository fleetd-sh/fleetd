package middleware

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"connectrpc.com/connect"
	"golang.org/x/time/rate"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
)

// RateLimiter manages rate limiting for API clients
type RateLimiter struct {
	mu            sync.RWMutex
	limiters      map[string]*limiterState
	rate          rate.Limit
	burst         int
	expiration    time.Duration
	cleanupTicker *time.Ticker
	done          chan struct{}
}

type limiterState struct {
	limiter  *rate.Limiter
	lastUsed time.Time
}

// RateLimiterConfig configures the rate limiter
type RateLimiterConfig struct {
	Rate       float64       // Rate limit in requests per second
	Burst      int           // Maximum burst size
	Expiration time.Duration // How long to keep limiters for inactive clients
}

// Validate checks if the configuration is valid
func (c *RateLimiterConfig) Validate() error {
	if c.Rate <= 0 {
		return errors.New("rate must be positive")
	}
	if c.Burst <= 0 {
		return errors.New("burst must be positive")
	}
	if c.Expiration <= 0 {
		return errors.New("expiration must be positive")
	}
	if c.Burst < int(c.Rate) {
		return errors.New("burst should not be less than rate")
	}
	return nil
}

// NewRateLimiter creates a new RateLimiter
func NewRateLimiter(config RateLimiterConfig) (*RateLimiter, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	rl := &RateLimiter{
		limiters:      make(map[string]*limiterState),
		rate:          rate.Limit(config.Rate),
		burst:         config.Burst,
		expiration:    config.Expiration,
		cleanupTicker: time.NewTicker(config.Expiration),
		done:          make(chan struct{}),
	}

	go rl.cleanupLoop()
	return rl, nil
}

// getLimiter gets or creates a rate limiter for a client
func (rl *RateLimiter) getLimiter(clientID string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	state, exists := rl.limiters[clientID]
	if !exists {
		limiter := rate.NewLimiter(rl.rate, rl.burst)
		rl.limiters[clientID] = &limiterState{limiter: limiter, lastUsed: time.Now()}
		return limiter
	}

	// Update last used time
	state.lastUsed = time.Now()
	return state.limiter
}

// cleanup periodically removes inactive limiters
func (rl *RateLimiter) cleanup() {
	rl.mu.Lock()
	for clientID, state := range rl.limiters {
		if time.Since(state.lastUsed) > rl.expiration {
			delete(rl.limiters, clientID)
		}
	}
	rl.mu.Unlock()
}

// UnaryServerInterceptor returns a Connect interceptor that rate limits requests
func (rl *RateLimiter) UnaryServerInterceptor() connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			clientID := getClientID(ctx)
			if clientID == "" {
				return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("client ID not found"))
			}

			limiter := rl.getLimiter(clientID)
			if !limiter.Allow() {
				return nil, connect.NewError(connect.CodeResourceExhausted, errors.New("rate limit exceeded"))
			}

			return next(ctx, req)
		}
	}
}

type streamInterceptor struct {
	rateLimiter *RateLimiter
}

func (rl *RateLimiter) StreamServerInterceptor() connect.Interceptor {
	return &streamInterceptor{rateLimiter: rl}
}

func (s *streamInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return next
}

func (s *streamInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (s *streamInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, stream connect.StreamingHandlerConn) error {
		clientID := getClientID(ctx)
		if clientID == "" {
			return connect.NewError(connect.CodeUnauthenticated, errors.New("client ID not found"))
		}

		limiter := s.rateLimiter.getLimiter(clientID)
		wrappedStream := &rateLimitedServerStream{
			StreamingHandlerConn: stream,
			limiter:              limiter,
		}

		return next(ctx, wrappedStream)
	}
}

// rateLimitedServerStream wraps a connect.ServerStream with rate limiting
type rateLimitedServerStream struct {
	connect.StreamingHandlerConn
	limiter *rate.Limiter
}

// Receive rate limits incoming messages
func (s *rateLimitedServerStream) Receive(m any) error {
	if !s.limiter.Allow() {
		return connect.NewError(connect.CodeResourceExhausted, errors.New("rate limit exceeded"))
	}
	return s.StreamingHandlerConn.Receive(m)
}

// getClientID gets the client ID from the context
// This should be customized based on your authentication mechanism
func getClientID(ctx context.Context) string {
	// Priority 1: Check for authenticated user ID
	if userID, ok := ctx.Value("user_id").(string); ok && userID != "" {
		return "user:" + userID
	}

	// Priority 2: Get API key from metadata
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if keys := md.Get("x-api-key"); len(keys) > 0 && keys[0] != "" {
			return "api:" + keys[0]
		}
	}

	// Priority 3: Get IP address from peer info
	if pr, ok := peer.FromContext(ctx); ok && pr.Addr != nil {
		return "ip:" + pr.Addr.String()
	}

	return ""
}

// RateLimitMiddleware returns HTTP middleware that rate limits requests
func RateLimitMiddleware(rl *RateLimiter) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get client ID (e.g., from API key header or IP address)
			clientID := r.Header.Get("X-API-Key")
			if clientID == "" {
				clientID = r.RemoteAddr
			}

			// Get rate limiter for client
			limiter := rl.getLimiter(clientID)

			// Try to allow request
			if !limiter.Allow() {
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}

			// Process request
			next.ServeHTTP(w, r)
		})
	}
}

// WithRateLimit wraps an http.Handler with rate limiting
func WithRateLimit(handler http.Handler, rl *RateLimiter) http.Handler {
	return RateLimitMiddleware(rl)(handler)
}

// RateLimitHandler wraps an http.HandlerFunc with rate limiting
func RateLimitHandler(handler http.HandlerFunc, rl *RateLimiter) http.Handler {
	return WithRateLimit(handler, rl)
}

func (rl *RateLimiter) cleanupLoop() {
	for {
		select {
		case <-rl.cleanupTicker.C:
			rl.cleanup()
		case <-rl.done:
			return
		}
	}
}

// Stop gracefully stops the rate limiter
func (rl *RateLimiter) Stop() {
	select {
	case <-rl.done:
		// Already stopped
		return
	default:
		close(rl.done)
		rl.cleanupTicker.Stop()

		// Clear all limiters
		rl.mu.Lock()
		rl.limiters = make(map[string]*limiterState)
		rl.mu.Unlock()
	}
}

// UnaryInterceptor method for RateLimiter
func (rl *RateLimiter) UnaryInterceptor() connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			// Implement rate limiting logic here
			// For example, check if the request is allowed and return an error if not
			if !rl.allowRequest(req.Header()) {
				return nil, connect.NewError(connect.CodeResourceExhausted, errors.New("rate limit exceeded"))
			}
			// Call the next handler if the request is allowed
			return next(ctx, req)
		}
	}
}

// Helper method to check if a request is allowed
func (rl *RateLimiter) allowRequest(header http.Header) bool {
	// Extract client ID or other necessary information from the header
	clientID := header.Get("X-API-Key")
	if clientID == "" {
		// Fall back to other identification methods
		clientID = header.Get("X-Client-Id")
		if clientID == "" {
			return false // Require some form of identification
		}
	}

	// Implement your rate limiting logic here using clientID
	limiter := rl.getLimiter(clientID)
	return limiter.Allow()
}

// GetStats returns current rate limiter statistics
func (rl *RateLimiter) GetStats() map[string]interface{} {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	active := 0
	for _, state := range rl.limiters {
		if time.Since(state.lastUsed) < rl.expiration {
			active++
		}
	}

	return map[string]interface{}{
		"total_clients":  len(rl.limiters),
		"active_clients": active,
		"rate_limit":     float64(rl.rate),
		"burst_limit":    rl.burst,
		"expiration":     rl.expiration.String(),
	}
}

// Define a custom type for streaming interceptors
type StreamingInterceptor func(connect.StreamingHandlerFunc) connect.StreamingHandlerFunc

func (rl *RateLimiter) StreamInterceptor() StreamingInterceptor {
	return func(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
		return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
			// Implement rate limiting logic here
			if !rl.allowRequest(conn.RequestHeader()) {
				return connect.NewError(connect.CodeResourceExhausted, errors.New("rate limit exceeded"))
			}
			return next(ctx, conn)
		}
	}
}
