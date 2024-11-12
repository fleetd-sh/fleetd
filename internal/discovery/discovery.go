package discovery

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/mdns"
)

const (
	// ServiceName is the mDNS service name for fleetd
	ServiceName = "_fleetd._tcp"
)

// Discovery handles mDNS service discovery
type Discovery struct {
	deviceID string
	port     int
	server   *mdns.Server
	mu       sync.Mutex
	ctx      context.Context
	cancel   context.CancelFunc
}

// New creates a new Discovery instance
func New(deviceID string, port int) *Discovery {
	ctx, cancel := context.WithCancel(context.Background())
	return &Discovery{
		deviceID: deviceID,
		port:     port,
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Start begins advertising the device on the network
func (d *Discovery) Start() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Get host information
	host, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("failed to get hostname: %w", err)
	}

	// Setup service
	service, err := mdns.NewMDNSService(
		host,
		ServiceName,
		"",
		"",
		d.port,
		nil,
		[]string{
			fmt.Sprintf("deviceid=%s", d.deviceID),
		},
	)
	if err != nil {
		return fmt.Errorf("failed to create mDNS service: %w", err)
	}

	// Create the server with IPv6 disabled for testing
	config := &mdns.Config{
		Zone: service,
	}

	server, err := mdns.NewServer(config)
	if err != nil {
		return fmt.Errorf("failed to create mDNS server: %w", err)
	}

	d.server = server
	return nil
}

// Stop terminates the mDNS advertisement
func (d *Discovery) Stop() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.server != nil {
		d.server.Shutdown()
		d.server = nil
	}
	d.cancel()
	return nil
}

// Browse looks for other fleetd devices on the network
func (d *Discovery) Browse(timeout time.Duration) ([]string, error) {
	devices := make(map[string]bool)
	entriesCh := make(chan *mdns.ServiceEntry, 10)

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Start collecting entries
	go func() {
		for {
			select {
			case entry, ok := <-entriesCh:
				if !ok {
					return
				}
				for _, field := range entry.InfoFields {
					if strings.HasPrefix(field, "deviceid=") {
						deviceID := strings.TrimPrefix(field, "deviceid=")
						devices[deviceID] = true
						log.Printf("[DEBUG] Discovered device: %s", deviceID)
					}
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Setup query parameters
	params := mdns.DefaultParams(ServiceName)
	params.Timeout = timeout
	params.Entries = entriesCh
	params.DisableIPv6 = true

	// Perform query
	if err := mdns.Query(params); err != nil {
		return nil, fmt.Errorf("mdns query failed: %w", err)
	}

	// Wait for timeout or context cancellation
	<-ctx.Done()

	// Convert map to slice
	var result []string
	for device := range devices {
		result = append(result, device)
	}

	return result, nil
}
