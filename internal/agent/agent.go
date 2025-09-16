package agent

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/slog"
	stdnet "net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	agentrpc "fleetd.sh/gen/agent/v1/agentpbconnect"
	"fleetd.sh/internal/discovery"
	rt "fleetd.sh/internal/runtime"
	"fleetd.sh/internal/state"
	"fleetd.sh/internal/update"
	"fleetd.sh/pkg/telemetry"
	"fleetd.sh/pkg/telemetry/handlers"
	"fleetd.sh/pkg/telemetry/sources"

	"github.com/google/uuid"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/process"
)

// Agent represents the main fleetd device agent
type Agent struct {
	cfg        *Config
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	started    bool
	mu         sync.RWMutex
	discovery  *discovery.Discovery
	runtime    *rt.Runtime
	telemetry  *telemetry.Collector
	updater    *update.Updater
	state      *state.Manager
	statePath  string
	deviceInfo *DeviceInfo
	config     *Configuration
	ready      chan struct{}
	server     *http.Server
	listener   stdnet.Listener
}

// New creates a new Agent instance
func New(cfg *Config) *Agent {
	ctx, cancel := context.WithCancel(context.Background())

	return &Agent{
		cfg:    cfg,
		ctx:    ctx,
		cancel: cancel,
		ready:  make(chan struct{}),
		deviceInfo: &DeviceInfo{
			DeviceID:   cfg.DeviceID,
			DeviceType: runtime.GOARCH,
			Version:    "0.1.0", // TODO: Make this configurable
		},
	}
}

// Start initializes and starts all agent components
func (a *Agent) Start() error {
	// Configure logging
	logHandler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level:     slog.LevelDebug,
		AddSource: true,
	})
	slog.SetDefault(slog.New(logHandler))

	slog.With("cfg", a.cfg).Info("Starting agent")
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.started {
		return nil
	}

	// Ensure directories exist
	if err := os.MkdirAll(filepath.Join(a.cfg.StorageDir, "state"), 0o755); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	// Initialize all components
	var err error

	// Initialize state manager
	a.state, err = state.New(filepath.Join(a.cfg.StorageDir, "state", "state.json"))
	if err != nil {
		return fmt.Errorf("failed to initialize state manager: %w", err)
	}

	// Initialize runtime
	a.runtime, err = rt.New(filepath.Join(a.cfg.StorageDir, "runtime"))
	if err != nil {
		return fmt.Errorf("failed to initialize runtime: %w", err)
	}

	// Initialize device info
	err = a.state.Update(func(s *state.State) error {
		if s.DeviceInfo.ID == "" {
			s.DeviceInfo = state.DeviceInfo{
				ID:            a.cfg.DeviceID,
				Hardware:      runtime.GOARCH,
				Architecture:  runtime.GOARCH,
				OSInfo:        runtime.GOOS,
				FirstSeenTime: time.Now(),
				Tags:          make(map[string]string),
			}
		}
		s.LastStartTime = time.Now()

		// Check if we need to register with fleet server
		if a.cfg.ServerURL != "" && s.DeviceInfo.APIKey == "" {
			slog.Info("Registering device with fleet server", "server", a.cfg.ServerURL)
			requestID := fmt.Sprintf("reg-%s-%d", a.cfg.DeviceID[:8], time.Now().Unix())
			regClient := NewRegistrationClient(a.cfg.ServerURL, requestID)

			// Use hostname as device name if not configured
			deviceName := a.cfg.DeviceName
			if deviceName == "" {
				if hostname, err := os.Hostname(); err == nil {
					deviceName = hostname
				} else {
					deviceName = "fleetd-" + a.cfg.DeviceID[:8]
				}
			}

			regResp, err := regClient.RegisterDevice(
				context.Background(),
				deviceName,
				runtime.GOARCH,
				"0.1.0", // TODO: Get from build info
			)
			if err != nil {
				slog.Error("Failed to register device", "error", err)
				// Continue without registration - device can work offline
			} else {
				// Update state with registration info
				s.DeviceInfo.ID = regResp.DeviceId
				s.DeviceInfo.APIKey = regResp.ApiKey
				s.DeviceInfo.Configured = true

				// Update agent's device info
				a.deviceInfo.DeviceID = regResp.DeviceId
				a.deviceInfo.APIKey = regResp.ApiKey
				a.deviceInfo.Configured = true

				slog.Info("Device registered successfully", "device_id", regResp.DeviceId)
			}
		} else if s.DeviceInfo.APIKey != "" {
			// Restore device info from state
			a.deviceInfo.DeviceID = s.DeviceInfo.ID
			a.deviceInfo.APIKey = s.DeviceInfo.APIKey
			a.deviceInfo.Configured = s.DeviceInfo.Configured
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to initialize device info: %w", err)
	}

	// Initialize other components
	// Discovery will be initialized later after we know the actual RPC port
	a.telemetry = telemetry.New(time.Duration(a.cfg.TelemetryInterval) * time.Second)

	// Add telemetry sources and handlers
	systemStats := sources.NewSystemStats()
	a.telemetry.AddSource(systemStats)

	telemetryPath := filepath.Join(a.cfg.StorageDir, "telemetry", "metrics.json")
	localHandler, err := handlers.NewLocalFile(telemetryPath)
	if err != nil {
		return fmt.Errorf("failed to initialize telemetry handler: %w", err)
	}
	a.telemetry.AddHandler(localHandler)

	// Initialize daemon service
	service := NewDaemonService(a)

	// Initialize and start RPC server
	mux := http.NewServeMux()
	// Add the daemon service handler
	mux.Handle(agentrpc.NewDaemonServiceHandler(service))
	// Add the discovery service handler
	discoveryService := NewDiscoveryService(a)
	path, handler := agentrpc.NewDiscoveryServiceHandler(discoveryService)
	mux.Handle(path, handler)

	// Create listener - bind to all interfaces
	listener, err := stdnet.Listen("tcp", fmt.Sprintf(":%d", a.cfg.RPCPort))
	if err != nil {
		return fmt.Errorf("failed to create listener: %w", err)
	}
	a.listener = listener

	// Get the actual port if it was dynamically allocated
	actualPort := listener.Addr().(*stdnet.TCPAddr).Port

	// Update discovery with actual port and start it if not disabled
	if !a.cfg.DisableMDNS {
		a.discovery = discovery.New(a.cfg.DeviceID, actualPort, a.cfg.ServiceType)
		if err := a.discovery.Start(); err != nil {
			return fmt.Errorf("failed to start discovery: %w", err)
		}
	}

	// Create server
	a.server = &http.Server{
		Handler: mux,
	}

	// Start server in goroutine
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		slog.Info("Starting RPC server", "address", listener.Addr().String())
		if err := a.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			slog.Error("RPC server error", "error", err)
		}
	}()

	// Start core services
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		a.runMainLoop()
	}()

	// Start telemetry collection
	a.telemetry.Start()

	// Signal that agent is ready
	close(a.ready)
	a.started = true
	slog.Info("Agent started")
	return nil
}

// Stop gracefully shuts down the agent
func (a *Agent) Stop() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.started {
		return nil
	}

	// Shutdown RPC server first
	if a.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := a.server.Shutdown(ctx); err != nil {
			slog.Error("Error shutting down RPC server", "error", err)
		}
	}

	// Stop discovery if it was initialized
	if a.discovery != nil {
		if err := a.discovery.Stop(); err != nil {
			log.Printf("Error stopping discovery: %v", err)
		}
	}

	// Stop telemetry
	a.telemetry.Stop()

	a.cancel()
	a.wg.Wait()
	a.started = false
	return nil
}

func (a *Agent) runMainLoop() {
	// Main agent loop
	for {
		select {
		case <-a.ctx.Done():
			return
		default:
			// Core agent operations will go here
		}
	}
}

// Update performs a self-update of the agent
func (a *Agent) Update(binary io.Reader, info update.UpdateInfo) error {
	if a.updater == nil {
		return fmt.Errorf("update support not available")
	}

	// Stop all services before update
	if err := a.Stop(); err != nil {
		return fmt.Errorf("failed to stop services for update: %w", err)
	}

	// Perform update
	if err := a.updater.Update(context.Background(), binary, info); err != nil {
		return fmt.Errorf("update failed: %w", err)
	}

	// Cleanup update files
	if err := a.updater.Cleanup(); err != nil {
		log.Printf("Failed to cleanup update files: %v", err)
	}

	// Restart agent
	if err := a.Start(); err != nil {
		return fmt.Errorf("failed to restart agent after update: %w", err)
	}

	return nil
}

// BinaryInfo represents the status of a deployed binary
type BinaryInfo struct {
	Name      string    `json:"name"`
	Version   string    `json:"version"`
	Status    string    `json:"status"`
	StartedAt time.Time `json:"started_at,omitempty"`
}

// UpdateBinaryState updates the state of a binary in the agent's state store
func (a *Agent) UpdateBinaryState(name, version, status string) error {
	return a.state.Update(func(s *state.State) error {
		if s.RuntimeState.DeployedBinaries == nil {
			s.RuntimeState.DeployedBinaries = make(map[string]state.BinaryInfo)
		}
		s.RuntimeState.DeployedBinaries[name] = state.BinaryInfo{
			Version:    version,
			Status:     status,
			DeployedAt: time.Now(),
		}
		return nil
	})
}

// ListBinaries returns information about all deployed binaries
func (a *Agent) ListBinaries() ([]BinaryInfo, error) {
	state := a.state.Get()
	binaries := make([]BinaryInfo, 0)

	// Get runtime status for each binary
	for name, info := range state.RuntimeState.DeployedBinaries {
		status := info.Status
		if a.runtime != nil {
			// Check actual runtime status
			if running, _ := a.runtime.IsRunning(name); running {
				status = "running"
			}
		}

		binaries = append(binaries, BinaryInfo{
			Name:      name,
			Version:   info.Version,
			Status:    status,
			StartedAt: info.LastStarted,
		})
	}

	return binaries, nil
}

// DeployBinary deploys a new binary to the agent
func (a *Agent) DeployBinary(name string, data []byte) error {
	if a.runtime == nil {
		return fmt.Errorf("runtime support not available")
	}

	slog.Info("Agent received deploy binary request",
		"name", name,
		"size", len(data),
		"runtime_initialized", a.runtime != nil)

	// Create a new reader from the byte slice - no need to pre-read
	reader := bytes.NewReader(data)

	// Deploy binary through runtime
	if err := a.runtime.Deploy(name, reader); err != nil {
		slog.Error("Runtime deploy failed",
			"error", err,
			"name", name)
		return fmt.Errorf("failed to deploy binary: %w", err)
	}

	slog.Info("Runtime deploy successful, updating state",
		"name", name)

	// Update state
	if err := a.UpdateBinaryState(name, "unknown", "deployed"); err != nil {
		slog.Error("Failed to update binary state",
			"error", err,
			"name", name)
		return fmt.Errorf("failed to update binary state: %w", err)
	}

	slog.Info("Binary deployment completed successfully",
		"name", name)
	return nil
}

// StartBinary starts a deployed binary
func (a *Agent) StartBinary(name string, args []string) error {
	if a.runtime == nil {
		return fmt.Errorf("runtime support not available")
	}

	// Start binary through runtime
	if err := a.runtime.Start(name, args, &rt.Config{
		HealthCheck: &rt.HealthConfig{
			Interval:    1 * time.Second,
			Timeout:     5 * time.Second,
			MaxFailures: 3,
		},
	}); err != nil {
		return fmt.Errorf("failed to start binary: %w", err)
	}

	// Update state
	return a.state.Update(func(s *state.State) error {
		if s.RuntimeState.DeployedBinaries == nil {
			s.RuntimeState.DeployedBinaries = make(map[string]state.BinaryInfo)
		}
		binary := s.RuntimeState.DeployedBinaries[name]
		binary.Status = "running"
		binary.LastStarted = time.Now()
		s.RuntimeState.DeployedBinaries[name] = binary
		return nil
	})
}

// StopBinary stops a running binary
func (a *Agent) StopBinary(name string) error {
	if a.runtime == nil {
		return fmt.Errorf("runtime support not available")
	}

	// Stop binary through runtime
	if err := a.runtime.Stop(name); err != nil {
		return fmt.Errorf("failed to stop binary: %w", err)
	}

	// Update state
	return a.state.Update(func(s *state.State) error {
		if s.RuntimeState.DeployedBinaries == nil {
			return nil
		}
		if binary, exists := s.RuntimeState.DeployedBinaries[name]; exists {
			binary.Status = "stopped"
			s.RuntimeState.DeployedBinaries[name] = binary
		}
		return nil
	})
}

// RecordUpdate records the result of an agent update
func (a *Agent) RecordUpdate(version string, success bool, errorDetail string) error {
	return a.state.Update(func(s *state.State) error {
		record := state.UpdateRecord{
			Version:     version,
			UpdatedAt:   time.Now(),
			Success:     success,
			ErrorDetail: errorDetail,
		}
		s.UpdateHistory = append(s.UpdateHistory, record)
		return nil
	})
}

// State returns the agent's state manager
func (a *Agent) State() *state.Manager {
	return a.state
}

// Runtime returns the agent's runtime manager
func (a *Agent) Runtime() *rt.Runtime {
	return a.runtime
}

// Add these types to support device info and stats
type DeviceInfo struct {
	DeviceID   string
	Configured bool
	DeviceType string
	Version    string
	APIKey     string
}

type SystemStats struct {
	CPUUsage    float64
	MemoryTotal uint64
	MemoryUsed  uint64
	DiskTotal   uint64
	DiskUsed    uint64
}

// CollectSystemInfo gathers comprehensive system information for device registration
func CollectSystemInfo() (*SystemInfo, error) {
	info := &SystemInfo{
		Extra: make(map[string]string),
	}

	// Get host info
	hostInfo, err := host.Info()
	if err != nil {
		return nil, fmt.Errorf("failed to get host info: %w", err)
	}
	info.Hostname = hostInfo.Hostname
	info.OS = hostInfo.OS
	info.Platform = hostInfo.Platform
	info.OSVersion = hostInfo.PlatformVersion
	info.KernelVersion = hostInfo.KernelVersion
	info.Arch = hostInfo.KernelArch

	// Get CPU info
	cpuInfo, err := cpu.Info()
	if err != nil {
		return nil, fmt.Errorf("failed to get CPU info: %w", err)
	}
	if len(cpuInfo) > 0 {
		info.CPUModel = cpuInfo[0].ModelName
		info.CPUCores = int32(cpuInfo[0].Cores)
		// Add CPU vendor as extra info
		info.Extra["cpu_vendor"] = cpuInfo[0].VendorID
		info.Extra["cpu_family"] = cpuInfo[0].Family
	}

	// Get logical CPU count
	logicalCores, err := cpu.Counts(true)
	if err == nil {
		info.Extra["cpu_logical_cores"] = fmt.Sprintf("%d", logicalCores)
	}

	// Get memory info
	memInfo, err := mem.VirtualMemory()
	if err != nil {
		return nil, fmt.Errorf("failed to get memory info: %w", err)
	}
	info.MemoryTotal = memInfo.Total

	// Get disk info
	diskInfo, err := disk.Usage("/")
	if err != nil {
		return nil, fmt.Errorf("failed to get disk info: %w", err)
	}
	info.StorageTotal = diskInfo.Total

	// Get network interfaces
	interfaces, err := stdnet.Interfaces()
	if err == nil {
		for _, iface := range interfaces {
			netIface := NetworkInterface{
				Name:       iface.Name,
				MACAddress: iface.HardwareAddr.String(),
				IsUp:       iface.Flags&stdnet.FlagUp != 0,
				IsLoopback: iface.Flags&stdnet.FlagLoopback != 0,
				MTU:        uint64(iface.MTU),
			}

			// Get IP addresses for this interface
			addrs, err := iface.Addrs()
			if err == nil {
				for _, addr := range addrs {
					netIface.IPAddresses = append(netIface.IPAddresses, addr.String())
				}
			}

			info.NetworkInterfaces = append(info.NetworkInterfaces, netIface)
		}
	}

	// Get timezone
	info.Timezone = time.Local.String()

	// Set agent version (TODO: Get from build info)
	info.AgentVersion = "0.1.0"

	// Get system load averages
	loadAvg, err := load.Avg()
	if err == nil {
		info.LoadAverage = LoadAverage{
			Load1:  loadAvg.Load1,
			Load5:  loadAvg.Load5,
			Load15: loadAvg.Load15,
		}
	}

	// Get process count
	processes, err := process.Processes()
	if err == nil {
		info.ProcessCount = int32(len(processes))
	}

	// Try to get BIOS info from hostInfo
	// Note: gopsutil doesn't directly provide BIOS info on all platforms
	// This would need platform-specific implementation for full support
	// For now, we'll extract what we can from host info
	if runtime.GOOS == "linux" {
		// On Linux, try to read from DMI
		if vendor, err := os.ReadFile("/sys/class/dmi/id/bios_vendor"); err == nil {
			info.BiosInfo.Vendor = string(bytes.TrimSpace(vendor))
		}
		if version, err := os.ReadFile("/sys/class/dmi/id/bios_version"); err == nil {
			info.BiosInfo.Version = string(bytes.TrimSpace(version))
		}
		if date, err := os.ReadFile("/sys/class/dmi/id/bios_date"); err == nil {
			info.BiosInfo.ReleaseDate = string(bytes.TrimSpace(date))
		}

		// Get product info
		if product, err := os.ReadFile("/sys/class/dmi/id/product_name"); err == nil {
			info.ProductName = string(bytes.TrimSpace(product))
		}
		if vendor, err := os.ReadFile("/sys/class/dmi/id/sys_vendor"); err == nil {
			info.Manufacturer = string(bytes.TrimSpace(vendor))
		}
		if serial, err := os.ReadFile("/sys/class/dmi/id/product_serial"); err == nil {
			info.SerialNumber = string(bytes.TrimSpace(serial))
		}
	} else if runtime.GOOS == "darwin" {
		// On macOS, we can use system_profiler but it's slow
		// For now, use what's available from host info
		info.Manufacturer = "Apple Inc."
		// Serial number would require system_profiler SPHardwareDataType
	}

	// Add additional useful info
	info.Extra["boot_time"] = fmt.Sprintf("%d", hostInfo.BootTime)
	info.Extra["uptime"] = fmt.Sprintf("%d", hostInfo.Uptime)
	info.Extra["virtualization_system"] = hostInfo.VirtualizationSystem
	info.Extra["virtualization_role"] = hostInfo.VirtualizationRole
	info.Extra["host_id"] = hostInfo.HostID // Unique machine ID

	// Add Go runtime info
	info.Extra["go_version"] = runtime.Version()
	info.Extra["go_arch"] = runtime.GOARCH
	info.Extra["go_os"] = runtime.GOOS

	// Add number of CPUs
	info.Extra["num_cpu"] = fmt.Sprintf("%d", runtime.NumCPU())

	return info, nil
}

// SystemInfo represents comprehensive system information
type SystemInfo struct {
	Hostname      string
	OS            string
	OSVersion     string
	Arch          string
	CPUModel      string
	CPUCores      int32
	MemoryTotal   uint64
	StorageTotal  uint64
	KernelVersion string
	Platform      string
	Extra         map[string]string

	// Network information
	NetworkInterfaces []NetworkInterface

	// System identification
	Timezone     string
	AgentVersion string
	SerialNumber string
	ProductName  string
	Manufacturer string

	// Runtime metrics
	LoadAverage  LoadAverage
	ProcessCount int32

	// BIOS/Firmware
	BiosInfo BiosInfo
}

type NetworkInterface struct {
	Name        string
	MACAddress  string
	IPAddresses []string
	IsUp        bool
	IsLoopback  bool
	MTU         uint64
}

type LoadAverage struct {
	Load1  float64
	Load5  float64
	Load15 float64
}

type BiosInfo struct {
	Vendor      string
	Version     string
	ReleaseDate string
}

// GetSystemStats collects current system statistics
func GetSystemStats() (*SystemStats, error) {
	stats := &SystemStats{}

	// Get CPU usage
	cpuPercent, err := cpu.Percent(time.Second, false)
	if err != nil {
		return nil, fmt.Errorf("failed to get CPU usage: %w", err)
	}
	if len(cpuPercent) > 0 {
		stats.CPUUsage = cpuPercent[0]
	}

	// Get memory stats
	memInfo, err := mem.VirtualMemory()
	if err != nil {
		return nil, fmt.Errorf("failed to get memory stats: %w", err)
	}
	stats.MemoryTotal = memInfo.Total
	stats.MemoryUsed = memInfo.Used

	// Get disk stats
	diskInfo, err := disk.Usage("/")
	if err != nil {
		return nil, fmt.Errorf("failed to get disk stats: %w", err)
	}
	stats.DiskTotal = diskInfo.Total
	stats.DiskUsed = diskInfo.Used

	return stats, nil
}

type Configuration struct {
	DeviceName  string
	APIEndpoint string
}

// GetDeviceInfo returns a copy of the current device info
func (a *Agent) GetDeviceInfo() DeviceInfo {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return *a.deviceInfo
}

// GetSystemStats returns the current system statistics
func (a *Agent) GetSystemStats() SystemStats {
	stats := SystemStats{}

	if cpu, err := cpu.Percent(0, false); err == nil && len(cpu) > 0 {
		stats.CPUUsage = cpu[0]
	}

	if mem, err := mem.VirtualMemory(); err == nil {
		stats.MemoryTotal = mem.Total
		stats.MemoryUsed = mem.Used
	}

	if disk, err := disk.Usage("/"); err == nil {
		stats.DiskTotal = disk.Total
		stats.DiskUsed = disk.Used
	}

	return stats
}

// Configure updates the agent's configuration and persists it
func (a *Agent) Configure(cfg Configuration) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.configure(cfg)
}

// UpdateConfigFromJSON updates configuration from a JSON string
func (a *Agent) UpdateConfigFromJSON(configJSON string) error {
	var config map[string]any
	if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
		return fmt.Errorf("failed to parse config JSON: %w", err)
	}

	// Build configuration from JSON
	cfg := Configuration{}

	if serverURL, ok := config["server_url"].(string); ok {
		cfg.APIEndpoint = serverURL
		a.cfg.ServerURL = serverURL
	}

	if deviceName, ok := config["device_name"].(string); ok {
		cfg.DeviceName = deviceName
	}

	if apiKey, ok := config["api_key"].(string); ok {
		// Store API key if provided
		a.deviceInfo.APIKey = apiKey
	}

	// Apply configuration if we have at least a server URL
	if cfg.APIEndpoint != "" {
		return a.Configure(cfg)
	}

	return nil
}

// configure is the internal implementation of configuration updates
func (a *Agent) configure(cfg Configuration) error {
	// Validate configuration
	if cfg.APIEndpoint == "" {
		return fmt.Errorf("api endpoint is required")
	}

	// Update internal state
	a.config = &cfg

	if a.deviceInfo == nil {
		a.deviceInfo = &DeviceInfo{}
	}

	a.deviceInfo.Configured = true
	if a.deviceInfo.DeviceID == "" {
		a.deviceInfo.DeviceID = generateDeviceID()
	}
	a.deviceInfo.APIKey = generateAPIKey()

	// Persist state
	if a.state != nil {
		if err := a.state.Update(func(s *state.State) error {
			s.DeviceInfo.ID = a.deviceInfo.DeviceID
			// Update other state fields as needed
			return nil
		}); err != nil {
			return fmt.Errorf("failed to persist configuration: %w", err)
		}
	}

	return nil
}

// Helper functions
func generateDeviceID() string {
	// Generate a UUID v4
	id := uuid.New()
	return id.String()
}

func generateAPIKey() string {
	// Generate a random 32-byte key and base64 encode it
	key := make([]byte, 32)
	rand.Read(key)
	return base64.URLEncoding.EncodeToString(key)
}

func (a *Agent) WaitForReady(ctx context.Context) error {
	select {
	case <-a.ready:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// RPCAddr returns the actual address of the RPC server
func (a *Agent) RPCAddr() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.listener == nil {
		return ""
	}
	return a.listener.Addr().String()
}
