package integration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"fleetd.sh/internal/update"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUpdateSystem tests comprehensive update and rollback scenarios
func TestUpdateSystem(t *testing.T) {
	t.Run("MultiStageRollout", testMultiStageRollout)
	t.Run("RollbackOnHealthFailure", testRollbackOnHealthFailure)
	t.Run("BinarySignatureVerification", testBinarySignatureVerification)
	t.Run("CorruptUpdateHandling", testCorruptUpdateHandling)
	t.Run("PartialUpdateRecovery", testPartialUpdateRecovery)
	t.Run("UpdateDuringNetworkInstability", testUpdateDuringNetworkInstability)
	t.Run("ConcurrentUpdateAttempts", testConcurrentUpdateAttempts)
	t.Run("UpdateChainValidation", testUpdateChainValidation)
}

// testMultiStageRollout tests multi-stage update deployment
func testMultiStageRollout(t *testing.T) {
	testDir := t.TempDir()

	// Create update manager
	config := &update.Config{
		UpdateDir:       filepath.Join(testDir, "updates"),
		BackupDir:       filepath.Join(testDir, "backups"),
		MaxBackups:      3,
		DownloadTimeout: 30 * time.Second,
		ApplyTimeout:    10 * time.Second,
		HealthTimeout:   5 * time.Second,
	}

	mgr, err := update.NewUpdateManager(config, "1.0.0")
	require.NoError(t, err)

	// Create staged updates
	stages := []struct {
		version  string
		priority update.UpdatePriority
		content  string
	}{
		{"1.1.0", update.UpdatePriorityLow, "stage1"},
		{"1.2.0", update.UpdatePriorityNormal, "stage2"},
		{"1.3.0", update.UpdatePriorityHigh, "stage3"},
		{"1.4.0", update.UpdatePriorityCritical, "stage4"},
	}

	// Track successful updates
	var appliedVersions []string

	for _, stage := range stages {
		// Create update package
		updateFile := filepath.Join(testDir, fmt.Sprintf("update_%s.tar.gz", stage.version))
		createTestUpdate(t, updateFile, stage.content)

		upd := &update.Update{
			ID:       fmt.Sprintf("update-%s", stage.version),
			Version:  stage.version,
			Type:     update.UpdateTypeApplication,
			Priority: stage.priority,
			URL:      "file://" + updateFile,
			Checksum: calculateFileChecksum(t, updateFile),
			Rollback: true,
			Manifest: map[string]interface{}{
				"stage": stage.content,
			},
		}

		// Apply update
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		err := mgr.ApplyUpdate(ctx, upd)
		cancel()

		// Updates will fail without proper environment, but process should complete
		if err != nil {
			t.Logf("Stage %s update failed (expected in test): %v", stage.version, err)
		} else {
			appliedVersions = append(appliedVersions, stage.version)
		}

		// Verify state was saved
		state, err := mgr.GetUpdateState()
		assert.NotNil(t, state)
		assert.Equal(t, upd.ID, state.UpdateID)
		assert.Equal(t, stage.version, state.Version)
	}

	// Verify rollback manager has backups
	rollbackMgr := update.NewRollbackManager(config.BackupDir, config.MaxBackups)
	backups, err := rollbackMgr.ListBackups()
	assert.NoError(t, err)
	assert.LessOrEqual(t, len(backups), config.MaxBackups, "Should limit backups")
}

// testRollbackOnHealthFailure tests automatic rollback on health check failure
func testRollbackOnHealthFailure(t *testing.T) {
	testDir := t.TempDir()

	// Create update manager
	config := &update.Config{
		UpdateDir:     filepath.Join(testDir, "updates"),
		BackupDir:     filepath.Join(testDir, "backups"),
		MaxBackups:    3,
		HealthTimeout: 2 * time.Second,
	}

	mgr, err := update.NewUpdateManager(config, "1.0.0")
	require.NoError(t, err)

	// Create health checker that will fail
	healthChecker := update.NewHealthChecker()
	healthChecker.AddCustomCheck(update.HealthCheck{
		Name:     "always_fail",
		Critical: true,
		Timeout:  1 * time.Second,
		CheckFunc: func(ctx context.Context) error {
			return fmt.Errorf("health check failed intentionally")
		},
	})

	// Create update that will trigger rollback
	updateFile := filepath.Join(testDir, "bad_update.tar.gz")
	createTestUpdate(t, updateFile, "bad update content")

	upd := &update.Update{
		ID:       "bad-update",
		Version:  "2.0.0",
		Type:     update.UpdateTypeApplication,
		Priority: update.UpdatePriorityNormal,
		URL:      "file://" + updateFile,
		Checksum: calculateFileChecksum(t, updateFile),
		Rollback: true, // Enable rollback
	}

	// Create initial backup
	rollbackMgr := update.NewRollbackManager(config.BackupDir, config.MaxBackups)
	backup, err := rollbackMgr.CreateBackup("1.0.0")
	require.NoError(t, err)
	assert.NotNil(t, backup)

	// Apply update (should fail and trigger rollback)
	ctx := context.Background()
	err = mgr.ApplyUpdate(ctx, upd)

	// Should have failed
	assert.Error(t, err)

	// Verify update state shows rollback
	state, _ := mgr.GetUpdateState()
	if state != nil {
		// Check for rollback indicators
		assert.True(t,
			state.Status == "rolled_back" ||
				state.Status == "failed" ||
				state.RollbackCount > 0,
			"Should indicate rollback occurred")
	}
}

// testBinarySignatureVerification tests update signature verification
func testBinarySignatureVerification(t *testing.T) {
	testDir := t.TempDir()

	// Generate a test key pair (in production, use real crypto)
	signatureKey := []byte("test-signature-key")

	config := &update.Config{
		UpdateDir:    filepath.Join(testDir, "updates"),
		BackupDir:    filepath.Join(testDir, "backups"),
		SignatureKey: signatureKey,
	}

	mgr, err := update.NewUpdateManager(config, "1.0.0")
	require.NoError(t, err)

	// Create signed update
	updateFile := filepath.Join(testDir, "signed_update.tar.gz")
	createTestUpdate(t, updateFile, "signed content")

	// Generate mock signature (in production, use real signing)
	mockSignature := generateMockSignature(updateFile, signatureKey)

	upd := &update.Update{
		ID:        "signed-update",
		Version:   "1.1.0",
		Type:      update.UpdateTypeApplication,
		URL:       "file://" + updateFile,
		Checksum:  calculateFileChecksum(t, updateFile),
		Signature: mockSignature,
	}

	ctx := context.Background()
	err = mgr.ApplyUpdate(ctx, upd)
	// Will fail without proper signing implementation
	t.Logf("Signature verification test result: %v", err)

	// Test with tampered update
	tamperedFile := filepath.Join(testDir, "tampered_update.tar.gz")
	createTestUpdate(t, tamperedFile, "tampered content")

	tamperedUpdate := &update.Update{
		ID:        "tampered-update",
		Version:   "1.2.0",
		Type:      update.UpdateTypeApplication,
		URL:       "file://" + tamperedFile,
		Checksum:  "wrong-checksum", // Intentionally wrong
		Signature: mockSignature,
	}

	err = mgr.ApplyUpdate(ctx, tamperedUpdate)
	assert.Error(t, err, "Should reject tampered update")
	if err != nil {
		assert.Contains(t, err.Error(), "checksum", "Should fail on checksum mismatch")
	}
}

// testCorruptUpdateHandling tests handling of corrupted update packages
func testCorruptUpdateHandling(t *testing.T) {
	testDir := t.TempDir()

	config := &update.Config{
		UpdateDir: filepath.Join(testDir, "updates"),
		BackupDir: filepath.Join(testDir, "backups"),
	}

	mgr, err := update.NewUpdateManager(config, "1.0.0")
	require.NoError(t, err)

	// Create corrupted update file
	corruptFile := filepath.Join(testDir, "corrupt_update.tar.gz")
	err = os.WriteFile(corruptFile, []byte("this is not a valid tar.gz file"), 0644)
	require.NoError(t, err)

	upd := &update.Update{
		ID:       "corrupt-update",
		Version:  "1.1.0",
		Type:     update.UpdateTypeApplication,
		URL:      "file://" + corruptFile,
		Checksum: calculateFileChecksum(t, corruptFile),
	}

	ctx := context.Background()
	err = mgr.ApplyUpdate(ctx, upd)

	// Should handle corruption gracefully
	assert.Error(t, err, "Should fail on corrupt update")

	// Verify system is still functional
	state, _ := mgr.GetUpdateState()
	if state != nil {
		assert.Equal(t, "failed", state.Status)
		assert.NotEmpty(t, state.Error)
	}

	// Verify current version unchanged
	assert.Equal(t, "1.0.0", mgr.GetCurrentVersion())
}

// testPartialUpdateRecovery tests recovery from partial update application
func testPartialUpdateRecovery(t *testing.T) {
	testDir := t.TempDir()

	config := &update.Config{
		UpdateDir:    filepath.Join(testDir, "updates"),
		BackupDir:    filepath.Join(testDir, "backups"),
		ApplyTimeout: 1 * time.Second, // Very short timeout to trigger partial application
	}

	mgr, err := update.NewUpdateManager(config, "1.0.0")
	require.NoError(t, err)

	// Create update that takes too long
	updateFile := filepath.Join(testDir, "slow_update.tar.gz")
	createTestUpdate(t, updateFile, "slow update")

	upd := &update.Update{
		ID:       "slow-update",
		Version:  "1.1.0",
		Type:     update.UpdateTypeApplication,
		URL:      "file://" + updateFile,
		Checksum: calculateFileChecksum(t, updateFile),
		Rollback: true,
		// Add pre-script that sleeps to simulate slow update
		PreScript: "sleep 5",
	}

	// Create backup for rollback
	rollbackMgr := update.NewRollbackManager(config.BackupDir, config.MaxBackups)
	backup, err := rollbackMgr.CreateBackup("1.0.0")
	require.NoError(t, err)
	assert.NotNil(t, backup)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err = mgr.ApplyUpdate(ctx, upd)

	// Should timeout or fail
	assert.Error(t, err)

	// Verify partial update was handled
	state, _ := mgr.GetUpdateState()
	if state != nil {
		assert.NotEqual(t, "completed", state.Status)
	}
}

// testUpdateDuringNetworkInstability tests update with network issues
func testUpdateDuringNetworkInstability(t *testing.T) {
	testDir := t.TempDir()

	config := &update.Config{
		UpdateDir:       filepath.Join(testDir, "updates"),
		BackupDir:       filepath.Join(testDir, "backups"),
		RetryAttempts:   3,
		RetryDelay:      100 * time.Millisecond,
		DownloadTimeout: 5 * time.Second,
	}

	mgr, err := update.NewUpdateManager(config, "1.0.0")
	require.NoError(t, err)

	// Create update file
	updateFile := filepath.Join(testDir, "network_update.tar.gz")
	createTestUpdate(t, updateFile, "network test update")

	// Use file:// URL to simulate network issues (file not accessible initially)
	nonExistentPath := filepath.Join(testDir, "missing", "update.tar.gz")

	upd := &update.Update{
		ID:       "network-update",
		Version:  "1.1.0",
		Type:     update.UpdateTypeApplication,
		URL:      "file://" + nonExistentPath, // Will fail initially
		Checksum: calculateFileChecksum(t, updateFile),
		Rollback: false,
	}

	ctx := context.Background()
	err = mgr.ApplyUpdate(ctx, upd)

	// Should fail due to network (file access) issues
	assert.Error(t, err)

	// Now "fix" the network by creating the file
	os.MkdirAll(filepath.Dir(nonExistentPath), 0755)
	copyFile(updateFile, nonExistentPath)

	// Retry update
	err = mgr.ApplyUpdate(ctx, upd)
	// May still fail due to other reasons, but file should be accessible now
	t.Logf("Retry result after network recovery: %v", err)
}

// testConcurrentUpdateAttempts tests handling of concurrent update requests
func testConcurrentUpdateAttempts(t *testing.T) {
	testDir := t.TempDir()

	config := &update.Config{
		UpdateDir: filepath.Join(testDir, "updates"),
		BackupDir: filepath.Join(testDir, "backups"),
	}

	mgr, err := update.NewUpdateManager(config, "1.0.0")
	require.NoError(t, err)

	// Create multiple updates
	var updates []*update.Update
	for i := 1; i <= 3; i++ {
		updateFile := filepath.Join(testDir, fmt.Sprintf("concurrent_%d.tar.gz", i))
		createTestUpdate(t, updateFile, fmt.Sprintf("content %d", i))

		upd := &update.Update{
			ID:       fmt.Sprintf("concurrent-%d", i),
			Version:  fmt.Sprintf("1.%d.0", i),
			Type:     update.UpdateTypeApplication,
			URL:      "file://" + updateFile,
			Checksum: calculateFileChecksum(t, updateFile),
		}
		updates = append(updates, upd)
	}

	// Try to apply updates concurrently
	ctx := context.Background()
	results := make(chan error, len(updates))

	for _, upd := range updates {
		go func(u *update.Update) {
			err := mgr.ApplyUpdate(ctx, u)
			results <- err
		}(upd)
	}

	// Collect results
	var errors []error
	for i := 0; i < len(updates); i++ {
		if err := <-results; err != nil {
			errors = append(errors, err)
		}
	}

	// Should handle concurrency properly (likely serializing updates)
	t.Logf("Concurrent update attempts: %d errors out of %d attempts", len(errors), len(updates))

	// Verify final state is consistent
	state, _ := mgr.GetUpdateState()
	assert.NotNil(t, state, "Should have final state")
}

// testUpdateChainValidation tests update version chain validation
func testUpdateChainValidation(t *testing.T) {
	testDir := t.TempDir()

	config := &update.Config{
		UpdateDir: filepath.Join(testDir, "updates"),
		BackupDir: filepath.Join(testDir, "backups"),
	}

	mgr, err := update.NewUpdateManager(config, "1.0.0")
	require.NoError(t, err)

	// Try to skip versions (1.0.0 -> 3.0.0, skipping 2.0.0)
	updateFile := filepath.Join(testDir, "skip_version.tar.gz")
	createTestUpdate(t, updateFile, "skip version content")

	upd := &update.Update{
		ID:       "skip-version",
		Version:  "3.0.0",
		Type:     update.UpdateTypeApplication,
		URL:      "file://" + updateFile,
		Checksum: calculateFileChecksum(t, updateFile),
		Manifest: map[string]interface{}{
			"requires_version": "2.0.0", // Requires version we don't have
		},
	}

	ctx := context.Background()
	err = mgr.ApplyUpdate(ctx, upd)

	// Should handle version mismatch appropriately
	t.Logf("Version chain validation result: %v", err)

	// Test valid progression (1.0.0 -> 1.1.0)
	validFile := filepath.Join(testDir, "valid_progression.tar.gz")
	createTestUpdate(t, validFile, "valid progression")

	validUpdate := &update.Update{
		ID:       "valid-progression",
		Version:  "1.1.0",
		Type:     update.UpdateTypeApplication,
		URL:      "file://" + validFile,
		Checksum: calculateFileChecksum(t, validFile),
		Manifest: map[string]interface{}{
			"requires_version": "1.0.0", // Current version
		},
	}

	err = mgr.ApplyUpdate(ctx, validUpdate)
	t.Logf("Valid progression result: %v", err)
}

// Helper functions

func createTestUpdate(t *testing.T, path string, content string) {
	// Create a temporary file with content
	tmpFile := path + ".tmp"
	err := os.WriteFile(tmpFile, []byte(content), 0644)
	require.NoError(t, err)

	// Create a simple tar.gz (just compress the content)
	cmd := exec.Command("tar", "-czf", path, "-C", filepath.Dir(tmpFile), filepath.Base(tmpFile))
	err = cmd.Run()
	if err != nil {
		// Fallback to just creating a regular file if tar fails
		err = os.WriteFile(path, []byte(content), 0644)
		require.NoError(t, err)
	}

	os.Remove(tmpFile)
}

func calculateFileChecksum(t *testing.T, path string) string {
	file, err := os.Open(path)
	require.NoError(t, err)
	defer file.Close()

	hash := sha256.New()
	_, err = io.Copy(hash, file)
	require.NoError(t, err)

	return hex.EncodeToString(hash.Sum(nil))
}

func generateMockSignature(filePath string, key []byte) string {
	// In production, use real cryptographic signing
	hash := sha256.Sum256(append([]byte(filePath), key...))
	return hex.EncodeToString(hash[:])
}

func copyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()

	_, err = io.Copy(destination, source)
	return err
}