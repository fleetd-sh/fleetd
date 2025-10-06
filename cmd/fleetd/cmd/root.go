package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "fleetd",
	Short: "fleetd agent for device management",
	Long: `fleetd is a lightweight agent that enables remote management,
monitoring, and updates for edge devices.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	// Add subcommands
	rootCmd.AddCommand(agentCmd)
	rootCmd.AddCommand(versionCmd)
}
