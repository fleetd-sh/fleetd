package integration

import (
	"context"
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tmpDir := t.TempDir()

	cfg := agent.DefaultConfig()
	cfg.DeviceID = "state-test-device"
	cfg.StorageDir = filepath.Join(tmpDir, "data")

	a1 := agent.New(cfg)
	if err := a1.Start(); err != nil {
		t.Fatalf("Failed to start first agent: %v", err)
	}
	defer func() {
		if err := a1.Stop(); err != nil {
			t.Errorf("Failed to stop first agent: %v", err)
		}
	}()

	if err := a1.UpdateBinaryState("test-binary", "1.0.0", "running"); err != nil {
		t.Fatalf("Failed to update binary state: %v", err)
	}

	// Stop first agent
	if err := a1.Stop(); err != nil {
		t.Fatalf("Failed to stop first agent: %v", err)
	}

	// Wait for filesystem sync with timeout
	select {
	case <-ctx.Done():
		t.Fatal("Timeout waiting for state persistence")
	case <-time.After(50 * time.Millisecond):
		// Continue with test
	}

	stateFile := filepath.Join(tmpDir, "data", "state", "state.json")
	if _, err := os.Stat(stateFile); os.IsNotExist(err) {
		t.Error("State file was not created")
	}
}
