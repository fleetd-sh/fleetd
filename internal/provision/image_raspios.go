package provision

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RaspberryPiOSProvider provides Raspberry Pi OS images
type RaspberryPiOSProvider struct {
	variant string // lite, desktop, full
}

// NewRaspberryPiOSProvider creates a new Raspberry Pi OS provider
func NewRaspberryPiOSProvider() *RaspberryPiOSProvider {
	return &RaspberryPiOSProvider{
		variant: "lite", // Default to lite version
	}
}

// GetImageURL returns the download URL for Raspberry Pi OS
func (p *RaspberryPiOSProvider) GetImageURL(arch string) (string, error) {
	// Map architecture to image URL
	var imageURL string
	switch arch {
	case "arm64", "aarch64":
		// 64-bit Raspberry Pi OS
		imageURL = "https://downloads.raspberrypi.com/raspios_lite_arm64/images/raspios_lite_arm64-2024-11-19/2024-11-19-raspios-bookworm-arm64-lite.img.xz"
	case "armv7", "armhf", "arm":
		// 32-bit Raspberry Pi OS
		imageURL = "https://downloads.raspberrypi.com/raspios_lite_armhf/images/raspios_lite_armhf-2024-11-19/2024-11-19-raspios-bookworm-armhf-lite.img.xz"
	default:
		return "", fmt.Errorf("unsupported architecture for Raspberry Pi OS: %s", arch)
	}
	
	return imageURL, nil
}

// GetImageName returns the name of this image
func (p *RaspberryPiOSProvider) GetImageName() string {
	return fmt.Sprintf("raspios-%s", p.variant)
}

// ValidateImage validates the downloaded image
func (p *RaspberryPiOSProvider) ValidateImage(imagePath string) error {
	// Check if file exists and is not empty
	fi, err := os.Stat(imagePath)
	if err != nil {
		return err
	}
	
	if fi.Size() == 0 {
		return fmt.Errorf("image file is empty")
	}
	
	// TODO: Verify SHA256 checksum from Raspberry Pi Foundation
	return nil
}

// GetBootPartitionLabel returns the boot partition label
func (p *RaspberryPiOSProvider) GetBootPartitionLabel() string {
	return "bootfs"
}

// GetRootPartitionLabel returns the root partition label
func (p *RaspberryPiOSProvider) GetRootPartitionLabel() string {
	return "rootfs"
}

// PostWriteSetup performs Raspberry Pi OS specific setup
func (p *RaspberryPiOSProvider) PostWriteSetup(bootPath, rootPath string, config *Config) error {
	// Enable SSH
	if config.Security.EnableSSH {
		sshFile := filepath.Join(bootPath, "ssh")
		if err := os.WriteFile(sshFile, []byte(""), 0644); err != nil {
			return fmt.Errorf("failed to enable SSH: %w", err)
		}
	}
	
	// Configure WiFi
	if config.Network.WiFiSSID != "" {
		if err := p.writeWPASupplicant(bootPath, config); err != nil {
			return fmt.Errorf("failed to write WiFi config: %w", err)
		}
	}
	
	// Set hostname
	if err := p.writeUserConfig(bootPath, config); err != nil {
		return fmt.Errorf("failed to write user config: %w", err)
	}
	
	// Write fleetd configuration
	if err := p.writeFleetdConfig(bootPath, config); err != nil {
		return fmt.Errorf("failed to write fleetd config: %w", err)
	}
	
	// Create first boot script
	if err := p.writeFirstRunScript(bootPath, config); err != nil {
		return fmt.Errorf("failed to write first run script: %w", err)
	}
	
	// Add SSH key if provided
	if config.Security.SSHKey != "" {
		if err := p.setupSSHKey(rootPath, config.Security.SSHKey); err != nil {
			return fmt.Errorf("failed to setup SSH key: %w", err)
		}
	}
	
	return nil
}

// writeWPASupplicant creates the WiFi configuration
func (p *RaspberryPiOSProvider) writeWPASupplicant(bootPath string, config *Config) error {
	wpaConfig := fmt.Sprintf(`country=US
ctrl_interface=DIR=/var/run/wpa_supplicant GROUP=netdev
ap_scan=1

update_config=1
network={
    ssid="%s"
    psk="%s"
    key_mgmt=WPA-PSK
}
`, config.Network.WiFiSSID, config.Network.WiFiPass)
	
	wpaPath := filepath.Join(bootPath, "wpa_supplicant.conf")
	return os.WriteFile(wpaPath, []byte(wpaConfig), 0644)
}

// writeUserConfig creates user configuration using cloud-init style
func (p *RaspberryPiOSProvider) writeUserConfig(bootPath string, config *Config) error {
	// Create userconf.txt for headless setup (username:encrypted_password)
	// Default: pi:raspberry (encrypted)
	userConf := "pi:$6$rBoByrWRKMY1EHFy$ho.LISnfm83CLBWBE/yqJ6Lq1TinRlxw/ImMTPcvvMuUfhQYcMmFnpFXUPowjy2br1NA0IACwF9JKugSNuHoe0"
	
	userConfPath := filepath.Join(bootPath, "userconf.txt")
	if err := os.WriteFile(userConfPath, []byte(userConf), 0644); err != nil {
		return err
	}
	
	// Create custom firstrun.sh script
	firstRun := fmt.Sprintf(`#!/bin/bash
set +e

# Set hostname
CURRENT_HOSTNAME=$(cat /etc/hostname | tr -d " \t\n\r")
if [ "$CURRENT_HOSTNAME" != "%s" ]; then
    echo "%s" > /etc/hostname
    sed -i "s/127.0.1.1.*$CURRENT_HOSTNAME/127.0.1.1\t%s/g" /etc/hosts
fi

# Continue with fleetd setup
if [ -f /boot/fleetd-firstrun.sh ]; then
    bash /boot/fleetd-firstrun.sh
fi

rm -f /boot/firstrun.sh
sed -i 's| systemd.run.*||g' /boot/cmdline.txt
exit 0
`, config.DeviceName, config.DeviceName, config.DeviceName)
	
	firstRunPath := filepath.Join(bootPath, "firstrun.sh")
	if err := os.WriteFile(firstRunPath, []byte(firstRun), 0755); err != nil {
		return err
	}
	
	// Update cmdline.txt to run firstrun.sh
	cmdlinePath := filepath.Join(bootPath, "cmdline.txt")
	cmdlineContent, err := os.ReadFile(cmdlinePath)
	if err == nil {
		// Append firstrun command if not already present
		cmdline := string(cmdlineContent)
		if !strings.Contains(cmdline, "systemd.run=") {
			cmdline = strings.TrimSpace(cmdline) + " systemd.run=/boot/firstrun.sh systemd.run_success_action=reboot systemd.unit=kernel-command-line.target"
			os.WriteFile(cmdlinePath, []byte(cmdline), 0644)
		}
	}
	
	return nil
}

// setupSSHKey adds an SSH public key for the pi user
func (p *RaspberryPiOSProvider) setupSSHKey(rootPath string, sshKey string) error {
	// Create .ssh directory for pi user
	sshDir := filepath.Join(rootPath, "home", "pi", ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		return err
	}
	
	// Write authorized_keys
	authKeysPath := filepath.Join(sshDir, "authorized_keys")
	return os.WriteFile(authKeysPath, []byte(sshKey), 0600)
}

// writeFleetdConfig writes the fleetd agent configuration
func (p *RaspberryPiOSProvider) writeFleetdConfig(bootPath string, config *Config) error {
	fleetdYAML := fmt.Sprintf(`# FleetD Agent Configuration
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

telemetry:
  enabled: true
  interval: 60s
`, config.DeviceID, config.DeviceName, config.DeviceType)
	
	if config.Fleet.ServerURL != "" {
		fleetdYAML += fmt.Sprintf(`
server:
  url: %s
`, config.Fleet.ServerURL)
	}
	
	configPath := filepath.Join(bootPath, "fleetd.yaml")
	return os.WriteFile(configPath, []byte(fleetdYAML), 0644)
}

// writeFirstRunScript creates the fleetd installation script
func (p *RaspberryPiOSProvider) writeFirstRunScript(bootPath string, config *Config) error {
	script := `#!/bin/bash
# FleetD First Run Setup Script

echo "Starting fleetd setup..."

# Wait for network
while ! ping -c 1 -W 1 8.8.8.8 &> /dev/null; do
    echo "Waiting for network..."
    sleep 2
done

# Update system
apt-get update

# Download and install fleetd
ARCH=$(uname -m)
case $ARCH in
    aarch64) ARCH="arm64" ;;
    armv7l)  ARCH="armv7" ;;
    armv6l)  ARCH="armv6" ;;
esac

echo "Downloading fleetd for $ARCH..."
wget -q -O /tmp/fleetd "https://github.com/fleetd-sh/fleetd/releases/latest/download/fleetd-linux-$ARCH"
chmod +x /tmp/fleetd
mv /tmp/fleetd /usr/local/bin/fleetd

# Copy configuration
mkdir -p /etc/fleetd
cp /boot/fleetd.yaml /etc/fleetd/config.yaml

# Create systemd service
cat > /etc/systemd/system/fleetd.service << 'EOF'
[Unit]
Description=FleetD Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/fleetd agent --config /etc/fleetd/config.yaml
Restart=always
RestartSec=10
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
EOF

# Enable and start fleetd
systemctl daemon-reload
systemctl enable fleetd
systemctl start fleetd

echo "FleetD setup complete!"
`
	
	// Add k3s installation if configured
	if plugins, ok := config.Plugins["k3s"].(map[string]any); ok {
		role, _ := plugins["role"].(string)
		if role == "server" {
			script += `
# Install k3s server
curl -sfL https://get.k3s.io | sh -s - server --write-kubeconfig-mode 644
`
		} else if role == "agent" {
			serverURL, _ := plugins["server"].(string)
			token, _ := plugins["token"].(string)
			script += fmt.Sprintf(`
# Install k3s agent
curl -sfL https://get.k3s.io | K3S_URL=%s K3S_TOKEN=%s sh -
`, serverURL, token)
		}
	}
	
	scriptPath := filepath.Join(bootPath, "fleetd-firstrun.sh")
	return os.WriteFile(scriptPath, []byte(script), 0755)
}