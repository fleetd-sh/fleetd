package authclient_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authpb "fleetd.sh/gen/auth/v1"
	authrpc "fleetd.sh/gen/auth/v1/authv1connect"
	"fleetd.sh/pkg/authclient"
)

type mockAuthService struct {
	authrpc.UnimplementedAuthServiceHandler
	authenticateFunc   func(context.Context, *authpb.AuthenticateRequest) (*authpb.AuthenticateResponse, error)
	generateAPIKeyFunc func(context.Context, *authpb.GenerateAPIKeyRequest) (*authpb.GenerateAPIKeyResponse, error)
	revokeAPIKeyFunc   func(context.Context, *authpb.RevokeAPIKeyRequest) (*authpb.RevokeAPIKeyResponse, error)
}

func (m *mockAuthService) Authenticate(ctx context.Context, req *connect.Request[authpb.AuthenticateRequest]) (*connect.Response[authpb.AuthenticateResponse], error) {
	if m.authenticateFunc != nil {
		resp, err := m.authenticateFunc(ctx, req.Msg)
		return connect.NewResponse(resp), err
	}
	return connect.NewResponse(&authpb.AuthenticateResponse{
		Authenticated: true,
		DeviceId:      "test-device-001",
	}), nil
}

func (m *mockAuthService) GenerateAPIKey(ctx context.Context, req *connect.Request[authpb.GenerateAPIKeyRequest]) (*connect.Response[authpb.GenerateAPIKeyResponse], error) {
	if m.generateAPIKeyFunc != nil {
		resp, err := m.generateAPIKeyFunc(ctx, req.Msg)
		return connect.NewResponse(resp), err
	}
	return connect.NewResponse(&authpb.GenerateAPIKeyResponse{
		ApiKey: "test-api-key",
	}), nil
}

func (m *mockAuthService) RevokeAPIKey(ctx context.Context, req *connect.Request[authpb.RevokeAPIKeyRequest]) (*connect.Response[authpb.RevokeAPIKeyResponse], error) {
	if m.revokeAPIKeyFunc != nil {
		resp, err := m.revokeAPIKeyFunc(ctx, req.Msg)
		return connect.NewResponse(resp), err
	}
	return connect.NewResponse(&authpb.RevokeAPIKeyResponse{
		Success: true,
	}), nil
}

func TestAuthClient_Unit(t *testing.T) {
	testCases := []struct {
		name          string
		setupMock     func(*mockAuthService)
		testFunc      func(*testing.T, *authclient.Client)
		expectedError string
	}{
		{
			name: "Authenticate success",
			setupMock: func(m *mockAuthService) {
				m.authenticateFunc = func(_ context.Context, req *authpb.AuthenticateRequest) (*authpb.AuthenticateResponse, error) {
					return &authpb.AuthenticateResponse{
						Authenticated: true,
						DeviceId:      "device-123",
					}, nil
				}
			},
			testFunc: func(t *testing.T, client *authclient.Client) {
				result, err := client.Authenticate(context.Background(), "valid-api-key")
				require.NoError(t, err)
				assert.True(t, result.Authenticated)
				assert.Equal(t, "device-123", result.DeviceID)
			},
		},
		{
			name: "Generate API key success",
			setupMock: func(m *mockAuthService) {
				m.generateAPIKeyFunc = func(_ context.Context, req *authpb.GenerateAPIKeyRequest) (*authpb.GenerateAPIKeyResponse, error) {
					return &authpb.GenerateAPIKeyResponse{
						ApiKey: "new-api-key",
					}, nil
				}
			},
			testFunc: func(t *testing.T, client *authclient.Client) {
				apiKey, err := client.GenerateAPIKey(context.Background(), "valid-api-key")
				require.NoError(t, err)
				assert.Equal(t, "new-api-key", apiKey)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockService := &mockAuthService{}
			tc.setupMock(mockService)

			mux := http.NewServeMux()
			path, handler := authrpc.NewAuthServiceHandler(mockService)
			mux.Handle(path, handler)

			server := httptest.NewUnstartedServer(mux)
			server.Start()
			defer server.Close()

			client := authclient.NewClient(server.URL)
			tc.testFunc(t, client)
		})
	}
}
