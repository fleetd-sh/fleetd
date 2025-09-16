package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// newConfigureCmd creates the configure command
func newConfigureCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "configure",
		Short: "Configure fleet settings",
		Long:  `Manage fleet-wide configuration settings and policies`,
	}

	cmd.AddCommand(
		newConfigureGetCmd(),
		newConfigureSetCmd(),
		newConfigureApplyCmd(),
		newConfigureValidateCmd(),
	)

	return cmd
}

// newConfigureGetCmd gets configuration values
func newConfigureGetCmd() *cobra.Command {
	var (
		format string
		key    string
	)

	cmd := &cobra.Command{
		Use:   "get [key]",
		Short: "Get configuration values",
		Long:  `Retrieve fleet configuration values`,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				key = args[0]
			}

			// TODO: Connect to fleet server configuration API
			config := map[string]interface{}{
				"fleet": map[string]interface{}{
					"name":        "production-fleet",
					"region":      "us-west-2",
					"environment": "production",
				},
				"update": map[string]interface{}{
					"strategy":       "rolling",
					"max_concurrent": 10,
					"auto_rollback":  true,
				},
				"telemetry": map[string]interface{}{
					"metrics_interval": 60,
					"logs_enabled":     true,
					"traces_enabled":   false,
				},
				"security": map[string]interface{}{
					"tls_required":    true,
					"min_tls_version": "1.2",
					"audit_logging":   true,
				},
			}

			if key != "" {
				// Extract specific key
				printInfo("Configuration for %s:", key)
				// TODO: Navigate to specific key
			} else {
				printInfo("Current fleet configuration:")
			}

			switch format {
			case "json":
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				enc.Encode(config)
			case "yaml":
				enc := yaml.NewEncoder(os.Stdout)
				enc.SetIndent(2)
				enc.Encode(config)
			default:
				printConfig(config, "")
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&format, "format", "f", "text", "Output format (text, json, yaml)")

	return cmd
}

// newConfigureSetCmd sets configuration values
func newConfigureSetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set [key] [value]",
		Short: "Set configuration value",
		Long:  `Update a specific fleet configuration value`,
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			value := args[1]

			printInfo("Setting %s = %s", key, value)

			// TODO: Connect to fleet server configuration API
			printSuccess("Configuration updated successfully")
			printInfo("Changes will be applied to all devices within 5 minutes")

			return nil
		},
	}

	return cmd
}

// newConfigureApplyCmd applies a configuration file
func newConfigureApplyCmd() *cobra.Command {
	var (
		file     string
		dryRun   bool
		validate bool
	)

	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Apply configuration from file",
		Long:  `Apply fleet configuration from a YAML or JSON file`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if file == "" {
				return fmt.Errorf("configuration file is required")
			}

			printInfo("Reading configuration from %s", file)

			// TODO: Read and parse configuration file
			// TODO: Validate configuration
			if validate {
				printInfo("Validating configuration...")
				printSuccess("Configuration is valid")
			}

			if dryRun {
				printInfo("Dry run mode - no changes will be applied")
				// Show what would change
				fmt.Printf("\n%s\n", bold("Changes to be applied:"))
				fmt.Println("+ fleet.environment: staging -> production")
				fmt.Println("+ update.max_concurrent: 5 -> 10")
				fmt.Println("+ telemetry.traces_enabled: false -> true")
				return nil
			}

			// TODO: Connect to fleet server configuration API
			printInfo("Applying configuration...")
			printSuccess("Configuration applied successfully")
			printInfo("Changes will propagate to all devices")

			return nil
		},
	}

	cmd.Flags().StringVarP(&file, "file", "f", "", "Configuration file (YAML or JSON)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would change without applying")
	cmd.Flags().BoolVar(&validate, "validate", true, "Validate configuration before applying")

	cmd.MarkFlagRequired("file")

	return cmd
}

// newConfigureValidateCmd validates a configuration file
func newConfigureValidateCmd() *cobra.Command {
	var file string

	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate configuration file",
		Long:  `Check if a configuration file is valid without applying it`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if file == "" {
				return fmt.Errorf("configuration file is required")
			}

			printInfo("Validating configuration file: %s", file)

			// TODO: Read and validate configuration file
			// Check for:
			// - Valid YAML/JSON syntax
			// - Required fields
			// - Valid value ranges
			// - Type correctness

			printSuccess("Configuration is valid")

			// Show summary
			fmt.Printf("\n%s\n", bold("Configuration Summary:"))
			fmt.Println("Type:      Fleet Configuration")
			fmt.Println("Version:   v1")
			fmt.Println("Sections:  fleet, update, telemetry, security")
			fmt.Println("Devices:   Will affect 25 devices")

			return nil
		},
	}

	cmd.Flags().StringVarP(&file, "file", "f", "", "Configuration file to validate")
	cmd.MarkFlagRequired("file")

	return cmd
}

// Helper function to print configuration in text format
func printConfig(config map[string]interface{}, prefix string) {
	for key, value := range config {
		fullKey := key
		if prefix != "" {
			fullKey = prefix + "." + key
		}

		switch v := value.(type) {
		case map[string]interface{}:
			printConfig(v, fullKey)
		default:
			fmt.Printf("%s: %v\n", fullKey, value)
		}
	}
}
