package deviceclient_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "github.com/tursodatabase/libsql-client-go/libsql"

	"fleetd.sh/auth"
	"fleetd.sh/device"
	authrpc "fleetd.sh/gen/auth/v1/authv1connect"
	devicerpc "fleetd.sh/gen/device/v1/devicev1connect"
	"fleetd.sh/internal/config"
	"fleetd.sh/internal/migrations"
	"fleetd.sh/internal/testutil"
	"fleetd.sh/pkg/authclient"
	"fleetd.sh/pkg/deviceclient"
)

func TestDeviceClient_Integration(t *testing.T) {
	if testing.Short() || config.GetIntFromEnv("INTEGRATION", 0) != 1 {
		t.Skip("Skipping integration test in short mode")
	}

	// Create temp directory for test database
	tempDir, err := os.MkdirTemp("", "device-integration-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Set up SQLite database
	db := testutil.NewTestDB(t)

	// Run migrations
	version, dirty, err := migrations.MigrateUp(db.DB)
	require.NoError(t, err)
	require.Greater(t, version, -1)
	require.False(t, dirty)

	// Set up auth service first
	authService := auth.NewAuthService(db.DB)
	authPath, authHandler := authrpc.NewAuthServiceHandler(authService)

	authMux := http.NewServeMux()
	authMux.Handle(authPath, authHandler)

	authServer := httptest.NewServer(authMux)
	defer authServer.Close()

	// Create auth client with proper server URL
	authClient := authclient.NewClient(authServer.URL)

	// Create device service with proper auth client
	deviceService := device.NewDeviceService(db.DB, authClient)
	devicePath, deviceHandler := devicerpc.NewDeviceServiceHandler(deviceService)

	// Set up device server
	deviceMux := http.NewServeMux()
	deviceMux.Handle(devicePath, deviceHandler)
	deviceServer := httptest.NewServer(deviceMux)
	defer deviceServer.Close()

	// Create client using device server URL
	client := deviceclient.NewClient(deviceServer.URL)
	ctx := context.Background()

	t.Run("DeviceLifecycle", func(t *testing.T) {
		deviceName := "Integration Test Device"
		deviceType := "SENSOR"

		deviceID, apiKey, err := client.RegisterDevice(ctx, &deviceclient.NewDevice{
			Name:    deviceName,
			Type:    deviceType,
			Version: "v1.0.0",
		})
		require.NoError(t, err)
		require.NotEmpty(t, deviceID)
		require.NotEmpty(t, apiKey)

		// Give the server some time to process the registration
		time.Sleep(time.Second)

		// Update device status
		success, err := client.UpdateDeviceStatus(ctx, deviceID, "ACTIVE")
		require.NoError(t, err)
		assert.True(t, success)

		// List devices and verify our device is present
		deviceCh, errCh := client.ListDevices(ctx)

		var foundDevice bool
		for device := range deviceCh {
			if device.ID == deviceID {
				assert.Equal(t, deviceName, device.Name)
				assert.Equal(t, deviceType, device.Type)
				assert.Equal(t, "ACTIVE", device.Status)
				foundDevice = true
				break
			}
		}

		err = <-errCh
		require.NoError(t, err)
		assert.True(t, foundDevice, "Device not found in listing")
	})
}
