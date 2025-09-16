package provision

import (
	"strings"
	"testing"
)

func TestValidateDevicePath(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		wantErr     bool
		errContains string
	}{
		// Valid paths
		{"valid macOS disk", "/dev/disk2", false, ""},
		{"valid Linux SCSI", "/dev/sda", false, ""},
		{"valid Linux SCSI partition", "/dev/sdb", false, ""},
		{"valid Linux MMC", "/dev/mmcblk0", false, ""},
		{"valid USB serial", "/dev/ttyUSB0", false, ""},
		{"valid ACM serial", "/dev/ttyACM1", false, ""},
		{"valid macOS serial", "/dev/cu.usbserial", false, ""},

		// Invalid paths
		{"empty path", "", true, "device path cannot be empty"},
		{"path traversal", "/dev/../etc/passwd", true, "path traversal"},
		{"path traversal hidden", "/dev/disk2/../../../etc/passwd", true, "path traversal"},
		{"invalid pattern", "/etc/passwd", true, "unrecognized device path pattern"},
		{"invalid pattern proc", "/proc/1/mem", true, "unrecognized device path pattern"},
		{"too long path", "/" + strings.Repeat("a", MaxPathLength), true, "device path too long"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDevicePath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateDevicePath() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.errContains != "" && err != nil {
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("ValidateDevicePath() error = %v, want error containing %v", err, tt.errContains)
				}
			}
		})
	}
}

func TestValidateHostname(t *testing.T) {
	tests := []struct {
		name     string
		hostname string
		wantErr  bool
	}{
		// Valid hostnames
		{"empty allowed", "", false},
		{"simple hostname", "device1", false},
		{"hostname with hyphen", "my-device", false},
		{"hostname with numbers", "rpi4node1", false},
		{"max length hostname", strings.Repeat("a", MaxDeviceNameLength), false},

		// Invalid hostnames
		{"too long", strings.Repeat("a", MaxDeviceNameLength+1), true},
		{"starts with hyphen", "-device", true},
		{"ends with hyphen", "device-", true},
		{"contains underscore", "my_device", true},
		{"contains space", "my device", true},
		{"contains special char", "device@home", true},
		{"contains dot", "device.local", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateHostname(tt.hostname)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateHostname() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateSSHKey(t *testing.T) {
	// Real SSH keys generated for testing
	validRSAKey := "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDHJtUV6ixpRgOaEdhuzOomZXdo30d/CIe33yCvLaRIzzq/nPPfW80JHQkmm3h2yDgo6OI8Vm2ml/zKIq/sNSrsVXSmT6yVaPE6aipqUa/d4gDn8suzT/NI83jYp+AMmMPb1EqKXWXHQIsAEbgRFYE04J6w0TvHRrd6AmOFym9yqwuFXgSBeK/baAfgCUnfMpr3ngdoiD/m8OP9ZG7SjEriM5FpdQhmgTDZuFY6dFBcLxTc8Q5E3f6vRKqOvp0ttbDX2VFWm70pfmFanT0VuT9Iq0HbOHI5B+tJlBaUPuZBJ24lVSAbyfY3cI7JW8g9hHW+9zoTTom3V5vXuujP/BVP user@host"
	validEd25519Key := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIE2dl5QlpA4zDALiC9PGhECygSZyRQtt/ed4xCRZBpQp user@host"
	validECDSAKey := "ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTYAAAAIbmlzdHAyNTYAAABBBB1Z0+6rSD+IdLlug4rVRC4vy/XJZiA7UGbpe2oDfO6ijUSMqlcvwEtloIAUD4ZI4U1zeZEzreLQLrH1ufS1LtA= user@host"

	tests := []struct {
		name    string
		key     string
		wantErr bool
	}{
		// Valid keys
		{"empty allowed", "", false},
		{"valid RSA key", validRSAKey, false},
		{"valid Ed25519 key", validEd25519Key, false},
		{"valid ECDSA key", validECDSAKey, false},

		// Invalid keys
		{"invalid prefix", "ssh-dss AAAAB3NzaC1kc3M user@host", true},
		{"missing parts", "ssh-rsa", true},
		{"invalid base64", "ssh-rsa not-base64 user@host", true},
		{"too large", "ssh-rsa " + strings.Repeat("A", MaxSSHKeySize), true},
		{"no prefix", "AAAAB3NzaC1yc2E user@host", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSSHKey(tt.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateSSHKey() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateWiFiSSID(t *testing.T) {
	tests := []struct {
		name    string
		ssid    string
		wantErr bool
	}{
		// Valid SSIDs
		{"empty allowed", "", false},
		{"simple SSID", "MyNetwork", false},
		{"SSID with spaces", "My Home Network", false},
		{"max length SSID", strings.Repeat("a", 32), false},
		{"SSID with numbers", "Network2024", false},
		{"SSID with special chars", "Net-Work_123!", false},

		// Invalid SSIDs
		{"too long", strings.Repeat("a", 33), true},
		{"control character", "Network\x00", true},
		{"newline character", "Network\n", true},
		{"tab character", "Network\t", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateWiFiSSID(tt.ssid)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateWiFiSSID() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateWiFiPassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  bool
	}{
		// Valid passwords
		{"empty allowed", "", false},
		{"minimum length", "12345678", false},
		{"typical password", "MySecurePassword123", false},
		{"max length password", strings.Repeat("a", 63), false},
		{"special characters", "P@ssw0rd!#$%", false},

		// Invalid passwords
		{"too short", "1234567", true},
		{"too long", strings.Repeat("a", 64), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateWiFiPassword(tt.password)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateWiFiPassword() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateIPAddress(t *testing.T) {
	tests := []struct {
		name    string
		ip      string
		wantErr bool
	}{
		// Valid IPs
		{"empty allowed", "", false},
		{"valid IPv4", "192.168.1.1", false},
		{"valid IPv4 localhost", "127.0.0.1", false},
		{"valid IPv6", "2001:db8::1", false},
		{"valid IPv6 localhost", "::1", false},
		{"valid IPv6 full", "2001:0db8:0000:0000:0000:0000:0000:0001", false},

		// Invalid IPs
		{"invalid IPv4", "256.256.256.256", true},
		{"invalid format", "192.168.1", true},
		{"not an IP", "hostname", true},
		{"with port", "192.168.1.1:8080", true},
		{"with CIDR", "192.168.1.0/24", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateIPAddress(tt.ip)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateIPAddress() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateK3sToken(t *testing.T) {
	tests := []struct {
		name    string
		token   string
		wantErr bool
	}{
		// Valid tokens
		{"empty allowed", "", false},
		{"typical token", "K10abc123def456ghi789jkl", false},
		{"token with special chars", "K10abc-123_def.456", false},
		{"long token", strings.Repeat("a", MaxTokenLength), false},

		// Invalid tokens
		{"too long", strings.Repeat("a", MaxTokenLength+1), true},
		{"contains newline", "token\nwith\nnewline", true},
		{"contains tab", "token\twith\ttab", true},
		{"contains carriage return", "token\rwith\rCR", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateK3sToken(tt.token)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateK3sToken() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateFilePath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
		errType error
	}{
		// Valid paths
		{"absolute path", "/etc/config.yaml", false, nil},
		{"nested path", "/var/lib/fleetd/data.db", false, nil},
		{"home path", "/home/user/file.txt", false, nil},

		// Invalid paths
		{"empty path", "", true, ErrInvalidInput},
		{"path traversal", "/etc/../../../etc/passwd", true, ErrPathTraversal},
		{"path traversal relative", "../../../etc/passwd", true, ErrPathTraversal},
		{"too long", "/" + strings.Repeat("a", MaxPathLength), true, ErrInvalidInput},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFilePath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateFilePath() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.errType != nil && err != nil {
				if !strings.Contains(err.Error(), tt.errType.Error()) {
					t.Errorf("ValidateFilePath() error type = %v, want %v", err, tt.errType)
				}
			}
		})
	}
}

func TestSanitizeForTemplate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple text", "hello world", "hello world"},
		{"backticks", "echo `date`", "echo \\`date\\`"},
		{"dollar signs", "echo $USER", "echo \\$USER"},
		{"quotes", `echo "hello"`, `echo \"hello\"`},
		{"backslashes", `path\to\file`, `path\\to\\file`},
		{"newlines", "line1\nline2", "line1\\nline2"},
		{"tabs", "col1\tcol2", "col1\\tcol2"},
		{"carriage returns", "text\rmore", "text\\rmore"},
		{"mixed special", `$USER said "hello\nworld"`, `\$USER said \"hello\\nworld\"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeForTemplate(tt.input)
			if result != tt.expected {
				t.Errorf("SanitizeForTemplate() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestGenerateSecureToken(t *testing.T) {
	// Test valid lengths
	lengths := []int{8, 16, 32, 64, 128, 256}
	for _, length := range lengths {
		token, err := GenerateSecureToken(length)
		if err != nil {
			t.Errorf("GenerateSecureToken(%d) error = %v", length, err)
			continue
		}

		// Check that token is base64
		if len(token) == 0 {
			t.Errorf("GenerateSecureToken(%d) returned empty token", length)
		}

		// Tokens should be different
		token2, _ := GenerateSecureToken(length)
		if token == token2 {
			t.Errorf("GenerateSecureToken(%d) returned same token twice", length)
		}
	}

	// Test invalid lengths
	invalidLengths := []int{0, -1, 257, 1000}
	for _, length := range invalidLengths {
		_, err := GenerateSecureToken(length)
		if err == nil {
			t.Errorf("GenerateSecureToken(%d) should have returned error", length)
		}
	}
}

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
			errMsg:  "nil config",
		},
		{
			name: "valid minimal config",
			config: &Config{
				DevicePath: "/dev/disk2",
			},
			wantErr: false,
		},
		{
			name: "valid full config",
			config: &Config{
				DevicePath: "/dev/disk2",
				Network: NetworkConfig{
					Hostname: "device1",
					WiFiSSID: "MyNetwork",
					WiFiPass: "password123",
					StaticIP: "192.168.1.100",
				},
				Security: SecurityConfig{
					SSHKey: "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDHJtUV6ixpRgOaEdhuzOomZXdo30d/CIe33yCvLaRIzzq/nPPfW80JHQkmm3h2yDgo6OI8Vm2ml/zKIq/sNSrsVXSmT6yVaPE6aipqUa/d4gDn8suzT/NI83jYp+AMmMPb1EqKXWXHQIsAEbgRFYE04J6w0TvHRrd6AmOFym9yqwuFXgSBeK/baAfgCUnfMpr3ngdoiD/m8OP9ZG7SjEriM5FpdQhmgTDZuFY6dFBcLxTc8Q5E3f6vRKqOvp0ttbDX2VFWm70pfmFanT0VuT9Iq0HbOHI5B+tJlBaUPuZBJ24lVSAbyfY3cI7JW8g9hHW+9zoTTom3V5vXuujP/BVP user@host",
				},
			},
			wantErr: false,
		},
		{
			name: "invalid device path",
			config: &Config{
				DevicePath: "/etc/passwd",
			},
			wantErr: true,
			errMsg:  "device path",
		},
		{
			name: "invalid hostname",
			config: &Config{
				DevicePath: "/dev/disk2",
				Network: NetworkConfig{
					Hostname: "-invalid-",
				},
			},
			wantErr: true,
			errMsg:  "hostname",
		},
		{
			name: "invalid WiFi SSID",
			config: &Config{
				DevicePath: "/dev/disk2",
				Network: NetworkConfig{
					WiFiSSID: strings.Repeat("a", 33),
				},
			},
			wantErr: true,
			errMsg:  "WiFi SSID",
		},
		{
			name: "invalid WiFi password",
			config: &Config{
				DevicePath: "/dev/disk2",
				Network: NetworkConfig{
					WiFiPass: "short",
				},
			},
			wantErr: true,
			errMsg:  "WiFi password",
		},
		{
			name: "invalid IP",
			config: &Config{
				DevicePath: "/dev/disk2",
				Network: NetworkConfig{
					StaticIP: "999.999.999.999",
				},
			},
			wantErr: true,
			errMsg:  "static IP",
		},
		{
			name: "invalid SSH key",
			config: &Config{
				DevicePath: "/dev/disk2",
				Security: SecurityConfig{
					SSHKey: "not-a-valid-key",
				},
			},
			wantErr: true,
			errMsg:  "SSH key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateConfig(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errMsg != "" {
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("ValidateConfig() error message = %v, want containing %v", err, tt.errMsg)
				}
			}
		})
	}
}
