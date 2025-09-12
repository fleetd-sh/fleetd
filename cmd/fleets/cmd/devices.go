package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var devicesCmd = &cobra.Command{
	Use:   "devices",
	Short: "Manage fleet devices",
	Long:  `List, register, and manage devices in the fleet.`,
}

var listDevicesCmd = &cobra.Command{
	Use:   "list",
	Short: "List all registered devices",
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: Connect to server API and list devices
		fmt.Println("Registered devices:")
		fmt.Println("(This feature requires a running fleet server)")
		return nil
	},
}

var registerDeviceCmd = &cobra.Command{
	Use:   "register [device-id]",
	Short: "Register a discovered device",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		deviceID := args[0]
		fmt.Printf("Registering device %s...\n", deviceID)
		fmt.Println("(This feature requires a running fleet server)")
		return nil
	},
}

func init() {
	devicesCmd.AddCommand(listDevicesCmd)
	devicesCmd.AddCommand(registerDeviceCmd)
}
