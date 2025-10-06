package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// CIMetrics represents metrics exported for CI/CD systems
type CIMetrics struct {
	TestName        string           `json:"test_name"`
	Timestamp       time.Time        `json:"timestamp"`
	Duration        time.Duration    `json:"duration"`
	Passed          bool             `json:"passed"`
	Grade           string           `json:"grade"`
	OverallScore    float64          `json:"overall_score"`
	SuccessRate     float64          `json:"success_rate"`
	ErrorRate       float64          `json:"error_rate"`
	AvgThroughput   float64          `json:"avg_throughput"`
	PeakThroughput  float64          `json:"peak_throughput"`
	P50Latency      time.Duration    `json:"p50_latency"`
	P95Latency      time.Duration    `json:"p95_latency"`
	P99Latency      time.Duration    `json:"p99_latency"`
	CPUPeak         float64          `json:"cpu_peak"`
	MemoryPeak      float64          `json:"memory_peak"`
	TotalRequests   int64            `json:"total_requests"`
	FailedRequests  int64            `json:"failed_requests"`
	Recommendations []string         `json:"recommendations"`
	CriticalIssues  []string         `json:"critical_issues"`
	Thresholds      ThresholdResults `json:"thresholds"`
	Environment     EnvironmentInfo  `json:"environment"`
}

// ThresholdResults contains pass/fail results for various thresholds
type ThresholdResults struct {
	SuccessRatePass   bool `json:"success_rate_pass"`
	ErrorRatePass     bool `json:"error_rate_pass"`
	LatencyPass       bool `json:"latency_pass"`
	ThroughputPass    bool `json:"throughput_pass"`
	ResourceUsagePass bool `json:"resource_usage_pass"`
}

// EnvironmentInfo contains information about the test environment
type EnvironmentInfo struct {
	Platform      string `json:"platform"`
	GoVersion     string `json:"go_version"`
	TestFramework string `json:"test_framework"`
	ServerURL     string `json:"server_url"`
	DeviceCount   int    `json:"device_count"`
	TestDuration  string `json:"test_duration"`
	CISystem      string `json:"ci_system"`
	Branch        string `json:"branch"`
	Commit        string `json:"commit"`
}

// CIConfig holds configuration for CI integration
type CIConfig struct {
	ReportPath     string
	OutputFormat   string
	ExitOnFailure  bool
	SuccessRateMin float64
	ErrorRateMax   float64
	P95LatencyMax  time.Duration
	ThroughputMin  float64
	CPUUsageMax    float64
	MemoryUsageMax float64
	Verbose        bool
}

func main() {
	config := parseFlags()

	if config.Verbose {
		log.SetFlags(log.LstdFlags | log.Lshortfile)
		log.Println("Starting CI integration tool")
	}

	// Process the load test report
	metrics, err := processReport(config)
	if err != nil {
		log.Fatalf("Failed to process report: %v", err)
	}

	// Output results in specified format
	if err := outputResults(config, metrics); err != nil {
		log.Fatalf("Failed to output results: %v", err)
	}

	// Check thresholds and exit appropriately
	if config.ExitOnFailure && !metrics.Passed {
		log.Println("Load test failed thresholds")
		os.Exit(1)
	}

	if config.Verbose {
		log.Println("CI integration completed successfully")
	}
}

func parseFlags() *CIConfig {
	config := &CIConfig{}

	flag.StringVar(&config.ReportPath, "report", "", "Path to load test report (JSON)")
	flag.StringVar(&config.OutputFormat, "format", "json", "Output format (json, junit, github)")
	flag.BoolVar(&config.ExitOnFailure, "exit-on-failure", true, "Exit with non-zero code on failure")
	flag.Float64Var(&config.SuccessRateMin, "success-rate-min", 0.95, "Minimum success rate threshold")
	flag.Float64Var(&config.ErrorRateMax, "error-rate-max", 0.05, "Maximum error rate threshold")
	flag.DurationVar(&config.P95LatencyMax, "p95-latency-max", 100*time.Millisecond, "Maximum P95 latency threshold")
	flag.Float64Var(&config.ThroughputMin, "throughput-min", 100, "Minimum throughput threshold (req/s)")
	flag.Float64Var(&config.CPUUsageMax, "cpu-max", 80, "Maximum CPU usage threshold (%)")
	flag.Float64Var(&config.MemoryUsageMax, "memory-max", 85, "Maximum memory usage threshold (%)")
	flag.BoolVar(&config.Verbose, "verbose", false, "Enable verbose output")

	flag.Parse()

	if config.ReportPath == "" {
		// Try to find the most recent report
		if report, err := findLatestReport(); err == nil {
			config.ReportPath = report
		} else {
			log.Fatalf("No report path specified and could not find recent report: %v", err)
		}
	}

	return config
}

func findLatestReport() (string, error) {
	// Look in common output directories
	searchPaths := []string{
		"./load_test_results",
		"./test_results",
		"./results",
		".",
	}

	var latestReport string
	var latestTime time.Time

	for _, searchPath := range searchPaths {
		matches, err := filepath.Glob(filepath.Join(searchPath, "load_test_report_*.json"))
		if err != nil {
			continue
		}

		for _, match := range matches {
			info, err := os.Stat(match)
			if err != nil {
				continue
			}

			if info.ModTime().After(latestTime) {
				latestTime = info.ModTime()
				latestReport = match
			}
		}
	}

	if latestReport == "" {
		return "", fmt.Errorf("no load test reports found")
	}

	return latestReport, nil
}

func processReport(config *CIConfig) (*CIMetrics, error) {
	// Read the report file
	data, err := os.ReadFile(config.ReportPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read report file: %w", err)
	}

	// Parse the JSON report
	var report map[string]interface{}
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, fmt.Errorf("failed to parse JSON report: %w", err)
	}

	// Extract metrics
	metrics := &CIMetrics{
		TestName:  "fleetd Load Test",
		Timestamp: time.Now(),
	}

	// Extract basic metrics
	if metadata, ok := report["metadata"].(map[string]interface{}); ok {
		if testName, ok := metadata["test_name"].(string); ok {
			metrics.TestName = testName
		}
		if duration, ok := metadata["test_duration"].(string); ok {
			if d, err := time.ParseDuration(duration); err == nil {
				metrics.Duration = d
			}
		}
	}

	// Extract executive summary
	if summary, ok := report["executive_summary"].(map[string]interface{}); ok {
		if passed, ok := summary["test_passed"].(bool); ok {
			metrics.Passed = passed
		}
		if grade, ok := summary["performance_grade"].(string); ok {
			metrics.Grade = grade
		}
		if score, ok := summary["overall_score"].(float64); ok {
			metrics.OverallScore = score
		}
	}

	// Extract results
	if results, ok := report["results"].(map[string]interface{}); ok {
		if successRate, ok := results["success_rate"].(float64); ok {
			metrics.SuccessRate = successRate
		}
		if avgThroughput, ok := results["average_throughput"].(float64); ok {
			metrics.AvgThroughput = avgThroughput
		}
		if peakThroughput, ok := results["peak_throughput"].(float64); ok {
			metrics.PeakThroughput = peakThroughput
		}
		if totalRequests, ok := results["total_requests"].(float64); ok {
			metrics.TotalRequests = int64(totalRequests)
		}
		if failedRequests, ok := results["failed_requests"].(float64); ok {
			metrics.FailedRequests = int64(failedRequests)
		}
	}

	// Extract performance analysis
	if performance, ok := report["performance"].(map[string]interface{}); ok {
		// Latency analysis
		if latency, ok := performance["latency_analysis"].(map[string]interface{}); ok {
			if p50, ok := latency["median"].(string); ok {
				if d, err := time.ParseDuration(p50); err == nil {
					metrics.P50Latency = d
				}
			}
			if p95, ok := latency["p95"].(string); ok {
				if d, err := time.ParseDuration(p95); err == nil {
					metrics.P95Latency = d
				}
			}
			if p99, ok := latency["p99"].(string); ok {
				if d, err := time.ParseDuration(p99); err == nil {
					metrics.P99Latency = d
				}
			}
		}

		// Error analysis
		if errorAnalysis, ok := performance["error_analysis"].(map[string]interface{}); ok {
			if errorRate, ok := errorAnalysis["error_rate"].(float64); ok {
				metrics.ErrorRate = errorRate
			}
		}

		// Resource analysis
		if resource, ok := performance["resource_analysis"].(map[string]interface{}); ok {
			if cpu, ok := resource["cpu"].(map[string]interface{}); ok {
				if peak, ok := cpu["peak"].(float64); ok {
					metrics.CPUPeak = peak
				}
			}
			if memory, ok := resource["memory"].(map[string]interface{}); ok {
				if peak, ok := memory["peak"].(float64); ok {
					metrics.MemoryPeak = peak
				}
			}
		}
	}

	// Extract recommendations
	if recommendations, ok := report["recommendations"].([]interface{}); ok {
		for _, rec := range recommendations {
			if recMap, ok := rec.(map[string]interface{}); ok {
				if title, ok := recMap["title"].(string); ok {
					metrics.Recommendations = append(metrics.Recommendations, title)
				}
			}
		}
	}

	// Extract critical issues
	if summary, ok := report["executive_summary"].(map[string]interface{}); ok {
		if issues, ok := summary["critical_issues"].([]interface{}); ok {
			for _, issue := range issues {
				if issueMap, ok := issue.(map[string]interface{}); ok {
					if title, ok := issueMap["title"].(string); ok {
						metrics.CriticalIssues = append(metrics.CriticalIssues, title)
					}
				}
			}
		}
	}

	// Check thresholds
	metrics.Thresholds = ThresholdResults{
		SuccessRatePass:   metrics.SuccessRate >= config.SuccessRateMin,
		ErrorRatePass:     metrics.ErrorRate <= config.ErrorRateMax,
		LatencyPass:       metrics.P95Latency <= config.P95LatencyMax,
		ThroughputPass:    metrics.AvgThroughput >= config.ThroughputMin,
		ResourceUsagePass: metrics.CPUPeak <= config.CPUUsageMax && metrics.MemoryPeak <= config.MemoryUsageMax,
	}

	// Overall pass/fail
	metrics.Passed = metrics.Thresholds.SuccessRatePass &&
		metrics.Thresholds.ErrorRatePass &&
		metrics.Thresholds.LatencyPass &&
		metrics.Thresholds.ThroughputPass &&
		metrics.Thresholds.ResourceUsagePass

	// Environment info
	metrics.Environment = EnvironmentInfo{
		Platform:      "linux",
		GoVersion:     "1.21",
		TestFramework: "fleetd Load Testing Framework",
		CISystem:      os.Getenv("CI_SYSTEM"),
		Branch:        os.Getenv("GITHUB_REF_NAME"),
		Commit:        os.Getenv("GITHUB_SHA"),
	}

	if config.Verbose {
		log.Printf("Processed metrics: Passed=%v, Grade=%s, Score=%.1f",
			metrics.Passed, metrics.Grade, metrics.OverallScore)
	}

	return metrics, nil
}

func outputResults(config *CIConfig, metrics *CIMetrics) error {
	switch config.OutputFormat {
	case "json":
		return outputJSON(metrics)
	case "junit":
		return outputJUnit(metrics)
	case "github":
		return outputGitHubActions(config, metrics)
	default:
		return fmt.Errorf("unsupported output format: %s", config.OutputFormat)
	}
}

func outputJSON(metrics *CIMetrics) error {
	data, err := json.MarshalIndent(metrics, "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(data))
	return nil
}

func outputJUnit(metrics *CIMetrics) error {
	// Generate JUnit XML format
	testCase := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<testsuites name="fleetd Load Tests" tests="1" failures="%d" time="%.2f">
  <testsuite name="Load Test" tests="1" failures="%d" time="%.2f">
    <testcase name="%s" classname="LoadTest" time="%.2f">`,
		boolToInt(!metrics.Passed),
		metrics.Duration.Seconds(),
		boolToInt(!metrics.Passed),
		metrics.Duration.Seconds(),
		metrics.TestName,
		metrics.Duration.Seconds())

	if !metrics.Passed {
		testCase += fmt.Sprintf(`
      <failure message="Load test failed thresholds" type="TestFailure">
        Success Rate: %.2f%% (threshold: %.2f%%)
        Error Rate: %.2f%% (threshold: %.2f%%)
        P95 Latency: %v (threshold: %v)
        Throughput: %.1f req/s (threshold: %.1f req/s)
        CPU Peak: %.1f%% (threshold: %.1f%%)
        Memory Peak: %.1f%% (threshold: %.1f%%)

        Critical Issues:
        %s
      </failure>`,
			metrics.SuccessRate*100, 95.0,
			metrics.ErrorRate*100, 5.0,
			metrics.P95Latency, 100*time.Millisecond,
			metrics.AvgThroughput, 100.0,
			metrics.CPUPeak, 80.0,
			metrics.MemoryPeak, 85.0,
			strings.Join(metrics.CriticalIssues, "\n        "))
	}

	testCase += `
    </testcase>
  </testsuite>
</testsuites>`

	fmt.Println(testCase)
	return nil
}

func outputGitHubActions(config *CIConfig, metrics *CIMetrics) error {
	// Generate GitHub Actions output
	fmt.Printf("::set-output name=test_passed::%v\n", metrics.Passed)
	fmt.Printf("::set-output name=grade::%s\n", metrics.Grade)
	fmt.Printf("::set-output name=overall_score::%.1f\n", metrics.OverallScore)
	fmt.Printf("::set-output name=success_rate::%.2f\n", metrics.SuccessRate*100)
	fmt.Printf("::set-output name=error_rate::%.2f\n", metrics.ErrorRate*100)
	fmt.Printf("::set-output name=avg_throughput::%.1f\n", metrics.AvgThroughput)
	fmt.Printf("::set-output name=p95_latency::%v\n", metrics.P95Latency)

	// Generate summary
	if metrics.Passed {
		fmt.Println("::notice title=Load Test Passed::Load test completed successfully with grade " + metrics.Grade)
	} else {
		fmt.Println("::error title=Load Test Failed::Load test failed to meet performance thresholds")
	}

	// Add annotations for critical issues
	for _, issue := range metrics.CriticalIssues {
		fmt.Printf("::warning title=Critical Issue::%s\n", issue)
	}

	// Add recommendations as notices
	for _, rec := range metrics.Recommendations {
		fmt.Printf("::notice title=Recommendation::%s\n", rec)
	}

	return nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
