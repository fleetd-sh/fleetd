package models

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"
	"time"
)

// APIKey represents an API key for authentication
type APIKey struct {
	ID             string            `json:"id" db:"id"`
	Key            string            `json:"-" db:"key"`                 // The actual key (hashed)
	KeyPrefix      string            `json:"key_prefix" db:"key_prefix"` // First 8 chars for identification
	Name           string            `json:"name" db:"name"`
	Description    string            `json:"description" db:"description"`
	OrganizationID string            `json:"organization_id" db:"organization_id"`
	UserID         string            `json:"user_id" db:"user_id"`
	Scopes         []string          `json:"scopes" db:"scopes"`
	Metadata       map[string]string `json:"metadata" db:"metadata"`
	ExpiresAt      *time.Time        `json:"expires_at" db:"expires_at"`
	LastUsedAt     *time.Time        `json:"last_used_at" db:"last_used_at"`
	CreatedAt      time.Time         `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at" db:"updated_at"`
	RevokedAt      *time.Time        `json:"revoked_at" db:"revoked_at"`
	IsActive       bool              `json:"is_active" db:"is_active"`
}

// APIKeyScope defines the scopes an API key can have
type APIKeyScope string

const (
	// Read scopes
	APIKeyScopeReadDevices      APIKeyScope = "read:devices"
	APIKeyScopeReadFleets       APIKeyScope = "read:fleets"
	APIKeyScopeReadDeployments  APIKeyScope = "read:deployments"
	APIKeyScopeReadMetrics      APIKeyScope = "read:metrics"
	APIKeyScopeReadLogs         APIKeyScope = "read:logs"
	APIKeyScopeReadOrganization APIKeyScope = "read:organization"

	// Write scopes
	APIKeyScopeWriteDevices      APIKeyScope = "write:devices"
	APIKeyScopeWriteFleets       APIKeyScope = "write:fleets"
	APIKeyScopeWriteDeployments  APIKeyScope = "write:deployments"
	APIKeyScopeWriteOrganization APIKeyScope = "write:organization"

	// Admin scopes
	APIKeyScopeAdmin APIKeyScope = "admin"

	// Special scopes
	APIKeyScopeDeviceRegister  APIKeyScope = "device:register"  // For device self-registration
	APIKeyScopeDeviceTelemetry APIKeyScope = "device:telemetry" // For sending telemetry data
)

// GenerateAPIKey generates a new random API key
func GenerateAPIKey(prefix string) (string, error) {
	// Generate 32 random bytes
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	// Encode to base64 URL-safe
	key := base64.URLEncoding.EncodeToString(bytes)

	// Remove padding
	key = strings.TrimRight(key, "=")

	// Add prefix if provided
	if prefix != "" {
		return fmt.Sprintf("%s_%s", prefix, key), nil
	}

	return fmt.Sprintf("fld_%s", key), nil // Default prefix
}

// ExtractKeyPrefix extracts the first 8 characters of the key for identification
func ExtractKeyPrefix(key string) string {
	// Skip the prefix (e.g., "fld_")
	parts := strings.Split(key, "_")
	if len(parts) > 1 && len(parts[1]) >= 8 {
		return parts[1][:8]
	}
	if len(key) >= 8 {
		return key[:8]
	}
	return key
}

// ValidateAPIKeyFormat validates the format of an API key
func ValidateAPIKeyFormat(key string) error {
	// Check minimum length
	if len(key) < 20 {
		return fmt.Errorf("API key too short")
	}

	// Check for valid prefix
	validPrefixes := []string{"fld_", "fldk_", "flds_"} // fleetd, fleetd-key, fleetd-secret
	hasValidPrefix := false
	for _, prefix := range validPrefixes {
		if strings.HasPrefix(key, prefix) {
			hasValidPrefix = true
			break
		}
	}

	if !hasValidPrefix {
		return fmt.Errorf("invalid API key prefix")
	}

	// Check that the key part contains only valid characters
	parts := strings.Split(key, "_")
	if len(parts) < 2 {
		return fmt.Errorf("invalid API key format")
	}

	keyPart := parts[1]
	for _, c := range keyPart {
		if !isValidKeyChar(c) {
			return fmt.Errorf("API key contains invalid characters")
		}
	}

	return nil
}

// isValidKeyChar checks if a character is valid for an API key
func isValidKeyChar(c rune) bool {
	return (c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') ||
		c == '-' || c == '_'
}

// IsExpired checks if the API key has expired
func (k *APIKey) IsExpired() bool {
	if k.ExpiresAt == nil {
		return false // No expiration
	}
	return time.Now().After(*k.ExpiresAt)
}

// IsRevoked checks if the API key has been revoked
func (k *APIKey) IsRevoked() bool {
	return k.RevokedAt != nil
}

// IsValid checks if the API key is valid for use
func (k *APIKey) IsValid() bool {
	return k.IsActive && !k.IsExpired() && !k.IsRevoked()
}

// HasScope checks if the API key has a specific scope
func (k *APIKey) HasScope(scope APIKeyScope) bool {
	for _, s := range k.Scopes {
		if s == string(scope) || s == string(APIKeyScopeAdmin) {
			return true
		}
	}
	return false
}

// HasAnyScope checks if the API key has any of the specified scopes
func (k *APIKey) HasAnyScope(scopes ...APIKeyScope) bool {
	for _, scope := range scopes {
		if k.HasScope(scope) {
			return true
		}
	}
	return false
}

// HasAllScopes checks if the API key has all of the specified scopes
func (k *APIKey) HasAllScopes(scopes ...APIKeyScope) bool {
	for _, scope := range scopes {
		if !k.HasScope(scope) {
			return false
		}
	}
	return true
}

// UpdateLastUsed updates the last used timestamp
func (k *APIKey) UpdateLastUsed() {
	now := time.Now()
	k.LastUsedAt = &now
}
