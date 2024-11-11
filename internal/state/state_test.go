package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestStateManager(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	manager, err := New(statePath)
	if err != nil {
		t.Fatalf("Failed to create state manager: %v", err)
	}

	// Make initial state update
	err = manager.Update(func(s *State) error {
		s.Version = 1
		s.DeviceInfo = DeviceInfo{
			ID: "test-device",
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Failed to update state: %v", err)
	}

	// Verify initial state file exists
	if _, err := os.Stat(statePath); os.IsNotExist(err) {
		t.Error("State file was not created")
	}

	// Make second update to trigger backup
	err = manager.Update(func(s *State) error {
		s.Version = 2
		return nil
	})
	if err != nil {
		t.Fatalf("Failed to update state second time: %v", err)
	}

	// Verify backup file exists
	backupPath := statePath + ".bak"
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		t.Error("Backup file was not created")
	}

	// Verify backup contains version 1
	backupData, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("Failed to read backup file: %v", err)
	}

	var backupState State
	if err := json.Unmarshal(backupData, &backupState); err != nil {
		t.Fatalf("Failed to parse backup state: %v", err)
	}

	if backupState.Version != 1 {
		t.Errorf("Expected backup version to be 1, got %d", backupState.Version)
	}

	// Verify current state has version 2
	currentState := manager.Get()
	if currentState.Version != 2 {
		t.Errorf("Expected current version to be 2, got %d", currentState.Version)
	}
}
