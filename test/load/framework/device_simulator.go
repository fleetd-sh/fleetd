package framework

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"math"
	"math/big"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"fleetd.sh/gen/public/v1"
	"fleetd.sh/gen/public/v1/publicv1connect"
	"golang.org/x/time/rate"
)

// DeviceProfile defines the characteristics of a simulated device
type DeviceProfile string

const (
	ProfileFull        DeviceProfile = "full"        // High-end device with all capabilities
	ProfileConstrained DeviceProfile = "constrained" // Mid-range device with limited resources
	ProfileMinimal     DeviceProfile = "minimal"     // Low-end device with basic capabilities
)

// DeviceConfig holds configuration for a simulated device
type DeviceConfig struct {
	Profile           DeviceProfile
	DeviceID          string
	Name              string
	Type              string
	MetricsInterval   time.Duration
	HeartbeatInterval time.Duration
	ReconnectDelay    time.Duration
	MaxReconnectDelay time.Duration
	JitterPercent     float64
	ErrorRate         float64
	ServerURL         string
	TLSEnabled        bool
	AuthToken         string
}

// VirtualDevice represents a simulated fleetd device
type VirtualDevice struct {
	Config  *DeviceConfig
	ctx     context.Context
	cancel  context.CancelFunc
	client  publicv1connect.FleetServiceClient
	metrics *DeviceMetrics
	state   DeviceState
	mu      sync.RWMutex
	wg      sync.WaitGroup
	started bool
	limiter *rate.Limiter
	logger  *slog.Logger
}

// DeviceState tracks the current state of a virtual device
type DeviceState struct {
	Status       publicv1.DeviceStatus
	LastSeen     time.Time
	Registered   bool
	Updating     bool
	ErrorCount   int64
	MessagesSent int64
	MetricsSent  int64
	Uptime       time.Duration
	StartTime    time.Time
}

// DeviceMetrics holds metrics for a virtual device
type DeviceMetrics struct {
	CPU     CPUMetrics
	Memory  MemoryMetrics
	Disk    DiskMetrics
	Network NetworkMetrics
	System  DeviceSystemInfo
	Custom  map[string]interface{}
	mu      sync.RWMutex
}

type CPUMetrics struct {
	UsagePercent float64
	LoadAvg1     float64
	LoadAvg5     float64
	LoadAvg15    float64
	Cores        int
}

type MemoryMetrics struct {
	Total       uint64
	Used        uint64
	Available   uint64
	UsedPercent float64
}

type DiskMetrics struct {
	Total       uint64
	Used        uint64
	Available   uint64
	UsedPercent float64
}

type NetworkMetrics struct {
	BytesSent   uint64
	BytesRecv   uint64
	PacketsSent uint64
	PacketsRecv uint64
	ErrorsIn    uint64
	ErrorsOut   uint64
}

type DeviceSystemInfo struct {
	Hostname     string
	OS           string
	Platform     string
	Arch         string
	Uptime       time.Duration
	ProcessCount int32
	Temperature  float64
}

// NewVirtualDevice creates a new virtual device
func NewVirtualDevice(config *DeviceConfig) *VirtualDevice {
	ctx, cancel := context.WithCancel(context.Background())

	// Set device profile defaults
	applyProfileDefaults(config)

	// Create rate limiter based on profile
	var rps float64
	switch config.Profile {
	case ProfileFull:
		rps = 100 // 100 requests per second
	case ProfileConstrained:
		rps = 50 // 50 requests per second
	case ProfileMinimal:
		rps = 10 // 10 requests per second
	}

	logger := slog.Default().With(
		"device_id", config.DeviceID,
		"profile", config.Profile,
	)

	return &VirtualDevice{
		Config: config,
		ctx:    ctx,
		cancel: cancel,
		metrics: &DeviceMetrics{
			Custom: make(map[string]interface{}),
		},
		state: DeviceState{
			Status:    publicv1.DeviceStatus_DEVICE_STATUS_OFFLINE,
			StartTime: time.Now(),
		},
		limiter: rate.NewLimiter(rate.Limit(rps), int(rps)),
		logger:  logger,
	}
}

// applyProfileDefaults sets defaults based on device profile
func applyProfileDefaults(config *DeviceConfig) {
	switch config.Profile {
	case ProfileFull:
		if config.MetricsInterval == 0 {
			config.MetricsInterval = 30 * time.Second
		}
		if config.HeartbeatInterval == 0 {
			config.HeartbeatInterval = 60 * time.Second
		}
		if config.ErrorRate == 0 {
			config.ErrorRate = 0.01 // 1% error rate
		}
		if config.JitterPercent == 0 {
			config.JitterPercent = 0.1 // 10% jitter
		}
	case ProfileConstrained:
		if config.MetricsInterval == 0 {
			config.MetricsInterval = 60 * time.Second
		}
		if config.HeartbeatInterval == 0 {
			config.HeartbeatInterval = 120 * time.Second
		}
		if config.ErrorRate == 0 {
			config.ErrorRate = 0.05 // 5% error rate
		}
		if config.JitterPercent == 0 {
			config.JitterPercent = 0.2 // 20% jitter
		}
	case ProfileMinimal:
		if config.MetricsInterval == 0 {
			config.MetricsInterval = 300 * time.Second
		}
		if config.HeartbeatInterval == 0 {
			config.HeartbeatInterval = 600 * time.Second
		}
		if config.ErrorRate == 0 {
			config.ErrorRate = 0.1 // 10% error rate
		}
		if config.JitterPercent == 0 {
			config.JitterPercent = 0.3 // 30% jitter
		}
	}

	if config.ReconnectDelay == 0 {
		config.ReconnectDelay = 5 * time.Second
	}
	if config.MaxReconnectDelay == 0 {
		config.MaxReconnectDelay = 5 * time.Minute
	}
}

// Start begins the virtual device simulation
func (d *VirtualDevice) Start() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.started {
		return fmt.Errorf("device already started")
	}

	d.logger.Info("Starting virtual device",
		"profile", d.Config.Profile,
		"metrics_interval", d.Config.MetricsInterval,
		"heartbeat_interval", d.Config.HeartbeatInterval,
	)

	// Initialize metrics based on profile
	d.initializeMetrics()

	// Start background goroutines
	d.wg.Add(4)
	go d.metricsLoop()
	go d.heartbeatLoop()
	go d.stateUpdateLoop()
	go d.errorSimulationLoop()

	d.started = true
	d.state.Status = publicv1.DeviceStatus_DEVICE_STATUS_ONLINE
	d.state.StartTime = time.Now()

	return nil
}

// Stop gracefully shuts down the virtual device
func (d *VirtualDevice) Stop() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.started {
		return nil
	}

	d.logger.Info("Stopping virtual device")

	d.cancel()
	d.wg.Wait()

	d.started = false
	d.state.Status = publicv1.DeviceStatus_DEVICE_STATUS_OFFLINE

	return nil
}

// initializeMetrics sets up initial metrics based on device profile
func (d *VirtualDevice) initializeMetrics() {
	d.metrics.mu.Lock()
	defer d.metrics.mu.Unlock()

	// Base system info
	d.metrics.System = DeviceSystemInfo{
		Hostname:     fmt.Sprintf("device-%s", d.Config.DeviceID[:8]),
		OS:           "linux",
		Platform:     "ubuntu",
		Arch:         runtime.GOARCH,
		ProcessCount: 50,
	}

	// Profile-specific resource allocation
	switch d.Config.Profile {
	case ProfileFull:
		d.metrics.CPU.Cores = 8
		d.metrics.Memory.Total = 16 * 1024 * 1024 * 1024 // 16GB
		d.metrics.Disk.Total = 1024 * 1024 * 1024 * 1024 // 1TB
	case ProfileConstrained:
		d.metrics.CPU.Cores = 4
		d.metrics.Memory.Total = 4 * 1024 * 1024 * 1024 // 4GB
		d.metrics.Disk.Total = 256 * 1024 * 1024 * 1024 // 256GB
	case ProfileMinimal:
		d.metrics.CPU.Cores = 2
		d.metrics.Memory.Total = 1024 * 1024 * 1024    // 1GB
		d.metrics.Disk.Total = 32 * 1024 * 1024 * 1024 // 32GB
	}
}

// metricsLoop periodically generates and sends metrics
func (d *VirtualDevice) metricsLoop() {
	defer d.wg.Done()

	ticker := time.NewTicker(d.addJitter(d.Config.MetricsInterval))
	defer ticker.Stop()

	for {
		select {
		case <-d.ctx.Done():
			return
		case <-ticker.C:
			if err := d.limiter.Wait(d.ctx); err != nil {
				continue
			}

			if d.shouldSimulateError() {
				d.logger.Debug("Simulating metrics error")
				atomic.AddInt64(&d.state.ErrorCount, 1)
				continue
			}

			d.generateMetrics()
			if err := d.sendMetrics(); err != nil {
				d.logger.Error("Failed to send metrics", "error", err)
				atomic.AddInt64(&d.state.ErrorCount, 1)
			} else {
				atomic.AddInt64(&d.state.MetricsSent, 1)
			}

			// Reset ticker with jitter
			ticker.Reset(d.addJitter(d.Config.MetricsInterval))
		}
	}
}

// heartbeatLoop sends periodic heartbeats
func (d *VirtualDevice) heartbeatLoop() {
	defer d.wg.Done()

	ticker := time.NewTicker(d.addJitter(d.Config.HeartbeatInterval))
	defer ticker.Stop()

	for {
		select {
		case <-d.ctx.Done():
			return
		case <-ticker.C:
			if err := d.limiter.Wait(d.ctx); err != nil {
				continue
			}

			if d.shouldSimulateError() {
				d.logger.Debug("Simulating heartbeat error")
				atomic.AddInt64(&d.state.ErrorCount, 1)
				continue
			}

			if err := d.sendHeartbeat(); err != nil {
				d.logger.Error("Failed to send heartbeat", "error", err)
				atomic.AddInt64(&d.state.ErrorCount, 1)
			} else {
				atomic.AddInt64(&d.state.MessagesSent, 1)
				d.state.LastSeen = time.Now()
			}

			// Reset ticker with jitter
			ticker.Reset(d.addJitter(d.Config.HeartbeatInterval))
		}
	}
}

// stateUpdateLoop periodically updates device state
func (d *VirtualDevice) stateUpdateLoop() {
	defer d.wg.Done()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-d.ctx.Done():
			return
		case <-ticker.C:
			d.updateState()
		}
	}
}

// errorSimulationLoop occasionally simulates various error conditions
func (d *VirtualDevice) errorSimulationLoop() {
	defer d.wg.Done()

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-d.ctx.Done():
			return
		case <-ticker.C:
			d.simulateRandomErrors()
		}
	}
}

// generateMetrics creates realistic metrics based on device profile
func (d *VirtualDevice) generateMetrics() {
	d.metrics.mu.Lock()
	defer d.metrics.mu.Unlock()

	now := time.Now()
	baseLoad := d.getBaseLoad()

	// Generate CPU metrics with realistic patterns
	d.metrics.CPU.UsagePercent = d.generateCPUUsage(baseLoad)
	d.metrics.CPU.LoadAvg1 = d.metrics.CPU.UsagePercent / 100.0 * float64(d.metrics.CPU.Cores)
	d.metrics.CPU.LoadAvg5 = d.metrics.CPU.LoadAvg1 * (0.8 + 0.4*d.randomFloat())
	d.metrics.CPU.LoadAvg15 = d.metrics.CPU.LoadAvg5 * (0.7 + 0.6*d.randomFloat())

	// Generate memory metrics
	d.metrics.Memory.Used = uint64(float64(d.metrics.Memory.Total) * (0.3 + 0.5*d.randomFloat()))
	d.metrics.Memory.Available = d.metrics.Memory.Total - d.metrics.Memory.Used
	d.metrics.Memory.UsedPercent = float64(d.metrics.Memory.Used) / float64(d.metrics.Memory.Total) * 100

	// Generate disk metrics
	d.metrics.Disk.Used = uint64(float64(d.metrics.Disk.Total) * (0.2 + 0.6*d.randomFloat()))
	d.metrics.Disk.Available = d.metrics.Disk.Total - d.metrics.Disk.Used
	d.metrics.Disk.UsedPercent = float64(d.metrics.Disk.Used) / float64(d.metrics.Disk.Total) * 100

	// Generate network metrics
	d.updateNetworkMetrics()

	// System uptime
	d.metrics.System.Uptime = time.Since(d.state.StartTime)

	// Temperature simulation (for devices that support it)
	if d.Config.Profile != ProfileMinimal {
		d.metrics.System.Temperature = 35 + 25*d.metrics.CPU.UsagePercent/100 + 10*d.randomFloat()
	}

	// Custom metrics based on profile
	d.generateCustomMetrics(now)
}

// getBaseLoad returns the base load factor for the device
func (d *VirtualDevice) getBaseLoad() float64 {
	switch d.Config.Profile {
	case ProfileFull:
		return 0.2 + 0.3*math.Sin(float64(time.Now().Unix())/3600) // Hourly cycles
	case ProfileConstrained:
		return 0.3 + 0.4*math.Sin(float64(time.Now().Unix())/1800) // 30-minute cycles
	case ProfileMinimal:
		return 0.5 + 0.3*math.Sin(float64(time.Now().Unix())/900) // 15-minute cycles
	}
	return 0.3
}

// generateCPUUsage creates realistic CPU usage patterns
func (d *VirtualDevice) generateCPUUsage(baseLoad float64) float64 {
	// Add some randomness and spikes
	usage := baseLoad * 100

	// Random spikes
	if d.randomFloat() < 0.05 { // 5% chance of spike
		usage += 30 + 40*d.randomFloat()
	}

	// Add noise
	usage += (d.randomFloat() - 0.5) * 20

	// Clamp to valid range
	if usage < 0 {
		usage = 0
	}
	if usage > 100 {
		usage = 100
	}

	return usage
}

// updateNetworkMetrics simulates network activity
func (d *VirtualDevice) updateNetworkMetrics() {
	// Simulate data transfer based on activity
	baseTransfer := uint64(1024 * 1024) // 1MB base

	switch d.Config.Profile {
	case ProfileFull:
		baseTransfer *= 10
	case ProfileConstrained:
		baseTransfer *= 5
	case ProfileMinimal:
		baseTransfer *= 2
	}

	// Add randomness
	transfer := uint64(float64(baseTransfer) * (0.5 + d.randomFloat()))

	d.metrics.Network.BytesSent += transfer / 4
	d.metrics.Network.BytesRecv += transfer
	d.metrics.Network.PacketsSent += transfer / 1500
	d.metrics.Network.PacketsRecv += transfer / 1500

	// Occasional errors
	if d.randomFloat() < 0.001 {
		d.metrics.Network.ErrorsIn++
	}
	if d.randomFloat() < 0.001 {
		d.metrics.Network.ErrorsOut++
	}
}

// generateCustomMetrics creates profile-specific custom metrics
func (d *VirtualDevice) generateCustomMetrics(timestamp time.Time) {
	switch d.Config.Profile {
	case ProfileFull:
		d.metrics.Custom["gpu_usage"] = 20 + 60*d.randomFloat()
		d.metrics.Custom["power_consumption"] = 50 + 100*d.randomFloat()
		d.metrics.Custom["fan_speed"] = 1000 + 2000*d.randomFloat()
	case ProfileConstrained:
		d.metrics.Custom["battery_level"] = 20 + 80*d.randomFloat()
		d.metrics.Custom["signal_strength"] = -60 - 30*d.randomFloat()
	case ProfileMinimal:
		d.metrics.Custom["sensor_reading"] = 100 + 900*d.randomFloat()
	}

	// Common metrics
	d.metrics.Custom["device_uptime"] = d.metrics.System.Uptime.Seconds()
	d.metrics.Custom["timestamp"] = timestamp.Unix()
}

// sendMetrics sends metrics to the server
func (d *VirtualDevice) sendMetrics() error {
	// This would integrate with the actual fleetd API
	d.logger.Debug("Sending metrics",
		"cpu_usage", d.metrics.CPU.UsagePercent,
		"memory_usage", d.metrics.Memory.UsedPercent,
	)

	// Simulate network delay
	delay := time.Duration(10+d.randomFloat()*40) * time.Millisecond
	time.Sleep(delay)

	return nil
}

// sendHeartbeat sends a heartbeat to the server
func (d *VirtualDevice) sendHeartbeat() error {
	d.logger.Debug("Sending heartbeat", "status", d.state.Status)

	// Simulate network delay
	delay := time.Duration(5+d.randomFloat()*20) * time.Millisecond
	time.Sleep(delay)

	return nil
}

// updateState updates the device state
func (d *VirtualDevice) updateState() {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Simulate state transitions
	if d.randomFloat() < 0.001 { // 0.1% chance per update
		switch d.state.Status {
		case publicv1.DeviceStatus_DEVICE_STATUS_ONLINE:
			if d.randomFloat() < 0.5 {
				d.state.Status = publicv1.DeviceStatus_DEVICE_STATUS_UPDATING
				d.state.Updating = true
			} else {
				d.state.Status = publicv1.DeviceStatus_DEVICE_STATUS_MAINTENANCE
			}
		case publicv1.DeviceStatus_DEVICE_STATUS_UPDATING:
			d.state.Status = publicv1.DeviceStatus_DEVICE_STATUS_ONLINE
			d.state.Updating = false
		case publicv1.DeviceStatus_DEVICE_STATUS_MAINTENANCE:
			d.state.Status = publicv1.DeviceStatus_DEVICE_STATUS_ONLINE
		}
	}
}

// simulateRandomErrors occasionally triggers error conditions
func (d *VirtualDevice) simulateRandomErrors() {
	if d.randomFloat() < 0.01 { // 1% chance per minute
		d.logger.Warn("Simulating temporary error condition")
		d.state.Status = publicv1.DeviceStatus_DEVICE_STATUS_ERROR

		// Recover after a short time
		go func() {
			time.Sleep(time.Duration(5+d.randomFloat()*25) * time.Second)
			d.mu.Lock()
			if d.state.Status == publicv1.DeviceStatus_DEVICE_STATUS_ERROR {
				d.state.Status = publicv1.DeviceStatus_DEVICE_STATUS_ONLINE
			}
			d.mu.Unlock()
		}()
	}
}

// shouldSimulateError determines if an error should be simulated
func (d *VirtualDevice) shouldSimulateError() bool {
	return d.randomFloat() < d.Config.ErrorRate
}

// addJitter adds random jitter to a duration
func (d *VirtualDevice) addJitter(duration time.Duration) time.Duration {
	jitter := float64(duration) * d.Config.JitterPercent * (d.randomFloat() - 0.5) * 2
	return duration + time.Duration(jitter)
}

// randomFloat returns a random float between 0 and 1
func (d *VirtualDevice) randomFloat() float64 {
	n, _ := rand.Int(rand.Reader, big.NewInt(1000000))
	return float64(n.Int64()) / 1000000.0
}

// GetState returns the current device state
func (d *VirtualDevice) GetState() DeviceState {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.state
}

// GetMetrics returns a copy of current metrics
func (d *VirtualDevice) GetMetrics() DeviceMetrics {
	d.metrics.mu.RLock()
	defer d.metrics.mu.RUnlock()

	// Deep copy without mutex
	custom := make(map[string]interface{})
	for k, v := range d.metrics.Custom {
		custom[k] = v
	}

	return DeviceMetrics{
		CPU:     d.metrics.CPU,
		Memory:  d.metrics.Memory,
		Disk:    d.metrics.Disk,
		Network: d.metrics.Network,
		System:  d.metrics.System,
		Custom:  custom,
	}
}

// IsStarted returns whether the device is currently running
func (d *VirtualDevice) IsStarted() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.started
}
