package metrics_test

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	metricspb "fleetd.sh/gen/metrics/v1"
	"fleetd.sh/internal/testutil"
	"fleetd.sh/metrics"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestMetricsService_SendMetrics(t *testing.T) {
	// Mock InfluxDB client
	mockClient := &testutil.MockInfluxDBClient{}
	service := metrics.NewMetricsService(mockClient, "org", "bucket")

	// Test cases
	testCases := []struct {
		name           string
		deviceID       string
		metrics        []*metricspb.Metric
		expectedResult bool
	}{
		{
			name:     "Valid metrics",
			deviceID: uuid.New().String(),
			metrics: []*metricspb.Metric{
				{
					Measurement: "temperature",
					Tags:        map[string]string{"location": "room1"},
					Fields:      map[string]float64{"value": 25.5},
					Timestamp:   timestamppb.Now(),
				},
				{
					Measurement: "humidity",
					Tags:        map[string]string{"location": "room1"},
					Fields:      map[string]float64{"value": 60.0},
					Timestamp:   timestamppb.Now(),
				},
			},
			expectedResult: true,
		},
		{
			name:           "Empty metrics",
			deviceID:       uuid.New().String(),
			metrics:        []*metricspb.Metric{},
			expectedResult: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := connect.NewRequest(&metricspb.SendMetricsRequest{
				DeviceId:  tc.deviceID,
				Metrics:   tc.metrics,
				Precision: "ns",
			})

			resp, err := service.SendMetrics(context.Background(), req)
			require.NoError(t, err)
			assert.Equal(t, tc.expectedResult, resp.Msg.Success)

			// Verify metrics were sent to InfluxDB
			assert.Equal(t, len(tc.metrics), mockClient.WriteCallCount)
		})
	}
}

func TestMetricsService_SendMetrics_InvalidInput(t *testing.T) {
	// Mock InfluxDB client
	mockClient := &testutil.MockInfluxDBClient{}
	service := metrics.NewMetricsService(mockClient, "org", "bucket")

	testCases := []struct {
		name        string
		deviceID    string
		metrics     []*metricspb.Metric
		expectError bool
	}{
		{
			name:        "Empty device ID",
			deviceID:    "",
			metrics:     []*metricspb.Metric{{Measurement: "test", Fields: map[string]float64{"value": 1}, Timestamp: timestamppb.Now()}},
			expectError: true,
		},
		{
			name:        "Empty measurement",
			deviceID:    "device-1",
			metrics:     []*metricspb.Metric{{Measurement: "", Fields: map[string]float64{"value": 1}, Timestamp: timestamppb.Now()}},
			expectError: true,
		},
		{
			name:        "Nil timestamp",
			deviceID:    "device-1",
			metrics:     []*metricspb.Metric{{Measurement: "test", Fields: map[string]float64{"value": 1}, Timestamp: nil}},
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := connect.NewRequest(&metricspb.SendMetricsRequest{
				DeviceId:  tc.deviceID,
				Metrics:   tc.metrics,
				Precision: "ns",
			})

			resp, err := service.SendMetrics(context.Background(), req)
			require.NoError(t, err)
			assert.Equal(t, !tc.expectError, resp.Msg.Success)
		})
	}
}
