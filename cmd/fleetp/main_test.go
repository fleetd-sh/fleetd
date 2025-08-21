package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fleetd.sh/internal/provision"
)

func TestExpandPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("Cannot get user home directory")
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"home path", "~/test", filepath.Join(home, "test")},
		{"home path with subdirs", "~/foo/bar", filepath.Join(home, "foo/bar")},
		{"absolute path", "/tmp/test", "/tmp/test"},
		{"relative path", "test", "test"},
		{"current dir", ".", "."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := expandPath(tt.input)
			if result != tt.expected {
				t.Errorf("expandPath(%s) = %s, want %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestRunSimpleProvision(t *testing.T) {
	// Create a temporary SSH key file for testing
	tmpDir := t.TempDir()
	sshKeyFile := filepath.Join(tmpDir, "id_rsa.pub")
	sshKeyContent := "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDcthLR test@example.com"
	if err := os.WriteFile(sshKeyFile, []byte(sshKeyContent), 0600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		flags   *SimpleFlags
		wantErr bool
		errMsg  string
	}{
		{
			name: "minimal valid config",
			flags: &SimpleFlags{
				device:     "/dev/disk2",
				deviceType: "rpi",
			},
			wantErr: false,
		},
		{
			name: "with WiFi config",
			flags: &SimpleFlags{
				device:   "/dev/disk2",
				wifiSSID: "TestNetwork",
				wifiPass: "password123",
			},
			wantErr: false,
		},
		{
			name: "with SSH key",
			flags: &SimpleFlags{
				device:     "/dev/disk2",
				sshKeyFile: sshKeyFile,
			},
			wantErr: false,
		},
		{
			name: "with fleet server",
			flags: &SimpleFlags{
				device:      "/dev/disk2",
				fleetServer: "https://fleet.example.com",
			},
			wantErr: false,
		},
		{
			name: "with plugin options",
			flags: &SimpleFlags{
				device:     "/dev/disk2",
				plugins:    []string{"k3s"},
				pluginOpts: []string{"k3s.role=server", "k3s.token=secret"},
			},
			wantErr: false,
		},
		{
			name: "invalid plugin option format",
			flags: &SimpleFlags{
				device:     "/dev/disk2",
				pluginOpts: []string{"invalid-format"},
			},
			wantErr: true,
			errMsg:  "invalid plugin option",
		},
		{
			name: "invalid plugin option key format",
			flags: &SimpleFlags{
				device:     "/dev/disk2",
				pluginOpts: []string{"invalidkey=value"},
			},
			wantErr: true,
			errMsg:  "plugin.key=value",
		},
		{
			name: "non-existent SSH key file",
			flags: &SimpleFlags{
				device:     "/dev/disk2",
				sshKeyFile: "/non/existent/key.pub",
			},
			wantErr: true,
			errMsg:  "failed to read SSH key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Override verbose for cleaner test output
			tt.flags.verbose = false

			// Since we can't actually provision a device in tests,
			// we'll test the configuration setup part
			err := setupConfig(tt.flags)

			if (err != nil) != tt.wantErr {
				t.Errorf("setupConfig() error = %v, wantErr %v", err, tt.wantErr)
			}

			if err != nil && tt.errMsg != "" {
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("setupConfig() error = %v, want containing %v", err, tt.errMsg)
				}
			}
		})
	}
}

// Helper function to test configuration setup without actual provisioning
func setupConfig(flags *SimpleFlags) error {
	// Create config
	config := &provision.Config{
		DevicePath: flags.device,
		DeviceName: flags.name,
		DeviceID:   "test-uuid",
		Network: provision.NetworkConfig{
			WiFiSSID: flags.wifiSSID,
			WiFiPass: flags.wifiPass,
		},
		Fleet: provision.FleetConfig{
			ServerURL:    flags.fleetServer,
			AutoRegister: true,
		},
		Security: provision.SecurityConfig{
			EnableSSH: flags.sshKeyFile != "",
		},
	}

	// Auto-detect device type if not specified
	if flags.deviceType == "" {
		detectedType, err := provision.DetectDeviceType(flags.device)
		if err != nil {
			return err
		}
		config.DeviceType = detectedType
	} else {
		config.DeviceType = provision.DeviceType(flags.deviceType)
	}

	// Set device name if not specified
	if config.DeviceName == "" {
		config.DeviceName = "test-device"
	}

	// Load SSH key if specified
	if flags.sshKeyFile != "" {
		keyData, err := os.ReadFile(flags.sshKeyFile)
		if err != nil {
			return &parseError{msg: "failed to read SSH key: " + err.Error()}
		}
		config.Security.SSHKey = string(keyData)
	}

	// Parse plugin options
	if len(flags.pluginOpts) > 0 {
		if config.Plugins == nil {
			config.Plugins = make(map[string]any)
		}

		// Parse plugin options
		for _, opt := range flags.pluginOpts {
			parts := strings.SplitN(opt, "=", 2)
			if len(parts) != 2 {
				return &parseError{msg: "invalid plugin option: " + opt}
			}

			keyParts := strings.SplitN(parts[0], ".", 2)
			if len(keyParts) != 2 {
				return &parseError{msg: "plugin option must be in format plugin.key=value: " + opt}
			}

			plugin := keyParts[0]
			key := keyParts[1]
			value := parts[1]

			// Create plugin map if needed
			if _, ok := config.Plugins[plugin]; !ok {
				config.Plugins[plugin] = make(map[string]any)
			}

			// Set the option
			if pluginMap, ok := config.Plugins[plugin].(map[string]any); ok {
				pluginMap[key] = value
			}
		}
	}

	// Validate the configuration
	return provision.ValidateConfig(config)
}

type parseError struct {
	msg string
}

func (e *parseError) Error() string {
	return e.msg
}

func TestStringSliceFlag(t *testing.T) {
	var s stringSlice

	// Test initial state
	if s.String() != "" {
		t.Errorf("Initial String() = %s, want empty", s.String())
	}

	// Test adding values
	s.Set("value1")
	s.Set("value2")
	s.Set("value3")

	if len(s) != 3 {
		t.Errorf("Length = %d, want 3", len(s))
	}

	if s.String() != "value1,value2,value3" {
		t.Errorf("String() = %s, want value1,value2,value3", s.String())
	}

	// Test individual values
	if s[0] != "value1" {
		t.Errorf("s[0] = %s, want value1", s[0])
	}
	if s[1] != "value2" {
		t.Errorf("s[1] = %s, want value2", s[1])
	}
	if s[2] != "value3" {
		t.Errorf("s[2] = %s, want value3", s[2])
	}
}
