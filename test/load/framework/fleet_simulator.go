package framework

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

// FleetSimulator manages multiple virtual devices
type FleetSimulator struct {
	devices   map[string]*VirtualDevice
	mu        sync.RWMutex
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	config    *FleetConfig
	metrics   *FleetMetrics
	eventChan chan Event
	logger    *slog.Logger
}

// FleetConfig defines the configuration for fleet simulation
type FleetConfig struct {
	ServerURL             string
	TotalDevices          int
	DeviceProfiles        map[DeviceProfile]int // Number of devices per profile
	StartupBatchSize      int                   // Number of devices to start in each batch
	StartupBatchInterval  time.Duration         // Delay between batches
	MaxConcurrentRequests int
	TestDuration          time.Duration
	RampUpDuration        time.Duration
	RampDownDuration      time.Duration
	AuthToken             string
	TLSEnabled            bool
}

// FleetMetrics tracks overall fleet performance
type FleetMetrics struct {
	TotalDevices       int64
	OnlineDevices      int64
	OfflineDevices     int64
	ErrorDevices       int64
	UpdatingDevices    int64
	TotalRequests      int64
	SuccessfulRequests int64
	FailedRequests     int64
	TotalMetricsSent   int64
	TotalErrors        int64
	StartTime          time.Time
	mu                 sync.RWMutex
}

// Event represents a fleet simulation event
type Event struct {
	Type      EventType
	DeviceID  string
	Message   string
	Timestamp time.Time
	Data      map[string]interface{}
}

// EventType defines the type of simulation event
type EventType string

const (
	EventDeviceStarted    EventType = "device_started"
	EventDeviceStopped    EventType = "device_stopped"
	EventDeviceError      EventType = "device_error"
	EventDeviceRegistered EventType = "device_registered"
	EventMetricsSent      EventType = "metrics_sent"
	EventHeartbeatSent    EventType = "heartbeat_sent"
	EventFleetStarted     EventType = "fleet_started"
	EventFleetStopped     EventType = "fleet_stopped"
	EventBatchStarted     EventType = "batch_started"
	EventBatchCompleted   EventType = "batch_completed"
)

// NewFleetSimulator creates a new fleet simulator
func NewFleetSimulator(config *FleetConfig) *FleetSimulator {
	ctx, cancel := context.WithCancel(context.Background())

	logger := slog.Default().With("component", "fleet_simulator")

	return &FleetSimulator{
		devices:   make(map[string]*VirtualDevice),
		ctx:       ctx,
		cancel:    cancel,
		config:    config,
		metrics:   &FleetMetrics{StartTime: time.Now()},
		eventChan: make(chan Event, 10000), // Large buffer for high-frequency events
		logger:    logger,
	}
}

// Start begins the fleet simulation
func (fs *FleetSimulator) Start() error {
	fs.logger.Info("Starting fleet simulation",
		"total_devices", fs.config.TotalDevices,
		"profiles", fs.config.DeviceProfiles,
		"batch_size", fs.config.StartupBatchSize,
	)

	// Emit fleet started event
	fs.emitEvent(Event{
		Type:      EventFleetStarted,
		Message:   fmt.Sprintf("Starting fleet with %d devices", fs.config.TotalDevices),
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"total_devices": fs.config.TotalDevices,
			"profiles":      fs.config.DeviceProfiles,
		},
	})

	// Start metrics collection
	fs.wg.Add(1)
	go fs.metricsCollectionLoop()

	// Create devices
	devices := fs.createDevices()

	// Start devices in batches
	return fs.startDevicesInBatches(devices)
}

// Stop gracefully shuts down all devices
func (fs *FleetSimulator) Stop() error {
	fs.logger.Info("Stopping fleet simulation")

	// Emit fleet stopped event
	fs.emitEvent(Event{
		Type:      EventFleetStopped,
		Message:   "Stopping fleet simulation",
		Timestamp: time.Now(),
	})

	// Stop all devices
	fs.mu.RLock()
	var devices []*VirtualDevice
	for _, device := range fs.devices {
		devices = append(devices, device)
	}
	fs.mu.RUnlock()

	// Stop devices in parallel with controlled concurrency
	sem := make(chan struct{}, 50) // Limit concurrent stops
	var wg sync.WaitGroup

	for _, device := range devices {
		wg.Add(1)
		go func(d *VirtualDevice) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			if err := d.Stop(); err != nil {
				fs.logger.Error("Failed to stop device", "device_id", d.Config.DeviceID, "error", err)
			}
		}(device)
	}

	wg.Wait()

	// Cancel context and wait for goroutines
	fs.cancel()
	fs.wg.Wait()

	// Close event channel
	close(fs.eventChan)

	return nil
}

// createDevices creates all virtual devices based on the configuration
func (fs *FleetSimulator) createDevices() []*VirtualDevice {
	var devices []*VirtualDevice

	for profile, count := range fs.config.DeviceProfiles {
		for i := 0; i < count; i++ {
			deviceID := uuid.New().String()
			deviceConfig := &DeviceConfig{
				Profile:    profile,
				DeviceID:   deviceID,
				Name:       fmt.Sprintf("device-%s-%d", profile, i),
				Type:       string(profile),
				ServerURL:  fs.config.ServerURL,
				TLSEnabled: fs.config.TLSEnabled,
				AuthToken:  fs.config.AuthToken,
			}

			device := NewVirtualDevice(deviceConfig)
			devices = append(devices, device)

			fs.mu.Lock()
			fs.devices[deviceID] = device
			fs.mu.Unlock()

			atomic.AddInt64(&fs.metrics.TotalDevices, 1)
		}
	}

	fs.logger.Info("Created devices", "count", len(devices))
	return devices
}

// startDevicesInBatches starts devices in controlled batches
func (fs *FleetSimulator) startDevicesInBatches(devices []*VirtualDevice) error {
	batchSize := fs.config.StartupBatchSize
	if batchSize <= 0 {
		batchSize = 10 // Default batch size
	}

	for i := 0; i < len(devices); i += batchSize {
		end := i + batchSize
		if end > len(devices) {
			end = len(devices)
		}

		batch := devices[i:end]
		batchNum := i/batchSize + 1

		fs.logger.Info("Starting device batch",
			"batch", batchNum,
			"devices", len(batch),
			"total_batches", (len(devices)+batchSize-1)/batchSize,
		)

		// Emit batch started event
		fs.emitEvent(Event{
			Type:      EventBatchStarted,
			Message:   fmt.Sprintf("Starting batch %d with %d devices", batchNum, len(batch)),
			Timestamp: time.Now(),
			Data: map[string]interface{}{
				"batch_number": batchNum,
				"batch_size":   len(batch),
			},
		})

		// Start devices in parallel within the batch
		var batchWg sync.WaitGroup
		sem := make(chan struct{}, 20) // Limit concurrent starts within batch

		for _, device := range batch {
			batchWg.Add(1)
			go func(d *VirtualDevice) {
				defer batchWg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				if err := d.Start(); err != nil {
					fs.logger.Error("Failed to start device", "device_id", d.Config.DeviceID, "error", err)
					atomic.AddInt64(&fs.metrics.FailedRequests, 1)
					fs.emitEvent(Event{
						Type:      EventDeviceError,
						DeviceID:  d.Config.DeviceID,
						Message:   fmt.Sprintf("Failed to start device: %v", err),
						Timestamp: time.Now(),
					})
				} else {
					atomic.AddInt64(&fs.metrics.OnlineDevices, 1)
					atomic.AddInt64(&fs.metrics.SuccessfulRequests, 1)
					fs.emitEvent(Event{
						Type:      EventDeviceStarted,
						DeviceID:  d.Config.DeviceID,
						Message:   "Device started successfully",
						Timestamp: time.Now(),
					})
				}
			}(device)
		}

		batchWg.Wait()

		// Emit batch completed event
		fs.emitEvent(Event{
			Type:      EventBatchCompleted,
			Message:   fmt.Sprintf("Completed batch %d", batchNum),
			Timestamp: time.Now(),
			Data: map[string]interface{}{
				"batch_number": batchNum,
			},
		})

		// Wait before starting next batch (except for the last batch)
		if end < len(devices) && fs.config.StartupBatchInterval > 0 {
			select {
			case <-fs.ctx.Done():
				return fs.ctx.Err()
			case <-time.After(fs.config.StartupBatchInterval):
			}
		}
	}

	fs.logger.Info("All devices started successfully")
	return nil
}

// metricsCollectionLoop periodically collects metrics from all devices
func (fs *FleetSimulator) metricsCollectionLoop() {
	defer fs.wg.Done()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-fs.ctx.Done():
			return
		case <-ticker.C:
			fs.collectMetrics()
		}
	}
}

// collectMetrics aggregates metrics from all devices
func (fs *FleetSimulator) collectMetrics() {
	fs.mu.RLock()
	devices := make([]*VirtualDevice, 0, len(fs.devices))
	for _, device := range fs.devices {
		devices = append(devices, device)
	}
	fs.mu.RUnlock()

	var online, offline, error_count, updating int64
	var totalRequests, metricsSent, totalErrors int64

	for _, device := range devices {
		state := device.GetState()

		switch state.Status {
		case 1: // ONLINE
			atomic.AddInt64(&online, 1)
		case 2: // OFFLINE
			atomic.AddInt64(&offline, 1)
		case 3: // UPDATING
			atomic.AddInt64(&updating, 1)
		case 4: // ERROR
			atomic.AddInt64(&error_count, 1)
		}

		atomic.AddInt64(&totalRequests, state.MessagesSent)
		atomic.AddInt64(&metricsSent, state.MetricsSent)
		atomic.AddInt64(&totalErrors, state.ErrorCount)
	}

	// Update fleet metrics
	fs.metrics.mu.Lock()
	fs.metrics.OnlineDevices = online
	fs.metrics.OfflineDevices = offline
	fs.metrics.ErrorDevices = error_count
	fs.metrics.UpdatingDevices = updating
	fs.metrics.TotalRequests = totalRequests
	fs.metrics.TotalMetricsSent = metricsSent
	fs.metrics.TotalErrors = totalErrors
	fs.metrics.mu.Unlock()
}

// emitEvent sends an event to the event channel
func (fs *FleetSimulator) emitEvent(event Event) {
	select {
	case fs.eventChan <- event:
	default:
		// Channel is full, drop the event
		fs.logger.Warn("Event channel full, dropping event", "type", event.Type)
	}
}

// GetMetrics returns a copy of current fleet metrics
func (fs *FleetSimulator) GetMetrics() *FleetMetrics {
	fs.metrics.mu.RLock()
	defer fs.metrics.mu.RUnlock()
	return &FleetMetrics{
		TotalDevices:       fs.metrics.TotalDevices,
		OnlineDevices:      fs.metrics.OnlineDevices,
		OfflineDevices:     fs.metrics.OfflineDevices,
		ErrorDevices:       fs.metrics.ErrorDevices,
		UpdatingDevices:    fs.metrics.UpdatingDevices,
		TotalRequests:      fs.metrics.TotalRequests,
		SuccessfulRequests: fs.metrics.SuccessfulRequests,
		FailedRequests:     fs.metrics.FailedRequests,
		TotalMetricsSent:   fs.metrics.TotalMetricsSent,
		TotalErrors:        fs.metrics.TotalErrors,
		StartTime:          fs.metrics.StartTime,
	}
}

// GetEvents returns the event channel for consuming simulation events
func (fs *FleetSimulator) GetEvents() <-chan Event {
	return fs.eventChan
}

// GetDevice returns a specific device by ID
func (fs *FleetSimulator) GetDevice(deviceID string) (*VirtualDevice, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	device, exists := fs.devices[deviceID]
	if !exists {
		return nil, fmt.Errorf("device not found: %s", deviceID)
	}

	return device, nil
}

// ListDevices returns all devices
func (fs *FleetSimulator) ListDevices() []*VirtualDevice {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	devices := make([]*VirtualDevice, 0, len(fs.devices))
	for _, device := range fs.devices {
		devices = append(devices, device)
	}

	return devices
}

// ScaleFleet dynamically adds or removes devices
func (fs *FleetSimulator) ScaleFleet(profileCounts map[DeviceProfile]int) error {
	fs.logger.Info("Scaling fleet", "new_counts", profileCounts)

	// Calculate current counts
	currentCounts := make(map[DeviceProfile]int)
	fs.mu.RLock()
	for _, device := range fs.devices {
		currentCounts[device.Config.Profile]++
	}
	fs.mu.RUnlock()

	// Add new devices
	for profile, targetCount := range profileCounts {
		currentCount := currentCounts[profile]
		if targetCount > currentCount {
			toAdd := targetCount - currentCount
			fs.logger.Info("Adding devices", "profile", profile, "count", toAdd)

			var newDevices []*VirtualDevice
			for i := 0; i < toAdd; i++ {
				deviceID := uuid.New().String()
				deviceConfig := &DeviceConfig{
					Profile:    profile,
					DeviceID:   deviceID,
					Name:       fmt.Sprintf("device-%s-scaled-%d", profile, i),
					Type:       string(profile),
					ServerURL:  fs.config.ServerURL,
					TLSEnabled: fs.config.TLSEnabled,
					AuthToken:  fs.config.AuthToken,
				}

				device := NewVirtualDevice(deviceConfig)
				newDevices = append(newDevices, device)

				fs.mu.Lock()
				fs.devices[deviceID] = device
				fs.mu.Unlock()

				atomic.AddInt64(&fs.metrics.TotalDevices, 1)
			}

			// Start new devices
			go fs.startDevicesInBatches(newDevices)
		}
	}

	// Remove excess devices (if needed)
	for profile, targetCount := range profileCounts {
		currentCount := currentCounts[profile]
		if targetCount < currentCount {
			toRemove := currentCount - targetCount
			fs.logger.Info("Removing devices", "profile", profile, "count", toRemove)

			// Find devices to remove
			var devicesToRemove []*VirtualDevice
			removed := 0
			fs.mu.RLock()
			for _, device := range fs.devices {
				if device.Config.Profile == profile && removed < toRemove {
					devicesToRemove = append(devicesToRemove, device)
					removed++
				}
			}
			fs.mu.RUnlock()

			// Stop and remove devices
			for _, device := range devicesToRemove {
				go func(d *VirtualDevice) {
					if err := d.Stop(); err != nil {
						fs.logger.Error("Failed to stop device during scaling", "device_id", d.Config.DeviceID, "error", err)
					}

					fs.mu.Lock()
					delete(fs.devices, d.Config.DeviceID)
					fs.mu.Unlock()

					atomic.AddInt64(&fs.metrics.TotalDevices, -1)
					atomic.AddInt64(&fs.metrics.OnlineDevices, -1)
				}(device)
			}
		}
	}

	return nil
}

// SimulateNetworkPartition simulates a network partition affecting some devices
func (fs *FleetSimulator) SimulateNetworkPartition(percentage float64, duration time.Duration) {
	fs.logger.Info("Simulating network partition", "percentage", percentage, "duration", duration)

	devices := fs.ListDevices()
	affectedCount := int(float64(len(devices)) * percentage)

	// Randomly select devices to affect
	for i := 0; i < affectedCount && i < len(devices); i++ {
		device := devices[i]
		go func(d *VirtualDevice) {
			// Simulate by stopping the device temporarily
			if err := d.Stop(); err != nil {
				fs.logger.Error("Failed to stop device for partition simulation", "device_id", d.Config.DeviceID, "error", err)
				return
			}

			atomic.AddInt64(&fs.metrics.OnlineDevices, -1)
			atomic.AddInt64(&fs.metrics.OfflineDevices, 1)

			// Wait for partition duration
			time.Sleep(duration)

			// Restart the device
			if err := d.Start(); err != nil {
				fs.logger.Error("Failed to restart device after partition", "device_id", d.Config.DeviceID, "error", err)
				return
			}

			atomic.AddInt64(&fs.metrics.OnlineDevices, 1)
			atomic.AddInt64(&fs.metrics.OfflineDevices, -1)
		}(device)
	}
}

// GetDevicesByProfile returns devices filtered by profile
func (fs *FleetSimulator) GetDevicesByProfile(profile DeviceProfile) []*VirtualDevice {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	var devices []*VirtualDevice
	for _, device := range fs.devices {
		if device.Config.Profile == profile {
			devices = append(devices, device)
		}
	}

	return devices
}
