package testutil

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"fleetd.sh/auth"
	"fleetd.sh/device"
	authrpc "fleetd.sh/gen/auth/v1/authv1connect"
	devicerpc "fleetd.sh/gen/device/v1/devicev1connect"
	metricsrpc "fleetd.sh/gen/metrics/v1/metricsv1connect"
	storagerpc "fleetd.sh/gen/storage/v1/storagev1connect"
	updaterpc "fleetd.sh/gen/update/v1/updatev1connect"
	"fleetd.sh/internal/migrations"
	"fleetd.sh/internal/testutil/containers"
	"fleetd.sh/metrics"
	"fleetd.sh/storage"
	"fleetd.sh/update"
)

// Stack represents a complete test stack of fleetd services
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

// SetupStack creates a complete test stack with all services configured
// and running on test HTTP servers
func SetupStack(t *testing.T) (*Stack, error) {
	t.Helper()
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
	db := NewTestDB(t)

	version, dirty, err := migrations.MigrateUp(db.DB)
	if err != nil {
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}
	require.Greater(t, version, -1)
	require.False(t, dirty)

	// Initialize all services
	authService := auth.NewAuthService(db.DB)
	deviceService := device.NewDeviceService(db.DB)
	updateService := update.NewUpdateService(db.DB)
	storageService := storage.NewStorageService(fmt.Sprintf("%s/storage", tempDir))

	// Create handlers for each service
	_, authHandler := authrpc.NewAuthServiceHandler(authService)
	_, deviceHandler := devicerpc.NewDeviceServiceHandler(deviceService)
	_, metricsHandler := metricsrpc.NewMetricsServiceHandler(metricsService)
	_, updateHandler := updaterpc.NewUpdateServiceHandler(updateService)
	_, storageHandler := storagerpc.NewStorageServiceHandler(storageService)

	// Start HTTP test servers for each service
	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHandler.ServeHTTP(w, r)
	}))

	deviceServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deviceHandler.ServeHTTP(w, r)
	}))

	metricsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		metricsHandler.ServeHTTP(w, r)
	}))

	updateServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		updateHandler.ServeHTTP(w, r)
	}))

	storageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		storageHandler.ServeHTTP(w, r)
	}))

	cleanup := func() {
		os.RemoveAll(tempDir)
		authServer.Close()
		deviceServer.Close()
		metricsServer.Close()
		updateServer.Close()
		storageServer.Close()
		influxContainer.Close()
	}

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

// Cleanup performs cleanup of all resources used by the stack
func (s *Stack) Cleanup() {
	if s.cleanup != nil {
		s.cleanup()
	}
}
