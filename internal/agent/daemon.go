package agent

import (
	"context"
	"log/slog"

	"connectrpc.com/connect"
	agentpb "fleetd.sh/gen/agent/v1"
	"google.golang.org/protobuf/types/known/emptypb"
)

type DaemonService struct {
	agent *Agent
}

func NewDaemonService(agent *Agent) *DaemonService {
	return &DaemonService{agent: agent}
}

func (s *DaemonService) DeployBinary(
	ctx context.Context,
	req *connect.Request[agentpb.DeployBinaryRequest],
) (*connect.Response[agentpb.DeployBinaryResponse], error) {
	slog.Info("Received deploy binary request",
		"name", req.Msg.Name,
		"size", len(req.Msg.Data))

	if err := s.agent.DeployBinary(req.Msg.Name, req.Msg.Data); err != nil {
		slog.Error("Failed to deploy binary",
			"error", err,
			"name", req.Msg.Name,
			"size", len(req.Msg.Data))
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	slog.Info("Successfully deployed binary",
		"name", req.Msg.Name,
		"size", len(req.Msg.Data))
	return connect.NewResponse(&agentpb.DeployBinaryResponse{}), nil
}

func (s *DaemonService) StartBinary(
	ctx context.Context,
	req *connect.Request[agentpb.StartBinaryRequest],
) (*connect.Response[agentpb.StartBinaryResponse], error) {
	if err := s.agent.StartBinary(req.Msg.Name, req.Msg.Args); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&agentpb.StartBinaryResponse{}), nil
}

func (s *DaemonService) StopBinary(
	ctx context.Context,
	req *connect.Request[agentpb.StopBinaryRequest],
) (*connect.Response[agentpb.StopBinaryResponse], error) {
	if err := s.agent.StopBinary(req.Msg.Name); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&agentpb.StopBinaryResponse{}), nil
}

func (s *DaemonService) ListBinaries(
	ctx context.Context,
	req *connect.Request[agentpb.ListBinariesRequest],
) (*connect.Response[agentpb.ListBinariesResponse], error) {
	binaries, err := s.agent.ListBinaries()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	resp := &agentpb.ListBinariesResponse{
		Binaries: make([]*agentpb.Binary, len(binaries)),
	}

	for i, b := range binaries {
		resp.Binaries[i] = &agentpb.Binary{
			Name:    b.Name,
			Version: b.Version,
			Status:  b.Status,
		}
	}

	return connect.NewResponse(resp), nil
}

func (s *DaemonService) GetDeviceInfo(
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

func (s *DaemonService) ConfigureDevice(
	ctx context.Context,
	req *connect.Request[agentpb.ConfigureDeviceRequest],
) (*connect.Response[agentpb.ConfigureDeviceResponse], error) {
	err := s.agent.Configure(Configuration{
		DeviceName:  req.Msg.DeviceName,
		APIEndpoint: req.Msg.ApiEndpoint,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	info := s.agent.GetDeviceInfo()
	return connect.NewResponse(&agentpb.ConfigureDeviceResponse{
		Success: true,
		ApiKey:  info.APIKey,
	}), nil
}
