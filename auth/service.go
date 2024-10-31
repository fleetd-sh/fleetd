package auth

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"

	"connectrpc.com/connect"
	"github.com/segmentio/ksuid"

	authpb "fleetd.sh/gen/auth/v1"
	"fleetd.sh/internal/telemetry"
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
	defer telemetry.TrackSQLOperation(ctx, "Authenticate")(nil)

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
	defer telemetry.TrackSQLOperation(ctx, "GenerateAPIKey")(nil)

	// Generate API key using KSUID for uniqueness
	apiKey := ksuid.New().String()
	keyHash := hashAPIKey(apiKey)

	// Store API key hash in database
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO api_key (key_hash, device_id)
		VALUES (?, ?)`,
		keyHash, req.Msg.DeviceId,
	)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to store API key: %w", err))
	}

	return connect.NewResponse(&authpb.GenerateAPIKeyResponse{
		ApiKey: apiKey,
	}), nil
}

func (s *AuthService) RevokeAPIKey(
	ctx context.Context,
	req *connect.Request[authpb.RevokeAPIKeyRequest],
) (*connect.Response[authpb.RevokeAPIKeyResponse], error) {
	defer telemetry.TrackSQLOperation(ctx, "RevokeAPIKey")(nil)
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{
		Isolation: sql.LevelSerializable,
	})
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

func hashAPIKey(key string) string {
	hash := sha256.Sum256([]byte(key))
	return hex.EncodeToString(hash[:])
}
