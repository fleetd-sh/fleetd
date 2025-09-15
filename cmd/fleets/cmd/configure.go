package cmd

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	agentpb "fleetd.sh/gen/agent/v1"
	"fleetd.sh/gen/agent/v1/agentpbconnect"

	"connectrpc.com/connect"
	"github.com/hashicorp/mdns"
	"github.com/spf13/cobra"
)

var (
	configServerURL  string
	configAutoAccept bool
	configAPIKey     string
)

var configureCmd = &cobra.Command{
	Use:   "configure",
	Short: "Configure discovered fleetd devices",
	Long: `Configure fleetd devices discovered on the network with fleet server information.
	
This command will:
1. Discover devices on the local network
2. Allow you to select which devices to configure
3. Send the fleet server configuration to selected devices via their RPC service`,
	RunE: runConfigure,
}

func init() {
	configureCmd.Flags().StringVar(&configServerURL, "server", "", "Fleet server URL (e.g., http://192.168.1.100:8080)")
	configureCmd.Flags().BoolVar(&configAutoAccept, "auto", false, "Automatically configure all discovered devices")
	configureCmd.Flags().StringVar(&configAPIKey, "api-key", "", "API key for device authentication")
}

func runConfigure(cmd *cobra.Command, args []string) error {
	// First, discover devices
	fmt.Println("Discovering fleetd devices on the network...")

	devices, err := discoverDevices(10 * time.Second)
	if err != nil {
		return fmt.Errorf("discovery failed: %w", err)
	}

	if len(devices) == 0 {
		fmt.Println("No devices found")
		return nil
	}

	// Display found devices
	fmt.Printf("\nFound %d device(s):\n", len(devices))
	for i, device := range devices {
		fmt.Printf("\n%d. Device: %s\n", i+1, device.Name)
		fmt.Printf("   Host: %s\n", device.Host)
		if device.AddrV4 != nil {
			fmt.Printf("   IPv4: %v\n", device.AddrV4)
		}
		fmt.Printf("   Port: %d\n", device.Port)

		// Extract device ID from info fields if available
		for _, info := range device.InfoFields {
			if strings.HasPrefix(info, "deviceid=") {
				fmt.Printf("   Device ID: %s\n", strings.TrimPrefix(info, "deviceid="))
			}
		}
	}

	// Get server URL if not provided
	if configServerURL == "" {
		fmt.Print("\nEnter fleet server URL (e.g., http://192.168.1.100:8080): ")
		reader := bufio.NewReader(os.Stdin)
		configServerURL, _ = reader.ReadString('\n')
		configServerURL = strings.TrimSpace(configServerURL)
	}

	if configServerURL == "" {
		return fmt.Errorf("server URL is required for configuration")
	}

	// Select devices to configure
	var selectedDevices []*mdns.ServiceEntry

	if configAutoAccept {
		selectedDevices = devices
	} else {
		fmt.Print("\nWhich devices would you like to configure? (comma-separated numbers, or 'all'): ")
		reader := bufio.NewReader(os.Stdin)
		selection, _ := reader.ReadString('\n')
		selection = strings.TrimSpace(selection)

		if selection == "all" {
			selectedDevices = devices
		} else {
			// Parse selected device numbers
			parts := strings.Split(selection, ",")
			for _, part := range parts {
				num, err := strconv.Atoi(strings.TrimSpace(part))
				if err != nil || num < 1 || num > len(devices) {
					fmt.Printf("Invalid selection: %s\n", part)
					continue
				}
				selectedDevices = append(selectedDevices, devices[num-1])
			}
		}
	}

	if len(selectedDevices) == 0 {
		fmt.Println("No devices selected")
		return nil
	}

	// Configure selected devices
	fmt.Printf("\nConfiguring %d device(s) with server URL: %s\n\n", len(selectedDevices), configServerURL)

	var wg sync.WaitGroup
	for _, device := range selectedDevices {
		wg.Add(1)
		go func(d *mdns.ServiceEntry) {
			defer wg.Done()
			if err := configureDevice(d, configServerURL, configAPIKey); err != nil {
				fmt.Printf("❌ Failed to configure %s: %v\n", d.Name, err)
			} else {
				fmt.Printf("✅ Successfully configured %s\n", d.Name)
			}
		}(device)
	}

	wg.Wait()
	fmt.Println("\nConfiguration complete!")

	return nil
}

func discoverDevices(timeout time.Duration) ([]*mdns.ServiceEntry, error) {
	entriesCh := make(chan *mdns.ServiceEntry, 10)
	var devices []*mdns.ServiceEntry
	var mu sync.Mutex

	// Collect entries
	go func() {
		for entry := range entriesCh {
			mu.Lock()
			devices = append(devices, entry)
			mu.Unlock()
		}
	}()

	// Setup query
	params := mdns.DefaultParams("_fleetd._tcp")
	params.Timeout = timeout
	params.Entries = entriesCh
	params.DisableIPv6 = true

	// Perform query
	if err := mdns.Query(params); err != nil {
		close(entriesCh)
		return nil, err
	}

	close(entriesCh)

	mu.Lock()
	defer mu.Unlock()
	return devices, nil
}

func configureDevice(device *mdns.ServiceEntry, serverURL, apiKey string) error {
	// Extract device ID from info fields
	var deviceID string
	for _, info := range device.InfoFields {
		if strings.HasPrefix(info, "deviceid=") {
			deviceID = strings.TrimPrefix(info, "deviceid=")
			break
		}
	}

	// Connect to the device's RPC service
	var targetURL string
	if device.AddrV4 != nil {
		targetURL = fmt.Sprintf("http://%v:%d", device.AddrV4, device.Port)
	} else if device.AddrV6 != nil {
		targetURL = fmt.Sprintf("http://[%v]:%d", device.AddrV6, device.Port)
	} else {
		targetURL = fmt.Sprintf("http://%s:%d", device.Host, device.Port)
	}

	client := agentpbconnect.NewDiscoveryServiceClient(
		http.DefaultClient,
		targetURL,
	)

	// Send configuration to device
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Use ConfigureDevice method which exists in the proto
	_, err := client.ConfigureDevice(ctx, connect.NewRequest(&agentpb.ConfigureDeviceRequest{
		ApiEndpoint: serverURL,
		DeviceName:  deviceID,
	}))
	if err != nil {
		return fmt.Errorf("failed to configure device: %w", err)
	}

	return nil
}
