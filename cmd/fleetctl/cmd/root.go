package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	cfgFile     string
	projectRoot string
	verbose     bool
	noColor     bool

	// Color functions
	green  = color.New(color.FgGreen).SprintFunc()
	yellow = color.New(color.FgYellow).SprintFunc()
	red    = color.New(color.FgRed).SprintFunc()
	blue   = color.New(color.FgBlue).SprintFunc()
	cyan   = color.New(color.FgCyan).SprintFunc()
	bold   = color.New(color.Bold).SprintFunc()
)

// rootCmd represents the base command
var rootCmd = &cobra.Command{
	Use:   "fleetctl",
	Short: "fleetctl - Management CLI for fleetd platform",
	Long: `fleetctl is a unified CLI for managing fleetd infrastructure,
provisioning devices, and controlling the fleet platform.

Similar to kubectl, fleetctl provides comprehensive management capabilities
for your fleet of edge devices and the platform services.`,
	Version: "0.1.0",
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	cobra.OnInitialize(initConfig)

	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./config.toml)")
	rootCmd.PersistentFlags().StringVar(&projectRoot, "project-root", "", "project root directory")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
	rootCmd.PersistentFlags().BoolVar(&noColor, "no-color", false, "disable colored output")

	// Bind flags to viper
	viper.BindPFlag("verbose", rootCmd.PersistentFlags().Lookup("verbose"))
	viper.BindPFlag("no-color", rootCmd.PersistentFlags().Lookup("no-color"))

	// Add commands
	rootCmd.AddCommand(
		newStartCmd(),
		newStopCmd(),
		newStatusCmd(),
		newLogsCmd(),
		newInitCmd(),
		newProvisionCmd(),
		newDbCmd(),
		newGenerateCmd(),
		newDeployCmd(),
		newLoginCmd(),
		newConfigCmd(),
		newResetCmd(),
		newDevicesCmd(),
		newDiscoverCmd(),
		newConfigureCmd(),
		newMigrateCmd(),
		newVersionCmd(),
		newSecurityCmd(),
		newDocsCmd(),
	)

	// Disable color if requested
	if noColor {
		color.NoColor = true
	}
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find project root by looking for config.toml
		root := findProjectRoot()
		if root != "" {
			projectRoot = root
			viper.SetConfigFile(filepath.Join(root, "config.toml"))
		}
	}

	// Set config type
	viper.SetConfigType("toml")

	// Environment variables
	viper.SetEnvPrefix("FLEET")
	viper.AutomaticEnv()

	// Read config file
	if err := viper.ReadInConfig(); err == nil {
		if verbose {
			fmt.Printf("Using config file: %s\n", viper.ConfigFileUsed())
		}
	}

	// Set defaults
	setDefaults()
}

// findProjectRoot finds the project root by looking for config.toml
func findProjectRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "config.toml")); err == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return ""
}

// setDefaults sets default configuration values
func setDefaults() {
	// Project defaults
	viper.SetDefault("project.name", "fleet-project")

	// API defaults
	viper.SetDefault("api.port", 8080)
	viper.SetDefault("api.host", "localhost")

	// Database defaults
	viper.SetDefault("db.port", 5432)
	viper.SetDefault("db.host", "localhost")
	viper.SetDefault("db.name", "fleetd")
	viper.SetDefault("db.user", "fleetd")
	viper.SetDefault("db.password", "fleetd_secret")

	// Stack defaults
	viper.SetDefault("stack.services", []string{
		"postgres",
		"victoriametrics",
		"loki",
		"valkey",
		"traefik",
	})

	// Gateway defaults
	viper.SetDefault("gateway.port", 80)
	viper.SetDefault("gateway.dashboard_port", 8080)

	// Telemetry defaults
	viper.SetDefault("telemetry.victoria_metrics_port", 8428)
	viper.SetDefault("telemetry.loki_port", 3100)
	viper.SetDefault("telemetry.grafana_port", 3001)
}

// Helper functions for consistent output

func printSuccess(format string, a ...any) {
	fmt.Printf("%s %s\n", green("[OK]"), fmt.Sprintf(format, a...))
}

func printError(format string, a ...any) {
	fmt.Printf("%s %s\n", red("[ERROR]"), fmt.Sprintf(format, a...))
}

func printWarning(format string, a ...any) {
	fmt.Printf("%s %s\n", yellow("[WARN]"), fmt.Sprintf(format, a...))
}

func printInfo(format string, a ...any) {
	fmt.Printf("%s %s\n", blue("[INFO]"), fmt.Sprintf(format, a...))
}

func printHeader(text string) {
	fmt.Println(bold(text))
}

// checkDocker checks if Docker is available
func checkDocker() error {
	if err := runCommand("docker", "version", "--format", "json"); err != nil {
		return fmt.Errorf("Docker is not available. Please install Docker first")
	}
	return nil
}

// getProjectRoot returns the project root directory
func getProjectRoot() string {
	if projectRoot != "" {
		return projectRoot
	}
	root := findProjectRoot()
	if root != "" {
		return root
	}
	// Default to current directory
	dir, _ := os.Getwd()
	return dir
}
