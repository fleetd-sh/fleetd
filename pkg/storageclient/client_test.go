package storageclient_test

import (
	"bytes"
	"context"
	"io"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	storagepb "fleetd.sh/gen/storage/v1"
	storagerpc "fleetd.sh/gen/storage/v1/storagev1connect"
	"fleetd.sh/pkg/storageclient"
)

type mockStorageService struct {
	storagerpc.UnimplementedStorageServiceHandler
	putObjectFunc    func(context.Context, *storagepb.PutObjectRequest) (*storagepb.PutObjectResponse, error)
	getObjectFunc    func(context.Context, *storagepb.GetObjectRequest) (*storagepb.GetObjectResponse, error)
	listObjectsFunc  func(context.Context, *storagepb.ListObjectsRequest, *connect.ServerStream[storagepb.ListObjectsResponse]) error
	deleteObjectFunc func(context.Context, *storagepb.DeleteObjectRequest) (*storagepb.DeleteObjectResponse, error)
}

func (m *mockStorageService) PutObject(ctx context.Context, req *connect.Request[storagepb.PutObjectRequest]) (*connect.Response[storagepb.PutObjectResponse], error) {
	if m.putObjectFunc != nil {
		resp, err := m.putObjectFunc(ctx, req.Msg)
		return connect.NewResponse(resp), err
	}
	return connect.NewResponse(&storagepb.PutObjectResponse{Success: true}), nil
}

func TestStorageClient_Unit(t *testing.T) {
	testCases := []struct {
		name          string
		setupMock     func(*mockStorageService)
		testFunc      func(*testing.T, *storageclient.Client)
		expectedError string
	}{
		{
			name: "PutObject success",
			setupMock: func(m *mockStorageService) {
				m.putObjectFunc = func(_ context.Context, req *storagepb.PutObjectRequest) (*storagepb.PutObjectResponse, error) {
					return &storagepb.PutObjectResponse{
						Success: true,
						Message: "Object stored successfully",
					}, nil
				}
			},
			testFunc: func(t *testing.T, client *storageclient.Client) {
				err := client.PutObject(context.Background(), &storageclient.Object{
					Bucket: "test-bucket",
					Key:    "test-key",
					Data:   io.NopCloser(bytes.NewReader([]byte("test-data"))),
					Size:   int64(len([]byte("test-data"))),
				})
				require.NoError(t, err)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockService := &mockStorageService{}
			if tc.setupMock != nil {
				tc.setupMock(mockService)
			}

			_, handler := storagerpc.NewStorageServiceHandler(mockService)
			server := httptest.NewServer(handler)
			defer server.Close()

			client := storageclient.NewClient(server.URL)
			tc.testFunc(t, client)
		})
	}
}
