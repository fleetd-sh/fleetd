package authclient_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"fleetd.sh/internal/config"
	"fleetd.sh/pkg/authclient"
)

func TestAuthClient_Integration(t *testing.T) {
	if testing.Short() || config.GetIntFromEnv("INTEGRATION", 0) != 1 {
		t.Skip("Skipping integration test in short mode")
	}

	client := authclient.NewClient("http://localhost:50051")
	ctx := context.Background()

	t.Run("AuthenticationFlow", func(t *testing.T) {
		// Generate an API key
		deviceID := "test-device-001"
		apiKey, err := client.GenerateAPIKey(ctx, deviceID)
		require.NoError(t, err)
		require.NotEmpty(t, apiKey)

		// Authenticate with the generated API key
		authenticated, returnedDeviceID, err := client.Authenticate(ctx, apiKey)
		require.NoError(t, err)
		assert.True(t, authenticated)
		assert.Equal(t, deviceID, returnedDeviceID)

		// Revoke the API key
		success, err := client.RevokeAPIKey(ctx, apiKey)
		require.NoError(t, err)
		assert.True(t, success)

		// Verify the API key is no longer valid
		authenticated, _, err = client.Authenticate(ctx, apiKey)
		require.NoError(t, err)
		assert.False(t, authenticated)
	})
}
