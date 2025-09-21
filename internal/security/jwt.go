package security

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"fmt"
	"log/slog"
	"time"

	"fleetd.sh/internal/ferrors"
	"github.com/golang-jwt/jwt/v5"
)

// JWTConfig holds JWT configuration
type JWTConfig struct {
	SigningKey      []byte
	SigningMethod   jwt.SigningMethod
	Issuer          string
	Audience        string
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
	RSAPrivateKey   *rsa.PrivateKey
	RSAPublicKey    *rsa.PublicKey
}

// DefaultJWTConfig returns default JWT configuration
func DefaultJWTConfig() *JWTConfig {
	return &JWTConfig{
		SigningMethod:   jwt.SigningMethodHS256,
		Issuer:          "fleetd",
		Audience:        "fleetd-api",
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 7 * 24 * time.Hour,
	}
}

// JWTManager manages JWT tokens
type JWTManager struct {
	config         *JWTConfig
	logger         *slog.Logger
	tokenBlacklist TokenBlacklist
}

// NewJWTManager creates a new JWT manager
func NewJWTManager(config *JWTConfig) (*JWTManager, error) {
	if config == nil {
		config = DefaultJWTConfig()
	}

	// Apply defaults for missing fields
	if config.SigningMethod == nil {
		config.SigningMethod = jwt.SigningMethodHS256
	}
	if config.Issuer == "" {
		config.Issuer = "fleetd"
	}
	if config.Audience == "" {
		config.Audience = "fleetd-api"
	}
	if config.AccessTokenTTL == 0 {
		config.AccessTokenTTL = 15 * time.Minute
	}
	if config.RefreshTokenTTL == 0 {
		config.RefreshTokenTTL = 7 * 24 * time.Hour
	}

	// Generate signing key if not provided
	if len(config.SigningKey) == 0 && config.RSAPrivateKey == nil {
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return nil, ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to generate signing key")
		}
		config.SigningKey = key
	}

	// Create in-memory blacklist by default
	blacklist := NewMemoryTokenBlacklist()

	return &JWTManager{
		config:         config,
		logger:         slog.Default().With("component", "jwt"),
		tokenBlacklist: blacklist,
	}, nil
}

// Claims represents JWT claims
type Claims struct {
	jwt.RegisteredClaims
	UserID      string       `json:"user_id"`
	Username    string       `json:"username"`
	Email       string       `json:"email"`
	Roles       []Role       `json:"roles"`
	Permissions []Permission `json:"permissions,omitempty"`
	DeviceID    string       `json:"device_id,omitempty"`
	TokenType   TokenType    `json:"token_type"`
}

// TokenType represents the type of token
type TokenType string

const (
	TokenTypeAccess  TokenType = "access"
	TokenTypeRefresh TokenType = "refresh"
	TokenTypeDevice  TokenType = "device"
	TokenTypeService TokenType = "service"
)

// Token represents a JWT token pair
type Token struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	TokenType    string    `json:"token_type"`
	ExpiresIn    int64     `json:"expires_in"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// GenerateTokenPair generates access and refresh tokens
func (m *JWTManager) GenerateTokenPair(user *User) (*Token, error) {
	// Generate access token
	accessToken, accessExp, err := m.generateToken(user, TokenTypeAccess, m.config.AccessTokenTTL)
	if err != nil {
		return nil, ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to generate access token")
	}

	// Generate refresh token
	refreshToken, _, err := m.generateToken(user, TokenTypeRefresh, m.config.RefreshTokenTTL)
	if err != nil {
		return nil, ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to generate refresh token")
	}

	return &Token{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int64(m.config.AccessTokenTTL.Seconds()),
		ExpiresAt:    accessExp,
	}, nil
}

// GenerateDeviceToken generates a token for device authentication
func (m *JWTManager) GenerateDeviceToken(deviceID string) (string, error) {
	now := time.Now()
	expiresAt := now.Add(365 * 24 * time.Hour) // 1 year for device tokens

	claims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.config.Issuer,
			Subject:   deviceID,
			Audience:  jwt.ClaimStrings{m.config.Audience},
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			NotBefore: jwt.NewNumericDate(now),
			IssuedAt:  jwt.NewNumericDate(now),
			ID:        generateTokenID(),
		},
		DeviceID:  deviceID,
		TokenType: TokenTypeDevice,
		Roles:     []Role{RoleDevice},
	}

	token := jwt.NewWithClaims(m.config.SigningMethod, claims)

	signedToken, err := m.signToken(token)
	if err != nil {
		return "", ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to sign device token")
	}

	m.logger.Info("Device token generated",
		"device_id", deviceID,
		"expires_at", expiresAt,
	)

	return signedToken, nil
}

// generateToken generates a JWT token
func (m *JWTManager) generateToken(user *User, tokenType TokenType, ttl time.Duration) (string, time.Time, error) {
	now := time.Now()
	expiresAt := now.Add(ttl)

	claims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.config.Issuer,
			Subject:   user.ID,
			Audience:  jwt.ClaimStrings{m.config.Audience},
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			NotBefore: jwt.NewNumericDate(now),
			IssuedAt:  jwt.NewNumericDate(now),
			ID:        generateTokenID(),
		},
		UserID:      user.ID,
		Username:    user.Username,
		Email:       user.Email,
		Roles:       user.Roles,
		Permissions: user.Permissions,
		TokenType:   tokenType,
	}

	token := jwt.NewWithClaims(m.config.SigningMethod, claims)

	signedToken, err := m.signToken(token)
	if err != nil {
		return "", time.Time{}, err
	}

	return signedToken, expiresAt, nil
}

// signToken signs a JWT token
func (m *JWTManager) signToken(token *jwt.Token) (string, error) {
	if m.config.RSAPrivateKey != nil {
		return token.SignedString(m.config.RSAPrivateKey)
	}
	return token.SignedString(m.config.SigningKey)
}

// ValidateToken validates and parses a JWT token
func (m *JWTManager) ValidateToken(tokenString string) (*Claims, error) {
	// Parse token
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (any, error) {
		// Verify signing method
		if token.Method != m.config.SigningMethod {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}

		// Return the appropriate key
		if m.config.RSAPublicKey != nil {
			return m.config.RSAPublicKey, nil
		}
		return m.config.SigningKey, nil
	})

	if err != nil {
		return nil, ferrors.Wrap(err, ferrors.ErrCodePermissionDenied, "invalid token")
	}

	// Check if token is valid
	if !token.Valid {
		return nil, ferrors.New(ferrors.ErrCodePermissionDenied, "token is not valid")
	}

	// Extract claims
	claims, ok := token.Claims.(*Claims)
	if !ok {
		return nil, ferrors.New(ferrors.ErrCodeInternal, "failed to parse claims")
	}

	// Validate claims
	if err := m.validateClaims(claims); err != nil {
		return nil, err
	}

	// Check if token is blacklisted
	if claims.ID != "" && m.tokenBlacklist != nil {
		isRevoked, err := m.IsTokenRevoked(claims.ID)
		if err != nil {
			m.logger.Error("Failed to check token blacklist", "error", err, "jti", claims.ID)
			// Continue without failing - blacklist check is best-effort
		} else if isRevoked {
			return nil, ferrors.New(ferrors.ErrCodePermissionDenied, "token has been revoked")
		}
	}

	return claims, nil
}

// validateClaims validates JWT claims
func (m *JWTManager) validateClaims(claims *Claims) error {
	now := time.Now()

	// Check expiration
	if claims.ExpiresAt != nil && claims.ExpiresAt.Before(now) {
		return ferrors.New(ferrors.ErrCodePermissionDenied, "token has expired")
	}

	// Check not before
	if claims.NotBefore != nil && claims.NotBefore.After(now) {
		return ferrors.New(ferrors.ErrCodePermissionDenied, "token not yet valid")
	}

	// Check issuer
	if claims.Issuer != m.config.Issuer {
		return ferrors.Newf(ferrors.ErrCodePermissionDenied, "invalid issuer: %s", claims.Issuer)
	}

	// Check audience
	validAudience := false
	for _, aud := range claims.Audience {
		if aud == m.config.Audience {
			validAudience = true
			break
		}
	}
	if !validAudience {
		return ferrors.New(ferrors.ErrCodePermissionDenied, "invalid audience")
	}

	return nil
}

// RefreshToken refreshes an access token using a refresh token
func (m *JWTManager) RefreshToken(refreshTokenString string) (*Token, error) {
	// Validate refresh token
	claims, err := m.ValidateToken(refreshTokenString)
	if err != nil {
		return nil, err
	}

	// Check if it's a refresh token
	if claims.TokenType != TokenTypeRefresh {
		return nil, ferrors.New(ferrors.ErrCodePermissionDenied, "not a refresh token")
	}

	// Create user from claims
	user := &User{
		ID:          claims.UserID,
		Username:    claims.Username,
		Email:       claims.Email,
		Roles:       claims.Roles,
		Permissions: claims.Permissions,
	}

	// Generate new token pair
	return m.GenerateTokenPair(user)
}

// ParseUnverified parses a token without verifying the signature
// This should only be used when you need to extract claims from an untrusted token
func (m *JWTManager) ParseUnverified(tokenString string) (*Claims, error) {
	// Parse without verification
	token, _, err := new(jwt.Parser).ParseUnverified(tokenString, &Claims{})
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok {
		return nil, ErrInvalidToken
	}

	return claims, nil
}

// RevokeToken revokes a token by adding it to the blacklist
func (m *JWTManager) RevokeToken(tokenID string, expiresAt time.Time) error {
	if m.tokenBlacklist == nil {
		return ferrors.New(ferrors.ErrCodeInternal, "token blacklist not configured")
	}

	ctx := context.Background()
	if err := m.tokenBlacklist.Add(ctx, tokenID, expiresAt); err != nil {
		return ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to revoke token")
	}

	m.logger.Info("Token revoked", "token_id", tokenID, "expires_at", expiresAt)
	return nil
}

// IsTokenRevoked checks if a token has been revoked
func (m *JWTManager) IsTokenRevoked(tokenID string) (bool, error) {
	if m.tokenBlacklist == nil {
		return false, nil // No blacklist means no revocation
	}

	ctx := context.Background()
	return m.tokenBlacklist.IsBlacklisted(ctx, tokenID)
}

// SetTokenBlacklist sets a custom token blacklist implementation
func (m *JWTManager) SetTokenBlacklist(blacklist TokenBlacklist) {
	m.tokenBlacklist = blacklist
}

// ExtractTokenFromHeader extracts token from Authorization header
func ExtractTokenFromHeader(authHeader string) (string, error) {
	if authHeader == "" {
		return "", ferrors.New(ferrors.ErrCodePermissionDenied, "authorization header missing")
	}

	// Check for Bearer token
	const bearerPrefix = "Bearer "
	if len(authHeader) < len(bearerPrefix) || authHeader[:len(bearerPrefix)] != bearerPrefix {
		return "", ferrors.New(ferrors.ErrCodePermissionDenied, "invalid authorization header format")
	}

	return authHeader[len(bearerPrefix):], nil
}

// generateTokenID generates a unique token ID
func generateTokenID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// ContextKey for storing auth information in context
type ContextKey string

const (
	ContextKeyClaims ContextKey = "claims"
	ContextKeyUser   ContextKey = "user"
)

// GetClaimsFromContext retrieves claims from context
func GetClaimsFromContext(ctx context.Context) (*Claims, bool) {
	claims, ok := ctx.Value(ContextKeyClaims).(*Claims)
	return claims, ok
}

// SetClaimsInContext sets claims in context
func SetClaimsInContext(ctx context.Context, claims *Claims) context.Context {
	return context.WithValue(ctx, ContextKeyClaims, claims)
}

// GetUserFromContext retrieves user from context
func GetUserFromContext(ctx context.Context) (*User, bool) {
	user, ok := ctx.Value(ContextKeyUser).(*User)
	return user, ok
}

// SetUserInContext sets user in context
func SetUserInContext(ctx context.Context, user *User) context.Context {
	return context.WithValue(ctx, ContextKeyUser, user)
}

// TokenInfo contains token information
type TokenInfo struct {
	TokenID   string    `json:"token_id"`
	UserID    string    `json:"user_id"`
	DeviceID  string    `json:"device_id,omitempty"`
	TokenType TokenType `json:"token_type"`
	IssuedAt  time.Time `json:"issued_at"`
	ExpiresAt time.Time `json:"expires_at"`
	Valid     bool      `json:"valid"`
	Expired   bool      `json:"expired"`
}

// GetTokenInfo returns information about a token
func (m *JWTManager) GetTokenInfo(tokenString string) (*TokenInfo, error) {
	// Parse token without validation to get info even for expired tokens
	token, _ := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (any, error) {
		if m.config.RSAPublicKey != nil {
			return m.config.RSAPublicKey, nil
		}
		return m.config.SigningKey, nil
	})

	if token == nil {
		return nil, ferrors.New(ferrors.ErrCodeInvalidInput, "failed to parse token")
	}

	claims, ok := token.Claims.(*Claims)
	if !ok {
		return nil, ferrors.New(ferrors.ErrCodeInternal, "failed to parse claims")
	}

	now := time.Now()
	expired := claims.ExpiresAt != nil && claims.ExpiresAt.Before(now)

	return &TokenInfo{
		TokenID:   claims.ID,
		UserID:    claims.UserID,
		DeviceID:  claims.DeviceID,
		TokenType: claims.TokenType,
		IssuedAt:  claims.IssuedAt.Time,
		ExpiresAt: claims.ExpiresAt.Time,
		Valid:     token.Valid,
		Expired:   expired,
	}, nil
}
