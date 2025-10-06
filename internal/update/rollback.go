package update

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// RollbackManager handles system rollbacks
type RollbackManager struct {
	backupDir  string
	maxBackups int
}

// Backup represents a system backup
type Backup struct {
	ID        string                 `json:"id"`
	Version   string                 `json:"version"`
	CreatedAt time.Time              `json:"created_at"`
	Size      int64                  `json:"size"`
	Type      string                 `json:"type"`
	Files     []string               `json:"files"`
	Metadata  map[string]interface{} `json:"metadata"`
}

// NewRollbackManager creates a new rollback manager
func NewRollbackManager(backupDir string, maxBackups int) *RollbackManager {
	return &RollbackManager{
		backupDir:  backupDir,
		maxBackups: maxBackups,
	}
}

// CreateBackup creates a backup of the current system
func (rm *RollbackManager) CreateBackup(version string) (*Backup, error) {
	backupID := fmt.Sprintf("backup_%s_%d", version, time.Now().Unix())
	backupPath := filepath.Join(rm.backupDir, backupID)

	// Create backup directory
	if err := os.MkdirAll(backupPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create backup directory: %w", err)
	}

	backup := &Backup{
		ID:        backupID,
		Version:   version,
		CreatedAt: time.Now(),
		Type:      "full",
		Files:     []string{},
		Metadata:  make(map[string]interface{}),
	}

	// Backup critical files and directories
	itemsToBackup := rm.getBackupItems()

	for _, item := range itemsToBackup {
		if err := rm.backupItem(item, backupPath); err != nil {
			log.Printf("Failed to backup %s: %v", item, err)
			continue
		}
		backup.Files = append(backup.Files, item)
	}

	// Save backup metadata
	metadataFile := filepath.Join(backupPath, "backup.json")
	data, err := json.MarshalIndent(backup, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal backup metadata: %w", err)
	}

	if err := os.WriteFile(metadataFile, data, 0644); err != nil {
		return nil, fmt.Errorf("failed to save backup metadata: %w", err)
	}

	// Calculate backup size
	backup.Size = rm.calculateBackupSize(backupPath)

	log.Printf("Created backup %s (size: %d bytes, files: %d)", backup.ID, backup.Size, len(backup.Files))
	return backup, nil
}

// Rollback performs a system rollback to the most recent backup
func (rm *RollbackManager) Rollback(ctx context.Context) error {
	// Get most recent backup
	backup, err := rm.GetLatestBackup()
	if err != nil {
		return fmt.Errorf("failed to get latest backup: %w", err)
	}

	if backup == nil {
		return fmt.Errorf("no backup available for rollback")
	}

	log.Printf("Starting rollback to backup %s (version %s)", backup.ID, backup.Version)
	return rm.RollbackToBackup(ctx, backup.ID)
}

// RollbackToBackup rolls back to a specific backup
func (rm *RollbackManager) RollbackToBackup(ctx context.Context, backupID string) error {
	backupPath := filepath.Join(rm.backupDir, backupID)

	// Load backup metadata
	metadataFile := filepath.Join(backupPath, "backup.json")
	data, err := os.ReadFile(metadataFile)
	if err != nil {
		return fmt.Errorf("failed to read backup metadata: %w", err)
	}

	var backup Backup
	if err := json.Unmarshal(data, &backup); err != nil {
		return fmt.Errorf("failed to parse backup metadata: %w", err)
	}

	// Stop services before rollback
	if err := rm.stopServices(ctx); err != nil {
		log.Printf("Warning: failed to stop services: %v", err)
	}

	// Restore files
	for _, file := range backup.Files {
		backupFile := filepath.Join(backupPath, strings.ReplaceAll(file, "/", "_"))
		if err := rm.restoreFile(backupFile, file); err != nil {
			log.Printf("Failed to restore %s: %v", file, err)
			// Continue with other files
		}
	}

	// Run post-rollback script if exists
	postRollbackScript := filepath.Join(backupPath, "post_rollback.sh")
	if _, err := os.Stat(postRollbackScript); err == nil {
		cmd := exec.CommandContext(ctx, "/bin/sh", postRollbackScript)
		if output, err := cmd.CombinedOutput(); err != nil {
			log.Printf("Post-rollback script failed: %v\nOutput: %s", err, string(output))
		}
	}

	// Start services after rollback
	if err := rm.startServices(ctx); err != nil {
		log.Printf("Warning: failed to start services: %v", err)
	}

	log.Printf("Rollback completed successfully to version %s", backup.Version)
	return nil
}

// GetLatestBackup returns the most recent backup
func (rm *RollbackManager) GetLatestBackup() (*Backup, error) {
	backups, err := rm.ListBackups()
	if err != nil {
		return nil, err
	}

	if len(backups) == 0 {
		return nil, nil
	}

	// Sort by creation time (newest first)
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].CreatedAt.After(backups[j].CreatedAt)
	})

	return &backups[0], nil
}

// ListBackups lists all available backups
func (rm *RollbackManager) ListBackups() ([]Backup, error) {
	entries, err := os.ReadDir(rm.backupDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Backup{}, nil
		}
		return nil, err
	}

	var backups []Backup
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		metadataFile := filepath.Join(rm.backupDir, entry.Name(), "backup.json")
		data, err := os.ReadFile(metadataFile)
		if err != nil {
			continue
		}

		var backup Backup
		if err := json.Unmarshal(data, &backup); err != nil {
			continue
		}

		backups = append(backups, backup)
	}

	return backups, nil
}

// CleanupOldBackups removes old backups exceeding maxBackups limit
func (rm *RollbackManager) CleanupOldBackups() error {
	backups, err := rm.ListBackups()
	if err != nil {
		return err
	}

	if len(backups) <= rm.maxBackups {
		return nil
	}

	// Sort by creation time (oldest first)
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].CreatedAt.Before(backups[j].CreatedAt)
	})

	// Remove oldest backups
	toRemove := len(backups) - rm.maxBackups
	for i := 0; i < toRemove; i++ {
		backupPath := filepath.Join(rm.backupDir, backups[i].ID)
		if err := os.RemoveAll(backupPath); err != nil {
			log.Printf("Failed to remove old backup %s: %v", backups[i].ID, err)
		} else {
			log.Printf("Removed old backup %s", backups[i].ID)
		}
	}

	return nil
}

// getBackupItems returns the list of items to backup
func (rm *RollbackManager) getBackupItems() []string {
	items := []string{
		"/usr/local/bin/fleetd-agent", // Agent binary
		"/etc/fleetd/agent.yaml",      // Configuration
		"/var/lib/fleetd/state.db",    // State database
	}

	// Add platform-specific items
	switch runtime.GOOS {
	case "linux":
		items = append(items,
			"/etc/systemd/system/fleetd.service", // Service file
		)
	}

	// Filter out non-existent items
	var existingItems []string
	for _, item := range items {
		if _, err := os.Stat(item); err == nil {
			existingItems = append(existingItems, item)
		}
	}

	return existingItems
}

// backupItem backs up a file or directory
func (rm *RollbackManager) backupItem(item, backupPath string) error {
	info, err := os.Stat(item)
	if err != nil {
		return err
	}

	destName := strings.ReplaceAll(item, "/", "_")
	destPath := filepath.Join(backupPath, destName)

	if info.IsDir() {
		// Create tar archive for directories
		cmd := exec.Command("tar", "-czf", destPath+".tar.gz", "-C", filepath.Dir(item), filepath.Base(item))
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to archive directory: %w\nOutput: %s", err, string(output))
		}
	} else {
		// Copy single file
		return copyFile(item, destPath)
	}

	return nil
}

// restoreFile restores a backed up file
func (rm *RollbackManager) restoreFile(backupFile, originalPath string) error {
	// Check if it's an archive
	if strings.HasSuffix(backupFile, ".tar.gz") {
		// Extract archive
		dir := filepath.Dir(originalPath)
		cmd := exec.Command("tar", "-xzf", backupFile, "-C", dir)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to extract archive: %w\nOutput: %s", err, string(output))
		}
	} else {
		// Copy single file
		// Create backup of current file
		if _, err := os.Stat(originalPath); err == nil {
			os.Rename(originalPath, originalPath+".rollback")
		}

		if err := copyFile(backupFile, originalPath); err != nil {
			// Restore original if copy fails
			os.Rename(originalPath+".rollback", originalPath)
			return err
		}

		// Remove rollback file on success
		os.Remove(originalPath + ".rollback")
	}

	return nil
}

// calculateBackupSize calculates the total size of a backup
func (rm *RollbackManager) calculateBackupSize(backupPath string) int64 {
	var size int64
	filepath.Walk(backupPath, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size
}

// stopServices stops critical services before rollback
func (rm *RollbackManager) stopServices(ctx context.Context) error {
	if runtime.GOOS != "linux" {
		return nil
	}

	services := []string{"fleetd"}
	for _, service := range services {
		cmd := exec.CommandContext(ctx, "systemctl", "stop", service)
		if err := cmd.Run(); err != nil {
			log.Printf("Failed to stop service %s: %v", service, err)
		}
	}

	// Wait for services to stop
	time.Sleep(2 * time.Second)
	return nil
}

// startServices starts critical services after rollback
func (rm *RollbackManager) startServices(ctx context.Context) error {
	if runtime.GOOS != "linux" {
		return nil
	}

	services := []string{"fleetd"}
	for _, service := range services {
		cmd := exec.CommandContext(ctx, "systemctl", "start", service)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to start service %s: %w", service, err)
		}
	}

	return nil
}

// ValidateBackup validates a backup's integrity
func (rm *RollbackManager) ValidateBackup(backupID string) error {
	backupPath := filepath.Join(rm.backupDir, backupID)

	// Check if backup directory exists
	if _, err := os.Stat(backupPath); err != nil {
		return fmt.Errorf("backup directory not found: %w", err)
	}

	// Load and validate metadata
	metadataFile := filepath.Join(backupPath, "backup.json")
	data, err := os.ReadFile(metadataFile)
	if err != nil {
		return fmt.Errorf("failed to read backup metadata: %w", err)
	}

	var backup Backup
	if err := json.Unmarshal(data, &backup); err != nil {
		return fmt.Errorf("invalid backup metadata: %w", err)
	}

	// Verify all backup files exist
	for _, file := range backup.Files {
		backupFile := filepath.Join(backupPath, strings.ReplaceAll(file, "/", "_"))
		// Check for both regular file and archive
		if _, err := os.Stat(backupFile); err != nil {
			if _, err := os.Stat(backupFile + ".tar.gz"); err != nil {
				return fmt.Errorf("backup file missing: %s", file)
			}
		}
	}

	return nil
}

// GetBackupSize returns the size of a specific backup
func (rm *RollbackManager) GetBackupSize(backupID string) (int64, error) {
	backupPath := filepath.Join(rm.backupDir, backupID)
	return rm.calculateBackupSize(backupPath), nil
}
