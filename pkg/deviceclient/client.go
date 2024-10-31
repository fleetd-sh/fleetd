package deviceclient

import (
	"context"
	"net/http"
	"time"

	"log/slog"

	"connectrpc.com/connect"
	devicepb "fleetd.sh/gen/device/v1"
	devicerpc "fleetd.sh/gen/device/v1/devicev1connect"
)

type Device struct {
	ID       string
	APIKey   string
	Name     string
	Type     string
	Status   string
	LastSeen time.Time
	Version  string
}

type NewDevice struct {
	Name    string
	Type    string
	Version string
}

// Client represents a client for the Device Management API.
type Client struct {
	client devicerpc.DeviceServiceClient
	logger *slog.Logger
}

type ClientOption func(*Client)

func WithLogger(logger *slog.Logger) ClientOption {
	return func(c *Client) {
		c.logger = logger
	}
}

// NewClient creates a new Device Management API client.
//
// baseURL is the base URL of the API server.
// opts are optional client options, such as WithLogger.
func NewClient(baseURL string, opts ...ClientOption) *Client {
	httpClient := &http.Client{
		Timeout: 10 * time.Second,
	}

	c := &Client{
		client: devicerpc.NewDeviceServiceClient(httpClient, baseURL),
		logger: slog.Default(),
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// RegisterDevice registers a new device with the given name and type.
//
// It returns the device ID and API key on success, or an error if the registration fails.
func (c *Client) RegisterDevice(ctx context.Context, device *NewDevice) (deviceID string, apiKey string, err error) {
	c.logger.With("device", device).Info("Registering device")

	req := connect.NewRequest(&devicepb.RegisterDeviceRequest{
		Name:    device.Name,
		Type:    device.Type,
		Version: device.Version,
	})

	resp, err := c.client.RegisterDevice(ctx, req)
	if err != nil {
		return "", "", err
	}

	return resp.Msg.DeviceId, resp.Msg.ApiKey, nil
}

func (c *Client) UnregisterDevice(ctx context.Context, deviceID string) (bool, error) {
	c.logger.With("deviceID", deviceID).Info("Unregistering device")
	req := connect.NewRequest(&devicepb.UnregisterDeviceRequest{
		DeviceId: deviceID,
	})

	resp, err := c.client.UnregisterDevice(ctx, req)
	if err != nil {
		return false, err
	}

	return resp.Msg.Success, nil
}

func protoToDevice(d *devicepb.Device) *Device {
	return &Device{
		ID:       d.Id,
		Name:     d.Name,
		Type:     d.Type,
		Status:   d.Status,
		LastSeen: d.LastSeen.AsTime(),
		Version:  d.Version,
	}
}

func (c *Client) GetDevice(ctx context.Context, deviceID string) (*Device, error) {
	c.logger.With("deviceID", deviceID).Info("Getting device")
	req := connect.NewRequest(&devicepb.GetDeviceRequest{
		DeviceId: deviceID,
	})

	resp, err := c.client.GetDevice(ctx, req)
	if err != nil {
		return nil, err
	}

	return protoToDevice(resp.Msg.Device), nil
}

func (c *Client) ListDevices(ctx context.Context) (<-chan *Device, <-chan error) {
	c.logger.Info("Listing devices")
	req := connect.NewRequest(&devicepb.ListDevicesRequest{})

	stream, err := c.client.ListDevices(ctx, req)
	if err != nil {
		errCh := make(chan error, 1)
		errCh <- err
		return nil, errCh
	}

	deviceCh := make(chan *Device)
	errCh := make(chan error, 1)

	go func() {
		defer close(deviceCh)
		defer close(errCh)

		for stream.Receive() {
			deviceCh <- protoToDevice(stream.Msg().Device)
		}

		if err := stream.Err(); err != nil {
			errCh <- err
		}
	}()

	return deviceCh, errCh
}

func (c *Client) UpdateDeviceStatus(ctx context.Context, deviceID, status string) (bool, error) {
	c.logger.With("deviceID", deviceID, "status", status).Info("Updating device status")
	req := connect.NewRequest(&devicepb.UpdateDeviceStatusRequest{
		DeviceId: deviceID,
		Status:   status,
	})

	resp, err := c.client.UpdateDeviceStatus(ctx, req)
	if err != nil {
		return false, err
	}

	return resp.Msg.Success, nil
}
