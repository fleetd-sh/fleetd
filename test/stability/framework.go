package stability

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// StabilityTest represents the main stability testing framework
type StabilityTest struct {
	config     *Config
	logger     *logrus.Logger
	monitor    *SystemMonitor
	validators []Validator
	running    bool
	ctx        context.Context
	cancel     context.CancelFunc
	mu         sync.RWMutex
	metrics    *StabilityMetrics
	startTime  time.Time
}

// Config holds configuration for stability testing
type Config struct {
	// Test duration (default 72 hours)
	Duration time.Duration `json:"duration"`

	// Monitoring intervals
	MonitorInterval     time.Duration `json:"monitor_interval"`
	ValidationInterval  time.Duration `json:"validation_interval"`
	MetricsInterval     time.Duration `json:"metrics_interval"`

	// Resource thresholds
	MaxMemoryMB         int64   `json:"max_memory_mb"`
	MaxCPUPercent       float64 `json:"max_cpu_percent"`
	MaxGoroutines       int     `json:"max_goroutines"`
	MaxOpenFiles        int     `json:"max_open_files"`
	MaxConnections      int     `json:"max_connections"`

	// Memory leak detection
	MemoryLeakThreshold float64 `json:"memory_leak_threshold"` // Percentage increase over time
	MemoryLeakWindow    time.Duration `json:"memory_leak_window"`

	// Performance degradation detection
	PerformanceThreshold float64 `json:"performance_threshold"` // Percentage degradation
	ResponseTimeLimit    time.Duration `json:"response_time_limit"`

	// Database settings
	DatabasePath        string `json:"database_path"`
	MaxDBConnections    int    `json:"max_db_connections"`

	// Network settings
	NetworkTimeout      time.Duration `json:"network_timeout"`
	RetryAttempts       int           `json:"retry_attempts"`

	// TLS settings
	TLSCertPath         string `json:"tls_cert_path"`
	TLSKeyPath          string `json:"tls_key_path"`
	CertRenewalWindow   time.Duration `json:"cert_renewal_window"`

	// Output settings
	OutputDir           string `json:"output_dir"`
	ReportFormat        string `json:"report_format"` // json, html, text
	LogLevel            string `json:"log_level"`

	// Test components
	EnabledComponents   []string `json:"enabled_components"`

	// Failure criteria
	FailOnMemoryLeak    bool `json:"fail_on_memory_leak"`
	FailOnCrash         bool `json:"fail_on_crash"`
	FailOnDeadlock      bool `json:"fail_on_deadlock"`
	FailOnDataCorruption bool `json:"fail_on_data_corruption"`
}

// DefaultConfig returns default configuration for 72-hour stability testing
func DefaultConfig() *Config {
	return &Config{
		Duration:            72 * time.Hour,
		MonitorInterval:     30 * time.Second,
		ValidationInterval:  5 * time.Minute,
		MetricsInterval:     1 * time.Minute,
		MaxMemoryMB:         2048,
		MaxCPUPercent:       80.0,
		MaxGoroutines:       10000,
		MaxOpenFiles:        1000,
		MaxConnections:      500,
		MemoryLeakThreshold: 10.0, // 10% increase triggers alert
		MemoryLeakWindow:    1 * time.Hour,
		PerformanceThreshold: 20.0, // 20% degradation triggers alert
		ResponseTimeLimit:   10 * time.Second,
		MaxDBConnections:    100,
		NetworkTimeout:      30 * time.Second,
		RetryAttempts:       3,
		CertRenewalWindow:   24 * time.Hour,
		OutputDir:           "./stability-results",
		ReportFormat:        "json",
		LogLevel:            "info",
		EnabledComponents: []string{
			"memory", "cpu", "goroutines", "connections",
			"database", "tls", "network", "data_integrity",
		},
		FailOnMemoryLeak:     true,
		FailOnCrash:          true,
		FailOnDeadlock:       true,
		FailOnDataCorruption: true,
	}
}

// LoadConfig loads configuration from file
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	config := DefaultConfig()
	if err := json.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return config, nil
}

// SaveConfig saves configuration to file
func (c *Config) SaveConfig(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	return os.WriteFile(path, data, 0644)
}

// NewStabilityTest creates a new stability test instance
func NewStabilityTest(config *Config) (*StabilityTest, error) {
	if config == nil {
		config = DefaultConfig()
	}

	// Create output directory
	if err := os.MkdirAll(config.OutputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	// Setup logger
	logger := logrus.New()
	level, err := logrus.ParseLevel(config.LogLevel)
	if err != nil {
		level = logrus.InfoLevel
	}
	logger.SetLevel(level)

	// Create log file
	logFile, err := os.OpenFile(
		filepath.Join(config.OutputDir, "stability.log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND,
		0666,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create log file: %w", err)
	}
	logger.SetOutput(logFile)

	// Create system monitor
	monitor, err := NewSystemMonitor(config, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create system monitor: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), config.Duration)

	return &StabilityTest{
		config:  config,
		logger:  logger,
		monitor: monitor,
		ctx:     ctx,
		cancel:  cancel,
		metrics: NewStabilityMetrics(),
	}, nil
}

// AddValidator adds a validator to the test
func (st *StabilityTest) AddValidator(validator Validator) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.validators = append(st.validators, validator)
}

// Start begins the stability test
func (st *StabilityTest) Start() error {
	st.mu.Lock()
	if st.running {
		st.mu.Unlock()
		return fmt.Errorf("stability test is already running")
	}
	st.running = true
	st.startTime = time.Now()
	st.mu.Unlock()

	st.logger.Info("Starting 72-hour stability test")
	st.logger.WithFields(logrus.Fields{
		"duration":         st.config.Duration,
		"monitor_interval": st.config.MonitorInterval,
		"validators":       len(st.validators),
	}).Info("Stability test configuration")

	// Start monitoring
	go st.runMonitoring()
	go st.runValidation()
	go st.runMetricsCollection()

	// Wait for completion or cancellation
	<-st.ctx.Done()

	st.mu.Lock()
	st.running = false
	st.mu.Unlock()

	return st.generateReport()
}

// Stop stops the stability test
func (st *StabilityTest) Stop() {
	st.cancel()
}

// IsRunning returns whether the test is currently running
func (st *StabilityTest) IsRunning() bool {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.running
}

// GetMetrics returns current stability metrics
func (st *StabilityTest) GetMetrics() *StabilityMetrics {
	return st.metrics
}

// runMonitoring runs the system monitoring loop
func (st *StabilityTest) runMonitoring() {
	ticker := time.NewTicker(st.config.MonitorInterval)
	defer ticker.Stop()

	for {
		select {
		case <-st.ctx.Done():
			return
		case <-ticker.C:
			if err := st.monitor.CollectMetrics(); err != nil {
				st.logger.WithError(err).Error("Failed to collect system metrics")
				st.metrics.AddError("system_monitoring", err)
			}
		}
	}
}

// runValidation runs the validation loop
func (st *StabilityTest) runValidation() {
	ticker := time.NewTicker(st.config.ValidationInterval)
	defer ticker.Stop()

	for {
		select {
		case <-st.ctx.Done():
			return
		case <-ticker.C:
			st.runValidators()
		}
	}
}

// runValidators executes all registered validators
func (st *StabilityTest) runValidators() {
	for _, validator := range st.validators {
		if err := validator.Validate(st.ctx); err != nil {
			st.logger.WithFields(logrus.Fields{
				"validator": validator.Name(),
				"error":     err,
			}).Error("Validation failed")
			st.metrics.AddError(validator.Name(), err)

			// Check if this is a critical failure
			if st.shouldFailFast(validator.Name(), err) {
				st.logger.Fatal("Critical failure detected, stopping test")
				st.Stop()
				return
			}
		}
	}
}

// shouldFailFast determines if a failure should stop the test immediately
func (st *StabilityTest) shouldFailFast(validatorName string, err error) bool {
	switch validatorName {
	case "memory_leak":
		return st.config.FailOnMemoryLeak
	case "crash_detector":
		return st.config.FailOnCrash
	case "deadlock_detector":
		return st.config.FailOnDeadlock
	case "data_integrity":
		return st.config.FailOnDataCorruption
	default:
		return false
	}
}

// runMetricsCollection collects and persists metrics
func (st *StabilityTest) runMetricsCollection() {
	ticker := time.NewTicker(st.config.MetricsInterval)
	defer ticker.Stop()

	for {
		select {
		case <-st.ctx.Done():
			return
		case <-ticker.C:
			st.collectAndPersistMetrics()
		}
	}
}

// collectAndPersistMetrics collects current metrics and saves them
func (st *StabilityTest) collectAndPersistMetrics() {
	// Collect runtime metrics
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	snapshot := MetricsSnapshot{
		Timestamp:    time.Now(),
		MemoryUsage:  int64(m.Alloc),
		CPUUsage:     st.monitor.GetCPUUsage(),
		Goroutines:   runtime.NumGoroutine(),
		OpenFiles:    st.monitor.GetOpenFileCount(),
		Connections:  st.monitor.GetConnectionCount(),
		Uptime:       time.Since(st.startTime),
	}

	st.metrics.AddSnapshot(snapshot)

	// Persist to file
	if err := st.persistMetrics(snapshot); err != nil {
		st.logger.WithError(err).Error("Failed to persist metrics")
	}
}

// persistMetrics saves metrics snapshot to file
func (st *StabilityTest) persistMetrics(snapshot MetricsSnapshot) error {
	filename := filepath.Join(st.config.OutputDir, "metrics.jsonl")
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	data, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}

	_, err = file.Write(append(data, '\n'))
	return err
}

// generateReport generates the final stability test report
func (st *StabilityTest) generateReport() error {
	report := st.metrics.GenerateReport()
	report.Config = st.config
	report.Duration = time.Since(st.startTime)
	report.GeneratedAt = time.Now()

	// Save report
	filename := filepath.Join(st.config.OutputDir, fmt.Sprintf("stability-report.%s", st.config.ReportFormat))

	switch st.config.ReportFormat {
	case "json":
		return st.saveJSONReport(filename, report)
	case "html":
		return st.saveHTMLReport(filename, report)
	default:
		return st.saveTextReport(filename, report)
	}
}

// saveJSONReport saves report as JSON
func (st *StabilityTest) saveJSONReport(filename string, report *StabilityReport) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filename, data, 0644)
}

// saveHTMLReport saves report as HTML
func (st *StabilityTest) saveHTMLReport(filename string, report *StabilityReport) error {
	// Implementation would generate HTML report
	// For now, fallback to JSON
	return st.saveJSONReport(filename, report)
}

// saveTextReport saves report as plain text
func (st *StabilityTest) saveTextReport(filename string, report *StabilityReport) error {
	content := fmt.Sprintf(`
fleetd 72-Hour Stability Test Report
====================================

Test Duration: %v
Start Time: %v
End Time: %v
Success: %t

Resource Usage Summary:
- Peak Memory: %d MB
- Average CPU: %.2f%%
- Max Goroutines: %d
- Total Errors: %d

Memory Leak Detection: %s
Performance Degradation: %s
Connection Stability: %s
Data Integrity: %s

For detailed metrics, see metrics.jsonl
`,
		report.Duration,
		report.StartTime,
		report.EndTime,
		report.Success,
		report.PeakMemoryMB,
		report.AverageCPU,
		report.MaxGoroutines,
		len(report.Errors),
		report.getStatusString("memory_leak"),
		report.getStatusString("performance"),
		report.getStatusString("connections"),
		report.getStatusString("data_integrity"),
	)

	return os.WriteFile(filename, []byte(content), 0644)
}