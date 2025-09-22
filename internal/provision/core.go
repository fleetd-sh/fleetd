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

	// Get decompressed image (from cache if available)
	if progress != nil {
		progress.UpdateStatus("Preparing image...")
	}

	decompressedPath, err := imageManager.GetDecompressedImage(ctx, imagePath, func(status string) {
		if progress != nil {
			progress.UpdateStatus(status)
		}
	})
	if err != nil {
		return fmt.Errorf("failed to prepare image: %w", err)
	}

	// Create SD card writer
	writer, err := NewSDCardWriter(p.config.DevicePath, dryRun)
	if err != nil {
		return fmt.Errorf("failed to create writer: %w", err)
	}

	// Write image to SD card
	if progress != nil {
		progress.UpdateStatus("Writing image to SD card...")
	}

	if err := writer.WriteImage(ctx, decompressedPath, func(written, total int64) {
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
	// Don't defer cleanup yet - we need to sync first

	// Perform OS-specific setup
	if progress != nil {
		progress.UpdateStatus("Configuring system...")
	}

	if err := provider.PostWriteSetup(bootPath, rootPath, p.config); err != nil {
		cleanup()
		return fmt.Errorf("failed to perform post-write setup: %w", err)
	}

	// Write additional plugin files
	pluginFiles, err := p.plugins.GetAdditionalFiles()
	if err != nil {
		cleanup()
		return fmt.Errorf("failed to get plugin files: %w", err)
	}

	for path, content := range pluginFiles {
		fullPath := filepath.Join(bootPath, path)
		if err := os.WriteFile(fullPath, content, 0o644); err != nil {
			cleanup()
			return fmt.Errorf("failed to write plugin file %s: %w", path, err)
		}
	}

	// Post-provision hooks
	if err := p.plugins.PostProvision(ctx, p.config); err != nil {
		cleanup()
		return fmt.Errorf("post-provision failed: %w", err)
	}

	// Sync filesystem to ensure all writes are persisted
	if progress != nil {
		progress.UpdateStatus("Syncing data to SD card...")
	}

	// Force sync on macOS before unmounting
	if err := writer.SyncPartitions(bootPath, rootPath); err != nil {
		cleanup()
		return fmt.Errorf("failed to sync partitions: %w", err)
	}

	// Verify critical files were written before claiming success
	if progress != nil {
		progress.UpdateStatus("Verifying provisioning...")
	}

	if err := p.verifyProvisioning(bootPath, rootPath, provider); err != nil {
		cleanup()
		return fmt.Errorf("provisioning verification failed: %w", err)
	}

	// Now we can safely cleanup
	cleanup()

	if progress != nil {
		progress.UpdateStatus("Provisioning complete!")
	}

	return nil
}

// ProvisionWithCustomImage provisions using a custom image URL
func (p *CoreProvisioner) ProvisionWithCustomImage(ctx context.Context, imageURL, imageSHA256URL string, dryRun bool, progress ProgressReporter) error {
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

	// Register custom image provider
	customProvider := NewCustomImageProvider(imageURL, imageSHA256URL)
	imageManager.RegisterProvider("custom", customProvider)

	// Download OS image
	if progress != nil {
		progress.UpdateStatus("Downloading custom OS image...")
	}

	imagePath, err := imageManager.DownloadImage(ctx, "custom", "", func(downloaded, total int64) {
		if progress != nil {
			percent := float64(downloaded) / float64(total) * 100
			progress.UpdateProgress(fmt.Sprintf("Downloading: %.1f%%", percent), downloaded, total)
		}
	})
	if err != nil {
		return fmt.Errorf("failed to download image: %w", err)
	}

	// Get the image provider
	provider, err := imageManager.GetProvider("custom")
	if err != nil {
		return err
	}

	// Get decompressed image (from cache if available)
	if progress != nil {
		progress.UpdateStatus("Preparing image...")
	}

	decompressedPath, err := imageManager.GetDecompressedImage(ctx, imagePath, func(status string) {
		if progress != nil {
			progress.UpdateStatus(status)
		}
	})
	if err != nil {
		return fmt.Errorf("failed to prepare image: %w", err)
	}

	// Create SD card writer
	writer, err := NewSDCardWriter(p.config.DevicePath, dryRun)
	if err != nil {
		return fmt.Errorf("failed to create writer: %w", err)
	}

	// Write image to SD card
	if progress != nil {
		progress.UpdateStatus("Writing image to SD card...")
	}

	if err := writer.WriteImage(ctx, decompressedPath, func(written, total int64) {
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
	// Don't defer cleanup yet - we need to sync first

	// Perform OS-specific setup
	if progress != nil {
		progress.UpdateStatus("Configuring system...")
	}

	if err := provider.PostWriteSetup(bootPath, rootPath, p.config); err != nil {
		cleanup()
		return fmt.Errorf("failed to perform post-write setup: %w", err)
	}

	// Write additional plugin files
	pluginFiles, err := p.plugins.GetAdditionalFiles()
	if err != nil {
		cleanup()
		return fmt.Errorf("failed to get plugin files: %w", err)
	}

	for path, content := range pluginFiles {
		fullPath := filepath.Join(bootPath, path)
		if err := os.WriteFile(fullPath, content, 0o644); err != nil {
			cleanup()
			return fmt.Errorf("failed to write plugin file %s: %w", path, err)
		}
	}

	// Post-provision hooks
	if err := p.plugins.PostProvision(ctx, p.config); err != nil {
		cleanup()
		return fmt.Errorf("post-provision failed: %w", err)
	}

	// Sync filesystem to ensure all writes are persisted
	if progress != nil {
		progress.UpdateStatus("Syncing data to SD card...")
	}

	// Force sync on macOS before unmounting
	if err := writer.SyncPartitions(bootPath, rootPath); err != nil {
		cleanup()
		return fmt.Errorf("failed to sync partitions: %w", err)
	}

	// Verify critical files were written before claiming success
	if progress != nil {
		progress.UpdateStatus("Verifying provisioning...")
	}

	if err := p.verifyProvisioning(bootPath, rootPath, provider); err != nil {
		cleanup()
		return fmt.Errorf("provisioning verification failed: %w", err)
	}

	// Now we can safely cleanup
	cleanup()

	if progress != nil {
		progress.UpdateStatus("Provisioning complete!")
	}

	return nil
}

func (p *CoreProvisioner) generateFleetdConfig() string {
	config := fmt.Sprintf(`# fleetd Agent Configuration
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
Description=fleetd Agent
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
# fleetd Setup Script

set -e

echo "Setting up fleetd agent..."

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

// ConfigureOnly configures an existing OS image without writing a new image
func (p *CoreProvisioner) ConfigureOnly(ctx context.Context, osType string, dryRun bool, progress ProgressReporter) error {
	// Pre-provision hooks
	if err := p.plugins.PreProvision(ctx, p.config); err != nil {
		return fmt.Errorf("pre-provision failed: %w", err)
	}

	// Let plugins modify config
	if err := p.plugins.ModifyConfig(p.config); err != nil {
		return fmt.Errorf("config modification failed: %w", err)
	}

	// Initialize image manager to get the provider
	imageManager := NewImageManager("")
	InitializeProviders(imageManager)

	// Get the image provider for OS-specific configuration
	provider, err := imageManager.GetProvider(osType)
	if err != nil {
		// If no specific provider, use a generic one
		if osType == "" || osType == "generic" {
			// For generic, we'll just write basic files
			return p.configureGeneric(ctx, dryRun, progress)
		}
		return fmt.Errorf("unsupported OS type: %s", osType)
	}

	// Create SD card writer (no image writing)
	writer, err := NewSDCardWriter(p.config.DevicePath, dryRun)
	if err != nil {
		return fmt.Errorf("failed to create writer: %w", err)
	}

	// Mount existing partitions
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
	// Don't defer cleanup yet - we need to sync first

	// Perform OS-specific setup
	if progress != nil {
		progress.UpdateStatus("Configuring system...")
	}

	if err := provider.PostWriteSetup(bootPath, rootPath, p.config); err != nil {
		cleanup()
		return fmt.Errorf("failed to configure system: %w", err)
	}

	// Write additional plugin files
	pluginFiles, err := p.plugins.GetAdditionalFiles()
	if err != nil {
		cleanup()
		return fmt.Errorf("failed to get plugin files: %w", err)
	}

	for path, content := range pluginFiles {
		fullPath := filepath.Join(bootPath, path)
		if err := os.WriteFile(fullPath, content, 0o644); err != nil {
			cleanup()
			return fmt.Errorf("failed to write plugin file %s: %w", path, err)
		}
	}

	// Post-provision hooks
	if err := p.plugins.PostProvision(ctx, p.config); err != nil {
		cleanup()
		return fmt.Errorf("post-provision failed: %w", err)
	}

	// Sync filesystem to ensure all writes are persisted
	if progress != nil {
		progress.UpdateStatus("Syncing data to SD card...")
	}

	// Force sync on macOS before unmounting
	if err := writer.SyncPartitions(bootPath, rootPath); err != nil {
		cleanup()
		return fmt.Errorf("failed to sync partitions: %w", err)
	}

	// Verify critical files were written before claiming success
	if progress != nil {
		progress.UpdateStatus("Verifying configuration...")
	}

	if err := p.verifyProvisioning(bootPath, rootPath, provider); err != nil {
		cleanup()
		return fmt.Errorf("configuration verification failed: %w", err)
	}

	// Now we can safely cleanup
	cleanup()

	if progress != nil {
		progress.UpdateStatus("Configuration complete!")
	}

	return nil
}

// configureGeneric handles generic configuration when no OS provider is available
func (p *CoreProvisioner) configureGeneric(ctx context.Context, dryRun bool, progress ProgressReporter) error {
	// For generic configuration, we assume the boot partition is already mounted
	// This is a simplified version that just writes basic files

	writer, err := NewSDCardWriter(p.config.DevicePath, dryRun)
	if err != nil {
		return fmt.Errorf("failed to create writer: %w", err)
	}

	// Try to find existing mount points
	bootDev, _, err := writer.findPartitions()
	if err != nil {
		return fmt.Errorf("failed to find partitions: %w", err)
	}

	bootPath, _ := writer.findExistingMounts(bootDev, "")
	if bootPath == "" {
		return fmt.Errorf("boot partition not mounted - please mount the SD card first")
	}

	if progress != nil {
		progress.UpdateStatus("Configuring system...")
	}

	// Write basic configuration files
	// 1. Enable SSH
	sshFile := filepath.Join(bootPath, "ssh")
	if err := os.WriteFile(sshFile, []byte(""), 0o644); err != nil {
		return fmt.Errorf("failed to enable SSH: %w", err)
	}

	// 2. Configure WiFi
	if p.config.Network.WiFiSSID != "" {
		wpaConf := fmt.Sprintf(`ctrl_interface=DIR=/var/run/wpa_supplicant GROUP=netdev
update_config=1
country=US

network={
    ssid="%s"
    psk="%s"
    key_mgmt=WPA-PSK
}
`, p.config.Network.WiFiSSID, p.config.Network.WiFiPass)

		wpaFile := filepath.Join(bootPath, "wpa_supplicant.conf")
		if err := os.WriteFile(wpaFile, []byte(wpaConf), 0o644); err != nil {
			return fmt.Errorf("failed to configure WiFi: %w", err)
		}
	}

	// 3. Create user configuration
	userConf := "pi:$6$Zmd8gFg8RFR0M5Xf$nFgQNqVKDMFfKz3lYkvEGywz.8INzF9fPE8ci3IMTLfxKPpMFsNs8Sw9koYoB1sZ8sNHZGJ/M0uYUUJw8Xqdn."
	userConfFile := filepath.Join(bootPath, "userconf.txt")
	if err := os.WriteFile(userConfFile, []byte(userConf), 0o644); err != nil {
		return fmt.Errorf("failed to configure user: %w", err)
	}

	// Sync filesystem
	if progress != nil {
		progress.UpdateStatus("Syncing data to SD card...")
	}

	if err := writer.SyncPartitions(bootPath, ""); err != nil {
		return fmt.Errorf("failed to sync: %w", err)
	}

	// Verify critical files were written before claiming success
	if progress != nil {
		progress.UpdateStatus("Verifying configuration...")
	}

	if err := p.verifyGenericProvisioning(bootPath); err != nil {
		return fmt.Errorf("configuration verification failed: %w", err)
	}

	if progress != nil {
		progress.UpdateStatus("Configuration complete!")
	}

	return nil
}

// verifyProvisioning verifies that critical files were written correctly
func (p *CoreProvisioner) verifyProvisioning(bootPath, rootPath string, provider ImageProvider) error {
	// List of critical files that must exist for provisioning to be considered successful
	criticalFiles := []struct {
		path        string
		description string
		minSize     int64 // minimum expected file size in bytes
	}{
		// For DietPi
		{filepath.Join(bootPath, "dietpi.txt"), "DietPi configuration", 100},
		{filepath.Join(bootPath, "Automation_Custom_Script.sh"), "DietPi automation script", 100},
		{filepath.Join(bootPath, "fleetd"), "fleetd binary", 1024 * 1024}, // at least 1MB

		// For RaspiOS
		{filepath.Join(bootPath, "userconf.txt"), "RaspiOS user configuration", 10},
		{filepath.Join(bootPath, "fleetd.service"), "fleetd systemd service", 100},
		{filepath.Join(bootPath, "fleetd-install.sh"), "fleetd installation script", 100},
		{filepath.Join(bootPath, "FLEETD_README.txt"), "fleetd installation guide", 100},

		// Common files
		{filepath.Join(bootPath, "ssh"), "SSH enablement", 0}, // can be empty
	}

	// Add WiFi config if configured
	if p.config.Network.WiFiSSID != "" {
		criticalFiles = append(criticalFiles, struct {
			path        string
			description string
			minSize     int64
		}{
			filepath.Join(bootPath, "dietpi-wifi.txt"), "WiFi configuration", 50,
		})
		criticalFiles = append(criticalFiles, struct {
			path        string
			description string
			minSize     int64
		}{
			filepath.Join(bootPath, "wpa_supplicant.conf"), "WiFi configuration", 50,
		})
	}

	// Check at least one critical file exists (different OS types have different files)
	foundCriticalFile := false
	var verificationErrors []string

	for _, file := range criticalFiles {
		fi, err := os.Stat(file.path)
		if err != nil {
			if !os.IsNotExist(err) {
				// Unexpected error reading file
				verificationErrors = append(verificationErrors, fmt.Sprintf("%s: %v", file.description, err))
			}
			continue
		}

		// File exists, check size
		if fi.Size() < file.minSize {
			verificationErrors = append(verificationErrors,
				fmt.Sprintf("%s appears to be incomplete (size: %d bytes, expected at least %d)",
					file.description, fi.Size(), file.minSize))
			continue
		}

		// Found at least one valid critical file
		foundCriticalFile = true
	}

	if !foundCriticalFile {
		return fmt.Errorf("no critical configuration files found on SD card - provisioning may have failed")
	}

	// Check that fleetd binary specifically exists and is executable
	fleetdPath := filepath.Join(bootPath, "fleetd")
	fi, err := os.Stat(fleetdPath)
	if err != nil {
		return fmt.Errorf("fleetd binary not found on SD card: %w", err)
	}

	if fi.Size() < 1024*1024 {
		return fmt.Errorf("fleetd binary appears to be corrupted (size: %d bytes)", fi.Size())
	}

	// If we have specific errors but found some files, warn but don't fail
	if len(verificationErrors) > 0 && foundCriticalFile {
		fmt.Printf("Warning: Some files could not be verified:\n")
		for _, err := range verificationErrors {
			fmt.Printf("  - %s\n", err)
		}
	}

	return nil
}

// verifyGenericProvisioning verifies generic provisioning (used when no OS provider is available)
func (p *CoreProvisioner) verifyGenericProvisioning(bootPath string) error {
	// For generic provisioning, we check basic files
	criticalFiles := []struct {
		path        string
		description string
		minSize     int64
	}{
		{filepath.Join(bootPath, "ssh"), "SSH enablement", 0}, // can be empty
	}

	// Add WiFi config if configured
	if p.config.Network.WiFiSSID != "" {
		criticalFiles = append(criticalFiles, struct {
			path        string
			description string
			minSize     int64
		}{
			filepath.Join(bootPath, "wpa_supplicant.conf"), "WiFi configuration", 50,
		})
	}

	// Add user config
	criticalFiles = append(criticalFiles, struct {
		path        string
		description string
		minSize     int64
	}{
		filepath.Join(bootPath, "userconf.txt"), "User configuration", 20,
	})

	// Verify all files exist
	for _, file := range criticalFiles {
		fi, err := os.Stat(file.path)
		if err != nil {
			return fmt.Errorf("failed to verify %s: %w", file.description, err)
		}

		if fi.Size() < file.minSize {
			return fmt.Errorf("%s appears to be incomplete (size: %d bytes, expected at least %d)",
				file.description, fi.Size(), file.minSize)
		}
	}

	return nil
}
