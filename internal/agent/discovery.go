package agent

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	agentpb "fleetd.sh/gen/agent/v1"
	"google.golang.org/protobuf/types/known/emptypb"
)

type DiscoveryService struct {
	agent *Agent
}

func NewDiscoveryService(agent *Agent) *DiscoveryService {
	return &DiscoveryService{agent: agent}
}

func (s *DiscoveryService) GetDeviceInfo(
	ctx context.Context,
	req *connect.Request[emptypb.Empty],
) (*connect.Response[agentpb.GetDeviceInfoResponse], error) {
	info := s.agent.GetDeviceInfo()
	stats := s.agent.GetSystemStats()

	resp := &agentpb.GetDeviceInfoResponse{
		DeviceInfo: &agentpb.DeviceInfo{
			Id:         info.DeviceID,
			Configured: info.Configured,
			DeviceType: info.DeviceType,
			Version:    info.Version,
			System: &agentpb.SystemStats{
				CpuUsage:    stats.CPUUsage,
				MemoryTotal: stats.MemoryTotal,
				MemoryUsed:  stats.MemoryUsed,
				DiskTotal:   stats.DiskTotal,
				DiskUsed:    stats.DiskUsed,
			},
		},
	}

	return connect.NewResponse(resp), nil
}

func (s *DiscoveryService) ConfigureDevice(
	ctx context.Context,
	req *connect.Request[agentpb.ConfigureDeviceRequest],
) (*connect.Response[agentpb.ConfigureDeviceResponse], error) {
	if req.Msg.ApiEndpoint == "" {
		return nil, connect.NewError(
			connect.CodeInvalidArgument,
			fmt.Errorf("api_endpoint is required"),
		)
	}

	err := s.agent.Configure(Configuration{
		DeviceName:  req.Msg.DeviceName,
		APIEndpoint: req.Msg.ApiEndpoint,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	info := s.agent.GetDeviceInfo()
	resp := &agentpb.ConfigureDeviceResponse{
		Success:  true,
		DeviceId: info.DeviceID,
		ApiKey:   info.APIKey,
		Message:  "Device configured successfully",
	}

	return connect.NewResponse(resp), nil
}

func (s *DiscoveryService) UpdateConfig(
	ctx context.Context,
	req *connect.Request[agentpb.UpdateConfigRequest],
) (*connect.Response[agentpb.UpdateConfigResponse], error) {
	// Parse the JSON config and update agent configuration
	config := req.Msg.Config

	// For now, we'll just update the server URL if provided
	// This can be extended to handle more configuration options
	if config != "" {
		// The agent's Configure method will handle the actual configuration update
		// including persisting to disk
		err := s.agent.UpdateConfigFromJSON(config)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal,
				fmt.Errorf("failed to update configuration: %w", err))
		}
	}

	return connect.NewResponse(&agentpb.UpdateConfigResponse{
		Success: true,
		Message: "Configuration updated successfully",
	}), nil
}
