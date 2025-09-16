package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
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
			// TODO: Connect to fleet server API
			printInfo("Fetching devices from fleet server...")

			// For now, show example output
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tNAME\tSTATUS\tIP\tLAST SEEN\tVERSION")
			fmt.Fprintln(w, "device-001\tEdge Gateway 1\tOnline\t192.168.1.100\t2m ago\tv0.5.2")
			fmt.Fprintln(w, "device-002\tEdge Gateway 2\tOnline\t192.168.1.101\t1m ago\tv0.5.2")
			fmt.Fprintln(w, "device-003\tRaspberry Pi\tOffline\t192.168.1.102\t1h ago\tv0.5.1")
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

			printInfo("Fetching device %s...", deviceID)

			// TODO: Connect to fleet server API
			fmt.Printf("\n%s\n", bold("Device Details"))
			fmt.Printf("ID:          %s\n", deviceID)
			fmt.Printf("Name:        Edge Gateway 1\n")
			fmt.Printf("Status:      %s\n", green("Online"))
			fmt.Printf("IP Address:  192.168.1.100\n")
			fmt.Printf("MAC Address: aa:bb:cc:dd:ee:ff\n")
			fmt.Printf("Version:     v0.5.2\n")
			fmt.Printf("Last Seen:   %s\n", time.Now().Format(time.RFC3339))
			fmt.Printf("\n%s\n", bold("Hardware"))
			fmt.Printf("CPU:         ARM Cortex-A72\n")
			fmt.Printf("Memory:      4GB\n")
			fmt.Printf("Storage:     32GB\n")
			fmt.Printf("\n%s\n", bold("Tags"))
			fmt.Printf("Environment: production\n")
			fmt.Printf("Location:    warehouse-1\n")

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