package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"fleetd.sh/test/stability"
)

func main() {
	var (
		configPath   = flag.String("config", "", "Path to configuration file (uses defaults if not specified)")
		outputDir    = flag.String("output", "./stability-results", "Output directory for test results")
		duration     = flag.Duration("duration", 72*time.Hour, "Test duration (default: 72h)")
		generateConfig = flag.String("generate-config", "", "Generate configuration template and exit")
		validateConfig = flag.String("validate-config", "", "Validate configuration file and exit")
		verbose      = flag.Bool("verbose", false, "Enable verbose logging")
		components   = flag.String("components", "memory,cpu,goroutines,connections,database,tls,network,data_integrity", "Comma-separated list of components to test")
	)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `fleetd 72-Hour Stability Test Framework

This tool runs comprehensive stability tests for fleetd, monitoring for:
- Memory leaks and resource usage patterns
- Connection stability and network resilience
- Database integrity and connection pool health
- Goroutine leaks and potential deadlocks
- Performance degradation over time
- TLS certificate management
- Data consistency validation

Usage: %s [options]

Options:
`, os.Args[0])
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
Examples:
  # Run with default 72-hour configuration
  %s

  # Run with custom configuration
  %s -config stability-config.json

  # Generate configuration template
  %s -generate-config stability-config.json

  # Run shorter test for development
  %s -duration 1h -output ./dev-test-results

  # Test specific components only
  %s -components memory,connections -duration 30m

  # Validate existing configuration
  %s -validate-config stability-config.json

Configuration:
  The tool uses a JSON configuration file to specify test parameters,
  thresholds, and component settings. Use -generate-config to create
  a template configuration file.

Output:
  Results are written to the specified output directory including:
  - stability.log: Detailed test logs
  - metrics.jsonl: Time-series metrics data
  - stability-report.json: Final test report
  - goroutine-dumps/: Stack traces if issues detected
  - memory-profiles/: Memory profiles if leaks detected

Exit Codes:
  0: Test passed successfully
  1: Test failed or configuration error
  2: Test was interrupted
`, os.Args[0], os.Args[0], os.Args[0], os.Args[0], os.Args[0], os.Args[0])
	}

	flag.Parse()

	// Generate configuration template
	if *generateConfig != "" {
		if err := stability.GenerateConfigTemplate(*generateConfig); err != nil {
			fmt.Fprintf(os.Stderr, "Error generating config template: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Configuration template generated: %s\n", *generateConfig)
		fmt.Println("Edit the configuration file and run the stability test with -config flag")
		return
	}

	// Validate configuration
	if *validateConfig != "" {
		if err := stability.ValidateConfig(*validateConfig); err != nil {
			fmt.Fprintf(os.Stderr, "Configuration validation failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Configuration is valid: %s\n", *validateConfig)
		return
	}

	// Override config with command line options if no config file specified
	if *configPath == "" && (*duration != 72*time.Hour || *verbose) {
		config := stability.DefaultConfig()
		config.Duration = *duration
		config.OutputDir = *outputDir

		if *verbose {
			config.LogLevel = "debug"
		}

		// Parse components
		if *components != "" {
			config.EnabledComponents = parseComponents(*components)
		}

		// Create temporary config file
		tempConfigPath := fmt.Sprintf("%s/temp-config.json", *outputDir)
		if err := os.MkdirAll(*outputDir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating output directory: %v\n", err)
			os.Exit(1)
		}

		if err := config.SaveConfig(tempConfigPath); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving temporary config: %v\n", err)
			os.Exit(1)
		}
		*configPath = tempConfigPath

		fmt.Printf("Using temporary configuration with duration: %v\n", *duration)
	}

	// Run stability test
	fmt.Printf("Starting fleetd 72-hour stability test framework\n")
	fmt.Printf("Output directory: %s\n", *outputDir)
	if *configPath != "" {
		fmt.Printf("Configuration: %s\n", *configPath)
	}
	fmt.Printf("Test will run for: %v\n", *duration)
	fmt.Println()

	if err := stability.RunStabilityTest(*configPath, *outputDir); err != nil {
		fmt.Fprintf(os.Stderr, "Stability test failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Stability test completed successfully!")
	fmt.Printf("Results available in: %s\n", *outputDir)
}

func parseComponents(componentStr string) []string {
	if componentStr == "" {
		return nil
	}

	var components []string
	for _, component := range strings.Split(componentStr, ",") {
		component = strings.TrimSpace(component)
		if component != "" {
			components = append(components, component)
		}
	}
	return components
}