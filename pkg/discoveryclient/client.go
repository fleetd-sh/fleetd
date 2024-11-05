package discoveryclient

import (
	"context"
	"log/slog"
	"net/http"

	"connectrpc.com/connect"

	discoverypb "fleetd.sh/gen/discovery/v1"
	discoveryrpc "fleetd.sh/gen/discovery/v1/discoveryv1connect"
)

type Client struct {
	client discoveryrpc.DiscoveryServiceClient
	logger *slog.Logger
}

type Option func(*Client)

func WithLogger(logger *slog.Logger) Option {
	return func(c *Client) {
		c.logger = logger
	}
}

// NewClient creates a new discovery client
func NewClient(baseURL string, opts ...Option) *Client {
	c := &Client{
		client: discoveryrpc.NewDiscoveryServiceClient(
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

// Device represents a device to be configured
type Device struct {
	Name string
	ID   string // Temporary ID during discovery
}

// Configuration contains the fleet service URLs
type Configuration struct {
	APIEndpoint string
}

// ConfigureDevice configures a device with the fleet service
func (c *Client) ConfigureDevice(ctx context.Context, device Device, config Configuration) (bool, error) {
	req := &discoverypb.ConfigureDeviceRequest{
		DeviceName:  device.Name,
		ApiEndpoint: config.APIEndpoint,
	}

	resp, err := c.client.ConfigureDevice(ctx, connect.NewRequest(req))
	if err != nil || !resp.Msg.Success {
		return false, err
	}

	return resp.Msg.Success, nil
}
