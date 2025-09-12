package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	// Global flags
	verbose bool

	// Root command
	rootCmd = &cobra.Command{
		Use:   "fleetp",
		Short: "Fleet device provisioner",
		Long: `fleetp - Fleet Device Provisioner

Provisions devices with the fleetd agent so they can:
- Boot up and connect to your network  
- Be discovered via mDNS
- Be managed by a fleet server

Supports Raspberry Pi and other ARM devices with custom OS images.`,
		Version: "0.1.0",
	}
)

// Execute runs the root command
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")

	// Add commands
	rootCmd.AddCommand(provisionCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(completionCmd)
}
