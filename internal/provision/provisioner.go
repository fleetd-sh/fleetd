package provision

import (
	"context"
	"fmt"
)

// ProgressReporter provides methods for reporting provisioning progress
type ProgressReporter interface {
	UpdateStatus(status string)
	UpdateProgress(message string, current, total int64)
}

// DeviceType represents the type of device being provisioned
type DeviceType string

const (
	DeviceTypeRaspberryPi DeviceType = "rpi"
	DeviceTypeESP32       DeviceType = "esp32"
)

// Config is the provisioning configuration focused on fleetd
type Config struct {
	// Device identification
	DeviceType DeviceType `json:"device_type"`
	DevicePath string     `json:"device_path"`
	DeviceName string     `json:"device_name"`
	DeviceID   string     `json:"device_id"`

	// Target platform/architecture for plugin compatibility
	TargetPlatform string `json:"target_platform,omitempty"` // e.g., "linux", "rtos"
	TargetArch     string `json:"target_arch,omitempty"`     // e.g., "arm64", "arm", "amd64", "xtensa"

	// Network configuration (essential for connectivity)
	Network NetworkConfig `json:"network"`

	// Fleet configuration (core purpose)
	Fleet FleetConfig `json:"fleet"`

	// Security (basic access)
	Security SecurityConfig `json:"security"`

	// Plugin data for extensions
	Plugins map[string]any `json:"plugins,omitempty"`
}

// NetworkConfig contains network settings
type NetworkConfig struct {
	WiFiSSID string `json:"wifi_ssid,omitempty"`
	WiFiPass string `json:"wifi_pass,omitempty"`
	Hostname string `json:"hostname"`
	StaticIP string `json:"static_ip,omitempty"`
}

// FleetConfig contains fleet management settings
type FleetConfig struct {
	ServerURL    string            `json:"server_url,omitempty"` // Empty = use mDNS
	Token        string            `json:"token,omitempty"`
	AutoRegister bool              `json:"auto_register"`
	Labels       map[string]string `json:"labels,omitempty"`
}

// SecurityConfig contains security settings
type SecurityConfig struct {
	EnableSSH bool   `json:"enable_ssh"`
	SSHKey    string `json:"ssh_key,omitempty"`
}

// Provisioner is the interface for device provisioning
type Provisioner interface {
	// Provision performs the device provisioning
	Provision(ctx context.Context) error

	// Validate checks if the configuration is valid
	Validate() error

	// Cleanup performs any necessary cleanup
	Cleanup() error
}

// DetectDeviceType attempts to detect the device type from the path
func DetectDeviceType(path string) (DeviceType, error) {
	// Check if it's a disk image file (for QEMU testing)
	if isDiskImage(path) {
		return DeviceTypeRaspberryPi, nil
	}

	// Check if it's a block device (SD card for RPi)
	if isBlockDevice(path) {
		return DeviceTypeRaspberryPi, nil
	}

	// Check if it's a serial port (ESP32)
	if isSerialPort(path) {
		return DeviceTypeESP32, nil
	}

	return "", fmt.Errorf("unable to detect device type for path: %s", path)
}

// Helper functions
func isBlockDevice(path string) bool {
	if len(path) < 6 {
		return false
	}

	// Check for different block device patterns
	if len(path) >= 9 && path[:9] == "/dev/disk" {
		return true
	}
	if len(path) >= 7 && path[:7] == "/dev/sd" {
		return true
	}
	if len(path) >= 11 && path[:11] == "/dev/mmcblk" {
		return true
	}

	return false
}

func isSerialPort(path string) bool {
	if len(path) < 7 {
		return false
	}

	// Check for different serial port patterns
	if len(path) >= 8 && path[:8] == "/dev/tty" {
		return true
	}
	if len(path) >= 7 && path[:7] == "/dev/cu" {
		return true
	}

	return false
}

func isDiskImage(path string) bool {
	// Check if it's a disk image file (for QEMU testing)
	if len(path) < 4 {
		return false
	}

	// Check for common disk image extensions
	extensions := []string{".img", ".qcow2", ".raw", ".vdi", ".vmdk"}
	for _, ext := range extensions {
		if len(path) >= len(ext) && path[len(path)-len(ext):] == ext {
			return true
		}
	}

	return false
}
