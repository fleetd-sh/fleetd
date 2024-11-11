package integration

import (
	"context"
	"database/sql"
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
	_ "modernc.org/sqlite"
)

func setupAnalyticsServer(t *testing.T) (*http.Server, *httptest.Server, *sql.DB, func()) {
	// Setup test database
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db, err := sql.Open("sqlite", dbPath)
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
		"INSERT INTO device (id, name, type, version, api_key) VALUES (?, ?, ?, ?, ?)",
		"test-device", "test", "raspberry-pi", "1.0.0", "test-key")
	require.NoError(t, err)

	// Test device metrics
	now := time.Now()
	_, err = db.Exec(
		`INSERT INTO device_metric (device_id, metric_name, cpu_usage, timestamp)
		 VALUES (?, ?, ?, ?)`,
		"test-device", "cpu", 50.0, now.UTC().Format(time.RFC3339))
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
	if metricsResp.Msg.Metrics[0].Name != "cpu_usage" {
		t.Errorf("Expected metric name %q, got %q", "cpu_usage", metricsResp.Msg.Metrics[0].Name)
	}

	// Test device health
	_, err = db.Exec(
		`INSERT INTO device_health (device_id, status, message, timestamp)
		 VALUES (?, ?, ?, ?)`,
		"test-device", "warning", "High CPU usage", now.UTC().Format(time.RFC3339))
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
	tx, err := db.Begin()
	require.NoError(t, err)
	defer tx.Rollback()

	// First create a test binary
	_, err = tx.Exec(
		`INSERT INTO binary (id, name, version, platform, architecture, size, sha256, storage_path)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"test-binary", "test.bin", "1.0.0", "linux", "amd64", 1024, "abc123", "/tmp/test.bin")
	require.NoError(t, err)

	// Then create the update campaign
	_, err = tx.Exec(
		`INSERT INTO update_campaign (id, name, description, binary_id, target_version, target_platforms, target_architectures, strategy, status, total_devices)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"test-campaign", "Test Update", "Test update campaign", "test-binary", "1.0.0", "linux", "amd64", "immediate", "in_progress", 2)
	require.NoError(t, err)

	_, err = tx.Exec(
		`INSERT INTO update_metric (campaign_id, timestamp, success_rate, avg_duration)
		 VALUES (?, ?, ?, ?)`,
		"test-campaign", now.UTC().Format(time.RFC3339), 1.0, 120)
	require.NoError(t, err)

	err = tx.Commit()
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
		`INSERT INTO performance_metric (device_id, metric_name, value, unit, timestamp)
		 VALUES (?, ?, ?, ?, ?)`,
		"test-device", "request_latency", 100.0, "ms", now.UTC().Format(time.RFC3339))
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
