package provision

import (
	"testing"
)

func TestDetectDeviceType(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		wantType DeviceType
		wantErr  bool
	}{
		// Block devices (Raspberry Pi)
		{"macOS disk", "/dev/disk2", DeviceTypeRaspberryPi, false},
		{"Linux SCSI", "/dev/sda", DeviceTypeRaspberryPi, false},
		{"Linux MMC", "/dev/mmcblk0", DeviceTypeRaspberryPi, false},

		// Serial ports (ESP32)
		{"USB serial", "/dev/ttyUSB0", DeviceTypeESP32, false},
		{"ACM serial", "/dev/ttyACM0", DeviceTypeESP32, false},
		{"macOS serial", "/dev/cu.usbserial", DeviceTypeESP32, false},

		// Unknown
		{"unknown device", "/dev/unknown", "", true},
		{"regular file", "/etc/passwd", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotType, err := DetectDeviceType(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("DetectDeviceType() error = %v, wantErr %v", err, tt.wantErr)
			}
			if gotType != tt.wantType {
				t.Errorf("DetectDeviceType() = %v, want %v", gotType, tt.wantType)
			}
		})
	}
}

func TestIsBlockDevice(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{"macOS disk", "/dev/disk2", true},
		{"Linux SCSI", "/dev/sda", true},
		{"Linux SCSI partition", "/dev/sdb1", true},
		{"Linux MMC", "/dev/mmcblk0", true},
		{"Linux MMC partition", "/dev/mmcblk0p1", true},
		{"serial port", "/dev/ttyUSB0", false},
		{"regular file", "/etc/passwd", false},
		{"empty", "", false},
		{"short path", "/dev", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isBlockDevice(tt.path); got != tt.want {
				t.Errorf("isBlockDevice() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsSerialPort(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{"USB serial", "/dev/ttyUSB0", true},
		{"ACM serial", "/dev/ttyACM0", true},
		{"regular tty", "/dev/tty0", true},
		{"macOS serial", "/dev/cu.usbserial", true},
		{"block device", "/dev/sda", false},
		{"regular file", "/etc/passwd", false},
		{"empty", "", false},
		{"short path", "/dev", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isSerialPort(tt.path); got != tt.want {
				t.Errorf("isSerialPort() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfigStructure(t *testing.T) {
	// Test that Config can be properly initialized
	config := &Config{
		DeviceType: DeviceTypeRaspberryPi,
		DevicePath: "/dev/disk2",
		DeviceName: "test-device",
		DeviceID:   "uuid-123",
		Network: NetworkConfig{
			WiFiSSID: "TestNetwork",
			WiFiPass: "password",
			Hostname: "test-host",
			StaticIP: "192.168.1.100",
		},
		Fleet: FleetConfig{
			ServerURL:    "https://fleet.example.com",
			Token:        "token-123",
			AutoRegister: true,
			Labels: map[string]string{
				"env":  "test",
				"type": "rpi",
			},
		},
		Security: SecurityConfig{
			EnableSSH: true,
			SSHKey:    "ssh-rsa AAAAB3...",
		},
		Plugins: map[string]any{
			"k3s": map[string]any{
				"role": "server",
			},
		},
	}

	// Verify all fields are set correctly
	if config.DeviceType != DeviceTypeRaspberryPi {
		t.Error("DeviceType not set correctly")
	}
	if config.DevicePath != "/dev/disk2" {
		t.Error("DevicePath not set correctly")
	}
	if config.Network.WiFiSSID != "TestNetwork" {
		t.Error("Network.WiFiSSID not set correctly")
	}
	if config.Fleet.ServerURL != "https://fleet.example.com" {
		t.Error("Fleet.ServerURL not set correctly")
	}
	if !config.Fleet.AutoRegister {
		t.Error("Fleet.AutoRegister not set correctly")
	}
	if config.Fleet.Labels["env"] != "test" {
		t.Error("Fleet.Labels not set correctly")
	}
	if !config.Security.EnableSSH {
		t.Error("Security.EnableSSH not set correctly")
	}
	if config.Plugins["k3s"] == nil {
		t.Error("Plugins not set correctly")
	}
}
