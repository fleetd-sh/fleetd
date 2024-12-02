package middleware

import (
	"context"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
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

// NewRateLimiter creates a new RateLimiter
func NewRateLimiter(config RateLimiterConfig) *RateLimiter {
	rl := &RateLimiter{
		limiters:      make(map[string]*limiterState),
		rate:          rate.Limit(config.Rate),
		burst:         config.Burst,
		expiration:    config.Expiration,
		cleanupTicker: time.NewTicker(config.Expiration),
		done:          make(chan struct{}),
	}

	go rl.cleanupLoop()
	return rl
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

// UnaryServerInterceptor returns a gRPC interceptor that rate limits requests
func (rl *RateLimiter) UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// Get client ID from context (e.g., API key or IP address)
		clientID := getClientID(ctx)
		if clientID == "" {
			return nil, status.Error(codes.Unauthenticated, "client ID not found")
		}

		// Get rate limiter for client
		limiter := rl.getLimiter(clientID)

		// Try to allow request
		if !limiter.Allow() {
			return nil, status.Error(codes.ResourceExhausted, "rate limit exceeded")
		}

		// Process request
		return handler(ctx, req)
	}
}

// StreamServerInterceptor returns a gRPC interceptor that rate limits streams
func (rl *RateLimiter) StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		// Get client ID from context
		clientID := getClientID(ss.Context())
		if clientID == "" {
			return status.Error(codes.Unauthenticated, "client ID not found")
		}

		// Get rate limiter for client
		limiter := rl.getLimiter(clientID)

		// Create wrapped stream that rate limits RecvMsg
		wrappedStream := &rateLimitedServerStream{
			ServerStream: ss,
			limiter:      limiter,
		}

		// Process stream
		return handler(srv, wrappedStream)
	}
}

// rateLimitedServerStream wraps a grpc.ServerStream with rate limiting
type rateLimitedServerStream struct {
	grpc.ServerStream
	limiter *rate.Limiter
}

// RecvMsg rate limits incoming messages
func (s *rateLimitedServerStream) RecvMsg(m interface{}) error {
	if !s.limiter.Allow() {
		return status.Error(codes.ResourceExhausted, "rate limit exceeded")
	}
	return s.ServerStream.RecvMsg(m)
}

// getClientID gets the client ID from the context
// This should be customized based on your authentication mechanism
func getClientID(ctx context.Context) string {
	// Example: Get API key from metadata
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if keys := md.Get("x-api-key"); len(keys) > 0 {
			return keys[0]
		}
	}

	// Example: Get IP address from peer info
	if pr, ok := peer.FromContext(ctx); ok {
		return pr.Addr.String()
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

func (rl *RateLimiter) Stop() {
	close(rl.done)
	rl.cleanupTicker.Stop()
}
