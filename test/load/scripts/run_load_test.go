package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"fleetd.sh/test/load/dashboard"
	"fleetd.sh/test/load/framework"
	"fleetd.sh/test/load/reports"
	"fleetd.sh/test/load/scenarios"
)

// TestConfig holds the configuration for a load test
type TestConfig struct {
	// Basic configuration
	ServerURL        string
	TotalDevices     int
	TestDuration     time.Duration
	OutputDir        string

	// Device profiles
	FullDevices        int
	ConstrainedDevices int
	MinimalDevices     int

	// Scenario selection
	RunOnboarding     bool
	RunSteadyState    bool
	RunUpdateCampaign bool
	RunNetworkResilience bool

	// Advanced options
	DashboardPort    int
	EnableDashboard  bool
	ReportFormats    []reports.ReportFormat
	MetricsInterval  time.Duration
	Verbose          bool
	AuthToken        string
	TLSEnabled       bool

	// Performance targets
	TargetRPS        float64
	MaxLatency       time.Duration
	MinSuccessRate   float64
}

func main() {
	config := parseFlags()

	if config.Verbose {
		log.SetFlags(log.LstdFlags | log.Lshortfile)
		log.Println("Starting fleetd load testing framework")
		printConfig(config)
	}

	// Validate configuration
	if err := validateConfig(config); err != nil {
		log.Fatalf("Configuration error: %v", err)
	}

	// Create output directory
	if err := os.MkdirAll(config.OutputDir, 0755); err != nil {
		log.Fatalf("Failed to create output directory: %v", err)
	}

	// Run the load test
	if err := runLoadTest(config); err != nil {
		log.Fatalf("Load test failed: %v", err)
	}

	log.Println("Load test completed successfully!")
}

func parseFlags() *TestConfig {
	config := &TestConfig{
		ReportFormats: []reports.ReportFormat{reports.FormatHTML, reports.FormatJSON},
	}

	// Basic flags
	flag.StringVar(&config.ServerURL, "server", "http://localhost:8080", "Server URL to test")
	flag.IntVar(&config.TotalDevices, "devices", 100, "Total number of devices to simulate")
	flag.DurationVar(&config.TestDuration, "duration", 10*time.Minute, "Test duration")
	flag.StringVar(&config.OutputDir, "output", "./load_test_results", "Output directory for results")

	// Device profile flags
	flag.IntVar(&config.FullDevices, "full-devices", 0, "Number of full-featured devices (0 = auto)")
	flag.IntVar(&config.ConstrainedDevices, "constrained-devices", 0, "Number of constrained devices (0 = auto)")
	flag.IntVar(&config.MinimalDevices, "minimal-devices", 0, "Number of minimal devices (0 = auto)")

	// Scenario flags
	flag.BoolVar(&config.RunOnboarding, "onboarding", true, "Run onboarding storm scenario")
	flag.BoolVar(&config.RunSteadyState, "steady-state", true, "Run steady state scenario")
	flag.BoolVar(&config.RunUpdateCampaign, "update-campaign", false, "Run update campaign scenario")
	flag.BoolVar(&config.RunNetworkResilience, "network-resilience", false, "Run network resilience scenario")

	// Advanced flags
	flag.IntVar(&config.DashboardPort, "dashboard-port", 8080, "Dashboard port")
	flag.BoolVar(&config.EnableDashboard, "dashboard", true, "Enable real-time dashboard")
	flag.DurationVar(&config.MetricsInterval, "metrics-interval", 5*time.Second, "Metrics collection interval")
	flag.BoolVar(&config.Verbose, "verbose", false, "Enable verbose logging")
	flag.StringVar(&config.AuthToken, "auth-token", "", "Authentication token")
	flag.BoolVar(&config.TLSEnabled, "tls", false, "Enable TLS")

	// Performance target flags
	flag.Float64Var(&config.TargetRPS, "target-rps", 1000, "Target requests per second")
	flag.DurationVar(&config.MaxLatency, "max-latency", 100*time.Millisecond, "Maximum acceptable latency")
	flag.Float64Var(&config.MinSuccessRate, "min-success-rate", 0.95, "Minimum success rate")

	// Report format flag
	var reportFormats string
	flag.StringVar(&reportFormats, "report-formats", "html,json", "Report formats (html,json,csv)")

	flag.Parse()

	// Parse report formats
	if reportFormats != "" {
		config.ReportFormats = []reports.ReportFormat{}
		for _, format := range strings.Split(reportFormats, ",") {
			switch strings.TrimSpace(format) {
			case "html":
				config.ReportFormats = append(config.ReportFormats, reports.FormatHTML)
			case "json":
				config.ReportFormats = append(config.ReportFormats, reports.FormatJSON)
			case "csv":
				config.ReportFormats = append(config.ReportFormats, reports.FormatCSV)
			default:
				log.Printf("Warning: Unknown report format '%s'", format)
			}
		}
	}

	return config
}

func validateConfig(config *TestConfig) error {
	if config.TotalDevices <= 0 {
		return fmt.Errorf("total devices must be greater than 0")
	}

	if config.TestDuration <= 0 {
		return fmt.Errorf("test duration must be greater than 0")
	}

	if config.ServerURL == "" {
		return fmt.Errorf("server URL cannot be empty")
	}

	if config.TargetRPS <= 0 {
		return fmt.Errorf("target RPS must be greater than 0")
	}

	if config.MinSuccessRate < 0 || config.MinSuccessRate > 1 {
		return fmt.Errorf("minimum success rate must be between 0 and 1")
	}

	// Set device profile defaults if not specified
	if config.FullDevices == 0 && config.ConstrainedDevices == 0 && config.MinimalDevices == 0 {
		config.FullDevices = int(float64(config.TotalDevices) * 0.3)
		config.ConstrainedDevices = int(float64(config.TotalDevices) * 0.5)
		config.MinimalDevices = config.TotalDevices - config.FullDevices - config.ConstrainedDevices
	}

	// Ensure device counts add up
	total := config.FullDevices + config.ConstrainedDevices + config.MinimalDevices
	if total != config.TotalDevices {
		return fmt.Errorf("device profile counts (%d) don't match total devices (%d)", total, config.TotalDevices)
	}

	return nil
}

func printConfig(config *TestConfig) {
	log.Printf("=== Load Test Configuration ===")
	log.Printf("Server URL: %s", config.ServerURL)
	log.Printf("Total Devices: %d", config.TotalDevices)
	log.Printf("  - Full devices: %d", config.FullDevices)
	log.Printf("  - Constrained devices: %d", config.ConstrainedDevices)
	log.Printf("  - Minimal devices: %d", config.MinimalDevices)
	log.Printf("Test Duration: %v", config.TestDuration)
	log.Printf("Target RPS: %.1f", config.TargetRPS)
	log.Printf("Max Latency: %v", config.MaxLatency)
	log.Printf("Min Success Rate: %.2f%%", config.MinSuccessRate*100)
	log.Printf("Dashboard: %v (port %d)", config.EnableDashboard, config.DashboardPort)
	log.Printf("Output Directory: %s", config.OutputDir)
	log.Printf("Report Formats: %v", config.ReportFormats)
	log.Printf("===============================")
}

func runLoadTest(config *TestConfig) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupt signals
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-signalChan
		log.Println("Received interrupt signal, shutting down...")
		cancel()
	}()

	// Create metrics collector
	metricsCollector := framework.NewMetricsCollector()
	if err := metricsCollector.Start(); err != nil {
		return fmt.Errorf("failed to start metrics collector: %w", err)
	}
	defer metricsCollector.Stop()

	// Create dashboard if enabled
	var dashboardInstance *dashboard.Dashboard
	if config.EnableDashboard {
		dashboardConfig := &dashboard.DashboardConfig{
			Port:             config.DashboardPort,
			MetricsCollector: metricsCollector,
		}
		dashboardInstance = dashboard.NewDashboard(dashboardConfig)

		if err := dashboardInstance.Start(); err != nil {
			log.Printf("Warning: Failed to start dashboard: %v", err)
		} else {
			log.Printf("Dashboard available at: %s", dashboardInstance.GetURL())
			defer dashboardInstance.Stop()
		}
	}

	// Create fleet simulator
	deviceProfiles := map[framework.DeviceProfile]int{
		framework.ProfileFull:        config.FullDevices,
		framework.ProfileConstrained: config.ConstrainedDevices,
		framework.ProfileMinimal:     config.MinimalDevices,
	}

	fleetConfig := &framework.FleetConfig{
		ServerURL:             config.ServerURL,
		TotalDevices:          config.TotalDevices,
		DeviceProfiles:        deviceProfiles,
		StartupBatchSize:      50,
		StartupBatchInterval:  2 * time.Second,
		MaxConcurrentRequests: 500,
		TestDuration:          config.TestDuration,
		AuthToken:             config.AuthToken,
		TLSEnabled:            config.TLSEnabled,
	}

	fleet := framework.NewFleetSimulator(fleetConfig)

	// Update dashboard with fleet simulator
	if dashboardInstance != nil {
		// This would require updating the dashboard to accept fleet simulator
		// For now, we'll log the dashboard URL
		log.Printf("Dashboard running at: %s", dashboardInstance.GetURL())
	}

	// Results storage
	scenarioResults := make(map[string]interface{})

	// Run scenarios
	if config.RunOnboarding {
		log.Println("Running onboarding storm scenario...")
		if err := runOnboardingScenario(ctx, config, fleet, scenarioResults); err != nil {
			log.Printf("Onboarding scenario failed: %v", err)
		}
	}

	if config.RunSteadyState {
		log.Println("Running steady state scenario...")
		if err := runSteadyStateScenario(ctx, config, fleet, scenarioResults); err != nil {
			log.Printf("Steady state scenario failed: %v", err)
		}
	}

	if config.RunUpdateCampaign {
		log.Println("Running update campaign scenario...")
		if err := runUpdateCampaignScenario(ctx, config, fleet, scenarioResults); err != nil {
			log.Printf("Update campaign scenario failed: %v", err)
		}
	}

	if config.RunNetworkResilience {
		log.Println("Running network resilience scenario...")
		if err := runNetworkResilienceScenario(ctx, config, fleet, scenarioResults); err != nil {
			log.Printf("Network resilience scenario failed: %v", err)
		}
	}

	// Generate reports
	log.Println("Generating performance reports...")
	if err := generateReports(config, metricsCollector, fleet, scenarioResults); err != nil {
		log.Printf("Warning: Failed to generate reports: %v", err)
	}

	return nil
}

func runOnboardingScenario(ctx context.Context, config *TestConfig, fleet *framework.FleetSimulator, results map[string]interface{}) error {
	scenarioConfig := &scenarios.OnboardingStormConfig{
		TotalDevices:     config.TotalDevices,
		DevicesPerSecond: 10,
		BurstSize:        50,
		BurstInterval:    2 * time.Second,
		ServerURL:        config.ServerURL,
		TestDuration:     config.TestDuration / 4, // Use 1/4 of total time
		SuccessThreshold: config.MinSuccessRate,
		LatencyThreshold: config.MaxLatency,
		AuthToken:        config.AuthToken,
		TLSEnabled:       config.TLSEnabled,
	}

	scenario := scenarios.NewOnboardingStormScenario(scenarioConfig)

	startTime := time.Now()
	err := scenario.Run(ctx)
	duration := time.Since(startTime)

	metrics := scenario.GetMetrics()
	results["onboarding"] = map[string]interface{}{
		"name":        scenario.GetName(),
		"description": scenario.GetDescription(),
		"duration":    duration,
		"success":     err == nil,
		"metrics":     metrics,
		"error":       err,
	}

	if err != nil {
		log.Printf("Onboarding scenario completed with error: %v", err)
	} else {
		log.Printf("Onboarding scenario completed successfully in %v", duration)
	}

	return err
}

func runSteadyStateScenario(ctx context.Context, config *TestConfig, fleet *framework.FleetSimulator, results map[string]interface{}) error {
	scenarioConfig := &scenarios.SteadyStateConfig{
		TotalDevices:       config.TotalDevices,
		ServerURL:          config.ServerURL,
		TestDuration:       config.TestDuration / 2, // Use 1/2 of total time
		WarmupDuration:     1 * time.Minute,
		MetricsTargetRate:  int64(config.TargetRPS / 2), // Metrics are subset of total requests
		HeartbeatTargetRate: int64(config.TargetRPS / 4), // Heartbeats are less frequent
		MaxErrorRate:       1.0 - config.MinSuccessRate,
		MaxLatency:         config.MaxLatency,
		AuthToken:          config.AuthToken,
		TLSEnabled:         config.TLSEnabled,
	}

	scenario := scenarios.NewSteadyStateScenario(scenarioConfig)

	startTime := time.Now()
	err := scenario.Run(ctx)
	duration := time.Since(startTime)

	metrics := scenario.GetMetrics()
	results["steady_state"] = map[string]interface{}{
		"name":        scenario.GetName(),
		"description": scenario.GetDescription(),
		"duration":    duration,
		"success":     err == nil,
		"metrics":     metrics,
		"error":       err,
	}

	if err != nil {
		log.Printf("Steady state scenario completed with error: %v", err)
	} else {
		log.Printf("Steady state scenario completed successfully in %v", duration)
	}

	return err
}

func runUpdateCampaignScenario(ctx context.Context, config *TestConfig, fleet *framework.FleetSimulator, results map[string]interface{}) error {
	scenarioConfig := &scenarios.UpdateCampaignConfig{
		TotalDevices:     config.TotalDevices,
		ServerURL:        config.ServerURL,
		TestDuration:     config.TestDuration / 3, // Use 1/3 of total time
		UpdateBatchSize:  25,
		UpdateBatchInterval: 1 * time.Minute,
		UpdateSuccessRate: config.MinSuccessRate,
		UpdateDuration:   2 * time.Minute,
		RollbackThreshold: 0.2,
		CanaryPercentage: 0.05,
		AuthToken:        config.AuthToken,
		TLSEnabled:       config.TLSEnabled,
	}

	scenario := scenarios.NewUpdateCampaignScenario(scenarioConfig)

	startTime := time.Now()
	err := scenario.Run(ctx)
	duration := time.Since(startTime)

	metrics := scenario.GetMetrics()
	results["update_campaign"] = map[string]interface{}{
		"name":        scenario.GetName(),
		"description": scenario.GetDescription(),
		"duration":    duration,
		"success":     err == nil,
		"metrics":     metrics,
		"error":       err,
	}

	if err != nil {
		log.Printf("Update campaign scenario completed with error: %v", err)
	} else {
		log.Printf("Update campaign scenario completed successfully in %v", duration)
	}

	return err
}

func runNetworkResilienceScenario(ctx context.Context, config *TestConfig, fleet *framework.FleetSimulator, results map[string]interface{}) error {
	scenarioConfig := &scenarios.NetworkResilienceConfig{
		TotalDevices:        config.TotalDevices,
		ServerURL:           config.ServerURL,
		TestDuration:        config.TestDuration / 3, // Use 1/3 of total time
		RecoveryTargetTime:  30 * time.Second,
		MaxReconnectAttempts: 5,
		AuthToken:           config.AuthToken,
		TLSEnabled:          config.TLSEnabled,
	}

	scenario := scenarios.NewNetworkResilienceScenario(scenarioConfig)

	startTime := time.Now()
	err := scenario.Run(ctx)
	duration := time.Since(startTime)

	metrics := scenario.GetMetrics()
	results["network_resilience"] = map[string]interface{}{
		"name":        scenario.GetName(),
		"description": scenario.GetDescription(),
		"duration":    duration,
		"success":     err == nil,
		"metrics":     metrics,
		"error":       err,
	}

	if err != nil {
		log.Printf("Network resilience scenario completed with error: %v", err)
	} else {
		log.Printf("Network resilience scenario completed successfully in %v", duration)
	}

	return err
}

func generateReports(config *TestConfig, metricsCollector *framework.MetricsCollector, fleet *framework.FleetSimulator, scenarioResults map[string]interface{}) error {
	reportConfig := &reports.ReportConfig{
		OutputDirectory: config.OutputDir,
		ReportFormats:   config.ReportFormats,
		IncludeCharts:   true,
		IncludeRawData:  config.Verbose,
	}

	reporter := reports.NewReporter(reportConfig)

	// Generate comprehensive report
	report, err := reporter.GenerateReport(metricsCollector, fleet, scenarioResults, reportConfig)
	if err != nil {
		return fmt.Errorf("failed to generate report: %w", err)
	}

	// Save report in specified formats
	if err := reporter.SaveReport(report, config.ReportFormats); err != nil {
		return fmt.Errorf("failed to save report: %w", err)
	}

	log.Printf("Reports saved to: %s", reporter.GetReportPath())

	// Print summary to console
	printTestSummary(report)

	return nil
}

func printTestSummary(report *reports.TestReport) {
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("LOAD TEST SUMMARY")
	fmt.Println(strings.Repeat("=", 60))

	fmt.Printf("Test Status: %s\n", strings.ToUpper(report.Results.TestOutcome.Status))
	fmt.Printf("Overall Grade: %s\n", report.ExecutiveSummary.PerformanceGrade)
	fmt.Printf("Overall Score: %.1f/100\n", report.ExecutiveSummary.OverallScore)
	fmt.Printf("Test Duration: %v\n", report.Metadata.TestDuration)

	fmt.Println("\nKey Metrics:")
	for _, metric := range report.ExecutiveSummary.KeyMetrics {
		status := strings.ToUpper(metric.Status)
		fmt.Printf("  %-20s: %v %s [%s]\n", metric.Name, metric.Value, metric.Unit, status)
	}

	fmt.Printf("\nTotal Requests: %d\n", report.Results.TotalRequests)
	fmt.Printf("Success Rate: %.2f%%\n", report.Results.SuccessRate*100)
	fmt.Printf("Average Throughput: %.1f req/s\n", report.Results.AverageThroughput)
	fmt.Printf("P95 Latency: %v\n", report.Performance.LatencyAnalysis.P95)

	if len(report.ExecutiveSummary.CriticalIssues) > 0 {
		fmt.Printf("\nCritical Issues (%d):\n", len(report.ExecutiveSummary.CriticalIssues))
		for i, issue := range report.ExecutiveSummary.CriticalIssues {
			fmt.Printf("  %d. %s: %s\n", i+1, issue.Title, issue.Description)
		}
	}

	if len(report.Recommendations) > 0 {
		fmt.Printf("\nTop Recommendations:\n")
		count := 3
		if len(report.Recommendations) < count {
			count = len(report.Recommendations)
		}
		for i := 0; i < count; i++ {
			rec := report.Recommendations[i]
			fmt.Printf("  %d. [%s] %s\n", i+1, strings.ToUpper(rec.Priority), rec.Title)
		}
	}

	fmt.Println(strings.Repeat("=", 60))
}