package fleetd

import (
	"context"
	"errors"
	"time"

	pb "fleetd.sh/gen/fleetd/v1"
	rpc "fleetd.sh/gen/fleetd/v1/fleetpbconnect"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// DeviceClient is a client for the device service
type DeviceClient struct {
	client  rpc.DeviceServiceClient
	timeout time.Duration
}

// Device represents a device
type Device struct {
	ID       string
	Name     string
	Type     string
	Version  string
	Metadata Metadata
	LastSeen time.Time
}

// fromProto converts a protobuf Device to Device
func fromProtoDevice(d *pb.Device) *Device {
	if d == nil {
		return nil
	}
	return &Device{
		ID:       d.Id,
		Name:     d.Name,
		Type:     d.Type,
		Version:  d.Version,
		Metadata: fromProtoMetadata(d.Metadata),
		LastSeen: d.LastSeen.AsTime(),
	}
}

// toProto converts a Device to protobuf
func (d *Device) toProto() *pb.Device {
	return &pb.Device{
		Id:       d.ID,
		Name:     d.Name,
		Type:     d.Type,
		Version:  d.Version,
		Metadata: d.Metadata.toProto(),
		LastSeen: timestamppb.New(d.LastSeen),
	}
}

// RegisterRequest represents a device registration request
type RegisterRequest struct {
	Name         string
	Type         string
	Version      string
	Capabilities Metadata
}

// RegisterResponse represents a device registration response
type RegisterResponse struct {
	DeviceID string
	APIKey   string
}

// Register registers a new device
func (c *DeviceClient) Register(ctx context.Context, req RegisterRequest) (*RegisterResponse, error) {
	ctx, cancel := withTimeout(ctx, c.timeout)
	defer cancel()

	resp, err := c.client.Register(ctx, &connect.Request[pb.RegisterRequest]{
		Msg: &pb.RegisterRequest{
			Name:         req.Name,
			Type:         req.Type,
			Version:      req.Version,
			Capabilities: req.Capabilities.toProto(),
		},
	})
	if err != nil {
		return nil, err
	}

	return &RegisterResponse{
		DeviceID: resp.Msg.DeviceId,
		APIKey:   resp.Msg.ApiKey,
	}, nil
}

// HeartbeatRequest represents a device heartbeat request
type HeartbeatRequest struct {
	DeviceID string
	Metrics  Metadata
}

// HeartbeatResponse represents a device heartbeat response
type HeartbeatResponse struct {
	HasUpdate bool
}

// Heartbeat sends a device heartbeat
func (c *DeviceClient) Heartbeat(ctx context.Context, req HeartbeatRequest) (*HeartbeatResponse, error) {
	ctx, cancel := withTimeout(ctx, c.timeout)
	defer cancel()

	resp, err := c.client.Heartbeat(ctx, &connect.Request[pb.HeartbeatRequest]{
		Msg: &pb.HeartbeatRequest{
			DeviceId: req.DeviceID,
			Metrics:  req.Metrics.toProto(),
		},
	})
	if err != nil {
		return nil, err
	}

	return &HeartbeatResponse{
		HasUpdate: resp.Msg.HasUpdate,
	}, nil
}

// ReportStatusRequest represents a device status report request
type ReportStatusRequest struct {
	DeviceID string
	Status   string
	Metrics  Metadata
}

// ReportStatusResponse represents a device status report response
type ReportStatusResponse struct {
	Success bool
}

// ReportStatus reports device status
func (c *DeviceClient) ReportStatus(ctx context.Context, req ReportStatusRequest) (*ReportStatusResponse, error) {
	ctx, cancel := withTimeout(ctx, c.timeout)
	defer cancel()

	resp, err := c.client.ReportStatus(ctx, &connect.Request[pb.ReportStatusRequest]{
		Msg: &pb.ReportStatusRequest{
			DeviceId: req.DeviceID,
			Status:   req.Status,
			Metrics:  req.Metrics.toProto(),
		},
	})
	if err != nil {
		return nil, err
	}

	return &ReportStatusResponse{
		Success: resp.Msg.Success,
	}, nil
}

// GetDeviceRequest represents a get device request
type GetDeviceRequest struct {
	DeviceID string
}

// GetDevice gets a device by ID
func (c *DeviceClient) GetDevice(ctx context.Context, req GetDeviceRequest) (*Device, error) {
	ctx, cancel := withTimeout(ctx, c.timeout)
	defer cancel()

	resp, err := c.client.GetDevice(ctx, &connect.Request[pb.GetDeviceRequest]{
		Msg: &pb.GetDeviceRequest{
			DeviceId: req.DeviceID,
		},
	})
	if err != nil {
		return nil, err
	}

	return fromProtoDevice(resp.Msg.Device), nil
}

// ListDevicesRequest represents a list devices request
type ListDevicesRequest struct {
	Type     string
	Version  string
	Status   string
	PageSize int32
	Token    string
}

// ListDevicesResponse represents a list devices response
type ListDevicesResponse struct {
	Devices       []*Device
	NextPageToken string
}

// ListDevices lists devices
func (c *DeviceClient) ListDevices(ctx context.Context, req ListDevicesRequest) (*ListDevicesResponse, error) {
	ctx, cancel := withTimeout(ctx, c.timeout)
	defer cancel()

	resp, err := c.client.ListDevices(ctx, &connect.Request[pb.ListDevicesRequest]{
		Msg: &pb.ListDevicesRequest{
			Type:      req.Type,
			Version:   req.Version,
			Status:    req.Status,
			PageSize:  req.PageSize,
			PageToken: req.Token,
		},
	})
	if err != nil {
		return nil, err
	}

	devices := make([]*Device, len(resp.Msg.Devices))
	for i, d := range resp.Msg.Devices {
		devices[i] = fromProtoDevice(d)
	}

	return &ListDevicesResponse{
		Devices:       devices,
		NextPageToken: resp.Msg.NextPageToken,
	}, nil
}

// DeleteDeviceRequest represents a delete device request
type DeleteDeviceRequest struct {
	DeviceID string
}

// DeleteDevice deletes a device
func (c *DeviceClient) DeleteDevice(ctx context.Context, req DeleteDeviceRequest) error {
	ctx, cancel := withTimeout(ctx, c.timeout)
	defer cancel()

	resp, err := c.client.DeleteDevice(ctx, &connect.Request[pb.DeleteDeviceRequest]{
		Msg: &pb.DeleteDeviceRequest{
			DeviceId: req.DeviceID,
		},
	})
	if err != nil {
		return err
	}
	if !resp.Msg.Success {
		return connect.NewError(connect.CodeInternal, errors.New("failed to delete device"))
	}

	return nil
}
