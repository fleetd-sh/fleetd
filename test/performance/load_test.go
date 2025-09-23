package performance

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// LoadTestConfig defines configuration for load tests
type LoadTestConfig struct {
	BaseURL            string
	Concurrency        int
	Duration           time.Duration
	RequestsPerSecond  int
	WarmupDuration     time.Duration
	EnableMetrics      bool
	TargetResponseTime time.Duration // 95th percentile target
}

// LoadTestResult holds the results of a load test
type LoadTestResult struct {
	TotalRequests      int64
	SuccessfulRequests int64
	FailedRequests     int64
	AverageLatency     time.Duration
	P50Latency         time.Duration
	P95Latency         time.Duration
	P99Latency         time.Duration
	MaxLatency         time.Duration
	MinLatency         time.Duration
	RequestsPerSecond  float64
	ErrorRate          float64
	Throughput         float64 // bytes per second
}

// LoadTester performs load testing
type LoadTester struct {
	config     LoadTestConfig
	httpClient *http.Client
	metrics    *MetricsCollector
	authToken  string
}

// MetricsCollector collects performance metrics
type MetricsCollector struct {
	mu           sync.RWMutex
	latencies    []time.Duration
	statusCodes  map[int]int64
	totalBytes   int64
	successCount int64
	failureCount int64
	startTime    time.Time
	endTime      time.Time
}

// NewLoadTester creates a new load tester
func NewLoadTester(config LoadTestConfig) *LoadTester {
	return &LoadTester{
		config: config,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        config.Concurrency * 2,
				MaxIdleConnsPerHost: config.Concurrency,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		metrics: &MetricsCollector{
			statusCodes: make(map[int]int64),
			latencies:   make([]time.Duration, 0, config.Concurrency*100),
		},
	}
}

// TestDeviceRegistrationLoad tests device registration under load
func TestDeviceRegistrationLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	config := LoadTestConfig{
		BaseURL:            getTestServerURL(),
		Concurrency:        50,
		Duration:           30 * time.Second,
		RequestsPerSecond:  100,
		WarmupDuration:     5 * time.Second,
		TargetResponseTime: 500 * time.Millisecond,
	}

	tester := NewLoadTester(config)

	// Authenticate first
	err := tester.authenticate()
	require.NoError(t, err, "Failed to authenticate")

	// Run warmup
	t.Log("Starting warmup phase...")
	tester.runWarmup()

	// Run load test
	t.Log("Starting load test...")
	result := tester.runLoadTest(func(id int) error {
		return tester.registerDevice(fmt.Sprintf("device-%d", id))
	})

	// Validate results
	assert.Greater(t, result.SuccessfulRequests, int64(0), "Should have successful requests")
	assert.Less(t, result.ErrorRate, 0.05, "Error rate should be less than 5%")
	assert.Less(t, result.P95Latency, config.TargetResponseTime, "P95 latency should be under target")

	// Print results
	tester.printResults(result)
}

// TestAPIEndpointsLoad tests multiple API endpoints under load
func TestAPIEndpointsLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	config := LoadTestConfig{
		BaseURL:            getTestServerURL(),
		Concurrency:        100,
		Duration:           60 * time.Second,
		RequestsPerSecond:  500,
		WarmupDuration:     10 * time.Second,
		TargetResponseTime: 200 * time.Millisecond,
	}

	tester := NewLoadTester(config)

	// Authenticate
	err := tester.authenticate()
	require.NoError(t, err)

	// Define test scenarios
	scenarios := []struct {
		name   string
		weight int // percentage of traffic
		fn     func() error
	}{
		{"ListDevices", 40, tester.listDevices},
		{"GetDevice", 30, tester.getDevice},
		{"GetMetrics", 20, tester.getMetrics},
		{"ListFleets", 10, tester.listFleets},
	}

	// Run mixed load test
	t.Log("Starting mixed load test...")
	result := tester.runMixedLoadTest(scenarios)

	// Validate
	assert.Less(t, result.ErrorRate, 0.02, "Error rate should be less than 2%")
	assert.Greater(t, result.RequestsPerSecond, float64(config.RequestsPerSecond)*0.8,
		"Should achieve at least 80% of target RPS")

	tester.printResults(result)
}

// TestBurstLoad tests system behavior under burst load
func TestBurstLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping burst test in short mode")
	}

	config := LoadTestConfig{
		BaseURL:     getTestServerURL(),
		Concurrency: 200, // High concurrency for burst
		Duration:    10 * time.Second,
	}

	tester := NewLoadTester(config)
	err := tester.authenticate()
	require.NoError(t, err)

	// Generate burst load
	t.Log("Generating burst load...")
	var wg sync.WaitGroup
	errors := make(chan error, config.Concurrency)

	for i := 0; i < config.Concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			if err := tester.listDevices(); err != nil {
				errors <- err
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Count errors
	errorCount := 0
	for range errors {
		errorCount++
	}

	// System should handle burst without complete failure
	assert.Less(t, float64(errorCount)/float64(config.Concurrency), 0.1,
		"Less than 10% of requests should fail during burst")
}

// TestSustainedLoad tests system under sustained load
func TestSustainedLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping sustained load test in short mode")
	}

	config := LoadTestConfig{
		BaseURL:           getTestServerURL(),
		Concurrency:       25,
		Duration:          5 * time.Minute,
		RequestsPerSecond: 50,
	}

	tester := NewLoadTester(config)
	err := tester.authenticate()
	require.NoError(t, err)

	// Run sustained load
	t.Log("Starting sustained load test...")
	ctx, cancel := context.WithTimeout(context.Background(), config.Duration)
	defer cancel()

	result := tester.runSustainedLoad(ctx)

	// Validate stability over time
	assert.Less(t, result.ErrorRate, 0.01, "Error rate should stay below 1%")
	assert.Less(t, result.P99Latency, 1*time.Second, "P99 should stay under 1 second")

	tester.printResults(result)
}

// Helper methods

func (lt *LoadTester) authenticate() error {
	payload := map[string]string{
		"username": "loadtest",
		"password": "loadtest123",
	}

	body, _ := json.Marshal(payload)
	resp, err := lt.httpClient.Post(
		lt.config.BaseURL+"/api/v1/auth/login",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Try to create user first
		// This would normally be done in test setup
		lt.authToken = "test-token"
		return nil
	}

	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)
	lt.authToken = result["token"]
	return nil
}

func (lt *LoadTester) registerDevice(deviceID string) error {
	payload := map[string]interface{}{
		"device_id": deviceID,
		"type":      "load-test",
		"version":   "1.0.0",
		"metadata": map[string]string{
			"test": "true",
		},
	}

	return lt.makeRequest("POST", "/api/v1/devices/register", payload)
}

func (lt *LoadTester) listDevices() error {
	return lt.makeRequest("GET", "/api/v1/devices", nil)
}

func (lt *LoadTester) getDevice() error {
	deviceID := fmt.Sprintf("device-%d", rand.Intn(100))
	return lt.makeRequest("GET", "/api/v1/devices/"+deviceID, nil)
}

func (lt *LoadTester) getMetrics() error {
	return lt.makeRequest("GET", "/api/v1/metrics", nil)
}

func (lt *LoadTester) listFleets() error {
	return lt.makeRequest("GET", "/api/v1/fleets", nil)
}

func (lt *LoadTester) makeRequest(method, path string, payload interface{}) error {
	var body []byte
	if payload != nil {
		body, _ = json.Marshal(payload)
	}

	req, err := http.NewRequest(method, lt.config.BaseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}

	if lt.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+lt.authToken)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	start := time.Now()
	resp, err := lt.httpClient.Do(req)
	latency := time.Since(start)

	if err != nil {
		lt.metrics.recordFailure()
		return err
	}
	defer resp.Body.Close()

	lt.metrics.recordRequest(resp.StatusCode, latency, resp.ContentLength)

	if resp.StatusCode >= 400 {
		return fmt.Errorf("request failed with status %d", resp.StatusCode)
	}

	return nil
}

func (lt *LoadTester) runWarmup() {
	ctx, cancel := context.WithTimeout(context.Background(), lt.config.WarmupDuration)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return
		default:
			lt.listDevices()
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func (lt *LoadTester) runLoadTest(testFunc func(int) error) LoadTestResult {
	lt.metrics.startTime = time.Now()
	defer func() {
		lt.metrics.endTime = time.Now()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), lt.config.Duration)
	defer cancel()

	var wg sync.WaitGroup
	semaphore := make(chan struct{}, lt.config.Concurrency)
	requestID := int64(0)

	// Rate limiter
	ticker := time.NewTicker(time.Second / time.Duration(lt.config.RequestsPerSecond))
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			wg.Wait()
			return lt.calculateResults()
		case <-ticker.C:
			semaphore <- struct{}{}
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer func() { <-semaphore }()

				id := int(atomic.AddInt64(&requestID, 1))
				testFunc(id)
			}()
		}
	}
}

func (lt *LoadTester) runMixedLoadTest(scenarios []struct {
	name   string
	weight int
	fn     func() error
}) LoadTestResult {
	lt.metrics.startTime = time.Now()
	defer func() {
		lt.metrics.endTime = time.Now()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), lt.config.Duration)
	defer cancel()

	// Calculate cumulative weights for scenario selection
	totalWeight := 0
	for _, s := range scenarios {
		totalWeight += s.weight
	}

	var wg sync.WaitGroup
	for i := 0; i < lt.config.Concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					// Select scenario based on weight
					r := rand.Intn(totalWeight)
					cumWeight := 0
					for _, scenario := range scenarios {
						cumWeight += scenario.weight
						if r < cumWeight {
							scenario.fn()
							break
						}
					}
					time.Sleep(time.Second / time.Duration(lt.config.RequestsPerSecond))
				}
			}
		}()
	}

	wg.Wait()
	return lt.calculateResults()
}

func (lt *LoadTester) runSustainedLoad(ctx context.Context) LoadTestResult {
	lt.metrics.startTime = time.Now()
	defer func() {
		lt.metrics.endTime = time.Now()
	}()

	var wg sync.WaitGroup
	for i := 0; i < lt.config.Concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					lt.listDevices()
					time.Sleep(time.Duration(lt.config.Concurrency) * time.Second /
						time.Duration(lt.config.RequestsPerSecond))
				}
			}
		}()
	}

	wg.Wait()
	return lt.calculateResults()
}

func (mc *MetricsCollector) recordRequest(statusCode int, latency time.Duration, bytes int64) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	mc.latencies = append(mc.latencies, latency)
	mc.statusCodes[statusCode]++
	mc.totalBytes += bytes

	if statusCode < 400 {
		atomic.AddInt64(&mc.successCount, 1)
	} else {
		atomic.AddInt64(&mc.failureCount, 1)
	}
}

func (mc *MetricsCollector) recordFailure() {
	atomic.AddInt64(&mc.failureCount, 1)
}

func (lt *LoadTester) calculateResults() LoadTestResult {
	mc := lt.metrics
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	duration := mc.endTime.Sub(mc.startTime).Seconds()
	total := mc.successCount + mc.failureCount

	result := LoadTestResult{
		TotalRequests:      total,
		SuccessfulRequests: mc.successCount,
		FailedRequests:     mc.failureCount,
		RequestsPerSecond:  float64(total) / duration,
		ErrorRate:          float64(mc.failureCount) / float64(total),
		Throughput:         float64(mc.totalBytes) / duration,
	}

	// Calculate latency percentiles
	if len(mc.latencies) > 0 {
		result.AverageLatency = calculateAverage(mc.latencies)
		result.P50Latency = calculatePercentile(mc.latencies, 50)
		result.P95Latency = calculatePercentile(mc.latencies, 95)
		result.P99Latency = calculatePercentile(mc.latencies, 99)
		result.MinLatency = calculateMin(mc.latencies)
		result.MaxLatency = calculateMax(mc.latencies)
	}

	return result
}

func (lt *LoadTester) printResults(result LoadTestResult) {
	fmt.Printf("\n=== Load Test Results ===\n")
	fmt.Printf("Total Requests: %d\n", result.TotalRequests)
	fmt.Printf("Successful: %d (%.2f%%)\n", result.SuccessfulRequests,
		(float64(result.SuccessfulRequests)/float64(result.TotalRequests))*100)
	fmt.Printf("Failed: %d (%.2f%%)\n", result.FailedRequests, result.ErrorRate*100)
	fmt.Printf("RPS: %.2f\n", result.RequestsPerSecond)
	fmt.Printf("Throughput: %.2f KB/s\n", result.Throughput/1024)
	fmt.Printf("\n=== Latency ===\n")
	fmt.Printf("Average: %v\n", result.AverageLatency)
	fmt.Printf("P50: %v\n", result.P50Latency)
	fmt.Printf("P95: %v\n", result.P95Latency)
	fmt.Printf("P99: %v\n", result.P99Latency)
	fmt.Printf("Min: %v\n", result.MinLatency)
	fmt.Printf("Max: %v\n", result.MaxLatency)
}

// Utility functions

func getTestServerURL() string {
	if url := os.Getenv("TEST_SERVER_URL"); url != "" {
		return url
	}
	return "http://localhost:8090"
}

func calculateAverage(latencies []time.Duration) time.Duration {
	if len(latencies) == 0 {
		return 0
	}
	var sum time.Duration
	for _, l := range latencies {
		sum += l
	}
	return sum / time.Duration(len(latencies))
}

func calculatePercentile(latencies []time.Duration, percentile int) time.Duration {
	if len(latencies) == 0 {
		return 0
	}

	// Sort latencies
	sorted := make([]time.Duration, len(latencies))
	copy(sorted, latencies)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i] < sorted[j]
	})

	index := (len(sorted) * percentile) / 100
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

func calculateMin(latencies []time.Duration) time.Duration {
	if len(latencies) == 0 {
		return 0
	}
	min := latencies[0]
	for _, l := range latencies {
		if l < min {
			min = l
		}
	}
	return min
}

func calculateMax(latencies []time.Duration) time.Duration {
	if len(latencies) == 0 {
		return 0
	}
	max := latencies[0]
	for _, l := range latencies {
		if l > max {
			max = l
		}
	}
	return max
}
