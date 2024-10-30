package updateclient

import (
	"context"
	"log/slog"
	"net/http"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	updatepb "fleetd.sh/gen/update/v1"
	updaterpc "fleetd.sh/gen/update/v1/updatev1connect"
)

type Client struct {
	client updaterpc.UpdateServiceClient
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
		client: updaterpc.NewUpdateServiceClient(
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

func (c *Client) CreateUpdatePackage(ctx context.Context, req *updatepb.CreateUpdatePackageRequest) (bool, error) {
	c.logger.With(
		"version", req.Version,
		"deviceTypes", req.DeviceTypes,
	).Info("Creating update package")

	resp, err := c.client.CreateUpdatePackage(ctx, connect.NewRequest(req))
	if err != nil {
		return false, err
	}

	return resp.Msg.Success, nil
}

func (c *Client) GetAvailableUpdates(ctx context.Context, deviceType string, lastUpdateDate *timestamppb.Timestamp) ([]*updatepb.UpdatePackage, error) {
	c.logger.With(
		"deviceType", deviceType,
		"lastUpdateDate", lastUpdateDate.AsTime(),
	).Info("Getting available updates")

	req := connect.NewRequest(&updatepb.GetAvailableUpdatesRequest{
		DeviceType:     deviceType,
		LastUpdateDate: lastUpdateDate,
	})

	resp, err := c.client.GetAvailableUpdates(ctx, req)
	if err != nil {
		return nil, err
	}

	return resp.Msg.Updates, nil
}
