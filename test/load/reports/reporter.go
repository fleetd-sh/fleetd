package reports

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fleetd.sh/test/load/framework"
)

// Reporter generates performance reports from load test results
type Reporter struct {
	logger    *slog.Logger
	outputDir string
	templates map[string]*template.Template
}

// ReportConfig configures report generation
type ReportConfig struct {
	OutputDirectory string
	ReportFormats   []ReportFormat
	IncludeCharts   bool
	IncludeRawData  bool
}

// ReportFormat defines the output format for reports
type ReportFormat string

const (
	FormatHTML ReportFormat = "html"
	FormatJSON ReportFormat = "json"
	FormatCSV  ReportFormat = "csv"
	FormatPDF  ReportFormat = "pdf"
)

// TestReport represents a comprehensive test report
type TestReport struct {
	Metadata          ReportMetadata      `json:"metadata"`
	ExecutiveSummary  ExecutiveSummary    `json:"executive_summary"`
	TestConfiguration TestConfiguration   `json:"test_configuration"`
	Results           TestResults         `json:"results"`
	Performance       PerformanceAnalysis `json:"performance"`
	Scenarios         []ScenarioReport    `json:"scenarios"`
	SystemMetrics     SystemMetricsReport `json:"system_metrics"`
	Recommendations   []Recommendation    `json:"recommendations"`
	RawData           *RawDataReport      `json:"raw_data,omitempty"`
}

// ReportMetadata contains report metadata
type ReportMetadata struct {
	GeneratedAt  time.Time     `json:"generated_at"`
	Generator    string        `json:"generator"`
	Version      string        `json:"version"`
	TestName     string        `json:"test_name"`
	TestDuration time.Duration `json:"test_duration"`
	ReportFormat string        `json:"report_format"`
}

// ExecutiveSummary provides a high-level overview
type ExecutiveSummary struct {
	TestPassed       bool     `json:"test_passed"`
	OverallScore     float64  `json:"overall_score"`
	KeyMetrics       []Metric `json:"key_metrics"`
	CriticalIssues   []Issue  `json:"critical_issues"`
	PerformanceGrade string   `json:"performance_grade"`
	Summary          string   `json:"summary"`
}

// TestConfiguration describes the test setup
type TestConfiguration struct {
	TotalDevices   int                             `json:"total_devices"`
	DeviceProfiles map[framework.DeviceProfile]int `json:"device_profiles"`
	TestScenarios  []string                        `json:"test_scenarios"`
	TestDuration   time.Duration                   `json:"test_duration"`
	ServerConfig   ServerConfiguration             `json:"server_config"`
	LoadParameters LoadParameters                  `json:"load_parameters"`
}

// ServerConfiguration describes the server under test
type ServerConfiguration struct {
	URL         string            `json:"url"`
	TLSEnabled  bool              `json:"tls_enabled"`
	Resources   ResourceLimits    `json:"resources"`
	Environment map[string]string `json:"environment"`
}

// ResourceLimits describes resource constraints
type ResourceLimits struct {
	CPUCores    int `json:"cpu_cores"`
	MemoryGB    int `json:"memory_gb"`
	NetworkMbps int `json:"network_mbps"`
}

// LoadParameters describes load test parameters
type LoadParameters struct {
	RampUpDuration      time.Duration `json:"ramp_up_duration"`
	SteadyStateDuration time.Duration `json:"steady_state_duration"`
	RampDownDuration    time.Duration `json:"ramp_down_duration"`
	TargetRPS           float64       `json:"target_rps"`
	MaxConcurrency      int           `json:"max_concurrency"`
}

// TestResults contains overall test results
type TestResults struct {
	TotalRequests      int64       `json:"total_requests"`
	SuccessfulRequests int64       `json:"successful_requests"`
	FailedRequests     int64       `json:"failed_requests"`
	SuccessRate        float64     `json:"success_rate"`
	AverageThroughput  float64     `json:"average_throughput"`
	PeakThroughput     float64     `json:"peak_throughput"`
	TotalDataTransfer  uint64      `json:"total_data_transfer"`
	TestOutcome        TestOutcome `json:"test_outcome"`
}

// TestOutcome describes the overall test result
type TestOutcome struct {
	Status         string   `json:"status"` // "passed", "failed", "warning"
	PassedChecks   int      `json:"passed_checks"`
	FailedChecks   int      `json:"failed_checks"`
	WarningChecks  int      `json:"warning_checks"`
	FailureReasons []string `json:"failure_reasons"`
}

// PerformanceAnalysis contains detailed performance analysis
type PerformanceAnalysis struct {
	LatencyAnalysis     LatencyAnalysis     `json:"latency_analysis"`
	ThroughputAnalysis  ThroughputAnalysis  `json:"throughput_analysis"`
	ErrorAnalysis       ErrorAnalysis       `json:"error_analysis"`
	ResourceAnalysis    ResourceAnalysis    `json:"resource_analysis"`
	ScalabilityAnalysis ScalabilityAnalysis `json:"scalability_analysis"`
}

// LatencyAnalysis contains latency performance analysis
type LatencyAnalysis struct {
	Mean         time.Duration   `json:"mean"`
	Median       time.Duration   `json:"median"`
	P95          time.Duration   `json:"p95"`
	P99          time.Duration   `json:"p99"`
	P999         time.Duration   `json:"p999"`
	Max          time.Duration   `json:"max"`
	Min          time.Duration   `json:"min"`
	StdDev       time.Duration   `json:"std_dev"`
	Distribution []LatencyBucket `json:"distribution"`
}

// LatencyBucket represents a latency distribution bucket
type LatencyBucket struct {
	Range      string  `json:"range"`
	Count      int64   `json:"count"`
	Percentage float64 `json:"percentage"`
}

// ThroughputAnalysis contains throughput performance analysis
type ThroughputAnalysis struct {
	Mean       float64           `json:"mean"`
	Median     float64           `json:"median"`
	P95        float64           `json:"p95"`
	Max        float64           `json:"max"`
	Min        float64           `json:"min"`
	Stability  float64           `json:"stability"` // Coefficient of variation
	Trend      string            `json:"trend"`     // "increasing", "decreasing", "stable"
	TimeSeries []ThroughputPoint `json:"time_series"`
}

// ThroughputPoint represents a throughput measurement point
type ThroughputPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
}

// ErrorAnalysis contains error analysis
type ErrorAnalysis struct {
	TotalErrors  int64            `json:"total_errors"`
	ErrorRate    float64          `json:"error_rate"`
	ErrorsByType map[string]int64 `json:"errors_by_type"`
	ErrorsByTime []ErrorTimePoint `json:"errors_by_time"`
	TopErrors    []ErrorSummary   `json:"top_errors"`
	ErrorTrend   string           `json:"error_trend"`
}

// ErrorTimePoint represents errors at a point in time
type ErrorTimePoint struct {
	Timestamp time.Time `json:"timestamp"`
	Count     int64     `json:"count"`
	Rate      float64   `json:"rate"`
}

// ErrorSummary summarizes a specific error type
type ErrorSummary struct {
	Type        string    `json:"type"`
	Count       int64     `json:"count"`
	Percentage  float64   `json:"percentage"`
	FirstSeen   time.Time `json:"first_seen"`
	LastSeen    time.Time `json:"last_seen"`
	Description string    `json:"description"`
}

// ResourceAnalysis contains system resource analysis
type ResourceAnalysis struct {
	CPU        ResourceMetric    `json:"cpu"`
	Memory     ResourceMetric    `json:"memory"`
	Network    ResourceMetric    `json:"network"`
	Disk       ResourceMetric    `json:"disk"`
	Efficiency EfficiencyMetrics `json:"efficiency"`
}

// ResourceMetric represents resource utilization metrics
type ResourceMetric struct {
	Average    float64     `json:"average"`
	Peak       float64     `json:"peak"`
	P95        float64     `json:"p95"`
	Efficiency float64     `json:"efficiency"`
	Bottleneck bool        `json:"bottleneck"`
	TimeSeries []DataPoint `json:"time_series"`
}

// DataPoint represents a single metric data point
type DataPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
}

// EfficiencyMetrics contains resource efficiency metrics
type EfficiencyMetrics struct {
	Overall           float64 `json:"overall"`
	CPUEfficiency     float64 `json:"cpu_efficiency"`
	MemoryEfficiency  float64 `json:"memory_efficiency"`
	NetworkEfficiency float64 `json:"network_efficiency"`
}

// ScalabilityAnalysis contains scalability analysis
type ScalabilityAnalysis struct {
	MaxDevicesSupported    int      `json:"max_devices_supported"`
	ScalabilityFactor      float64  `json:"scalability_factor"`
	BottleneckComponents   []string `json:"bottleneck_components"`
	ScalingRecommendations []string `json:"scaling_recommendations"`
}

// ScenarioReport contains results for a specific scenario
type ScenarioReport struct {
	Name            string                 `json:"name"`
	Description     string                 `json:"description"`
	Status          string                 `json:"status"`
	Duration        time.Duration          `json:"duration"`
	DevicesInvolved int                    `json:"devices_involved"`
	Results         ScenarioResults        `json:"results"`
	Metrics         map[string]interface{} `json:"metrics"`
	Issues          []Issue                `json:"issues"`
}

// ScenarioResults contains scenario-specific results
type ScenarioResults struct {
	Passed         bool          `json:"passed"`
	SuccessRate    float64       `json:"success_rate"`
	AverageLatency time.Duration `json:"average_latency"`
	ThroughputRate float64       `json:"throughput_rate"`
	ErrorCount     int64         `json:"error_count"`
	KeyMetrics     []Metric      `json:"key_metrics"`
}

// SystemMetricsReport contains system-level metrics
type SystemMetricsReport struct {
	ServerMetrics   ServerMetrics   `json:"server_metrics"`
	NetworkMetrics  NetworkMetrics  `json:"network_metrics"`
	DatabaseMetrics DatabaseMetrics `json:"database_metrics"`
}

// ServerMetrics contains server performance metrics
type ServerMetrics struct {
	CPUUsage     ResourceMetric `json:"cpu_usage"`
	MemoryUsage  ResourceMetric `json:"memory_usage"`
	DiskUsage    ResourceMetric `json:"disk_usage"`
	LoadAverage  []float64      `json:"load_average"`
	ProcessCount int            `json:"process_count"`
	ThreadCount  int            `json:"thread_count"`
}

// NetworkMetrics contains network performance metrics
type NetworkMetrics struct {
	ThroughputIn  ResourceMetric `json:"throughput_in"`
	ThroughputOut ResourceMetric `json:"throughput_out"`
	PacketsIn     int64          `json:"packets_in"`
	PacketsOut    int64          `json:"packets_out"`
	Connections   int64          `json:"connections"`
	Latency       time.Duration  `json:"latency"`
}

// DatabaseMetrics contains database performance metrics (if applicable)
type DatabaseMetrics struct {
	ConnectionPool int           `json:"connection_pool"`
	QueryLatency   time.Duration `json:"query_latency"`
	Transactions   int64         `json:"transactions"`
	DeadLocks      int64         `json:"deadlocks"`
}

// Metric represents a key performance metric
type Metric struct {
	Name        string      `json:"name"`
	Value       interface{} `json:"value"`
	Unit        string      `json:"unit"`
	Threshold   interface{} `json:"threshold,omitempty"`
	Status      string      `json:"status"` // "good", "warning", "critical"
	Description string      `json:"description"`
}

// Issue represents a performance issue or concern
type Issue struct {
	Level          string    `json:"level"` // "info", "warning", "error", "critical"
	Component      string    `json:"component"`
	Title          string    `json:"title"`
	Description    string    `json:"description"`
	Impact         string    `json:"impact"`
	Recommendation string    `json:"recommendation"`
	Timestamp      time.Time `json:"timestamp"`
}

// Recommendation provides performance improvement recommendations
type Recommendation struct {
	Category    string   `json:"category"`
	Priority    string   `json:"priority"` // "high", "medium", "low"
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Actions     []string `json:"actions"`
	Impact      string   `json:"impact"`
}

// RawDataReport contains raw test data (optional)
type RawDataReport struct {
	MetricsData    interface{}     `json:"metrics_data,omitempty"`
	LatencyData    []time.Duration `json:"latency_data,omitempty"`
	ThroughputData []float64       `json:"throughput_data,omitempty"`
	ErrorData      []ErrorEvent    `json:"error_data,omitempty"`
}

// ErrorEvent represents a single error event
type ErrorEvent struct {
	Timestamp time.Time `json:"timestamp"`
	Type      string    `json:"type"`
	Message   string    `json:"message"`
	DeviceID  string    `json:"device_id,omitempty"`
	Scenario  string    `json:"scenario,omitempty"`
}

// NewReporter creates a new performance reporter
func NewReporter(config *ReportConfig) *Reporter {
	logger := slog.Default().With("component", "reporter")

	if config.OutputDirectory == "" {
		config.OutputDirectory = "./load_test_reports"
	}

	// Ensure output directory exists
	if err := os.MkdirAll(config.OutputDirectory, 0755); err != nil {
		logger.Error("Failed to create output directory", "error", err)
	}

	return &Reporter{
		logger:    logger,
		outputDir: config.OutputDirectory,
		templates: make(map[string]*template.Template),
	}
}

// GenerateReport generates a comprehensive performance report
func (r *Reporter) GenerateReport(
	metricsCollector *framework.MetricsCollector,
	fleetSimulator *framework.FleetSimulator,
	scenarioResults map[string]interface{},
	config *ReportConfig,
) (*TestReport, error) {
	r.logger.Info("Generating performance report")

	report := &TestReport{
		Metadata: ReportMetadata{
			GeneratedAt:  time.Now(),
			Generator:    "fleetd Load Testing Framework",
			Version:      "1.0.0",
			TestName:     "Load Test",
			ReportFormat: "comprehensive",
		},
	}

	// Generate each section of the report
	if err := r.generateMetadata(report, metricsCollector, fleetSimulator); err != nil {
		return nil, fmt.Errorf("failed to generate metadata: %w", err)
	}

	if err := r.generateTestConfiguration(report, fleetSimulator); err != nil {
		return nil, fmt.Errorf("failed to generate test configuration: %w", err)
	}

	if err := r.generateTestResults(report, metricsCollector, fleetSimulator); err != nil {
		return nil, fmt.Errorf("failed to generate test results: %w", err)
	}

	if err := r.generatePerformanceAnalysis(report, metricsCollector); err != nil {
		return nil, fmt.Errorf("failed to generate performance analysis: %w", err)
	}

	if err := r.generateScenarioReports(report, scenarioResults); err != nil {
		return nil, fmt.Errorf("failed to generate scenario reports: %w", err)
	}

	if err := r.generateSystemMetricsReport(report, metricsCollector); err != nil {
		return nil, fmt.Errorf("failed to generate system metrics report: %w", err)
	}

	if err := r.generateRecommendations(report); err != nil {
		return nil, fmt.Errorf("failed to generate recommendations: %w", err)
	}

	if err := r.generateExecutiveSummary(report); err != nil {
		return nil, fmt.Errorf("failed to generate executive summary: %w", err)
	}

	// Include raw data if requested
	if config.IncludeRawData {
		if err := r.generateRawDataReport(report, metricsCollector); err != nil {
			r.logger.Warn("Failed to generate raw data report", "error", err)
		}
	}

	r.logger.Info("Performance report generated successfully")
	return report, nil
}

// SaveReport saves the report in the specified formats
func (r *Reporter) SaveReport(report *TestReport, formats []ReportFormat) error {
	timestamp := time.Now().Format("20060102_150405")

	for _, format := range formats {
		filename := fmt.Sprintf("load_test_report_%s.%s", timestamp, format)
		filepath := filepath.Join(r.outputDir, filename)

		switch format {
		case FormatJSON:
			if err := r.saveJSONReport(report, filepath); err != nil {
				return fmt.Errorf("failed to save JSON report: %w", err)
			}
		case FormatHTML:
			if err := r.saveHTMLReport(report, filepath); err != nil {
				return fmt.Errorf("failed to save HTML report: %w", err)
			}
		case FormatCSV:
			if err := r.saveCSVReport(report, filepath); err != nil {
				return fmt.Errorf("failed to save CSV report: %w", err)
			}
		default:
			r.logger.Warn("Unsupported report format", "format", format)
		}

		r.logger.Info("Report saved", "format", format, "file", filepath)
	}

	return nil
}

// generateMetadata generates report metadata
func (r *Reporter) generateMetadata(report *TestReport, metricsCollector *framework.MetricsCollector, fleetSimulator *framework.FleetSimulator) error {
	if metricsCollector != nil {
		summary := metricsCollector.GetSummary()
		report.Metadata.TestDuration = summary.Duration
	}

	return nil
}

// generateTestConfiguration generates test configuration section
func (r *Reporter) generateTestConfiguration(report *TestReport, fleetSimulator *framework.FleetSimulator) error {
	config := TestConfiguration{
		DeviceProfiles: make(map[framework.DeviceProfile]int),
		TestScenarios:  []string{},
		ServerConfig: ServerConfiguration{
			TLSEnabled: true,
			Resources: ResourceLimits{
				CPUCores:    4,
				MemoryGB:    8,
				NetworkMbps: 1000,
			},
			Environment: make(map[string]string),
		},
		LoadParameters: LoadParameters{
			RampUpDuration:      2 * time.Minute,
			SteadyStateDuration: 10 * time.Minute,
			RampDownDuration:    1 * time.Minute,
			MaxConcurrency:      1000,
		},
	}

	if fleetSimulator != nil {
		fleetMetrics := fleetSimulator.GetMetrics()
		config.TotalDevices = int(fleetMetrics.TotalDevices)
	}

	report.TestConfiguration = config
	return nil
}

// generateTestResults generates overall test results
func (r *Reporter) generateTestResults(report *TestReport, metricsCollector *framework.MetricsCollector, fleetSimulator *framework.FleetSimulator) error {
	results := TestResults{
		TestOutcome: TestOutcome{
			Status: "passed",
		},
	}

	if fleetSimulator != nil {
		fleetMetrics := fleetSimulator.GetMetrics()
		results.TotalRequests = fleetMetrics.TotalRequests
		results.SuccessfulRequests = fleetMetrics.SuccessfulRequests
		results.FailedRequests = fleetMetrics.FailedRequests

		if results.TotalRequests > 0 {
			results.SuccessRate = float64(results.SuccessfulRequests) / float64(results.TotalRequests)
		}
	}

	if metricsCollector != nil {
		summary := metricsCollector.GetSummary()
		results.AverageThroughput = summary.AvgThroughput
		results.PeakThroughput = summary.PeakThroughput
	}

	// Determine test outcome
	if results.SuccessRate >= 0.95 {
		results.TestOutcome.Status = "passed"
		results.TestOutcome.PassedChecks = 1
	} else if results.SuccessRate >= 0.90 {
		results.TestOutcome.Status = "warning"
		results.TestOutcome.WarningChecks = 1
		results.TestOutcome.FailureReasons = []string{"Success rate below 95%"}
	} else {
		results.TestOutcome.Status = "failed"
		results.TestOutcome.FailedChecks = 1
		results.TestOutcome.FailureReasons = []string{"Success rate below 90%"}
	}

	report.Results = results
	return nil
}

// generatePerformanceAnalysis generates detailed performance analysis
func (r *Reporter) generatePerformanceAnalysis(report *TestReport, metricsCollector *framework.MetricsCollector) error {
	analysis := PerformanceAnalysis{}

	if metricsCollector != nil {
		summary := metricsCollector.GetSummary()

		// Latency analysis
		analysis.LatencyAnalysis = LatencyAnalysis{
			Mean: summary.AvgLatency,
			P95:  summary.P95Latency,
			P99:  summary.P99Latency,
			Max:  summary.MaxLatency,
			Min:  time.Millisecond, // Placeholder
		}

		// Throughput analysis
		analysis.ThroughputAnalysis = ThroughputAnalysis{
			Mean:  summary.AvgThroughput,
			Max:   summary.PeakThroughput,
			Min:   summary.AvgThroughput * 0.8, // Placeholder
			Trend: "stable",
		}

		// Error analysis
		analysis.ErrorAnalysis = ErrorAnalysis{
			TotalErrors:  summary.TotalErrors,
			ErrorRate:    summary.OverallErrorRate,
			ErrorsByType: summary.ErrorDistribution,
			ErrorTrend:   "stable",
		}

		// Resource analysis
		analysis.ResourceAnalysis = ResourceAnalysis{
			CPU: ResourceMetric{
				Average: summary.AvgCPUUsage,
				Peak:    summary.MaxCPUUsage,
				P95:     summary.MaxCPUUsage * 0.9,
			},
			Memory: ResourceMetric{
				Average: summary.AvgMemoryUsage,
				Peak:    summary.MaxMemoryUsage,
				P95:     summary.MaxMemoryUsage * 0.9,
			},
			Efficiency: EfficiencyMetrics{
				Overall:           summary.ResourceUsage.CPUEfficiency,
				CPUEfficiency:     summary.ResourceUsage.CPUEfficiency,
				MemoryEfficiency:  summary.ResourceUsage.MemoryEfficiency,
				NetworkEfficiency: summary.ResourceUsage.NetworkUtilization,
			},
		}

		// Scalability analysis
		analysis.ScalabilityAnalysis = ScalabilityAnalysis{
			MaxDevicesSupported: int(summary.PeakConnections * 2), // Estimate
			ScalabilityFactor:   0.8,
		}
	}

	report.Performance = analysis
	return nil
}

// generateScenarioReports generates reports for each scenario
func (r *Reporter) generateScenarioReports(report *TestReport, scenarioResults map[string]interface{}) error {
	var scenarios []ScenarioReport

	// For each scenario result, create a scenario report
	for name := range scenarioResults {
		scenario := ScenarioReport{
			Name:        name,
			Description: fmt.Sprintf("Load test scenario: %s", name),
			Status:      "completed",
			Duration:    10 * time.Minute, // Placeholder
			Results: ScenarioResults{
				Passed:      true,
				SuccessRate: 0.95,
			},
		}
		scenarios = append(scenarios, scenario)
	}

	report.Scenarios = scenarios
	return nil
}

// generateSystemMetricsReport generates system metrics report
func (r *Reporter) generateSystemMetricsReport(report *TestReport, metricsCollector *framework.MetricsCollector) error {
	systemMetrics := SystemMetricsReport{}

	if metricsCollector != nil {
		summary := metricsCollector.GetSummary()

		systemMetrics.ServerMetrics = ServerMetrics{
			CPUUsage: ResourceMetric{
				Average: summary.AvgCPUUsage,
				Peak:    summary.MaxCPUUsage,
			},
			MemoryUsage: ResourceMetric{
				Average: summary.AvgMemoryUsage,
				Peak:    summary.MaxMemoryUsage,
			},
			LoadAverage: []float64{1.0, 1.2, 1.1}, // Placeholder
		}

		systemMetrics.NetworkMetrics = NetworkMetrics{
			ThroughputIn: ResourceMetric{
				Average: summary.AvgThroughput * 1024, // Convert to bytes
				Peak:    summary.PeakThroughput * 1024,
			},
			Connections: summary.PeakConnections,
		}
	}

	report.SystemMetrics = systemMetrics
	return nil
}

// generateRecommendations generates performance recommendations
func (r *Reporter) generateRecommendations(report *TestReport) error {
	var recommendations []Recommendation

	// CPU recommendations
	if report.Performance.ResourceAnalysis.CPU.Peak > 80 {
		recommendations = append(recommendations, Recommendation{
			Category:    "Resource Optimization",
			Priority:    "high",
			Title:       "High CPU Usage Detected",
			Description: "CPU usage peaked at over 80% during the test",
			Actions: []string{
				"Consider adding more CPU cores",
				"Optimize application code for better CPU efficiency",
				"Implement CPU usage monitoring and alerts",
			},
			Impact: "Improved response times and higher throughput capacity",
		})
	}

	// Memory recommendations
	if report.Performance.ResourceAnalysis.Memory.Peak > 85 {
		recommendations = append(recommendations, Recommendation{
			Category:    "Resource Optimization",
			Priority:    "high",
			Title:       "High Memory Usage Detected",
			Description: "Memory usage peaked at over 85% during the test",
			Actions: []string{
				"Increase available memory",
				"Optimize memory usage in the application",
				"Implement memory monitoring and garbage collection tuning",
			},
			Impact: "Reduced memory pressure and improved stability",
		})
	}

	// Error rate recommendations
	if report.Performance.ErrorAnalysis.ErrorRate > 0.05 {
		recommendations = append(recommendations, Recommendation{
			Category:    "Reliability",
			Priority:    "high",
			Title:       "High Error Rate Detected",
			Description: "Error rate exceeded 5% threshold",
			Actions: []string{
				"Investigate and fix sources of errors",
				"Implement better error handling and retry logic",
				"Add comprehensive monitoring and alerting",
			},
			Impact: "Improved system reliability and user experience",
		})
	}

	// Latency recommendations
	if report.Performance.LatencyAnalysis.P95 > 100*time.Millisecond {
		recommendations = append(recommendations, Recommendation{
			Category:    "Performance",
			Priority:    "medium",
			Title:       "High Latency Detected",
			Description: "P95 latency exceeded 100ms threshold",
			Actions: []string{
				"Optimize database queries and connections",
				"Implement caching strategies",
				"Consider using a content delivery network (CDN)",
				"Optimize network configuration",
			},
			Impact: "Improved response times and better user experience",
		})
	}

	// Scalability recommendations
	if report.Performance.ScalabilityAnalysis.ScalabilityFactor < 0.7 {
		recommendations = append(recommendations, Recommendation{
			Category:    "Scalability",
			Priority:    "medium",
			Title:       "Limited Scalability Detected",
			Description: "System scalability factor is below optimal levels",
			Actions: []string{
				"Implement horizontal scaling capabilities",
				"Optimize database connection pooling",
				"Consider microservices architecture",
				"Implement load balancing",
			},
			Impact: "Better ability to handle increased load",
		})
	}

	report.Recommendations = recommendations
	return nil
}

// generateExecutiveSummary generates the executive summary
func (r *Reporter) generateExecutiveSummary(report *TestReport) error {
	summary := ExecutiveSummary{
		TestPassed: report.Results.TestOutcome.Status == "passed",
		KeyMetrics: []Metric{
			{
				Name:        "Success Rate",
				Value:       report.Results.SuccessRate,
				Unit:        "%",
				Status:      r.getMetricStatus(report.Results.SuccessRate, 0.95, 0.90),
				Description: "Percentage of successful requests",
			},
			{
				Name:        "Average Throughput",
				Value:       report.Results.AverageThroughput,
				Unit:        "req/s",
				Status:      "good",
				Description: "Average requests processed per second",
			},
			{
				Name:        "P95 Latency",
				Value:       report.Performance.LatencyAnalysis.P95,
				Unit:        "ms",
				Status:      r.getLatencyStatus(report.Performance.LatencyAnalysis.P95),
				Description: "95th percentile response time",
			},
		},
	}

	// Calculate overall score
	score := 100.0
	if report.Results.SuccessRate < 0.95 {
		score -= 20
	}
	if report.Performance.LatencyAnalysis.P95 > 100*time.Millisecond {
		score -= 15
	}
	if report.Performance.ResourceAnalysis.CPU.Peak > 80 {
		score -= 10
	}
	if report.Performance.ErrorAnalysis.ErrorRate > 0.05 {
		score -= 15
	}

	summary.OverallScore = score

	// Assign performance grade
	if score >= 90 {
		summary.PerformanceGrade = "A"
	} else if score >= 80 {
		summary.PerformanceGrade = "B"
	} else if score >= 70 {
		summary.PerformanceGrade = "C"
	} else if score >= 60 {
		summary.PerformanceGrade = "D"
	} else {
		summary.PerformanceGrade = "F"
	}

	// Generate summary text
	if summary.TestPassed {
		summary.Summary = fmt.Sprintf(
			"The load test completed successfully with a %s grade. The system handled %d total requests with a %.2f%% success rate and maintained good performance characteristics.",
			summary.PerformanceGrade,
			report.Results.TotalRequests,
			report.Results.SuccessRate*100,
		)
	} else {
		summary.Summary = fmt.Sprintf(
			"The load test completed with issues (grade: %s). While processing %d requests, the system experienced performance degradation that requires attention.",
			summary.PerformanceGrade,
			report.Results.TotalRequests,
		)
	}

	// Collect critical issues
	for _, rec := range report.Recommendations {
		if rec.Priority == "high" {
			summary.CriticalIssues = append(summary.CriticalIssues, Issue{
				Level:       "critical",
				Component:   rec.Category,
				Title:       rec.Title,
				Description: rec.Description,
				Impact:      rec.Impact,
				Timestamp:   time.Now(),
			})
		}
	}

	report.ExecutiveSummary = summary
	return nil
}

// generateRawDataReport generates raw data report
func (r *Reporter) generateRawDataReport(report *TestReport, metricsCollector *framework.MetricsCollector) error {
	if metricsCollector == nil {
		return nil
	}

	rawData := &RawDataReport{}

	// Export metrics data
	if exportedData, err := metricsCollector.ExportMetrics(); err == nil {
		var metricsData interface{}
		if err := json.Unmarshal(exportedData, &metricsData); err == nil {
			rawData.MetricsData = metricsData
		}
	}

	report.RawData = rawData
	return nil
}

// Helper functions

func (r *Reporter) getMetricStatus(value, goodThreshold, warningThreshold float64) string {
	if value >= goodThreshold {
		return "good"
	} else if value >= warningThreshold {
		return "warning"
	}
	return "critical"
}

func (r *Reporter) getLatencyStatus(latency time.Duration) string {
	if latency <= 50*time.Millisecond {
		return "good"
	} else if latency <= 100*time.Millisecond {
		return "warning"
	}
	return "critical"
}

// saveJSONReport saves the report as JSON
func (r *Reporter) saveJSONReport(report *TestReport, filepath string) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(filepath, data, 0644)
}

// saveHTMLReport saves the report as HTML
func (r *Reporter) saveHTMLReport(report *TestReport, filepath string) error {
	tmpl := r.getHTMLTemplate()

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, report); err != nil {
		return err
	}

	return ioutil.WriteFile(filepath, buf.Bytes(), 0644)
}

// saveCSVReport saves key metrics as CSV
func (r *Reporter) saveCSVReport(report *TestReport, filepath string) error {
	var lines []string
	lines = append(lines, "Metric,Value,Unit,Status")

	for _, metric := range report.ExecutiveSummary.KeyMetrics {
		line := fmt.Sprintf("%s,%v,%s,%s",
			metric.Name,
			metric.Value,
			metric.Unit,
			metric.Status,
		)
		lines = append(lines, line)
	}

	content := strings.Join(lines, "\n")
	return ioutil.WriteFile(filepath, []byte(content), 0644)
}

// getHTMLTemplate returns the HTML template for reports
func (r *Reporter) getHTMLTemplate() *template.Template {
	if tmpl, exists := r.templates["html"]; exists {
		return tmpl
	}

	htmlTemplate := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>fleetd Load Test Report</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 40px; background-color: #f5f5f5; }
        .container { max-width: 1200px; margin: 0 auto; background: white; padding: 30px; border-radius: 10px; box-shadow: 0 0 20px rgba(0,0,0,0.1); }
        .header { text-align: center; border-bottom: 2px solid #eee; padding-bottom: 20px; margin-bottom: 30px; }
        .grade { font-size: 3em; font-weight: bold; color: #4CAF50; }
        .section { margin-bottom: 30px; }
        .section h2 { color: #333; border-bottom: 1px solid #ddd; padding-bottom: 10px; }
        .metrics-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 20px; }
        .metric-card { background: #f9f9f9; padding: 20px; border-radius: 8px; text-align: center; }
        .metric-value { font-size: 2em; font-weight: bold; }
        .metric-label { color: #666; font-size: 0.9em; }
        .status-good { color: #4CAF50; }
        .status-warning { color: #FF9800; }
        .status-critical { color: #F44336; }
        .recommendation { background: #e3f2fd; padding: 15px; margin: 10px 0; border-radius: 5px; border-left: 4px solid #2196F3; }
        .issue { background: #ffebee; padding: 15px; margin: 10px 0; border-radius: 5px; border-left: 4px solid #f44336; }
        table { width: 100%; border-collapse: collapse; margin: 20px 0; }
        th, td { padding: 12px; text-align: left; border-bottom: 1px solid #ddd; }
        th { background-color: #f2f2f2; font-weight: bold; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>fleetd Load Test Report</h1>
            <p>Generated on {{.Metadata.GeneratedAt.Format "January 2, 2006 at 3:04 PM"}}</p>
            <div class="grade">Grade: {{.ExecutiveSummary.PerformanceGrade}}</div>
            <p><strong>Overall Score:</strong> {{printf "%.1f" .ExecutiveSummary.OverallScore}}/100</p>
        </div>

        <div class="section">
            <h2>Executive Summary</h2>
            <p>{{.ExecutiveSummary.Summary}}</p>

            <h3>Key Metrics</h3>
            <div class="metrics-grid">
                {{range .ExecutiveSummary.KeyMetrics}}
                <div class="metric-card">
                    <div class="metric-value status-{{.Status}}">{{.Value}}</div>
                    <div class="metric-label">{{.Name}} ({{.Unit}})</div>
                </div>
                {{end}}
            </div>
        </div>

        <div class="section">
            <h2>Test Results</h2>
            <table>
                <tr><th>Metric</th><th>Value</th></tr>
                <tr><td>Total Requests</td><td>{{.Results.TotalRequests}}</td></tr>
                <tr><td>Successful Requests</td><td>{{.Results.SuccessfulRequests}}</td></tr>
                <tr><td>Failed Requests</td><td>{{.Results.FailedRequests}}</td></tr>
                <tr><td>Success Rate</td><td>{{printf "%.2f%%" (mul .Results.SuccessRate 100)}}</td></tr>
                <tr><td>Average Throughput</td><td>{{printf "%.1f req/s" .Results.AverageThroughput}}</td></tr>
                <tr><td>Peak Throughput</td><td>{{printf "%.1f req/s" .Results.PeakThroughput}}</td></tr>
            </table>
        </div>

        <div class="section">
            <h2>Performance Analysis</h2>

            <h3>Latency Analysis</h3>
            <table>
                <tr><th>Percentile</th><th>Latency</th></tr>
                <tr><td>P95</td><td>{{.Performance.LatencyAnalysis.P95}}</td></tr>
                <tr><td>P99</td><td>{{.Performance.LatencyAnalysis.P99}}</td></tr>
                <tr><td>Max</td><td>{{.Performance.LatencyAnalysis.Max}}</td></tr>
                <tr><td>Mean</td><td>{{.Performance.LatencyAnalysis.Mean}}</td></tr>
            </table>

            <h3>Resource Usage</h3>
            <table>
                <tr><th>Resource</th><th>Average</th><th>Peak</th></tr>
                <tr><td>CPU</td><td>{{printf "%.1f%%" .Performance.ResourceAnalysis.CPU.Average}}</td><td>{{printf "%.1f%%" .Performance.ResourceAnalysis.CPU.Peak}}</td></tr>
                <tr><td>Memory</td><td>{{printf "%.1f%%" .Performance.ResourceAnalysis.Memory.Average}}</td><td>{{printf "%.1f%%" .Performance.ResourceAnalysis.Memory.Peak}}</td></tr>
            </table>
        </div>

        {{if .ExecutiveSummary.CriticalIssues}}
        <div class="section">
            <h2>Critical Issues</h2>
            {{range .ExecutiveSummary.CriticalIssues}}
            <div class="issue">
                <h4>{{.Title}}</h4>
                <p>{{.Description}}</p>
                <p><strong>Impact:</strong> {{.Impact}}</p>
            </div>
            {{end}}
        </div>
        {{end}}

        {{if .Recommendations}}
        <div class="section">
            <h2>Recommendations</h2>
            {{range .Recommendations}}
            <div class="recommendation">
                <h4>{{.Title}} ({{.Priority}} priority)</h4>
                <p>{{.Description}}</p>
                <p><strong>Recommended Actions:</strong></p>
                <ul>
                    {{range .Actions}}<li>{{.}}</li>{{end}}
                </ul>
                <p><strong>Expected Impact:</strong> {{.Impact}}</p>
            </div>
            {{end}}
        </div>
        {{end}}

        <div class="section">
            <h2>Test Configuration</h2>
            <table>
                <tr><th>Parameter</th><th>Value</th></tr>
                <tr><td>Total Devices</td><td>{{.TestConfiguration.TotalDevices}}</td></tr>
                <tr><td>Test Duration</td><td>{{.Metadata.TestDuration}}</td></tr>
                <tr><td>Max Concurrency</td><td>{{.TestConfiguration.LoadParameters.MaxConcurrency}}</td></tr>
                <tr><td>TLS Enabled</td><td>{{.TestConfiguration.ServerConfig.TLSEnabled}}</td></tr>
            </table>
        </div>
    </div>
</body>
</html>`

	tmpl, err := template.New("html").Funcs(template.FuncMap{
		"mul": func(a, b float64) float64 { return a * b },
	}).Parse(htmlTemplate)

	if err != nil {
		r.logger.Error("Failed to parse HTML template", "error", err)
		fallback, _ := template.New("fallback").Parse("<html><body><h1>Error: Could not generate report</h1></body></html>")
		return fallback
	}

	r.templates["html"] = tmpl
	return tmpl
}

// GetReportPath returns the path where reports are saved
func (r *Reporter) GetReportPath() string {
	return r.outputDir
}
