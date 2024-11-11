package runtime

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRuntime(t *testing.T) {
	tmpDir := t.TempDir()

	rt, err := New(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create runtime: %v", err)
	}

	// Test binary deployment
	testBinary := []byte("#!/bin/sh\necho 'test'")
	err = rt.Deploy("test.sh", bytes.NewReader(testBinary))
	if err != nil {
		t.Fatalf("Failed to deploy binary: %v", err)
	}

	// Verify binary exists
	binPath := filepath.Join(tmpDir, "test.sh")
	if _, err := os.Stat(binPath); os.IsNotExist(err) {
		t.Error("Binary file was not created")
	}

	// Test binary execution
	err = rt.Start("test.sh", []string{}, &Config{})
	if err != nil {
		t.Fatalf("Failed to start binary: %v", err)
	}

	// Give process time to start
	time.Sleep(50 * time.Millisecond)

	// Test process listing
	binaries, err := rt.List()
	if err != nil {
		t.Fatalf("Failed to list binaries: %v", err)
	}
	if len(binaries) != 1 || binaries[0] != "test.sh" {
		t.Errorf("Expected ['test.sh'], got %v", binaries)
	}

	// Test process stop
	err = rt.Stop("test.sh")
	if err != nil {
		t.Fatalf("Failed to stop process: %v", err)
	}
}

func TestIsRunning(t *testing.T) {
	tmpDir := t.TempDir()

	r, err := New(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create runtime: %v", err)
	}

	// Test with non-existent binary
	running, err := r.IsRunning("nonexistent")
	if err != nil {
		t.Errorf("IsRunning failed for nonexistent binary: %v", err)
	}
	if running {
		t.Error("Expected nonexistent binary to not be running")
	}

	// Create and deploy test script
	testScript := []byte(`#!/bin/sh
while true; do
  sleep 0.1
done
`)
	if err := r.Deploy("test-script", bytes.NewReader(testScript)); err != nil {
		t.Fatalf("Failed to deploy test script: %v", err)
	}

	// Test before starting
	running, err = r.IsRunning("test-script")
	if err != nil {
		t.Errorf("IsRunning failed before start: %v", err)
	}
	if running {
		t.Error("Expected script to not be running before start")
	}

	// Start script and test
	if err := r.Start("test-script", []string{}, &Config{}); err != nil {
		t.Fatalf("Failed to start script: %v", err)
	}

	time.Sleep(50 * time.Millisecond) // Give process time to start

	running, err = r.IsRunning("test-script")
	if err != nil {
		t.Errorf("IsRunning failed after start: %v", err)
	}
	if !running {
		t.Error("Expected script to be running after start")
	}

	// Stop script and test
	if err := r.Stop("test-script"); err != nil {
		t.Fatalf("Failed to stop script: %v", err)
	}

	time.Sleep(50 * time.Millisecond) // Give process time to stop

	running, err = r.IsRunning("test-script")
	if err != nil {
		t.Errorf("IsRunning failed after stop: %v", err)
	}
	if running {
		t.Error("Expected script to not be running after stop")
	}
}
