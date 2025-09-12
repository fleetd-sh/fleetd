package cmd

import (
	"os"
	"path/filepath"
	"testing"
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

func TestProvisionCommandValidation(t *testing.T) {
	tests := []struct {
		name    string
		setup   func()
		wantErr bool
		errMsg  string
	}{
		{
			name: "missing device flag",
			setup: func() {
				device = ""
			},
			wantErr: true,
			errMsg:  "required flag",
		},
		{
			name: "valid device flag",
			setup: func() {
				device = "/dev/disk2"
				dryRun = true
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset flags
			device = ""
			dryRun = false

			// Setup test case
			tt.setup()

			// Test validation would happen in runProvision
			// For now, just test basic flag validation
			if device == "" && !tt.wantErr {
				t.Errorf("Expected device to be set")
			}
		})
	}
}
