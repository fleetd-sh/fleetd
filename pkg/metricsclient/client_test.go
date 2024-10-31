package metricsclient_test

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	metricspb "fleetd.sh/gen/metrics/v1"
	metricsrpc "fleetd.sh/gen/metrics/v1/metricsv1connect"
	"fleetd.sh/pkg/metricsclient"
)

// Mock service implementation
type mockMetricsService struct {
	metricsrpc.UnimplementedMetricsServiceHandler
	sendMetricsFunc func(context.Context, *metricspb.SendMetricsRequest) (*metricspb.SendMetricsResponse, error)
	getMetricsFunc  func(context.Context, *metricspb.GetMetricsRequest) ([]*metricspb.Metric, error)
}

func (m *mockMetricsService) SendMetrics(ctx context.Context, req *connect.Request[metricspb.SendMetricsRequest]) (*connect.Response[metricspb.SendMetricsResponse], error) {
	if m.sendMetricsFunc != nil {
		resp, err := m.sendMetricsFunc(ctx, req.Msg)
		return connect.NewResponse(resp), err
	}
	return connect.NewResponse(&metricspb.SendMetricsResponse{Success: true}), nil
}

func (m *mockMetricsService) GetMetrics(ctx context.Context, req *connect.Request[metricspb.GetMetricsRequest], stream *connect.ServerStream[metricspb.GetMetricsResponse]) error {
	if m.getMetricsFunc != nil {
		metrics, err := m.getMetricsFunc(ctx, req.Msg)
		if err != nil {
			return err
		}
		for _, metric := range metrics {
			if err := stream.Send(&metricspb.GetMetricsResponse{Metric: metric}); err != nil {
				return err
			}
		}
	}
	return nil
}

func TestMetricsClient_Unit(t *testing.T) {
	testCases := []struct {
		name          string
		setupMock     func(*mockMetricsService)
		testFunc      func(*testing.T, *metricsclient.Client)
		expectedError string
	}{
		{
			name: "SendMetrics success",
			setupMock: func(m *mockMetricsService) {
				m.sendMetricsFunc = func(_ context.Context, req *metricspb.SendMetricsRequest) (*metricspb.SendMetricsResponse, error) {
					return &metricspb.SendMetricsResponse{Success: true}, nil
				}
			},
			testFunc: func(t *testing.T, client *metricsclient.Client) {
				err := client.SendMetrics(context.Background(), []*metricsclient.Metric{
					{
						DeviceID:    "device-1",
						Measurement: "temperature",
						Fields:      map[string]float64{"value": 25.5},
						Timestamp:   time.Now(),
					},
				}, "s")
				require.NoError(t, err)
			},
		},
		// TODO: More test cases
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockService := &mockMetricsService{}
			if tc.setupMock != nil {
				tc.setupMock(mockService)
			}

			_, handler := metricsrpc.NewMetricsServiceHandler(mockService)
			server := httptest.NewServer(handler)
			defer server.Close()

			client := metricsclient.NewClient(server.URL)
			tc.testFunc(t, client)
		})
	}
}
