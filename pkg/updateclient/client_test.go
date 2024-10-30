package updateclient_test

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	updatepb "fleetd.sh/gen/update/v1"
	updaterpc "fleetd.sh/gen/update/v1/updatev1connect"
	"fleetd.sh/pkg/updateclient"
)

type mockUpdateService struct {
	updaterpc.UnimplementedUpdateServiceHandler
	createUpdatePackageFunc func(context.Context, *updatepb.CreateUpdatePackageRequest) (*updatepb.CreateUpdatePackageResponse, error)
	getAvailableUpdatesFunc func(context.Context, *updatepb.GetAvailableUpdatesRequest) (*updatepb.GetAvailableUpdatesResponse, error)
}

func (m *mockUpdateService) CreateUpdatePackage(ctx context.Context, req *connect.Request[updatepb.CreateUpdatePackageRequest]) (*connect.Response[updatepb.CreateUpdatePackageResponse], error) {
	if m.createUpdatePackageFunc != nil {
		resp, err := m.createUpdatePackageFunc(ctx, req.Msg)
		return connect.NewResponse(resp), err
	}
	return connect.NewResponse(&updatepb.CreateUpdatePackageResponse{Success: true}), nil
}

func (m *mockUpdateService) GetAvailableUpdates(ctx context.Context, req *connect.Request[updatepb.GetAvailableUpdatesRequest]) (*connect.Response[updatepb.GetAvailableUpdatesResponse], error) {
	if m.getAvailableUpdatesFunc != nil {
		resp, err := m.getAvailableUpdatesFunc(ctx, req.Msg)
		return connect.NewResponse(resp), err
	}
	return connect.NewResponse(&updatepb.GetAvailableUpdatesResponse{}), nil
}

func TestUpdateClient_Unit(t *testing.T) {
	testCases := []struct {
		name          string
		setupMock     func(*mockUpdateService)
		testFunc      func(*testing.T, *updateclient.Client)
		expectedError string
	}{
		{
			name: "CreateUpdatePackage success",
			setupMock: func(m *mockUpdateService) {
				m.createUpdatePackageFunc = func(_ context.Context, req *updatepb.CreateUpdatePackageRequest) (*updatepb.CreateUpdatePackageResponse, error) {
					return &updatepb.CreateUpdatePackageResponse{
						Success: true,
						Message: "Update package created successfully",
					}, nil
				}
			},
			testFunc: func(t *testing.T, client *updateclient.Client) {
				req := &updatepb.CreateUpdatePackageRequest{
					Version:     "1.0.1",
					DeviceTypes: []string{"SENSOR"},
					ChangeLog:   "Test update",
				}
				success, err := client.CreateUpdatePackage(context.Background(), req)
				require.NoError(t, err)
				assert.True(t, success)
			},
		},
		{
			name: "GetAvailableUpdates success",
			setupMock: func(m *mockUpdateService) {
				m.getAvailableUpdatesFunc = func(_ context.Context, req *updatepb.GetAvailableUpdatesRequest) (*updatepb.GetAvailableUpdatesResponse, error) {
					return &updatepb.GetAvailableUpdatesResponse{
						Updates: []*updatepb.UpdatePackage{
							{
								Version:     "1.0.1",
								ReleaseDate: timestamppb.Now(),
								DeviceTypes: []string{"SENSOR"},
								ChangeLog:   "Test update",
							},
						},
					}, nil
				}
			},
			testFunc: func(t *testing.T, client *updateclient.Client) {
				updates, err := client.GetAvailableUpdates(
					context.Background(),
					"SENSOR",
					timestamppb.New(time.Now().Add(-24*time.Hour)),
				)
				require.NoError(t, err)
				require.Len(t, updates, 1)
				assert.Equal(t, "1.0.1", updates[0].Version)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockService := &mockUpdateService{}
			if tc.setupMock != nil {
				tc.setupMock(mockService)
			}

			_, handler := updaterpc.NewUpdateServiceHandler(mockService)
			server := httptest.NewServer(handler)
			defer server.Close()

			client := updateclient.NewClient(server.URL)
			tc.testFunc(t, client)
		})
	}
}
