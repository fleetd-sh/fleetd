package scenarios

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"fleetd.sh/test/load/framework"
)

// UpdateCampaignScenario simulates a fleet-wide update deployment
type UpdateCampaignScenario struct {
	config  *UpdateCampaignConfig
	logger  *slog.Logger
	metrics *UpdateCampaignMetrics
	mu      sync.RWMutex
}

// UpdateCampaignConfig defines the configuration for the update campaign scenario
type UpdateCampaignConfig struct {
	TotalDevices         int
	ProfileDistribution  map[framework.DeviceProfile]float64
	ServerURL            string
	TestDuration         time.Duration
	UpdateBatchSize      int           // Number of devices to update in parallel
	UpdateBatchInterval  time.Duration // Delay between update batches
	UpdateSuccessRate    float64       // Expected update success rate
	UpdateDuration       time.Duration // How long each update takes
	RollbackThreshold    float64       // Failure rate that triggers rollback
	CanaryPercentage     float64       // Percentage of devices for canary deployment
	AuthToken            string
	TLSEnabled           bool
}

// UpdateCampaignMetrics tracks metrics for the update campaign scenario
type UpdateCampaignMetrics struct {
	StartTime               time.Time
	EndTime                 time.Time
	TotalDevices            int64
	DevicesUpdating         int64
	DevicesUpdated          int64
	DevicesFailed           int64
	DevicesRolledBack       int64
	UpdatesInitiated        int64
	UpdatesCompleted        int64
	UpdateFailures          int64
	CanarySuccessRate       float64
	OverallSuccessRate      float64
	AverageUpdateDuration   time.Duration
	UpdateDurations         []time.Duration
	BatchMetrics            []BatchMetrics
	RollbackTriggered       bool
	CampaignPhase           CampaignPhase
	mu                      sync.RWMutex
}

// BatchMetrics tracks metrics for each update batch
type BatchMetrics struct {
	BatchNumber   int
	StartTime     time.Time
	EndTime       time.Time
	DeviceCount   int
	SuccessCount  int
	FailureCount  int
	SuccessRate   float64
	Duration      time.Duration
}

// CampaignPhase represents the current phase of the update campaign
type CampaignPhase string

const (
	PhaseCanary     CampaignPhase = "canary"
	PhaseRollout    CampaignPhase = "rollout"
	PhaseCompleted  CampaignPhase = "completed"
	PhaseRollback   CampaignPhase = "rollback"
	PhaseFailed     CampaignPhase = "failed"
)

// NewUpdateCampaignScenario creates a new update campaign scenario
func NewUpdateCampaignScenario(config *UpdateCampaignConfig) *UpdateCampaignScenario {
	logger := slog.Default().With("scenario", "update_campaign")

	// Set defaults
	if config.ProfileDistribution == nil {
		config.ProfileDistribution = map[framework.DeviceProfile]float64{
			framework.ProfileFull:        0.3,
			framework.ProfileConstrained: 0.5,
			framework.ProfileMinimal:     0.2,
		}
	}

	if config.UpdateBatchSize == 0 {
		config.UpdateBatchSize = 50
	}

	if config.UpdateBatchInterval == 0 {
		config.UpdateBatchInterval = 2 * time.Minute
	}

	if config.UpdateSuccessRate == 0 {
		config.UpdateSuccessRate = 0.95 // 95% success rate
	}

	if config.UpdateDuration == 0 {
		config.UpdateDuration = 5 * time.Minute
	}

	if config.RollbackThreshold == 0 {
		config.RollbackThreshold = 0.1 // 10% failure rate triggers rollback
	}

	if config.CanaryPercentage == 0 {
		config.CanaryPercentage = 0.05 // 5% canary deployment
	}

	return &UpdateCampaignScenario{
		config: config,
		logger: logger,
		metrics: &UpdateCampaignMetrics{
			StartTime:     time.Now(),
			CampaignPhase: PhaseCanary,
		},
	}
}

// Run executes the update campaign scenario
func (s *UpdateCampaignScenario) Run(ctx context.Context) error {
	s.logger.Info("Starting update campaign scenario",
		"total_devices", s.config.TotalDevices,
		"batch_size", s.config.UpdateBatchSize,
		"batch_interval", s.config.UpdateBatchInterval,
		"canary_percentage", s.config.CanaryPercentage,
		"expected_success_rate", s.config.UpdateSuccessRate,
	)

	s.metrics.StartTime = time.Now()

	// Calculate device profiles
	deviceProfiles := s.calculateDeviceProfiles()

	// Create fleet configuration
	fleetConfig := &framework.FleetConfig{
		ServerURL:             s.config.ServerURL,
		TotalDevices:          s.config.TotalDevices,
		DeviceProfiles:        deviceProfiles,
		StartupBatchSize:      100,
		StartupBatchInterval:  time.Second,
		MaxConcurrentRequests: 500,
		TestDuration:          s.config.TestDuration,
		AuthToken:             s.config.AuthToken,
		TLSEnabled:            s.config.TLSEnabled,
	}

	// Create and start fleet simulator
	fleet := framework.NewFleetSimulator(fleetConfig)

	// Start monitoring
	var wg sync.WaitGroup
	monitorCtx, cancelMonitor := context.WithCancel(ctx)

	wg.Add(1)
	go func() {
		defer wg.Done()
		s.monitorCampaign(monitorCtx, fleet)
	}()

	// Start the fleet
	err := fleet.Start()
	if err != nil {
		cancelMonitor()
		wg.Wait()
		return fmt.Errorf("failed to start fleet: %w", err)
	}

	// Wait for devices to come online
	s.logger.Info("Waiting for devices to come online")
	time.Sleep(30 * time.Second)

	// Execute the update campaign
	campaignErr := s.executeCampaign(ctx, fleet)

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

	if campaignErr != nil {
		return campaignErr
	}

	return analysisErr
}

// calculateDeviceProfiles converts percentage distribution to device counts
func (s *UpdateCampaignScenario) calculateDeviceProfiles() map[framework.DeviceProfile]int {
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

// executeCampaign runs the update campaign with canary and rollout phases
func (s *UpdateCampaignScenario) executeCampaign(ctx context.Context, fleet *framework.FleetSimulator) error {
	devices := fleet.ListDevices()
	s.metrics.TotalDevices = int64(len(devices))

	// Phase 1: Canary deployment
	canaryCount := int(float64(len(devices)) * s.config.CanaryPercentage)
	if canaryCount == 0 {
		canaryCount = 1
	}

	s.logger.Info("Starting canary deployment", "devices", canaryCount)
	s.metrics.CampaignPhase = PhaseCanary

	canaryDevices := devices[:canaryCount]
	canaryErr := s.executeCanaryPhase(ctx, canaryDevices)
	if canaryErr != nil {
		s.metrics.CampaignPhase = PhaseFailed
		return fmt.Errorf("canary phase failed: %w", canaryErr)
	}

	// Check canary success rate
	if s.metrics.CanarySuccessRate < s.config.UpdateSuccessRate {
		s.logger.Error("Canary deployment failed success rate threshold",
			"success_rate", s.metrics.CanarySuccessRate,
			"threshold", s.config.UpdateSuccessRate)
		s.metrics.CampaignPhase = PhaseFailed
		return fmt.Errorf("canary success rate %.2f%% below threshold %.2f%%",
			s.metrics.CanarySuccessRate*100, s.config.UpdateSuccessRate*100)
	}

	s.logger.Info("Canary deployment successful", "success_rate", s.metrics.CanarySuccessRate)

	// Phase 2: Full rollout
	remainingDevices := devices[canaryCount:]
	if len(remainingDevices) > 0 {
		s.logger.Info("Starting full rollout", "devices", len(remainingDevices))
		s.metrics.CampaignPhase = PhaseRollout

		rolloutErr := s.executeRolloutPhase(ctx, remainingDevices)
		if rolloutErr != nil {
			if s.metrics.RollbackTriggered {
				s.metrics.CampaignPhase = PhaseRollback
				s.logger.Info("Executing rollback due to high failure rate")
				s.executeRollback(ctx, devices)
			} else {
				s.metrics.CampaignPhase = PhaseFailed
			}
			return rolloutErr
		}
	}

	s.metrics.CampaignPhase = PhaseCompleted
	s.logger.Info("Update campaign completed successfully")
	return nil
}

// executeCanaryPhase runs the canary deployment
func (s *UpdateCampaignScenario) executeCanaryPhase(ctx context.Context, devices []*framework.VirtualDevice) error {
	s.logger.Info("Executing canary phase", "devices", len(devices))

	var successCount, failureCount int64

	// Update devices in the canary phase
	var wg sync.WaitGroup
	for _, device := range devices {
		wg.Add(1)
		go func(d *framework.VirtualDevice) {
			defer wg.Done()

			updateStart := time.Now()
			success := s.simulateDeviceUpdate(ctx, d)
			updateDuration := time.Since(updateStart)

			s.mu.Lock()
			s.metrics.UpdateDurations = append(s.metrics.UpdateDurations, updateDuration)
			s.metrics.UpdatesInitiated++

			if success {
				successCount++
				s.metrics.UpdatesCompleted++
				s.metrics.DevicesUpdated++
			} else {
				failureCount++
				s.metrics.UpdateFailures++
				s.metrics.DevicesFailed++
			}
			s.mu.Unlock()
		}(device)
	}

	wg.Wait()

	// Calculate canary success rate
	total := successCount + failureCount
	if total > 0 {
		s.metrics.CanarySuccessRate = float64(successCount) / float64(total)
	}

	s.logger.Info("Canary phase completed",
		"success_count", successCount,
		"failure_count", failureCount,
		"success_rate", s.metrics.CanarySuccessRate,
	)

	return nil
}

// executeRolloutPhase runs the full rollout in batches
func (s *UpdateCampaignScenario) executeRolloutPhase(ctx context.Context, devices []*framework.VirtualDevice) error {
	s.logger.Info("Executing rollout phase", "devices", len(devices))

	batchSize := s.config.UpdateBatchSize
	batchNumber := 1

	for i := 0; i < len(devices); i += batchSize {
		end := i + batchSize
		if end > len(devices) {
			end = len(devices)
		}

		batch := devices[i:end]

		// Check for context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		batchErr := s.executeBatch(ctx, batch, batchNumber)
		if batchErr != nil {
			return batchErr
		}

		// Check if rollback should be triggered
		if s.shouldTriggerRollback() {
			s.metrics.RollbackTriggered = true
			return fmt.Errorf("rollback triggered due to high failure rate")
		}

		batchNumber++

		// Wait before next batch (except for last batch)
		if end < len(devices) {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(s.config.UpdateBatchInterval):
			}
		}
	}

	return nil
}

// executeBatch updates a batch of devices
func (s *UpdateCampaignScenario) executeBatch(ctx context.Context, devices []*framework.VirtualDevice, batchNumber int) error {
	batchStart := time.Now()

	s.logger.Info("Executing update batch",
		"batch_number", batchNumber,
		"device_count", len(devices),
	)

	var successCount, failureCount int64
	var wg sync.WaitGroup

	// Update devices in parallel within the batch
	for _, device := range devices {
		wg.Add(1)
		go func(d *framework.VirtualDevice) {
			defer wg.Done()

			updateStart := time.Now()
			success := s.simulateDeviceUpdate(ctx, d)
			updateDuration := time.Since(updateStart)

			s.mu.Lock()
			s.metrics.UpdateDurations = append(s.metrics.UpdateDurations, updateDuration)
			s.metrics.UpdatesInitiated++

			if success {
				successCount++
				s.metrics.UpdatesCompleted++
				s.metrics.DevicesUpdated++
			} else {
				failureCount++
				s.metrics.UpdateFailures++
				s.metrics.DevicesFailed++
			}
			s.mu.Unlock()
		}(device)
	}

	wg.Wait()

	batchEnd := time.Now()
	batchDuration := batchEnd.Sub(batchStart)

	// Record batch metrics
	batchMetrics := BatchMetrics{
		BatchNumber:  batchNumber,
		StartTime:    batchStart,
		EndTime:      batchEnd,
		DeviceCount:  len(devices),
		SuccessCount: int(successCount),
		FailureCount: int(failureCount),
		Duration:     batchDuration,
	}

	if batchMetrics.DeviceCount > 0 {
		batchMetrics.SuccessRate = float64(batchMetrics.SuccessCount) / float64(batchMetrics.DeviceCount)
	}

	s.mu.Lock()
	s.metrics.BatchMetrics = append(s.metrics.BatchMetrics, batchMetrics)
	s.mu.Unlock()

	s.logger.Info("Batch completed",
		"batch_number", batchNumber,
		"success_count", successCount,
		"failure_count", failureCount,
		"success_rate", batchMetrics.SuccessRate,
		"duration", batchDuration,
	)

	return nil
}

// simulateDeviceUpdate simulates updating a single device
func (s *UpdateCampaignScenario) simulateDeviceUpdate(ctx context.Context, device *framework.VirtualDevice) bool {
	// Simulate the update process
	state := device.GetState()

	// Different profiles have different update characteristics
	var successProbability float64
	var updateDuration time.Duration

	switch device.config.Profile {
	case framework.ProfileFull:
		successProbability = s.config.UpdateSuccessRate + 0.02 // Slightly higher success rate
		updateDuration = s.config.UpdateDuration
	case framework.ProfileConstrained:
		successProbability = s.config.UpdateSuccessRate
		updateDuration = time.Duration(float64(s.config.UpdateDuration) * 1.2) // 20% longer
	case framework.ProfileMinimal:
		successProbability = s.config.UpdateSuccessRate - 0.05 // Lower success rate
		updateDuration = time.Duration(float64(s.config.UpdateDuration) * 1.5) // 50% longer
	}

	// Add some randomness to the duration
	jitter := time.Duration(s.randomFloat() * 0.3 * float64(updateDuration))
	updateDuration += jitter

	// Simulate update time
	select {
	case <-ctx.Done():
		return false
	case <-time.After(updateDuration):
	}

	// Determine if update succeeds
	success := s.randomFloat() < successProbability

	if success {
		s.logger.Debug("Device update successful", "device_id", state)
	} else {
		s.logger.Debug("Device update failed", "device_id", state)
	}

	return success
}

// shouldTriggerRollback determines if a rollback should be triggered
func (s *UpdateCampaignScenario) shouldTriggerRollback() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.metrics.UpdatesInitiated == 0 {
		return false
	}

	currentFailureRate := float64(s.metrics.UpdateFailures) / float64(s.metrics.UpdatesInitiated)
	return currentFailureRate > s.config.RollbackThreshold
}

// executeRollback simulates rolling back the update
func (s *UpdateCampaignScenario) executeRollback(ctx context.Context, devices []*framework.VirtualDevice) {
	s.logger.Info("Executing rollback", "devices", len(devices))

	var wg sync.WaitGroup
	for _, device := range devices {
		wg.Add(1)
		go func(d *framework.VirtualDevice) {
			defer wg.Done()

			// Simulate rollback time (usually faster than update)
			rollbackDuration := s.config.UpdateDuration / 3
			select {
			case <-ctx.Done():
				return
			case <-time.After(rollbackDuration):
			}

			s.mu.Lock()
			s.metrics.DevicesRolledBack++
			s.mu.Unlock()

			s.logger.Debug("Device rolled back", "device_id", d.config.DeviceID)
		}(device)
	}

	wg.Wait()
	s.logger.Info("Rollback completed", "devices_rolled_back", s.metrics.DevicesRolledBack)
}

// monitorCampaign monitors the campaign progress
func (s *UpdateCampaignScenario) monitorCampaign(ctx context.Context, fleet *framework.FleetSimulator) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.logProgress()
		}
	}
}

// logProgress logs the current campaign progress
func (s *UpdateCampaignScenario) logProgress() {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var currentSuccessRate float64
	if s.metrics.UpdatesInitiated > 0 {
		currentSuccessRate = float64(s.metrics.UpdatesCompleted) / float64(s.metrics.UpdatesInitiated)
	}

	s.logger.Info("Campaign progress",
		"phase", s.metrics.CampaignPhase,
		"updates_initiated", s.metrics.UpdatesInitiated,
		"updates_completed", s.metrics.UpdatesCompleted,
		"update_failures", s.metrics.UpdateFailures,
		"current_success_rate", currentSuccessRate,
		"devices_updated", s.metrics.DevicesUpdated,
		"devices_failed", s.metrics.DevicesFailed,
	)
}

// analyzeResults analyzes the campaign results
func (s *UpdateCampaignScenario) analyzeResults() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	duration := s.metrics.EndTime.Sub(s.metrics.StartTime)

	// Calculate overall success rate
	if s.metrics.UpdatesInitiated > 0 {
		s.metrics.OverallSuccessRate = float64(s.metrics.UpdatesCompleted) / float64(s.metrics.UpdatesInitiated)
	}

	// Calculate average update duration
	if len(s.metrics.UpdateDurations) > 0 {
		var total time.Duration
		for _, d := range s.metrics.UpdateDurations {
			total += d
		}
		s.metrics.AverageUpdateDuration = total / time.Duration(len(s.metrics.UpdateDurations))
	}

	s.logger.Info("Update campaign results",
		"duration", duration,
		"campaign_phase", s.metrics.CampaignPhase,
		"total_devices", s.metrics.TotalDevices,
		"devices_updated", s.metrics.DevicesUpdated,
		"devices_failed", s.metrics.DevicesFailed,
		"devices_rolled_back", s.metrics.DevicesRolledBack,
		"overall_success_rate", s.metrics.OverallSuccessRate,
		"average_update_duration", s.metrics.AverageUpdateDuration,
		"rollback_triggered", s.metrics.RollbackTriggered,
	)

	// Analyze batch performance
	if len(s.metrics.BatchMetrics) > 0 {
		s.analyzeBatchPerformance()
	}

	// Determine if campaign was successful
	switch s.metrics.CampaignPhase {
	case PhaseCompleted:
		if s.metrics.OverallSuccessRate >= s.config.UpdateSuccessRate {
			s.logger.Info("Update campaign scenario PASSED")
			return nil
		} else {
			return fmt.Errorf("overall success rate %.2f%% below threshold %.2f%%",
				s.metrics.OverallSuccessRate*100, s.config.UpdateSuccessRate*100)
		}
	case PhaseRollback:
		s.logger.Info("Update campaign triggered rollback - scenario outcome depends on rollback success")
		return nil // Rollback scenarios can be considered successful if rollback works
	case PhaseFailed:
		return fmt.Errorf("update campaign failed")
	default:
		return fmt.Errorf("update campaign did not complete successfully")
	}
}

// analyzeBatchPerformance analyzes the performance of individual batches
func (s *UpdateCampaignScenario) analyzeBatchPerformance() {
	var totalSuccessRate, totalDuration float64
	var minSuccessRate, maxSuccessRate float64 = 1.0, 0.0

	for i, batch := range s.metrics.BatchMetrics {
		totalSuccessRate += batch.SuccessRate
		totalDuration += batch.Duration.Seconds()

		if batch.SuccessRate < minSuccessRate {
			minSuccessRate = batch.SuccessRate
		}
		if batch.SuccessRate > maxSuccessRate {
			maxSuccessRate = batch.SuccessRate
		}

		s.logger.Debug("Batch analysis",
			"batch_number", batch.BatchNumber,
			"success_rate", batch.SuccessRate,
			"duration", batch.Duration,
		)

		// First and last few batches for detailed logging
		if i < 3 || i >= len(s.metrics.BatchMetrics)-3 {
			s.logger.Info("Batch details",
				"batch_number", batch.BatchNumber,
				"device_count", batch.DeviceCount,
				"success_count", batch.SuccessCount,
				"failure_count", batch.FailureCount,
				"success_rate", batch.SuccessRate,
				"duration", batch.Duration,
			)
		}
	}

	avgSuccessRate := totalSuccessRate / float64(len(s.metrics.BatchMetrics))
	avgDuration := totalDuration / float64(len(s.metrics.BatchMetrics))

	s.logger.Info("Batch performance summary",
		"total_batches", len(s.metrics.BatchMetrics),
		"avg_success_rate", avgSuccessRate,
		"min_success_rate", minSuccessRate,
		"max_success_rate", maxSuccessRate,
		"avg_duration_seconds", avgDuration,
	)
}

// randomFloat returns a pseudo-random float between 0 and 1
func (s *UpdateCampaignScenario) randomFloat() float64 {
	return float64(time.Now().UnixNano()%1000) / 1000.0
}

// GetMetrics returns the current scenario metrics
func (s *UpdateCampaignScenario) GetMetrics() UpdateCampaignMetrics {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Deep copy
	metrics := *s.metrics

	// Copy slices
	if s.metrics.UpdateDurations != nil {
		metrics.UpdateDurations = make([]time.Duration, len(s.metrics.UpdateDurations))
		copy(metrics.UpdateDurations, s.metrics.UpdateDurations)
	}

	if s.metrics.BatchMetrics != nil {
		metrics.BatchMetrics = make([]BatchMetrics, len(s.metrics.BatchMetrics))
		copy(metrics.BatchMetrics, s.metrics.BatchMetrics)
	}

	return metrics
}

// GetName returns the scenario name
func (s *UpdateCampaignScenario) GetName() string {
	return "update_campaign"
}

// GetDescription returns the scenario description
func (s *UpdateCampaignScenario) GetDescription() string {
	return "Simulates a fleet-wide update deployment with canary testing and rollback capabilities"
}