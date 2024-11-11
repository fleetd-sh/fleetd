package fleetd

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	pb "fleetd.sh/gen/fleetd/v1"
	rpc "fleetd.sh/gen/fleetd/v1/fleetpbconnect"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// mockDeviceService implements pb.DeviceServiceServer for testing
type mockDeviceService struct {
	rpc.UnimplementedDeviceServiceHandler
	devices map[string]*pb.Device
}

func newMockDeviceService() *mockDeviceService {
	return &mockDeviceService{
		devices: make(map[string]*pb.Device),
	}
}

func (s *mockDeviceService) Register(ctx context.Context, req *connect.Request[pb.RegisterRequest]) (*connect.Response[pb.RegisterResponse], error) {
	deviceID := "test-device"
	apiKey := "test-api-key"
	s.devices[deviceID] = &pb.Device{
		Id:       deviceID,
		Name:     req.Msg.Name,
		Type:     req.Msg.Type,
		Version:  req.Msg.Version,
		Metadata: req.Msg.Capabilities,
	}
	return connect.NewResponse(&pb.RegisterResponse{
		DeviceId: deviceID,
		ApiKey:   apiKey,
	}), nil
}

func (s *mockDeviceService) Heartbeat(ctx context.Context, req *connect.Request[pb.HeartbeatRequest]) (*connect.Response[pb.HeartbeatResponse], error) {
	device, ok := s.devices[req.Msg.DeviceId]
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("device not found"))
	}
	device.LastSeen = timestamppb.Now()
	return connect.NewResponse(&pb.HeartbeatResponse{HasUpdate: false}), nil
}

func (s *mockDeviceService) ReportStatus(ctx context.Context, req *connect.Request[pb.ReportStatusRequest]) (*connect.Response[pb.ReportStatusResponse], error) {
	_, ok := s.devices[req.Msg.DeviceId]
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("device not found"))
	}
	return connect.NewResponse(&pb.ReportStatusResponse{Success: true}), nil
}

func (s *mockDeviceService) GetDevice(ctx context.Context, req *connect.Request[pb.GetDeviceRequest]) (*connect.Response[pb.GetDeviceResponse], error) {
	device, ok := s.devices[req.Msg.DeviceId]
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("device not found"))
	}
	return connect.NewResponse(&pb.GetDeviceResponse{Device: device}), nil
}

func (s *mockDeviceService) ListDevices(ctx context.Context, req *connect.Request[pb.ListDevicesRequest]) (*connect.Response[pb.ListDevicesResponse], error) {
	var devices []*pb.Device
	for _, device := range s.devices {
		if req.Msg.Type != "" && device.Type != req.Msg.Type {
			continue
		}
		if req.Msg.Version != "" && device.Version != req.Msg.Version {
			continue
		}
		devices = append(devices, device)
	}
	return connect.NewResponse(&pb.ListDevicesResponse{Devices: devices}), nil
}

func (s *mockDeviceService) DeleteDevice(ctx context.Context, req *connect.Request[pb.DeleteDeviceRequest]) (*connect.Response[pb.DeleteDeviceResponse], error) {
	if _, ok := s.devices[req.Msg.DeviceId]; !ok {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("device not found"))
	}
	delete(s.devices, req.Msg.DeviceId)
	return connect.NewResponse(&pb.DeleteDeviceResponse{
		Success: true,
	}), nil
}

func setupTestServer() (*httptest.Server, rpc.DeviceServiceHandler) {
	mock := newMockDeviceService()
	mux := http.NewServeMux()
	path, handler := rpc.NewDeviceServiceHandler(mock)
	mux.Handle(path, handler)

	server := httptest.NewServer(mux)
	return server, mock
}

func TestClient_Device(t *testing.T) {
	server, _ := setupTestServer()
	defer server.Close()

	// Create client
	client := NewClient(server.URL, ClientOptions{
		DefaultTimeout: time.Second,
	})

	ctx := context.Background()

	// Test device registration
	registerResp, err := client.Device().Register(ctx, RegisterRequest{
		Name:    "test-device",
		Type:    "raspberry-pi",
		Version: "1.0.0",
		Capabilities: Metadata{
			"feature1": "enabled",
		},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, registerResp.DeviceID)
	assert.NotEmpty(t, registerResp.APIKey)

	// Test device heartbeat
	heartbeatResp, err := client.Device().Heartbeat(ctx, HeartbeatRequest{
		DeviceID: registerResp.DeviceID,
		Metrics: Metadata{
			"cpu": "50%",
		},
	})
	require.NoError(t, err)
	assert.False(t, heartbeatResp.HasUpdate)

	// Test report status
	statusResp, err := client.Device().ReportStatus(ctx, ReportStatusRequest{
		DeviceID: registerResp.DeviceID,
		Status:   "healthy",
		Metrics: Metadata{
			"memory": "2GB",
		},
	})
	require.NoError(t, err)
	assert.True(t, statusResp.Success)

	// Test get device
	device, err := client.Device().GetDevice(ctx, GetDeviceRequest{
		DeviceID: registerResp.DeviceID,
	})
	require.NoError(t, err)
	assert.Equal(t, registerResp.DeviceID, device.ID)
	assert.Equal(t, "test-device", device.Name)
	assert.Equal(t, "raspberry-pi", device.Type)
	assert.Equal(t, "1.0.0", device.Version)
	assert.Equal(t, "enabled", device.Metadata["feature1"])

	// Test list devices
	listResp, err := client.Device().ListDevices(ctx, ListDevicesRequest{
		Type: "raspberry-pi",
	})
	require.NoError(t, err)
	assert.Len(t, listResp.Devices, 1)
	assert.Equal(t, registerResp.DeviceID, listResp.Devices[0].ID)

	// Test delete device
	err = client.Device().DeleteDevice(ctx, DeleteDeviceRequest{
		DeviceID: registerResp.DeviceID,
	})
	require.NoError(t, err)

	// Verify device was deleted
	_, err = client.Device().GetDevice(ctx, GetDeviceRequest{
		DeviceID: registerResp.DeviceID,
	})
	assert.Error(t, err)
}

func TestClient_Errors(t *testing.T) {
	server, _ := setupTestServer()
	defer server.Close()

	client := NewClient(server.URL, ClientOptions{
		DefaultTimeout: time.Second,
	})

	ctx := context.Background()

	// Test not found error
	_, err := client.Device().GetDevice(ctx, GetDeviceRequest{
		DeviceID: "nonexistent",
	})
	require.Error(t, err)
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))

	// Test timeout error
	timeoutCtx, cancel := context.WithTimeout(ctx, time.Nanosecond)
	defer cancel()
	time.Sleep(time.Millisecond)
	_, err = client.Device().GetDevice(timeoutCtx, GetDeviceRequest{
		DeviceID: "test",
	})
	require.Error(t, err)
	assert.Equal(t, connect.CodeDeadlineExceeded, connect.CodeOf(err))
}
