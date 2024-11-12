package integration

import (
	"os"
	"testing"
	"time"

	"fleetd.sh/internal/agent"
	"fleetd.sh/internal/discovery"
)

const (
	TestClientPort = 5358
	Config1Port    = 5359
	Config2Port    = 5360
)

func TestDeviceDiscovery(t *testing.T) {
	if testing.Short() || os.Getenv("INTEGRATION") == "" {
		t.Skip("Skipping integration test")
	}

	// Create two agents
	cfg1 := agent.DefaultConfig()
	cfg1.DeviceID = "test-device-1"
	cfg1.EnableMDNS = true
	cfg1.MDNSPort = Config1Port
	cfg1.LocalStoragePath = t.TempDir()

	cfg2 := agent.DefaultConfig()
	cfg2.DeviceID = "test-device-2"
	cfg2.EnableMDNS = true
	cfg2.MDNSPort = Config2Port
	cfg2.LocalStoragePath = t.TempDir()

	a1 := agent.New(cfg1)
	a2 := agent.New(cfg2)

	// Start both agents
	if err := a1.Start(); err != nil {
		t.Fatalf("Failed to start agent 1: %v", err)
	}
	defer a1.Stop()

	if err := a2.Start(); err != nil {
		t.Fatalf("Failed to start agent 2: %v", err)
	}
	defer a2.Stop()

	// Give discovery time to initialize
	time.Sleep(1 * time.Second)

	// Create discovery client with timeout
	d := discovery.New("test-client", TestClientPort)

	// Try multiple discovery attempts
	found := make(map[string]bool)
	var devices []string
	var err error

	for i := 0; i < 3; i++ {
		devices, err = d.Browse(2 * time.Second)
		if err != nil {
			t.Logf("Browse attempt %d failed: %v", i+1, err)
			continue
		}

		// Check if we found both devices
		for _, device := range devices {
			found[device] = true
			t.Logf("Found device: %s", device)
		}

		if found[cfg1.DeviceID] && found[cfg2.DeviceID] {
			return // Success
		}

		// Wait before next attempt
		time.Sleep(500 * time.Millisecond)
	}

	// If we get here, we didn't find both devices
	t.Errorf("Failed to discover both devices. Found: %v", devices)
}
