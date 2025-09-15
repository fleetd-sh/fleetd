package process

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	pb "fleetd.sh/gen/fleetd/v1"
	"fleetd.sh/internal/ferrors"
	"github.com/shirou/gopsutil/v3/process"
)

// Config holds configuration for the process manager
type Config struct {
	RuntimeDir     string
	LogDir         string
	MaxRestarts    int32
	RestartDelay   time.Duration
	HealthInterval time.Duration
	MetricsBuffer  int
}

// ProcessConfig represents configuration for a single process
type ProcessConfig struct {
	Executable    string
	Args          []string
	Environment   map[string]string
	WorkingDir    string
	User          string
	Group         string
	RestartPolicy pb.RestartPolicy
	Resources     *pb.Resources
	HealthCheck   *pb.HealthCheck
}

// ProcessState represents the state of a managed process
type ProcessState int

const (
	StateUnknown ProcessState = iota
	StateStarting
	StateRunning
	StateStopping
	StateStopped
	StateCrashed
	StateRestarting
)

// ProcessMetrics contains metrics for a running process
type ProcessMetrics struct {
	AppID          string
	DeviceID       string
	PID            int32
	CPUPercent     float64
	MemoryBytes    uint64
	DiskReadBytes  uint64
	DiskWriteBytes uint64
	NetworkRx      uint64
	NetworkTx      uint64
	FDCount        int32
	ThreadCount    int32
	Timestamp      time.Time
}

// HealthStatus represents the health status of a process
type HealthStatus struct {
	Healthy   bool
	Message   string
	Timestamp time.Time
}

// MetricsCollector collects metrics for a managed process
type MetricsCollector struct {
	process *ManagedProcess
	logger  *slog.Logger
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector(mp *ManagedProcess) *MetricsCollector {
	return &MetricsCollector{
		process: mp,
		logger:  mp.logger.With("component", "metrics"),
	}
}

// LogStreamer handles log streaming for a process
type LogStreamer struct {
	process *ManagedProcess
	logDir  string
	logger  *slog.Logger
}

// NewLogStreamer creates a new log streamer
func NewLogStreamer(mp *ManagedProcess, logDir string) *LogStreamer {
	return &LogStreamer{
		process: mp,
		logDir:  logDir,
		logger:  mp.logger.With("component", "logs"),
	}
}

// StreamOutput streams output from a reader
func (ls *LogStreamer) StreamOutput(r io.ReadCloser, streamType string) {
	defer r.Close()
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		ls.logger.Info(streamType, "line", scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		ls.logger.Error("Stream error", "type", streamType, "error", err)
	}
}

// HealthChecker performs health checks on a process
type HealthChecker struct {
	process *ManagedProcess
	config  *pb.HealthCheck
	logger  *slog.Logger
}

// NewHealthChecker creates a new health checker
func NewHealthChecker(mp *ManagedProcess, config *pb.HealthCheck) *HealthChecker {
	return &HealthChecker{
		process: mp,
		config:  config,
		logger:  mp.logger.With("component", "health"),
	}
}

// Start starts the health checker
func (hc *HealthChecker) Start(healthCh chan<- HealthStatus) {
	// TODO: Implement health checking based on config
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-hc.process.ctx.Done():
			return
		case <-ticker.C:
			// Basic health check
			healthy := hc.process.Process != nil
			status := HealthStatus{
				Healthy:   healthy,
				Message:   "Process health check",
				Timestamp: time.Now(),
			}
			select {
			case healthCh <- status:
			default:
			}
		}
	}
}

// Manager manages application processes with enhanced error handling
type Manager struct {
	processes      map[string]*ManagedProcess
	mu             sync.RWMutex
	logger         *slog.Logger
	config         *Config
	metrics        chan ProcessMetrics
	circuitBreaker *ferrors.CircuitBreakerGroup
	errorHandler   *ferrors.ErrorHandler
	shutdownCh     chan struct{}
	shutdownWg     sync.WaitGroup
}

// ManagedProcess represents a process with production-ready error handling
type ManagedProcess struct {
	App          *pb.Application
	Artifact     *pb.Artifact
	Cmd          *exec.Cmd
	Process      *os.Process
	StartTime    time.Time
	RestartCount atomic.Int32
	State        atomic.Value // ProcessState

	// Channels
	stopCh   chan struct{}
	healthCh chan HealthStatus
	errorCh  chan error

	// Monitoring
	metrics     *MetricsCollector
	logs        *LogStreamer
	healthcheck *HealthChecker

	// Config
	config       *ProcessConfig
	retryPolicy  *ferrors.RetryPolicy
	errorHandler *ferrors.ErrorHandler

	// Context and cleanup
	ctx    context.Context
	cancel context.CancelFunc

	logger *slog.Logger
}

// NewManager creates a new process manager with enhanced error handling
func NewManager(config *Config) *Manager {
	// Create circuit breaker group for different operations
	cbConfig := &ferrors.CircuitBreakerConfig{
		MaxFailures: 5,
		MaxRequests: 1,
		Interval:    60 * time.Second,
		Timeout:     30 * time.Second,
		OnStateChange: func(from, to ferrors.CircuitBreakerState) {
			slog.Warn("Process manager circuit breaker state changed",
				"from", from.String(),
				"to", to.String(),
			)
		},
	}

	cbGroup := ferrors.NewCircuitBreakerGroup(cbConfig)

	// Create error handler
	errorHandler := &ferrors.ErrorHandler{
		OnError: func(err *ferrors.FleetError) {
			slog.Error("Process manager error",
				"code", err.Code,
				"message", err.Message,
				"severity", err.Severity,
				"retryable", err.Retryable,
			)
		},
		OnPanic: func(recovered any, stack string) {
			slog.Error("Process manager panic",
				"recovered", recovered,
				"stack", stack,
			)
		},
	}

	return &Manager{
		processes:      make(map[string]*ManagedProcess),
		logger:         slog.Default().With("component", "process-manager"),
		config:         config,
		metrics:        make(chan ProcessMetrics, config.MetricsBuffer),
		circuitBreaker: cbGroup,
		errorHandler:   errorHandler,
		shutdownCh:     make(chan struct{}),
	}
}

// DeployApplication deploys and starts an application with enhanced error handling
func (m *Manager) DeployApplication(ctx context.Context, app *pb.Application, artifacts map[string]*pb.Artifact) error {
	// Recover from panics
	defer m.errorHandler.HandlePanic()

	// Validate inputs
	if app == nil {
		return ferrors.New(ferrors.ErrCodeInvalidInput, "application is nil")
	}
	if app.Id == "" {
		return ferrors.New(ferrors.ErrCodeInvalidInput, "application ID is required")
	}

	m.logger.Info("Deploying application",
		"app", app.Name,
		"version", app.Version,
		"id", app.Id,
	)

	// Use circuit breaker for deployment
	return m.circuitBreaker.Execute(ctx, "deploy-"+app.Id, func() error {
		return m.deployWithRetry(ctx, app, artifacts)
	})
}

func (m *Manager) deployWithRetry(ctx context.Context, app *pb.Application, artifacts map[string]*pb.Artifact) error {
	retryConfig := &ferrors.RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 1 * time.Second,
		MaxDelay:     10 * time.Second,
		Multiplier:   2.0,
		RetryableFunc: func(err error) bool {
			code := ferrors.GetCode(err)
			// Retry on transient errors
			return code == ferrors.ErrCodeTimeout ||
				code == ferrors.ErrCodeUnavailable ||
				code == ferrors.ErrCodeResourceExhausted
		},
		OnRetry: func(attempt int, err error, delay time.Duration) {
			m.logger.Warn("Retrying deployment",
				"app", app.Name,
				"attempt", attempt,
				"error", err,
				"delay", delay,
			)
		},
	}

	return ferrors.Retry(ctx, retryConfig, func() error {
		// Stop existing version if running
		if existing := m.GetProcess(app.Id); existing != nil {
			if err := m.stopProcessWithTimeout(ctx, app.Id, 30*time.Second); err != nil {
				// Log but continue - old process might be dead already
				m.logger.Warn("Failed to stop existing process",
					"app", app.Id,
					"error", err,
				)
			}
		}

		// Extract and prepare artifacts
		execPath, err := m.prepareArtifactsWithValidation(ctx, app, artifacts)
		if err != nil {
			return ferrors.Wrapf(err, ferrors.ErrCodeDeploymentFailed,
				"failed to prepare artifacts for %s", app.Name)
		}

		// Create process configuration
		config, err := m.buildProcessConfigWithValidation(app, execPath)
		if err != nil {
			return ferrors.Wrapf(err, ferrors.ErrCodeInvalidInput,
				"invalid process configuration for %s", app.Name)
		}

		// Create managed process
		mp := m.createManagedProcess(app, config)

		// Start the process
		if err := mp.StartWithRetry(ctx); err != nil {
			return ferrors.Wrapf(err, ferrors.ErrCodeDeploymentFailed,
				"failed to start process %s", app.Name)
		}

		// Register with manager
		m.registerProcess(app.Id, mp)

		// Start monitoring
		m.shutdownWg.Add(1)
		go func() {
			defer m.shutdownWg.Done()
			mp.MonitorWithRecovery(m.metrics)
		}()

		m.logger.Info("Application deployed successfully",
			"app", app.Name,
			"version", app.Version,
			"pid", mp.Process.Pid,
		)

		return nil
	})
}

func (m *Manager) createManagedProcess(app *pb.Application, config *ProcessConfig) *ManagedProcess {
	ctx, cancel := context.WithCancel(context.Background())

	// Create retry policy for process operations
	retryConfig := &ferrors.RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 500 * time.Millisecond,
		MaxDelay:     5 * time.Second,
		Multiplier:   2.0,
	}

	retryPolicy := ferrors.NewRetryPolicy(retryConfig, nil)

	// Create error handler for this process
	errorHandler := &ferrors.ErrorHandler{
		RequestID: app.Id,
		OnError: func(err *ferrors.FleetError) {
			m.logger.Error("Process error",
				"app", app.Name,
				"error", err,
			)
		},
	}

	return &ManagedProcess{
		App:          app,
		config:       config,
		stopCh:       make(chan struct{}),
		healthCh:     make(chan HealthStatus, 1),
		errorCh:      make(chan error, 10),
		ctx:          ctx,
		cancel:       cancel,
		retryPolicy:  retryPolicy,
		errorHandler: errorHandler,
		logger:       m.logger.With("app", app.Name),
	}
}

// StartWithRetry starts the process with retry logic
func (mp *ManagedProcess) StartWithRetry(ctx context.Context) error {
	return mp.retryPolicy.Execute(ctx, func() error {
		return mp.start(ctx)
	})
}

func (mp *ManagedProcess) start(ctx context.Context) error {
	mp.logger.Info("Starting process")
	mp.State.Store(StateStarting)

	// Build command with proper error handling
	cmd, err := mp.buildCommand()
	if err != nil {
		mp.State.Store(StateCrashed)
		return ferrors.Wrap(err, ferrors.ErrCodeDeploymentFailed, "failed to build command")
	}

	mp.Cmd = cmd

	// Set up resource isolation
	if err := mp.setupResourceIsolation(); err != nil {
		// Log but continue - resource limits are best effort
		mp.logger.Warn("Failed to set resource limits", "error", err)
	}

	// Set up log streaming with error handling
	stdout, stderr, err := mp.setupLogStreaming()
	if err != nil {
		return ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to setup log streaming")
	}

	// Start the process with timeout
	startCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- mp.Cmd.Start()
	}()

	select {
	case <-startCtx.Done():
		return ferrors.Wrap(startCtx.Err(), ferrors.ErrCodeTimeout, "process start timeout")
	case err := <-errCh:
		if err != nil {
			mp.State.Store(StateCrashed)
			return ferrors.Wrap(err, ferrors.ErrCodeDeploymentFailed, "failed to start process")
		}
	}

	mp.Process = mp.Cmd.Process
	mp.StartTime = time.Now()
	mp.State.Store(StateRunning)

	// Start log streaming with error recovery
	mp.startLogStreamingWithRecovery(stdout, stderr)

	// Start health checking if configured
	if mp.config.HealthCheck != nil {
		mp.startHealthCheckingWithRecovery()
	}

	// Monitor process exit with recovery
	go mp.monitorExitWithRecovery()

	mp.logger.Info("Process started successfully", "pid", mp.Process.Pid)
	return nil
}

func (mp *ManagedProcess) buildCommand() (*exec.Cmd, error) {
	if mp.config.Executable == "" {
		return nil, ferrors.New(ferrors.ErrCodeInvalidInput, "executable path is empty")
	}

	// Validate executable exists
	if _, err := os.Stat(mp.config.Executable); err != nil {
		return nil, ferrors.Wrapf(err, ferrors.ErrCodeNotFound,
			"executable not found: %s", mp.config.Executable)
	}

	cmd := exec.Command(mp.config.Executable, mp.config.Args...)

	// Set environment with validation
	cmd.Env = os.Environ()
	for k, v := range mp.config.Environment {
		if k == "" {
			mp.logger.Warn("Skipping empty environment variable key")
			continue
		}
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	// Set working directory with validation
	if mp.config.WorkingDir != "" {
		if stat, err := os.Stat(mp.config.WorkingDir); err != nil {
			return nil, ferrors.Wrapf(err, ferrors.ErrCodeNotFound,
				"working directory not found: %s", mp.config.WorkingDir)
		} else if !stat.IsDir() {
			return nil, ferrors.Newf(ferrors.ErrCodeInvalidInput,
				"working directory is not a directory: %s", mp.config.WorkingDir)
		}
		cmd.Dir = mp.config.WorkingDir
	}

	return cmd, nil
}

func (mp *ManagedProcess) setupResourceIsolation() error {
	if mp.config.Resources == nil || mp.config.Resources.Limits == nil {
		return nil
	}

	// Call platform-specific resource limit setup
	mp.setResourceLimits()

	return nil
}

func (mp *ManagedProcess) setupLogStreaming() (io.ReadCloser, io.ReadCloser, error) {
	stdout, err := mp.Cmd.StdoutPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := mp.Cmd.StderrPipe()
	if err != nil {
		stdout.Close()
		return nil, nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	return stdout, stderr, nil
}

func (mp *ManagedProcess) startLogStreamingWithRecovery(stdout, stderr io.ReadCloser) {
	// Stream stdout with recovery
	go func() {
		defer func() {
			if r := recover(); r != nil {
				mp.logger.Error("Panic in stdout streaming", "recovered", r)
				mp.errorCh <- ferrors.Newf(ferrors.ErrCodeInternal,
					"stdout streaming panic: %v", r)
			}
		}()
		defer stdout.Close()

		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			select {
			case <-mp.stopCh:
				return
			default:
				mp.logger.Info("stdout", "line", scanner.Text())
			}
		}
		if err := scanner.Err(); err != nil {
			select {
			case <-mp.stopCh:
				return
			case mp.errorCh <- ferrors.Wrap(err, ferrors.ErrCodeInternal,
				"stdout streaming error"):
			}
		}
	}()

	// Stream stderr with recovery
	go func() {
		defer func() {
			if r := recover(); r != nil {
				mp.logger.Error("Panic in stderr streaming", "recovered", r)
				mp.errorCh <- ferrors.Newf(ferrors.ErrCodeInternal,
					"stderr streaming panic: %v", r)
			}
		}()
		defer stderr.Close()

		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			select {
			case <-mp.stopCh:
				return
			default:
				mp.logger.Warn("stderr", "line", scanner.Text())
			}
		}
		if err := scanner.Err(); err != nil {
			select {
			case <-mp.stopCh:
				return
			case mp.errorCh <- ferrors.Wrap(err, ferrors.ErrCodeInternal,
				"stderr streaming error"):
			}
		}
	}()
}

func (mp *ManagedProcess) startHealthCheckingWithRecovery() {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				mp.logger.Error("Panic in health checking", "recovered", r)
				mp.errorCh <- ferrors.Newf(ferrors.ErrCodeInternal,
					"health checking panic: %v", r)
			}
		}()

		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-mp.ctx.Done():
				return
			case <-ticker.C:
				health := mp.checkHealth()
				select {
				case mp.healthCh <- health:
				default:
					// Channel full, skip
				}
			}
		}
	}()
}

func (mp *ManagedProcess) checkHealth() HealthStatus {
	// Basic health check - process is running
	if mp.Process == nil {
		return HealthStatus{
			Healthy:   false,
			Message:   "Process not started",
			Timestamp: time.Now(),
		}
	}

	// Check if process is still alive
	p, err := process.NewProcess(int32(mp.Process.Pid))
	if err != nil {
		return HealthStatus{
			Healthy:   false,
			Message:   fmt.Sprintf("Failed to get process info: %v", err),
			Timestamp: time.Now(),
		}
	}

	running, err := p.IsRunning()
	if err != nil || !running {
		return HealthStatus{
			Healthy:   false,
			Message:   "Process not running",
			Timestamp: time.Now(),
		}
	}

	// TODO: Implement custom health check logic from mp.config.HealthCheck

	return HealthStatus{
		Healthy:   true,
		Message:   "Process is running",
		Timestamp: time.Now(),
	}
}

func (mp *ManagedProcess) monitorExitWithRecovery() {
	defer func() {
		if r := recover(); r != nil {
			mp.logger.Error("Panic in exit monitoring", "recovered", r)
			mp.State.Store(StateCrashed)
		}
	}()

	err := mp.Cmd.Wait()

	select {
	case <-mp.stopCh:
		// Intentional stop
		mp.State.Store(StateStopped)
		mp.logger.Info("Process stopped intentionally")
		return
	default:
		// Unexpected exit
		mp.State.Store(StateCrashed)

		fleetErr := ferrors.Wrapf(err, ferrors.ErrCodeDeploymentFailed,
			"process exited unexpectedly")
		mp.errorHandler.Handle(fleetErr)

		// Check restart policy
		if mp.shouldRestartWithBackoff() {
			mp.restartWithErrorHandling()
		}
	}
}

func (mp *ManagedProcess) shouldRestartWithBackoff() bool {
	restartCount := mp.RestartCount.Load()

	// Implement exponential backoff for restart decisions
	if restartCount > 10 {
		mp.logger.Error("Max restart count exceeded", "count", restartCount)
		return false
	}

	switch mp.config.RestartPolicy {
	case pb.RestartPolicy_RESTART_POLICY_ALWAYS:
		return true
	case pb.RestartPolicy_RESTART_POLICY_ON_FAILURE:
		return mp.Cmd.ProcessState != nil && !mp.Cmd.ProcessState.Success()
	case pb.RestartPolicy_RESTART_POLICY_NEVER:
		return false
	default:
		return false
	}
}

func (mp *ManagedProcess) restartWithErrorHandling() {
	count := mp.RestartCount.Add(1)
	mp.State.Store(StateRestarting)

	mp.logger.Info("Restarting process", "attempt", count)

	// Exponential backoff with jitter
	baseDelay := time.Duration(count) * time.Second
	if baseDelay > 30*time.Second {
		baseDelay = 30 * time.Second
	}

	// Add jitter to prevent thundering herd
	jitteredDelay := ferrors.ApplyJitter(baseDelay, 0.1)

	time.Sleep(jitteredDelay)

	// Restart with context
	ctx, cancel := context.WithTimeout(mp.ctx, 30*time.Second)
	defer cancel()

	if err := mp.StartWithRetry(ctx); err != nil {
		mp.logger.Error("Failed to restart process",
			"error", err,
			"attempt", count,
		)
		mp.State.Store(StateCrashed)

		// Send error to error channel
		select {
		case mp.errorCh <- err:
		default:
		}
	}
}

// MonitorWithRecovery monitors the process with panic recovery
func (mp *ManagedProcess) MonitorWithRecovery(metricsCh chan<- ProcessMetrics) {
	defer func() {
		if r := recover(); r != nil {
			mp.logger.Error("Panic in process monitoring", "recovered", r)
		}
	}()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-mp.ctx.Done():
			return
		case <-mp.stopCh:
			return
		case err := <-mp.errorCh:
			mp.errorHandler.Handle(err)
		case <-ticker.C:
			metrics, err := mp.collectMetrics()
			if err != nil {
				mp.logger.Debug("Failed to collect metrics", "error", err)
				continue
			}

			select {
			case metricsCh <- metrics:
			default:
				// Channel full, skip
			}
		}
	}
}

func (mp *ManagedProcess) collectMetrics() (ProcessMetrics, error) {
	metrics := ProcessMetrics{
		AppID:     mp.App.Id,
		Timestamp: time.Now(),
	}

	if mp.Process == nil {
		return metrics, ferrors.New(ferrors.ErrCodeInternal, "process not started")
	}

	p, err := process.NewProcess(int32(mp.Process.Pid))
	if err != nil {
		return metrics, ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to get process")
	}

	// Collect metrics with error handling for each
	if cpu, err := p.CPUPercent(); err == nil {
		metrics.CPUPercent = cpu
	}

	if mem, err := p.MemoryInfo(); err == nil && mem != nil {
		metrics.MemoryBytes = mem.RSS
	}

	if io, err := p.IOCounters(); err == nil && io != nil {
		metrics.DiskReadBytes = io.ReadBytes
		metrics.DiskWriteBytes = io.WriteBytes
	}

	if fds, err := p.NumFDs(); err == nil {
		metrics.FDCount = fds
	}

	if threads, err := p.NumThreads(); err == nil {
		metrics.ThreadCount = threads
	}

	metrics.PID = int32(mp.Process.Pid)

	return metrics, nil
}

// Helper methods for Manager

func (m *Manager) prepareArtifactsWithValidation(ctx context.Context, app *pb.Application, artifacts map[string]*pb.Artifact) (string, error) {
	if len(artifacts) == 0 {
		return "", ferrors.New(ferrors.ErrCodeInvalidInput, "no artifacts provided")
	}

	// TODO: Implement artifact extraction and validation
	// This is a placeholder
	return "/usr/local/bin/app", nil
}

func (m *Manager) buildProcessConfigWithValidation(app *pb.Application, execPath string) (*ProcessConfig, error) {
	if execPath == "" {
		return nil, ferrors.New(ferrors.ErrCodeInvalidInput, "executable path is empty")
	}

	config := &ProcessConfig{
		Executable:    execPath,
		Args:          app.Args,
		Environment:   app.Environment,
		WorkingDir:    app.WorkingDir,
		RestartPolicy: app.RestartPolicy,
		Resources:     app.Resources,
		HealthCheck:   app.HealthCheck,
	}

	// Validate configuration
	if err := m.validateProcessConfig(config); err != nil {
		return nil, err
	}

	return config, nil
}

func (m *Manager) validateProcessConfig(config *ProcessConfig) error {
	// Validate executable
	if _, err := os.Stat(config.Executable); err != nil {
		return ferrors.Wrapf(err, ferrors.ErrCodeNotFound,
			"executable not found: %s", config.Executable)
	}

	// Validate working directory if specified
	if config.WorkingDir != "" {
		if stat, err := os.Stat(config.WorkingDir); err != nil {
			return ferrors.Wrapf(err, ferrors.ErrCodeNotFound,
				"working directory not found: %s", config.WorkingDir)
		} else if !stat.IsDir() {
			return ferrors.Newf(ferrors.ErrCodeInvalidInput,
				"working directory is not a directory: %s", config.WorkingDir)
		}
	}

	return nil
}

func (m *Manager) stopProcessWithTimeout(ctx context.Context, appID string, timeout time.Duration) error {
	mp := m.GetProcess(appID)
	if mp == nil {
		return nil // Already stopped
	}

	stopCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Signal stop
	close(mp.stopCh)

	// Try graceful shutdown first
	if mp.Process != nil {
		if err := mp.Process.Signal(syscall.SIGTERM); err != nil {
			m.logger.Debug("Failed to send SIGTERM", "error", err)
		}

		// Wait for graceful shutdown
		done := make(chan struct{})
		go func() {
			mp.Cmd.Wait()
			close(done)
		}()

		select {
		case <-stopCtx.Done():
			// Force kill if timeout
			if err := mp.Process.Kill(); err != nil {
				return ferrors.Wrap(err, ferrors.ErrCodeTimeout,
					"failed to kill process after timeout")
			}
		case <-done:
			// Process exited gracefully
		}
	}

	// Cleanup
	mp.cancel()

	// Remove from manager
	m.mu.Lock()
	delete(m.processes, appID)
	m.mu.Unlock()

	return nil
}

func (m *Manager) registerProcess(appID string, mp *ManagedProcess) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.processes[appID] = mp
}

func (m *Manager) GetProcess(appID string) *ManagedProcess {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.processes[appID]
}

// Shutdown gracefully shuts down all processes
func (m *Manager) Shutdown(ctx context.Context) error {
	m.logger.Info("Shutting down process manager")

	// Signal shutdown
	close(m.shutdownCh)

	// Stop all processes
	var wg sync.WaitGroup
	m.mu.RLock()
	for appID := range m.processes {
		appID := appID
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := m.stopProcessWithTimeout(ctx, appID, 30*time.Second); err != nil {
				m.logger.Error("Failed to stop process during shutdown",
					"app", appID,
					"error", err,
				)
			}
		}()
	}
	m.mu.RUnlock()

	// Wait for all processes to stop
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-ctx.Done():
		return ferrors.Wrap(ctx.Err(), ferrors.ErrCodeTimeout,
			"shutdown timeout")
	case <-done:
		// All processes stopped
	}

	// Wait for monitoring goroutines
	m.shutdownWg.Wait()

	m.logger.Info("Process manager shutdown complete")
	return nil
}
