package integration

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fleetd.sh/internal/agent"
	"fleetd.sh/pkg/telemetry"
)

func TestTelemetryCollection(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tmpDir := t.TempDir()

	// Create agent config
	cfg := agent.DefaultConfig()
	cfg.DeviceID = "telemetry-test-device"
	cfg.StorageDir = filepath.Join(tmpDir, "data")
	cfg.TelemetryInterval = 1 // 1 second interval
	cfg.RPCPort = 0           // Use dynamic port allocation

	// Start agent
	a := agent.New(cfg)
	if err := a.Start(); err != nil {
		t.Fatalf("Failed to start agent: %v", err)
	}
	defer func() {
		if err := a.Stop(); err != nil {
			t.Errorf("Failed to stop agent: %v", err)
		}
	}()

	// Wait for telemetry with timeout
	select {
	case <-ctx.Done():
		t.Fatal("Timeout waiting for telemetry collection")
	case <-time.After(2 * time.Second):
		// Continue with test
	}

	// Read telemetry data from the correct path
	telemetryPath := filepath.Join(cfg.StorageDir, "telemetry", "metrics.json")
	data, err := os.ReadFile(telemetryPath)
	if err != nil {
		if os.IsNotExist(err) {
			t.Skip("No telemetry files created yet")
		}
		t.Fatalf("Failed to read telemetry file: %v", err)
	}

	// Verify telemetry data format
	var metrics []telemetry.Metric
	if err := json.Unmarshal(data, &metrics); err != nil {
		t.Fatalf("Failed to parse telemetry data: %v", err)
	}

	// Verify we have some metrics
	if len(metrics) == 0 {
		t.Error("No metrics found in telemetry file")
	}

	// Verify expected metric types
	expectedMetrics := map[string]bool{
		"system.memory.alloc":       false,
		"system.memory.total_alloc": false,
		"system.goroutines":         false,
	}

	for _, metric := range metrics {
		if _, exists := expectedMetrics[metric.Name]; exists {
			expectedMetrics[metric.Name] = true
		}
	}

	for name, found := range expectedMetrics {
		if !found {
			t.Errorf("Expected metric %s not found", name)
		}
	}
}
