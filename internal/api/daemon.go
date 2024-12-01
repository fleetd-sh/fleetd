package api

import (
	"bytes"
	"context"

	"connectrpc.com/connect"
	agentpb "fleetd.sh/gen/agent/v1"
	"fleetd.sh/internal/agent"
)

type DaemonService struct {
	agent *agent.Agent
}

func NewDaemonService(agent *agent.Agent) *DaemonService {
	return &DaemonService{agent: agent}
}

func (s *DaemonService) DeployBinary(
	ctx context.Context,
	req *connect.Request[agentpb.DeployBinaryRequest],
) (*connect.Response[agentpb.DeployBinaryResponse], error) {
	if err := s.agent.DeployBinary(req.Msg.Name, bytes.NewReader(req.Msg.Data)); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
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
