package provision

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Security validation constants
const (
	MaxDeviceNameLength = 63    // DNS hostname limit
	MaxPathLength       = 4096  // Linux PATH_MAX
	MaxSSHKeySize       = 16384 // 16KB max for SSH keys
	MaxTokenLength      = 512   // Reasonable limit for tokens
)

var (
	// ErrInvalidInput indicates invalid user input
	ErrInvalidInput = errors.New("invalid input")

	// ErrPathTraversal indicates a path traversal attempt
	ErrPathTraversal = errors.New("path traversal detected")

	// ErrInvalidHostname indicates an invalid hostname
	ErrInvalidHostname = errors.New("invalid hostname")

	// Hostname validation regex (RFC 1123)
	hostnameRegex = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?$`)
)

// ValidateHostname validates a hostname according to RFC 1123
func ValidateHostname(hostname string) error {
	if hostname == "" {
		return nil // Empty is allowed (will use default)
	}

	if len(hostname) > MaxDeviceNameLength {
		return fmt.Errorf("%w: hostname too long (max %d)", ErrInvalidHostname, MaxDeviceNameLength)
	}

	if !hostnameRegex.MatchString(hostname) {
		return fmt.Errorf("%w: must start/end with alphanumeric, contain only alphanumeric and hyphens", ErrInvalidHostname)
	}

	return nil
}

// ValidateSSHKey validates an SSH public key
func ValidateSSHKey(key string) error {
	if key == "" {
		return nil // Empty is allowed (no SSH)
	}

	if len(key) > MaxSSHKeySize {
		return fmt.Errorf("%w: SSH key too large", ErrInvalidInput)
	}

	// Basic SSH key format validation
	validPrefixes := []string{
		"ssh-rsa ",
		"ssh-ed25519 ",
		"ssh-ecdsa ",
		"ecdsa-sha2-nistp256 ",
		"ecdsa-sha2-nistp384 ",
		"ecdsa-sha2-nistp521 ",
	}

	hasValidPrefix := false
	for _, prefix := range validPrefixes {
		if strings.HasPrefix(key, prefix) {
			hasValidPrefix = true
			break
		}
	}

	if !hasValidPrefix {
		return fmt.Errorf("%w: invalid SSH key format", ErrInvalidInput)
	}

	// Check for basic structure (type key comment)
	parts := strings.Fields(key)
	if len(parts) < 2 {
		return fmt.Errorf("%w: SSH key missing required parts", ErrInvalidInput)
	}

	// Validate base64 encoding of the key
	_, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return fmt.Errorf("%w: SSH key contains invalid base64", ErrInvalidInput)
	}

	return nil
}

// ValidateWiFiSSID validates a WiFi SSID
func ValidateWiFiSSID(ssid string) error {
	if ssid == "" {
		return nil // Empty is allowed (no WiFi)
	}

	// SSID length: 1-32 bytes
	if len(ssid) > 32 {
		return fmt.Errorf("%w: WiFi SSID too long (max 32)", ErrInvalidInput)
	}

	// Check for control characters
	for _, r := range ssid {
		if r < 32 || r > 126 {
			return fmt.Errorf("%w: WiFi SSID contains invalid characters", ErrInvalidInput)
		}
	}

	return nil
}

// ValidateWiFiPassword validates a WiFi password
func ValidateWiFiPassword(password string) error {
	if password == "" {
		return nil // Empty is allowed (open network)
	}

	// WPA2 password: 8-63 characters
	if len(password) < 8 {
		return fmt.Errorf("%w: WiFi password too short (min 8)", ErrInvalidInput)
	}

	if len(password) > 63 {
		return fmt.Errorf("%w: WiFi password too long (max 63)", ErrInvalidInput)
	}

	return nil
}

// ValidateIPAddress validates an IP address
func ValidateIPAddress(ip string) error {
	if ip == "" {
		return nil // Empty is allowed
	}

	if net.ParseIP(ip) == nil {
		return fmt.Errorf("%w: invalid IP address", ErrInvalidInput)
	}

	return nil
}

// ValidateK3sToken validates a k3s token
func ValidateK3sToken(token string) error {
	if token == "" {
		return nil // Empty is allowed
	}

	if len(token) > MaxTokenLength {
		return fmt.Errorf("%w: token too long", ErrInvalidInput)
	}

	// K3s tokens are typically alphanumeric with some special chars
	// Don't be too restrictive but prevent obvious issues
	if strings.ContainsAny(token, "\n\r\t") {
		return fmt.Errorf("%w: token contains invalid characters", ErrInvalidInput)
	}

	return nil
}

// ValidateFilePath validates a file path for safety
func ValidateFilePath(path string) error {
	if path == "" {
		return fmt.Errorf("%w: empty file path", ErrInvalidInput)
	}

	if len(path) > MaxPathLength {
		return fmt.Errorf("%w: file path too long", ErrInvalidInput)
	}

	// Check for path traversal before cleaning
	if strings.Contains(path, "..") {
		return fmt.Errorf("%w: invalid file path", ErrPathTraversal)
	}

	// Clean the path for additional safety
	cleanPath := filepath.Clean(path)
	// After cleaning, check if it's trying to escape to parent directories
	if !filepath.IsAbs(cleanPath) && strings.HasPrefix(cleanPath, "..") {
		return fmt.Errorf("%w: invalid file path", ErrPathTraversal)
	}

	return nil
}

// SanitizeForTemplate escapes a string for safe use in templates
func SanitizeForTemplate(input string) string {
	// Escape shell metacharacters
	replacer := strings.NewReplacer(
		"`", "\\`",
		"$", "\\$",
		"\"", "\\\"",
		"\\", "\\\\",
		"\n", "\\n",
		"\r", "\\r",
		"\t", "\\t",
	)
	return replacer.Replace(input)
}

// GenerateSecureToken generates a cryptographically secure token
func GenerateSecureToken(length int) (string, error) {
	if length <= 0 || length > 256 {
		return "", fmt.Errorf("invalid token length")
	}

	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random token: %w", err)
	}

	return base64.URLEncoding.EncodeToString(bytes), nil
}

// ValidateConfig performs comprehensive validation on a Config
func ValidateConfig(config *Config) error {
	if config == nil {
		return fmt.Errorf("%w: nil config", ErrInvalidInput)
	}

	if err := ValidateDevicePath(config.DevicePath); err != nil {
		return fmt.Errorf("device path: %w", err)
	}

	if err := ValidateHostname(config.Network.Hostname); err != nil {
		return fmt.Errorf("hostname: %w", err)
	}

	if err := ValidateWiFiSSID(config.Network.WiFiSSID); err != nil {
		return fmt.Errorf("WiFi SSID: %w", err)
	}

	if err := ValidateWiFiPassword(config.Network.WiFiPass); err != nil {
		return fmt.Errorf("WiFi password: %w", err)
	}

	if err := ValidateIPAddress(config.Network.StaticIP); err != nil {
		return fmt.Errorf("static IP: %w", err)
	}

	if err := ValidateSSHKey(config.Security.SSHKey); err != nil {
		return fmt.Errorf("SSH key: %w", err)
	}

	// NOTE: Plugin validation would happen in the plugins themselves

	return nil
}

// SecureErase attempts to overwrite a file before deletion
func SecureErase(path string) error {
	file, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return err
	}

	// Overwrite with random data
	buffer := make([]byte, stat.Size())
	if _, err := rand.Read(buffer); err != nil {
		return err
	}

	if _, err := file.WriteAt(buffer, 0); err != nil {
		return err
	}

	if err := file.Sync(); err != nil {
		return err
	}

	return os.Remove(path)
}
