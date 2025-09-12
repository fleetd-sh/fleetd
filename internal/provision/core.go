package provision

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// CoreProvisioner focuses only on fleetd agent provisioning
type CoreProvisioner struct {
	config  *Config
	plugins *PluginManager
}

// NewCoreProvisioner creates a provisioner focused on fleetd
func NewCoreProvisioner(config *Config) *CoreProvisioner {
	return &CoreProvisioner{
		config:  config,
		plugins: NewPluginManager(),
	}
}

// LoadPlugins loads plugins from a directory
func (p *CoreProvisioner) LoadPlugins(dir string) error {
	return p.plugins.LoadPluginsFromDir(dir)
}

// RegisterHook registers a provisioning hook
func (p *CoreProvisioner) RegisterHook(hook Hook) {
	p.plugins.RegisterHook(hook)
}

// GetCoreFiles returns the essential files for fleetd provisioning
func (p *CoreProvisioner) GetCoreFiles() map[string][]byte {
	files := make(map[string][]byte)

	// Core fleetd configuration
	fleetdConfig := p.generateFleetdConfig()
	files["/boot/fleetd/config.yaml"] = []byte(fleetdConfig)

	// Fleetd systemd service
	serviceFile := p.generateSystemdService()
	files["/boot/fleetd/fleetd.service"] = []byte(serviceFile)

	// Network configuration (if WiFi)
	if p.config.Network.WiFiSSID != "" {
		wifiConfig := p.generateWiFiConfig()
		files["/boot/wifi-config.txt"] = []byte(wifiConfig)
	}

	// SSH key (if enabled)
	if p.config.Security.EnableSSH && p.config.Security.SSHKey != "" {
		files["/boot/ssh/authorized_keys"] = []byte(p.config.Security.SSHKey)
	}

	// Simple startup script
	startupScript := p.generateStartupScript()
	files["/boot/fleetd-setup.sh"] = []byte(startupScript)

	return files
}

// Provision performs the actual provisioning
func (p *CoreProvisioner) Provision(ctx context.Context) error {
	// Pre-provision hooks
	if err := p.plugins.PreProvision(ctx, p.config); err != nil {
		return fmt.Errorf("pre-provision failed: %w", err)
	}

	// Let plugins modify config
	if err := p.plugins.ModifyConfig(p.config); err != nil {
		return fmt.Errorf("config modification failed: %w", err)
	}

	// Get all files to write
	files := p.GetCoreFiles()

	// Get plugin files
	pluginFiles, err := p.plugins.GetAdditionalFiles()
	if err != nil {
		return fmt.Errorf("failed to get plugin files: %w", err)
	}

	// Merge files
	for path, content := range pluginFiles {
		files[path] = content
	}

	// Here we would actually write to the device
	// For now, this is a placeholder
	fmt.Printf("Would write %d files to device\n", len(files))

	// Post-provision hooks
	if err := p.plugins.PostProvision(ctx, p.config); err != nil {
		return fmt.Errorf("post-provision failed: %w", err)
	}

	return nil
}

// ProvisionWithImage downloads an OS image and writes it to the device
func (p *CoreProvisioner) ProvisionWithImage(ctx context.Context, osType, arch string, dryRun bool, progress ProgressReporter) error {
	// Pre-provision hooks
	if err := p.plugins.PreProvision(ctx, p.config); err != nil {
		return fmt.Errorf("pre-provision failed: %w", err)
	}

	// Let plugins modify config
	if err := p.plugins.ModifyConfig(p.config); err != nil {
		return fmt.Errorf("config modification failed: %w", err)
	}

	// Initialize image manager
	imageManager := NewImageManager("")
	InitializeProviders(imageManager)

	// Download OS image
	if progress != nil {
		progress.UpdateStatus("Downloading OS image...")
	}
	
	imagePath, err := imageManager.DownloadImage(ctx, osType, arch, func(downloaded, total int64) {
		if progress != nil {
			percent := float64(downloaded) / float64(total) * 100
			progress.UpdateProgress(fmt.Sprintf("Downloading: %.1f%%", percent), downloaded, total)
		}
	})
	if err != nil {
		return fmt.Errorf("failed to download image: %w", err)
	}

	// Get the image provider
	provider, err := imageManager.GetProvider(osType)
	if err != nil {
		return err
	}

	// Create SD card writer
	writer := NewSDCardWriter(p.config.DevicePath, dryRun)

	// Write image to SD card
	if progress != nil {
		progress.UpdateStatus("Writing image to SD card...")
	}
	
	if err := writer.WriteImage(ctx, imagePath, func(written, total int64) {
		if progress != nil {
			percent := float64(written) / float64(total) * 100
			progress.UpdateProgress(fmt.Sprintf("Writing: %.1f%%", percent), written, total)
		}
	}); err != nil {
		return fmt.Errorf("failed to write image: %w", err)
	}

	// Mount partitions
	if progress != nil {
		progress.UpdateStatus("Mounting partitions...")
	}
	
	bootPath, rootPath, cleanup, err := writer.MountPartitions(
		provider.GetBootPartitionLabel(),
		provider.GetRootPartitionLabel(),
	)
	if err != nil {
		return fmt.Errorf("failed to mount partitions: %w", err)
	}
	defer cleanup()

	// Perform OS-specific setup
	if progress != nil {
		progress.UpdateStatus("Configuring system...")
	}
	
	if err := provider.PostWriteSetup(bootPath, rootPath, p.config); err != nil {
		return fmt.Errorf("failed to perform post-write setup: %w", err)
	}

	// Write additional plugin files
	pluginFiles, err := p.plugins.GetAdditionalFiles()
	if err != nil {
		return fmt.Errorf("failed to get plugin files: %w", err)
	}

	for path, content := range pluginFiles {
		fullPath := filepath.Join(bootPath, path)
		if err := os.WriteFile(fullPath, content, 0644); err != nil {
			return fmt.Errorf("failed to write plugin file %s: %w", path, err)
		}
	}

	// Post-provision hooks
	if err := p.plugins.PostProvision(ctx, p.config); err != nil {
		return fmt.Errorf("post-provision failed: %w", err)
	}

	if progress != nil {
		progress.UpdateStatus("Provisioning complete!")
	}

	return nil
}

func (p *CoreProvisioner) generateFleetdConfig() string {
	config := fmt.Sprintf(`# FleetD Agent Configuration
agent:
  id: %s
  name: %s
  type: %s

discovery:
  enabled: true
  mdns:
    enabled: true
    service: "_fleetd._tcp"
    port: 8080
    txt:
      - "device_id=%s"
      - "device_type=%s"

telemetry:
  enabled: true
  interval: 60s
`, p.config.DeviceID, p.config.DeviceName, p.config.DeviceType,
		p.config.DeviceID, p.config.DeviceType)

	// Add fleet server if configured
	if p.config.Fleet.ServerURL != "" {
		config += fmt.Sprintf(`
server:
  url: %s
  token: %s
`, p.config.Fleet.ServerURL, p.config.Fleet.Token)
	} else {
		config += `
# Server will be configured via mDNS discovery
server:
  discovery: mdns
`
	}

	return config
}

func (p *CoreProvisioner) generateSystemdService() string {
	return fmt.Sprintf(`[Unit]
Description=FleetD Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=notify
ExecStart=/usr/local/bin/fleetd agent --config /etc/fleetd/config.yaml
Restart=always
RestartSec=10
Environment="FLEETD_DEVICE_ID=%s"

[Install]
WantedBy=multi-user.target`, p.config.DeviceID)
}

func (p *CoreProvisioner) generateWiFiConfig() string {
	return fmt.Sprintf(`# WiFi Configuration
SSID=%s
PASSWORD=%s
`, p.config.Network.WiFiSSID, p.config.Network.WiFiPass)
}

func (p *CoreProvisioner) generateStartupScript() string {
	return `#!/bin/bash
# FleetD Setup Script

set -e

echo "Setting up FleetD agent..."

# Create directories
mkdir -p /etc/fleetd
mkdir -p /var/lib/fleetd
mkdir -p /var/log/fleetd

# Copy configuration
cp /boot/fleetd/config.yaml /etc/fleetd/config.yaml

# Install service
cp /boot/fleetd/fleetd.service /etc/systemd/system/
systemctl daemon-reload
systemctl enable fleetd
systemctl start fleetd

echo "FleetD agent setup complete!"
`
}
