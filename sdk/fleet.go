package sdk

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	commonpb "fleetd.sh/gen/proto/common/v1"
	controlpb "fleetd.sh/gen/proto/control/v1"
	"fleetd.sh/gen/proto/control/v1/controlv1connect"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// FleetClient provides fleet management operations
type FleetClient struct {
	client  controlv1connect.FleetServiceClient
	timeout time.Duration
}

// GetStats retrieves fleet statistics
func (c *FleetClient) GetStats(ctx context.Context, organizationID string) (*controlpb.GetFleetStatsResponse, error) {
	req := connect.NewRequest(&controlpb.GetFleetStatsRequest{
		OrganizationId: organizationID,
	})

	resp, err := c.client.GetFleetStats(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get fleet stats: %w", err)
	}

	return resp.Msg, nil
}

// ListDevices retrieves a list of devices with filters
func (c *FleetClient) ListDevices(ctx context.Context, opts ListDevicesOptions) (*controlpb.ListDevicesResponse, error) {
	req := connect.NewRequest(&controlpb.ListDevicesRequest{
		OrganizationId: opts.OrganizationID,
		GroupIds:       opts.GroupIDs,
		DeviceTypes:    opts.DeviceTypes,
		Statuses:       opts.Statuses,
		Labels:         opts.Labels,
		SearchQuery:    opts.SearchQuery,
		PageSize:       opts.PageSize,
		PageToken:      opts.PageToken,
		OrderBy:        opts.OrderBy,
		Descending:     opts.Descending,
	})

	resp, err := c.client.ListDevices(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to list devices: %w", err)
	}

	return resp.Msg, nil
}

// GetDevice retrieves a specific device
func (c *FleetClient) GetDevice(ctx context.Context, deviceID string, includeMetrics bool) (*controlpb.GetDeviceResponse, error) {
	req := connect.NewRequest(&controlpb.GetDeviceRequest{
		DeviceId:       deviceID,
		IncludeMetrics: includeMetrics,
	})

	resp, err := c.client.GetDevice(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get device: %w", err)
	}

	return resp.Msg, nil
}

// UpdateDevice updates device configuration
func (c *FleetClient) UpdateDevice(ctx context.Context, deviceID string, updates UpdateDeviceOptions) (*controlpb.UpdateDeviceResponse, error) {
	req := connect.NewRequest(&controlpb.UpdateDeviceRequest{
		DeviceId:         deviceID,
		Labels:           updates.Labels,
		Metadata:         updates.Metadata,
		AddToGroups:      updates.AddToGroups,
		RemoveFromGroups: updates.RemoveFromGroups,
	})

	resp, err := c.client.UpdateDevice(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to update device: %w", err)
	}

	return resp.Msg, nil
}

// DeleteDevice removes a device from the fleet
func (c *FleetClient) DeleteDevice(ctx context.Context, deviceID string, force bool) error {
	req := connect.NewRequest(&controlpb.DeleteDeviceRequest{
		DeviceId: deviceID,
		Force:    force,
	})

	_, err := c.client.DeleteDevice(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to delete device: %w", err)
	}

	return nil
}

// ExecuteCommand executes a command on devices
func (c *FleetClient) ExecuteCommand(ctx context.Context, opts ExecuteCommandOptions) (*controlpb.ExecuteCommandResponse, error) {
	var executeAt *timestamppb.Timestamp
	if opts.ExecuteAt != nil {
		executeAt = timestamppb.New(*opts.ExecuteAt)
	}

	req := connect.NewRequest(&controlpb.ExecuteCommandRequest{
		DeviceIds:      opts.DeviceIDs,
		GroupIds:       opts.GroupIDs,
		Command:        opts.Command,
		ExecuteAt:      executeAt,
		TimeoutSeconds: opts.TimeoutSeconds,
	})

	resp, err := c.client.ExecuteCommand(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute command: %w", err)
	}

	return resp.Msg, nil
}

// CreateGroup creates a device group
func (c *FleetClient) CreateGroup(ctx context.Context, opts CreateGroupOptions) (*controlpb.CreateGroupResponse, error) {
	req := connect.NewRequest(&controlpb.CreateGroupRequest{
		Name:        opts.Name,
		Description: opts.Description,
		Labels:      opts.Labels,
		DeviceIds:   opts.DeviceIDs,
	})

	resp, err := c.client.CreateGroup(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to create group: %w", err)
	}

	return resp.Msg, nil
}

// Options types

// ListDevicesOptions contains options for listing devices
type ListDevicesOptions struct {
	OrganizationID string
	GroupIDs       []string
	DeviceTypes    []string
	Statuses       []commonpb.DeviceStatus
	Labels         map[string]string
	SearchQuery    string
	PageSize       int32
	PageToken      string
	OrderBy        string
	Descending     bool
}

// UpdateDeviceOptions contains options for updating a device
type UpdateDeviceOptions struct {
	Labels           map[string]string
	Metadata         map[string]string
	AddToGroups      []string
	RemoveFromGroups []string
}

// ExecuteCommandOptions contains options for executing commands
type ExecuteCommandOptions struct {
	DeviceIDs      []string
	GroupIDs       []string
	Command        *controlpb.Command
	ExecuteAt      *time.Time
	TimeoutSeconds int32
}

// CreateGroupOptions contains options for creating a group
type CreateGroupOptions struct {
	Name        string
	Description string
	Labels      map[string]string
	DeviceIDs   []string
}
