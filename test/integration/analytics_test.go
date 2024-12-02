package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"connectrpc.com/connect"
	pb "fleetd.sh/gen/fleetd/v1"
	rpc "fleetd.sh/gen/fleetd/v1/fleetpbconnect"
	"fleetd.sh/internal/api"
	"fleetd.sh/internal/migrations"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupAnalyticsServer(t *testing.T) (*http.Server, *httptest.Server, *sql.DB, func()) {
	// Setup test database
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db, err := sql.Open("sqlite3", dbPath)
	require.NoError(t, err)

	// Run migrations
	_, _, err = migrations.MigrateUp(db)
	require.NoError(t, err)

	// Setup HTTP mux with Connect handler
	mux := http.NewServeMux()
	analyticsService := api.NewAnalyticsService(db)
	mux.Handle(rpc.NewAnalyticsServiceHandler(
		analyticsService,
		connect.WithCompressMinBytes(1024),
	))

	// Create test server
	server := httptest.NewServer(h2c.NewHandler(mux, &http2.Server{}))

	cleanup := func() {
		server.Close()
		db.Close()
		os.RemoveAll(dir)
	}

	return &http.Server{Handler: mux}, server, db, cleanup
}

func TestAnalyticsServiceIntegration(t *testing.T) {
	_, server, db, cleanup := setupAnalyticsServer(t)
	defer cleanup()

	client := rpc.NewAnalyticsServiceClient(
		http.DefaultClient,
		server.URL,
	)

	// Create test device
	_, err := db.Exec(
		"INSERT INTO devices (id, name, type, version, api_key) VALUES (?, ?, ?, ?, ?)",
		"test-device", "test", "raspberry-pi", "1.0.0", "test-key")
	require.NoError(t, err)

	// Test device metrics
	now := time.Now()
	_, err = db.Exec(
		`INSERT INTO device_metrics (device_id, metric_name, metric_value, timestamp)
		 VALUES (?, ?, ?, ?)`,
		"test-device", "cpu", 50.0, now)
	require.NoError(t, err)

	metricsResp, err := client.GetDeviceMetrics(context.Background(), connect.NewRequest(&pb.GetDeviceMetricsRequest{
		DeviceId: "test-device",
		TimeRange: &pb.TimeRange{
			StartTime: timestamppb.New(now.Add(-time.Hour)),
			EndTime:   timestamppb.New(now.Add(time.Hour)),
		},
	}))
	require.NoError(t, err)
	assert.Len(t, metricsResp.Msg.Metrics, 1)
	assert.Equal(t, "cpu", metricsResp.Msg.Metrics[0].Name)

	// Test device health
	healthMetrics := map[string]string{"cpu": "50%", "memory": "2GB"}
	healthMetricsJSON, err := json.Marshal(healthMetrics)
	require.NoError(t, err)

	warnings := []string{"High CPU usage"}
	warningsJSON, err := json.Marshal(warnings)
	require.NoError(t, err)

	_, err = db.Exec(
		`INSERT INTO device_health (device_id, status, health_score, health_metrics, warnings, timestamp)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		"test-device", "warning", 0.8, string(healthMetricsJSON), string(warningsJSON), now)
	require.NoError(t, err)

	healthResp, err := client.GetDeviceHealth(context.Background(), connect.NewRequest(&pb.GetDeviceHealthRequest{
		DeviceId: "test-device",
		TimeRange: &pb.TimeRange{
			StartTime: timestamppb.New(now.Add(-time.Hour)),
			EndTime:   timestamppb.New(now.Add(time.Hour)),
		},
	}))
	require.NoError(t, err)
	assert.Equal(t, "warning", healthResp.Msg.CurrentStatus.Status)
	assert.Equal(t, 0.8, healthResp.Msg.CurrentStatus.HealthScore)

	// Test update analytics
	_, err = db.Exec(
		"INSERT INTO update_campaigns (id, name, total_devices) VALUES (?, ?, ?)",
		"test-campaign", "Test Update", 2)
	require.NoError(t, err)

	_, err = db.Exec(
		`INSERT INTO update_metrics (campaign_id, device_id, status, duration_seconds, timestamp)
		 VALUES (?, ?, ?, ?, ?)`,
		"test-campaign", "test-device", "INSTALLED", 120, now)
	require.NoError(t, err)

	updateResp, err := client.GetUpdateAnalytics(context.Background(), connect.NewRequest(&pb.GetUpdateAnalyticsRequest{
		TimeRange: &pb.TimeRange{
			StartTime: timestamppb.New(now.Add(-time.Hour)),
			EndTime:   timestamppb.New(now.Add(time.Hour)),
		},
	}))
	require.NoError(t, err)
	assert.Len(t, updateResp.Msg.Campaigns, 1)
	assert.Equal(t, float64(120), updateResp.Msg.AverageCompletionTime)

	// Test performance metrics
	_, err = db.Exec(
		`INSERT INTO performance_metrics (name, value, unit, timestamp)
		 VALUES (?, ?, ?, ?)`,
		"request_latency", 100.0, "ms", now)
	require.NoError(t, err)

	perfResp, err := client.GetPerformanceMetrics(context.Background(), connect.NewRequest(&pb.GetPerformanceMetricsRequest{
		TimeRange: &pb.TimeRange{
			StartTime: timestamppb.New(now.Add(-time.Hour)),
			EndTime:   timestamppb.New(now.Add(time.Hour)),
		},
	}))
	require.NoError(t, err)
	assert.Len(t, perfResp.Msg.Metrics, 1)
	assert.Equal(t, "request_latency", perfResp.Msg.Metrics[0].Name)
	assert.Equal(t, 100.0, perfResp.Msg.Metrics[0].Value)
	assert.Equal(t, "ms", perfResp.Msg.Metrics[0].Unit)
}
