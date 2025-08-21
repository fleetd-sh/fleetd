package discovery

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/hashicorp/mdns"
)

// DeviceInfo contains information about a discovered device
type DeviceInfo struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Type       string            `json:"type"`
	Hostname   string            `json:"hostname"`
	Address    string            `json:"address"`
	Port       int               `json:"port"`
	Configured bool              `json:"configured"`
	Properties map[string]string `json:"properties"`
	LastSeen   time.Time         `json:"last_seen"`
}

// RPiDiscovery extends Discovery with Raspberry Pi specific features
type RPiDiscovery struct {
	*Discovery
	devices map[string]*DeviceInfo
}

// NewRPiDiscovery creates a new RPi-aware discovery service
func NewRPiDiscovery(deviceID string, port int) *RPiDiscovery {
	return &RPiDiscovery{
		Discovery: New(deviceID, port, DefaultServiceName),
		devices:   make(map[string]*DeviceInfo),
	}
}

// StartWithInfo starts advertising with additional device information
func (r *RPiDiscovery) StartWithInfo(info map[string]string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Build TXT records
	var txtRecords []string
	txtRecords = append(txtRecords, fmt.Sprintf("device_id=%s", r.deviceID))

	// Add all info fields as TXT records
	for key, value := range info {
		txtRecords = append(txtRecords, fmt.Sprintf("%s=%s", key, value))
	}

	// Get host information
	hostname, _ := os.Hostname()

	// Setup service with extended info
	service, err := mdns.NewMDNSService(
		hostname,
		r.serviceType,
		"",
		"",
		r.port,
		nil,
		txtRecords,
	)
	if err != nil {
		return fmt.Errorf("failed to create mDNS service: %w", err)
	}

	// Create the server
	config := &mdns.Config{
		Zone: service,
	}

	server, err := mdns.NewServer(config)
	if err != nil {
		return fmt.Errorf("failed to create mDNS server: %w", err)
	}

	r.server = server
	return nil
}

// BrowseDevices discovers devices with detailed information
func (r *RPiDiscovery) BrowseDevices(ctx context.Context, timeout time.Duration) ([]*DeviceInfo, error) {
	entriesCh := make(chan *mdns.ServiceEntry, 10)

	// Start collecting entries
	go func() {
		for {
			select {
			case entry, ok := <-entriesCh:
				if !ok {
					return
				}
				device := r.parseServiceEntry(entry)
				if device != nil && device.ID != r.deviceID {
					r.devices[device.ID] = device
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Setup query parameters
	params := mdns.DefaultParams(r.serviceType)
	params.Timeout = timeout
	params.Entries = entriesCh
	params.DisableIPv6 = false // Support both IPv4 and IPv6

	// Perform query
	if err := mdns.Query(params); err != nil {
		return nil, fmt.Errorf("mdns query failed: %w", err)
	}

	// Wait for timeout or context cancellation
	select {
	case <-time.After(timeout):
	case <-ctx.Done():
	}

	// Convert map to slice
	var result []*DeviceInfo
	for _, device := range r.devices {
		result = append(result, device)
	}

	return result, nil
}

// WatchDevices continuously monitors for device changes
func (r *RPiDiscovery) WatchDevices(ctx context.Context, callback func(*DeviceInfo, bool)) error {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	knownDevices := make(map[string]time.Time)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			devices, err := r.BrowseDevices(ctx, 2*time.Second)
			if err != nil {
				continue
			}

			// Check for new or updated devices
			for _, device := range devices {
				lastSeen, exists := knownDevices[device.ID]
				if !exists || device.LastSeen.After(lastSeen) {
					callback(device, !exists) // true if new device
					knownDevices[device.ID] = device.LastSeen
				}
			}

			// Check for offline devices (not seen for 30 seconds)
			now := time.Now()
			for id, lastSeen := range knownDevices {
				if now.Sub(lastSeen) > 30*time.Second {
					// Device went offline
					if device, ok := r.devices[id]; ok {
						device.Address = "" // Clear address to indicate offline
						callback(device, false)
					}
					delete(knownDevices, id)
				}
			}
		}
	}
}

// parseServiceEntry converts mDNS entry to DeviceInfo
func (r *RPiDiscovery) parseServiceEntry(entry *mdns.ServiceEntry) *DeviceInfo {
	if entry == nil {
		return nil
	}

	device := &DeviceInfo{
		Hostname:   entry.Host,
		Port:       entry.Port,
		Properties: make(map[string]string),
		LastSeen:   time.Now(),
	}

	// Get IPv4 address (prefer over IPv6 for simplicity)
	if entry.AddrV4 != nil {
		device.Address = entry.AddrV4.String()
	} else if entry.AddrV6 != nil {
		device.Address = entry.AddrV6.String()
	}

	// Parse TXT records
	for _, field := range entry.InfoFields {
		parts := strings.SplitN(field, "=", 2)
		if len(parts) == 2 {
			key := parts[0]
			value := parts[1]

			switch key {
			case "device_id":
				device.ID = value
			case "device_name":
				device.Name = value
			case "device_type":
				device.Type = value
			case "configured":
				device.Configured = value == "true"
			default:
				device.Properties[key] = value
			}
		}
	}

	// Set defaults
	if device.Name == "" {
		device.Name = device.Hostname
	}
	if device.Type == "" {
		device.Type = "unknown"
	}

	return device
}

// FindK3sNodes discovers Raspberry Pi devices with k3s roles
func (r *RPiDiscovery) FindK3sNodes(ctx context.Context) ([]*DeviceInfo, error) {
	allDevices, err := r.BrowseDevices(ctx, 5*time.Second)
	if err != nil {
		return nil, err
	}

	var k3sNodes []*DeviceInfo
	for _, device := range allDevices {
		if role, ok := device.Properties["k3s_role"]; ok && role != "" {
			k3sNodes = append(k3sNodes, device)
		}
	}

	return k3sNodes, nil
}

// FindUnconfiguredDevices finds devices that need configuration
func (r *RPiDiscovery) FindUnconfiguredDevices(ctx context.Context) ([]*DeviceInfo, error) {
	allDevices, err := r.BrowseDevices(ctx, 5*time.Second)
	if err != nil {
		return nil, err
	}

	var unconfigured []*DeviceInfo
	for _, device := range allDevices {
		if !device.Configured {
			unconfigured = append(unconfigured, device)
		}
	}

	return unconfigured, nil
}

// GetDeviceByID retrieves a specific device by ID
func (r *RPiDiscovery) GetDeviceByID(ctx context.Context, deviceID string) (*DeviceInfo, error) {
	devices, err := r.BrowseDevices(ctx, 2*time.Second)
	if err != nil {
		return nil, err
	}

	for _, device := range devices {
		if device.ID == deviceID {
			return device, nil
		}
	}

	return nil, fmt.Errorf("device not found: %s", deviceID)
}
