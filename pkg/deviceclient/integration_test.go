package deviceclient_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	devicepb "fleetd.sh/gen/device/v1"
	"fleetd.sh/internal/config"
	"fleetd.sh/pkg/deviceclient"
)

func TestDeviceClient_Integration(t *testing.T) {
	if testing.Short() || config.GetIntFromEnv("INTEGRATION", 0) != 1 {
		t.Skip("Skipping integration test in short mode")
	}

	client := deviceclient.NewClient("http://localhost:50051")
	ctx := context.Background()

	t.Run("DeviceLifecycle", func(t *testing.T) {
		// Register a new device
		deviceName := "Integration Test Device"
		deviceType := "SENSOR"
		deviceID, apiKey, err := client.RegisterDevice(ctx, deviceName, deviceType)
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

		var foundDevice *devicepb.Device
		for device := range deviceCh {
			if device.Id == deviceID {
				foundDevice = device
				break
			}
		}

		err = <-errCh
		require.NoError(t, err)
		require.NotNil(t, foundDevice)
		assert.Equal(t, deviceName, foundDevice.Name)
		assert.Equal(t, deviceType, foundDevice.Type)
		assert.Equal(t, "ACTIVE", foundDevice.Status)
	})
}
