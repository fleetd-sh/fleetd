package discovery

import (
	"strings"
	"testing"
	"time"
)

const (
	D1Port               = 5354
	D2Port               = 5355
	DiscoveryStopPort    = 5356
	DiscoveryTimeoutPort = 5357
)

func TestDiscovery(t *testing.T) {
	// Create two discoveries with different device IDs
	d1 := New("test-device-1", D1Port)
	d2 := New("test-device-2", D2Port)

	// Start both discoveries
	err := d1.Start()
	if err != nil {
		// Skip if we can't bind to mDNS ports
		if isBindError(err) {
			t.Skip("Skipping test due to mDNS binding issues:", err)
		}
		t.Fatalf("Failed to start discovery 1: %v", err)
	}
	defer d1.Stop()

	err = d2.Start()
	if err != nil {
		if isBindError(err) {
			t.Skip("Skipping test due to mDNS binding issues:", err)
		}
		t.Fatalf("Failed to start discovery 2: %v", err)
	}
	defer d2.Stop()

	// Give services time to register
	time.Sleep(100 * time.Millisecond)

	// Test discovery
	t.Run("Device 1 discovers Device 2", func(t *testing.T) {
		devices, err := d1.Browse(2 * time.Second)
		if err != nil {
			if isNetworkError(err) {
				t.Skip("Skipping test due to network issues:", err)
			}
			t.Fatalf("Failed to browse devices: %v", err)
		}

		found := false
		for _, device := range devices {
			if device == "test-device-2" {
				found = true
				break
			}
		}

		if !found {
			t.Error("Device 2 was not discovered")
		}
	})

	t.Run("Device 2 discovers Device 1", func(t *testing.T) {
		devices, err := d2.Browse(2 * time.Second)
		if err != nil {
			if isNetworkError(err) {
				t.Skip("Skipping test due to network issues:", err)
			}
			t.Fatalf("Failed to browse devices: %v", err)
		}

		found := false
		for _, device := range devices {
			if device == "test-device-1" {
				found = true
				break
			}
		}

		if !found {
			t.Error("Device 1 was not discovered")
		}
	})
}

// Helper functions to identify common errors that should cause test skips
func isBindError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return contains(errStr,
		"bind",
		"address already in use",
		"permission denied",
		"cannot assign requested address",
	)
}

func isNetworkError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return contains(errStr,
		"no route to host",
		"network is unreachable",
		"connection refused",
	)
}

// contains checks if str contains any of the given substrings
func contains(str string, substrings ...string) bool {
	for _, sub := range substrings {
		if strings.Contains(str, sub) {
			return true
		}
	}
	return false
}

func TestDiscoveryStop(t *testing.T) {
	d := New("test-device", DiscoveryStopPort)

	// Test stopping without starting
	err := d.Stop()
	if err != nil {
		t.Errorf("Stop should not error when not started: %v", err)
	}

	// Start and then stop
	err = d.Start()
	if err != nil {
		if isBindError(err) {
			t.Skip("Skipping test due to mDNS binding issues:", err)
		}
		t.Fatalf("Failed to start discovery: %v", err)
	}

	err = d.Stop()
	if err != nil {
		t.Errorf("Failed to stop discovery: %v", err)
	}

	// Verify server is nil after stop
	if d.server != nil {
		t.Error("Server should be nil after stop")
	}
}

func TestDiscoveryBrowseTimeout(t *testing.T) {
	d := New("test-device", DiscoveryTimeoutPort)

	// Test with very short timeout
	_, err := d.Browse(1 * time.Millisecond)
	if err != nil {
		if isNetworkError(err) {
			t.Skip("Skipping test due to network issues:", err)
		}
		t.Fatalf("Browse failed with short timeout: %v", err)
	}
}
