package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	interceptor := rl.UnaryInterceptor()

	// Create test handler
	handler := func(_ context.Context, _ connect.AnyRequest) (connect.AnyResponse, error) {
		data := "ok"
		return connect.NewResponse(&data), nil
	}

	// Create test context with client ID
	ctx := context.Background()
	req := connect.NewRequest(&struct{}{})
	req.Header().Set("x-api-key", "test-client")

	// Test successful requests within burst limit
	for i := 0; i < 2; i++ {
		resp, err := interceptor(func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			return handler(ctx, req)
		})(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, "ok", *(resp.Any().(*string)))
	}

	// Test rate limit exceeded
	resp, err := interceptor(func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		return handler(ctx, req)
	})(ctx, req)
	require.Error(t, err)
	assert.Equal(t, connect.CodeResourceExhausted, connect.CodeOf(err))
	assert.Nil(t, resp)

	// Test request after waiting
	time.Sleep(time.Second)
	resp, err = interceptor(func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		return handler(ctx, req)
	})(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, "ok", *(resp.Any().(*string)))
}

type mockStreamingHandler[T any] struct {
	ctx    context.Context
	header http.Header
}

func (s *mockStreamingHandler[T]) Spec() connect.Spec           { return connect.Spec{} }
func (s *mockStreamingHandler[T]) ResponseHeader() http.Header  { return http.Header{} }
func (s *mockStreamingHandler[T]) Receive(msg T) error          { return nil }
func (s *mockStreamingHandler[T]) Context() context.Context     { return s.ctx }
func (s *mockStreamingHandler[T]) RequestHeader() http.Header   { return s.header }
func (s *mockStreamingHandler[T]) Peer() connect.Peer           { return connect.Peer{} }
func (s *mockStreamingHandler[T]) ResponseTrailer() http.Header { return http.Header{} }
func (s *mockStreamingHandler[T]) Send(msg T) error             { return nil }

func TestRateLimiter_StreamInterceptor(t *testing.T) {
	// Create rate limiter
	rl := NewRateLimiter(RateLimiterConfig{
		Rate:       1,
		Burst:      2,
		Expiration: time.Hour,
	})
	defer rl.Stop()

	// Create test interceptor
	interceptor := rl.StreamInterceptor()

	// Create test handler
	handler := func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		return nil
	}

	// Create test stream with client ID
	ctx := context.Background()
	req := connect.NewRequest(&struct{}{})
	req.Header().Set("x-api-key", "test-client")
	stream := &mockStreamingHandler[any]{
		ctx:    ctx,
		header: req.Header(),
	}

	// Test successful requests within burst limit
	for i := 0; i < 2; i++ {
		err := interceptor(func(ctx context.Context, conn connect.StreamingHandlerConn) error {
			return handler(ctx, conn)
		})(ctx, stream)
		require.NoError(t, err)
	}

	// Test rate limit exceeded
	wrapped := interceptor(handler)
	err := wrapped(ctx, stream)
	require.Error(t, err)
	assert.Equal(t, connect.CodeResourceExhausted, connect.CodeOf(err))

	// Test request after waiting
	time.Sleep(time.Second)
	wrapped = interceptor(handler)
	err = wrapped(ctx, stream)
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
