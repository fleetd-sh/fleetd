package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"fleetd.sh/internal/provision"
	"github.com/spf13/cobra"
)

func newPluginCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plugin",
		Short: "Manage fleetd provisioning plugins",
		Long: `Manage plugins that extend fleetd's provisioning capabilities.

Plugins add functionality during device provisioning such as:
- K3s Kubernetes cluster setup
- Docker installation
- Custom software configurations
- Hardware-specific optimizations`,
	}

	cmd.AddCommand(newPluginListCmd())
	cmd.AddCommand(newPluginInfoCmd())
	cmd.AddCommand(newPluginInstallCmd())
	cmd.AddCommand(newPluginUpdateCmd())

	return cmd
}

func newPluginListCmd() *cobra.Command {
	var showAll bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available plugins",
		RunE: func(cmd *cobra.Command, args []string) error {
			manager := provision.NewPluginManager(&provision.EmbeddedPlugins)

			// Load embedded plugins
			if err := provision.LoadCorePlugins(manager); err != nil {
				return fmt.Errorf("failed to load core plugins: %w", err)
			}

			// Load custom plugins if requested
			if showAll {
				customDir := expandPath("~/.fleetd/plugins")
				if _, err := os.Stat(customDir); err == nil {
					entries, _ := os.ReadDir(customDir)
					for _, entry := range entries {
						if entry.IsDir() {
							pluginPath := filepath.Join(customDir, entry.Name())
							_ = manager.LoadFromDirectory(pluginPath)
						}
					}
				}
			}

			// Display plugins
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
			fmt.Fprintln(w, "NAME\tVERSION\tSOURCE\tDESCRIPTION")

			for _, name := range provision.CorePluginNames {
				plugin := manager.GetPlugin(name)
				if plugin != nil {
					source := "embedded"
					fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
						plugin.Name,
						plugin.Version,
						source,
						truncate(plugin.Description, 50))
				}
			}

			w.Flush()
			return nil
		},
	}

	cmd.Flags().BoolVarP(&showAll, "all", "a", false, "Show all plugins including custom")

	return cmd
}

func newPluginInfoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "info <plugin-name>",
		Short: "Show detailed information about a plugin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pluginName := args[0]

			manager := provision.NewPluginManager(&provision.EmbeddedPlugins)
			if err := manager.LoadEmbeddedPlugin(pluginName); err != nil {
				return fmt.Errorf("plugin not found: %s", pluginName)
			}

			plugin := manager.GetPlugin(pluginName)
			if plugin == nil {
				return fmt.Errorf("plugin not found: %s", pluginName)
			}

			// Display plugin information
			printHeader(fmt.Sprintf("Plugin: %s", plugin.Name))
			fmt.Printf("Version: %s\n", plugin.Version)
			fmt.Printf("Author: %s\n", plugin.Author)
			fmt.Printf("Description: %s\n\n", plugin.Description)

			fmt.Println("Supported Platforms:")
			for _, platform := range plugin.Platforms {
				fmt.Printf("  - %s\n", platform)
			}
			fmt.Println()

			fmt.Println("Supported Architectures:")
			for _, arch := range plugin.Architectures {
				fmt.Printf("  - %s\n", arch)
			}
			fmt.Println()

			if len(plugin.Options) > 0 {
				fmt.Println("Configuration Options:")
				for optName, optDef := range plugin.Options {
					required := ""
					if optDef.Required {
						required = " (required)"
					} else if optDef.RequiredIf != "" {
						required = fmt.Sprintf(" (required if %s)", optDef.RequiredIf)
					}

					fmt.Printf("  %s%s\n", optName, required)
					fmt.Printf("    Type: %s\n", optDef.Type)
					fmt.Printf("    Description: %s\n", optDef.Description)

					if len(optDef.Values) > 0 {
						fmt.Printf("    Allowed values: %s\n", strings.Join(optDef.Values, ", "))
					}

					if optDef.Default != "" {
						fmt.Printf("    Default: %s\n", optDef.Default)
					}

					if optDef.Example != "" {
						fmt.Printf("    Example: %s\n", optDef.Example)
					}
					fmt.Println()
				}
			}

			// Show usage example
			fmt.Println("Usage Example:")
			fmt.Printf("  fleetctl provision --device /dev/disk2 \\\n")
			fmt.Printf("    --plugin %s \\\n", plugin.Name)

			// Show example with first required option
			for optName, optDef := range plugin.Options {
				if optDef.Required {
					example := optDef.Example
					if example == "" {
						example = "<value>"
					}
					fmt.Printf("    --plugin-opt %s.%s=%s\n", plugin.Name, optName, example)
					break
				}
			}

			return nil
		},
	}

	return cmd
}

func newPluginInstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install <source>",
		Short: "Show information about using plugins",
		Long: `fleetd does not use a central plugin registry.

Plugins can be loaded from three sources:
1. Embedded plugins (built into fleetctl): --plugin k3s
2. Local file paths: --plugin ./my-plugin/plugin.yaml
3. URLs: --plugin https://example.com/plugins/docker/plugin.yaml

Use 'fleetctl plugin info <name>' to see details about embedded plugins.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				fmt.Println("To use plugins during provisioning:")
				fmt.Println()
				fmt.Println("  # Embedded plugin")
				fmt.Println("  fleetctl provision --plugin k3s --plugin-opt k3s.role=server")
				fmt.Println()
				fmt.Println("  # Local file")
				fmt.Println("  fleetctl provision --plugin ./custom/plugin.yaml")
				fmt.Println()
				fmt.Println("  # Remote URL")
				fmt.Println("  fleetctl provision --plugin https://example.com/plugins/docker/plugin.yaml")
				return nil
			}

			pluginName := args[0]

			// Check if plugin is already embedded
			for _, name := range provision.CorePluginNames {
				if name == pluginName {
					printInfo("Plugin '%s' is already embedded in fleetctl", pluginName)
					return nil
				}
			}

			printInfo("Plugin '%s' is not embedded. You can use it via file path or URL:", pluginName)
			fmt.Println()
			fmt.Println("  fleetctl provision --plugin /path/to/" + pluginName + "/plugin.yaml")
			fmt.Println("  fleetctl provision --plugin https://example.com/" + pluginName + "/plugin.yaml")
			return nil
		},
	}

	return cmd
}

func newPluginUpdateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update embedded plugins",
		Long: `Update embedded plugins to their latest versions.

Embedded plugins are updated automatically with fleetctl releases.
For custom plugins, update your local files or use the latest URL.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			printInfo("Embedded plugins are updated automatically with fleetctl")
			printInfo("To update fleetctl and its embedded plugins, run: fleetctl update")
			return nil
		},
	}

	return cmd
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
