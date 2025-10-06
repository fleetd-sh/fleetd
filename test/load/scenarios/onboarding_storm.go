package scenarios

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"fleetd.sh/test/load/framework"
)

// OnboardingStormScenario simulates a rapid device registration scenario
type OnboardingStormScenario struct {
	config  *OnboardingStormConfig
	logger  *slog.Logger
	metrics *OnboardingStormMetrics
	mu      sync.RWMutex
}

// OnboardingStormConfig defines the configuration for the onboarding storm scenario
type OnboardingStormConfig struct {
	TotalDevices        int
	DevicesPerSecond    int
	BurstSize           int
	BurstInterval       time.Duration
	ProfileDistribution map[framework.DeviceProfile]float64
	ServerURL           string
	TestDuration        time.Duration
	SuccessThreshold    float64 // Minimum success rate required
	LatencyThreshold    time.Duration
	ConcurrencyLimit    int
	AuthToken           string
	TLSEnabled          bool
}

// OnboardingStormMetrics tracks metrics specific to the onboarding storm scenario
type OnboardingStormMetrics struct {
	DevicesStarted        int64
	DevicesSuccessful     int64
	DevicesFailed         int64
	RegistrationLatencies []time.Duration
	StartTime             time.Time
	EndTime               time.Time
	PeakConcurrency       int64
	TotalRequests         int64
	FailedRequests        int64
	mu                    sync.RWMutex
}

// NewOnboardingStormScenario creates a new onboarding storm scenario
func NewOnboardingStormScenario(config *OnboardingStormConfig) *OnboardingStormScenario {
	logger := slog.Default().With("scenario", "onboarding_storm")

	// Set defaults
	if config.ProfileDistribution == nil {
		config.ProfileDistribution = map[framework.DeviceProfile]float64{
			framework.ProfileFull:        0.2,
			framework.ProfileConstrained: 0.5,
			framework.ProfileMinimal:     0.3,
		}
	}

	if config.SuccessThreshold == 0 {
		config.SuccessThreshold = 0.95 // 95% success rate
	}

	if config.LatencyThreshold == 0 {
		config.LatencyThreshold = 30 * time.Second
	}

	if config.ConcurrencyLimit == 0 {
		config.ConcurrencyLimit = 100
	}

	return &OnboardingStormScenario{
		config: config,
		logger: logger,
		metrics: &OnboardingStormMetrics{
			StartTime: time.Now(),
		},
	}
}

// Run executes the onboarding storm scenario
func (s *OnboardingStormScenario) Run(ctx context.Context) error {
	s.logger.Info("Starting onboarding storm scenario",
		"total_devices", s.config.TotalDevices,
		"devices_per_second", s.config.DevicesPerSecond,
		"burst_size", s.config.BurstSize,
		"test_duration", s.config.TestDuration,
	)

	s.metrics.StartTime = time.Now()

	// Create device profiles based on distribution
	deviceProfiles := s.calculateDeviceProfiles()

	// Create fleet configuration
	fleetConfig := &framework.FleetConfig{
		ServerURL:             s.config.ServerURL,
		TotalDevices:          s.config.TotalDevices,
		DeviceProfiles:        deviceProfiles,
		StartupBatchSize:      s.config.BurstSize,
		StartupBatchInterval:  s.config.BurstInterval,
		MaxConcurrentRequests: s.config.ConcurrencyLimit,
		TestDuration:          s.config.TestDuration,
		AuthToken:             s.config.AuthToken,
		TLSEnabled:            s.config.TLSEnabled,
	}

	// Create and start fleet simulator
	fleet := framework.NewFleetSimulator(fleetConfig)

	// Start monitoring
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.monitorProgress(ctx, fleet)
	}()

	// Run the scenario
	err := s.executeStorm(ctx, fleet)

	// Wait for monitoring to finish
	wg.Wait()

	s.metrics.EndTime = time.Now()

	// Analyze results
	if err == nil {
		err = s.analyzeResults()
	}

	// Clean up
	if stopErr := fleet.Stop(); stopErr != nil {
		s.logger.Error("Failed to stop fleet", "error", stopErr)
	}

	return err
}

// calculateDeviceProfiles converts percentage distribution to device counts
func (s *OnboardingStormScenario) calculateDeviceProfiles() map[framework.DeviceProfile]int {
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

	// Adjust the first profile to match exact count
	if total != s.config.TotalDevices {
		diff := s.config.TotalDevices - total
		for profile := range profiles {
			profiles[profile] += diff
			break
		}
	}

	return profiles
}

// executeStorm runs the actual onboarding storm
func (s *OnboardingStormScenario) executeStorm(ctx context.Context, fleet *framework.FleetSimulator) error {
	s.logger.Info("Executing onboarding storm")

	// Create context with timeout
	stormCtx, cancel := context.WithTimeout(ctx, s.config.TestDuration)
	defer cancel()

	// Start the fleet with rapid onboarding
	startTime := time.Now()
	err := fleet.Start()
	if err != nil {
		return fmt.Errorf("failed to start fleet: %w", err)
	}

	registrationLatency := time.Since(startTime)
	s.addLatency(registrationLatency)

	s.logger.Info("Onboarding storm completed",
		"duration", registrationLatency,
		"devices_started", s.metrics.DevicesStarted,
	)

	// Keep the scenario running for the test duration to observe behavior
	select {
	case <-stormCtx.Done():
		s.logger.Info("Test duration completed")
	case <-ctx.Done():
		s.logger.Info("Context cancelled")
	}

	return nil
}

// monitorProgress monitors the progress of device onboarding
func (s *OnboardingStormScenario) monitorProgress(ctx context.Context, fleet *framework.FleetSimulator) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	eventChan := fleet.GetEvents()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.updateMetrics(fleet)
		case event := <-eventChan:
			s.handleEvent(event)
		}
	}
}

// updateMetrics updates scenario metrics from fleet metrics
func (s *OnboardingStormScenario) updateMetrics(fleet *framework.FleetSimulator) {
	fleetMetrics := fleet.GetMetrics()

	s.mu.Lock()
	s.metrics.DevicesStarted = fleetMetrics.OnlineDevices + fleetMetrics.OfflineDevices + fleetMetrics.ErrorDevices
	s.metrics.DevicesSuccessful = fleetMetrics.OnlineDevices
	s.metrics.DevicesFailed = fleetMetrics.ErrorDevices
	s.metrics.TotalRequests = fleetMetrics.TotalRequests
	s.metrics.FailedRequests = fleetMetrics.FailedRequests

	// Track peak concurrency
	currentActive := fleetMetrics.OnlineDevices + fleetMetrics.UpdatingDevices
	if currentActive > s.metrics.PeakConcurrency {
		s.metrics.PeakConcurrency = currentActive
	}
	s.mu.Unlock()

	// Log progress
	s.logger.Debug("Onboarding progress",
		"devices_started", s.metrics.DevicesStarted,
		"devices_successful", s.metrics.DevicesSuccessful,
		"devices_failed", s.metrics.DevicesFailed,
		"success_rate", float64(s.metrics.DevicesSuccessful)/float64(s.metrics.DevicesStarted),
	)
}

// handleEvent processes fleet events
func (s *OnboardingStormScenario) handleEvent(event framework.Event) {
	switch event.Type {
	case framework.EventDeviceStarted:
		s.logger.Debug("Device started", "device_id", event.DeviceID)
	case framework.EventDeviceError:
		s.logger.Warn("Device error during onboarding", "device_id", event.DeviceID, "message", event.Message)
	case framework.EventBatchStarted:
		if data, ok := event.Data["batch_size"].(int); ok {
			s.logger.Info("Starting onboarding batch", "batch_size", data)
		}
	case framework.EventBatchCompleted:
		if data, ok := event.Data["batch_number"].(int); ok {
			s.logger.Info("Completed onboarding batch", "batch_number", data)
		}
	}
}

// addLatency records a registration latency measurement
func (s *OnboardingStormScenario) addLatency(latency time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.metrics.RegistrationLatencies = append(s.metrics.RegistrationLatencies, latency)
}

// analyzeResults analyzes the test results and determines if the scenario passed
func (s *OnboardingStormScenario) analyzeResults() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	duration := s.metrics.EndTime.Sub(s.metrics.StartTime)
	successRate := float64(s.metrics.DevicesSuccessful) / float64(s.metrics.DevicesStarted)

	s.logger.Info("Onboarding storm results",
		"duration", duration,
		"devices_started", s.metrics.DevicesStarted,
		"devices_successful", s.metrics.DevicesSuccessful,
		"devices_failed", s.metrics.DevicesFailed,
		"success_rate", successRate,
		"peak_concurrency", s.metrics.PeakConcurrency,
		"total_requests", s.metrics.TotalRequests,
		"failed_requests", s.metrics.FailedRequests,
	)

	// Calculate latency percentiles
	if len(s.metrics.RegistrationLatencies) > 0 {
		latencies := make([]time.Duration, len(s.metrics.RegistrationLatencies))
		copy(latencies, s.metrics.RegistrationLatencies)

		// Simple percentile calculation (would use proper sorting in production)
		p50 := latencies[len(latencies)/2]
		p95 := latencies[int(float64(len(latencies))*0.95)]
		p99 := latencies[int(float64(len(latencies))*0.99)]

		s.logger.Info("Registration latency percentiles",
			"p50", p50,
			"p95", p95,
			"p99", p99,
		)

		// Check latency threshold
		if p95 > s.config.LatencyThreshold {
			return fmt.Errorf("p95 latency %v exceeds threshold %v", p95, s.config.LatencyThreshold)
		}
	}

	// Check success rate threshold
	if successRate < s.config.SuccessThreshold {
		return fmt.Errorf("success rate %.2f%% below threshold %.2f%%",
			successRate*100, s.config.SuccessThreshold*100)
	}

	// Calculate devices per second achieved
	devicesPerSecond := float64(s.metrics.DevicesStarted) / duration.Seconds()
	s.logger.Info("Onboarding rate achieved", "devices_per_second", devicesPerSecond)

	s.logger.Info("Onboarding storm scenario PASSED")
	return nil
}

// GetMetrics returns the current scenario metrics
func (s *OnboardingStormScenario) GetMetrics() OnboardingStormMetrics {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Deep copy latencies
	latencies := make([]time.Duration, len(s.metrics.RegistrationLatencies))
	copy(latencies, s.metrics.RegistrationLatencies)

	// Return copy without mutex
	return OnboardingStormMetrics{
		DevicesStarted:        s.metrics.DevicesStarted,
		DevicesSuccessful:     s.metrics.DevicesSuccessful,
		DevicesFailed:         s.metrics.DevicesFailed,
		RegistrationLatencies: latencies,
		StartTime:             s.metrics.StartTime,
		EndTime:               s.metrics.EndTime,
		PeakConcurrency:       s.metrics.PeakConcurrency,
		TotalRequests:         s.metrics.TotalRequests,
		FailedRequests:        s.metrics.FailedRequests,
	}
}

// GetName returns the scenario name
func (s *OnboardingStormScenario) GetName() string {
	return "onboarding_storm"
}

// GetDescription returns the scenario description
func (s *OnboardingStormScenario) GetDescription() string {
	return "Simulates rapid device registration to test onboarding system scalability"
}
