package authclient

import (
	"context"
	"log/slog"
	"net/http"

	"connectrpc.com/connect"

	authpb "fleetd.sh/gen/auth/v1"
	authrpc "fleetd.sh/gen/auth/v1/authv1connect"
)

type Client struct {
	client authrpc.AuthServiceClient
	logger *slog.Logger
}

type AuthResult struct {
	Authenticated bool
	DeviceID      string
}

func NewClient(baseURL string) *Client {
	return &Client{
		client: authrpc.NewAuthServiceClient(
			http.DefaultClient,
			baseURL,
		),
		logger: slog.Default(),
	}
}

func (c *Client) Authenticate(ctx context.Context, apiKey string) (*AuthResult, error) {
	req := connect.NewRequest(&authpb.AuthenticateRequest{
		ApiKey: apiKey,
	})

	resp, err := c.client.Authenticate(ctx, req)
	if err != nil {
		return nil, err
	}

	return &AuthResult{
		Authenticated: resp.Msg.Authenticated,
		DeviceID:      resp.Msg.DeviceId,
	}, nil
}

func (c *Client) GenerateAPIKey(ctx context.Context, deviceID string) (string, error) {
	req := connect.NewRequest(&authpb.GenerateAPIKeyRequest{
		DeviceId: deviceID,
	})

	resp, err := c.client.GenerateAPIKey(ctx, req)
	if err != nil {
		return "", err
	}

	return resp.Msg.ApiKey, nil
}

func (c *Client) RevokeAPIKey(ctx context.Context, deviceID string) (bool, error) {
	req := connect.NewRequest(&authpb.RevokeAPIKeyRequest{
		DeviceId: deviceID,
	})

	resp, err := c.client.RevokeAPIKey(ctx, req)
	if err != nil {
		return false, err
	}

	return resp.Msg.Success, nil
}
