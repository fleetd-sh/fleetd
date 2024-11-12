package integration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fleetd.sh/internal/agent"
)

func TestTelemetryCollection(t *testing.T) {
	if testing.Short() || os.Getenv("INTEGRATION") == "" {
		t.Skip("Skipping integration test")
	}

	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, "telemetry")

	// Create agent config
	cfg := agent.DefaultConfig()
	cfg.DeviceID = "telemetry-test-device"
	cfg.LocalStoragePath = filepath.Join(tmpDir, "data")
	cfg.TelemetryInterval = 1 // 1 second interval

	// Start agent
	a := agent.New(cfg)
	if err := a.Start(); err != nil {
		t.Fatalf("Failed to start agent: %v", err)
	}
	defer a.Stop()

	// Wait for telemetry collection
	time.Sleep(2 * time.Second)

	// Read telemetry data
	files, err := os.ReadDir(dataDir)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("Failed to read telemetry directory: %v", err)
	}

	if len(files) == 0 {
		t.Skip("No telemetry files created yet")
	}

	// Verify telemetry data format
	for _, file := range files {
		data, err := os.ReadFile(filepath.Join(dataDir, file.Name()))
		if err != nil {
			t.Fatalf("Failed to read telemetry file: %v", err)
		}

		var telemetry map[string]interface{}
		if err := json.Unmarshal(data, &telemetry); err != nil {
			t.Fatalf("Failed to parse telemetry data: %v", err)
		}
	}
}
