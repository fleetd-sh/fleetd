package integration

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"connectrpc.com/connect"
	pb "fleetd.sh/gen/fleetd/v1"
	rpc "fleetd.sh/gen/fleetd/v1/fleetpbconnect"
	"fleetd.sh/internal/api"
	"fleetd.sh/internal/migrations"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func setupDeviceServer(t *testing.T) (*http.Server, *httptest.Server, *sql.DB, func()) {
	// Setup test database
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)

	// Run migrations
	_, _, err = migrations.MigrateUp(db)
	require.NoError(t, err)

	// Setup HTTP mux with Connect handler
	mux := http.NewServeMux()
	deviceService := api.NewDeviceService(db)
	mux.Handle(rpc.NewDeviceServiceHandler(
		deviceService,
		connect.WithCompressMinBytes(1024),
	))

	// Create test server
	server := httptest.NewServer(h2c.NewHandler(mux, &http2.Server{}))

	cleanup := func() {
		server.Close()
		db.Close()
		os.RemoveAll(dir)
	}

	return &http.Server{Handler: mux}, server, db, cleanup
}

func TestDeviceRegistrationFlow(t *testing.T) {
	_, server, db, cleanup := setupDeviceServer(t)
	defer cleanup()

	client := rpc.NewDeviceServiceClient(
		http.DefaultClient,
		server.URL,
	)

	// Test device registration
	regReq := connect.NewRequest(&pb.RegisterRequest{
		Name:         "test-device",
		Type:         "raspberry-pi",
		Version:      "1.0.0",
		Capabilities: map[string]string{"feature1": "enabled"},
	})

	regResp, err := client.Register(context.Background(), regReq)
	require.NoError(t, err)
	assert.NotEmpty(t, regResp.Msg.DeviceId)
	assert.NotEmpty(t, regResp.Msg.ApiKey)

	// Test heartbeat
	time.Sleep(50 * time.Millisecond) // Ensure some time passes
	hbReq := connect.NewRequest(&pb.HeartbeatRequest{
		DeviceId: regResp.Msg.DeviceId,
		Metrics:  map[string]string{"cpu": "50%"},
	})

	hbResp, err := client.Heartbeat(context.Background(), hbReq)
	require.NoError(t, err)
	assert.False(t, hbResp.Msg.HasUpdate)

	// Verify last_seen was updated
	var lastSeenStr sql.NullString
	err = db.QueryRowContext(context.Background(),
		"SELECT strftime('%Y-%m-%dT%H:%M:%SZ', last_seen) FROM device WHERE id = ?",
		regResp.Msg.DeviceId).Scan(&lastSeenStr)
	require.NoError(t, err)
	assert.True(t, lastSeenStr.Valid)
	lastSeen, err := time.Parse(time.RFC3339, lastSeenStr.String)
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now(), lastSeen, 2*time.Second)

	// Test status reporting
	statusReq := connect.NewRequest(&pb.ReportStatusRequest{
		DeviceId: regResp.Msg.DeviceId,
		Status:   "healthy",
		Metrics:  map[string]string{"memory": "2GB", "disk": "50%"},
	})

	statusResp, err := client.ReportStatus(context.Background(), statusReq)
	require.NoError(t, err)
	assert.True(t, statusResp.Msg.Success)

	// Verify final state in database
	var (
		name     string
		devType  string
		version  string
		metadata string
	)
	err = db.QueryRowContext(context.Background(),
		"SELECT name, type, version, metadata FROM device WHERE id = ?",
		regResp.Msg.DeviceId).Scan(&name, &devType, &version, &metadata)
	require.NoError(t, err)

	assert.Equal(t, regReq.Msg.Name, name)
	assert.Equal(t, regReq.Msg.Type, devType)
	assert.Equal(t, regReq.Msg.Version, version)
	assert.Contains(t, metadata, "memory")
	assert.Contains(t, metadata, "disk")
}
