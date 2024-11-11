package handlers

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fleetd.sh/pkg/telemetry"
)

func TestLocalFileHandler(t *testing.T) {
	tmpDir := t.TempDir()

	filePath := filepath.Join(tmpDir, "telemetry.json")
	handler, err := NewLocalFile(filePath)
	if err != nil {
		t.Fatalf("Failed to create handler: %v", err)
	}

	// Test metric handling
	testMetrics := []telemetry.Metric{
		{
			Name:      "test.metric",
			Value:     42,
			Timestamp: time.Now(),
			Labels: telemetry.Labels{
				"test": "label",
			},
		},
	}

	err = handler.Handle(context.Background(), testMetrics)
	if err != nil {
		t.Fatalf("Failed to handle metrics: %v", err)
	}

	// Verify file contents
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read telemetry file: %v", err)
	}

	var savedMetrics []telemetry.Metric
	err = json.Unmarshal(data, &savedMetrics)
	if err != nil {
		t.Fatalf("Failed to parse telemetry file: %v", err)
	}

	if len(savedMetrics) != 1 {
		t.Fatalf("Expected 1 metric, got %d", len(savedMetrics))
	}

	if savedMetrics[0].Name != testMetrics[0].Name {
		t.Errorf("Expected metric name %s, got %s", testMetrics[0].Name, savedMetrics[0].Name)
	}
}
