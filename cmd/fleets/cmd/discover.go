package cmd

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/mdns"
	"github.com/spf13/cobra"
)

var (
	discoverTimeout time.Duration
	serviceType     string
	showAll         bool
)

var discoverCmd = &cobra.Command{
	Use:   "discover",
	Short: "Discover fleetd devices on the network",
	Long: `Discover fleetd devices on the local network using mDNS.
	
This command scans the network for devices running the fleetd agent
and displays their information including IP addresses and ports.`,
	RunE: runDiscover,
}

func init() {
	discoverCmd.Flags().DurationVar(&discoverTimeout, "timeout", 10*time.Second, "Discovery timeout")
	discoverCmd.Flags().StringVar(&serviceType, "service", "_fleetd._tcp", "mDNS service type to discover")
	discoverCmd.Flags().BoolVar(&showAll, "all", false, "Show all mDNS services (useful for debugging)")
}

func runDiscover(cmd *cobra.Command, args []string) error {
	if showAll {
		// Discover all services for debugging
		return discoverAllServices()
	}

	fmt.Printf("Discovering %s services...\n\n", serviceType)

	// Create mDNS query
	entriesCh := make(chan *mdns.ServiceEntry, 10)
	foundDevices := []*mdns.ServiceEntry{}
	var foundMutex sync.Mutex

	// Start collecting entries
	go func() {
		for entry := range entriesCh {
			// mDNS library should already filter by service type
			// but let's double-check by looking at the Name field
			// The entry.Name format is "hostname._service._protocol.local."
			if strings.Contains(entry.Name, serviceType) {
				foundMutex.Lock()
				foundDevices = append(foundDevices, entry)
				foundMutex.Unlock()
			}
		}
	}()

	// Setup query parameters - start with IPv4 only to avoid errors
	params := mdns.DefaultParams(serviceType)
	params.Timeout = discoverTimeout
	params.Entries = entriesCh
	params.DisableIPv6 = true // Disable IPv6 by default to avoid errors

	// Create a context for cancellation
	ctx, cancel := context.WithTimeout(context.Background(), discoverTimeout)
	defer cancel()

	// Show progress
	done := make(chan bool)
	go showProgress(ctx, done)

	// Perform query
	queryErr := mdns.Query(params)

	// Signal progress to stop
	close(done)
	time.Sleep(100 * time.Millisecond) // Let progress clear

	close(entriesCh)

	if queryErr != nil {
		fmt.Printf("\nWarning: mDNS query encountered an error: %v\n\n", queryErr)
	}

	// Display found devices
	foundMutex.Lock()
	deviceCount := len(foundDevices)
	foundMutex.Unlock()

	if deviceCount > 0 {
		fmt.Printf("\rFound %d device(s):\n\n", deviceCount)
		for _, entry := range foundDevices {
			displayDevice(entry)
		}
	} else {
		fmt.Println("\rNo fleetd devices found.                              ")
		fmt.Println("\nTroubleshooting:")
		fmt.Println("1. Ensure devices are powered on and connected to the network")
		fmt.Println("2. Check if fleetd agent is running on the devices:")
		fmt.Println("   ssh user@device 'systemctl status fleetd'")
		fmt.Println("3. Try discovering all services to see what's available:")
		fmt.Println("   fleets discover --all")
		fmt.Println("4. Try common service types:")
		fmt.Println("   fleets discover --service _ssh._tcp")
		fmt.Println("   fleets discover --service _workstation._tcp")
		fmt.Println("5. Check specific hostnames:")
		fmt.Println("   ping dietpi.local")
		fmt.Println("   ping raspberrypi.local")
		fmt.Println("6. Use network scanning:")
		fmt.Println("   nmap -sn 192.168.1.0/24")
	}

	return nil
}

func displayDevice(entry *mdns.ServiceEntry) {
	fmt.Printf("Device: %s\n", entry.Name)
	fmt.Printf("  Host: %s\n", entry.Host)

	if entry.AddrV4 != nil {
		fmt.Printf("  IPv4: %v\n", entry.AddrV4)
	}
	if entry.AddrV6 != nil {
		fmt.Printf("  IPv6: %v\n", entry.AddrV6)
	}

	fmt.Printf("  Port: %d\n", entry.Port)

	if len(entry.InfoFields) > 0 {
		fmt.Printf("  Info:\n")
		for _, field := range entry.InfoFields {
			fmt.Printf("    %s\n", field)
		}
	}

	// Construct management URLs
	if entry.AddrV4 != nil {
		fmt.Printf("  Agent URL: http://%v:%d\n", entry.AddrV4, entry.Port)
	}

	fmt.Println()
}

func showProgress(ctx context.Context, done <-chan bool) {
	spinner := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	i := 0

	for {
		select {
		case <-ctx.Done():
			fmt.Printf("\rScanning... done!                    \n")
			return
		case <-done:
			fmt.Printf("\r                                      \r")
			return
		default:
			fmt.Printf("\rScanning... %s", spinner[i%len(spinner)])
			i++
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func discoverAllServices() error {
	fmt.Println("Discovering all mDNS services...\n")

	services := []string{
		"_fleetd._tcp",
		"_ssh._tcp",
		"_workstation._tcp",
		"_http._tcp",
		"_https._tcp",
		"_device-info._tcp",
		"_services._dns-sd._udp",
	}

	for _, service := range services {
		fmt.Printf("Scanning for %s:", service)

		entriesCh := make(chan *mdns.ServiceEntry, 10)
		found := []string{}

		go func() {
			for entry := range entriesCh {
				if entry.AddrV4 != nil {
					found = append(found, fmt.Sprintf("%s at %v:%d", entry.Name, entry.AddrV4, entry.Port))
				} else if entry.AddrV6 != nil {
					found = append(found, fmt.Sprintf("%s at %v:%d", entry.Name, entry.AddrV6, entry.Port))
				}
			}
		}()

		params := mdns.DefaultParams(service)
		params.Timeout = 2 * time.Second
		params.Entries = entriesCh
		params.DisableIPv6 = true // Avoid IPv6 errors

		// Show a simple progress indicator
		ctx, cancel := context.WithTimeout(context.Background(), params.Timeout)
		defer cancel()

		done := make(chan bool)
		go func() {
			ticker := time.NewTicker(200 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-done:
					return
				case <-ticker.C:
					fmt.Print(".")
				}
			}
		}()

		mdns.Query(params)
		close(done)
		close(entriesCh)

		if len(found) > 0 {
			fmt.Println()
			for _, f := range found {
				fmt.Printf("  - %s\n", f)
			}
		} else {
			fmt.Printf(" (none found)\n")
		}
		fmt.Println()
	}

	return nil
}
