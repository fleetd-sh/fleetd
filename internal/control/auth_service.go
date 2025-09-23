package control

import (
	"context"
	"database/sql"
	"time"

	"connectrpc.com/connect"
	publicv1 "fleetd.sh/gen/public/v1"
	"fleetd.sh/internal/ferrors"
	"fleetd.sh/internal/security"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// AuthService handles authentication operations
type AuthService struct {
	db         *sql.DB
	jwtManager *security.JWTManager
}

// NewAuthService creates a new auth service
func NewAuthService(db *sql.DB, jwtManager *security.JWTManager) *AuthService {
	return &AuthService{
		db:         db,
		jwtManager: jwtManager,
	}
}

// Login authenticates a user and returns tokens
func (s *AuthService) Login(ctx context.Context, req *connect.Request[publicv1.LoginRequest]) (*connect.Response[publicv1.LoginResponse], error) {
	msg := req.Msg

	// Handle password authentication
	if cred, ok := msg.Credential.(*publicv1.LoginRequest_Password); ok {
		user, err := s.authenticatePassword(ctx, cred.Password.Email, cred.Password.Password)
		if err != nil {
			return nil, connect.NewError(connect.CodeUnauthenticated, err)
		}

		// Generate tokens
		tokenPair, err := s.jwtManager.GenerateTokenPair(&security.User{
			ID:    user.Id,
			Email: user.Email,
		})
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}

		return connect.NewResponse(&publicv1.LoginResponse{
			AccessToken:  tokenPair.AccessToken,
			RefreshToken: tokenPair.RefreshToken,
			ExpiresIn:    tokenPair.ExpiresIn,
			User:         user,
		}), nil
	}

	// Handle API key authentication
	if cred, ok := msg.Credential.(*publicv1.LoginRequest_ApiKey); ok {
		user, err := s.authenticateAPIKey(ctx, cred.ApiKey.Key)
		if err != nil {
			return nil, connect.NewError(connect.CodeUnauthenticated, err)
		}

		// Generate tokens (API keys only get access token)
		tokenPair, err := s.jwtManager.GenerateTokenPair(&security.User{
			ID:    user.Id,
			Email: user.Email,
		})
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}

		return connect.NewResponse(&publicv1.LoginResponse{
			AccessToken: tokenPair.AccessToken,
			ExpiresIn:   tokenPair.ExpiresIn,
			User:        user,
		}), nil
	}

	return nil, connect.NewError(connect.CodeInvalidArgument, ferrors.New(ferrors.ErrCodeInvalidInput, "invalid credentials"))
}

// Logout invalidates tokens
func (s *AuthService) Logout(ctx context.Context, req *connect.Request[publicv1.LogoutRequest]) (*connect.Response[emptypb.Empty], error) {
	// In a production system, we would invalidate the refresh token here
	// For now, just return success
	return connect.NewResponse(&emptypb.Empty{}), nil
}

// RefreshToken generates new access token
func (s *AuthService) RefreshToken(ctx context.Context, req *connect.Request[publicv1.RefreshTokenRequest]) (*connect.Response[publicv1.RefreshTokenResponse], error) {
	// Validate refresh token
	claims, err := s.jwtManager.ValidateToken(req.Msg.RefreshToken)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}

	// Generate new token pair
	tokenPair, err := s.jwtManager.GenerateTokenPair(&security.User{
		ID:    claims.UserID,
		Email: claims.Email,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&publicv1.RefreshTokenResponse{
		AccessToken:  tokenPair.AccessToken,
		RefreshToken: tokenPair.RefreshToken,
		ExpiresIn:    tokenPair.ExpiresIn,
	}), nil
}

// GetCurrentUser returns the current authenticated user
func (s *AuthService) GetCurrentUser(ctx context.Context, req *connect.Request[emptypb.Empty]) (*connect.Response[publicv1.GetCurrentUserResponse], error) {
	// Get user from context (set by JWT middleware)
	claims, ok := ctx.Value("user").(*security.Claims)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, ferrors.New(ferrors.ErrCodePermissionDenied, "not authenticated"))
	}

	// Fetch user details from database
	user, err := s.getUserByID(ctx, claims.UserID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&publicv1.GetCurrentUserResponse{
		User: user,
	}), nil
}

// CreateAPIKey creates a new API key
func (s *AuthService) CreateAPIKey(ctx context.Context, req *connect.Request[publicv1.CreateAPIKeyRequest]) (*connect.Response[publicv1.CreateAPIKeyResponse], error) {
	// Get user from context
	claims, ok := ctx.Value("user").(*security.Claims)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, ferrors.New(ferrors.ErrCodePermissionDenied, "not authenticated"))
	}

	// Generate API key
	apiKey := security.GenerateAPIKey()
	hashedKey, err := bcrypt.GenerateFromPassword([]byte(apiKey), bcrypt.DefaultCost)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// Store API key in database
	query := `
		INSERT INTO api_keys (id, user_id, name, description, key_hash, created_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`
	keyID := security.GenerateID()
	now := time.Now()
	_, err = s.db.ExecContext(ctx, query, keyID, claims.UserID, req.Msg.Name, req.Msg.Description, hashedKey, now, req.Msg.ExpiresAt.AsTime())
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&publicv1.CreateAPIKeyResponse{
		Id:        keyID,
		Key:       apiKey, // Only returned once at creation
		Name:      req.Msg.Name,
		CreatedAt: timestamppb.New(now),
		ExpiresAt: req.Msg.ExpiresAt,
	}), nil
}

// ListAPIKeys lists all API keys for the current user
func (s *AuthService) ListAPIKeys(ctx context.Context, req *connect.Request[publicv1.ListAPIKeysRequest]) (*connect.Response[publicv1.ListAPIKeysResponse], error) {
	// Get user from context
	claims, ok := ctx.Value("user").(*security.Claims)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, ferrors.New(ferrors.ErrCodePermissionDenied, "not authenticated"))
	}

	// Query API keys
	query := `
		SELECT id, name, description, created_at, expires_at, last_used_at
		FROM api_keys
		WHERE user_id = $1 AND (expires_at IS NULL OR expires_at > NOW())
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`
	limit := int32(100)
	offset := int32(0)
	if req.Msg.PageSize > 0 {
		limit = req.Msg.PageSize
	}
	// Simple page number based pagination for now
	// In production, use cursor-based pagination

	rows, err := s.db.QueryContext(ctx, query, claims.UserID, limit, offset)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	defer rows.Close()

	var keys []*publicv1.APIKey
	for rows.Next() {
		var key publicv1.APIKey
		var lastUsedAt sql.NullTime
		var expiresAt sql.NullTime
		var createdAt time.Time

		err := rows.Scan(&key.Id, &key.Name, &key.Description, &createdAt, &expiresAt, &lastUsedAt)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}

		key.CreatedAt = timestamppb.New(createdAt)
		if expiresAt.Valid {
			key.ExpiresAt = timestamppb.New(expiresAt.Time)
		}
		if lastUsedAt.Valid {
			key.LastUsed = timestamppb.New(lastUsedAt.Time)
		}

		keys = append(keys, &key)
	}

	return connect.NewResponse(&publicv1.ListAPIKeysResponse{
		Keys: keys,
	}), nil
}

// RevokeAPIKey revokes an API key
func (s *AuthService) RevokeAPIKey(ctx context.Context, req *connect.Request[publicv1.RevokeAPIKeyRequest]) (*connect.Response[emptypb.Empty], error) {
	// Get user from context
	claims, ok := ctx.Value("user").(*security.Claims)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, ferrors.New(ferrors.ErrCodePermissionDenied, "not authenticated"))
	}

	// Delete API key
	query := `DELETE FROM api_keys WHERE id = $1 AND user_id = $2`
	result, err := s.db.ExecContext(ctx, query, req.Msg.KeyId, claims.UserID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	if rowsAffected == 0 {
		return nil, connect.NewError(connect.CodeNotFound, ferrors.New(ferrors.ErrCodeNotFound, "API key not found"))
	}

	return connect.NewResponse(&emptypb.Empty{}), nil
}

// SSO methods (placeholders for now)

func (s *AuthService) GetSSOProviders(ctx context.Context, req *connect.Request[emptypb.Empty]) (*connect.Response[publicv1.GetSSOProvidersResponse], error) {
	// Return empty list for now - SSO not yet implemented
	return connect.NewResponse(&publicv1.GetSSOProvidersResponse{
		Providers: []*publicv1.SSOProvider{},
	}), nil
}

func (s *AuthService) InitiateSSOLogin(ctx context.Context, req *connect.Request[publicv1.InitiateSSOLoginRequest]) (*connect.Response[publicv1.InitiateSSOLoginResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, ferrors.New(ferrors.ErrCodeNotImplemented, "SSO not yet implemented"))
}

func (s *AuthService) CompleteSSOLogin(ctx context.Context, req *connect.Request[publicv1.CompleteSSOLoginRequest]) (*connect.Response[publicv1.CompleteSSOLoginResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, ferrors.New(ferrors.ErrCodeNotImplemented, "SSO not yet implemented"))
}

// Helper methods

func (s *AuthService) authenticatePassword(ctx context.Context, email, password string) (*publicv1.User, error) {
	// For development, create a default admin user if no users exist
	var count int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&count)
	if err == nil && count == 0 {
		// Create default admin user
		hashedPassword, _ := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.DefaultCost)
		userID := "user-admin"
		now := time.Now()

		_, err = s.db.ExecContext(ctx, `
			INSERT INTO users (id, email, password_hash, name, role, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
		`, userID, "admin@fleetd.local", hashedPassword, "Admin User", "admin", now, now)

		if err == nil && email == "admin@fleetd.local" && password == "admin123" {
			return &publicv1.User{
				Id:        userID,
				Email:     email,
				Name:      "Admin User",
				Role:      publicv1.UserRole_USER_ROLE_ADMIN,
				CreatedAt: timestamppb.New(now),
			}, nil
		}
	}

	// Query user from database
	var user publicv1.User
	var hashedPassword []byte
	var createdAt time.Time
	var roleStr string

	query := `
		SELECT id, email, name, password_hash, role, created_at
		FROM users
		WHERE email = $1
	`
	err = s.db.QueryRowContext(ctx, query, email).Scan(
		&user.Id, &user.Email, &user.Name, &hashedPassword, &roleStr, &createdAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ferrors.New(ferrors.ErrCodePermissionDenied, "invalid credentials")
		}
		return nil, err
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword(hashedPassword, []byte(password)); err != nil {
		return nil, ferrors.New(ferrors.ErrCodePermissionDenied, "invalid credentials")
	}

	user.Role = parseUserRole(roleStr)
	user.CreatedAt = timestamppb.New(createdAt)
	user.LastLogin = timestamppb.New(time.Now())

	// Update last login
	_, _ = s.db.ExecContext(ctx, "UPDATE users SET last_login = NOW() WHERE id = $1", user.Id)

	return &user, nil
}

func (s *AuthService) authenticateAPIKey(ctx context.Context, apiKey string) (*publicv1.User, error) {
	// Query API key
	var userID string
	var hashedKey []byte
	query := `
		SELECT user_id, key_hash
		FROM api_keys
		WHERE expires_at IS NULL OR expires_at > NOW()
		LIMIT 100
	`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Check each key (inefficient but simple for now)
	for rows.Next() {
		err := rows.Scan(&userID, &hashedKey)
		if err != nil {
			continue
		}

		// Check if this is the matching key
		if err := bcrypt.CompareHashAndPassword(hashedKey, []byte(apiKey)); err == nil {
			// Update last used
			_, _ = s.db.ExecContext(ctx, "UPDATE api_keys SET last_used_at = NOW() WHERE key_hash = $1", hashedKey)

			// Get user
			return s.getUserByID(ctx, userID)
		}
	}

	return nil, ferrors.New(ferrors.ErrCodePermissionDenied, "invalid API key")
}

func (s *AuthService) getUserByID(ctx context.Context, userID string) (*publicv1.User, error) {
	var user publicv1.User
	var createdAt time.Time
	var roleStr string

	query := `
		SELECT id, email, name, role, created_at
		FROM users
		WHERE id = $1
	`
	err := s.db.QueryRowContext(ctx, query, userID).Scan(
		&user.Id, &user.Email, &user.Name, &roleStr, &createdAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ferrors.New(ferrors.ErrCodeNotFound, "user not found")
		}
		return nil, err
	}

	user.Role = parseUserRole(roleStr)
	user.CreatedAt = timestamppb.New(createdAt)
	return &user, nil
}

// mapUserRole converts proto user role to string
func mapUserRole(role publicv1.UserRole) string {
	switch role {
	case publicv1.UserRole_USER_ROLE_ADMIN:
		return "admin"
	case publicv1.UserRole_USER_ROLE_OPERATOR:
		return "operator"
	case publicv1.UserRole_USER_ROLE_OWNER:
		return "owner"
	default:
		return "viewer"
	}
}

// parseUserRole converts string to proto user role
func parseUserRole(role string) publicv1.UserRole {
	switch role {
	case "admin":
		return publicv1.UserRole_USER_ROLE_ADMIN
	case "operator":
		return publicv1.UserRole_USER_ROLE_OPERATOR
	case "owner":
		return publicv1.UserRole_USER_ROLE_OWNER
	default:
		return publicv1.UserRole_USER_ROLE_VIEWER
	}
}
