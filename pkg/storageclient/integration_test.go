package storageclient_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"fleetd.sh/internal/config"
	"fleetd.sh/pkg/storageclient"
)

func TestStorageClient_Integration(t *testing.T) {
	if testing.Short() || config.GetIntFromEnv("INTEGRATION", 0) != 1 {
		t.Skip("Skipping integration test in short mode")
	}

	client := storageclient.NewClient("http://localhost:50054")
	ctx := context.Background()

	t.Run("StorageOperations", func(t *testing.T) {
		// Test putting an object
		bucket := "test-bucket"
		key := "test-key"
		data := []byte("test data")

		success, err := client.PutObject(ctx, bucket, key, data)
		require.NoError(t, err)
		assert.True(t, success)

		// Test getting the object back
		retrievedData, err := client.GetObject(ctx, bucket, key)
		require.NoError(t, err)
		assert.Equal(t, data, retrievedData)
	})
}
