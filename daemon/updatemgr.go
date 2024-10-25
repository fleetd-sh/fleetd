package daemon

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"time"

	"fleetd.sh/internal/version"
)

type UpdateManager struct {
	config *Config
	stopCh chan struct{}
}

type UpdateInfo struct {
	Version     string `json:"version"`
	DownloadURL string `json:"download_url"`
}

func NewUpdateManager(cfg *Config) (*UpdateManager, error) {
	return &UpdateManager{
		config: cfg,
		stopCh: make(chan struct{}),
	}, nil
}

func (um *UpdateManager) Start() {
	interval, _ := time.ParseDuration(um.config.UpdateCheckInterval)
	if interval == 0 {
		interval = time.Hour // Default to 1 hour if parsing fails
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := um.checkAndApplyUpdate(); err != nil {
				slog.With("error", err).Error("Error checking for updates")
			}
		case <-um.stopCh:
			return
		}
	}
}

func (um *UpdateManager) Stop() {
	close(um.stopCh)
}

func (um *UpdateManager) checkAndApplyUpdate() error {
	resp, err := http.Get(fmt.Sprintf("%s/check-update?device_id=%s", um.config.UpdateServerURL, um.config.DeviceID))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var updateInfo UpdateInfo
	if err := json.NewDecoder(resp.Body).Decode(&updateInfo); err != nil {
		return err
	}

	currentVersion := version.GetVersion()

	if updateInfo.Version == currentVersion {
		return nil // No update needed
	}

	// Download update
	if err := um.downloadUpdate(updateInfo.DownloadURL); err != nil {
		return err
	}

	// Apply update
	return um.applyUpdate()
}

func (um *UpdateManager) downloadUpdate(url string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	out, err := os.Create("/tmp/fleet-daemon-update")
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func (um *UpdateManager) applyUpdate() error {
	cmd := exec.Command("/bin/sh", "-c", "systemctl stop fleet-daemon && mv /tmp/fleet-daemon-update /usr/local/bin/fleet-daemon && systemctl start fleet-daemon")
	return cmd.Run()
}
