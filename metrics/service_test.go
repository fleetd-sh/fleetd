package metrics_test

import (
	"context"
	"encoding/json"
	"testing"

	"connectrpc.com/connect"
	metricspb "fleetd.sh/gen/metrics/v1"
	"fleetd.sh/internal/migrations"
	"fleetd.sh/internal/testutil"
	"fleetd.sh/metrics"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestMetricsService_SendMetrics(t *testing.T) {
	// Initialize the test database
	db, cleanup, err := testutil.NewDBTemp()
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer cleanup()

	// Run migrations
	if err := migrations.MigrateUp(db); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	// Initialize the service
	service := metrics.NewMetricsService(db)

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
					Name:      "temperature",
					Value:     25.5,
					Timestamp: timestamppb.Now(),
					Tags:      map[string]string{"location": "room1"},
				},
				{
					Name:      "humidity",
					Value:     60.0,
					Timestamp: timestamppb.Now(),
					Tags:      map[string]string{"location": "room1"},
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
				DeviceId: tc.deviceID,
				Metrics:  tc.metrics,
			})

			resp, err := service.SendMetrics(context.Background(), req)
			if err != nil {
				t.Fatalf("SendMetrics failed: %v", err)
			}

			if resp.Msg.Success != tc.expectedResult {
				t.Errorf("Expected success=%v, got %v", tc.expectedResult, resp.Msg.Success)
			}

			// Verify metrics were inserted into the database
			for _, metric := range tc.metrics {
				var count int
				var storedTags string
				err = db.QueryRow("SELECT COUNT(*), tags FROM metric WHERE device_id = ? AND name = ? AND value = ?",
					tc.deviceID, metric.Name, metric.Value).Scan(&count, &storedTags)
				if err != nil {
					t.Fatalf("Failed to query metrics: %v", err)
				}
				if count != 1 {
					t.Errorf("Expected 1 metric, got %d", count)
				}

				// Verify tags
				var storedTagsMap map[string]string
				err = json.Unmarshal([]byte(storedTags), &storedTagsMap)
				if err != nil {
					t.Fatalf("Failed to unmarshal stored tags: %v", err)
				}
				if !mapsEqual(metric.Tags, storedTagsMap) {
					t.Errorf("Stored tags %v don't match original tags %v", storedTagsMap, metric.Tags)
				}
			}

			// If no metrics were sent, verify that none were inserted
			if len(tc.metrics) == 0 {
				var count int
				err = db.QueryRow("SELECT COUNT(*) FROM metric WHERE device_id = ?", tc.deviceID).Scan(&count)
				if err != nil {
					t.Fatalf("Failed to query metrics: %v", err)
				}
				if count != 0 {
					t.Errorf("Expected 0 metrics, got %d", count)
				}
			}
		})
	}
}

func TestMetricsService_SendMetrics_InvalidInput(t *testing.T) {
	// Initialize the test database
	db, cleanup, err := testutil.NewDBTemp()
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer cleanup()

	// Run migrations
	if err := migrations.MigrateUp(db); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	// Initialize the service
	service := metrics.NewMetricsService(db)

	testCases := []struct {
		name        string
		deviceID    string
		metrics     []*metricspb.Metric
		expectError bool
	}{
		{
			name:        "Empty device ID",
			deviceID:    "",
			metrics:     []*metricspb.Metric{{Name: "test", Value: 1, Timestamp: timestamppb.Now()}},
			expectError: true,
		},
		{
			name:        "Empty metric name",
			deviceID:    "device-1",
			metrics:     []*metricspb.Metric{{Name: "", Value: 1, Timestamp: timestamppb.Now()}},
			expectError: true,
		},
		{
			name:        "Nil timestamp",
			deviceID:    "device-1",
			metrics:     []*metricspb.Metric{{Name: "test", Value: 1, Timestamp: nil}},
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := connect.NewRequest(&metricspb.SendMetricsRequest{
				DeviceId: tc.deviceID,
				Metrics:  tc.metrics,
			})

			resp, err := service.SendMetrics(context.Background(), req)
			if err != nil {
				t.Fatalf("SendMetrics failed: %v", err)
			}

			if tc.expectError && resp.Msg.Success {
				t.Error("Expected failure for invalid input, got success")
			}

			if !tc.expectError && !resp.Msg.Success {
				t.Error("Expected success for valid input, got failure")
			}
		})
	}
}

// Helper function to compare maps
func mapsEqual(m1, m2 map[string]string) bool {
	if len(m1) != len(m2) {
		return false
	}
	for k, v1 := range m1 {
		if v2, ok := m2[k]; !ok || v1 != v2 {
			return false
		}
	}
	return true
}
