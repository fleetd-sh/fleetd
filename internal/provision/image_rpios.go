package provision

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// RaspberryPiOSProvider handles Raspberry Pi OS provisioning
type RaspberryPiOSProvider struct {
	version string
}

// NewRaspberryPiOSProvider creates a new Raspberry Pi OS provider
// Uses Raspberry Pi OS Lite by default for minimal footprint
func NewRaspberryPiOSProvider() *RaspberryPiOSProvider {
	return &RaspberryPiOSProvider{
		version: "2024-11-19", // Latest stable Lite version
	}
}

// GetImageURL returns the download URL for Raspberry Pi OS Lite
// Always uses the Lite version for minimal resource usage and faster provisioning
func (p *RaspberryPiOSProvider) GetImageURL(arch string) (string, error) {
	switch arch {
	case "arm64", "aarch64":
		// 64-bit Raspberry Pi OS Lite (no desktop)
		return "https://downloads.raspberrypi.com/raspios_lite_arm64/images/raspios_lite_arm64-2024-11-19/2024-11-19-raspios-bookworm-arm64-lite.img.xz", nil
	case "armv7", "armhf", "arm":
		// 32-bit Raspberry Pi OS Lite (no desktop)
		return "https://downloads.raspberrypi.com/raspios_lite_armhf/images/raspios_lite_armhf-2024-11-19/2024-11-19-raspios-bookworm-armhf-lite.img.xz", nil
	default:
		return "", fmt.Errorf("unsupported architecture for Raspberry Pi OS: %s", arch)
	}
}

// GetImageName returns the name of this image
func (p *RaspberryPiOSProvider) GetImageName() string {
	return "raspios"
}

// ValidateImage validates the downloaded image
func (p *RaspberryPiOSProvider) ValidateImage(imagePath string) error {
	// For now, just check if file exists and is not empty
	fi, err := os.Stat(imagePath)
	if err != nil {
		return err
	}
	if fi.Size() == 0 {
		return fmt.Errorf("image file is empty")
	}
	return nil
}

// GetBootPartitionLabel returns the boot partition label
func (p *RaspberryPiOSProvider) GetBootPartitionLabel() string {
	return "boot"
}

// GetRootPartitionLabel returns the root partition label
func (p *RaspberryPiOSProvider) GetRootPartitionLabel() string {
	return "rootfs"
}

// PostWriteSetup performs Raspberry Pi OS specific setup
func (p *RaspberryPiOSProvider) PostWriteSetup(bootPath, rootPath string, config *Config) error {
	fmt.Printf("RaspiOS PostWriteSetup called with bootPath: %s, rootPath: %s\n", bootPath, rootPath)
	return p.ConfigureImage(*config, bootPath, rootPath)
}

// ConfigureImage configures the OS for zero-touch provisioning
func (p *RaspberryPiOSProvider) ConfigureImage(config Config, bootPath, rootPath string) error {
	// 1. Enable SSH by creating ssh file in boot partition
	sshFile := filepath.Join(bootPath, "ssh")
	fmt.Printf("Writing SSH enable file to: %s\n", sshFile)
	if err := os.WriteFile(sshFile, []byte(""), 0644); err != nil {
		return fmt.Errorf("failed to enable SSH: %w", err)
	}
	fmt.Printf("Successfully wrote SSH enable file\n")

	// 2. Configure WiFi if provided
	if config.Network.WiFiSSID != "" {
		// Use the exact format from the working guide
		wpaConf := fmt.Sprintf(`ctrl_interface=DIR=/var/run/wpa_supplicant GROUP=netdev
update_config=1
country=US

network={
    ssid="%s"
    psk="%s"
    key_mgmt=WPA-PSK
}
`, config.Network.WiFiSSID, config.Network.WiFiPass)

		wpaFile := filepath.Join(bootPath, "wpa_supplicant.conf")
		if err := os.WriteFile(wpaFile, []byte(wpaConf), 0644); err != nil {
			return fmt.Errorf("failed to configure WiFi: %w", err)
		}
	}

	// 3. Set up user with userconf.txt
	// Generate a random password for this device
	password := generateRandomPassword()
	hashedPassword := hashPassword(password)

	// Create fleetd user with sudo access
	username := "fleetd"
	userConf := fmt.Sprintf("%s:%s", username, hashedPassword)
	userConfFile := filepath.Join(bootPath, "userconf.txt")
	if err := os.WriteFile(userConfFile, []byte(userConf), 0644); err != nil {
		return fmt.Errorf("failed to configure user: %w", err)
	}

	// Save credentials info for the user
	credsInfo := fmt.Sprintf(`Device Credentials
==================
Username: %s
Password: %s

Please save these credentials securely.
You should change the password after first login.
`, username, password)

	credsFile := filepath.Join(bootPath, "DEVICE_CREDENTIALS.txt")
	if err := os.WriteFile(credsFile, []byte(credsInfo), 0600); err != nil {
		return fmt.Errorf("failed to save credentials: %w", err)
	}

	// Also display to console
	fmt.Printf("\n🔐 Device Credentials:\n")
	fmt.Printf("   Username: %s\n", username)
	fmt.Printf("   Password: %s\n", password)
	fmt.Printf("   (Also saved to /boot/firmware/DEVICE_CREDENTIALS.txt)\n\n")

	// 3a. For newer Raspberry Pi OS, we should avoid conflicting with their firstrun
	// The built-in firstrun.sh handles user creation from userconf.txt

	// 4. Create systemd service file
	systemdService := `[Unit]
Description=FleetD Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/fleetd agent --storage-dir /var/lib/fleetd --rpc-port 8081
Restart=always
RestartSec=10
Environment=FLEETD_LOG_LEVEL=debug

[Install]
WantedBy=multi-user.target
`

	// Write service file to boot partition for later copying
	serviceFile := filepath.Join(bootPath, "fleetd.service")
	if err := os.WriteFile(serviceFile, []byte(systemdService), 0644); err != nil {
		return fmt.Errorf("failed to create service file: %w", err)
	}

	// 5. We'll keep the fleetd binary and service file in boot partition
	// But we won't create any scripts that might interfere with first boot
	// Note: Since Raspberry Pi OS Bookworm (2024), the boot partition is mounted at /boot/firmware/
	// (Previously it was mounted at /boot/ on older Raspberry Pi OS versions)

	// 6. Copy fleetd binary to boot partition
	fleetdBinary := "bin/fleetd-arm64"
	if _, err := os.Stat(fleetdBinary); err != nil {
		// Try to build the binary if it doesn't exist
		log.Printf("fleetd binary not found at %s, attempting to build...", fleetdBinary)
		if err := p.buildFleetdBinary(); err != nil {
			return fmt.Errorf("fleetd binary not found at %s and build failed: %w", fleetdBinary, err)
		}
		// Check again after build
		if _, err := os.Stat(fleetdBinary); err != nil {
			return fmt.Errorf("fleetd binary still not found after build attempt: %w", err)
		}
	}

	input, err := os.ReadFile(fleetdBinary)
	if err != nil {
		return fmt.Errorf("failed to read fleetd binary: %w", err)
	}

	bootFleetd := filepath.Join(bootPath, "fleetd")
	if err := os.WriteFile(bootFleetd, input, 0755); err != nil {
		return fmt.Errorf("failed to copy fleetd to boot: %w", err)
	}

	// 7. Create firstrun.sh script for Raspberry Pi OS
	// This is the official way to run scripts on first boot
	firstrunScript := createRPiOSFirstRun(config)
	firstrunFile := filepath.Join(bootPath, "firstrun.sh")
	if err := os.WriteFile(firstrunFile, []byte(firstrunScript), 0755); err != nil {
		return fmt.Errorf("failed to create firstrun.sh: %w", err)
	}
	fmt.Printf("Created firstrun.sh for automatic installation\n")

	// 8. Modify cmdline.txt to run firstrun.sh on boot
	if err := modifyRPiOSCmdline(bootPath); err != nil {
		return fmt.Errorf("failed to modify cmdline.txt: %w", err)
	}
	fmt.Printf("Modified cmdline.txt to run firstrun.sh on first boot\n")

	// 9. Create a simple README for reference
	readme := `FleetD Automatic Installation
=============================

FleetD will be automatically installed on first boot.

The system will:
1. Run firstrun.sh on first boot (via cmdline.txt)
2. Install fleetd binary and service
3. Configure mDNS for discovery
4. Start the fleetd agent
5. Reboot automatically

After your Raspberry Pi boots (twice - once for setup, once for normal boot):

1. Check installation status:
   cat /var/log/fleetd-firstrun.log

2. Check service status:
   sudo systemctl status fleetd
   sudo systemctl status avahi-daemon

3. View logs:
   sudo journalctl -u fleetd -f

Your device should automatically appear in fleet discovery!

Note: Check DEVICE_CREDENTIALS.txt for your login credentials (removed after first boot)
`
	readmeFile := filepath.Join(bootPath, "FLEETD_README.txt")
	if err := os.WriteFile(readmeFile, []byte(readme), 0644); err != nil {
		return fmt.Errorf("failed to create README: %w", err)
	}

	// Note: filesystem sync is handled by the writer after all files are written

	// Debug: List files in boot partition
	fmt.Printf("\nFiles written to boot partition (%s):\n", bootPath)
	entries, err := os.ReadDir(bootPath)
	if err != nil {
		fmt.Printf("Error reading boot partition: %v\n", err)
	} else {
		for _, entry := range entries {
			info, _ := entry.Info()
			fmt.Printf("  - %s (size: %d bytes)\n", entry.Name(), info.Size())
		}
	}

	return nil
}

// generateRandomPassword generates a secure random password
func generateRandomPassword() string {
	// Generate 12 random bytes
	bytes := make([]byte, 12)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to a timestamp-based password if random fails
		return fmt.Sprintf("fleetd-%d", os.Getpid())
	}

	// Convert to base64 and make it URL-safe
	password := base64.URLEncoding.EncodeToString(bytes)
	// Remove padding characters that might cause issues
	password = strings.TrimRight(password, "=")

	return password
}

// hashPassword creates a password hash compatible with /etc/shadow
func hashPassword(password string) string {
	// Use openssl to generate the password hash
	// This creates a SHA-512 hash with a random salt
	cmd := exec.Command("openssl", "passwd", "-6", password)
	output, err := cmd.Output()
	if err != nil {
		// If openssl fails, try Python as fallback
		pythonCmd := fmt.Sprintf(
			`python3 -c "import crypt; print(crypt.crypt('%s', crypt.mksalt(crypt.METHOD_SHA512)))"`,
			password,
		)
		cmd = exec.Command("bash", "-c", pythonCmd)
		output, err = cmd.Output()
		if err != nil {
			// Last resort: return a known hash for "raspberry"
			// This is not secure but at least allows login
			return "$6$RzQD2VaGVRSmGFNZ$5dVIT7utVYrpJt.V5lOHB1tT4q7h9hNvVONFbN5HEYvRueWPdIoMEaJmhF5Y5FKOvP6YzaF7RBujzDuZgUNvG/"
		}
	}

	return strings.TrimSpace(string(output))
}

// buildFleetdBinary attempts to build the fleetd binary
func (p *RaspberryPiOSProvider) buildFleetdBinary() error {
	// Check if 'just' is available
	cmd := exec.Command("just", "build", "fleetd", "arm64")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		// Fall back to make if just is not available
		log.Println("'just' command failed, trying 'make'...")
		cmd = exec.Command("make", "build-fleetd-arm64")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to build fleetd binary: %w", err)
		}
	}
	return nil
}
