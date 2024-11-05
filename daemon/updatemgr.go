package daemon

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"fleetd.sh/pkg/updateclient"
	"golang.org/x/mod/semver"
)

type UpdateManager struct {
	client      *updateclient.Client
	currentVer  string
	deviceType  string
	execPath    string // Path to current executable
	lastChecked time.Time
}

func NewUpdateManager(fleetURL, currentVersion string) (*UpdateManager, error) {
	execPath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("failed to get executable path: %w", err)
	}

	return &UpdateManager{
		client:     updateclient.NewClient(fleetURL),
		currentVer: currentVersion,
		deviceType: "FLEETD",
		execPath:   execPath,
	}, nil
}

func (um *UpdateManager) CheckForUpdates(ctx context.Context) (*updateclient.Package, error) {
	updates, err := um.client.GetAvailableUpdates(
		ctx,
		um.deviceType,
		um.lastChecked,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to check for updates: %w", err)
	}

	um.lastChecked = time.Now()

	// Find latest applicable update
	var latest *updateclient.Package
	for _, update := range updates {
		if semver.Compare(update.Version, um.currentVer) > 0 {
			if latest == nil || semver.Compare(update.Version, latest.Version) > 0 {
				latest = update
			}
		}
	}

	return latest, nil
}

func (um *UpdateManager) ApplyUpdate(ctx context.Context, pkg *updateclient.Package) error {
	// Download and replace binary like before
	if err := um.downloadAndReplaceBinary(ctx, pkg); err != nil {
		return err
	}

	// Signal systemd to restart the service
	if err := um.restartService(); err != nil {
		return fmt.Errorf("failed to restart service: %w", err)
	}

	return nil
}

func (um *UpdateManager) downloadAndReplaceBinary(ctx context.Context, pkg *updateclient.Package) error {
	// Download update to temporary file
	resp, err := http.Get(pkg.FileURL)
	if err != nil {
		return fmt.Errorf("failed to download update: %w", err)
	}
	defer resp.Body.Close()

	tempDir := filepath.Dir(um.execPath)
	tempFile, err := os.CreateTemp(tempDir, "fleetd.*.update")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tempFile.Name())

	if _, err := io.Copy(tempFile, resp.Body); err != nil {
		return fmt.Errorf("failed to write update: %w", err)
	}
	tempFile.Close()

	// Make temporary file executable
	if err := os.Chmod(tempFile.Name(), 0755); err != nil {
		return fmt.Errorf("failed to make update executable: %w", err)
	}

	// Rename current binary to .old for backup
	oldPath := um.execPath + ".old"
	if err := os.Rename(um.execPath, oldPath); err != nil {
		return fmt.Errorf("failed to backup current binary: %w", err)
	}

	// Move new binary into place
	if err := os.Rename(tempFile.Name(), um.execPath); err != nil {
		// Try to restore old binary on failure
		os.Rename(oldPath, um.execPath)
		return fmt.Errorf("failed to install update: %w", err)
	}

	// Success - remove old binary
	os.Remove(oldPath)

	return nil
}

func (um *UpdateManager) restartService() error {
	if os.Getenv("INVOCATION_ID") == "" {
		return fmt.Errorf("not running under systemd")
	}

	cmd := exec.Command("systemctl", "restart", "fleetd.service")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to restart service: %w", err)
	}

	return nil
}
