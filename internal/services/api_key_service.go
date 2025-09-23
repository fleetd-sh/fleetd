package services

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	"fleetd.sh/internal/models"
	"fleetd.sh/internal/security"
)

// APIKeyService handles API key operations
type APIKeyService struct {
	db *sql.DB
}

// NewAPIKeyService creates a new API key service
func NewAPIKeyService(db *sql.DB) *APIKeyService {
	return &APIKeyService{
		db: db,
	}
}

// CreateAPIKey creates a new API key
func (s *APIKeyService) CreateAPIKey(ctx context.Context, req *CreateAPIKeyRequest) (*CreateAPIKeyResponse, error) {
	// Generate new API key
	rawKey, err := models.GenerateAPIKey("fld")
	if err != nil {
		return nil, fmt.Errorf("failed to generate API key: %w", err)
	}

	// Hash the key for storage
	hashedKey := s.hashAPIKey(rawKey)

	// Extract prefix for identification
	keyPrefix := models.ExtractKeyPrefix(rawKey)

	// Create API key record
	apiKey := &models.APIKey{
		ID:             generateID("key"),
		Key:            hashedKey,
		KeyPrefix:      keyPrefix,
		Name:           req.Name,
		Description:    req.Description,
		OrganizationID: req.OrganizationID,
		UserID:         req.UserID,
		Scopes:         req.Scopes,
		Metadata:       req.Metadata,
		ExpiresAt:      req.ExpiresAt,
		IsActive:       true,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	// Store in database
	query := `
		INSERT INTO api_keys (
			id, key, key_prefix, name, description,
			organization_id, user_id, scopes, metadata,
			expires_at, is_active, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
		)`

	_, err = s.db.ExecContext(ctx, query,
		apiKey.ID, apiKey.Key, apiKey.KeyPrefix, apiKey.Name, apiKey.Description,
		apiKey.OrganizationID, apiKey.UserID, apiKey.Scopes, apiKey.Metadata,
		apiKey.ExpiresAt, apiKey.IsActive, apiKey.CreatedAt, apiKey.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to store API key: %w", err)
	}

	return &CreateAPIKeyResponse{
		APIKey: apiKey,
		RawKey: rawKey, // Return raw key only on creation
	}, nil
}

// ValidateAPIKey validates an API key and returns the associated metadata
func (s *APIKeyService) ValidateAPIKey(ctx context.Context, rawKey string) (*models.APIKey, error) {
	// Validate format
	if err := models.ValidateAPIKeyFormat(rawKey); err != nil {
		return nil, fmt.Errorf("invalid API key format: %w", err)
	}

	// Hash the provided key
	hashedKey := s.hashAPIKey(rawKey)

	// Look up the key in database
	query := `
		SELECT
			id, key, key_prefix, name, description,
			organization_id, user_id, scopes, metadata,
			expires_at, last_used_at, created_at, updated_at,
			revoked_at, is_active
		FROM api_keys
		WHERE key = $1`

	var apiKey models.APIKey
	err := s.db.QueryRowContext(ctx, query, hashedKey).Scan(
		&apiKey.ID, &apiKey.Key, &apiKey.KeyPrefix, &apiKey.Name, &apiKey.Description,
		&apiKey.OrganizationID, &apiKey.UserID, &apiKey.Scopes, &apiKey.Metadata,
		&apiKey.ExpiresAt, &apiKey.LastUsedAt, &apiKey.CreatedAt, &apiKey.UpdatedAt,
		&apiKey.RevokedAt, &apiKey.IsActive,
	)

	if err == sql.ErrNoRows {
		return nil, ErrInvalidAPIKey
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query API key: %w", err)
	}

	// Check if key is valid
	if !apiKey.IsValid() {
		if apiKey.IsExpired() {
			return nil, ErrAPIKeyExpired
		}
		if apiKey.IsRevoked() {
			return nil, ErrAPIKeyRevoked
		}
		if !apiKey.IsActive {
			return nil, ErrAPIKeyInactive
		}
		return nil, ErrInvalidAPIKey
	}

	// Update last used timestamp (async)
	go s.updateLastUsed(context.Background(), apiKey.ID)

	return &apiKey, nil
}

// ValidateAPIKeyWithClaims validates an API key and returns JWT-style claims
func (s *APIKeyService) ValidateAPIKeyWithClaims(ctx context.Context, rawKey string) (*security.Claims, error) {
	apiKey, err := s.ValidateAPIKey(ctx, rawKey)
	if err != nil {
		return nil, err
	}

	// Convert API key to claims
	claims := &security.Claims{
		UserID:      apiKey.UserID,
		Username:    apiKey.Name, // Use key name as username
		Email:       "",          // API keys don't have emails
		Roles:       scopesToRoles(apiKey.Scopes),
		Permissions: scopesToPermissions(apiKey.Scopes),
		TokenType:   security.TokenTypeAccess,
	}

	// Note: Organization ID could be added to claims metadata if Claims struct had a Metadata field
	// For now, we'll rely on the UserID field to track the API key owner

	return claims, nil
}

// RevokeAPIKey revokes an API key
func (s *APIKeyService) RevokeAPIKey(ctx context.Context, keyID string) error {
	query := `
		UPDATE api_keys
		SET revoked_at = $1, is_active = false, updated_at = $2
		WHERE id = $3`

	now := time.Now()
	result, err := s.db.ExecContext(ctx, query, now, now, keyID)
	if err != nil {
		return fmt.Errorf("failed to revoke API key: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return ErrAPIKeyNotFound
	}

	return nil
}

// ListAPIKeys lists API keys for an organization or user
func (s *APIKeyService) ListAPIKeys(ctx context.Context, filter *APIKeyFilter) ([]*models.APIKey, error) {
	query := `
		SELECT
			id, key_prefix, name, description,
			organization_id, user_id, scopes, metadata,
			expires_at, last_used_at, created_at, updated_at,
			revoked_at, is_active
		FROM api_keys
		WHERE 1=1`

	args := []interface{}{}
	argCount := 0

	if filter.OrganizationID != "" {
		argCount++
		query += fmt.Sprintf(" AND organization_id = $%d", argCount)
		args = append(args, filter.OrganizationID)
	}

	if filter.UserID != "" {
		argCount++
		query += fmt.Sprintf(" AND user_id = $%d", argCount)
		args = append(args, filter.UserID)
	}

	if filter.OnlyActive {
		query += " AND is_active = true AND revoked_at IS NULL"
		query += " AND (expires_at IS NULL OR expires_at > NOW())"
	}

	query += " ORDER BY created_at DESC"

	if filter.Limit > 0 {
		argCount++
		query += fmt.Sprintf(" LIMIT $%d", argCount)
		args = append(args, filter.Limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list API keys: %w", err)
	}
	defer rows.Close()

	var apiKeys []*models.APIKey
	for rows.Next() {
		var apiKey models.APIKey
		err := rows.Scan(
			&apiKey.ID, &apiKey.KeyPrefix, &apiKey.Name, &apiKey.Description,
			&apiKey.OrganizationID, &apiKey.UserID, &apiKey.Scopes, &apiKey.Metadata,
			&apiKey.ExpiresAt, &apiKey.LastUsedAt, &apiKey.CreatedAt, &apiKey.UpdatedAt,
			&apiKey.RevokedAt, &apiKey.IsActive,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan API key: %w", err)
		}
		apiKeys = append(apiKeys, &apiKey)
	}

	return apiKeys, nil
}

// hashAPIKey hashes an API key using SHA-256
func (s *APIKeyService) hashAPIKey(rawKey string) string {
	hash := sha256.Sum256([]byte(rawKey))
	return hex.EncodeToString(hash[:])
}

// updateLastUsed updates the last used timestamp for an API key
func (s *APIKeyService) updateLastUsed(ctx context.Context, keyID string) {
	query := `UPDATE api_keys SET last_used_at = $1 WHERE id = $2`
	s.db.ExecContext(ctx, query, time.Now(), keyID)
}

// scopesToPermissions converts API key scopes to permissions
func scopesToPermissions(scopes []string) []security.Permission {
	permissions := make([]security.Permission, 0, len(scopes))

	for _, scope := range scopes {
		switch scope {
		case string(models.APIKeyScopeAdmin):
			// Admin has all permissions
			permissions = append(permissions,
				security.PermissionDeviceList, security.PermissionDeviceView,
				security.PermissionDeviceCreate, security.PermissionDeviceUpdate,
				security.PermissionDeviceDelete, security.PermissionDeviceRegister,
				security.PermissionUpdateList, security.PermissionUpdateView,
				security.PermissionUpdateCreate, security.PermissionAnalyticsView)
		case string(models.APIKeyScopeReadDevices):
			permissions = append(permissions, security.PermissionDeviceList, security.PermissionDeviceView)
		case string(models.APIKeyScopeWriteDevices):
			permissions = append(permissions, security.PermissionDeviceCreate, security.PermissionDeviceUpdate)
		case string(models.APIKeyScopeReadFleets):
			// Use device permissions for fleet operations for now
			permissions = append(permissions, security.PermissionDeviceList, security.PermissionDeviceView)
		case string(models.APIKeyScopeWriteFleets):
			// Use device permissions for fleet operations for now
			permissions = append(permissions, security.PermissionDeviceCreate, security.PermissionDeviceUpdate)
		case string(models.APIKeyScopeDeviceRegister):
			permissions = append(permissions, security.PermissionDeviceRegister)
		case string(models.APIKeyScopeDeviceTelemetry):
			// Use analytics view for telemetry
			permissions = append(permissions, security.PermissionAnalyticsView)
		}
	}

	return permissions
}

// scopesToRoles converts API key scopes to roles
func scopesToRoles(scopes []string) []security.Role {
	roleMap := make(map[security.Role]bool)

	for _, scope := range scopes {
		switch scope {
		case string(models.APIKeyScopeAdmin):
			roleMap[security.RoleAdmin] = true
		case string(models.APIKeyScopeWriteDevices),
			string(models.APIKeyScopeWriteFleets),
			string(models.APIKeyScopeWriteDeployments):
			roleMap[security.RoleOperator] = true
		case string(models.APIKeyScopeReadDevices),
			string(models.APIKeyScopeReadFleets),
			string(models.APIKeyScopeReadMetrics):
			roleMap[security.RoleViewer] = true
		case string(models.APIKeyScopeDeviceRegister),
			string(models.APIKeyScopeDeviceTelemetry):
			roleMap[security.RoleDevice] = true
		}
	}

	roles := []security.Role{}
	for role := range roleMap {
		roles = append(roles, role)
	}

	return roles
}

// generateID generates a unique ID with a prefix
func generateID(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
}

// Request/Response types

type CreateAPIKeyRequest struct {
	Name           string            `json:"name"`
	Description    string            `json:"description"`
	OrganizationID string            `json:"organization_id"`
	UserID         string            `json:"user_id"`
	Scopes         []string          `json:"scopes"`
	Metadata       map[string]string `json:"metadata"`
	ExpiresAt      *time.Time        `json:"expires_at"`
}

type CreateAPIKeyResponse struct {
	APIKey *models.APIKey `json:"api_key"`
	RawKey string         `json:"raw_key"` // Only returned on creation
}

type APIKeyFilter struct {
	OrganizationID string
	UserID         string
	OnlyActive     bool
	Limit          int
}

// Errors

var (
	ErrInvalidAPIKey  = fmt.Errorf("invalid API key")
	ErrAPIKeyExpired  = fmt.Errorf("API key has expired")
	ErrAPIKeyRevoked  = fmt.Errorf("API key has been revoked")
	ErrAPIKeyInactive = fmt.Errorf("API key is inactive")
	ErrAPIKeyNotFound = fmt.Errorf("API key not found")
)
