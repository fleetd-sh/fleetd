package update

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"testing"
	"time"
)

func TestUpdater(t *testing.T) {
	// Create temporary test directory
	tmpDir := t.TempDir()

	// Create test binary data
	testData := []byte("test binary data")
	hash := sha256.Sum256(testData)
	hashStr := hex.EncodeToString(hash[:])

	// Create test update info
	info := UpdateInfo{
		Version:     "1.0.0",
		SHA256:      hashStr,
		ReleaseDate: time.Now(),
	}

	// Initialize updater
	updater, err := New(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create updater: %v", err)
	}

	// Test update process
	err = updater.Update(context.Background(), bytes.NewReader(testData), info)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Verify backup file exists
	if _, err := os.Stat(updater.backupPath); os.IsNotExist(err) {
		t.Error("Backup file was not created")
	}

	// Test cleanup
	err = updater.Cleanup()
	if err != nil {
		t.Errorf("Cleanup failed: %v", err)
	}

	// Verify cleanup removed files
	if _, err := os.Stat(updater.stagingPath); !os.IsNotExist(err) {
		t.Error("Staging file was not cleaned up")
	}
	if _, err := os.Stat(updater.backupPath); !os.IsNotExist(err) {
		t.Error("Backup file was not cleaned up")
	}
}

func TestUpdaterChecksumMismatch(t *testing.T) {
	tmpDir := t.TempDir()

	testData := []byte("test binary data")
	info := UpdateInfo{
		Version:     "1.0.0",
		SHA256:      "invalid checksum",
		ReleaseDate: time.Now(),
	}

	updater, err := New(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create updater: %v", err)
	}

	err = updater.Update(context.Background(), bytes.NewReader(testData), info)
	if err == nil {
		t.Error("Expected checksum mismatch error")
	}
}
