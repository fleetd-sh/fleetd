package integration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "github.com/tursodatabase/libsql-client-go/libsql"
	"golang.org/x/exp/rand"

	"fleetd.sh/auth"
	"fleetd.sh/device"
	authrpc "fleetd.sh/gen/auth/v1/authv1connect"
	devicerpc "fleetd.sh/gen/device/v1/devicev1connect"
	metricsrpc "fleetd.sh/gen/metrics/v1/metricsv1connect"
	storagerpc "fleetd.sh/gen/storage/v1/storagev1connect"
	updaterpc "fleetd.sh/gen/update/v1/updatev1connect"
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

const (
	InfluxDBOrg        = "my-org"
	InfluxDBBucket     = "my-bucket"
	InfluxDBAdminToken = "my-super-secret-admin-token"
	InfluxDBUsername   = "admin"
	InfluxDBPassword   = "password123"
)

type Stack struct {
	AuthService       *auth.AuthService
	DeviceService     *device.DeviceService
	MetricsService    *metrics.MetricsService
	UpdateService     *update.UpdateService
	StorageService    *storage.StorageService
	AuthServiceURL    string
	DeviceServiceURL  string
	MetricsServiceURL string
	UpdateServiceURL  string
	StorageServiceURL string
	cleanup           func()
}

type TestDevice struct {
	Name       string
	ID         string
	Version    string
	DeviceType string
	Cleanup    func()
}

func TestFleetdIntegration(t *testing.T) {
	// Create a temporary directory for the test
	tempDir, err := os.MkdirTemp("", "fleetd-integration-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Initialize the test stack with dynamic ports
	stack, err := setupStack(t)
	if err != nil {
		t.Fatalf("Failed to set up stack: %v", err)
	}
	if stack != nil {
		defer stack.cleanup()
	}

	// Set up a test device with fleetd
	testDevice, err := setupTestDevice()
	if err != nil {
		t.Fatalf("Failed to set up test device: %v", err)
	}
	if testDevice != nil {
		defer testDevice.Cleanup()
	}

	// Create test database
	testDB := testutil.NewTestDB(t)
	db := testDB.GetDB() // Get the underlying *sql.DB

	// Run migrations
	if _, _, err := migrations.MigrateUp(db); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	// Register a test device
	t.Run("DeviceRegistration", func(t *testing.T) {
		ctx := context.Background()
		client := deviceclient.NewClient(stack.DeviceServiceURL)
		id, _, err := client.RegisterDevice(ctx, &deviceclient.NewDevice{
			Name:    testDevice.Name,
			Type:    testDevice.DeviceType,
			Version: "v1.0.0",
		})
		assert.NoError(t, err)
		testDevice.ID = id

		// Verify device is registered
		deviceCh, errorCh := client.ListDevices(ctx)
		foundDevice := false
		for device := range deviceCh {
			if device.ID == testDevice.ID {
				foundDevice = true
				assert.Equal(t, testDevice.Name, device.Name)
				assert.Equal(t, testDevice.DeviceType, device.Type)
				break
			}
		}
		assert.NoError(t, <-errorCh)
		assert.True(t, foundDevice, "Registered device not found in device list")
	})

	// Test metric collection
	t.Run("MetricCollection", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		client := metricsclient.NewClient(stack.MetricsServiceURL)

		// Create metric with proper fields
		metric := &metricsclient.Metric{
			DeviceID:    testDevice.ID,
			Measurement: "temperature",
			Fields: map[string]float64{
				"value": 25.5,
			},
			Tags: map[string]string{
				"type": "temperature",
			},
			Timestamp: time.Now(),
		}

		// Send metrics with proper device ID
		t.Log("Sending metric to InfluxDB")
		err := client.SendMetrics(ctx, []*metricsclient.Metric{metric}, "s")
		require.NoError(t, err)

		// Wait for metrics to be processed
		t.Log("Waiting for metrics to be processed")
		time.Sleep(2 * time.Second)

		// Query metrics with proper authentication and timeout
		t.Log("Querying metrics")
		metricsCh, errCh := client.GetMetrics(
			ctx,
			&metricsclient.MetricQuery{
				DeviceID:    testDevice.ID,
				Measurement: "temperature",
				StartTime:   time.Now().Add(-1 * time.Hour),
				EndTime:     time.Now(),
			},
		)

		// Add timeout for reading metrics
		done := make(chan bool)
		go func() {
			for metric := range metricsCh {
				t.Logf("Received metric: %+v", metric)
			}
			done <- true
		}()

		// Wait for either completion or timeout
		select {
		case err := <-errCh:
			require.NoError(t, err, "Error reading metrics")
		case <-done:
			t.Log("Successfully read all metrics")
		case <-time.After(5 * time.Second):
			t.Fatal("Timeout waiting for metrics")
		}
	})

	// Test update management
	t.Run("UpdateManagement", func(t *testing.T) {
		ctx := context.Background()
		client := updateclient.NewClient(stack.UpdateServiceURL)

		// Create temp file for update package
		f, err := os.CreateTemp("", "update-*.zip")
		require.NoError(t, err)
		defer os.Remove(f.Name())

		// Write some content
		content := []byte("test update content")
		_, err = f.Write(content)
		require.NoError(t, err)
		f.Close()

		// Calculate SHA256 checksum properly
		hasher := sha256.New()
		_, err = hasher.Write(content)
		require.NoError(t, err)
		checksum := "sha256:" + hex.EncodeToString(hasher.Sum(nil))

		// Create update package with matching device type and checksum
		req := &updateclient.Package{
			Version:     "v1.0.1",
			ChangeLog:   "Test update",
			DeviceTypes: []string{testDevice.DeviceType},
			FileURL:     f.Name(),
			FileSize:    int64(len(content)),
			Checksum:    checksum,
		}

		pkgID, err := client.CreatePackage(ctx, req)
		require.NoError(t, err)
		assert.NotEmpty(t, pkgID)

		// Wait for update to be processed
		time.Sleep(2 * time.Second)

		// Check for updates with matching device type
		updates, err := client.GetAvailableUpdates(
			ctx,
			"TEST_DEVICE",
			time.Now().Add(-24*time.Hour),
		)
		require.NoError(t, err)
		require.NotEmpty(t, updates)
		assert.Equal(t, "v1.0.1", updates[0].Version)

		// Delete update package
		err = client.DeletePackage(ctx, pkgID)
		require.NoError(t, err)

		// Verify update package is deleted
		_, err = client.GetPackage(ctx, pkgID)
		require.Error(t, err)
	})
}

func setupStack(t *testing.T) (*Stack, error) {
	t.Log("Starting stack setup")

	ctx := context.Background()

	// Create a temporary directory for the test
	tempDir, err := os.MkdirTemp("", "fleetd-integration-test")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}

	// Start InfluxDB using testcontainers
	influxContainer, err := containers.NewInfluxDBContainer(ctx)
	if err != nil {
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("failed to start InfluxDB: %w", err)
	}

	t.Cleanup(func() {
		if err := influxContainer.Close(); err != nil {
			t.Logf("failed to close InfluxDB container: %v", err)
		}
		os.RemoveAll(tempDir)
	})

	t.Log("Setting up InfluxDB client",
		"url", influxContainer.URL,
		"token", influxContainer.Token[:8]+"...",
		"org", influxContainer.Org,
		"bucket", influxContainer.Bucket,
	)

	// Create metrics service with the InfluxDB client
	metricsService := metrics.NewMetricsService(
		influxContainer.Client,
		influxContainer.Org,
		influxContainer.Bucket,
	)

	// Set up SQLite database
	db := testutil.NewTestDB(t)

	version, dirty, err := migrations.MigrateUp(db.DB)
	if err != nil {
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}
	require.Greater(t, version, -1)
	require.False(t, dirty)

	// Start the auth service server first
	authService := auth.NewAuthService(db.DB)
	authPath, authHandler := authrpc.NewAuthServiceHandler(authService)
	authMux := http.NewServeMux()
	authMux.Handle(authPath, authHandler)
	authServer := httptest.NewServer(authMux)

	deviceService := device.NewDeviceService(db.DB)
	updateService := update.NewUpdateService(db.DB)
	storageService := storage.NewStorageService(fmt.Sprintf("%s/storage", tempDir))

	// Create ConnectRPC handlers for each service
	devicePath, deviceHandler := devicerpc.NewDeviceServiceHandler(deviceService)
	metricsPath, metricsHandler := metricsrpc.NewMetricsServiceHandler(metricsService)
	updatePath, updateHandler := updaterpc.NewUpdateServiceHandler(updateService)
	storagePath, storageHandler := storagerpc.NewStorageServiceHandler(storageService)

	// Start remaining HTTP test servers with ConnectRPC handlers
	deviceMux := http.NewServeMux()
	deviceMux.Handle(devicePath, deviceHandler)
	deviceServer := httptest.NewServer(deviceMux)

	metricsMux := http.NewServeMux()
	metricsMux.Handle(metricsPath, metricsHandler)
	metricsServer := httptest.NewServer(metricsMux)

	updateMux := http.NewServeMux()
	updateMux.Handle(updatePath, updateHandler)
	updateServer := httptest.NewServer(updateMux)

	storageMux := http.NewServeMux()
	storageMux.Handle(storagePath, storageHandler)
	storageServer := httptest.NewServer(storageMux)

	cleanup := func() {
		os.RemoveAll(tempDir)
		authServer.Close()
		deviceServer.Close()
		metricsServer.Close()
		updateServer.Close()
		storageServer.Close()
		influxContainer.Close()
	}

	t.Log("AuthServiceURL", "url", authServer.URL)
	t.Log("DeviceServiceURL", "url", deviceServer.URL)
	t.Log("MetricsServiceURL", "url", metricsServer.URL)
	t.Log("UpdateServiceURL", "url", updateServer.URL)
	t.Log("StorageServiceURL", "url", storageServer.URL)

	return &Stack{
		AuthService:       authService,
		DeviceService:     deviceService,
		MetricsService:    metricsService,
		UpdateService:     updateService,
		StorageService:    storageService,
		AuthServiceURL:    authServer.URL,
		DeviceServiceURL:  deviceServer.URL,
		MetricsServiceURL: metricsServer.URL,
		UpdateServiceURL:  updateServer.URL,
		StorageServiceURL: storageServer.URL,
		cleanup:           cleanup,
	}, nil
}

func setupTestDevice() (*TestDevice, error) {
	// In a real scenario, this would set up a container running fleetd
	// For this example, we'll simulate a device
	return &TestDevice{
		Name:       "test-device-001",
		Version:    "v1.0.0",
		DeviceType: "TEST_DEVICE",
		Cleanup: func() {
			// Clean up resources if needed
		},
	}, nil
}

func (d *TestDevice) SendMetrics(url string) error {
	client := metricsclient.NewClient(url)

	err := client.SendMetrics(context.Background(), []*metricsclient.Metric{
		{
			DeviceID:    d.ID,
			Measurement: "temperature",
			Fields: map[string]float64{
				"value": float64(rand.Intn(100)),
			},
		},
	}, "s")

	return err
}

func (d *TestDevice) CheckForUpdates(url string) error {
	client := updateclient.NewClient(url)
	lastUpdateDate := time.Now().Add(-1 * time.Hour)
	_, err := client.GetAvailableUpdates(context.Background(), d.DeviceType, lastUpdateDate)
	return err
}
