package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"fleetd.sh/test/security"
	"fleetd.sh/test/security/compliance"
	"fleetd.sh/test/security/reporting"
)

// Configuration for the security test runner
type Config struct {
	TargetURL        string   `json:"target_url"`
	TestCategories   []string `json:"test_categories"`
	OutputFormats    []string `json:"output_formats"`
	OutputDir        string   `json:"output_dir"`
	Timeout          string   `json:"timeout"`
	CIMode           bool     `json:"ci_mode"`
	FailThreshold    string   `json:"fail_threshold"`
	ConfigFile       string   `json:"config_file"`
	IncludeEvidence  bool     `json:"include_evidence"`
	IncludePayloads  bool     `json:"include_payloads"`
	Parallel         int      `json:"parallel"`
	Verbose          bool     `json:"verbose"`
}

func main() {
	// Parse command line flags
	config := parseFlags()

	// Load configuration file if specified
	if config.ConfigFile != "" {
		if err := loadConfigFile(config); err != nil {
			log.Fatalf("Failed to load config file: %v", err)
		}
	}

	// Validate configuration
	if err := validateConfig(config); err != nil {
		log.Fatalf("Invalid configuration: %v", err)
	}

	// Set up context with timeout
	timeout, err := time.ParseDuration(config.Timeout)
	if err != nil {
		log.Fatalf("Invalid timeout duration: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Create security test framework
	testConfig := &security.TestConfig{
		TargetURL:       config.TargetURL,
		APIKeys:         make(map[string]string),
		Certificates:    &security.CertConfig{},
		RateLimit:       &security.RateLimitConfig{},
		Timeout:         30 * time.Second,
		MaxPayloadSize:  1024 * 1024, // 1MB
		TLSMinVersion:   0x0303,       // TLS 1.2
		EnabledTests:    config.TestCategories,
		ComplianceLevel: "standard",
	}

	// Load API keys from environment
	loadAPIKeysFromEnv(testConfig)

	framework := security.NewSecurityTestFramework(testConfig)

	// Run security tests
	fmt.Printf("Starting security assessment for %s\n", config.TargetURL)
	fmt.Printf("Test categories: %s\n", strings.Join(config.TestCategories, ", "))
	fmt.Printf("Timeout: %s\n", config.Timeout)

	if err := framework.RunSecurityTests(ctx); err != nil {
		log.Fatalf("Security tests failed: %v", err)
	}

	// Run compliance checks
	fmt.Println("Running compliance checks...")
	complianceChecker := compliance.NewComplianceChecker()
	complianceReport, err := complianceChecker.CheckCompliance(ctx, config.TargetURL)
	if err != nil {
		log.Printf("Compliance checking failed: %v", err)
	}

	// Generate comprehensive report
	fmt.Println("Generating security report...")
	reporter := reporting.NewSecurityReporter()

	securityReport, err := reporter.GenerateReport(framework.Results, complianceReport)
	if err != nil {
		log.Fatalf("Failed to generate report: %v", err)
	}

	// Export report in requested formats
	if err := reporter.ExportReport(securityReport); err != nil {
		log.Fatalf("Failed to export report: %v", err)
	}

	// Print summary to console
	printSummary(securityReport, config.Verbose)

	// Check CI failure criteria
	if config.CIMode {
		ciResult := reporter.CheckCIFailureCriteria(securityReport)

		fmt.Printf("\n=== CI/CD Results ===\n")
		fmt.Printf("Status: %s\n", ciResult.Summary)

		if len(ciResult.FailureReasons) > 0 {
			fmt.Printf("Failure Reasons:\n")
			for _, reason := range ciResult.FailureReasons {
				fmt.Printf("  - %s\n", reason)
			}
		}

		if len(ciResult.RecommendedActions) > 0 {
			fmt.Printf("Recommended Actions:\n")
			for _, action := range ciResult.RecommendedActions {
				fmt.Printf("  - %s\n", action)
			}
		}

		// Exit with appropriate code for CI/CD
		if ciResult.ShouldFail {
			fmt.Printf("\n❌ Security testing failed - vulnerabilities found above threshold\n")
			os.Exit(ciResult.ExitCode)
		} else {
			fmt.Printf("\n✅ Security testing passed\n")
		}
	}

	fmt.Printf("\n✅ Security assessment completed successfully\n")
	fmt.Printf("📊 Reports generated in: %s\n", config.OutputDir)
}

func parseFlags() *Config {
	config := &Config{
		TestCategories:  []string{"all"},
		OutputFormats:   []string{"json", "html"},
		OutputDir:       "./security-reports",
		Timeout:         "30m",
		CIMode:          false,
		FailThreshold:   "High",
		IncludeEvidence: true,
		IncludePayloads: false,
		Parallel:        10,
		Verbose:         false,
	}

	flag.StringVar(&config.TargetURL, "target", "", "Target URL to test (required)")
	flag.StringVar(&config.ConfigFile, "config", "", "Configuration file path")

	var categories string
	flag.StringVar(&categories, "categories", "all", "Test categories (comma-separated): auth,injection,api,tls,all")

	var formats string
	flag.StringVar(&formats, "formats", "json,html", "Output formats (comma-separated): json,html,pdf,junit,sarif,csv")

	flag.StringVar(&config.OutputDir, "output", config.OutputDir, "Output directory for reports")
	flag.StringVar(&config.Timeout, "timeout", config.Timeout, "Test timeout duration (e.g., 30m, 1h)")
	flag.BoolVar(&config.CIMode, "ci", config.CIMode, "Enable CI/CD mode with exit codes")
	flag.StringVar(&config.FailThreshold, "fail-threshold", config.FailThreshold, "CI failure threshold: Critical,High,Medium,Low")
	flag.BoolVar(&config.IncludeEvidence, "evidence", config.IncludeEvidence, "Include evidence in reports")
	flag.BoolVar(&config.IncludePayloads, "payloads", config.IncludePayloads, "Include payloads in reports (security risk)")
	flag.IntVar(&config.Parallel, "parallel", config.Parallel, "Number of parallel test workers")
	flag.BoolVar(&config.Verbose, "verbose", config.Verbose, "Enable verbose output")

	var showVersion bool
	flag.BoolVar(&showVersion, "version", false, "Show version information")

	var showHelp bool
	flag.BoolVar(&showHelp, "help", false, "Show help information")

	flag.Parse()

	if showVersion {
		fmt.Println("fleetd Security Testing Framework v1.0.0")
		fmt.Println("Built with ❤️ for fleetd security")
		os.Exit(0)
	}

	if showHelp {
		printUsage()
		os.Exit(0)
	}

	// Parse comma-separated values
	if categories != "" {
		config.TestCategories = strings.Split(categories, ",")
	}
	if formats != "" {
		config.OutputFormats = strings.Split(formats, ",")
	}

	return config
}

func loadConfigFile(config *Config) error {
	data, err := os.ReadFile(config.ConfigFile)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	if err := json.Unmarshal(data, config); err != nil {
		return fmt.Errorf("failed to parse config file: %w", err)
	}

	return nil
}

func validateConfig(config *Config) error {
	if config.TargetURL == "" {
		return fmt.Errorf("target URL is required")
	}

	if !strings.HasPrefix(config.TargetURL, "http://") && !strings.HasPrefix(config.TargetURL, "https://") {
		return fmt.Errorf("target URL must start with http:// or https://")
	}

	validCategories := map[string]bool{
		"all": true, "auth": true, "authentication": true, "authorization": true,
		"injection": true, "api": true, "tls": true, "rate_limiting": true,
		"input_validation": true, "session_management": true, "cryptography": true,
		"configuration": true,
	}

	for _, category := range config.TestCategories {
		if !validCategories[category] {
			return fmt.Errorf("invalid test category: %s", category)
		}
	}

	validFormats := map[string]bool{
		"json": true, "html": true, "pdf": true, "junit": true, "sarif": true, "csv": true,
	}

	for _, format := range config.OutputFormats {
		if !validFormats[format] {
			return fmt.Errorf("invalid output format: %s", format)
		}
	}

	validThresholds := map[string]bool{
		"Critical": true, "High": true, "Medium": true, "Low": true,
	}

	if !validThresholds[config.FailThreshold] {
		return fmt.Errorf("invalid fail threshold: %s", config.FailThreshold)
	}

	return nil
}

func loadAPIKeysFromEnv(testConfig *security.TestConfig) {
	// Load API keys from environment variables
	if apiKey := os.Getenv("FLEETD_API_KEY"); apiKey != "" {
		testConfig.APIKeys["fleetd"] = apiKey
	}

	if apiKey := os.Getenv("TEST_API_KEY"); apiKey != "" {
		testConfig.APIKeys["test"] = apiKey
	}

	// Load certificate paths from environment
	if certPath := os.Getenv("FLEETD_CERT_PATH"); certPath != "" {
		testConfig.Certificates.CertPath = certPath
	}

	if keyPath := os.Getenv("FLEETD_KEY_PATH"); keyPath != "" {
		testConfig.Certificates.KeyPath = keyPath
	}

	if caPath := os.Getenv("FLEETD_CA_PATH"); caPath != "" {
		testConfig.Certificates.CAPath = caPath
	}
}

func printSummary(report *reporting.SecurityReport, verbose bool) {
	fmt.Printf("\n=== Security Assessment Summary ===\n")
	fmt.Printf("Target: %s\n", report.Metadata.TargetURL)
	fmt.Printf("Generated: %s\n", report.Metadata.Generated.Format("2006-01-02 15:04:05"))
	fmt.Printf("Duration: %s\n", report.Metadata.TestDuration)

	summary := report.Summary
	fmt.Printf("\n📊 Overall Results:\n")
	fmt.Printf("  Security Score: %.1f%%\n", summary.OverallScore)
	fmt.Printf("  Security Grade: %s\n", summary.SecurityGrade)
	fmt.Printf("  Risk Level: %s\n", summary.RiskLevel)

	if report.Compliance != nil {
		fmt.Printf("  Compliance Score: %.1f%%\n", report.Compliance.OverallScore)
		fmt.Printf("  Compliance Status: %s\n", report.Compliance.OverallStatus)
	}

	fmt.Printf("\n🔍 Vulnerability Summary:\n")
	fmt.Printf("  Total: %d\n", summary.TotalVulnerabilities)
	fmt.Printf("  Critical: %d\n", summary.CriticalCount)
	fmt.Printf("  High: %d\n", summary.HighCount)
	fmt.Printf("  Medium: %d\n", summary.MediumCount)
	fmt.Printf("  Low: %d\n", summary.LowCount)

	if len(summary.CategoryBreakdown) > 0 {
		fmt.Printf("\n📋 Category Breakdown:\n")
		for category, catSummary := range summary.CategoryBreakdown {
			fmt.Printf("  %s: %d vulnerabilities (highest: %s)\n",
				category, catSummary.VulnCount, catSummary.HighestSeverity)
		}
	}

	if len(summary.TopRisks) > 0 {
		fmt.Printf("\n⚠️  Top Risks:\n")
		for _, risk := range summary.TopRisks {
			fmt.Printf("  - %s\n", risk)
		}
	}

	if len(summary.KeyRecommendations) > 0 {
		fmt.Printf("\n💡 Key Recommendations:\n")
		for _, rec := range summary.KeyRecommendations {
			fmt.Printf("  - %s\n", rec)
		}
	}

	if verbose && len(report.Results.Vulnerabilities) > 0 {
		fmt.Printf("\n🔍 Detailed Vulnerabilities:\n")
		for i, vuln := range report.Results.Vulnerabilities {
			if i >= 10 { // Limit to first 10 in summary
				fmt.Printf("  ... and %d more (see full report)\n", len(report.Results.Vulnerabilities)-10)
				break
			}
			fmt.Printf("  [%s] %s - %s (CVSS: %.1f)\n",
				vuln.Severity, vuln.ID, vuln.Title, vuln.CVSSScore)
		}
	}
}

func printUsage() {
	fmt.Printf(`fleetd Security Testing Framework

USAGE:
    security-test [OPTIONS]

REQUIRED:
    -target <URL>           Target URL to test (e.g., https://api.example.com)

OPTIONS:
    -config <file>          Configuration file path
    -categories <list>      Test categories (comma-separated)
                           Available: all,auth,injection,api,tls,rate_limiting,
                           input_validation,session_management,cryptography,configuration
                           Default: all
    -formats <list>         Output formats (comma-separated)
                           Available: json,html,pdf,junit,sarif,csv
                           Default: json,html
    -output <dir>           Output directory for reports
                           Default: ./security-reports
    -timeout <duration>     Test timeout (e.g., 30m, 1h)
                           Default: 30m
    -ci                     Enable CI/CD mode with exit codes
    -fail-threshold <level> CI failure threshold
                           Options: Critical,High,Medium,Low
                           Default: High
    -evidence               Include evidence in reports (default: true)
    -payloads               Include payloads in reports (security risk, default: false)
    -parallel <n>           Number of parallel test workers
                           Default: 10
    -verbose                Enable verbose output
    -version                Show version information
    -help                   Show this help message

EXAMPLES:
    # Basic security test
    security-test -target https://api.fleetd.sh

    # Full test suite with CI mode
    security-test -target https://api.fleetd.sh -ci -fail-threshold High

    # Specific categories only
    security-test -target https://api.fleetd.sh -categories auth,injection,api

    # Custom output formats and directory
    security-test -target https://api.fleetd.sh -formats json,html,pdf -output ./reports

    # Use configuration file
    security-test -config security-test-config.json

ENVIRONMENT VARIABLES:
    FLEETD_API_KEY         API key for authentication
    FLEETD_CERT_PATH       Client certificate path
    FLEETD_KEY_PATH        Client private key path
    FLEETD_CA_PATH         CA certificate path

CONFIGURATION FILE FORMAT:
    {
        "target_url": "https://api.fleetd.sh",
        "test_categories": ["auth", "injection", "api"],
        "output_formats": ["json", "html"],
        "output_dir": "./security-reports",
        "timeout": "30m",
        "ci_mode": true,
        "fail_threshold": "High",
        "include_evidence": true,
        "include_payloads": false,
        "parallel": 10,
        "verbose": false
    }

EXIT CODES (CI Mode):
    0    All tests passed
    1    Security vulnerabilities found above threshold
    2    Configuration or runtime error

For more information, visit: https://github.com/your-org/fleetd
`)
}