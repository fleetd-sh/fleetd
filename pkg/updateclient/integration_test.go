package updateclient_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "github.com/tursodatabase/libsql-client-go/libsql"

	updaterpc "fleetd.sh/gen/update/v1/updatev1connect"
	"fleetd.sh/internal/config"
	"fleetd.sh/internal/migrations"
	"fleetd.sh/internal/testutil"
	"fleetd.sh/pkg/updateclient"
	"fleetd.sh/update"
)

func TestUpdateClient_Integration(t *testing.T) {
	if testing.Short() || config.GetIntFromEnv("INTEGRATION", 0) != 1 {
		t.Skip("Skipping integration test in short mode")
	}

	// Create temp directory for test database and storage
	tempDir, err := os.MkdirTemp("", "update-integration-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Set up SQLite database
	db := testutil.NewTestDB(t)

	// Run migrations
	version, dirty, err := migrations.MigrateUp(db.DB)
	require.NoError(t, err)
	require.Greater(t, version, -1)
	require.False(t, dirty)

	// Create update service
	updateService := update.NewUpdateService(db.DB)

	// Create ConnectRPC handler
	path, handler := updaterpc.NewUpdateServiceHandler(updateService)
	mux := http.NewServeMux()
	mux.Handle(path, handler)
	server := httptest.NewServer(mux)
	defer server.Close()

	client := updateclient.NewClient(server.URL)

	t.Run("UpdateLifecycle", func(t *testing.T) {
		// Create temp file with content
		content := []byte("test update content")
		updatesDir := filepath.Join(tempDir, "updates")
		require.NoError(t, os.MkdirAll(updatesDir, 0755))
		packagePath := filepath.Join(updatesDir, "test-package.zip")
		err := os.WriteFile(packagePath, content, 0644)
		require.NoError(t, err)

		// Calculate checksum
		hasher := sha256.New()
		_, err = hasher.Write(content)
		require.NoError(t, err)
		checksum := "sha256:" + hex.EncodeToString(hasher.Sum(nil))

		// Create package
		pkg := &updateclient.Package{
			Version:     "v1.0.1",
			DeviceTypes: []string{"SENSOR"},
			FileURL:     packagePath,
			FileSize:    int64(len(content)),
			Checksum:    checksum,
			ChangeLog:   "Test update",
		}

		// Create package and verify
		id, err := client.CreatePackage(context.Background(), pkg)
		require.NoError(t, err)
		require.NotEmpty(t, id)

		// Wait for package to be processed
		time.Sleep(2 * time.Second)

		// Check for updates
		updates, err := client.GetAvailableUpdates(context.Background(), "SENSOR", time.Now().Add(-24*time.Hour))
		require.NoError(t, err)
		require.NotEmpty(t, updates)
		assert.Equal(t, pkg.Version, updates[0].Version)
	})

	t.Run("InvalidUpdates", func(t *testing.T) {
		ctx := context.Background()

		// Test with invalid version
		_, err := client.CreatePackage(ctx, &updateclient.Package{
			Version:     "",
			DeviceTypes: []string{"SENSOR"},
			FileURL:     "nonexistent.zip",
		})
		assert.Error(t, err)

		// Test with empty device types
		_, err = client.CreatePackage(ctx, &updateclient.Package{
			Version:     "v1.0.0",
			DeviceTypes: []string{},
			FileURL:     "nonexistent.zip",
		})
		assert.Error(t, err)

		// Test with non-existent package
		_, err = client.CreatePackage(ctx, &updateclient.Package{
			Version:     "v1.0.0",
			DeviceTypes: []string{"SENSOR"},
			FileURL:     "nonexistent.zip",
		})
		assert.Error(t, err)

		// Test getting non-existent package
		_, err = client.GetPackage(ctx, "nonexistent-id")
		assert.Error(t, err)
	})
}
