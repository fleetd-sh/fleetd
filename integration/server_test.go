package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "github.com/tursodatabase/libsql-client-go/libsql"

	"fleetd.sh/auth"
	"fleetd.sh/device"
	"fleetd.sh/internal/config"
	"fleetd.sh/internal/migrations"
	"fleetd.sh/internal/testutil"
	"fleetd.sh/internal/testutil/containers"
	"fleetd.sh/metrics"
	"fleetd.sh/pkg/deviceclient"
	"fleetd.sh/pkg/metricsclient"
	"fleetd.sh/pkg/updateclient"
	"fleetd.sh/storage"
	"fleetd.sh/update"
)

func TestAllInOneServer(t *testing.T) {
	if testing.Short() || config.GetIntFromEnv("INTEGRATION", 0) != 1 {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Create temp directory
	tempDir, err := os.MkdirTemp("", "fleet-server-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Start InfluxDB container
	influxContainer, err := containers.NewInfluxDBContainer(ctx)
	require.NoError(t, err)
	t.Cleanup(func() {
		if err := influxContainer.Close(); err != nil {
			t.Logf("failed to close InfluxDB container: %v", err)
		}
	})

	// Set up test database
	db := testutil.NewTestDB(t)

	version, dirty, err := migrations.MigrateUp(db.DB)
	require.NoError(t, err)
	require.False(t, dirty)
	require.Greater(t, version, -1)
	t.Logf("Migrated to version %d, dirty: %t", version, dirty)

	// Initialize all services
	authService := auth.NewAuthService(db.DB)
	deviceService := device.NewDeviceService(db.DB)
	metricsService := metrics.NewMetricsService(
		influxContainer.Client,
		influxContainer.Org,
		influxContainer.Bucket,
	)
	updateService := update.NewUpdateService(db.DB)
	storageService := storage.NewStorageService(tempDir)

	// Create server with all services
	server := NewTestServer(t,
		authService,
		deviceService,
		metricsService,
		updateService,
		storageService,
	)
	defer server.Close()

	// Create clients using server URL
	deviceClient := deviceclient.NewClient(server.URL)
	metricsClient := metricsclient.NewClient(server.URL)
	updateClient := updateclient.NewClient(server.URL)

	ctx = context.Background()

	// Test device registration
	t.Run("DeviceRegistration", func(t *testing.T) {
		deviceID, apiKey, err := deviceClient.RegisterDevice(ctx, &deviceclient.NewDevice{
			Name:    "Test Device",
			Type:    "SENSOR",
			Version: "v1.0.0",
		})
		require.NoError(t, err)
		assert.NotEmpty(t, deviceID)
		assert.NotEmpty(t, apiKey)

		// Verify device exists
		deviceCh, errCh := deviceClient.ListDevices(ctx)
		var found bool
		for device := range deviceCh {
			if device.ID == deviceID {
				found = true
				break
			}
		}
		require.NoError(t, <-errCh)
		assert.True(t, found)
	})

	// Test metrics collection
	t.Run("MetricsCollection", func(t *testing.T) {
		// Send metrics
		err := metricsClient.SendMetrics(ctx, []*metricsclient.Metric{
			{
				DeviceID:    "test-device",
				Measurement: "temperature",
				Fields: map[string]float64{
					"value": 23.5,
				},
				Tags: map[string]string{
					"location": "office",
				},
				Timestamp: time.Now(),
			},
		}, "s")
		require.NoError(t, err)

		// Add delay to allow InfluxDB to process the metrics
		time.Sleep(2 * time.Second)

		// Query metrics back with retry logic
		var found bool
		for retries := 0; retries < 3; retries++ {
			metricsCh, errCh := metricsClient.GetMetrics(ctx, &metricsclient.MetricQuery{
				StartTime: time.Now().Add(-1 * time.Hour),
				EndTime:   time.Now().Add(1 * time.Hour),
			})

			for metric := range metricsCh {
				if metric.DeviceID == "test-device" {
					found = true
					t.Logf("Found metric: %+v", metric)
					break
				}
			}

			if err := <-errCh; err != nil {
				t.Logf("Error querying metrics: %v", err)
				if retries < 2 {
					t.Logf("Retrying query (attempt %d)", retries+1)
					continue
				}
				t.Fatalf("Failed to query metrics after 3 attempts: %v", err)
			}
		}

		require.True(t, found)
	})

	// Test update management
	t.Run("UpdateManagement", func(t *testing.T) {
		pkg := &updateclient.Package{
			Version:     "v1.0.1",
			DeviceTypes: []string{"SENSOR"},
			ChangeLog:   "Test update",
			FileURL:     "https://example.com/test-update.zip",
			Checksum:    "sha256:1234567890abcdef",
			FileSize:    1234567,
		}

		pkgID, err := updateClient.CreatePackage(ctx, pkg)
		require.NoError(t, err)
		assert.NotEmpty(t, pkgID)

		updates, err := updateClient.GetAvailableUpdates(ctx, "SENSOR", time.Now().Add(-24*time.Hour))
		require.NoError(t, err)
		assert.NotEmpty(t, updates)
		assert.Equal(t, "v1.0.1", updates[0].Version)
	})
}
