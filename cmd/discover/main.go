package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/hashicorp/mdns"
)

func main() {
	var (
		serviceType = flag.String("service", "_fleetd._tcp", "mDNS service type to discover")
		timeout     = flag.Duration("timeout", 10*time.Second, "Discovery timeout")
	)
	flag.Parse()

	fmt.Printf("Discovering %s services for %v...\n", *serviceType, *timeout)

	// Create mDNS query
	entriesCh := make(chan *mdns.ServiceEntry, 10)
	foundAny := false

	// Start collecting entries
	go func() {
		for entry := range entriesCh {
			foundAny = true
			fmt.Printf("\nFound device:\n")
			fmt.Printf("  Name: %s\n", entry.Name)
			fmt.Printf("  Host: %s\n", entry.Host)
			fmt.Printf("  AddrV4: %v\n", entry.AddrV4)
			fmt.Printf("  AddrV6: %v\n", entry.AddrV6)
			fmt.Printf("  Port: %d\n", entry.Port)

			if len(entry.InfoFields) > 0 {
				fmt.Printf("  Info:\n")
				for _, field := range entry.InfoFields {
					fmt.Printf("    %s\n", field)
				}
			}

		}
	}()

	// Setup query parameters
	params := mdns.DefaultParams(*serviceType)
	params.Timeout = *timeout
	params.Entries = entriesCh
	params.DisableIPv6 = false

	// Perform query
	if err := mdns.Query(params); err != nil {
		fmt.Fprintf(os.Stderr, "mDNS query failed: %v\n", err)
		os.Exit(1)
	}

	close(entriesCh)

	if !foundAny {
		fmt.Println("\nNo devices found.")
		fmt.Println("\nTroubleshooting:")
		fmt.Println("1. Check if your Pi is powered on and booted (LED activity)")
		fmt.Println("2. Try scanning for all mDNS services: dns-sd -B _services._dns-sd._udp")
		fmt.Println("3. Check your Pi's IP on your router's DHCP leases")
		fmt.Println("4. Try pinging: ping raspberrypi.local or ping dietpi.local")
		fmt.Println("5. Scan your network: nmap -sn 192.168.1.0/24 (adjust subnet)")
	}
}
