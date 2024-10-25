package auth

import (
	"context"
	"database/sql"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	authpb "fleetd.sh/gen/auth/v1"
)

type AuthService struct {
	db *sql.DB
}

func NewAuthService(db *sql.DB) (*AuthService, error) {
	return &AuthService{
		db: db,
	}, nil
}

func (s *AuthService) Authenticate(
	ctx context.Context,
	req *connect.Request[authpb.AuthenticateRequest],
) (*connect.Response[authpb.AuthenticateResponse], error) {
	var deviceID string
	err := s.db.QueryRowContext(ctx, "SELECT device_id FROM api_key WHERE api_key = ?", req.Msg.ApiKey).Scan(&deviceID)
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
	apiKey := uuid.New().String()

	_, err := s.db.ExecContext(ctx, "INSERT INTO api_key (api_key, device_id) VALUES (?, ?)", apiKey, req.Msg.DeviceId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to insert API key: %w", err))
	}

	return connect.NewResponse(&authpb.GenerateAPIKeyResponse{
		ApiKey: apiKey,
	}), nil
}

func (s *AuthService) RevokeAPIKey(
	ctx context.Context,
	req *connect.Request[authpb.RevokeAPIKeyRequest],
) (*connect.Response[authpb.RevokeAPIKeyResponse], error) {
	result, err := s.db.ExecContext(ctx, "DELETE FROM api_key WHERE device_id = ?", req.Msg.DeviceId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete API key: %w", err))
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get rows affected: %w", err))
	}

	return connect.NewResponse(&authpb.RevokeAPIKeyResponse{
		Success: rowsAffected > 0,
	}), nil
}
