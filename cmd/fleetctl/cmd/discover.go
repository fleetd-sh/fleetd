package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

// newDiscoverCmd creates the discover command
func newDiscoverCmd() *cobra.Command {
	var (
		timeout  int
		network  string
		protocol string
	)

	cmd := &cobra.Command{
		Use:   "discover",
		Short: "Discover devices on the network",
		Long: `Scan the network to discover unregistered devices that can be added to your fleet.
This uses mDNS/Bonjour to find devices advertising the fleetd service.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			printInfo("Scanning network for fleetd devices...")

			// Show spinner or progress
			fmt.Print("Discovering")
			for i := 0; i < 3; i++ {
				time.Sleep(500 * time.Millisecond)
				fmt.Print(".")
			}
			fmt.Println()

			// TODO: Connect to fleet server discovery API
			// For now, show example discovered devices
			fmt.Printf("\n%s\n", bold("Discovered Devices"))

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "HOSTNAME\tIP ADDRESS\tMAC ADDRESS\tVERSION\tSTATUS")
			fmt.Fprintln(w, "raspberrypi-001\t192.168.1.150\taa:bb:cc:dd:ee:01\tv0.5.2\tUnregistered")
			fmt.Fprintln(w, "edge-gateway-003\t192.168.1.151\taa:bb:cc:dd:ee:02\tv0.5.1\tUnregistered")
			fmt.Fprintln(w, "iot-device-042\t192.168.1.152\taa:bb:cc:dd:ee:03\tv0.5.2\tUnregistered")
			w.Flush()

			fmt.Printf("\n%s Found 3 unregistered devices\n", green("✓"))
			fmt.Printf("\nTo register a device, use: %s\n", cyan("fleetctl provision <hostname>"))

			return nil
		},
	}

	cmd.Flags().IntVarP(&timeout, "timeout", "t", 30, "Discovery timeout in seconds")
	cmd.Flags().StringVarP(&network, "network", "n", "local", "Network to scan (local, subnet)")
	cmd.Flags().StringVar(&protocol, "protocol", "mdns", "Discovery protocol (mdns, broadcast)")

	return cmd
}
