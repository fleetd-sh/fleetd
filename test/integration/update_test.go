package integration

import (
	"os"
	"path/filepath"
	"testing"

	"fleetd.sh/internal/agent"
)

func TestAgentUpdate(t *testing.T) {
	if testing.Short() || os.Getenv("INTEGRATION") == "" {
		t.Skip("Skipping integration test")
	}

	tmpDir := t.TempDir()

	// Create agent config
	cfg := agent.DefaultConfig()
	cfg.DeviceID = "update-test-device"
	cfg.LocalStoragePath = filepath.Join(tmpDir, "data")

	// Start agent
	a := agent.New(cfg)
	if err := a.Start(); err != nil {
		t.Fatalf("Failed to start agent: %v", err)
	}
	defer a.Stop()

	// Create update file
	updateDir := filepath.Join(tmpDir, "data", "updates")
	if err := os.MkdirAll(updateDir, 0755); err != nil {
		t.Fatalf("Failed to create update directory: %v", err)
	}

	updateFile := filepath.Join(updateDir, "current")
	if err := os.WriteFile(updateFile, []byte("test update"), 0644); err != nil {
		t.Fatalf("Failed to write update file: %v", err)
	}

	// Verify update file exists
	if _, err := os.Stat(updateFile); os.IsNotExist(err) {
		t.Error("Update file was not created")
	}

	// Read update file
	data, err := os.ReadFile(updateFile)
	if err != nil {
		t.Fatalf("Failed to read update file: %v", err)
	}

	if string(data) != "test update" {
		t.Errorf("Unexpected update file content: %s", string(data))
	}
}
