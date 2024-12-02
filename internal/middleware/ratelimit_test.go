package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestRateLimiter_UnaryInterceptor(t *testing.T) {
	// Create rate limiter with low limit for testing
	rl := NewRateLimiter(RateLimiterConfig{
		Rate:       1, // 1 request per second
		Burst:      2, // Allow burst of 2
		Expiration: time.Hour,
	})
	defer rl.Stop()

	// Create test interceptor
	interceptor := rl.UnaryServerInterceptor()

	// Create test handler
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return "ok", nil
	}

	// Create test context with client ID
	ctx := metadata.NewIncomingContext(context.Background(),
		metadata.Pairs("x-api-key", "test-client"))

	// Test successful requests within burst limit
	for i := 0; i < 2; i++ {
		resp, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{}, handler)
		require.NoError(t, err)
		assert.Equal(t, "ok", resp)
	}

	// Test rate limit exceeded
	resp, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{}, handler)
	require.Error(t, err)
	assert.Equal(t, codes.ResourceExhausted, status.Code(err))
	assert.Nil(t, resp)

	// Test request after waiting
	time.Sleep(time.Second)
	resp, err = interceptor(ctx, nil, &grpc.UnaryServerInfo{}, handler)
	require.NoError(t, err)
	assert.Equal(t, "ok", resp)
}

type mockServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *mockServerStream) Context() context.Context {
	return s.ctx
}

func TestRateLimiter_StreamInterceptor(t *testing.T) {
	// Create rate limiter
	rl := NewRateLimiter(RateLimiterConfig{
		Rate:       1,
		Burst:      2,
		Expiration: time.Hour,
	})
	defer rl.Stop()

	// Create test interceptor
	interceptor := rl.StreamServerInterceptor()

	// Create test handler
	handler := func(srv interface{}, stream grpc.ServerStream) error {
		return nil
	}

	// Create test stream with client ID
	ctx := metadata.NewIncomingContext(context.Background(),
		metadata.Pairs("x-api-key", "test-client"))
	stream := &mockServerStream{ctx: ctx}

	// Test successful requests within burst limit
	for i := 0; i < 2; i++ {
		err := interceptor(nil, stream, &grpc.StreamServerInfo{}, handler)
		require.NoError(t, err)
	}

	// Test rate limit exceeded
	err := interceptor(nil, stream, &grpc.StreamServerInfo{}, handler)
	require.Error(t, err)
	assert.Equal(t, codes.ResourceExhausted, status.Code(err))

	// Test request after waiting
	time.Sleep(time.Second)
	err = interceptor(nil, stream, &grpc.StreamServerInfo{}, handler)
	require.NoError(t, err)
}

func TestRateLimiter_HTTPMiddleware(t *testing.T) {
	// Create rate limiter
	rl := NewRateLimiter(RateLimiterConfig{
		Rate:       1,
		Burst:      2,
		Expiration: time.Hour,
	})
	defer rl.Stop()

	// Create test handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Create test server
	server := httptest.NewServer(WithRateLimit(handler, rl))
	defer server.Close()

	// Create test client
	client := &http.Client{}

	// Create test request
	req, err := http.NewRequest("GET", server.URL, nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", "test-client")

	// Test successful requests within burst limit
	for i := 0; i < 2; i++ {
		resp, err := client.Do(req)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		resp.Body.Close()
	}

	// Test rate limit exceeded
	resp, err := client.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
	resp.Body.Close()

	// Test request after waiting
	time.Sleep(time.Second)
	resp, err = client.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()
}

func TestRateLimiter_Cleanup(t *testing.T) {
	// Create rate limiter with short expiration
	rl := NewRateLimiter(RateLimiterConfig{
		Rate:       1,
		Burst:      1,
		Expiration: 100 * time.Millisecond,
	})
	defer rl.Stop()

	// Create test client
	clientID := "test-client"
	limiter := rl.getLimiter(clientID)
	require.NotNil(t, limiter)

	// Wait for cleanup
	time.Sleep(200 * time.Millisecond)

	// Verify limiter was removed
	rl.mu.RLock()
	_, exists := rl.limiters[clientID]
	rl.mu.RUnlock()
	assert.False(t, exists)
}

func TestRateLimiter_MultipleClients(t *testing.T) {
	// Create rate limiter
	rl := NewRateLimiter(RateLimiterConfig{
		Rate:       1,
		Burst:      1,
		Expiration: time.Hour,
	})
	defer rl.Stop()

	// Test that different clients have separate limits
	client1 := rl.getLimiter("client1")
	client2 := rl.getLimiter("client2")

	// Client 1 uses its limit
	assert.True(t, client1.Allow())
	assert.False(t, client1.Allow())

	// Client 2 should still have its limit
	assert.True(t, client2.Allow())
	assert.False(t, client2.Allow())
}
