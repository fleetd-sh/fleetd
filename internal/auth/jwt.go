package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// JWTManager handles JWT token creation and validation
type JWTManager struct {
	secretKey     []byte
	tokenDuration time.Duration
	issuer        string
}

// Claims represents the JWT claims
type Claims struct {
	jwt.RegisteredClaims
	UserID       string   `json:"user_id"`
	Email        string   `json:"email"`
	Roles        []string `json:"roles"`
	Permissions  []string `json:"permissions"`
	DeviceID     string   `json:"device_id,omitempty"` // For device tokens
	RefreshToken bool     `json:"refresh_token,omitempty"`
}

// TokenPair represents access and refresh tokens
type TokenPair struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// NewJWTManager creates a new JWT manager
func NewJWTManager(secretKey string, tokenDuration time.Duration, issuer string) *JWTManager {
	if secretKey == "" {
		// Generate a random key if not provided (not for production!)
		key := make([]byte, 32)
		rand.Read(key)
		secretKey = base64.StdEncoding.EncodeToString(key)
	}

	return &JWTManager{
		secretKey:     []byte(secretKey),
		tokenDuration: tokenDuration,
		issuer:        issuer,
	}
}

// GenerateTokenPair creates both access and refresh tokens
func (m *JWTManager) GenerateTokenPair(userID, email string, roles, permissions []string) (*TokenPair, error) {
	// Create access token
	accessToken, expiresAt, err := m.generateToken(userID, email, roles, permissions, false, m.tokenDuration)
	if err != nil {
		return nil, err
	}

	// Create refresh token (longer duration)
	refreshToken, _, err := m.generateToken(userID, email, roles, permissions, true, 7*24*time.Hour)
	if err != nil {
		return nil, err
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    expiresAt,
	}, nil
}

// GenerateDeviceToken creates a token for device authentication
func (m *JWTManager) GenerateDeviceToken(deviceID string, permissions []string) (string, error) {
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(365 * 24 * time.Hour)), // 1 year for devices
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    m.issuer,
			Subject:   "device",
			ID:        generateTokenID(),
		},
		DeviceID:    deviceID,
		Permissions: permissions,
		Roles:       []string{"device"},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.secretKey)
}

// generateToken creates a JWT token
func (m *JWTManager) generateToken(userID, email string, roles, permissions []string, isRefresh bool, duration time.Duration) (string, time.Time, error) {
	expiresAt := time.Now().Add(duration)

	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    m.issuer,
			Subject:   userID,
			ID:        generateTokenID(),
		},
		UserID:       userID,
		Email:        email,
		Roles:        roles,
		Permissions:  permissions,
		RefreshToken: isRefresh,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedToken, err := token.SignedString(m.secretKey)
	if err != nil {
		return "", time.Time{}, err
	}

	return signedToken, expiresAt, nil
}

// ValidateToken validates and parses a JWT token
func (m *JWTManager) ValidateToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		// Validate signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return m.secretKey, nil
	})

	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}

	// Additional validation
	if claims.Issuer != m.issuer {
		return nil, fmt.Errorf("invalid issuer")
	}

	return claims, nil
}

// RefreshAccessToken creates a new access token from a refresh token
func (m *JWTManager) RefreshAccessToken(refreshTokenString string) (*TokenPair, error) {
	claims, err := m.ValidateToken(refreshTokenString)
	if err != nil {
		return nil, err
	}

	if !claims.RefreshToken {
		return nil, fmt.Errorf("not a refresh token")
	}

	return m.GenerateTokenPair(claims.UserID, claims.Email, claims.Roles, claims.Permissions)
}

// RevokeToken adds a token to the blacklist (requires external storage)
func (m *JWTManager) RevokeToken(ctx context.Context, tokenID string) error {
	// This should store the token ID in a blacklist (Redis, database, etc.)
	// For now, this is a placeholder
	return nil
}

// generateTokenID generates a unique token ID
func generateTokenID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// ExtractClaims extracts claims without validation (for logging/debugging)
func (m *JWTManager) ExtractClaims(tokenString string) (*Claims, error) {
	token, _, err := new(jwt.Parser).ParseUnverified(tokenString, &Claims{})
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok {
		return nil, fmt.Errorf("invalid claims type")
	}

	return claims, nil
}