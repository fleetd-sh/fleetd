package scenarios

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"fleetd.sh/test/load/framework"
)

// NetworkResilienceScenario simulates network issues and device reconnections
type NetworkResilienceScenario struct {
	config  *NetworkResilienceConfig
	logger  *slog.Logger
	metrics *NetworkResilienceMetrics
	mu      sync.RWMutex
}

// NetworkResilienceConfig defines the configuration for the network resilience scenario
type NetworkResilienceConfig struct {
	TotalDevices         int
	ProfileDistribution  map[framework.DeviceProfile]float64
	ServerURL            string
	TestDuration         time.Duration
	NetworkEvents        []NetworkEvent
	RecoveryTargetTime   time.Duration // Time devices should reconnect within
	MaxReconnectAttempts int
	AuthToken            string
	TLSEnabled           bool
}

// NetworkEvent represents a network disruption event
type NetworkEvent struct {
	Type            NetworkEventType
	StartTime       time.Duration // Time from test start
	Duration        time.Duration
	AffectedPercent float64 // Percentage of devices affected
	Severity        NetworkSeverity
	Description     string
}

// NetworkEventType defines different types of network events
type NetworkEventType string

const (
	EventPartition      NetworkEventType = "partition"       // Complete network partition
	EventLatency        NetworkEventType = "latency"         // High latency
	EventPacketLoss     NetworkEventType = "packet_loss"     // Packet loss
	EventBandwidthLimit NetworkEventType = "bandwidth_limit" // Bandwidth limitation
	EventDNSFailure     NetworkEventType = "dns_failure"     // DNS resolution failure
	EventIntermittent   NetworkEventType = "intermittent"    // Intermittent connectivity
)

// NetworkSeverity defines the severity of network events
type NetworkSeverity string

const (
	SeverityLow      NetworkSeverity = "low"
	SeverityMedium   NetworkSeverity = "medium"
	SeverityHigh     NetworkSeverity = "high"
	SeverityCritical NetworkSeverity = "critical"
)

// NetworkResilienceMetrics tracks metrics for the network resilience scenario
type NetworkResilienceMetrics struct {
	StartTime               time.Time
	EndTime                 time.Time
	TotalDevices            int64
	DevicesAffected         int64
	DevicesRecovered        int64
	DevicesFailedToRecover  int64
	NetworkEventsExecuted   int64
	ReconnectionAttempts    int64
	SuccessfulReconnections int64
	FailedReconnections     int64
	AverageRecoveryTime     time.Duration
	P95RecoveryTime         time.Duration
	P99RecoveryTime         time.Duration
	RecoveryTimes           []time.Duration
	EventMetrics            []NetworkEventMetrics
	ReconnectionStorms      []ReconnectionStormMetrics
	MaxConcurrentReconnects int64
	NetworkHealthScore      float64
	mu                      sync.RWMutex
}

// NetworkEventMetrics tracks metrics for individual network events
type NetworkEventMetrics struct {
	Event                   NetworkEvent
	StartTime               time.Time
	EndTime                 time.Time
	DevicesAffected         int64
	DevicesRecoveredQuickly int64 // Recovered within target time
	DevicesRecoveredSlow    int64 // Recovered after target time
	DevicesFailedToRecover  int64
	AverageRecoveryTime     time.Duration
	ReconnectionStormPeak   int64 // Peak concurrent reconnections
}

// ReconnectionStormMetrics tracks reconnection storm characteristics
type ReconnectionStormMetrics struct {
	StartTime              time.Time
	Duration               time.Duration
	PeakConcurrentConnects int64
	TotalReconnectAttempts int64
	SuccessfulReconnects   int64
	FailedReconnects       int64
	ServerLoad             ServerLoadMetrics
}

// ServerLoadMetrics tracks server load during reconnection storms
type ServerLoadMetrics struct {
	CPUUsage        float64
	MemoryUsage     float64
	ConnectionCount int64
	RequestRate     float64
	ErrorRate       float64
	ResponseTime    time.Duration
}

// NewNetworkResilienceScenario creates a new network resilience scenario
func NewNetworkResilienceScenario(config *NetworkResilienceConfig) *NetworkResilienceScenario {
	logger := slog.Default().With("scenario", "network_resilience")

	// Set defaults
	if config.ProfileDistribution == nil {
		config.ProfileDistribution = map[framework.DeviceProfile]float64{
			framework.ProfileFull:        0.3,
			framework.ProfileConstrained: 0.5,
			framework.ProfileMinimal:     0.2,
		}
	}

	if config.RecoveryTargetTime == 0 {
		config.RecoveryTargetTime = 2 * time.Minute
	}

	if config.MaxReconnectAttempts == 0 {
		config.MaxReconnectAttempts = 5
	}

	// Set default network events if none provided
	if len(config.NetworkEvents) == 0 {
		config.NetworkEvents = []NetworkEvent{
			{
				Type:            EventPartition,
				StartTime:       2 * time.Minute,
				Duration:        30 * time.Second,
				AffectedPercent: 0.2,
				Severity:        SeverityHigh,
				Description:     "Network partition affecting 20% of devices",
			},
			{
				Type:            EventLatency,
				StartTime:       5 * time.Minute,
				Duration:        2 * time.Minute,
				AffectedPercent: 0.5,
				Severity:        SeverityMedium,
				Description:     "High latency affecting 50% of devices",
			},
			{
				Type:            EventIntermittent,
				StartTime:       8 * time.Minute,
				Duration:        5 * time.Minute,
				AffectedPercent: 0.3,
				Severity:        SeverityMedium,
				Description:     "Intermittent connectivity affecting 30% of devices",
			},
		}
	}

	return &NetworkResilienceScenario{
		config: config,
		logger: logger,
		metrics: &NetworkResilienceMetrics{
			StartTime: time.Now(),
		},
	}
}

// Run executes the network resilience scenario
func (s *NetworkResilienceScenario) Run(ctx context.Context) error {
	s.logger.Info("Starting network resilience scenario",
		"total_devices", s.config.TotalDevices,
		"test_duration", s.config.TestDuration,
		"network_events", len(s.config.NetworkEvents),
		"recovery_target_time", s.config.RecoveryTargetTime,
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
		MaxConcurrentRequests: 1000, // Higher limit for reconnection storms
		TestDuration:          s.config.TestDuration,
		AuthToken:             s.config.AuthToken,
		TLSEnabled:            s.config.TLSEnabled,
	}

	// Create and start fleet simulator
	fleet := framework.NewFleetSimulator(fleetConfig)

	// Start monitoring
	var wg sync.WaitGroup
	monitorCtx, cancelMonitor := context.WithCancel(ctx)

	wg.Add(2)
	go func() {
		defer wg.Done()
		s.monitorResilienceMetrics(monitorCtx, fleet)
	}()

	go func() {
		defer wg.Done()
		s.monitorReconnectionStorms(monitorCtx, fleet)
	}()

	// Start the fleet
	err := fleet.Start()
	if err != nil {
		cancelMonitor()
		wg.Wait()
		return fmt.Errorf("failed to start fleet: %w", err)
	}

	s.metrics.TotalDevices = int64(s.config.TotalDevices)

	// Wait for initial stabilization
	s.logger.Info("Waiting for fleet stabilization")
	time.Sleep(30 * time.Second)

	// Execute network events
	scenarioErr := s.executeNetworkEvents(ctx, fleet)

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

	if scenarioErr != nil {
		return scenarioErr
	}

	return analysisErr
}

// calculateDeviceProfiles converts percentage distribution to device counts
func (s *NetworkResilienceScenario) calculateDeviceProfiles() map[framework.DeviceProfile]int {
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

// executeNetworkEvents runs all configured network events
func (s *NetworkResilienceScenario) executeNetworkEvents(ctx context.Context, fleet *framework.FleetSimulator) error {
	// Schedule network events
	for i, event := range s.config.NetworkEvents {
		eventIndex := i
		eventConfig := event

		go func(idx int, evt NetworkEvent) {
			// Wait for event start time
			select {
			case <-ctx.Done():
				return
			case <-time.After(evt.StartTime):
			}

			s.logger.Info("Starting network event",
				"event_index", idx,
				"type", evt.Type,
				"duration", evt.Duration,
				"affected_percent", evt.AffectedPercent,
				"severity", evt.Severity,
				"description", evt.Description,
			)

			s.executeNetworkEvent(ctx, fleet, evt, idx)
		}(eventIndex, eventConfig)
	}

	// Wait for test duration
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(s.config.TestDuration):
		s.logger.Info("Network resilience test duration completed")
	}

	return nil
}

// executeNetworkEvent simulates a specific network event
func (s *NetworkResilienceScenario) executeNetworkEvent(ctx context.Context, fleet *framework.FleetSimulator, event NetworkEvent, eventIndex int) {
	eventStart := time.Now()

	devices := fleet.ListDevices()
	affectedCount := int(float64(len(devices)) * event.AffectedPercent)

	s.logger.Info("Executing network event",
		"type", event.Type,
		"affected_devices", affectedCount,
		"duration", event.Duration,
	)

	eventMetrics := NetworkEventMetrics{
		Event:     event,
		StartTime: eventStart,
	}

	// Select devices to affect
	affectedDevices := devices[:affectedCount]

	// Start the network disruption
	var wg sync.WaitGroup
	recoveryTimes := make([]time.Duration, 0, affectedCount)
	var recoveryMu sync.Mutex

	for _, device := range affectedDevices {
		wg.Add(1)
		go func(d *framework.VirtualDevice) {
			defer wg.Done()

			disruptionStart := time.Now()
			s.simulateNetworkDisruption(ctx, d, event)

			// Wait for event duration
			select {
			case <-ctx.Done():
				return
			case <-time.After(event.Duration):
			}

			// Start recovery
			recovered := s.simulateNetworkRecovery(ctx, d, event)

			if recovered {
				recoveryTime := time.Since(disruptionStart)

				recoveryMu.Lock()
				recoveryTimes = append(recoveryTimes, recoveryTime)
				s.metrics.RecoveryTimes = append(s.metrics.RecoveryTimes, recoveryTime)

				if recoveryTime <= s.config.RecoveryTargetTime {
					eventMetrics.DevicesRecoveredQuickly++
					s.metrics.DevicesRecovered++
				} else {
					eventMetrics.DevicesRecoveredSlow++
					s.metrics.DevicesRecovered++
				}
				s.metrics.SuccessfulReconnections++
				recoveryMu.Unlock()

				s.logger.Debug("Device recovered from network event",
					"device_id", d.Config.DeviceID,
					"recovery_time", recoveryTime,
					"event_type", event.Type,
				)
			} else {
				recoveryMu.Lock()
				eventMetrics.DevicesFailedToRecover++
				s.metrics.DevicesFailedToRecover++
				s.metrics.FailedReconnections++
				recoveryMu.Unlock()

				s.logger.Warn("Device failed to recover from network event",
					"device_id", d.Config.DeviceID,
					"event_type", event.Type,
				)
			}
		}(device)
	}

	wg.Wait()

	eventMetrics.EndTime = time.Now()
	eventMetrics.DevicesAffected = int64(affectedCount)

	// Calculate average recovery time for this event
	if len(recoveryTimes) > 0 {
		var total time.Duration
		for _, rt := range recoveryTimes {
			total += rt
		}
		eventMetrics.AverageRecoveryTime = total / time.Duration(len(recoveryTimes))
	}

	s.mu.Lock()
	s.metrics.EventMetrics = append(s.metrics.EventMetrics, eventMetrics)
	s.metrics.NetworkEventsExecuted++
	s.metrics.DevicesAffected += int64(affectedCount)
	s.mu.Unlock()

	s.logger.Info("Network event completed",
		"type", event.Type,
		"devices_affected", affectedCount,
		"devices_recovered_quickly", eventMetrics.DevicesRecoveredQuickly,
		"devices_recovered_slow", eventMetrics.DevicesRecoveredSlow,
		"devices_failed_to_recover", eventMetrics.DevicesFailedToRecover,
		"average_recovery_time", eventMetrics.AverageRecoveryTime,
	)
}

// simulateNetworkDisruption simulates network disruption for a device
func (s *NetworkResilienceScenario) simulateNetworkDisruption(ctx context.Context, device *framework.VirtualDevice, event NetworkEvent) {
	switch event.Type {
	case EventPartition:
		// Complete network partition - stop the device
		if err := device.Stop(); err != nil {
			s.logger.Error("Failed to stop device for partition simulation", "device_id", device.Config.DeviceID, "error", err)
		}

	case EventLatency, EventPacketLoss, EventBandwidthLimit:
		// These would typically be simulated by modifying network behavior
		// For this simulation, we'll introduce delays in device operations
		s.logger.Debug("Simulating network degradation", "device_id", device.Config.DeviceID, "type", event.Type)

	case EventDNSFailure:
		// DNS failure simulation
		s.logger.Debug("Simulating DNS failure", "device_id", device.Config.DeviceID)

	case EventIntermittent:
		// Intermittent connectivity - stop and start device randomly
		go s.simulateIntermittentConnectivity(ctx, device, event.Duration)
	}
}

// simulateIntermittentConnectivity creates intermittent connectivity issues
func (s *NetworkResilienceScenario) simulateIntermittentConnectivity(ctx context.Context, device *framework.VirtualDevice, duration time.Duration) {
	endTime := time.Now().Add(duration)

	for time.Now().Before(endTime) {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Random connectivity periods
		if s.randomFloat() < 0.3 { // 30% chance of disconnection
			if device.IsStarted() {
				device.Stop()
				s.logger.Debug("Device disconnected (intermittent)", "device_id", device.Config.DeviceID)
			}
		} else {
			if !device.IsStarted() {
				device.Start()
				s.logger.Debug("Device reconnected (intermittent)", "device_id", device.Config.DeviceID)
			}
		}

		// Wait 5-15 seconds before next state change
		waitTime := time.Duration(5+s.randomFloat()*10) * time.Second
		select {
		case <-ctx.Done():
			return
		case <-time.After(waitTime):
		}
	}
}

// simulateNetworkRecovery simulates device recovery after network event
func (s *NetworkResilienceScenario) simulateNetworkRecovery(ctx context.Context, device *framework.VirtualDevice, event NetworkEvent) bool {
	maxAttempts := s.config.MaxReconnectAttempts
	backoff := 2 * time.Second

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return false
		default:
		}

		s.metrics.ReconnectionAttempts++

		// Simulate reconnection attempt
		s.logger.Debug("Attempting device reconnection",
			"device_id", device.Config.DeviceID,
			"attempt", attempt,
			"max_attempts", maxAttempts,
		)

		// Different profiles have different recovery characteristics
		var successProbability float64
		switch device.Config.Profile {
		case framework.ProfileFull:
			successProbability = 0.8 // 80% success rate per attempt
		case framework.ProfileConstrained:
			successProbability = 0.6 // 60% success rate per attempt
		case framework.ProfileMinimal:
			successProbability = 0.4 // 40% success rate per attempt
		}

		// Higher severity events are harder to recover from
		switch event.Severity {
		case SeverityHigh:
			successProbability *= 0.7
		case SeverityCritical:
			successProbability *= 0.5
		}

		if s.randomFloat() < successProbability {
			// Successful reconnection
			if !device.IsStarted() {
				if err := device.Start(); err != nil {
					s.logger.Error("Failed to restart device during recovery", "device_id", device.Config.DeviceID, "error", err)
				} else {
					return true
				}
			} else {
				return true
			}
		}

		// Failed attempt - wait with exponential backoff
		if attempt < maxAttempts {
			select {
			case <-ctx.Done():
				return false
			case <-time.After(backoff):
			}
			backoff *= 2
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
			}
		}
	}

	return false
}

// monitorResilienceMetrics monitors resilience-specific metrics
func (s *NetworkResilienceScenario) monitorResilienceMetrics(ctx context.Context, fleet *framework.FleetSimulator) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.updateResilienceMetrics(fleet)
		}
	}
}

// monitorReconnectionStorms monitors for reconnection storms
func (s *NetworkResilienceScenario) monitorReconnectionStorms(ctx context.Context, fleet *framework.FleetSimulator) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	var lastReconnects int64
	stormThreshold := int64(50) // Consider >50 reconnects/sec a storm

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			fleetMetrics := fleet.GetMetrics()
			currentReconnects := s.metrics.ReconnectionAttempts

			reconnectsPerSec := currentReconnects - lastReconnects
			lastReconnects = currentReconnects

			if reconnectsPerSec > stormThreshold {
				s.logger.Warn("Reconnection storm detected",
					"reconnects_per_sec", reconnectsPerSec,
					"online_devices", fleetMetrics.OnlineDevices,
				)
			}

			// Track max concurrent reconnects
			if reconnectsPerSec > s.metrics.MaxConcurrentReconnects {
				s.metrics.MaxConcurrentReconnects = reconnectsPerSec
			}
		}
	}
}

// updateResilienceMetrics updates scenario-specific metrics
func (s *NetworkResilienceScenario) updateResilienceMetrics(fleet *framework.FleetSimulator) {
	fleetMetrics := fleet.GetMetrics()

	// Calculate network health score
	totalDevices := float64(s.metrics.TotalDevices)
	if totalDevices > 0 {
		onlineRatio := float64(fleetMetrics.OnlineDevices) / totalDevices
		errorRatio := float64(fleetMetrics.TotalErrors) / float64(fleetMetrics.TotalRequests+1)

		// Network health score (0-100)
		s.metrics.NetworkHealthScore = (onlineRatio * 100) * (1.0 - errorRatio)
		if s.metrics.NetworkHealthScore < 0 {
			s.metrics.NetworkHealthScore = 0
		}
	}
}

// analyzeResults analyzes the network resilience test results
func (s *NetworkResilienceScenario) analyzeResults() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	duration := s.metrics.EndTime.Sub(s.metrics.StartTime)

	// Calculate recovery time percentiles
	if len(s.metrics.RecoveryTimes) > 0 {
		// Sort recovery times (simplified for this example)
		times := make([]time.Duration, len(s.metrics.RecoveryTimes))
		copy(times, s.metrics.RecoveryTimes)

		// Calculate percentiles (simplified)
		p95Index := int(float64(len(times)) * 0.95)
		p99Index := int(float64(len(times)) * 0.99)
		if p95Index >= len(times) {
			p95Index = len(times) - 1
		}
		if p99Index >= len(times) {
			p99Index = len(times) - 1
		}

		s.metrics.P95RecoveryTime = times[p95Index]
		s.metrics.P99RecoveryTime = times[p99Index]

		// Calculate average recovery time
		var total time.Duration
		for _, rt := range times {
			total += rt
		}
		s.metrics.AverageRecoveryTime = total / time.Duration(len(times))
	}

	s.logger.Info("Network resilience scenario results",
		"duration", duration,
		"total_devices", s.metrics.TotalDevices,
		"devices_affected", s.metrics.DevicesAffected,
		"devices_recovered", s.metrics.DevicesRecovered,
		"devices_failed_to_recover", s.metrics.DevicesFailedToRecover,
		"network_events_executed", s.metrics.NetworkEventsExecuted,
		"reconnection_attempts", s.metrics.ReconnectionAttempts,
		"successful_reconnections", s.metrics.SuccessfulReconnections,
		"failed_reconnections", s.metrics.FailedReconnections,
		"average_recovery_time", s.metrics.AverageRecoveryTime,
		"p95_recovery_time", s.metrics.P95RecoveryTime,
		"p99_recovery_time", s.metrics.P99RecoveryTime,
		"max_concurrent_reconnects", s.metrics.MaxConcurrentReconnects,
		"network_health_score", s.metrics.NetworkHealthScore,
	)

	// Analyze individual events
	for i, eventMetric := range s.metrics.EventMetrics {
		s.logger.Info("Network event analysis",
			"event_index", i,
			"event_type", eventMetric.Event.Type,
			"devices_affected", eventMetric.DevicesAffected,
			"devices_recovered_quickly", eventMetric.DevicesRecoveredQuickly,
			"devices_recovered_slow", eventMetric.DevicesRecoveredSlow,
			"devices_failed_to_recover", eventMetric.DevicesFailedToRecover,
			"average_recovery_time", eventMetric.AverageRecoveryTime,
		)
	}

	// Determine success criteria
	var errors []string

	// Check recovery rate
	if s.metrics.DevicesAffected > 0 {
		recoveryRate := float64(s.metrics.DevicesRecovered) / float64(s.metrics.DevicesAffected)
		if recoveryRate < 0.90 { // 90% recovery rate required
			errors = append(errors, fmt.Sprintf("recovery rate %.2f%% below threshold 90%%", recoveryRate*100))
		}
	}

	// Check recovery time
	if s.metrics.P95RecoveryTime > s.config.RecoveryTargetTime*2 {
		errors = append(errors, fmt.Sprintf("P95 recovery time %v exceeds 2x target time %v",
			s.metrics.P95RecoveryTime, s.config.RecoveryTargetTime))
	}

	// Check network health score
	if s.metrics.NetworkHealthScore < 70 {
		errors = append(errors, fmt.Sprintf("network health score %.1f below threshold 70",
			s.metrics.NetworkHealthScore))
	}

	if len(errors) > 0 {
		s.logger.Error("Network resilience scenario FAILED", "errors", errors)
		return fmt.Errorf("scenario failed: %v", errors)
	}

	s.logger.Info("Network resilience scenario PASSED")
	return nil
}

// randomFloat returns a pseudo-random float between 0 and 1
func (s *NetworkResilienceScenario) randomFloat() float64 {
	return float64(time.Now().UnixNano()%1000) / 1000.0
}

// GetMetrics returns the current scenario metrics
func (s *NetworkResilienceScenario) GetMetrics() NetworkResilienceMetrics {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Copy slices
	var recoveryTimes []time.Duration
	if s.metrics.RecoveryTimes != nil {
		recoveryTimes = make([]time.Duration, len(s.metrics.RecoveryTimes))
		copy(recoveryTimes, s.metrics.RecoveryTimes)
	}

	var eventMetrics []NetworkEventMetrics
	if s.metrics.EventMetrics != nil {
		eventMetrics = make([]NetworkEventMetrics, len(s.metrics.EventMetrics))
		copy(eventMetrics, s.metrics.EventMetrics)
	}

	var reconnectionStorms []ReconnectionStormMetrics
	if s.metrics.ReconnectionStorms != nil {
		reconnectionStorms = make([]ReconnectionStormMetrics, len(s.metrics.ReconnectionStorms))
		copy(reconnectionStorms, s.metrics.ReconnectionStorms)
	}

	// Deep copy without mutex
	return NetworkResilienceMetrics{
		StartTime:               s.metrics.StartTime,
		EndTime:                 s.metrics.EndTime,
		TotalDevices:            s.metrics.TotalDevices,
		DevicesAffected:         s.metrics.DevicesAffected,
		DevicesRecovered:        s.metrics.DevicesRecovered,
		DevicesFailedToRecover:  s.metrics.DevicesFailedToRecover,
		NetworkEventsExecuted:   s.metrics.NetworkEventsExecuted,
		ReconnectionAttempts:    s.metrics.ReconnectionAttempts,
		SuccessfulReconnections: s.metrics.SuccessfulReconnections,
		FailedReconnections:     s.metrics.FailedReconnections,
		AverageRecoveryTime:     s.metrics.AverageRecoveryTime,
		P95RecoveryTime:         s.metrics.P95RecoveryTime,
		P99RecoveryTime:         s.metrics.P99RecoveryTime,
		RecoveryTimes:           recoveryTimes,
		EventMetrics:            eventMetrics,
		ReconnectionStorms:      reconnectionStorms,
		MaxConcurrentReconnects: s.metrics.MaxConcurrentReconnects,
		NetworkHealthScore:      s.metrics.NetworkHealthScore,
	}
}

// GetName returns the scenario name
func (s *NetworkResilienceScenario) GetName() string {
	return "network_resilience"
}

// GetDescription returns the scenario description
func (s *NetworkResilienceScenario) GetDescription() string {
	return "Simulates network issues and tests device reconnection capabilities and system resilience"
}
