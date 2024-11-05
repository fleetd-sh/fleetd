package discoveryclient_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"fleetd.sh/daemon"
	"fleetd.sh/device"
	devicerpc "fleetd.sh/gen/device/v1/devicev1connect"
	discoveryrpc "fleetd.sh/gen/discovery/v1/discoveryv1connect"
	"fleetd.sh/internal/config"
	"fleetd.sh/internal/migrations"
	"fleetd.sh/internal/testutil"
	"fleetd.sh/pkg/discoveryclient"
)

func TestDiscoveryClient_Integration(t *testing.T) {
	if testing.Short() || config.GetIntFromEnv("INTEGRATION", 0) != 1 {
		t.Skip("Skipping integration test in short mode")
	}

	// Create temp directory for config
	tempDir := t.TempDir()

	// Create config directory
	configDir := filepath.Join(tempDir, "config")
	require.NoError(t, os.MkdirAll(configDir, 0755))

	db := testutil.NewTestDB(t)

	// Run migrations
	version, dirty, err := migrations.MigrateUp(db.DB)
	require.NoError(t, err)
	require.Greater(t, version, -1)
	require.False(t, dirty)

	// Create services
	deviceService := device.NewDeviceService(db.DB)
	require.NoError(t, err)

	// Create handlers
	devicePath, deviceHandler := devicerpc.NewDeviceServiceHandler(deviceService)

	// Set up single mux for both services
	mux := http.NewServeMux()
	mux.Handle(devicePath, deviceHandler)

	// Create test server
	server := httptest.NewServer(mux)
	defer server.Close()

	// Create client
	client := discoveryclient.NewClient(server.URL)
	ctx := context.Background()

	// Set up daemon
	fleetd, err := daemon.NewFleetDaemon(&daemon.Config{
		ConfigDir:   configDir,
		APIEndpoint: server.URL,
		DeviceName:  "test-device",
		DeviceID:    "test-id",
	})
	require.NoError(t, err)

	// Set up discovery service
	discoveryService := daemon.NewDiscoveryService(fleetd)
	discoveryPath, discoveryHandler := discoveryrpc.NewDiscoveryServiceHandler(discoveryService)
	mux.Handle(discoveryPath, discoveryHandler)

	t.Run("ConfigureDevice", func(t *testing.T) {
		device := discoveryclient.Device{
			Name: fleetd.GetConfig().DeviceName,
			ID:   fleetd.GetConfig().DeviceID,
		}

		config := discoveryclient.Configuration{
			APIEndpoint: server.URL,
		}

		success, err := client.ConfigureDevice(ctx, device, config)
		require.NoError(t, err)
		assert.True(t, success, "Device configuration should succeed")

		// Verify device configuration

		require.NoError(t, err)
		cfg := fleetd.GetConfig()
		assert.True(t, fleetd.IsConfigured(), "Device should be marked as configured")
		assert.Equal(t, server.URL, cfg.APIEndpoint)
		assert.Equal(t, device.Name, cfg.DeviceName)
		assert.NotEmpty(t, cfg.DeviceID)
		assert.NotEmpty(t, cfg.APIKey)
	})
}
