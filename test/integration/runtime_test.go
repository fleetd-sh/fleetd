package integration

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fleetd.sh/internal/runtime"
)

func TestBinaryDeployment(t *testing.T) {
	if testing.Short() || os.Getenv("INTEGRATION") == "" {
		t.Skip("Skipping integration test")
	}

	tmpDir := t.TempDir()

	// Create test binary that sleeps
	binaryPath := filepath.Join(tmpDir, "test-binary")
	data := []byte(`#!/bin/sh
trap 'exit 0' TERM
sleep 10
`)
	if err := os.WriteFile(binaryPath, data, 0o755); err != nil {
		t.Fatalf("Failed to create test binary: %v", err)
	}

	// Create runtime
	rt, err := runtime.New(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create runtime: %v", err)
	}

	// Deploy binary
	if err := rt.Deploy("test", bytes.NewReader(data)); err != nil {
		t.Fatalf("Failed to deploy binary: %v", err)
	}

	// Start binary
	if err := rt.Start("test", []string{}, &runtime.Config{}); err != nil {
		t.Fatalf("Failed to start binary: %v", err)
	}

	// Ensure cleanup on test completion
	defer func() {
		running, err := rt.IsRunning("test")
		if err != nil {
			t.Errorf("Failed to check if binary is running: %v", err)
			return
		}
		if running {
			if err := rt.Stop("test"); err != nil {
				t.Errorf("Failed to stop binary during cleanup: %v", err)
			}
			// Wait a bit for process to exit
			time.Sleep(50 * time.Millisecond)
		}
	}()

	// Verify binary is running
	running, err := rt.IsRunning("test")
	if err != nil {
		t.Fatalf("Failed to check if binary is running: %v", err)
	}
	if !running {
		t.Fatal("Binary should be running")
	}

	// Stop binary
	if err := rt.Stop("test"); err != nil {
		t.Fatalf("Failed to stop binary: %v", err)
	}

	// Give process time to exit
	time.Sleep(50 * time.Millisecond)

	// Verify binary is stopped
	running, err = rt.IsRunning("test")
	if err != nil {
		t.Fatalf("Failed to check if binary is running: %v", err)
	}
	if running {
		t.Error("Binary still running after stop command")
	}
}
