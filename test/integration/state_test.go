package integration

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"fleetd.sh/internal/agent"
)

func TestStatePersistence(t *testing.T) {
	if testing.Short() || os.Getenv("INTEGRATION") == "" {
		t.Skip("Skipping integration test")
	}

	tmpDir := t.TempDir()

	// Create initial agent
	cfg := agent.DefaultConfig()
	cfg.DeviceID = "state-test-device"
	cfg.LocalStoragePath = filepath.Join(tmpDir, "data")

	// Start first agent instance
	a1 := agent.New(cfg)
	if err := a1.Start(); err != nil {
		t.Fatalf("Failed to start first agent: %v", err)
	}

	// Update binary state through agent interface
	if err := a1.UpdateBinaryState("test-binary", "1.0.0", "running"); err != nil {
		t.Fatalf("Failed to update binary state: %v", err)
	}

	// Stop first agent
	if err := a1.Stop(); err != nil {
		t.Fatalf("Failed to stop first agent: %v", err)
	}

	// Give filesystem time to sync
	time.Sleep(100 * time.Millisecond)

	// Verify state file exists
	stateFile := filepath.Join(tmpDir, "data", "state", "state.json")
	if _, err := os.Stat(stateFile); os.IsNotExist(err) {
		t.Error("State file was not created")
	}

	// Start second agent instance with same config
	a2 := agent.New(cfg)
	if err := a2.Start(); err != nil {
		t.Fatalf("Failed to start second agent: %v", err)
	}
	defer a2.Stop()
}
