package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// UpdateInfo contains metadata about an update
type UpdateInfo struct {
	Version     string            `json:"version"`
	SHA256      string            `json:"sha256"`
	ReleaseDate time.Time         `json:"releaseDate"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// Updater handles the self-update process
type Updater struct {
	execPath    string
	backupPath  string
	stagingPath string
}

// New creates a new Updater instance
func New(basePath string) (*Updater, error) {
	execPath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("failed to get executable path: %w", err)
	}

	// Create update directories
	if err := os.MkdirAll(basePath, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create update directory: %w", err)
	}

	return &Updater{
		execPath:    execPath,
		backupPath:  filepath.Join(basePath, "backup"),
		stagingPath: filepath.Join(basePath, "staging"),
	}, nil
}

// Update performs the self-update process
func (u *Updater) Update(ctx context.Context, binary io.Reader, info UpdateInfo) error {
	// Verify we can write to all necessary paths
	if err := u.verifyWriteAccess(); err != nil {
		return fmt.Errorf("update path verification failed: %w", err)
	}

	// Create staging file
	staging, err := os.OpenFile(u.stagingPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return fmt.Errorf("failed to create staging file: %w", err)
	}
	defer staging.Close()

	// Calculate checksum while copying
	hash := sha256.New()
	writer := io.MultiWriter(staging, hash)

	if _, err := io.Copy(writer, binary); err != nil {
		return fmt.Errorf("failed to write update: %w", err)
	}

	// Verify checksum
	sum := hex.EncodeToString(hash.Sum(nil))
	if sum != info.SHA256 {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", info.SHA256, sum)
	}

	// Backup current executable
	if err := u.backup(); err != nil {
		return fmt.Errorf("backup failed: %w", err)
	}

	// Replace executable with update
	if err := u.replace(); err != nil {
		// Attempt rollback on failure
		if rbErr := u.rollback(); rbErr != nil {
			return fmt.Errorf("update failed and rollback failed: %v (rollback: %v)", err, rbErr)
		}
		return fmt.Errorf("update failed (rolled back): %w", err)
	}

	return nil
}

func (u *Updater) verifyWriteAccess() error {
	paths := []string{u.stagingPath, u.backupPath}

	for _, path := range paths {
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("cannot create directory %s: %w", dir, err)
		}
	}

	return nil
}

func (u *Updater) backup() error {
	if err := os.Rename(u.execPath, u.backupPath); err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}
	return nil
}

func (u *Updater) replace() error {
	if runtime.GOOS == "windows" {
		// Windows requires special handling due to file locking
		if err := os.Rename(u.stagingPath, u.execPath+".new"); err != nil {
			return fmt.Errorf("failed to stage update: %w", err)
		}
		if err := os.Rename(u.execPath+".new", u.execPath); err != nil {
			return fmt.Errorf("failed to replace executable: %w", err)
		}
	} else {
		if err := os.Rename(u.stagingPath, u.execPath); err != nil {
			return fmt.Errorf("failed to replace executable: %w", err)
		}
	}
	return nil
}

func (u *Updater) rollback() error {
	if err := os.Rename(u.backupPath, u.execPath); err != nil {
		return fmt.Errorf("failed to restore backup: %w", err)
	}
	return nil
}

// Cleanup removes temporary update files
func (u *Updater) Cleanup() error {
	for _, path := range []string{u.stagingPath, u.backupPath} {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to cleanup %s: %w", path, err)
		}
	}
	return nil
}
