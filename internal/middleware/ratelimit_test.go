package middleware

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"
)

func TestRateLimiter_UnaryInterceptor(t *testing.T) {
	// Create rate limiter with low limit for testing
	rl, err := NewRateLimiter(RateLimiterConfig{
		Rate:       1, // 1 request per second
		Burst:      2, // Allow burst of 2
		Expiration: time.Hour,
	})
	require.NoError(t, err)
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
	rl, err := NewRateLimiter(RateLimiterConfig{
		Rate:       1,
		Burst:      2,
		Expiration: time.Hour,
	})
	require.NoError(t, err)
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
	errExceeded := wrapped(ctx, stream)
	require.Error(t, errExceeded)
	assert.Equal(t, connect.CodeResourceExhausted, connect.CodeOf(errExceeded))

	// Test request after waiting
	time.Sleep(time.Second)
	wrapped = interceptor(handler)
	err = wrapped(ctx, stream)
	require.NoError(t, err)
}

func TestRateLimiter_HTTPMiddleware(t *testing.T) {
	// Create rate limiter
	rl, err := NewRateLimiter(RateLimiterConfig{
		Rate:       1,
		Burst:      2,
		Expiration: time.Hour,
	})
	require.NoError(t, err)
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
	rl, err := NewRateLimiter(RateLimiterConfig{
		Rate:       1,
		Burst:      1,
		Expiration: 100 * time.Millisecond,
	})
	require.NoError(t, err)
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
	rl, err := NewRateLimiter(RateLimiterConfig{
		Rate:       1,
		Burst:      1,
		Expiration: time.Hour,
	})
	require.NoError(t, err)
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

func TestRateLimiterConfig_Validation(t *testing.T) {
	tests := []struct {
		name    string
		config  RateLimiterConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			config: RateLimiterConfig{
				Rate:       10,
				Burst:      20,
				Expiration: time.Hour,
			},
			wantErr: false,
		},
		{
			name: "negative rate",
			config: RateLimiterConfig{
				Rate:       -1,
				Burst:      10,
				Expiration: time.Hour,
			},
			wantErr: true,
			errMsg:  "rate must be positive",
		},
		{
			name: "zero rate",
			config: RateLimiterConfig{
				Rate:       0,
				Burst:      10,
				Expiration: time.Hour,
			},
			wantErr: true,
			errMsg:  "rate must be positive",
		},
		{
			name: "negative burst",
			config: RateLimiterConfig{
				Rate:       10,
				Burst:      -1,
				Expiration: time.Hour,
			},
			wantErr: true,
			errMsg:  "burst must be positive",
		},
		{
			name: "zero burst",
			config: RateLimiterConfig{
				Rate:       10,
				Burst:      0,
				Expiration: time.Hour,
			},
			wantErr: true,
			errMsg:  "burst must be positive",
		},
		{
			name: "negative expiration",
			config: RateLimiterConfig{
				Rate:       10,
				Burst:      20,
				Expiration: -time.Hour,
			},
			wantErr: true,
			errMsg:  "expiration must be positive",
		},
		{
			name: "burst less than rate",
			config: RateLimiterConfig{
				Rate:       10,
				Burst:      5,
				Expiration: time.Hour,
			},
			wantErr: true,
			errMsg:  "burst should not be less than rate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestRateLimiter_GetStats(t *testing.T) {
	rl, err := NewRateLimiter(RateLimiterConfig{
		Rate:       10,
		Burst:      20,
		Expiration: time.Hour,
	})
	require.NoError(t, err)
	defer rl.Stop()

	// Create some clients
	rl.getLimiter("client1")
	rl.getLimiter("client2")
	rl.getLimiter("client3")

	// Get stats
	stats := rl.GetStats()
	assert.Equal(t, 3, stats["total_clients"])
	assert.Equal(t, 3, stats["active_clients"])
	assert.Equal(t, float64(10), stats["rate_limit"])
	assert.Equal(t, 20, stats["burst_limit"])
	assert.Equal(t, time.Hour.String(), stats["expiration"])
}

func TestRateLimiter_ClientIDExtraction(t *testing.T) {
	tests := []struct {
		name        string
		setupCtx    func() context.Context
		expectedID  string
		expectEmpty bool
	}{
		{
			name: "user ID in context",
			setupCtx: func() context.Context {
				return context.WithValue(context.Background(), "user_id", "user123")
			},
			expectedID: "user:user123",
		},
		{
			name: "API key in metadata",
			setupCtx: func() context.Context {
				md := metadata.New(map[string]string{"x-api-key": "key123"})
				return metadata.NewIncomingContext(context.Background(), md)
			},
			expectedID: "api:key123",
		},
		{
			name: "empty context",
			setupCtx: func() context.Context {
				return context.Background()
			},
			expectEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := tt.setupCtx()
			clientID := getClientID(ctx)

			if tt.expectEmpty {
				assert.Empty(t, clientID)
			} else {
				assert.Equal(t, tt.expectedID, clientID)
			}
		})
	}
}

func TestRateLimiter_StopIdempotency(t *testing.T) {
	rl, err := NewRateLimiter(RateLimiterConfig{
		Rate:       10,
		Burst:      20,
		Expiration: time.Hour,
	})
	require.NoError(t, err)

	// First stop should work
	rl.Stop()

	// Second stop should not panic
	require.NotPanics(t, func() {
		rl.Stop()
	})
}

func TestRateLimiter_ConcurrentAccess(t *testing.T) {
	rl, err := NewRateLimiter(RateLimiterConfig{
		Rate:       100,
		Burst:      200,
		Expiration: time.Hour,
	})
	require.NoError(t, err)
	defer rl.Stop()

	// Run concurrent requests from multiple clients
	var wg sync.WaitGroup
	numClients := 10
	numRequests := 50

	for i := 0; i < numClients; i++ {
		wg.Add(1)
		go func(clientID int) {
			defer wg.Done()

			for j := 0; j < numRequests; j++ {
				limiter := rl.getLimiter(fmt.Sprintf("client%d", clientID))
				_ = limiter.Allow()
				time.Sleep(time.Millisecond)
			}
		}(i)
	}

	wg.Wait()

	// Verify stats
	stats := rl.GetStats()
	assert.Equal(t, numClients, stats["total_clients"])
}

func TestRateLimiter_BurstBehavior(t *testing.T) {
	rl, err := NewRateLimiter(RateLimiterConfig{
		Rate:       2,  // 2 per second
		Burst:      5,  // Allow burst of 5
		Expiration: time.Hour,
	})
	require.NoError(t, err)
	defer rl.Stop()

	limiter := rl.getLimiter("burst-test")

	// Should allow burst of 5 immediately
	for i := 0; i < 5; i++ {
		assert.True(t, limiter.Allow(), "request %d should be allowed", i+1)
	}

	// 6th request should be denied
	assert.False(t, limiter.Allow(), "6th request should be denied")

	// After 1 second, should allow 2 more requests (rate of 2/sec)
	time.Sleep(1 * time.Second)
	assert.True(t, limiter.Allow(), "request after 1 sec should be allowed")
	assert.True(t, limiter.Allow(), "second request after 1 sec should be allowed")
	assert.False(t, limiter.Allow(), "third request after 1 sec should be denied")
}

func TestRateLimiter_LastUsedUpdate(t *testing.T) {
	rl, err := NewRateLimiter(RateLimiterConfig{
		Rate:       10,
		Burst:      20,
		Expiration: 100 * time.Millisecond,
	})
	require.NoError(t, err)
	defer rl.Stop()

	// Create a client
	clientID := "test-client"
	_ = rl.getLimiter(clientID)

	// Get initial last used time
	rl.mu.RLock()
	state1 := rl.limiters[clientID]
	lastUsed1 := state1.lastUsed
	rl.mu.RUnlock()

	// Wait a bit
	time.Sleep(10 * time.Millisecond)

	// Access the limiter again
	_ = rl.getLimiter(clientID)

	// Verify last used was updated
	rl.mu.RLock()
	state2 := rl.limiters[clientID]
	lastUsed2 := state2.lastUsed
	rl.mu.RUnlock()

	assert.True(t, lastUsed2.After(lastUsed1), "lastUsed should be updated on access")
}
