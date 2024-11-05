package daemon

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	discoverypb "fleetd.sh/gen/discovery/v1"
	"fleetd.sh/internal/version"
	"fleetd.sh/pkg/deviceclient"
)

type DiscoveryService struct {
	daemon *FleetDaemon
}

func NewDiscoveryService(d *FleetDaemon) *DiscoveryService {
	return &DiscoveryService{
		daemon: d,
	}
}

func (s *DiscoveryService) GetDeviceInfo(
	ctx context.Context,
	req *connect.Request[discoverypb.GetDeviceInfoRequest],
) (*connect.Response[discoverypb.GetDeviceInfoResponse], error) {
	return connect.NewResponse(&discoverypb.GetDeviceInfoResponse{
		DeviceId:   s.daemon.GetDeviceID(),
		Configured: s.daemon.IsConfigured(),
	}), nil
}

func (s *DiscoveryService) ConfigureDevice(
	ctx context.Context,
	req *connect.Request[discoverypb.ConfigureDeviceRequest],
) (*connect.Response[discoverypb.ConfigureDeviceResponse], error) {
	if s.daemon.IsConfigured() {
		return connect.NewResponse(&discoverypb.ConfigureDeviceResponse{
			Success: false,
			Message: "device is already configured",
		}), nil
	}

	client := deviceclient.NewClient(req.Msg.ApiEndpoint)
	deviceID, apiKey, err := client.RegisterDevice(ctx, &deviceclient.NewDevice{
		Name:    req.Msg.DeviceName,
		Type:    s.daemon.config.DeviceType,
		Version: version.GetVersion(),
	})
	if err != nil {
		return connect.NewResponse(&discoverypb.ConfigureDeviceResponse{
			Success: false,
			Message: fmt.Sprintf("failed to register device: %v", err),
		}), nil
	}

	s.daemon.config.DeviceID = deviceID
	s.daemon.config.APIKey = apiKey
	s.daemon.config.APIEndpoint = req.Msg.ApiEndpoint

	if err := s.daemon.config.SaveConfig(); err != nil {
		return connect.NewResponse(&discoverypb.ConfigureDeviceResponse{
			Success: false,
			Message: fmt.Sprintf("failed to save configuration: %v", err),
		}), nil
	}

	return connect.NewResponse(&discoverypb.ConfigureDeviceResponse{
		Success:  true,
		Message:  "device configured successfully",
		DeviceId: deviceID,
	}), nil
}
