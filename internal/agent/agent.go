package agent

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net"
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
	"github.com/shirou/gopsutil/v3/mem"
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
	listener   net.Listener
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
	if err := os.MkdirAll(filepath.Join(a.cfg.StorageDir, "state"), 0755); err != nil {
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
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", a.cfg.RPCPort))
	if err != nil {
		return fmt.Errorf("failed to create listener: %w", err)
	}
	a.listener = listener

	// Get the actual port if it was dynamically allocated
	actualPort := listener.Addr().(*net.TCPAddr).Port

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
