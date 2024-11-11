package integration

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"fleetd.sh/internal/agent"
	"fleetd.sh/internal/discovery"
)

func createTestAgent(t *testing.T, deviceID string, rpcPort, mDNSPort int) *agent.Agent {
	tmpDir := t.TempDir()
	cfg := agent.DefaultConfig()
	cfg.DeviceID = deviceID
	cfg.StorageDir = filepath.Join(tmpDir, deviceID)
	cfg.ServerURL = "http://localhost:8080"
	cfg.RPCPort = rpcPort
	cfg.EnableMDNS = true
	cfg.MDNSPort = mDNSPort
	cfg.DisableMDNS = false
	cfg.ServiceType = "_fleetd-test._tcp"
	return agent.New(cfg)
}

func TestDeviceDiscovery(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Create two agents with different configs
	agent1 := createTestAgent(t, "test-device-1", 0, 5359)
	agent2 := createTestAgent(t, "test-device-2", 8081, 5360)

	// Start both agents
	if err := agent1.Start(); err != nil {
		t.Fatalf("Failed to start agent1: %v", err)
	}
	if err := agent2.Start(); err != nil {
		t.Fatalf("Failed to start agent2: %v", err)
	}

	// Ensure cleanup
	defer func() {
		agent1.Stop()
		agent2.Stop()
	}()

	// Wait for both agents to be ready
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := agent1.WaitForReady(ctx); err != nil {
		t.Fatalf("Agent1 failed to become ready: %v", err)
	}
	if err := agent2.WaitForReady(ctx); err != nil {
		t.Fatalf("Agent2 failed to become ready: %v", err)
	}

	// Give mDNS services time to advertise
	time.Sleep(500 * time.Millisecond)

	// Create discovery browser with longer timeouts
	browser := discovery.NewBrowser("_fleetd-test._tcp")
	defer browser.Stop()

	// Track discovered devices across retries
	discoveredDevices := make(map[string]bool)
	maxRetries := 5
	discoveryTimeout := 1 * time.Second

	for i := 0; i < maxRetries; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), discoveryTimeout)
		devices, err := browser.Browse(ctx, discoveryTimeout)
		cancel()

		if err != nil {
			t.Logf("Browse attempt %d failed: %v", i+1, err)
			continue
		}

		// Add newly discovered devices to our map
		for _, device := range devices {
			discoveredDevices[device] = true
		}

		t.Logf("Browse attempt %d found devices: %v", i+1, devices)

		// Break if we've found both devices
		if discoveredDevices["test-device-1"] && discoveredDevices["test-device-2"] {
			break
		}

		// Wait between retries
		time.Sleep(500 * time.Millisecond)
	}

	// Convert map to slice for assertion
	var deviceList []string
	for device := range discoveredDevices {
		deviceList = append(deviceList, device)
	}

	// Detailed failure message
	if len(deviceList) != 2 {
		t.Errorf("Expected to discover both test devices\nFound devices: %v\nDiscovered over time: %v",
			deviceList, discoveredDevices)
	}

	// Verify specific devices
	if !discoveredDevices["test-device-1"] {
		t.Error("Failed to discover test-device-1")
	}
	if !discoveredDevices["test-device-2"] {
		t.Error("Failed to discover test-device-2")
	}
}
