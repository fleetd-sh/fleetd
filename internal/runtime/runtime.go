package runtime

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

// Enhanced Runtime implementation
type Runtime struct {
	mu        sync.RWMutex
	processes map[string]*managedProcess
	baseDir   string
	logger    *slog.Logger
}

type managedProcess struct {
	process *os.Process
	cmd     *exec.Cmd
	cancel  context.CancelFunc
	health  *health
	logs    *logManager
	stats   *resourceStats
}

type Config struct {
	MaxLogSize    int64           // Maximum size of log files in bytes
	LogRotateKeep int             // Number of rotated log files to keep
	HealthCheck   *HealthConfig   // Health check configuration
	Resources     *ResourceConfig // Resource limits
}

type HealthConfig struct {
	Interval    time.Duration // How often to check health
	MaxFailures int           // Maximum failures before restart
	Timeout     time.Duration // Timeout for health checks
	URL         string        // URL to check
}

type ResourceConfig struct {
	MaxCPU    float64 // Percentage (0-100)
	MaxMemory uint64  // Bytes
	MaxDisk   uint64  // Bytes
}

type health struct {
	lastCheck   time.Time
	status      string
	failures    int
	checker     HealthChecker
	maxFailures int
}

type HealthChecker interface {
	Check(context.Context) error
}

type logManager struct {
	stdout     *os.File
	stderr     *os.File
	maxSize    int64
	keepFiles  int
	logDir     string
	currentGen int
}

type resourceStats struct {
	cpu       float64
	memory    uint64
	diskUsage uint64
	limits    *ResourceConfig
}

// New creates a new runtime manager
func New(baseDir string) (*Runtime, error) {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create base directory: %w", err)
	}

	return &Runtime{
		processes: make(map[string]*managedProcess),
		baseDir:   baseDir,
		logger: slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})),
	}, nil
}

// Deploy installs a new binary
func (r *Runtime) Deploy(name string, binary io.Reader) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.logger.Info("Starting binary deployment", "name", name)

	binPath := filepath.Join(r.baseDir, name)
	r.logger.Debug("Binary path", "path", binPath)

	// Create temporary file
	tmpPath := binPath + ".tmp"
	r.logger.Debug("Creating temporary file", "path", tmpPath)

	f, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %w", err)
	}
	defer f.Close()

	// Copy binary data
	r.logger.Debug("Copying binary data")
	written, err := io.Copy(f, binary)
	if err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to write binary: %w", err)
	}
	r.logger.Debug("Binary data copied", "bytes", written)

	// Ensure all data is written to disk
	if err := f.Sync(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to sync binary data: %w", err)
	}

	// Close the file before renaming
	if err := f.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to close temporary file: %w", err)
	}

	// Rename temporary file to final location
	r.logger.Debug("Renaming temporary file to final location")
	if err := os.Rename(tmpPath, binPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to install binary: %w", err)
	}

	r.logger.Info("Binary deployment completed", "name", name)
	return nil
}

// Start launches a deployed binary
func (r *Runtime) Start(name string, args []string, config *Config) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// if no health config, set defaults
	if config.HealthCheck == nil {
		config.HealthCheck = &HealthConfig{
			Interval:    1 * time.Second,
			Timeout:     5 * time.Second,
			MaxFailures: 3,
		}
	}

	binPath := filepath.Join(r.baseDir, name)
	if _, err := os.Stat(binPath); err != nil {
		return fmt.Errorf("binary not found: %w", err)
	}

	// Setup logging
	logManager, err := newLogManager(name, r.baseDir, config.MaxLogSize, config.LogRotateKeep)
	if err != nil {
		return fmt.Errorf("failed to setup logging: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, binPath, args...)
	cmd.Stdout = logManager.stdout
	cmd.Stderr = logManager.stderr

	if err := cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("failed to start process: %w", err)
	}

	proc := &managedProcess{
		process: cmd.Process,
		cmd:     cmd,
		cancel:  cancel,
		health: &health{
			checker: &HTTPHealthChecker{
				URL:     config.HealthCheck.URL,
				Timeout: config.HealthCheck.Timeout,
			},
			maxFailures: config.HealthCheck.MaxFailures,
		},
		logs:  logManager,
		stats: &resourceStats{limits: config.Resources},
	}

	r.processes[name] = proc

	// Start monitoring goroutines
	go r.monitorResources(ctx, name, proc)
	go r.monitorHealth(ctx, name, proc)

	// Monitor process
	go func() {
		cmd.Wait()
		r.mu.Lock()
		delete(r.processes, name)
		r.mu.Unlock()
	}()

	return nil
}

// Stop terminates a running binary
func (r *Runtime) Stop(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	procs := len(r.processes)

	r.logger.With("name", name, "procs", procs).Info("Stopping process")

	proc, exists := r.processes[name]
	if !exists {
		// Process might have already exited and been cleaned up
		return nil
	}

	if proc.cancel != nil {
		proc.cancel()
	}

	// Remove from map (the monitoring goroutine might also do this,
	// but delete is safe to call multiple times)
	delete(r.processes, name)

	return nil
}

// List returns all deployed binaries
func (r *Runtime) List() ([]string, error) {
	entries, err := os.ReadDir(r.baseDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read runtime directory: %w", err)
	}

	binaries := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			binaries = append(binaries, entry.Name())
		}
	}

	return binaries, nil
}

// IsRunning checks if a binary is currently running
func (r *Runtime) IsRunning(name string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	proc, exists := r.processes[name]
	if !exists {
		return false, nil
	}

	// Check if process exists and is running
	if proc.cmd == nil || proc.cmd.Process == nil {
		return false, nil
	}

	// Try to get process state
	if err := proc.cmd.Process.Signal(syscall.Signal(0)); err != nil {
		// Process is not running
		return false, nil
	}

	return true, nil
}

// GetProcess returns the process for a given binary name
func (r *Runtime) GetProcess(name string) *os.Process {
	r.mu.Lock()
	defer r.mu.Unlock()

	proc, ok := r.processes[name]
	if !ok {
		return nil
	}
	return proc.cmd.Process
}
