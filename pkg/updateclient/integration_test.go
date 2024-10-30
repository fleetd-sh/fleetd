package updateclient_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	updatepb "fleetd.sh/gen/update/v1"
	"fleetd.sh/internal/config"
	"fleetd.sh/pkg/updateclient"
)

func TestUpdateClient_Integration(t *testing.T) {
	if testing.Short() || config.GetIntFromEnv("INTEGRATION", 0) != 1 {
		t.Skip("Skipping integration test in short mode")
	}

	client := updateclient.NewClient("http://localhost:50055")
	ctx := context.Background()

	t.Run("UpdateLifecycle", func(t *testing.T) {
		// Create a new update package
		createReq := &updatepb.CreateUpdatePackageRequest{
			Version:     "1.0.1",
			ReleaseDate: timestamppb.Now(),
			ChangeLog:   "Test update package",
			DeviceTypes: []string{"SENSOR"},
		}

		success, err := client.CreateUpdatePackage(ctx, createReq)
		require.NoError(t, err)
		assert.True(t, success)

		// Give the server some time to process the update
		time.Sleep(time.Second)

		// Check for available updates
		updates, err := client.GetAvailableUpdates(
			ctx,
			"SENSOR",
			timestamppb.New(time.Now().Add(-24*time.Hour)),
		)
		require.NoError(t, err)
		require.NotEmpty(t, updates)

		// Verify the update details
		found := false
		for _, update := range updates {
			if update.Version == createReq.Version {
				found = true
				assert.Equal(t, createReq.ChangeLog, update.ChangeLog)
				assert.Equal(t, createReq.DeviceTypes, update.DeviceTypes)
				break
			}
		}
		assert.True(t, found, "Created update package not found in available updates")
	})
}
