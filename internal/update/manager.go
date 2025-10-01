package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// UpdateManager handles OTA updates with rollback support
type UpdateManager struct {
	config          *Config
	client          *http.Client
	currentVersion  string
	updateLock      sync.Mutex
	rollbackManager *RollbackManager
	healthChecker   *HealthChecker
	stateFile       string
}

// Config holds update manager configuration
type Config struct {
	UpdateDir       string
	BackupDir       string
	MaxBackups      int
	DownloadTimeout time.Duration
	ApplyTimeout    time.Duration
	HealthTimeout   time.Duration
	RetryAttempts   int
	RetryDelay      time.Duration
	SignatureKey    []byte
}

// Update represents an OTA update
type Update struct {
	ID          string                 `json:"id"`
	Version     string                 `json:"version"`
	ReleaseDate time.Time              `json:"release_date"`
	Type        UpdateType             `json:"type"`
	Priority    UpdatePriority         `json:"priority"`
	URL         string                 `json:"url"`
	Size        int64                  `json:"size"`
	Checksum    string                 `json:"checksum"`
	Signature   string                 `json:"signature"`
	Changelog   string                 `json:"changelog"`
	PreScript   string                 `json:"pre_script,omitempty"`
	PostScript  string                 `json:"post_script,omitempty"`
	Manifest    map[string]interface{} `json:"manifest"`
	Rollback    bool                   `json:"rollback_enabled"`
}

// UpdateType defines the type of update
type UpdateType string

const (
	UpdateTypeApplication UpdateType = "application"
	UpdateTypeFirmware    UpdateType = "firmware"
	UpdateTypeConfig      UpdateType = "config"
	UpdateTypeSystem      UpdateType = "system"
)

// UpdatePriority defines update priority
type UpdatePriority string

const (
	UpdatePriorityCritical UpdatePriority = "critical"
	UpdatePriorityHigh     UpdatePriority = "high"
	UpdatePriorityNormal   UpdatePriority = "normal"
	UpdatePriorityLow      UpdatePriority = "low"
)

// UpdateState tracks the state of an update
type UpdateState struct {
	UpdateID      string    `json:"update_id"`
	Version       string    `json:"version"`
	Status        string    `json:"status"`
	Progress      int       `json:"progress"`
	StartedAt     time.Time `json:"started_at"`
	CompletedAt   time.Time `json:"completed_at,omitempty"`
	Error         string    `json:"error,omitempty"`
	RollbackCount int       `json:"rollback_count"`
}

// NewUpdateManager creates a new update manager
func NewUpdateManager(config *Config, currentVersion string) (*UpdateManager, error) {
	if config.UpdateDir == "" {
		config.UpdateDir = "/var/lib/fleetd/updates"
	}
	if config.BackupDir == "" {
		config.BackupDir = "/var/lib/fleetd/backups"
	}
	if config.MaxBackups == 0 {
		config.MaxBackups = 3
	}
	if config.DownloadTimeout == 0 {
		config.DownloadTimeout = 30 * time.Minute
	}
	if config.ApplyTimeout == 0 {
		config.ApplyTimeout = 10 * time.Minute
	}
	if config.HealthTimeout == 0 {
		config.HealthTimeout = 5 * time.Minute
	}

	// Create directories
	dirs := []string{config.UpdateDir, config.BackupDir}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	rollbackMgr := NewRollbackManager(config.BackupDir, config.MaxBackups)
	healthChecker := NewHealthChecker()

	return &UpdateManager{
		config:          config,
		client:          &http.Client{Timeout: config.DownloadTimeout},
		currentVersion:  currentVersion,
		rollbackManager: rollbackMgr,
		healthChecker:   healthChecker,
		stateFile:       filepath.Join(config.UpdateDir, "update.state"),
	}, nil
}

// ApplyUpdate downloads and applies an update
func (um *UpdateManager) ApplyUpdate(ctx context.Context, update *Update) error {
	um.updateLock.Lock()
	defer um.updateLock.Unlock()

	state := &UpdateState{
		UpdateID:  update.ID,
		Version:   update.Version,
		Status:    "downloading",
		StartedAt: time.Now(),
	}

	// Save initial state
	if err := um.saveState(state); err != nil {
		log.Printf("Failed to save update state: %v", err)
	}

	// Create backup before update
	if update.Rollback {
		log.Printf("Creating backup before update to version %s", update.Version)
		backup, err := um.rollbackManager.CreateBackup(um.currentVersion)
		if err != nil {
			state.Status = "failed"
			state.Error = fmt.Sprintf("backup failed: %v", err)
			um.saveState(state)
			return fmt.Errorf("failed to create backup: %w", err)
		}
		log.Printf("Backup created: %s", backup.ID)
	}

	// Download update
	log.Printf("Downloading update %s (version %s)", update.ID, update.Version)
	state.Status = "downloading"
	state.Progress = 10
	um.saveState(state)

	updatePath, err := um.downloadUpdate(ctx, update)
	if err != nil {
		state.Status = "failed"
		state.Error = fmt.Sprintf("download failed: %v", err)
		um.saveState(state)
		return fmt.Errorf("failed to download update: %w", err)
	}

	// Verify update
	log.Printf("Verifying update integrity")
	state.Status = "verifying"
	state.Progress = 30
	um.saveState(state)

	if err := um.verifyUpdate(updatePath, update); err != nil {
		state.Status = "failed"
		state.Error = fmt.Sprintf("verification failed: %v", err)
		um.saveState(state)
		os.Remove(updatePath)
		return fmt.Errorf("update verification failed: %w", err)
	}

	// Run pre-update script if provided
	if update.PreScript != "" {
		log.Printf("Running pre-update script")
		state.Status = "preparing"
		state.Progress = 40
		um.saveState(state)

		if err := um.runScript(ctx, update.PreScript, "pre-update"); err != nil {
			state.Status = "failed"
			state.Error = fmt.Sprintf("pre-script failed: %v", err)
			um.saveState(state)
			return fmt.Errorf("pre-update script failed: %w", err)
		}
	}

	// Apply update based on type
	log.Printf("Applying %s update", update.Type)
	state.Status = "applying"
	state.Progress = 50
	um.saveState(state)

	err = um.applyUpdateByType(ctx, updatePath, update)
	if err != nil {
		log.Printf("Update application failed: %v", err)
		state.Status = "rolling_back"
		state.Error = fmt.Sprintf("apply failed: %v", err)
		um.saveState(state)

		// Attempt rollback if enabled
		if update.Rollback {
			log.Printf("Attempting rollback to previous version")
			if rollbackErr := um.rollbackManager.Rollback(ctx); rollbackErr != nil {
				state.Status = "failed"
				state.Error = fmt.Sprintf("apply failed and rollback failed: %v", rollbackErr)
				um.saveState(state)
				return fmt.Errorf("update failed and rollback failed: %w", rollbackErr)
			}
			state.Status = "rolled_back"
			state.RollbackCount++
			um.saveState(state)
			return fmt.Errorf("update failed, successfully rolled back: %w", err)
		}

		state.Status = "failed"
		um.saveState(state)
		return err
	}

	// Run post-update script if provided
	if update.PostScript != "" {
		log.Printf("Running post-update script")
		state.Status = "configuring"
		state.Progress = 70
		um.saveState(state)

		if err := um.runScript(ctx, update.PostScript, "post-update"); err != nil {
			log.Printf("Post-update script failed: %v", err)
			// Non-fatal, continue with health check
		}
	}

	// Perform health check
	log.Printf("Performing post-update health check")
	state.Status = "health_check"
	state.Progress = 80
	um.saveState(state)

	healthCtx, cancel := context.WithTimeout(ctx, um.config.HealthTimeout)
	defer cancel()

	if err := um.healthChecker.CheckHealth(healthCtx); err != nil {
		log.Printf("Health check failed: %v", err)
		state.Status = "unhealthy"
		state.Error = fmt.Sprintf("health check failed: %v", err)
		um.saveState(state)

		// Attempt rollback if health check fails
		if update.Rollback {
			log.Printf("Health check failed, attempting rollback")
			if rollbackErr := um.rollbackManager.Rollback(ctx); rollbackErr != nil {
				state.Status = "failed"
				state.Error = fmt.Sprintf("health check failed and rollback failed: %v", rollbackErr)
				um.saveState(state)
				return fmt.Errorf("health check failed and rollback failed: %w", rollbackErr)
			}
			state.Status = "rolled_back"
			state.RollbackCount++
			um.saveState(state)
			return fmt.Errorf("health check failed, successfully rolled back: %w", err)
		}

		return fmt.Errorf("health check failed: %w", err)
	}

	// Update successful
	log.Printf("Update completed successfully to version %s", update.Version)
	state.Status = "completed"
	state.Progress = 100
	state.CompletedAt = time.Now()
	um.saveState(state)

	// Update current version
	um.currentVersion = update.Version

	// Clean up old backups
	um.rollbackManager.CleanupOldBackups()

	// Clean up update file
	os.Remove(updatePath)

	return nil
}

// downloadUpdate downloads an update package
func (um *UpdateManager) downloadUpdate(ctx context.Context, update *Update) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", update.URL, nil)
	if err != nil {
		return "", err
	}

	resp, err := um.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	// Create temporary file
	tmpFile := filepath.Join(um.config.UpdateDir, fmt.Sprintf("update_%s.tmp", update.ID))
	out, err := os.Create(tmpFile)
	if err != nil {
		return "", err
	}
	defer out.Close()

	// Download with progress tracking
	written, err := io.Copy(out, resp.Body)
	if err != nil {
		os.Remove(tmpFile)
		return "", err
	}

	if update.Size > 0 && written != update.Size {
		os.Remove(tmpFile)
		return "", fmt.Errorf("size mismatch: expected %d, got %d", update.Size, written)
	}

	return tmpFile, nil
}

// verifyUpdate verifies the integrity and authenticity of an update
func (um *UpdateManager) verifyUpdate(filePath string, update *Update) error {
	// Calculate checksum
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return err
	}

	checksum := hex.EncodeToString(hash.Sum(nil))
	if checksum != update.Checksum {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", update.Checksum, checksum)
	}

	// TODO: Verify signature if signature key is configured
	if len(um.config.SignatureKey) > 0 && update.Signature != "" {
		// Implement signature verification
		log.Printf("Signature verification not yet implemented")
	}

	return nil
}

// applyUpdateByType applies an update based on its type
func (um *UpdateManager) applyUpdateByType(ctx context.Context, updatePath string, update *Update) error {
	switch update.Type {
	case UpdateTypeApplication:
		return um.applyApplicationUpdate(ctx, updatePath, update)
	case UpdateTypeFirmware:
		return um.applyFirmwareUpdate(ctx, updatePath, update)
	case UpdateTypeConfig:
		return um.applyConfigUpdate(ctx, updatePath, update)
	case UpdateTypeSystem:
		return um.applySystemUpdate(ctx, updatePath, update)
	default:
		return fmt.Errorf("unsupported update type: %s", update.Type)
	}
}

// applyApplicationUpdate applies an application update
func (um *UpdateManager) applyApplicationUpdate(ctx context.Context, updatePath string, update *Update) error {
	// Extract update package
	extractDir := filepath.Join(um.config.UpdateDir, "extract")
	os.RemoveAll(extractDir)
	defer os.RemoveAll(extractDir)

	if err := os.MkdirAll(extractDir, 0755); err != nil {
		return err
	}

	// Determine extraction method based on file extension
	var cmd *exec.Cmd
	if strings.HasSuffix(updatePath, ".tar.gz") || strings.HasSuffix(updatePath, ".tgz") {
		cmd = exec.CommandContext(ctx, "tar", "-xzf", updatePath, "-C", extractDir)
	} else if strings.HasSuffix(updatePath, ".tar") {
		cmd = exec.CommandContext(ctx, "tar", "-xf", updatePath, "-C", extractDir)
	} else if strings.HasSuffix(updatePath, ".zip") {
		cmd = exec.CommandContext(ctx, "unzip", "-q", updatePath, "-d", extractDir)
	} else {
		// Assume it's a binary update
		return um.applyBinaryUpdate(ctx, updatePath, update)
	}

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("extraction failed: %w\nOutput: %s", err, string(output))
	}

	// Look for update script
	updateScript := filepath.Join(extractDir, "update.sh")
	if _, err := os.Stat(updateScript); err == nil {
		// Run update script
		cmd := exec.CommandContext(ctx, "/bin/sh", updateScript)
		cmd.Dir = extractDir
		cmd.Env = append(os.Environ(),
			fmt.Sprintf("VERSION=%s", update.Version),
			fmt.Sprintf("UPDATE_ID=%s", update.ID),
		)

		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("update script failed: %w\nOutput: %s", err, string(output))
		}
	} else {
		// Manual update: copy files to appropriate locations
		return um.manualApplicationUpdate(extractDir, update)
	}

	return nil
}

// applyBinaryUpdate applies a binary update
func (um *UpdateManager) applyBinaryUpdate(ctx context.Context, updatePath string, update *Update) error {
	// Get current binary path
	currentBinary, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get current binary: %w", err)
	}

	// Create backup of current binary
	backupPath := currentBinary + ".backup"
	if err := copyFile(currentBinary, backupPath); err != nil {
		return fmt.Errorf("failed to backup current binary: %w", err)
	}

	// Replace binary
	if err := copyFile(updatePath, currentBinary); err != nil {
		// Restore backup
		copyFile(backupPath, currentBinary)
		return fmt.Errorf("failed to replace binary: %w", err)
	}

	// Make executable
	if err := os.Chmod(currentBinary, 0755); err != nil {
		// Restore backup
		copyFile(backupPath, currentBinary)
		return fmt.Errorf("failed to set permissions: %w", err)
	}

	// Restart service
	if runtime.GOOS == "linux" {
		cmd := exec.CommandContext(ctx, "systemctl", "restart", "fleetd")
		if output, err := cmd.CombinedOutput(); err != nil {
			// Restore backup
			copyFile(backupPath, currentBinary)
			return fmt.Errorf("failed to restart service: %w\nOutput: %s", err, string(output))
		}
	}

	return nil
}

// applyFirmwareUpdate applies a firmware update
func (um *UpdateManager) applyFirmwareUpdate(ctx context.Context, updatePath string, update *Update) error {
	// Platform-specific firmware update logic
	switch runtime.GOARCH {
	case "arm", "arm64":
		return um.applyARMFirmwareUpdate(ctx, updatePath, update)
	default:
		return fmt.Errorf("firmware updates not supported on %s", runtime.GOARCH)
	}
}

// applyARMFirmwareUpdate applies firmware update on ARM devices
func (um *UpdateManager) applyARMFirmwareUpdate(ctx context.Context, updatePath string, update *Update) error {
	// Check if we're on a Raspberry Pi
	if _, err := os.Stat("/boot/config.txt"); err == nil {
		// Raspberry Pi firmware update
		cmd := exec.CommandContext(ctx, "rpi-update", updatePath)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("firmware update failed: %w\nOutput: %s", err, string(output))
		}

		// Schedule reboot
		log.Printf("Firmware update applied, reboot required")
		// Note: Actual reboot should be handled by the agent based on policy
	}

	return nil
}

// applyConfigUpdate applies a configuration update
func (um *UpdateManager) applyConfigUpdate(ctx context.Context, updatePath string, update *Update) error {
	// Parse configuration update
	data, err := os.ReadFile(updatePath)
	if err != nil {
		return err
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("invalid config format: %w", err)
	}

	// Apply configuration changes
	// This would integrate with your configuration management system
	log.Printf("Applying configuration update: %+v", config)

	return nil
}

// applySystemUpdate applies a system-level update
func (um *UpdateManager) applySystemUpdate(ctx context.Context, updatePath string, update *Update) error {
	// System package update (apt, yum, etc.)
	switch runtime.GOOS {
	case "linux":
		return um.applyLinuxSystemUpdate(ctx, updatePath, update)
	default:
		return fmt.Errorf("system updates not supported on %s", runtime.GOOS)
	}
}

// applyLinuxSystemUpdate applies system updates on Linux
func (um *UpdateManager) applyLinuxSystemUpdate(ctx context.Context, updatePath string, update *Update) error {
	// Detect package manager
	var cmd *exec.Cmd
	if _, err := exec.LookPath("apt-get"); err == nil {
		cmd = exec.CommandContext(ctx, "apt-get", "update")
	} else if _, err := exec.LookPath("yum"); err == nil {
		cmd = exec.CommandContext(ctx, "yum", "update", "-y")
	} else {
		return fmt.Errorf("no supported package manager found")
	}

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("system update failed: %w\nOutput: %s", err, string(output))
	}

	return nil
}

// manualApplicationUpdate manually copies update files
func (um *UpdateManager) manualApplicationUpdate(extractDir string, update *Update) error {
	// Implementation would depend on your application structure
	// This is a placeholder for manual file copying logic
	return fmt.Errorf("manual update not implemented")
}

// runScript executes a script
func (um *UpdateManager) runScript(ctx context.Context, script, phase string) error {
	scriptFile := filepath.Join(um.config.UpdateDir, fmt.Sprintf("%s.sh", phase))
	if err := os.WriteFile(scriptFile, []byte(script), 0755); err != nil {
		return err
	}
	defer os.Remove(scriptFile)

	cmd := exec.CommandContext(ctx, "/bin/sh", scriptFile)
	cmd.Env = os.Environ()

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("script failed: %w\nOutput: %s", err, string(output))
	}

	return nil
}

// saveState saves the current update state
func (um *UpdateManager) saveState(state *UpdateState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(um.stateFile, data, 0644)
}

// GetCurrentVersion returns the current version
func (um *UpdateManager) GetCurrentVersion() string {
	return um.currentVersion
}

// GetUpdateState returns the current update state
func (um *UpdateManager) GetUpdateState() (*UpdateState, error) {
	data, err := os.ReadFile(um.stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var state UpdateState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}

	return &state, nil
}

// copyFile copies a file from src to dst
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