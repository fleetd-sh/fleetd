package provision

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ValidatePath performs security validation on file paths
func ValidatePath(path string) error {
	if path == "" {
		return fmt.Errorf("path cannot be empty")
	}

	// Clean the path to remove any .., ., or multiple slashes
	cleanPath := filepath.Clean(path)

	// Check for path traversal attempts
	if strings.Contains(cleanPath, "..") {
		return fmt.Errorf("path traversal detected in path: %s", path)
	}

	// Ensure absolute path
	absPath, err := filepath.Abs(cleanPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Check if path contains suspicious patterns
	suspicious := []string{
		"/etc/passwd",
		"/etc/shadow",
		"/etc/sudoers",
		"/.ssh/",
		"/root/",
	}

	for _, pattern := range suspicious {
		if strings.Contains(absPath, pattern) {
			return fmt.Errorf("suspicious path pattern detected: %s", pattern)
		}
	}

	return nil
}

// ValidateDevicePath validates a device path for safety
func ValidateDevicePath(path string) error {
	if path == "" {
		return fmt.Errorf("device path cannot be empty")
	}

	if len(path) > 4096 { // MaxPathLength
		return fmt.Errorf("device path too long")
	}

	// Check for path traversal attempts
	if strings.Contains(path, "..") {
		return fmt.Errorf("path traversal detected in device path: %s", path)
	}

	// Device path validation patterns
	validDevicePaths := []string{
		`^/dev/disk\d+$`,         // macOS
		`^/dev/sd[a-z]+$`,        // Linux SCSI/SATA
		`^/dev/mmcblk\d+$`,       // Linux MMC/SD
		`^/dev/tty(USB|ACM)\d+$`, // Serial USB
		`^/dev/cu\..+$`,          // macOS serial
	}

	// Validate against known device patterns
	valid := false
	for _, pattern := range validDevicePaths {
		if matched, _ := regexp.MatchString(pattern, path); matched {
			valid = true
			break
		}
	}

	if !valid {
		return fmt.Errorf("unrecognized device path pattern: %s", path)
	}

	return nil
}

// ValidateImagePath validates an image file path
func ValidateImagePath(path string) error {
	if err := ValidatePath(path); err != nil {
		return fmt.Errorf("invalid image path: %w", err)
	}

	// Check if file exists
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("image file does not exist: %s", path)
		}
		return fmt.Errorf("failed to stat image file: %w", err)
	}

	// Check if it's a regular file
	if !info.Mode().IsRegular() {
		return fmt.Errorf("image path is not a regular file: %s", path)
	}

	// Check file size (prevent accidentally using wrong files)
	minSize := int64(100 * 1024 * 1024)       // 100MB minimum
	maxSize := int64(64 * 1024 * 1024 * 1024) // 64GB maximum

	if info.Size() < minSize {
		return fmt.Errorf("image file too small (%d bytes), minimum is %d bytes", info.Size(), minSize)
	}

	if info.Size() > maxSize {
		return fmt.Errorf("image file too large (%d bytes), maximum is %d bytes", info.Size(), maxSize)
	}

	return nil
}

// SanitizeFilename removes potentially dangerous characters from filenames
func SanitizeFilename(name string) string {
	// Remove path separators and other dangerous characters
	dangerous := []string{"/", "\\", "..", "~", "|", ";", "&", "$", "`", "\n", "\r"}
	result := name
	for _, char := range dangerous {
		result = strings.ReplaceAll(result, char, "_")
	}

	// Limit length
	const maxLen = 255
	if len(result) > maxLen {
		result = result[:maxLen]
	}

	return result
}
