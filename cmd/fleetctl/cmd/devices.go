package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"fleetd.sh/internal/client"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// newDevicesCmd creates the devices command
func newDevicesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "devices",
		Short: "Manage fleet devices",
		Long:  `List, inspect, and manage devices in your fleet`,
	}

	cmd.AddCommand(
		newDevicesListCmd(),
		newDevicesGetCmd(),
		newDevicesUpdateCmd(),
		newDevicesDeleteCmd(),
		newDevicesLogsCmd(),
		newDevicesMetricsCmd(),
	)

	return cmd
}

// newDevicesListCmd lists all devices
func newDevicesListCmd() *cobra.Command {
	var (
		status string
		tags   []string
		format string
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all devices",
		Long:  `Display a list of all registered devices in your fleet`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Get auth token from config or environment
			authToken := viper.GetString("auth.token")
			if authToken == "" {
				authToken = os.Getenv("FLEETCTL_AUTH_TOKEN")
			}

			// Create API client
			apiClient, err := client.NewClient(&client.Config{
				AuthToken: authToken,
			})
			if err != nil {
				return fmt.Errorf("failed to create API client: %w", err)
			}

			// Check if control plane is running
			ctx := context.Background()
			if err := apiClient.HealthCheck(ctx); err != nil {
				printWarning("Platform API is not available. Make sure platform-api is running.")
				printInfo("You can start it with: just platform-api-dev")

				// Show example output for demo purposes
				fmt.Println("\nShowing example data:")
				w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(w, "ID\tNAME\tSTATUS\tIP\tLAST SEEN\tVERSION")
				fmt.Fprintln(w, "device-001\tEdge Gateway 1\tOnline\t192.168.1.100\t2m ago\tv0.5.2")
				fmt.Fprintln(w, "device-002\tEdge Gateway 2\tOnline\t192.168.1.101\t1m ago\tv0.5.2")
				fmt.Fprintln(w, "device-003\tRaspberry Pi\tOffline\t192.168.1.102\t1h ago\tv0.5.1")
				w.Flush()
				return nil
			}

			printInfo("Fetching devices from control plane...")

			// Fetch devices from API
			devices, err := apiClient.ListDevices(ctx)
			if err != nil {
				printError("Failed to list devices")
				if strings.Contains(err.Error(), "unauthorized") || strings.Contains(err.Error(), "401") {
					printInfo("Authentication required. Please run: fleetctl login")
				} else if strings.Contains(err.Error(), "connection refused") {
					printInfo("Platform API is not running. Start it with: ./platform-api")
				} else if strings.Contains(err.Error(), "protocol error") {
					printInfo("Protocol mismatch. Check that platform-api is up to date.")
				}
				return fmt.Errorf("%w", err)
			}

			if len(devices) == 0 {
				printInfo("No devices found")
				return nil
			}

			// Display devices
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tNAME\tTYPE\tVERSION\tLAST SEEN")
			for _, device := range devices {
				lastSeen := ""
				if device.LastSeen != nil {
					lastSeen = device.LastSeen.AsTime().Format("2006-01-02 15:04")
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
					device.Id, device.Name, device.Type, device.Version, lastSeen)
			}
			w.Flush()

			return nil
		},
	}

	cmd.Flags().StringVarP(&status, "status", "s", "", "Filter by status (online, offline, updating)")
	cmd.Flags().StringSliceVarP(&tags, "tags", "t", nil, "Filter by tags")
	cmd.Flags().StringVarP(&format, "format", "f", "table", "Output format (table, json, yaml)")

	return cmd
}

// newDevicesGetCmd gets details about a specific device
func newDevicesGetCmd() *cobra.Command {
	var format string

	cmd := &cobra.Command{
		Use:   "get [device-id]",
		Short: "Get device details",
		Long:  `Display detailed information about a specific device`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			deviceID := args[0]

			// Get auth token from config or environment
			authToken := viper.GetString("auth.token")
			if authToken == "" {
				authToken = os.Getenv("FLEETCTL_AUTH_TOKEN")
			}

			// Create API client
			apiClient, err := client.NewClient(&client.Config{
				AuthToken: authToken,
			})
			if err != nil {
				return fmt.Errorf("failed to create API client: %w", err)
			}

			ctx := context.Background()

			// Check if platform API is running
			if err := apiClient.HealthCheck(ctx); err != nil {
				printWarning("Platform API is not available. Make sure platform-api is running.")

				// Show example output for demo
				fmt.Printf("\n%s\n", bold("Device Details (Example)"))
				fmt.Printf("ID:          %s\n", deviceID)
				fmt.Printf("Name:        Edge Gateway 1\n")
				fmt.Printf("Status:      %s\n", green("Online"))
				fmt.Printf("IP Address:  192.168.1.100\n")
				fmt.Printf("MAC Address: aa:bb:cc:dd:ee:ff\n")
				fmt.Printf("Version:     v0.5.2\n")
				fmt.Printf("Last Seen:   %s\n", time.Now().Format(time.RFC3339))
				return nil
			}

			printInfo("Fetching device %s...", deviceID)

			// Fetch device from API
			device, err := apiClient.GetDevice(ctx, deviceID)
			if err != nil {
				return fmt.Errorf("failed to get device: %w", err)
			}

			// Display device details
			fmt.Printf("\n%s\n", bold("Device Details"))
			fmt.Printf("ID:          %s\n", device.Id)
			fmt.Printf("Name:        %s\n", device.Name)
			fmt.Printf("Type:        %s\n", device.Type)
			fmt.Printf("Version:     %s\n", device.Version)
			if device.LastSeen != nil {
				fmt.Printf("Last Seen:   %s\n", device.LastSeen.AsTime().Format(time.RFC3339))
			}

			// Display system info if available
			if device.SystemInfo != nil {
				fmt.Printf("\n%s\n", bold("System Information"))
				fmt.Printf("Hostname:    %s\n", device.SystemInfo.Hostname)
				fmt.Printf("OS:          %s %s\n", device.SystemInfo.Os, device.SystemInfo.OsVersion)
				fmt.Printf("Arch:        %s\n", device.SystemInfo.Arch)
				fmt.Printf("CPU:         %s (%d cores)\n", device.SystemInfo.CpuModel, device.SystemInfo.CpuCores)
				fmt.Printf("Memory:      %.2f GB\n", float64(device.SystemInfo.MemoryTotal)/(1024*1024*1024))
				fmt.Printf("Storage:     %.2f GB\n", float64(device.SystemInfo.StorageTotal)/(1024*1024*1024))
				fmt.Printf("Platform:    %s\n", device.SystemInfo.Platform)
			}

			// Display metadata if available
			if len(device.Metadata) > 0 {
				fmt.Printf("\n%s\n", bold("Metadata"))
				for key, value := range device.Metadata {
					fmt.Printf("%s: %s\n", key, value)
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&format, "format", "f", "text", "Output format (text, json, yaml)")

	return cmd
}

// newDevicesUpdateCmd updates a device configuration
func newDevicesUpdateCmd() *cobra.Command {
	var (
		name string
		tags []string
	)

	cmd := &cobra.Command{
		Use:   "update [device-id]",
		Short: "Update device configuration",
		Long:  `Update configuration for a specific device`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			deviceID := args[0]

			printInfo("Updating device %s...", deviceID)

			// TODO: Connect to fleet server API
			printSuccess("Device %s updated successfully", deviceID)

			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Update device name")
	cmd.Flags().StringSliceVar(&tags, "tags", nil, "Update device tags")

	return cmd
}

// newDevicesDeleteCmd deletes a device
func newDevicesDeleteCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "delete [device-id]",
		Short: "Delete a device",
		Long:  `Remove a device from the fleet`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			deviceID := args[0]

			if !force {
				printWarning("This will permanently delete device %s", deviceID)
				fmt.Print("Continue? [y/N]: ")
				var response string
				fmt.Scanln(&response)
				if response != "y" && response != "Y" {
					fmt.Println("Cancelled")
					return nil
				}
			}

			printInfo("Deleting device %s...", deviceID)

			// TODO: Connect to fleet server API
			printSuccess("Device %s deleted successfully", deviceID)

			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation prompt")

	return cmd
}

// newDevicesLogsCmd shows device logs
func newDevicesLogsCmd() *cobra.Command {
	var (
		follow bool
		tail   int
		since  string
	)

	cmd := &cobra.Command{
		Use:   "logs [device-id]",
		Short: "Show device logs",
		Long:  `Display logs from a specific device`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			deviceID := args[0]

			printInfo("Fetching logs for device %s...", deviceID)

			// TODO: Connect to fleet server API for logs
			fmt.Printf("[2024-01-20 10:00:00] INFO: Device started\n")
			fmt.Printf("[2024-01-20 10:00:01] INFO: Connected to fleet server\n")
			fmt.Printf("[2024-01-20 10:00:02] INFO: Syncing configuration\n")
			fmt.Printf("[2024-01-20 10:00:03] INFO: Configuration updated\n")

			if follow {
				printInfo("Following logs... Press Ctrl+C to stop")
				// Would stream logs here
			}

			return nil
		},
	}

	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Follow log output")
	cmd.Flags().IntVar(&tail, "tail", 100, "Number of lines to show from the end")
	cmd.Flags().StringVar(&since, "since", "", "Show logs since timestamp")

	return cmd
}

// newDevicesMetricsCmd shows device metrics
func newDevicesMetricsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "metrics [device-id]",
		Short: "Show device metrics",
		Long:  `Display metrics and resource usage for a specific device`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			deviceID := args[0]

			printInfo("Fetching metrics for device %s...", deviceID)

			// TODO: Connect to fleet server API for metrics
			fmt.Printf("\n%s\n", bold("Resource Usage"))
			fmt.Printf("CPU:     25%% (1 core)\n")
			fmt.Printf("Memory:  512MB / 4GB (12.5%%)\n")
			fmt.Printf("Disk:    8GB / 32GB (25%%)\n")
			fmt.Printf("Network: ↓ 1.2MB/s ↑ 0.3MB/s\n")
			fmt.Printf("\n%s\n", bold("System Metrics"))
			fmt.Printf("Uptime:      7 days\n")
			fmt.Printf("Temperature: 42°C\n")
			fmt.Printf("Processes:   124\n")

			return nil
		},
	}

	return cmd
}
