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

type ClientOption func(*Client)

func WithLogger(logger *slog.Logger) ClientOption {
	return func(c *Client) {
		c.logger = logger
	}
}

func NewClient(baseURL string, opts ...ClientOption) *Client {
	c := &Client{
		client: authrpc.NewAuthServiceClient(
			http.DefaultClient,
			baseURL,
		),
		logger: slog.Default(),
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

func (c *Client) Authenticate(ctx context.Context, apiKey string) (bool, string, error) {
	c.logger.With("apiKey", apiKey).Info("Authenticating")
	req := connect.NewRequest(&authpb.AuthenticateRequest{
		ApiKey: apiKey,
	})

	resp, err := c.client.Authenticate(ctx, req)
	if err != nil {
		return false, "", err
	}

	return resp.Msg.Authenticated, resp.Msg.DeviceId, nil
}

func (c *Client) GenerateAPIKey(ctx context.Context, deviceID string) (string, error) {
	c.logger.With("deviceID", deviceID).Info("Generating API key")
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
	c.logger.With("deviceID", deviceID).Info("Revoking API key")
	req := connect.NewRequest(&authpb.RevokeAPIKeyRequest{
		DeviceId: deviceID,
	})

	resp, err := c.client.RevokeAPIKey(ctx, req)
	if err != nil {
		return false, err
	}

	return resp.Msg.Success, nil
}
