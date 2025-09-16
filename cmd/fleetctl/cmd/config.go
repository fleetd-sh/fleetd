package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage Fleet configuration",
		Long:  `View and manage Fleet configuration settings.`,
	}

	cmd.AddCommand(
		newConfigGetCmd(),
		newConfigSetCmd(),
		newConfigListCmd(),
	)

	return cmd
}

func newConfigGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get [key]",
		Short: "Get configuration value",
		Long:  `Get a specific configuration value by key.`,
		Args:  cobra.ExactArgs(1),
		RunE:  runConfigGet,
	}
}

func newConfigSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set [key] [value]",
		Short: "Set configuration value",
		Long:  `Set a configuration value.`,
		Args:  cobra.ExactArgs(2),
		RunE:  runConfigSet,
	}
}

func newConfigListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all configuration",
		Long:  `Display all configuration settings.`,
		RunE:  runConfigList,
	}
}

func runConfigGet(cmd *cobra.Command, args []string) error {
	key := args[0]
	value := viper.Get(key)

	if value == nil {
		printWarning("Configuration key '%s' not found", key)
		return nil
	}

	fmt.Println(value)
	return nil
}

func runConfigSet(cmd *cobra.Command, args []string) error {
	key := args[0]
	value := args[1]

	// Parse value type
	viper.Set(key, value)

	// Save to config file
	if err := viper.WriteConfig(); err != nil {
		// Try to write config if it doesn't exist
		configFile := viper.ConfigFileUsed()
		if configFile == "" {
			configFile = "config.toml"
		}
		if err := viper.WriteConfigAs(configFile); err != nil {
			printError("Failed to save configuration: %v", err)
			return err
		}
	}

	printSuccess("Set %s = %s", key, value)
	return nil
}

func runConfigList(cmd *cobra.Command, args []string) error {
	printHeader("Fleet Configuration")
	fmt.Println()

	settings := viper.AllSettings()
	printConfigMap(settings, "")

	fmt.Println()
	printInfo("Config file: %s", viper.ConfigFileUsed())

	return nil
}

func printConfigMap(m map[string]any, prefix string) {
	for key, value := range m {
		fullKey := key
		if prefix != "" {
			fullKey = prefix + "." + key
		}

		switch v := value.(type) {
		case map[string]any:
			// Nested configuration
			printConfigMap(v, fullKey)
		case []any:
			// Array value
			items := make([]string, len(v))
			for i, item := range v {
				items[i] = fmt.Sprintf("%v", item)
			}
			fmt.Printf("%-30s = [%s]\n", fullKey, strings.Join(items, ", "))
		default:
			// Simple value
			fmt.Printf("%-30s = %v\n", fullKey, value)
		}
	}
}
