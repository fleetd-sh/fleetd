package deviceclient_test

import (
	"context"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	devicepb "fleetd.sh/gen/device/v1"
	devicerpc "fleetd.sh/gen/device/v1/devicev1connect"
	"fleetd.sh/pkg/deviceclient"
)

type mockDeviceService struct {
	devicerpc.UnimplementedDeviceServiceHandler
	registerDeviceFunc     func(context.Context, *devicepb.RegisterDeviceRequest) (*devicepb.RegisterDeviceResponse, error)
	updateDeviceStatusFunc func(context.Context, *devicepb.UpdateDeviceStatusRequest) (*devicepb.UpdateDeviceStatusResponse, error)
	updateDeviceFunc       func(context.Context, *devicepb.UpdateDeviceRequest) (*devicepb.UpdateDeviceResponse, error)
	listDevicesFunc        func(context.Context, *devicepb.ListDevicesRequest, *connect.ServerStream[devicepb.ListDevicesResponse]) error
}

func (m *mockDeviceService) RegisterDevice(ctx context.Context, req *connect.Request[devicepb.RegisterDeviceRequest]) (*connect.Response[devicepb.RegisterDeviceResponse], error) {
	if m.registerDeviceFunc != nil {
		resp, err := m.registerDeviceFunc(ctx, req.Msg)
		return connect.NewResponse(resp), err
	}
	return connect.NewResponse(&devicepb.RegisterDeviceResponse{
		DeviceId: "test-device-001",
		ApiKey:   "test-api-key",
	}), nil
}

func (m *mockDeviceService) UpdateDevice(ctx context.Context, req *connect.Request[devicepb.UpdateDeviceRequest]) (*connect.Response[devicepb.UpdateDeviceResponse], error) {
	if m.updateDeviceFunc != nil {
		resp, err := m.updateDeviceFunc(ctx, req.Msg)
		return connect.NewResponse(resp), err
	}
	return connect.NewResponse(&devicepb.UpdateDeviceResponse{
		Success: true,
	}), nil
}

func (m *mockDeviceService) ListDevices(ctx context.Context, req *connect.Request[devicepb.ListDevicesRequest], stream *connect.ServerStream[devicepb.ListDevicesResponse]) error {
	if m.listDevicesFunc != nil {
		return m.listDevicesFunc(ctx, req.Msg, stream)
	}
	return nil
}

func (m *mockDeviceService) UpdateDeviceStatus(ctx context.Context, req *connect.Request[devicepb.UpdateDeviceStatusRequest]) (*connect.Response[devicepb.UpdateDeviceStatusResponse], error) {
	if m.updateDeviceFunc != nil {
		resp, err := m.updateDeviceStatusFunc(ctx, req.Msg)
		return connect.NewResponse(resp), err
	}
	return connect.NewResponse(&devicepb.UpdateDeviceStatusResponse{
		Success: true,
	}), nil
}

func TestDeviceClient_Unit(t *testing.T) {
	testCases := []struct {
		name          string
		setupMock     func(*mockDeviceService)
		testFunc      func(*testing.T, *deviceclient.Client)
		expectedError string
	}{
		{
			name: "RegisterDevice success",
			setupMock: func(m *mockDeviceService) {
				m.registerDeviceFunc = func(_ context.Context, req *devicepb.RegisterDeviceRequest) (*devicepb.RegisterDeviceResponse, error) {
					return &devicepb.RegisterDeviceResponse{
						DeviceId: "device-123",
						ApiKey:   "api-key-123",
					}, nil
				}
			},
			testFunc: func(t *testing.T, client *deviceclient.Client) {
				deviceID, apiKey, err := client.RegisterDevice(context.Background(), "Test Device", "SENSOR")
				require.NoError(t, err)
				assert.Equal(t, "device-123", deviceID)
				assert.Equal(t, "api-key-123", apiKey)
			},
		},
		{
			name: "UpdateDeviceStatus success",
			setupMock: func(m *mockDeviceService) {
				m.updateDeviceStatusFunc = func(_ context.Context, req *devicepb.UpdateDeviceStatusRequest) (*devicepb.UpdateDeviceStatusResponse, error) {
					return &devicepb.UpdateDeviceStatusResponse{
						Success: true,
					}, nil
				}
			},
			testFunc: func(t *testing.T, client *deviceclient.Client) {
				success, err := client.UpdateDeviceStatus(context.Background(), "device-123", "ACTIVE")
				require.NoError(t, err)
				assert.True(t, success)
			},
		},
		{
			name: "ListDevices success",
			setupMock: func(m *mockDeviceService) {
				m.listDevicesFunc = func(_ context.Context, req *devicepb.ListDevicesRequest, stream *connect.ServerStream[devicepb.ListDevicesResponse]) error {
					devices := []*devicepb.Device{
						{
							Id:       "device-1",
							Name:     "Device 1",
							Type:     "SENSOR",
							Status:   "ACTIVE",
							LastSeen: timestamppb.Now(),
						},
						{
							Id:       "device-2",
							Name:     "Device 2",
							Type:     "ACTUATOR",
							Status:   "INACTIVE",
							LastSeen: timestamppb.Now(),
						},
					}

					for _, device := range devices {
						if err := stream.Send(&devicepb.ListDevicesResponse{Device: device}); err != nil {
							return err
						}
					}
					return nil
				}
			},
			testFunc: func(t *testing.T, client *deviceclient.Client) {
				deviceCh, errCh := client.ListDevices(context.Background())

				var devices []*devicepb.Device
				for device := range deviceCh {
					devices = append(devices, device)
				}

				err := <-errCh
				require.NoError(t, err)
				assert.Len(t, devices, 2)
				assert.Equal(t, "device-1", devices[0].Id)
				assert.Equal(t, "device-2", devices[1].Id)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockService := &mockDeviceService{}
			if tc.setupMock != nil {
				tc.setupMock(mockService)
			}

			_, handler := devicerpc.NewDeviceServiceHandler(mockService)
			server := httptest.NewServer(handler)
			defer server.Close()

			client := deviceclient.NewClient(server.URL)
			tc.testFunc(t, client)
		})
	}
}
