package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "fleets",
	Short: "FleetD server management CLI",
	Long: `FleetD server (fleets) is the central management system for fleetd agents.
	
Use this CLI to:
- Discover devices on the network
- Manage device registrations
- Deploy updates and configurations
- Monitor fleet status`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	// Add subcommands
	rootCmd.AddCommand(discoverCmd)
	rootCmd.AddCommand(configureCmd)
	rootCmd.AddCommand(devicesCmd)
	rootCmd.AddCommand(serverCmd)
	rootCmd.AddCommand(versionCmd)
}
