package stability

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Validator interface for all stability validators
type Validator interface {
	Name() string
	Validate(ctx context.Context) error
	Configure(config map[string]interface{}) error
	Reset() error
}

// StabilityMetrics tracks all stability metrics during testing
type StabilityMetrics struct {
	mu        sync.RWMutex
	snapshots []MetricsSnapshot
	errors    map[string][]StabilityError
	alerts    []Alert
}

// MetricsSnapshot represents a point-in-time metrics collection
type MetricsSnapshot struct {
	Timestamp    time.Time `json:"timestamp"`
	MemoryUsage  int64     `json:"memory_usage_bytes"`
	CPUUsage     float64   `json:"cpu_usage_percent"`
	Goroutines   int       `json:"goroutines"`
	OpenFiles    int       `json:"open_files"`
	Connections  int       `json:"connections"`
	Uptime       time.Duration `json:"uptime"`

	// Database metrics
	DBConnections     int   `json:"db_connections"`
	DBSlowQueries     int   `json:"db_slow_queries"`
	DBConnectionPool  int   `json:"db_connection_pool_size"`

	// Network metrics
	ActiveRequests    int     `json:"active_requests"`
	ResponseTime      time.Duration `json:"avg_response_time"`
	ErrorRate         float64 `json:"error_rate"`
	ThroughputRPS     float64 `json:"throughput_rps"`

	// TLS metrics
	CertExpiry        time.Duration `json:"cert_expiry_time"`
	TLSHandshakes     int64   `json:"tls_handshakes_total"`
	TLSErrors         int64   `json:"tls_errors_total"`

	// Custom application metrics
	CustomMetrics     map[string]float64 `json:"custom_metrics"`
}

// StabilityError represents an error that occurred during testing
type StabilityError struct {
	Timestamp   time.Time `json:"timestamp"`
	Component   string    `json:"component"`
	Message     string    `json:"message"`
	Severity    string    `json:"severity"`
	Context     map[string]interface{} `json:"context"`
	Recovered   bool      `json:"recovered"`
}

// Alert represents an alert triggered during testing
type Alert struct {
	Timestamp   time.Time `json:"timestamp"`
	Type        string    `json:"type"`
	Severity    string    `json:"severity"`
	Message     string    `json:"message"`
	Metric      string    `json:"metric"`
	Value       float64   `json:"value"`
	Threshold   float64   `json:"threshold"`
	Resolved    bool      `json:"resolved"`
	ResolvedAt  *time.Time `json:"resolved_at,omitempty"`
}

// StabilityReport represents the final test report
type StabilityReport struct {
	Config       *Config           `json:"config"`
	StartTime    time.Time         `json:"start_time"`
	EndTime      time.Time         `json:"end_time"`
	Duration     time.Duration     `json:"duration"`
	GeneratedAt  time.Time         `json:"generated_at"`
	Success      bool              `json:"success"`

	// Summary statistics
	PeakMemoryMB    int64         `json:"peak_memory_mb"`
	AverageCPU      float64       `json:"average_cpu_percent"`
	MaxGoroutines   int           `json:"max_goroutines"`
	MaxOpenFiles    int           `json:"max_open_files"`
	MaxConnections  int           `json:"max_connections"`

	// Analysis results
	MemoryLeakDetected      bool    `json:"memory_leak_detected"`
	MemoryLeakRate          float64 `json:"memory_leak_rate_percent"`
	PerformanceDegradation  bool    `json:"performance_degradation"`
	PerformanceDegradationPercent float64 `json:"performance_degradation_percent"`
	ConnectionStability     bool    `json:"connection_stability"`
	DataIntegrityIssues     bool    `json:"data_integrity_issues"`

	// Detailed results
	Errors          []StabilityError  `json:"errors"`
	Alerts          []Alert           `json:"alerts"`
	MetricsSummary  MetricsSummary    `json:"metrics_summary"`

	// Pass/Fail criteria results
	PassFailCriteria map[string]CriteriaResult `json:"pass_fail_criteria"`
}

// MetricsSummary provides statistical summary of collected metrics
type MetricsSummary struct {
	Memory      StatSummary `json:"memory"`
	CPU         StatSummary `json:"cpu"`
	Goroutines  StatSummary `json:"goroutines"`
	OpenFiles   StatSummary `json:"open_files"`
	Connections StatSummary `json:"connections"`
	ResponseTime StatSummary `json:"response_time"`
}

// StatSummary provides statistical analysis of a metric
type StatSummary struct {
	Min     float64 `json:"min"`
	Max     float64 `json:"max"`
	Average float64 `json:"average"`
	Median  float64 `json:"median"`
	P95     float64 `json:"p95"`
	P99     float64 `json:"p99"`
	StdDev  float64 `json:"std_dev"`
	Trend   string  `json:"trend"` // "increasing", "decreasing", "stable"
}

// CriteriaResult represents the result of a pass/fail criteria
type CriteriaResult struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Passed      bool      `json:"passed"`
	Value       float64   `json:"value"`
	Threshold   float64   `json:"threshold"`
	Message     string    `json:"message"`
	CheckedAt   time.Time `json:"checked_at"`
}

// NewStabilityMetrics creates a new metrics tracker
func NewStabilityMetrics() *StabilityMetrics {
	return &StabilityMetrics{
		snapshots: make([]MetricsSnapshot, 0),
		errors:    make(map[string][]StabilityError),
		alerts:    make([]Alert, 0),
	}
}

// AddSnapshot adds a metrics snapshot
func (sm *StabilityMetrics) AddSnapshot(snapshot MetricsSnapshot) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.snapshots = append(sm.snapshots, snapshot)
}

// AddError adds an error to the metrics
func (sm *StabilityMetrics) AddError(component string, err error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	stabilityError := StabilityError{
		Timestamp: time.Now(),
		Component: component,
		Message:   err.Error(),
		Severity:  "error",
		Context:   make(map[string]interface{}),
		Recovered: false,
	}

	if sm.errors[component] == nil {
		sm.errors[component] = make([]StabilityError, 0)
	}
	sm.errors[component] = append(sm.errors[component], stabilityError)
}

// AddAlert adds an alert to the metrics
func (sm *StabilityMetrics) AddAlert(alert Alert) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.alerts = append(sm.alerts, alert)
}

// GetSnapshots returns all metrics snapshots
func (sm *StabilityMetrics) GetSnapshots() []MetricsSnapshot {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	snapshots := make([]MetricsSnapshot, len(sm.snapshots))
	copy(snapshots, sm.snapshots)
	return snapshots
}

// GetErrors returns all errors by component
func (sm *StabilityMetrics) GetErrors() map[string][]StabilityError {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	result := make(map[string][]StabilityError)
	for component, errors := range sm.errors {
		result[component] = make([]StabilityError, len(errors))
		copy(result[component], errors)
	}
	return result
}

// GetAlerts returns all alerts
func (sm *StabilityMetrics) GetAlerts() []Alert {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	alerts := make([]Alert, len(sm.alerts))
	copy(alerts, sm.alerts)
	return alerts
}

// GenerateReport generates a comprehensive stability report
func (sm *StabilityMetrics) GenerateReport() *StabilityReport {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if len(sm.snapshots) == 0 {
		return &StabilityReport{
			Success: false,
			Errors:  []StabilityError{},
			Alerts:  []Alert{},
		}
	}

	// Calculate summary statistics
	startTime := sm.snapshots[0].Timestamp
	endTime := sm.snapshots[len(sm.snapshots)-1].Timestamp

	// Find peaks and averages
	var peakMemory int64
	var totalCPU float64
	var maxGoroutines, maxOpenFiles, maxConnections int

	for _, snapshot := range sm.snapshots {
		if snapshot.MemoryUsage > peakMemory {
			peakMemory = snapshot.MemoryUsage
		}
		totalCPU += snapshot.CPUUsage
		if snapshot.Goroutines > maxGoroutines {
			maxGoroutines = snapshot.Goroutines
		}
		if snapshot.OpenFiles > maxOpenFiles {
			maxOpenFiles = snapshot.OpenFiles
		}
		if snapshot.Connections > maxConnections {
			maxConnections = snapshot.Connections
		}
	}

	avgCPU := totalCPU / float64(len(sm.snapshots))

	// Collect all errors
	allErrors := make([]StabilityError, 0)
	for _, errors := range sm.errors {
		allErrors = append(allErrors, errors...)
	}

	// Determine overall success
	success := len(allErrors) == 0 && len(sm.alerts) == 0

	// Generate metrics summary
	metricsSummary := sm.generateMetricsSummary()

	return &StabilityReport{
		StartTime:       startTime,
		EndTime:         endTime,
		Duration:        endTime.Sub(startTime),
		Success:         success,
		PeakMemoryMB:    peakMemory / 1024 / 1024,
		AverageCPU:      avgCPU,
		MaxGoroutines:   maxGoroutines,
		MaxOpenFiles:    maxOpenFiles,
		MaxConnections:  maxConnections,
		Errors:          allErrors,
		Alerts:          sm.alerts,
		MetricsSummary:  metricsSummary,
		PassFailCriteria: make(map[string]CriteriaResult),
	}
}

// generateMetricsSummary generates statistical summary for metrics
func (sm *StabilityMetrics) generateMetricsSummary() MetricsSummary {
	if len(sm.snapshots) == 0 {
		return MetricsSummary{}
	}

	// Extract metric series
	memoryValues := make([]float64, len(sm.snapshots))
	cpuValues := make([]float64, len(sm.snapshots))
	goroutineValues := make([]float64, len(sm.snapshots))
	fileValues := make([]float64, len(sm.snapshots))
	connectionValues := make([]float64, len(sm.snapshots))
	responseTimeValues := make([]float64, len(sm.snapshots))

	for i, snapshot := range sm.snapshots {
		memoryValues[i] = float64(snapshot.MemoryUsage)
		cpuValues[i] = snapshot.CPUUsage
		goroutineValues[i] = float64(snapshot.Goroutines)
		fileValues[i] = float64(snapshot.OpenFiles)
		connectionValues[i] = float64(snapshot.Connections)
		responseTimeValues[i] = float64(snapshot.ResponseTime.Nanoseconds())
	}

	return MetricsSummary{
		Memory:      calculateStatSummary(memoryValues),
		CPU:         calculateStatSummary(cpuValues),
		Goroutines:  calculateStatSummary(goroutineValues),
		OpenFiles:   calculateStatSummary(fileValues),
		Connections: calculateStatSummary(connectionValues),
		ResponseTime: calculateStatSummary(responseTimeValues),
	}
}

// calculateStatSummary calculates statistical summary for a series of values
func calculateStatSummary(values []float64) StatSummary {
	if len(values) == 0 {
		return StatSummary{}
	}

	// Sort for percentiles
	sorted := make([]float64, len(values))
	copy(sorted, values)

	// Simple bubble sort (good enough for this use case)
	for i := 0; i < len(sorted); i++ {
		for j := 0; j < len(sorted)-i-1; j++ {
			if sorted[j] > sorted[j+1] {
				sorted[j], sorted[j+1] = sorted[j+1], sorted[j]
			}
		}
	}

	// Calculate basic statistics
	min := sorted[0]
	max := sorted[len(sorted)-1]

	var sum float64
	for _, v := range values {
		sum += v
	}
	average := sum / float64(len(values))

	median := sorted[len(sorted)/2]
	p95 := sorted[int(float64(len(sorted))*0.95)]
	p99 := sorted[int(float64(len(sorted))*0.99)]

	// Calculate standard deviation
	var variance float64
	for _, v := range values {
		variance += (v - average) * (v - average)
	}
	stdDev := variance / float64(len(values))

	// Determine trend
	trend := "stable"
	if len(values) >= 10 {
		firstQuarter := values[:len(values)/4]
		lastQuarter := values[3*len(values)/4:]

		var firstSum, lastSum float64
		for _, v := range firstQuarter {
			firstSum += v
		}
		for _, v := range lastQuarter {
			lastSum += v
		}

		firstAvg := firstSum / float64(len(firstQuarter))
		lastAvg := lastSum / float64(len(lastQuarter))

		change := (lastAvg - firstAvg) / firstAvg * 100
		if change > 5 {
			trend = "increasing"
		} else if change < -5 {
			trend = "decreasing"
		}
	}

	return StatSummary{
		Min:     min,
		Max:     max,
		Average: average,
		Median:  median,
		P95:     p95,
		P99:     p99,
		StdDev:  stdDev,
		Trend:   trend,
	}
}

// getStatusString returns a human-readable status string
func (sr *StabilityReport) getStatusString(component string) string {
	switch component {
	case "memory_leak":
		if sr.MemoryLeakDetected {
			return fmt.Sprintf("DETECTED (%.2f%% increase)", sr.MemoryLeakRate)
		}
		return "PASS"
	case "performance":
		if sr.PerformanceDegradation {
			return fmt.Sprintf("DEGRADED (%.2f%% slower)", sr.PerformanceDegradationPercent)
		}
		return "PASS"
	case "connections":
		if !sr.ConnectionStability {
			return "UNSTABLE"
		}
		return "STABLE"
	case "data_integrity":
		if sr.DataIntegrityIssues {
			return "ISSUES DETECTED"
		}
		return "PASS"
	default:
		return "UNKNOWN"
	}
}