package provision

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// CustomImageProvider provides custom OS images from user-specified URLs
type CustomImageProvider struct {
	imageURL       string
	imageSHA256URL string
	platform       string   // Target platform (e.g., "linux", "rtos")
	architectures  []string // Supported architectures
}

// NewCustomImageProvider creates a new custom image provider
func NewCustomImageProvider(imageURL, imageSHA256URL string) *CustomImageProvider {
	return &CustomImageProvider{
		imageURL:       imageURL,
		imageSHA256URL: imageSHA256URL,
		platform:       "",  // Unknown platform by default
		architectures:  nil, // Unknown architectures by default
	}
}

// SetPlatformInfo sets the platform and architecture information for the custom image
func (p *CustomImageProvider) SetPlatformInfo(platform string, architectures []string) {
	if platform != "" {
		p.platform = platform
	}
	if len(architectures) > 0 {
		p.architectures = architectures
	}
}

// GetImageURL returns the user-provided image URL
func (p *CustomImageProvider) GetImageURL(arch string) (string, error) {
	if p.imageURL == "" {
		return "", fmt.Errorf("no image URL provided")
	}
	return p.imageURL, nil
}

// GetImageName returns a descriptive name for the image
func (p *CustomImageProvider) GetImageName() string {
	// Extract filename from URL
	parts := strings.Split(p.imageURL, "/")
	if len(parts) > 0 {
		filename := parts[len(parts)-1]
		// Remove compression extensions
		filename = strings.TrimSuffix(filename, ".xz")
		filename = strings.TrimSuffix(filename, ".gz")
		filename = strings.TrimSuffix(filename, ".7z")
		filename = strings.TrimSuffix(filename, ".zip")
		filename = strings.TrimSuffix(filename, ".zst")
		filename = strings.TrimSuffix(filename, ".img")
		return filename
	}
	return "custom-image"
}

// ValidateImage validates the downloaded image against provided checksum
func (p *CustomImageProvider) ValidateImage(imagePath string) error {
	// Basic validation
	fi, err := os.Stat(imagePath)
	if err != nil {
		return err
	}

	if fi.Size() == 0 {
		return fmt.Errorf("image file is empty")
	}

	// If SHA256 URL is provided, download and verify
	if p.imageSHA256URL != "" {
		expectedSum, err := p.downloadChecksum()
		if err != nil {
			return fmt.Errorf("failed to download checksum: %w", err)
		}

		actualSum, err := calculateSHA256(imagePath)
		if err != nil {
			return fmt.Errorf("failed to calculate checksum: %w", err)
		}

		if expectedSum != actualSum {
			return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedSum, actualSum)
		}
	}

	return nil
}

// GetBootPartitionLabel returns the boot partition label
func (p *CustomImageProvider) GetBootPartitionLabel() string {
	// Common labels for boot partitions
	// This might need to be customizable in the future
	return "boot"
}

// GetRootPartitionLabel returns the root partition label
func (p *CustomImageProvider) GetRootPartitionLabel() string {
	// Common labels for root partitions
	return "rootfs"
}

// GetPlatform returns the target platform
func (p *CustomImageProvider) GetPlatform() string {
	return p.platform
}

// GetSupportedArchitectures returns supported architectures
func (p *CustomImageProvider) GetSupportedArchitectures() []string {
	return p.architectures
}

// PostWriteSetup performs OS-specific setup
func (p *CustomImageProvider) PostWriteSetup(bootPath, rootPath string, config *Config) error {
	// For custom images, we'll try to detect the OS type and apply appropriate setup

	// Check if it's Raspberry Pi OS
	if _, err := os.Stat(filepath.Join(bootPath, "cmdline.txt")); err == nil {
		// Use Raspberry Pi OS setup
		raspios := NewRaspberryPiOSProvider()
		return raspios.PostWriteSetup(bootPath, rootPath, config)
	}

	// Generic setup for unknown OS
	return p.genericPostWriteSetup(bootPath, rootPath, config)
}

func (p *CustomImageProvider) genericPostWriteSetup(bootPath, rootPath string, config *Config) error {
	// Try to enable SSH (common across many distributions)
	if config.Security.EnableSSH {
		// Try boot partition method
		sshFile := filepath.Join(bootPath, "ssh")
		os.WriteFile(sshFile, []byte(""), 0o644)

		// Try to add SSH key if provided
		if config.Security.SSHKey != "" {
			// Try common SSH key locations
			for _, user := range []string{"root", "pi", "ubuntu"} {
				sshDir := filepath.Join(rootPath, "home", user, ".ssh")
				if err := os.MkdirAll(sshDir, 0o700); err == nil {
					authKeysPath := filepath.Join(sshDir, "authorized_keys")
					os.WriteFile(authKeysPath, []byte(config.Security.SSHKey), 0o600)
				}
			}
		}
	}

	// Write fleetd configuration to boot partition
	if err := p.writeFleetdConfig(bootPath, config); err != nil {
		return fmt.Errorf("failed to write fleetd config: %w", err)
	}

	return nil
}

func (p *CustomImageProvider) downloadChecksum() (string, error) {
	resp, err := http.Get(p.imageSHA256URL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("bad status: %s", resp.Status)
	}

	// Read checksum file
	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// Parse checksum (usually in format: "checksum  filename")
	parts := strings.Fields(string(content))
	if len(parts) > 0 {
		return parts[0], nil
	}

	return "", fmt.Errorf("invalid checksum format")
}

func (p *CustomImageProvider) writeFleetdConfig(bootPath string, config *Config) error {
	fleetdYAML := fmt.Sprintf(`# fleetd Agent Configuration
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
	return os.WriteFile(configPath, []byte(fleetdYAML), 0o644)
}

// calculateSHA256 calculates the SHA256 checksum of a file
func calculateSHA256(filepath string) (string, error) {
	file, err := os.Open(filepath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}
