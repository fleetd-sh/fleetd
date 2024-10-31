package updateclient_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	updatepb "fleetd.sh/gen/update/v1"
	updaterpc "fleetd.sh/gen/update/v1/updatev1connect"
	"fleetd.sh/pkg/updateclient"
)

type mockUpdateService struct {
	updaterpc.UnimplementedUpdateServiceHandler
	getAvailableUpdatesFunc func(context.Context, *updatepb.GetAvailableUpdatesRequest) (*updatepb.GetAvailableUpdatesResponse, error)
	getPackageFunc          func(context.Context, *updatepb.GetPackageRequest) (*updatepb.GetPackageResponse, error)
}

func (m *mockUpdateService) GetAvailableUpdates(ctx context.Context, req *connect.Request[updatepb.GetAvailableUpdatesRequest]) (*connect.Response[updatepb.GetAvailableUpdatesResponse], error) {
	if m.getAvailableUpdatesFunc != nil {
		resp, err := m.getAvailableUpdatesFunc(ctx, req.Msg)
		if err != nil {
			return nil, err
		}
		return connect.NewResponse(resp), nil
	}
	return connect.NewResponse(&updatepb.GetAvailableUpdatesResponse{}), nil
}

func (m *mockUpdateService) GetPackage(ctx context.Context, req *connect.Request[updatepb.GetPackageRequest]) (*connect.Response[updatepb.GetPackageResponse], error) {
	if m.getPackageFunc != nil {
		resp, err := m.getPackageFunc(ctx, req.Msg)
		if err != nil {
			return nil, err
		}
		return connect.NewResponse(resp), nil
	}
	return connect.NewResponse(&updatepb.GetPackageResponse{}), nil
}

func TestUpdateClient_Unit(t *testing.T) {
	testCases := []struct {
		name      string
		setupMock func(*mockUpdateService)
		testFunc  func(*testing.T, *updateclient.Client)
	}{
		{
			name: "GetAvailableUpdates_success",
			setupMock: func(m *mockUpdateService) {
				m.getAvailableUpdatesFunc = func(_ context.Context, req *updatepb.GetAvailableUpdatesRequest) (*updatepb.GetAvailableUpdatesResponse, error) {
					pkg := &updatepb.Package{
						Id:          "pkg-123",
						Version:     "v1.0.1",
						DeviceTypes: []string{"SENSOR"},
					}
					return &updatepb.GetAvailableUpdatesResponse{
						Packages: []*updatepb.Package{pkg},
					}, nil
				}
			},
			testFunc: func(t *testing.T, client *updateclient.Client) {
				updates, err := client.GetAvailableUpdates(context.Background(), "SENSOR", time.Now().Add(-24*time.Hour))
				require.NoError(t, err)
				require.Len(t, updates, 1)
				assert.Equal(t, "pkg-123", updates[0].ID)
			},
		},
		{
			name: "GetPackage_success",
			setupMock: func(m *mockUpdateService) {
				m.getPackageFunc = func(_ context.Context, req *updatepb.GetPackageRequest) (*updatepb.GetPackageResponse, error) {
					pkg := &updatepb.Package{
						Id:          "pkg-123",
						Version:     "v1.0.1",
						DeviceTypes: []string{"SENSOR"},
					}
					return &updatepb.GetPackageResponse{
						Package: pkg,
					}, nil
				}
			},
			testFunc: func(t *testing.T, client *updateclient.Client) {
				pkg, err := client.GetPackage(context.Background(), "pkg-123")
				require.NoError(t, err)
				require.NotNil(t, pkg)
				t.Logf("pkg: %+v", pkg)
				assert.Equal(t, "pkg-123", pkg.ID)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockService := &mockUpdateService{}
			if tc.setupMock != nil {
				tc.setupMock(mockService)
			}

			path, handler := updaterpc.NewUpdateServiceHandler(mockService)
			mux := http.NewServeMux()
			mux.Handle(path, handler)
			server := httptest.NewServer(mux)
			defer server.Close()

			client := updateclient.NewClient(server.URL)
			tc.testFunc(t, client)
		})
	}
}
