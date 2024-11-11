package agent

import (
	"context"
	"testing"
)

func TestBinaryManagement(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &Config{
		DeviceID:          "test-device",
		StorageDir:        tmpDir,
		TelemetryInterval: 60,
		DisableMDNS:       true,
	}

	agent := New(cfg)
	if err := agent.Start(); err != nil {
		t.Fatalf("Failed to start agent: %v", err)
	}
	defer agent.Stop()

	// Test binary deployment
	testScript := []byte(`#!/bin/sh
while true; do
  sleep 0.1
done
`)

	if err := agent.DeployBinary("test-script", testScript); err != nil {
		t.Fatalf("Failed to deploy binary: %v", err)
	}

	// Test binary listing after deployment
	binaries, err := agent.ListBinaries()
	if err != nil {
		t.Fatalf("Failed to list binaries: %v", err)
	}
	if len(binaries) != 1 {
		t.Errorf("Expected 1 binary, got %d", len(binaries))
	}
	if binaries[0].Name != "test-script" || binaries[0].Status != "deployed" {
		t.Errorf("Unexpected binary state: %+v", binaries[0])
	}

	// Test starting binary
	if err := agent.StartBinary("test-script", []string{}); err != nil {
		t.Fatalf("Failed to start binary: %v", err)
	}

	if err := agent.WaitForReady(context.Background()); err != nil {
		t.Fatalf("Failed waiting for binary to start: %v", err)
	}

	// Verify running state
	binaries, err = agent.ListBinaries()
	if err != nil {
		t.Fatalf("Failed to list binaries after start: %v", err)
	}
	if len(binaries) != 1 || binaries[0].Status != "running" {
		t.Errorf("Expected binary to be running, got status: %s", binaries[0].Status)
	}

	// Test stopping binary
	if err := agent.StopBinary("test-script"); err != nil {
		t.Fatalf("Failed to stop binary: %v", err)
	}

	if err := agent.WaitForReady(context.Background()); err != nil {
		t.Fatalf("Failed waiting for binary to stop: %v", err)
	}

	// Verify stopped state
	binaries, err = agent.ListBinaries()
	if err != nil {
		t.Fatalf("Failed to list binaries after stop: %v", err)
	}
	if len(binaries) != 1 || binaries[0].Status != "stopped" {
		t.Errorf("Expected binary to be stopped, got status: %s", binaries[0].Status)
	}
}

func TestUpdateBinaryState(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &Config{
		DeviceID:          "test-device",
		StorageDir:        tmpDir,
		TelemetryInterval: 60,
		DisableMDNS:       true,
	}

	agent := New(cfg)
	if err := agent.Start(); err != nil {
		t.Fatalf("Failed to start agent: %v", err)
	}
	defer agent.Stop()

	// Test updating binary state
	if err := agent.UpdateBinaryState("test-binary", "1.0.0", "running"); err != nil {
		t.Fatalf("Failed to update binary state: %v", err)
	}

	// Verify state was updated
	binaries, err := agent.ListBinaries()
	if err != nil {
		t.Fatalf("Failed to list binaries: %v", err)
	}
	if len(binaries) != 1 {
		t.Fatalf("Expected 1 binary, got %d", len(binaries))
	}

	binary := binaries[0]
	if binary.Name != "test-binary" ||
		binary.Version != "1.0.0" ||
		binary.Status != "running" {
		t.Errorf("Unexpected binary state: %+v", binary)
	}
}
