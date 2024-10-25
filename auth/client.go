package auth

import (
	"context"
	"net/http"

	"connectrpc.com/connect"

	authpb "fleetd.sh/gen/auth/v1"
	authrpc "fleetd.sh/gen/auth/v1/authv1connect"
)

type AuthClient struct {
	client authrpc.AuthServiceClient
}

func NewAuthClient(baseURL string) *AuthClient {
	return &AuthClient{
		client: authrpc.NewAuthServiceClient(
			http.DefaultClient,
			baseURL,
		),
	}
}

func (c *AuthClient) Authenticate(ctx context.Context, apiKey string) (bool, string, error) {
	req := connect.NewRequest(&authpb.AuthenticateRequest{
		ApiKey: apiKey,
	})

	resp, err := c.client.Authenticate(ctx, req)
	if err != nil {
		return false, "", err
	}

	return resp.Msg.Authenticated, resp.Msg.DeviceId, nil
}

func (c *AuthClient) GenerateAPIKey(ctx context.Context, deviceID string) (string, error) {
	req := connect.NewRequest(&authpb.GenerateAPIKeyRequest{
		DeviceId: deviceID,
	})

	resp, err := c.client.GenerateAPIKey(ctx, req)
	if err != nil {
		return "", err
	}

	return resp.Msg.ApiKey, nil
}

func (c *AuthClient) RevokeAPIKey(ctx context.Context, deviceID string) (bool, error) {
	req := connect.NewRequest(&authpb.RevokeAPIKeyRequest{
		DeviceId: deviceID,
	})

	resp, err := c.client.RevokeAPIKey(ctx, req)
	if err != nil {
		return false, err
	}

	return resp.Msg.Success, nil
}
