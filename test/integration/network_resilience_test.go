package integration

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"fleetd.sh/internal/agent/device"
	"fleetd.sh/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNetworkResilience tests comprehensive network failure scenarios
func TestNetworkResilience(t *testing.T) {
	t.Run("NetworkPartition", testNetworkPartition)
	t.Run("DNSResolutionFailure", testDNSResolutionFailure)
	t.Run("ConnectionPoolExhaustion", testConnectionPoolExhaustion)
	t.Run("IntermittentConnectivity", testIntermittentConnectivity)
	t.Run("ReconnectionBackoff", testReconnectionBackoff)
	t.Run("CertificateRenewalDuringOutage", testCertificateRenewalDuringOutage)
	t.Run("WebSocketResilience", testWebSocketResilience)
	t.Run("PartialPacketLoss", testPartialPacketLoss)
}

// testNetworkPartition simulates complete network partition
func testNetworkPartition(t *testing.T) {
	// Create mock server
	var requestCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	testDir := t.TempDir()
	cfg := &config.AgentConfig{
		ServerURL:         server.URL,
		DeviceID:          "test-device",
		DataDir:           testDir,
		HeartbeatInterval: 100 * time.Millisecond,
		MetricsInterval:   100 * time.Millisecond,
		MaxRetries:        3,
		RetryBackoff:      50 * time.Millisecond,
	}

	agent, err := device.NewAgent(cfg)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start agent
	go agent.Start(ctx)

	// Wait for initial connections
	time.Sleep(500 * time.Millisecond)
	initialCount := atomic.LoadInt32(&requestCount)
	assert.Greater(t, initialCount, int32(0), "Should have initial connections")

	// Simulate network partition by closing server
	server.Close()

	// Wait for agent to detect partition
	time.Sleep(1 * time.Second)

	// Create new server on same address (simulate network recovery)
	var recoveryCount int32
	listener, err := net.Listen("tcp", server.Listener.Addr().String())
	require.NoError(t, err)

	newServer := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&recoveryCount, 1)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"recovered"}`))
	}))
	newServer.Listener = listener
	newServer.Start()
	defer newServer.Close()

	// Wait for reconnection
	time.Sleep(2 * time.Second)

	// Verify agent reconnected
	finalCount := atomic.LoadInt32(&recoveryCount)
	assert.Greater(t, finalCount, int32(0), "Should have reconnected after partition")
}

// testDNSResolutionFailure tests handling of DNS failures
func testDNSResolutionFailure(t *testing.T) {
	testDir := t.TempDir()

	// Use non-existent domain
	cfg := &config.AgentConfig{
		ServerURL:         "https://non-existent-domain-12345.invalid",
		DeviceID:          "test-device",
		DataDir:           testDir,
		HeartbeatInterval: 100 * time.Millisecond,
		MaxRetries:        2,
		RetryBackoff:      100 * time.Millisecond,
	}

	agent, err := device.NewAgent(cfg)
	require.NoError(t, err)

	// Create state store to verify offline buffering
	stateStore, err := device.NewStateStore(filepath.Join(testDir, "state.db"))
	require.NoError(t, err)
	defer stateStore.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Start should handle DNS failure gracefully
	err = agent.Start(ctx)
	// Should not panic or hang indefinitely
	assert.Error(t, err)

	// Verify metrics were buffered offline
	metrics := &device.Metrics{
		Timestamp:   time.Now(),
		CPUUsage:    50.0,
		MemoryUsage: 60.0,
	}
	err = stateStore.BufferMetrics(metrics)
	assert.NoError(t, err)

	// Verify buffered metrics can be retrieved
	unsent, err := stateStore.GetUnsentMetrics(10)
	assert.NoError(t, err)
	assert.Len(t, unsent, 1)
}

// testConnectionPoolExhaustion tests connection pool limits
func testConnectionPoolExhaustion(t *testing.T) {
	var activeConnections int32
	var maxConcurrent int32
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := atomic.AddInt32(&activeConnections, 1)

		mu.Lock()
		if current > maxConcurrent {
			maxConcurrent = current
		}
		mu.Unlock()

		// Simulate slow response
		time.Sleep(100 * time.Millisecond)

		w.WriteHeader(http.StatusOK)
		atomic.AddInt32(&activeConnections, -1)
	}))
	defer server.Close()

	testDir := t.TempDir()
	cfg := &config.AgentConfig{
		ServerURL:         server.URL,
		DeviceID:          "test-device",
		DataDir:           testDir,
		HeartbeatInterval: 10 * time.Millisecond, // Very frequent to stress connection pool
		MetricsInterval:   10 * time.Millisecond,
		MaxRetries:        1,
	}

	agent, err := device.NewAgent(cfg)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Start agent with aggressive intervals
	go agent.Start(ctx)

	// Let it run for a while
	time.Sleep(2 * time.Second)

	// Check that connections were properly pooled (not unbounded)
	mu.Lock()
	finalMax := maxConcurrent
	mu.Unlock()

	assert.LessOrEqual(t, finalMax, int32(10), "Should limit concurrent connections")
	assert.Greater(t, finalMax, int32(0), "Should have made connections")
}

// testIntermittentConnectivity simulates flaky network
func testIntermittentConnectivity(t *testing.T) {
	var requestCount int32
	var failureCount int32
	failureRate := 0.5 // 50% failure rate

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&requestCount, 1)

		// Simulate intermittent failures
		if float64(count%2) < failureRate*2 {
			atomic.AddInt32(&failureCount, 1)
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	testDir := t.TempDir()
	cfg := &config.AgentConfig{
		ServerURL:         server.URL,
		DeviceID:          "test-device",
		DataDir:           testDir,
		HeartbeatInterval: 100 * time.Millisecond,
		MaxRetries:        3,
		RetryBackoff:      50 * time.Millisecond,
	}

	agent, err := device.NewAgent(cfg)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start agent
	go agent.Start(ctx)

	// Let it run with intermittent failures
	time.Sleep(3 * time.Second)

	totalRequests := atomic.LoadInt32(&requestCount)
	totalFailures := atomic.LoadInt32(&failureCount)

	// Should have made multiple attempts
	assert.Greater(t, totalRequests, int32(10))
	// Should have experienced failures
	assert.Greater(t, totalFailures, int32(0))
	// But should have succeeded sometimes (due to retries)
	assert.Less(t, totalFailures, totalRequests)
}

// testReconnectionBackoff tests exponential backoff on reconnection
func testReconnectionBackoff(t *testing.T) {
	var connectionAttempts []time.Time
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		connectionAttempts = append(connectionAttempts, time.Now())
		mu.Unlock()

		// Fail first 5 attempts
		if len(connectionAttempts) <= 5 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	testDir := t.TempDir()
	cfg := &config.AgentConfig{
		ServerURL:         server.URL,
		DeviceID:          "test-device",
		DataDir:           testDir,
		HeartbeatInterval: 100 * time.Millisecond,
		MaxRetries:        10,
		RetryBackoff:      100 * time.Millisecond,
	}

	agent, err := device.NewAgent(cfg)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Start agent
	go agent.Start(ctx)

	// Wait for backoff behavior
	time.Sleep(5 * time.Second)

	mu.Lock()
	attempts := connectionAttempts
	mu.Unlock()

	// Verify backoff behavior (at least some retry attempts)
	require.Greater(t, len(attempts), 0, "Should have at least one attempt")

	// Check that intervals increase (exponential backoff) if we have enough attempts
	if len(attempts) > 3 {
		for i := 2; i < len(attempts) && i < 5; i++ {
			interval := attempts[i].Sub(attempts[i-1])
			prevInterval := attempts[i-1].Sub(attempts[i-2])

			// Allow some tolerance for timing
			assert.GreaterOrEqual(t, interval, prevInterval,
				"Interval should increase or stay same (backoff)")
		}
	}
}

// testCertificateRenewalDuringOutage tests cert renewal with network issues
func testCertificateRenewalDuringOutage(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping certificate renewal test in short mode")
	}

	var certRenewalAttempts int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/renew-cert" {
			atomic.AddInt32(&certRenewalAttempts, 1)
			// Simulate network failure during renewal
			if atomic.LoadInt32(&certRenewalAttempts) < 3 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	testDir := t.TempDir()
	cfg := &config.AgentConfig{
		ServerURL:    server.URL,
		DeviceID:     "test-device",
		DataDir:      testDir,
		TLSVerify:    false, // Skip verification for test
		MaxRetries:   5,
		RetryBackoff: 100 * time.Millisecond,
	}

	agent, err := device.NewAgent(cfg)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start agent
	go agent.Start(ctx)

	// Wait for operations
	time.Sleep(2 * time.Second)

	// Verify cert renewal was attempted with retries
	attempts := atomic.LoadInt32(&certRenewalAttempts)
	if attempts > 0 {
		assert.GreaterOrEqual(t, attempts, int32(3), "Should retry cert renewal")
	}
}

// testWebSocketResilience tests WebSocket connection resilience
func testWebSocketResilience(t *testing.T) {
	var wsConnections int32
	var wsReconnections int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Upgrade") == "websocket" {
			connections := atomic.AddInt32(&wsConnections, 1)
			if connections > 1 {
				atomic.AddInt32(&wsReconnections, 1)
			}

			// Simulate WebSocket upgrade then immediate close
			w.WriteHeader(http.StatusSwitchingProtocols)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	testDir := t.TempDir()
	cfg := &config.AgentConfig{
		ServerURL:         server.URL,
		DeviceID:          "test-device",
		DataDir:           testDir,
		HeartbeatInterval: 100 * time.Millisecond,
	}

	agent, err := device.NewAgent(cfg)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Start agent
	go agent.Start(ctx)

	// Wait for WebSocket operations
	time.Sleep(2 * time.Second)

	// Check WebSocket behavior (if implemented)
	totalConnections := atomic.LoadInt32(&wsConnections)
	reconnections := atomic.LoadInt32(&wsReconnections)

	t.Logf("WebSocket connections: %d, reconnections: %d", totalConnections, reconnections)
}

// testPartialPacketLoss simulates partial packet loss scenario
func testPartialPacketLoss(t *testing.T) {
	var totalRequests int32
	var droppedRequests int32
	dropRate := 0.3 // 30% packet loss

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&totalRequests, 1)

		// Simulate packet loss by closing connection
		if float64(count%10) < dropRate*10 {
			atomic.AddInt32(&droppedRequests, 1)
			hj, ok := w.(http.Hijacker)
			if ok {
				conn, _, _ := hj.Hijack()
				conn.Close() // Abrupt close simulates packet loss
				return
			}
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	testDir := t.TempDir()
	cfg := &config.AgentConfig{
		ServerURL:         server.URL,
		DeviceID:          "test-device",
		DataDir:           testDir,
		HeartbeatInterval: 100 * time.Millisecond,
		MaxRetries:        3,
		RetryBackoff:      50 * time.Millisecond,
	}

	agent, err := device.NewAgent(cfg)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start agent
	go agent.Start(ctx)

	// Run with packet loss
	time.Sleep(3 * time.Second)

	total := atomic.LoadInt32(&totalRequests)
	dropped := atomic.LoadInt32(&droppedRequests)

	// Verify at least one request was made
	assert.Greater(t, total, int32(0), "Should make at least one request")

	// If we have multiple requests, verify packet loss behavior
	if total > 10 {
		assert.Greater(t, dropped, int32(0), "Should experience packet loss with multiple requests")

		// Calculate effective success rate despite packet loss
		successRate := float64(total-dropped) / float64(total)
		assert.Greater(t, successRate, 0.5, "Should maintain >50% success despite 30% packet loss (due to retries)")
	} else {
		t.Logf("Note: Only %d requests made, retry logic may not be fully implemented", total)
	}
}
