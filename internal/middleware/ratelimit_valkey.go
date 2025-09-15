package middleware

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"github.com/redis/go-redis/v9"
)

// ValkeyRateLimiter implements rate limiting using Valkey
type ValkeyRateLimiter struct {
	client        *redis.Client
	requestLimit  int
	windowSeconds int
}

// NewValkeyRateLimiter creates a new Valkey-based rate limiter
func NewValkeyRateLimiter(addr string, requestLimit int, windowSeconds int) (*ValkeyRateLimiter, error) {
	client := redis.NewClient(&redis.Options{
		Addr:         addr,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Valkey: %w", err)
	}

	return &ValkeyRateLimiter{
		client:        client,
		requestLimit:  requestLimit,
		windowSeconds: windowSeconds,
	}, nil
}

// UnaryInterceptor returns a Connect interceptor for rate limiting
func (r *ValkeyRateLimiter) UnaryInterceptor() connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			clientID := r.getClientID(req.Header())

			allowed, remaining := r.checkRateLimit(ctx, clientID)

			// Add rate limit headers to response
			if !allowed {
				return nil, connect.NewError(
					connect.CodeResourceExhausted,
					fmt.Errorf("rate limit exceeded: %d requests per %d seconds", r.requestLimit, r.windowSeconds),
				)
			}

			// Process request and add headers to response
			resp, err := next(ctx, req)
			if resp != nil && resp.Any() != nil {
				header := resp.Header()
				header.Set("X-RateLimit-Limit", fmt.Sprintf("%d", r.requestLimit))
				header.Set("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
				header.Set("X-RateLimit-Window", fmt.Sprintf("%d", r.windowSeconds))
			}

			return resp, err
		}
	}
}

// HTTPMiddleware returns HTTP middleware for rate limiting
func (r *ValkeyRateLimiter) HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		clientID := r.getClientIDFromHTTP(req)

		allowed, remaining := r.checkRateLimit(req.Context(), clientID)

		// Set rate limit headers
		w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", r.requestLimit))
		w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
		w.Header().Set("X-RateLimit-Window", fmt.Sprintf("%d", r.windowSeconds))

		if !allowed {
			http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, req)
	})
}

// checkRateLimit checks if request is allowed under rate limit using sliding window
func (r *ValkeyRateLimiter) checkRateLimit(ctx context.Context, clientID string) (allowed bool, remaining int) {
	key := fmt.Sprintf("ratelimit:%s", clientID)
	now := time.Now().Unix()
	windowStart := now - int64(r.windowSeconds)

	// Use sliding window with sorted sets
	pipe := r.client.Pipeline()

	// Remove old entries outside window
	pipe.ZRemRangeByScore(ctx, key, "-inf", fmt.Sprintf("%d", windowStart))

	// Add current request
	pipe.ZAdd(ctx, key, redis.Z{
		Score:  float64(now),
		Member: fmt.Sprintf("%d:%d", now, time.Now().UnixNano()),
	})

	// Count requests in window
	pipe.ZCard(ctx, key)

	// Set expiry to clean up old keys
	pipe.Expire(ctx, key, time.Duration(r.windowSeconds)*time.Second)

	results, err := pipe.Exec(ctx)
	if err != nil {
		// On error, be permissive and allow request
		return true, 0
	}

	// Get count from results (third command)
	intCmd, ok := results[2].(*redis.IntCmd)
	if !ok {
		// On type assertion failure, be permissive and allow request
		return true, 0
	}
	count := intCmd.Val()

	allowed = count <= int64(r.requestLimit)
	remaining = r.requestLimit - int(count)
	if remaining < 0 {
		remaining = 0
	}

	// If over limit, remove the request we just added
	if !allowed {
		r.client.ZRem(ctx, key, fmt.Sprintf("%d:%d", now, time.Now().UnixNano()))
	}

	return allowed, remaining
}

// getClientID extracts client identifier from Connect headers
func (r *ValkeyRateLimiter) getClientID(header http.Header) string {
	// Check for API key first
	if apiKey := header.Get("X-API-Key"); apiKey != "" {
		return "api:" + apiKey
	}

	// Check for device ID
	if deviceID := header.Get("X-Device-ID"); deviceID != "" {
		return "device:" + deviceID
	}

	// Fall back to IP from forwarded headers
	if ip := header.Get("X-Forwarded-For"); ip != "" {
		return "ip:" + ip
	}

	if ip := header.Get("X-Real-IP"); ip != "" {
		return "ip:" + ip
	}

	return "ip:unknown"
}

// getClientIDFromHTTP extracts client identifier from HTTP request
func (r *ValkeyRateLimiter) getClientIDFromHTTP(req *http.Request) string {
	return r.getClientID(req.Header)
}

// Close closes the Valkey connection
func (r *ValkeyRateLimiter) Close() error {
	return r.client.Close()
}
