package auth

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// DeviceAuthRequest represents a device authorization request
type DeviceAuthRequest struct {
	ID              string    `json:"id"`
	DeviceCode      string    `json:"device_code"`
	UserCode        string    `json:"user_code"`
	VerificationURL string    `json:"verification_url"`
	ExpiresAt       time.Time `json:"expires_at"`
	Interval        int       `json:"interval"`
	ClientID        string    `json:"client_id"`
	ClientName      string    `json:"client_name"`
	UserID          *string   `json:"user_id,omitempty"`
	ApprovedAt      *time.Time `json:"approved_at,omitempty"`
}

// DeviceAuthService handles device flow authentication
type DeviceAuthService struct {
	db *sql.DB
}

// DeviceFlow is an alias for DeviceAuthService for backward compatibility
type DeviceFlow = DeviceAuthService

// TokenResponse represents the response from token exchange
type TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
	Scope       string `json:"scope,omitempty"`
}

// DeviceAuthResponse represents the initial device auth response
type DeviceAuthResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURL         string `json:"verification_uri"`
	VerificationURLComplete string `json:"verification_uri_complete,omitempty"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

// NewDeviceAuthService creates a new device auth service
func NewDeviceAuthService(db *sql.DB) *DeviceAuthService {
	return &DeviceAuthService{db: db}
}

// NewDeviceFlow creates a new device flow (alias for NewDeviceAuthService)
func NewDeviceFlow(db *sql.DB) *DeviceFlow {
	return NewDeviceAuthService(db)
}

// CreateDeviceAuth creates a new device authorization request
func (s *DeviceAuthService) CreateDeviceAuth(clientID string, scope string) (*DeviceAuthResponse, error) {
	deviceCode := generateDeviceCode()
	userCode := generateUserCode()
	expiresAt := time.Now().Add(15 * time.Minute)

	authReq := &DeviceAuthRequest{
		ID:              uuid.New().String(),
		DeviceCode:      deviceCode,
		UserCode:        userCode,
		VerificationURL: "https://localhost:3000/auth/device",
		ExpiresAt:       expiresAt,
		Interval:        5,
		ClientID:        clientID,
		ClientName:      getClientName(clientID),
	}

	query := `
		INSERT INTO device_auth_request
		(id, device_code, user_code, verification_url, expires_at, interval_seconds, client_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`

	_, err := s.db.Exec(query,
		authReq.ID,
		authReq.DeviceCode,
		authReq.UserCode,
		authReq.VerificationURL,
		authReq.ExpiresAt,
		authReq.Interval,
		authReq.ClientID,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create device auth request: %w", err)
	}

	// Return DeviceAuthResponse format
	return &DeviceAuthResponse{
		DeviceCode:      deviceCode,
		UserCode:        formatUserCode(userCode),
		VerificationURL: authReq.VerificationURL,
		ExpiresIn:       900, // 15 minutes
		Interval:        5,
	}, nil
}

// CreateDeviceAuthWithURL creates a new device authorization request with custom base URL
func (s *DeviceAuthService) CreateDeviceAuthWithURL(clientID string, baseURL string) (*DeviceAuthRequest, error) {
	deviceCode := generateDeviceCode()
	userCode := generateUserCode()

	authReq := &DeviceAuthRequest{
		ID:              uuid.New().String(),
		DeviceCode:      deviceCode,
		UserCode:        userCode,
		VerificationURL: fmt.Sprintf("%s/auth/device", baseURL),
		ExpiresAt:       time.Now().Add(15 * time.Minute),
		Interval:        5,
		ClientID:        clientID,
		ClientName:      getClientName(clientID),
	}

	query := `
		INSERT INTO device_auth_request
		(id, device_code, user_code, verification_url, expires_at, interval_seconds, client_id, client_name)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`

	_, err := s.db.Exec(query,
		authReq.ID,
		authReq.DeviceCode,
		authReq.UserCode,
		authReq.VerificationURL,
		authReq.ExpiresAt,
		authReq.Interval,
		authReq.ClientID,
		authReq.ClientName,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create device auth request: %w", err)
	}

	return authReq, nil
}

// VerifyUserCode verifies a user code and returns the device auth request
func (s *DeviceAuthService) VerifyUserCode(userCode string) (*DeviceAuthRequest, error) {
	userCode = strings.ToUpper(strings.ReplaceAll(userCode, "-", ""))

	query := `
		SELECT id, device_code, user_code, verification_url, expires_at,
		       interval_seconds, client_id, client_name, user_id, approved_at
		FROM device_auth_request
		WHERE user_code = $1 AND expires_at > NOW()
	`

	var authReq DeviceAuthRequest
	var interval int

	err := s.db.QueryRow(query, userCode).Scan(
		&authReq.ID,
		&authReq.DeviceCode,
		&authReq.UserCode,
		&authReq.VerificationURL,
		&authReq.ExpiresAt,
		&interval,
		&authReq.ClientID,
		&authReq.ClientName,
		&authReq.UserID,
		&authReq.ApprovedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("invalid or expired code")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to verify user code: %w", err)
	}

	authReq.Interval = interval
	return &authReq, nil
}

// ApproveDeviceAuth approves a device authorization request by user code
func (s *DeviceAuthService) ApproveDeviceAuth(userCode, userID string) error {
	// First, strip dashes from user code
	userCode = strings.ToUpper(strings.ReplaceAll(userCode, "-", ""))

	query := `
		UPDATE device_auth_request
		SET user_id = $1, approved_at = NOW()
		WHERE user_code = $2 AND expires_at > NOW() AND approved_at IS NULL
	`

	result, err := s.db.Exec(query, userID, userCode)
	if err != nil {
		return fmt.Errorf("failed to approve device auth: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("device auth request not found or already approved")
	}

	return nil
}

// ApproveDeviceAuthByID approves a device authorization request by ID
func (s *DeviceAuthService) ApproveDeviceAuthByID(deviceAuthID, userID string) error {
	query := `
		UPDATE device_auth_request
		SET user_id = $1, approved_at = NOW()
		WHERE id = $2 AND expires_at > NOW() AND approved_at IS NULL
	`

	result, err := s.db.Exec(query, userID, deviceAuthID)
	if err != nil {
		return fmt.Errorf("failed to approve device auth: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("device auth request not found or already approved")
	}

	return nil
}

// CheckDeviceAuth checks if a device code has been approved
func (s *DeviceAuthService) CheckDeviceAuth(deviceCode string) (*DeviceAuthRequest, error) {
	query := `
		SELECT id, device_code, user_code, verification_url, expires_at,
		       interval_seconds, client_id, client_name, user_id, approved_at
		FROM device_auth_request
		WHERE device_code = $1 AND expires_at > NOW()
	`

	var authReq DeviceAuthRequest
	var interval int

	err := s.db.QueryRow(query, deviceCode).Scan(
		&authReq.ID,
		&authReq.DeviceCode,
		&authReq.UserCode,
		&authReq.VerificationURL,
		&authReq.ExpiresAt,
		&interval,
		&authReq.ClientID,
		&authReq.ClientName,
		&authReq.UserID,
		&authReq.ApprovedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("expired_token")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to check device auth: %w", err)
	}

	authReq.Interval = interval

	if authReq.ApprovedAt == nil {
		return nil, fmt.Errorf("authorization_pending")
	}

	return &authReq, nil
}

// CreateAccessToken creates an access token for an approved device
func (s *DeviceAuthService) CreateAccessToken(userID, deviceAuthID, clientID string) (string, error) {
	token := generateDeviceCode()
	expiresAt := time.Now().Add(24 * time.Hour)

	query := `
		INSERT INTO access_token
		(id, token, user_id, device_auth_id, expires_at, client_id, scope)
		VALUES ($1, $2, $3, $4, $5, $6, 'api')
	`

	_, err := s.db.Exec(query,
		uuid.New().String(),
		token,
		userID,
		deviceAuthID,
		expiresAt,
		clientID,
	)

	if err != nil {
		return "", fmt.Errorf("failed to create access token: %w", err)
	}

	return token, nil
}

// ValidateToken validates an access token and returns the user ID
func (s *DeviceAuthService) ValidateToken(token string) (string, error) {
	query := `
		SELECT user_id
		FROM access_token
		WHERE token = $1
		  AND expires_at > NOW()
		  AND revoked_at IS NULL
	`

	var userID string
	err := s.db.QueryRow(query, token).Scan(&userID)

	if err == sql.ErrNoRows {
		return "", fmt.Errorf("invalid or expired token")
	}
	if err != nil {
		return "", fmt.Errorf("failed to validate token: %w", err)
	}

	// Update last_used
	updateQuery := `UPDATE access_token SET last_used = NOW() WHERE token = $1`
	s.db.Exec(updateQuery, token)

	return userID, nil
}

// generateDeviceCode generates a secure random device code
func generateDeviceCode() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)[:43] // Remove padding
}

// ExchangeDeviceCode exchanges a device code for an access token
func (s *DeviceAuthService) ExchangeDeviceCode(deviceCode string, clientID string) (*TokenResponse, error) {
	// First check if the device code has been approved
	query := `
		SELECT user_id, approved_at, expires_at
		FROM device_auth_request
		WHERE device_code = $1 AND client_id = $2
	`

	var userID sql.NullString
	var approvedAt sql.NullTime
	var expiresAt time.Time

	err := s.db.QueryRow(query, deviceCode, clientID).Scan(&userID, &approvedAt, &expiresAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("invalid_grant")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to check device code: %w", err)
	}

	if time.Now().After(expiresAt) {
		return nil, fmt.Errorf("expired_token")
	}

	if !approvedAt.Valid {
		return nil, fmt.Errorf("authorization_pending")
	}

	// Create access token
	token, err := s.CreateAccessToken(userID.String, deviceCode, clientID)
	if err != nil {
		return nil, err
	}

	return &TokenResponse{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresIn:   86400, // 24 hours
		Scope:       "api",
	}, nil
}

// generateUserCode generates a user-friendly code
func generateUserCode() string {
	const charset = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // Avoid ambiguous characters
	b := make([]byte, 8)
	for i := range b {
		b[i] = charset[randInt(len(charset))]
	}
	return string(b)
}

// formatUserCode formats a user code with hyphen
func formatUserCode(code string) string {
	if len(code) == 8 {
		return code[:4] + "-" + code[4:]
	}
	return code
}

func randInt(max int) int {
	b := make([]byte, 1)
	rand.Read(b)
	return int(b[0]) % max
}

func getClientName(clientID string) string {
	switch clientID {
	case "fleetctl":
		return "Fleet CLI"
	case "studio":
		return "Fleet Studio"
	default:
		return clientID
	}
}