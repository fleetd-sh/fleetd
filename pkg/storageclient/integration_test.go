package storageclient_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	storagerpc "fleetd.sh/gen/storage/v1/storagev1connect"
	"fleetd.sh/pkg/storageclient"
	"fleetd.sh/storage"
)

func TestStorageClient_Integration(t *testing.T) {
	t.Run("StorageOperations", func(t *testing.T) {
		// Set up test server
		storagePath := t.TempDir()
		storageService := storage.NewStorageService(storagePath)
		path, handler := storagerpc.NewStorageServiceHandler(storageService)

		mux := http.NewServeMux()
		mux.Handle(path, handler)

		server := httptest.NewServer(mux)
		defer server.Close()

		client := storageclient.NewClient(server.URL)

		// Test data
		testData := []byte("test data")
		bucket := "test-bucket"
		key := "test-key"

		// Test PutObject
		err := client.PutObject(context.Background(), &storageclient.Object{
			Bucket: bucket,
			Key:    key,
			Data:   bytes.NewReader(testData),
			Size:   int64(len(testData)),
		})
		require.NoError(t, err)

		// Test GetObject
		obj, err := client.GetObject(context.Background(), bucket, key)
		require.NoError(t, err)
		require.NotNil(t, obj)

		// Read the data from the object
		data, err := io.ReadAll(obj.Data)
		require.NoError(t, err)

		// Compare the data
		assert.Equal(t, testData, data)
		assert.Equal(t, bucket, obj.Bucket)
		assert.Equal(t, key, obj.Key)
		assert.Equal(t, int64(len(testData)), obj.Size)
	})
}
