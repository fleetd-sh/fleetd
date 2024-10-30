package auth

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	authpb "fleetd.sh/gen/auth/v1"
)

type AuthService struct {
	db *sql.DB
}

func NewAuthService(db *sql.DB) *AuthService {
	return &AuthService{db}
}

func (s *AuthService) Authenticate(
	ctx context.Context,
	req *connect.Request[authpb.AuthenticateRequest],
) (*connect.Response[authpb.AuthenticateResponse], error) {
	var deviceID string
	hashedKey := hashKey(req.Msg.ApiKey)
	err := s.db.QueryRowContext(ctx, "SELECT device_id FROM api_key WHERE key_hash = ?", hashedKey).Scan(&deviceID)
	if err != nil {
		if err == sql.ErrNoRows {
			return connect.NewResponse(&authpb.AuthenticateResponse{
				Authenticated: false,
			}), nil
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to query database: %w", err))
	}

	return connect.NewResponse(&authpb.AuthenticateResponse{
		Authenticated: true,
		DeviceId:      deviceID,
	}), nil
}

func (s *AuthService) GenerateAPIKey(
	ctx context.Context,
	req *connect.Request[authpb.GenerateAPIKeyRequest],
) (*connect.Response[authpb.GenerateAPIKeyResponse], error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to begin transaction: %w", err))
	}
	defer tx.Rollback()

	// Generate new API key
	apiKey := uuid.New().String()
	hashedKey := hashKey(apiKey)

	// Insert API key
	_, err = tx.ExecContext(ctx, `
		INSERT INTO api_key (key_hash, device_id, created_at)
		VALUES (?, ?, ?)
	`, hashedKey, req.Msg.DeviceId, time.Now())
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to insert API key: %w", err))
	}

	// Update device last authentication
	_, err = tx.ExecContext(ctx, `
		UPDATE device 
		SET last_seen = ?
		WHERE id = ?
	`, time.Now().Format(time.RFC3339), req.Msg.DeviceId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update device: %w", err))
	}

	if err := tx.Commit(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to commit transaction: %w", err))
	}

	return connect.NewResponse(&authpb.GenerateAPIKeyResponse{
		ApiKey: apiKey,
	}), nil
}

func (s *AuthService) RevokeAPIKey(
	ctx context.Context,
	req *connect.Request[authpb.RevokeAPIKeyRequest],
) (*connect.Response[authpb.RevokeAPIKeyResponse], error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to begin transaction: %w", err))
	}
	defer tx.Rollback()

	// Delete API key
	result, err := tx.ExecContext(ctx, `
		DELETE FROM api_key
		WHERE device_id = ?
	`, req.Msg.DeviceId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to revoke API key: %w", err))
	}

	// Update device status if needed
	if affected, _ := result.RowsAffected(); affected > 0 {
		_, err = tx.ExecContext(ctx, `
			UPDATE device 
			SET status = 'INACTIVE'
			WHERE id IN (
				SELECT device_id 
				FROM api_key
				WHERE device_id = ?
			)
		`, req.Msg.DeviceId)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update device status: %w", err))
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to commit transaction: %w", err))
	}

	return connect.NewResponse(&authpb.RevokeAPIKeyResponse{
		Success: true,
	}), nil
}

func hashKey(key string) string {
	hash := sha256.Sum256([]byte(key))
	return hex.EncodeToString(hash[:])
}
