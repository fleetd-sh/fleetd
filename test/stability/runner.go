package stability

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
)

// Runner orchestrates the stability testing process
type Runner struct {
	config *Config
	logger *logrus.Logger
	test   *StabilityTest
}

// NewRunner creates a new stability test runner
func NewRunner(configPath string) (*Runner, error) {
	var config *Config
	var err error

	if configPath != "" {
		config, err = LoadConfig(configPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load config: %w", err)
		}
	} else {
		config = DefaultConfig()
	}

	// Setup logger
	logger := logrus.New()
	level, err := logrus.ParseLevel(config.LogLevel)
	if err != nil {
		level = logrus.InfoLevel
	}
	logger.SetLevel(level)

	return &Runner{
		config: config,
		logger: logger,
	}, nil
}

// Run executes the stability test
func (r *Runner) Run() error {
	r.logger.Info("Starting fleetd 72-hour stability test framework")

	// Create stability test
	test, err := NewStabilityTest(r.config)
	if err != nil {
		return fmt.Errorf("failed to create stability test: %w", err)
	}
	r.test = test

	// Setup validators
	if err := r.setupValidators(); err != nil {
		return fmt.Errorf("failed to setup validators: %w", err)
	}

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start test in goroutine
	testDone := make(chan error, 1)
	go func() {
		testDone <- r.test.Start()
	}()

	// Wait for completion or signal
	select {
	case err := <-testDone:
		if err != nil {
			r.logger.WithError(err).Error("Stability test failed")
			return err
		}
		r.logger.Info("Stability test completed successfully")

	case sig := <-sigChan:
		r.logger.WithField("signal", sig).Info("Received signal, stopping test")
		r.test.Stop()

		// Wait for graceful shutdown
		select {
		case err := <-testDone:
			if err != nil {
				r.logger.WithError(err).Warn("Test stopped with error")
			}
		case <-time.After(30 * time.Second):
			r.logger.Warn("Test shutdown timed out")
		}
	}

	return nil
}

// setupValidators initializes and configures all validators
func (r *Runner) setupValidators() error {
	r.logger.Info("Setting up stability validators")

	// Memory leak validator
	if r.isComponentEnabled("memory") {
		validator := NewMemoryLeakValidator(r.config, r.logger)
		r.test.AddValidator(validator)
		r.logger.Debug("Added memory leak validator")
	}

	// Connection stability validator
	if r.isComponentEnabled("connections") {
		endpoints := []string{
			"http://localhost:8080/health",
			"localhost:8080",
		}
		validator := NewConnectionStabilityValidator(r.config, r.logger, endpoints)
		r.test.AddValidator(validator)
		r.logger.Debug("Added connection stability validator")
	}

	// Database integrity validator
	if r.isComponentEnabled("database") && r.config.DatabasePath != "" {
		validator, err := NewDatabaseIntegrityValidator(r.config, r.logger)
		if err != nil {
			r.logger.WithError(err).Warn("Failed to create database validator, skipping")
		} else {
			r.test.AddValidator(validator)
			r.logger.Debug("Added database integrity validator")
		}
	}

	// Deadlock detector
	if r.isComponentEnabled("goroutines") {
		detector := NewDeadlockDetector(r.config, r.logger)
		r.test.AddValidator(detector)
		r.logger.Debug("Added deadlock detector")
	}

	// TLS validator
	if r.isComponentEnabled("tls") && r.config.TLSCertPath != "" {
		validator := NewTLSValidator(r.config, r.logger)
		r.test.AddValidator(validator)
		r.logger.Debug("Added TLS validator")
	}

	// Performance validator
	if r.isComponentEnabled("performance") {
		validator := NewPerformanceValidator(r.config, r.logger)
		r.test.AddValidator(validator)
		r.logger.Debug("Added performance validator")
	}

	return nil
}

// isComponentEnabled checks if a component is enabled in the configuration
func (r *Runner) isComponentEnabled(component string) bool {
	for _, enabled := range r.config.EnabledComponents {
		if enabled == component {
			return true
		}
	}
	return false
}

// GetStatus returns current test status
func (r *Runner) GetStatus() *TestStatus {
	if r.test == nil {
		return &TestStatus{
			Running:   false,
			StartTime: time.Time{},
			Duration:  0,
		}
	}

	metrics := r.test.GetMetrics()
	snapshots := metrics.GetSnapshots()
	errors := metrics.GetErrors()

	var startTime time.Time
	if len(snapshots) > 0 {
		startTime = snapshots[0].Timestamp
	}

	return &TestStatus{
		Running:      r.test.IsRunning(),
		StartTime:    startTime,
		Duration:     time.Since(startTime),
		TotalErrors:  len(errors),
		LastSnapshot: func() *MetricsSnapshot {
			if len(snapshots) > 0 {
				return &snapshots[len(snapshots)-1]
			}
			return nil
		}(),
	}
}

// GenerateConfigTemplate generates a configuration template file
func GenerateConfigTemplate(outputPath string) error {
	config := DefaultConfig()

	// Create output directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	return config.SaveConfig(outputPath)
}

// TestStatus represents the current status of a running test
type TestStatus struct {
	Running      bool              `json:"running"`
	StartTime    time.Time         `json:"start_time"`
	Duration     time.Duration     `json:"duration"`
	TotalErrors  int               `json:"total_errors"`
	LastSnapshot *MetricsSnapshot  `json:"last_snapshot,omitempty"`
}

// Main entry point for stability testing
func RunStabilityTest(configPath string, outputDir string) error {
	// Create runner
	runner, err := NewRunner(configPath)
	if err != nil {
		return fmt.Errorf("failed to create runner: %w", err)
	}

	// Override output directory if specified
	if outputDir != "" {
		runner.config.OutputDir = outputDir
	}

	// Ensure output directory exists
	if err := os.MkdirAll(runner.config.OutputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Save current configuration for reference
	configFile := filepath.Join(runner.config.OutputDir, "test-config.json")
	if err := runner.config.SaveConfig(configFile); err != nil {
		runner.logger.WithError(err).Warn("Failed to save test configuration")
	}

	// Run the test
	return runner.Run()
}

// ValidateConfig validates a configuration file
func ValidateConfig(configPath string) error {
	config, err := LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Basic validation
	if config.Duration <= 0 {
		return fmt.Errorf("duration must be positive")
	}

	if config.MonitorInterval <= 0 {
		return fmt.Errorf("monitor interval must be positive")
	}

	if config.ValidationInterval <= 0 {
		return fmt.Errorf("validation interval must be positive")
	}

	if config.OutputDir == "" {
		return fmt.Errorf("output directory must be specified")
	}

	// Check if we can create output directory
	if err := os.MkdirAll(config.OutputDir, 0755); err != nil {
		return fmt.Errorf("cannot create output directory: %w", err)
	}

	// Check database path if specified
	if config.DatabasePath != "" {
		dir := filepath.Dir(config.DatabasePath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("cannot create database directory: %w", err)
		}
	}

	// Check TLS files if specified
	if config.TLSCertPath != "" {
		if _, err := os.Stat(config.TLSCertPath); os.IsNotExist(err) {
			return fmt.Errorf("TLS certificate file does not exist: %s", config.TLSCertPath)
		}
	}

	if config.TLSKeyPath != "" {
		if _, err := os.Stat(config.TLSKeyPath); os.IsNotExist(err) {
			return fmt.Errorf("TLS key file does not exist: %s", config.TLSKeyPath)
		}
	}

	return nil
}