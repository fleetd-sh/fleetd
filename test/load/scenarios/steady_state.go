package scenarios

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"fleetd.sh/test/load/framework"
)

// SteadyStateScenario simulates normal fleet operations over time
type SteadyStateScenario struct {
	config  *SteadyStateConfig
	logger  *slog.Logger
	metrics *SteadyStateMetrics
	mu      sync.RWMutex
}

// SteadyStateConfig defines the configuration for the steady state scenario
type SteadyStateConfig struct {
	TotalDevices          int
	ProfileDistribution   map[framework.DeviceProfile]float64
	ServerURL             string
	TestDuration          time.Duration
	WarmupDuration        time.Duration
	MetricsTargetRate     int64 // metrics per second target
	HeartbeatTargetRate   int64 // heartbeats per second target
	MaxErrorRate          float64
	MaxLatency            time.Duration
	DeviceChurnRate       float64 // percentage of devices that go offline/online
	ChurnInterval         time.Duration
	AuthToken             string
	TLSEnabled            bool
}

// SteadyStateMetrics tracks metrics for the steady state scenario
type SteadyStateMetrics struct {
	StartTime              time.Time
	EndTime                time.Time
	DevicesOnline          int64
	DevicesOffline         int64
	TotalMetricsSent       int64
	TotalHeartbeatsSent    int64
	TotalErrors            int64
	MetricsRate            float64 // metrics per second
	HeartbeatRate          float64 // heartbeats per second
	ErrorRate              float64
	AverageLatency         time.Duration
	P95Latency             time.Duration
	P99Latency             time.Duration
	ResourceUtilization    ResourceUtilization
	LatencyMeasurements    []time.Duration
	ThroughputMeasurements []ThroughputMeasurement
	mu                     sync.RWMutex
}

// ResourceUtilization tracks system resource usage
type ResourceUtilization struct {
	CPUUsage    float64
	MemoryUsage float64
	NetworkIO   NetworkIO
}

// NetworkIO tracks network I/O metrics
type NetworkIO struct {
	BytesSent    uint64
	BytesRecv    uint64
	PacketsSent  uint64
	PacketsRecv  uint64
}

// ThroughputMeasurement represents a point-in-time throughput measurement
type ThroughputMeasurement struct {
	Timestamp       time.Time
	MetricsPerSec   float64
	HeartbeatsPerSec float64
	ErrorsPerSec    float64
}

// NewSteadyStateScenario creates a new steady state scenario
func NewSteadyStateScenario(config *SteadyStateConfig) *SteadyStateScenario {
	logger := slog.Default().With("scenario", "steady_state")

	// Set defaults
	if config.ProfileDistribution == nil {
		config.ProfileDistribution = map[framework.DeviceProfile]float64{
			framework.ProfileFull:        0.3,
			framework.ProfileConstrained: 0.5,
			framework.ProfileMinimal:     0.2,
		}
	}

	if config.WarmupDuration == 0 {
		config.WarmupDuration = 5 * time.Minute
	}

	if config.MaxErrorRate == 0 {
		config.MaxErrorRate = 0.05 // 5% max error rate
	}

	if config.MaxLatency == 0 {
		config.MaxLatency = 10 * time.Second
	}

	if config.DeviceChurnRate == 0 {
		config.DeviceChurnRate = 0.1 // 10% churn
	}

	if config.ChurnInterval == 0 {
		config.ChurnInterval = 10 * time.Minute
	}

	return &SteadyStateScenario{
		config: config,
		logger: logger,
		metrics: &SteadyStateMetrics{
			StartTime: time.Now(),
		},
	}
}

// Run executes the steady state scenario
func (s *SteadyStateScenario) Run(ctx context.Context) error {
	s.logger.Info("Starting steady state scenario",
		"total_devices", s.config.TotalDevices,
		"test_duration", s.config.TestDuration,
		"warmup_duration", s.config.WarmupDuration,
		"metrics_target_rate", s.config.MetricsTargetRate,
		"heartbeat_target_rate", s.config.HeartbeatTargetRate,
	)

	s.metrics.StartTime = time.Now()

	// Calculate device profiles
	deviceProfiles := s.calculateDeviceProfiles()

	// Create fleet configuration
	fleetConfig := &framework.FleetConfig{
		ServerURL:             s.config.ServerURL,
		TotalDevices:          s.config.TotalDevices,
		DeviceProfiles:        deviceProfiles,
		StartupBatchSize:      50, // Moderate batch size for steady state
		StartupBatchInterval:  2 * time.Second,
		MaxConcurrentRequests: 200,
		TestDuration:          s.config.TestDuration,
		AuthToken:             s.config.AuthToken,
		TLSEnabled:            s.config.TLSEnabled,
	}

	// Create and start fleet simulator
	fleet := framework.NewFleetSimulator(fleetConfig)

	// Start monitoring
	var wg sync.WaitGroup
	monitorCtx, cancelMonitor := context.WithCancel(ctx)

	wg.Add(3)
	go func() {
		defer wg.Done()
		s.monitorMetrics(monitorCtx, fleet)
	}()

	go func() {
		defer wg.Done()
		s.monitorThroughput(monitorCtx, fleet)
	}()

	go func() {
		defer wg.Done()
		s.simulateDeviceChurn(monitorCtx, fleet)
	}()

	// Start the fleet
	err := fleet.Start()
	if err != nil {
		cancelMonitor()
		wg.Wait()
		return fmt.Errorf("failed to start fleet: %w", err)
	}

	// Warmup period
	s.logger.Info("Starting warmup period", "duration", s.config.WarmupDuration)
	select {
	case <-time.After(s.config.WarmupDuration):
		s.logger.Info("Warmup period completed")
	case <-ctx.Done():
		cancelMonitor()
		wg.Wait()
		fleet.Stop()
		return ctx.Err()
	}

	// Reset metrics after warmup
	s.resetMetrics()

	// Run steady state test
	testCtx, cancelTest := context.WithTimeout(ctx, s.config.TestDuration)
	defer cancelTest()

	s.logger.Info("Starting steady state test period")

	select {
	case <-testCtx.Done():
		s.logger.Info("Steady state test completed")
	case <-ctx.Done():
		s.logger.Info("Context cancelled")
	}

	// Stop monitoring
	cancelMonitor()
	wg.Wait()

	s.metrics.EndTime = time.Now()

	// Analyze results
	analysisErr := s.analyzeResults()

	// Clean up
	if stopErr := fleet.Stop(); stopErr != nil {
		s.logger.Error("Failed to stop fleet", "error", stopErr)
	}

	return analysisErr
}

// calculateDeviceProfiles converts percentage distribution to device counts
func (s *SteadyStateScenario) calculateDeviceProfiles() map[framework.DeviceProfile]int {
	profiles := make(map[framework.DeviceProfile]int)

	for profile, percentage := range s.config.ProfileDistribution {
		count := int(float64(s.config.TotalDevices) * percentage)
		profiles[profile] = count
	}

	// Ensure we have the exact number of devices
	total := 0
	for _, count := range profiles {
		total += count
	}

	if total != s.config.TotalDevices {
		diff := s.config.TotalDevices - total
		for profile := range profiles {
			profiles[profile] += diff
			break
		}
	}

	return profiles
}

// monitorMetrics continuously monitors fleet metrics
func (s *SteadyStateScenario) monitorMetrics(ctx context.Context, fleet *framework.FleetSimulator) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	var lastMetricsSent, lastHeartbeatsSent, lastErrors int64
	var lastTime time.Time

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			fleetMetrics := fleet.GetMetrics()

			s.mu.Lock()
			s.metrics.DevicesOnline = fleetMetrics.OnlineDevices
			s.metrics.DevicesOffline = fleetMetrics.OfflineDevices
			s.metrics.TotalMetricsSent = fleetMetrics.TotalMetricsSent
			s.metrics.TotalHeartbeatsSent = fleetMetrics.TotalRequests
			s.metrics.TotalErrors = fleetMetrics.TotalErrors

			// Calculate rates
			if !lastTime.IsZero() {
				duration := now.Sub(lastTime).Seconds()
				s.metrics.MetricsRate = float64(fleetMetrics.TotalMetricsSent-lastMetricsSent) / duration
				s.metrics.HeartbeatRate = float64(fleetMetrics.TotalRequests-lastHeartbeatsSent) / duration

				if s.metrics.TotalMetricsSent > 0 {
					s.metrics.ErrorRate = float64(s.metrics.TotalErrors) / float64(s.metrics.TotalMetricsSent)
				}
			}

			lastMetricsSent = fleetMetrics.TotalMetricsSent
			lastHeartbeatsSent = fleetMetrics.TotalRequests
			lastErrors = fleetMetrics.TotalErrors
			lastTime = now
			s.mu.Unlock()

			// Log periodic status
			if now.Second()%30 == 0 {
				s.logger.Debug("Steady state metrics",
					"devices_online", s.metrics.DevicesOnline,
					"metrics_rate", s.metrics.MetricsRate,
					"heartbeat_rate", s.metrics.HeartbeatRate,
					"error_rate", s.metrics.ErrorRate,
				)
			}
		}
	}
}

// monitorThroughput tracks throughput over time
func (s *SteadyStateScenario) monitorThroughput(ctx context.Context, fleet *framework.FleetSimulator) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	var lastMetrics, lastHeartbeats, lastErrors int64
	var lastTime time.Time

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			fleetMetrics := fleet.GetMetrics()

			if !lastTime.IsZero() {
				duration := now.Sub(lastTime).Seconds()

				metricsPerSec := float64(fleetMetrics.TotalMetricsSent-lastMetrics) / duration
				heartbeatsPerSec := float64(fleetMetrics.TotalRequests-lastHeartbeats) / duration
				errorsPerSec := float64(fleetMetrics.TotalErrors-lastErrors) / duration

				measurement := ThroughputMeasurement{
					Timestamp:        now,
					MetricsPerSec:    metricsPerSec,
					HeartbeatsPerSec: heartbeatsPerSec,
					ErrorsPerSec:     errorsPerSec,
				}

				s.mu.Lock()
				s.metrics.ThroughputMeasurements = append(s.metrics.ThroughputMeasurements, measurement)
				s.mu.Unlock()
			}

			lastMetrics = fleetMetrics.TotalMetricsSent
			lastHeartbeats = fleetMetrics.TotalRequests
			lastErrors = fleetMetrics.TotalErrors
			lastTime = now
		}
	}
}

// simulateDeviceChurn periodically takes devices offline and brings them back
func (s *SteadyStateScenario) simulateDeviceChurn(ctx context.Context, fleet *framework.FleetSimulator) {
	ticker := time.NewTicker(s.config.ChurnInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.performDeviceChurn(fleet)
		}
	}
}

// performDeviceChurn simulates device churn by temporarily disconnecting devices
func (s *SteadyStateScenario) performDeviceChurn(fleet *framework.FleetSimulator) {
	devices := fleet.ListDevices()
	churnCount := int(float64(len(devices)) * s.config.DeviceChurnRate)

	if churnCount == 0 {
		return
	}

	s.logger.Info("Simulating device churn", "devices_affected", churnCount)

	// Take devices offline
	for i := 0; i < churnCount && i < len(devices); i++ {
		device := devices[i]
		go func(d *framework.VirtualDevice) {
			// Stop device temporarily
			if err := d.Stop(); err != nil {
				s.logger.Error("Failed to stop device for churn", "device_id", d.GetState(), "error", err)
				return
			}

			// Wait a random time between 30 seconds and 5 minutes
			downTime := time.Duration(30+120*s.randomFloat()) * time.Second
			time.Sleep(downTime)

			// Restart device
			if err := d.Start(); err != nil {
				s.logger.Error("Failed to restart device after churn", "error", err)
			}
		}(device)
	}
}

// resetMetrics resets metrics after warmup period
func (s *SteadyStateScenario) resetMetrics() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.metrics.StartTime = time.Now()
	s.metrics.TotalMetricsSent = 0
	s.metrics.TotalHeartbeatsSent = 0
	s.metrics.TotalErrors = 0
	s.metrics.LatencyMeasurements = nil
	s.metrics.ThroughputMeasurements = nil
}

// analyzeResults analyzes the test results
func (s *SteadyStateScenario) analyzeResults() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	duration := s.metrics.EndTime.Sub(s.metrics.StartTime)

	s.logger.Info("Steady state scenario results",
		"duration", duration,
		"devices_online", s.metrics.DevicesOnline,
		"total_metrics_sent", s.metrics.TotalMetricsSent,
		"total_heartbeats_sent", s.metrics.TotalHeartbeatsSent,
		"total_errors", s.metrics.TotalErrors,
		"average_metrics_rate", s.metrics.MetricsRate,
		"average_heartbeat_rate", s.metrics.HeartbeatRate,
		"error_rate", s.metrics.ErrorRate,
	)

	// Check targets and thresholds
	var errors []string

	// Check metrics rate target
	if s.config.MetricsTargetRate > 0 && s.metrics.MetricsRate < float64(s.config.MetricsTargetRate)*0.9 {
		errors = append(errors, fmt.Sprintf("metrics rate %.2f/s below target %d/s",
			s.metrics.MetricsRate, s.config.MetricsTargetRate))
	}

	// Check heartbeat rate target
	if s.config.HeartbeatTargetRate > 0 && s.metrics.HeartbeatRate < float64(s.config.HeartbeatTargetRate)*0.9 {
		errors = append(errors, fmt.Sprintf("heartbeat rate %.2f/s below target %d/s",
			s.metrics.HeartbeatRate, s.config.HeartbeatTargetRate))
	}

	// Check error rate threshold
	if s.metrics.ErrorRate > s.config.MaxErrorRate {
		errors = append(errors, fmt.Sprintf("error rate %.2f%% exceeds threshold %.2f%%",
			s.metrics.ErrorRate*100, s.config.MaxErrorRate*100))
	}

	// Analyze throughput stability
	if len(s.metrics.ThroughputMeasurements) > 0 {
		s.analyzeThroughputStability()
	}

	if len(errors) > 0 {
		s.logger.Error("Steady state scenario FAILED", "errors", errors)
		return fmt.Errorf("scenario failed: %v", errors)
	}

	s.logger.Info("Steady state scenario PASSED")
	return nil
}

// analyzeThroughputStability analyzes the stability of throughput over time
func (s *SteadyStateScenario) analyzeThroughputStability() {
	measurements := s.metrics.ThroughputMeasurements

	if len(measurements) < 2 {
		return
	}

	// Calculate coefficient of variation for metrics rate
	var sum, sumSquares float64
	for _, m := range measurements {
		sum += m.MetricsPerSec
		sumSquares += m.MetricsPerSec * m.MetricsPerSec
	}

	mean := sum / float64(len(measurements))
	variance := (sumSquares / float64(len(measurements))) - (mean * mean)
	stdDev := variance
	if variance > 0 {
		stdDev = variance * variance // simplified for this example
	}

	cv := stdDev / mean

	s.logger.Info("Throughput stability analysis",
		"mean_metrics_rate", mean,
		"coefficient_of_variation", cv,
		"measurements", len(measurements),
	)

	// A CV of less than 0.2 indicates good stability
	if cv > 0.3 {
		s.logger.Warn("High throughput variability detected", "cv", cv)
	}
}

// randomFloat returns a pseudo-random float between 0 and 1
func (s *SteadyStateScenario) randomFloat() float64 {
	// Simple pseudo-random for this example
	return float64(time.Now().UnixNano()%1000) / 1000.0
}

// GetMetrics returns the current scenario metrics
func (s *SteadyStateScenario) GetMetrics() SteadyStateMetrics {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Deep copy
	metrics := *s.metrics

	// Copy slices
	if s.metrics.LatencyMeasurements != nil {
		metrics.LatencyMeasurements = make([]time.Duration, len(s.metrics.LatencyMeasurements))
		copy(metrics.LatencyMeasurements, s.metrics.LatencyMeasurements)
	}

	if s.metrics.ThroughputMeasurements != nil {
		metrics.ThroughputMeasurements = make([]ThroughputMeasurement, len(s.metrics.ThroughputMeasurements))
		copy(metrics.ThroughputMeasurements, s.metrics.ThroughputMeasurements)
	}

	return metrics
}

// GetName returns the scenario name
func (s *SteadyStateScenario) GetName() string {
	return "steady_state"
}

// GetDescription returns the scenario description
func (s *SteadyStateScenario) GetDescription() string {
	return "Simulates normal fleet operations to test sustained performance and stability"
}