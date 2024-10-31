package authclient_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

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

func TestAuthClient_Integration(t *testing.T) {
	if testing.Short() || config.GetIntFromEnv("INTEGRATION", 0) != 1 {
		t.Skip("Skipping integration test in short mode")
	}

	// Set up test server and database
	db := testutil.NewTestDB(t)

	// Run migrations
	version, dirty, err := migrations.MigrateUp(db.DB)
	require.NoError(t, err)
	require.Greater(t, version, -1)
	require.False(t, dirty)

	// Create services and handlers
	authService := auth.NewAuthService(db.DB)
	authPath, authHandler := authrpc.NewAuthServiceHandler(authService)

	// Set up HTTP test server
	authMux := http.NewServeMux()
	authMux.Handle(authPath, authHandler)
	authServer := httptest.NewServer(authMux)
	defer authServer.Close()

	// Create clients
	authClient := authclient.NewClient(authServer.URL)

	deviceService := device.NewDeviceService(db.DB, authClient)
	devicePath, deviceHandler := devicerpc.NewDeviceServiceHandler(deviceService)
	deviceMux := http.NewServeMux()
	deviceMux.Handle(devicePath, deviceHandler)
	deviceServer := httptest.NewServer(deviceMux)
	defer deviceServer.Close()

	deviceClient := deviceclient.NewClient(deviceServer.URL)

	t.Run("AuthenticationFlow", func(t *testing.T) {

		// Register device and get API key
		id, key, err := deviceClient.RegisterDevice(context.Background(), &deviceclient.NewDevice{
			Name:    "test-device",
			Type:    "TEST_DEVICE",
			Version: "v1.0.0",
		})
		require.NoError(t, err)
		require.NotEmpty(t, id)
		require.NotEmpty(t, key)

		// Test authentication with the same key
		result, err := authClient.Authenticate(context.Background(), key)
		require.NoError(t, err)
		require.True(t, result.Authenticated)
		require.Equal(t, id, result.DeviceID)
	})
}
