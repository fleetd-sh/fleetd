package provision

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// createRasPiOSFirstRun creates a firstrun.sh script for Raspberry Pi OS
func createRasPiOSFirstRun(config Config) string {
	// Create a firstrun.sh script that follows Raspberry Pi OS conventions
	script := `#!/bin/bash
# FleetD First Boot Setup for Raspberry Pi OS
# This script runs once on first boot via cmdline.txt

set +e  # Don't exit on error to ensure cleanup happens

LOG_FILE="/var/log/fleetd-firstrun.log"
MARKER_FILE="/var/lib/fleetd/.firstrun-complete"

# Logging function
log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1" | tee -a "$LOG_FILE"
}

log "Starting FleetD first boot setup..."

# Check if we've already run
if [ -f "$MARKER_FILE" ]; then
    log "First run already completed, exiting"
    exit 0
fi

# Wait for network if WiFi was configured
if [ -f /boot/firmware/wpa_supplicant.conf ] || [ -f /boot/wpa_supplicant.conf ]; then
    log "Waiting for network connection..."
    for i in {1..30}; do
        if ping -c 1 -W 1 8.8.8.8 &>/dev/null; then
            log "Network connected"
            break
        fi
        sleep 2
    done
fi

# Determine boot partition mount point
BOOT_PARTITION="/boot"
if [ -d "/boot/firmware" ]; then
    BOOT_PARTITION="/boot/firmware"
fi
log "Boot partition mounted at: $BOOT_PARTITION"

# Install fleetd binary
if [ -f "$BOOT_PARTITION/fleetd" ]; then
    log "Installing fleetd binary..."
    cp "$BOOT_PARTITION/fleetd" /usr/local/bin/fleetd
    chmod +x /usr/local/bin/fleetd
    log "FleetD binary installed"
else
    log "ERROR: fleetd binary not found in $BOOT_PARTITION"
    exit 1
fi

# Install systemd service
if [ -f "$BOOT_PARTITION/fleetd.service" ]; then
    log "Installing fleetd systemd service..."
    cp "$BOOT_PARTITION/fleetd.service" /etc/systemd/system/
    systemctl daemon-reload
    log "FleetD service installed"
else
    log "ERROR: fleetd.service not found in $BOOT_PARTITION"
    exit 1
fi

# User should already be created by userconf.txt
# Just ensure they have sudo access
if id -u fleetd &>/dev/null; then
    log "Ensuring fleetd user has sudo access..."
    usermod -aG sudo fleetd || true
else
    log "WARNING: fleetd user not found - was userconf.txt processed?"
fi

# Create storage directory
log "Creating storage directory..."
mkdir -p /var/lib/fleetd
chown fleetd:fleetd /var/lib/fleetd

# Install mDNS support if not present
if ! command -v avahi-daemon &>/dev/null; then
    log "Installing mDNS support..."
    apt-get update -qq
    apt-get install -y avahi-daemon
fi

# Enable and start services
log "Enabling services..."
systemctl enable avahi-daemon
systemctl start avahi-daemon

systemctl enable fleetd
systemctl start fleetd

# Wait a moment for service to start
sleep 5

# Check status
if systemctl is-active --quiet fleetd; then
    log "FleetD is running successfully!"
    DEVICE_IP=$(hostname -I | awk '{print $1}')
    log "Device IP: $DEVICE_IP"
else
    log "WARNING: FleetD service failed to start"
    log "Check logs with: journalctl -u fleetd"
fi

# Create marker file
mkdir -p $(dirname "$MARKER_FILE")
touch "$MARKER_FILE"

# Clean up boot partition files (optional, for security)
log "Cleaning up boot files..."
rm -f "$BOOT_PARTITION/fleetd"
rm -f "$BOOT_PARTITION/fleetd.service"
rm -f "$BOOT_PARTITION/DEVICE_CREDENTIALS.txt"

# Execute plugin scripts if any exist
if [ -d "$BOOT_PARTITION/plugins" ]; then
    log "Found plugin scripts directory"
    for plugin_script in "$BOOT_PARTITION/plugins"/*.sh; do
        if [ -f "$plugin_script" ]; then
            plugin_name=$(basename "$plugin_script" .sh)
            log "Executing plugin script: $plugin_name"
            chmod +x "$plugin_script"
            if bash "$plugin_script"; then
                log "Plugin $plugin_name completed successfully"
            else
                log "WARNING: Plugin $plugin_name failed with exit code $?"
            fi
        fi
    done

    # Clean up plugin scripts after execution
    rm -rf "$BOOT_PARTITION/plugins"
fi

log "First boot setup completed!"

# Remove firstrun.sh from cmdline.txt to prevent re-runs
# This is what Raspberry Pi OS's own firstrun does
sed -i 's| systemd.run=/boot/firstrun.sh systemd.run_success_action=reboot systemd.unit=kernel-command-line.target||g' /boot/cmdline.txt
sed -i 's| systemd.run=/boot/firmware/firstrun.sh systemd.run_success_action=reboot systemd.unit=kernel-command-line.target||g' /boot/firmware/cmdline.txt

# The systemd.run_success_action=reboot will handle the reboot
`

	return script
}

// modifyRasPiOSCmdline modifies cmdline.txt to run firstrun.sh on boot
func modifyRasPiOSCmdline(bootPath string) error {
	cmdlinePath := filepath.Join(bootPath, "cmdline.txt")

	// Read existing cmdline.txt
	content, err := os.ReadFile(cmdlinePath)
	if err != nil {
		return fmt.Errorf("failed to read cmdline.txt: %w", err)
	}

	// Check if already modified
	if strings.Contains(string(content), "systemd.run=/boot/firstrun.sh") {
		return nil // Already modified
	}

	// Append firstrun parameters
	// Note: We use /boot/firstrun.sh because that's what cmdline.txt sees
	// even though it might be mounted at /boot/firmware/ in the running system
	newContent := strings.TrimSpace(string(content)) + " systemd.run=/boot/firstrun.sh systemd.run_success_action=reboot systemd.unit=kernel-command-line.target"

	// Write back
	if err := os.WriteFile(cmdlinePath, []byte(newContent), 0755); err != nil {
		return fmt.Errorf("failed to write cmdline.txt: %w", err)
	}

	return nil
}
