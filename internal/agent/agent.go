package agent

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"fleetd.sh/internal/discovery"
	rt "fleetd.sh/internal/runtime"
	"fleetd.sh/internal/state"
	"fleetd.sh/internal/update"
	"fleetd.sh/pkg/telemetry"
	"fleetd.sh/pkg/telemetry/handlers"
	"fleetd.sh/pkg/telemetry/sources"
)

// Agent represents the main fleetd device agent
type Agent struct {
	cfg       *Config
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	started   bool
	mu        sync.Mutex
	discovery *discovery.Discovery
	runtime   *rt.Runtime
	telemetry *telemetry.Collector
	updater   *update.Updater
	state     *state.Manager
	statePath string
}

// New creates a new Agent instance
func New(cfg *Config) *Agent {
	ctx, cancel := context.WithCancel(context.Background())

	rt, err := rt.New(filepath.Join(cfg.LocalStoragePath, "runtime"))
	if err != nil {
		log.Printf("Failed to initialize runtime: %v", err)
		// Continue without runtime support
		rt = nil
	}

	// Initialize telemetry
	collector := telemetry.New(time.Duration(cfg.TelemetryInterval) * time.Second)

	// Add system stats source
	collector.AddSource(sources.NewSystemStats())

	// Add local file handler
	if cfg.LocalStoragePath != "" {
		localHandler, err := handlers.NewLocalFile(
			filepath.Join(cfg.LocalStoragePath, "telemetry.json"),
		)
		if err != nil {
			log.Printf("Failed to initialize local telemetry handler: %v", err)
		} else {
			collector.AddHandler(localHandler)
		}
	}

	// Initialize updater
	updater, err := update.New(filepath.Join(cfg.LocalStoragePath, "updates"))
	if err != nil {
		log.Printf("Failed to initialize updater: %v", err)
		// Continue without update support
		updater = nil
	}

	// Initialize state manager
	stateManager, err := state.New(filepath.Join(cfg.LocalStoragePath, "state", "state.json"))
	if err != nil {
		log.Fatalf("Failed to initialize state manager: %v", err)
	}

	// Initialize device info if needed
	if err := stateManager.Update(func(s *state.State) error {
		if s.DeviceInfo.ID == "" {
			s.DeviceInfo = state.DeviceInfo{
				ID:            cfg.DeviceID,
				Hardware:      runtime.GOARCH,
				Architecture:  runtime.GOARCH,
				OSInfo:        runtime.GOOS,
				FirstSeenTime: time.Now(),
				Tags:          make(map[string]string),
			}
		}
		s.LastStartTime = time.Now()
		return nil
	}); err != nil {
		log.Printf("Failed to initialize device info: %v", err)
	}

	return &Agent{
		cfg:       cfg,
		ctx:       ctx,
		cancel:    cancel,
		discovery: discovery.New(cfg.DeviceID, cfg.MDNSPort),
		runtime:   rt,
		telemetry: collector,
		updater:   updater,
		state:     stateManager,
		statePath: filepath.Join(cfg.LocalStoragePath, "state", "state.json"),
	}
}

// Start initializes and starts all agent components
func (a *Agent) Start() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.started {
		return nil
	}

	// Ensure directories exist
	if err := os.MkdirAll(filepath.Dir(a.statePath), 0755); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	// Initialize state manager
	stateManager, err := state.New(a.statePath)
	if err != nil {
		return fmt.Errorf("failed to initialize state manager: %w", err)
	}
	a.state = stateManager

	// Initialize runtime
	runtime, err := rt.New(filepath.Join(a.cfg.LocalStoragePath, "runtime"))
	if err != nil {
		return fmt.Errorf("failed to initialize runtime: %w", err)
	}
	a.runtime = runtime

	// Initialize device info
	err = a.state.Update(func(s *state.State) error {
		s.DeviceInfo = state.DeviceInfo{
			ID:            a.cfg.DeviceID,
			FirstSeenTime: time.Now(),
			Tags:          make(map[string]string),
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to initialize device info: %w", err)
	}

	// Start discovery if enabled
	if a.cfg.EnableMDNS {
		if err := a.discovery.Start(); err != nil {
			return fmt.Errorf("failed to start discovery: %w", err)
		}
	}

	// Start core services
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		a.runMainLoop()
	}()

	// Start telemetry collection
	a.telemetry.Start()

	a.started = true
	return nil
}

// Stop gracefully shuts down the agent
func (a *Agent) Stop() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.started {
		return nil
	}

	// Stop discovery
	if a.cfg.EnableMDNS {
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
func (a *Agent) DeployBinary(name string, reader io.Reader) error {
	if a.runtime == nil {
		return fmt.Errorf("runtime support not available")
	}

	// Deploy binary through runtime
	if err := a.runtime.Deploy(name, reader); err != nil {
		return fmt.Errorf("failed to deploy binary: %w", err)
	}

	// Update state
	return a.UpdateBinaryState(name, "unknown", "deployed")
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
