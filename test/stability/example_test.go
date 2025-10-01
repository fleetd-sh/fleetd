package stability

import (
	"context"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStabilityFramework demonstrates how to use the stability testing framework
func TestStabilityFramework(t *testing.T) {
	// Create test configuration
	config := DefaultConfig()
	config.Duration = 30 * time.Second // Short duration for testing
	config.MonitorInterval = 1 * time.Second
	config.ValidationInterval = 2 * time.Second
	config.OutputDir = t.TempDir()

	// Create stability test
	test, err := NewStabilityTest(config)
	require.NoError(t, err)
	require.NotNil(t, test)

	// Add validators
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	memValidator := NewMemoryLeakValidator(config, logger)
	test.AddValidator(memValidator)

	deadlockDetector := NewDeadlockDetector(config, logger)
	test.AddValidator(deadlockDetector)

	// Start test in background
	done := make(chan error, 1)
	go func() {
		done <- test.Start()
	}()

	// Wait for test to complete
	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(45 * time.Second):
		test.Stop()
		t.Fatal("Test timeout")
	}

	// Verify results
	metrics := test.GetMetrics()
	snapshots := metrics.GetSnapshots()
	assert.Greater(t, len(snapshots), 10, "Should have collected multiple snapshots")

	errors := metrics.GetErrors()
	assert.Empty(t, errors, "Should not have any errors in short test")
}

// TestMemoryLeakValidator tests the memory leak detection
func TestMemoryLeakValidator(t *testing.T) {
	config := DefaultConfig()
	config.MemoryLeakThreshold = 5.0 // Low threshold for testing

	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	validator := NewMemoryLeakValidator(config, logger)
	assert.Equal(t, "memory_leak", validator.Name())

	ctx := context.Background()

	// Should not detect leak with few measurements
	for i := 0; i < 5; i++ {
		err := validator.Validate(ctx)
		assert.NoError(t, err)
	}

	// Reset and test with many measurements (simulating normal operation)
	err := validator.Reset()
	assert.NoError(t, err)

	// Validate multiple times to build baseline
	for i := 0; i < 50; i++ {
		err := validator.Validate(ctx)
		assert.NoError(t, err)
		time.Sleep(10 * time.Millisecond)
	}
}

// TestConnectionStabilityValidator tests connection validation
func TestConnectionStabilityValidator(t *testing.T) {
	config := DefaultConfig()
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	// Test with invalid endpoints to simulate failures
	endpoints := []string{"http://invalid-host:8080/health", "invalid-host:8080"}
	validator := NewConnectionStabilityValidator(config, logger, endpoints)

	assert.Equal(t, "connection_stability", validator.Name())

	ctx := context.Background()

	// Should detect connection failures
	for i := 0; i < 10; i++ {
		err := validator.Validate(ctx)
		// We expect errors here due to invalid endpoints
		if i >= 5 {
			// After enough failures, should trigger error
			if err != nil {
				assert.Contains(t, err.Error(), "connection error rate")
				break
			}
		}
	}
}

// TestDatabaseIntegrityValidator tests database validation
func TestDatabaseIntegrityValidator(t *testing.T) {
	config := DefaultConfig()
	config.DatabasePath = ":memory:" // Use in-memory SQLite for testing

	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	validator, err := NewDatabaseIntegrityValidator(config, logger)
	require.NoError(t, err)
	require.NotNil(t, validator)

	assert.Equal(t, "database_integrity", validator.Name())

	ctx := context.Background()

	// Should pass validation with in-memory database
	err = validator.Validate(ctx)
	assert.NoError(t, err)

	// Test multiple validations
	for i := 0; i < 5; i++ {
		err := validator.Validate(ctx)
		assert.NoError(t, err)
	}

	// Test reset
	err = validator.Reset()
	assert.NoError(t, err)
}

// TestDeadlockDetector tests deadlock detection
func TestDeadlockDetector(t *testing.T) {
	config := DefaultConfig()
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	detector := NewDeadlockDetector(config, logger)
	assert.Equal(t, "deadlock_detector", detector.Name())

	ctx := context.Background()

	// Should not detect deadlock with normal operation
	for i := 0; i < 25; i++ {
		err := detector.Validate(ctx)
		assert.NoError(t, err)

		// Create some goroutines to vary the count
		if i%5 == 0 {
			go func() {
				time.Sleep(10 * time.Millisecond)
			}()
		}
	}
}

// TestPerformanceValidator tests performance monitoring
func TestPerformanceValidator(t *testing.T) {
	config := DefaultConfig()
	config.PerformanceThreshold = 50.0 // High threshold for testing

	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	validator := NewPerformanceValidator(config, logger)
	assert.Equal(t, "performance", validator.Name())

	ctx := context.Background()

	// Build baseline measurements
	for i := 0; i < 20; i++ {
		err := validator.Validate(ctx)
		assert.NoError(t, err)
		time.Sleep(5 * time.Millisecond)
	}
}

// TestSystemMonitor tests the system monitoring functionality
func TestSystemMonitor(t *testing.T) {
	config := DefaultConfig()
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	monitor, err := NewSystemMonitor(config, logger)
	require.NoError(t, err)
	require.NotNil(t, monitor)

	// Collect metrics
	err = monitor.CollectMetrics()
	assert.NoError(t, err)

	// Get system info
	sysInfo, err := monitor.GetSystemInfo()
	require.NoError(t, err)
	assert.Greater(t, sysInfo.TotalMemoryMB, uint64(0))
	assert.Greater(t, sysInfo.CPUCount, 0)

	// Test multiple collections
	for i := 0; i < 5; i++ {
		err := monitor.CollectMetrics()
		assert.NoError(t, err)
		time.Sleep(100 * time.Millisecond)
	}

	// Verify metrics were collected
	cpuUsage := monitor.GetCPUUsage()
	assert.GreaterOrEqual(t, cpuUsage, 0.0)

	memUsage := monitor.GetMemoryUsage()
	assert.Greater(t, memUsage, uint64(0))
}

// TestStabilityMetrics tests the metrics collection and reporting
func TestStabilityMetrics(t *testing.T) {
	metrics := NewStabilityMetrics()
	require.NotNil(t, metrics)

	// Add some test snapshots
	for i := 0; i < 10; i++ {
		snapshot := MetricsSnapshot{
			Timestamp:   time.Now().Add(time.Duration(i) * time.Second),
			MemoryUsage: int64(1000000 + i*10000), // Simulating slight increase
			CPUUsage:    float64(10 + i),
			Goroutines:  100 + i,
			OpenFiles:   50 + i,
			Connections: 20 + i,
			Uptime:      time.Duration(i) * time.Second,
		}
		metrics.AddSnapshot(snapshot)
	}

	// Test snapshot retrieval
	snapshots := metrics.GetSnapshots()
	assert.Len(t, snapshots, 10)

	// Generate report
	report := metrics.GenerateReport()
	require.NotNil(t, report)
	assert.True(t, report.Success)
	assert.Greater(t, report.PeakMemoryMB, int64(0))
	assert.Greater(t, report.AverageCPU, 0.0)

	// Test metrics summary
	assert.NotEmpty(t, report.MetricsSummary.Memory.Average)
	assert.NotEmpty(t, report.MetricsSummary.CPU.Average)
	assert.Equal(t, "increasing", report.MetricsSummary.Memory.Trend)
}

// BenchmarkStabilityFramework benchmarks the framework performance
func BenchmarkStabilityFramework(b *testing.B) {
	config := DefaultConfig()
	config.Duration = 1 * time.Second
	config.MonitorInterval = 100 * time.Millisecond
	config.OutputDir = b.TempDir()

	for i := 0; i < b.N; i++ {
		test, err := NewStabilityTest(config)
		require.NoError(b, err)

		logger := logrus.New()
		logger.SetLevel(logrus.ErrorLevel) // Reduce logging for benchmark

		memValidator := NewMemoryLeakValidator(config, logger)
		test.AddValidator(memValidator)

		// Run very short test
		go func() {
			_ = test.Start()
		}()

		time.Sleep(config.Duration + 100*time.Millisecond)
		test.Stop()
	}
}

// BenchmarkSystemMonitor benchmarks the system monitor
func BenchmarkSystemMonitor(b *testing.B) {
	config := DefaultConfig()
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	monitor, err := NewSystemMonitor(config, logger)
	require.NoError(b, err)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = monitor.CollectMetrics()
	}
}