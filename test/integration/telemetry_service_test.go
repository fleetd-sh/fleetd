package integration

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	fleetpb "fleetd.sh/gen/fleetd/v1"
	"fleetd.sh/gen/fleetd/v1/fleetpbconnect"
	"fleetd.sh/internal/database"
	"fleetd.sh/internal/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTelemetryService(t *testing.T) {
	requireIntegrationMode(t)
	// Create test database
	db := setupTestDatabase(t)
	defer safeCloseDB(db)

	// Create service
	dbWrapper := &database.DB{DB: db}
	service := services.NewTelemetryService(dbWrapper)

	// Create test server
	mux := http.NewServeMux()
	path, handler := fleetpbconnect.NewTelemetryServiceHandler(service)
	mux.Handle(path, handler)
	server := httptest.NewServer(mux)
	defer server.Close()

	// Create client
	client := fleetpbconnect.NewTelemetryServiceClient(
		http.DefaultClient,
		server.URL,
	)

	t.Run("GetTelemetry", func(t *testing.T) {
		req := &fleetpb.GetTelemetryRequest{
			DeviceId: "test-device-001",
			Limit:    10,
		}

		resp, err := client.GetTelemetry(context.Background(), connect.NewRequest(req))
		require.NoError(t, err)
		assert.NotNil(t, resp)
		assert.NotEmpty(t, resp.Msg.Data)
		assert.LessOrEqual(t, len(resp.Msg.Data), 10)

		// Verify data fields
		for _, data := range resp.Msg.Data {
			assert.Equal(t, "test-device-001", data.DeviceId)
			assert.NotNil(t, data.Timestamp)
			assert.GreaterOrEqual(t, data.CpuUsage, 0.0)
			assert.LessOrEqual(t, data.CpuUsage, 100.0)
		}
	})

	t.Run("GetMetrics", func(t *testing.T) {
		req := &fleetpb.GetMetricsRequest{
			DeviceIds:   []string{"device-001", "device-002"},
			MetricNames: []string{"cpu", "memory"},
			Aggregation: "avg",
		}

		resp, err := client.GetMetrics(context.Background(), connect.NewRequest(req))
		require.NoError(t, err)
		assert.NotNil(t, resp)
		assert.NotEmpty(t, resp.Msg.Metrics)

		// Verify we got metrics for both devices and both metric types
		deviceMetrics := make(map[string]map[string]bool)
		for _, metric := range resp.Msg.Metrics {
			if deviceMetrics[metric.DeviceId] == nil {
				deviceMetrics[metric.DeviceId] = make(map[string]bool)
			}
			deviceMetrics[metric.DeviceId][metric.Name] = true
		}

		assert.True(t, deviceMetrics["device-001"]["cpu"])
		assert.True(t, deviceMetrics["device-001"]["memory"])
		assert.True(t, deviceMetrics["device-002"]["cpu"])
		assert.True(t, deviceMetrics["device-002"]["memory"])
	})

	t.Run("StreamTelemetry", func(t *testing.T) {
		t.Skip("Skipping streaming test - requires real stream implementation")

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		req := &fleetpb.StreamTelemetryRequest{
			DeviceIds: []string{"device-001"},
		}

		stream, err := client.StreamTelemetry(ctx, connect.NewRequest(req))
		require.NoError(t, err)

		// Receive at least one message
		hasMessage := stream.Receive()
		assert.True(t, hasMessage)

		if hasMessage {
			msg := stream.Msg()
			assert.Equal(t, "device-001", msg.DeviceId)
			assert.NotNil(t, msg.Timestamp)
		}
	})

	t.Run("GetLogs", func(t *testing.T) {
		req := &fleetpb.GetLogsRequest{
			DeviceIds: []string{"device-001"},
			Levels:    []fleetpb.LogLevel{fleetpb.LogLevel_LOG_LEVEL_INFO, fleetpb.LogLevel_LOG_LEVEL_ERROR},
			Limit:     20,
		}

		resp, err := client.GetLogs(context.Background(), connect.NewRequest(req))
		require.NoError(t, err)
		assert.NotNil(t, resp)
		assert.NotEmpty(t, resp.Msg.Logs)
		assert.LessOrEqual(t, len(resp.Msg.Logs), 20)

		// Verify log levels
		for _, log := range resp.Msg.Logs {
			assert.Contains(t, []fleetpb.LogLevel{
				fleetpb.LogLevel_LOG_LEVEL_INFO,
				fleetpb.LogLevel_LOG_LEVEL_ERROR,
			}, log.Level)
		}
	})

	t.Run("AlertManagement", func(t *testing.T) {
		// Create alert
		createReq := &fleetpb.ConfigureAlertRequest{
			Alert: &fleetpb.Alert{
				Name:        "Test CPU Alert",
				Description: "Alert when CPU exceeds 90%",
				Type:        fleetpb.AlertType_ALERT_TYPE_CPU,
				Threshold:   90,
				Condition:   fleetpb.AlertCondition_ALERT_CONDITION_GREATER_THAN,
				Enabled:     true,
				DeviceIds:   []string{"device-001"},
			},
		}

		createResp, err := client.ConfigureAlert(context.Background(), connect.NewRequest(createReq))
		require.NoError(t, err)
		assert.NotNil(t, createResp.Msg.Alert)
		assert.NotEmpty(t, createResp.Msg.Alert.Id)

		alertId := createResp.Msg.Alert.Id

		// List alerts
		listReq := &fleetpb.ListAlertsRequest{
			DeviceIds:   []string{"device-001"},
			EnabledOnly: true,
		}

		listResp, err := client.ListAlerts(context.Background(), connect.NewRequest(listReq))
		require.NoError(t, err)
		assert.NotEmpty(t, listResp.Msg.Alerts)

		// Delete alert
		deleteReq := &fleetpb.DeleteAlertRequest{
			AlertId: alertId,
		}

		_, err = client.DeleteAlert(context.Background(), connect.NewRequest(deleteReq))
		assert.NoError(t, err)
	})
}

func BenchmarkTelemetryService(b *testing.B) {
	// Create test database
	db := setupBenchDatabase(b)
	defer safeCloseDB(db)

	// Create service
	dbWrapper := &database.DB{DB: db}
	service := services.NewTelemetryService(dbWrapper)

	// Create test server
	mux := http.NewServeMux()
	path, handler := fleetpbconnect.NewTelemetryServiceHandler(service)
	mux.Handle(path, handler)
	server := httptest.NewServer(mux)
	defer server.Close()

	// Create client
	client := fleetpbconnect.NewTelemetryServiceClient(
		http.DefaultClient,
		server.URL,
	)

	b.Run("GetTelemetry", func(b *testing.B) {
		req := &fleetpb.GetTelemetryRequest{
			DeviceId: "bench-device",
			Limit:    100,
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := client.GetTelemetry(context.Background(), connect.NewRequest(req))
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("GetLogs", func(b *testing.B) {
		req := &fleetpb.GetLogsRequest{
			DeviceIds: []string{"bench-device"},
			Limit:     50,
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := client.GetLogs(context.Background(), connect.NewRequest(req))
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}
