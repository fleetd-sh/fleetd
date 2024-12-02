package integration

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"connectrpc.com/connect"
	pb "fleetd.sh/gen/fleetd/v1"
	rpc "fleetd.sh/gen/fleetd/v1/fleetpbconnect"
	"fleetd.sh/internal/api"
	"fleetd.sh/internal/migrations"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupUpdateServer(t *testing.T) (*http.Server, *httptest.Server, *sql.DB, func()) {
	// Create temporary directory
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	storagePath := filepath.Join(tmpDir, "binaries")

	// Setup database
	db, err := sql.Open("sqlite3", dbPath)
	require.NoError(t, err)

	// Run migrations
	_, _, err = migrations.MigrateUp(db)
	require.NoError(t, err)

	// Setup services
	binaryService, err := api.NewBinaryService(db, storagePath)
	require.NoError(t, err)
	updateService := api.NewUpdateService(db)

	// Setup HTTP mux with Connect handler
	mux := http.NewServeMux()
	mux.Handle(rpc.NewBinaryServiceHandler(
		binaryService,
		connect.WithCompressMinBytes(1024),
	))
	mux.Handle(rpc.NewUpdateServiceHandler(
		updateService,
		connect.WithCompressMinBytes(1024),
	))

	// Create test server
	server := httptest.NewServer(h2c.NewHandler(mux, &http2.Server{}))

	cleanup := func() {
		server.Close()
		db.Close()
		os.RemoveAll(tmpDir)
	}

	return &http.Server{Handler: mux}, server, db, cleanup
}

func TestUpdateServiceIntegration(t *testing.T) {
	_, listener, cleanup := setupUpdateServer(t)
	defer cleanup()

	// Create client connection
	ctx := context.Background()
	conn, err := grpc.DialContext(ctx, "bufnet",
		grpc.WithContextDialer(bufDialer(listener)),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()

	binaryClient := pb.NewBinaryServiceClient(conn)
	updateClient := pb.NewUpdateServiceClient(conn)

	// First register a test device
	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(
		`INSERT INTO devices (id, name, type, version, api_key, platform, architecture)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"test-device", "test", "raspberry-pi", "1.0.0", "test-key", "linux", "arm64")
	require.NoError(t, err)

	// Upload a binary
	stream, err := binaryClient.UploadBinary(ctx)
	require.NoError(t, err)

	err = stream.Send(&pb.UploadBinaryRequest{
		Data: &pb.UploadBinaryRequest_Metadata{
			Metadata: &pb.BinaryMetadata{
				Name:         "test-app",
				Version:      "2.0.0",
				Platform:     "linux",
				Architecture: "arm64",
			},
		},
	})
	require.NoError(t, err)

	err = stream.Send(&pb.UploadBinaryRequest{
		Data: &pb.UploadBinaryRequest_Chunk{
			Chunk: []byte("test binary data"),
		},
	})
	require.NoError(t, err)

	uploadResp, err := stream.CloseAndRecv()
	require.NoError(t, err)
	assert.NotEmpty(t, uploadResp.Id)

	// Create update campaign
	createResp, err := updateClient.CreateUpdateCampaign(ctx, &pb.CreateUpdateCampaignRequest{
		Name:                "Test Update",
		Description:         "Integration test update campaign",
		BinaryId:            uploadResp.Id,
		TargetVersion:       "2.0.0",
		TargetPlatforms:     []string{"linux"},
		TargetArchitectures: []string{"arm64"},
		Strategy:            pb.UpdateStrategy_UPDATE_STRATEGY_IMMEDIATE,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, createResp.CampaignId)

	// Get campaign status
	getResp, err := updateClient.GetUpdateCampaign(ctx, &pb.GetUpdateCampaignRequest{
		CampaignId: createResp.CampaignId,
	})
	require.NoError(t, err)
	assert.Equal(t, "Test Update", getResp.Campaign.Name)
	assert.Equal(t, pb.UpdateCampaignStatus_UPDATE_CAMPAIGN_STATUS_IN_PROGRESS, getResp.Campaign.Status)
	assert.Equal(t, int32(1), getResp.Campaign.TotalDevices)

	// Get device update status
	statusResp, err := updateClient.GetDeviceUpdateStatus(ctx, &pb.GetDeviceUpdateStatusRequest{
		DeviceId:   "test-device",
		CampaignId: createResp.CampaignId,
	})
	require.NoError(t, err)
	assert.Equal(t, pb.DeviceUpdateStatus_DEVICE_UPDATE_STATUS_PENDING, statusResp.Status)

	// Report update progress
	updateStates := []pb.DeviceUpdateStatus{
		pb.DeviceUpdateStatus_DEVICE_UPDATE_STATUS_DOWNLOADING,
		pb.DeviceUpdateStatus_DEVICE_UPDATE_STATUS_DOWNLOADED,
		pb.DeviceUpdateStatus_DEVICE_UPDATE_STATUS_INSTALLING,
		pb.DeviceUpdateStatus_DEVICE_UPDATE_STATUS_INSTALLED,
	}

	for _, state := range updateStates {
		reportResp, err := updateClient.ReportUpdateStatus(ctx, &pb.ReportUpdateStatusRequest{
			DeviceId:   "test-device",
			CampaignId: createResp.CampaignId,
			Status:     state,
		})
		require.NoError(t, err)
		assert.True(t, reportResp.Success)

		// Verify status was updated
		statusResp, err = updateClient.GetDeviceUpdateStatus(ctx, &pb.GetDeviceUpdateStatusRequest{
			DeviceId:   "test-device",
			CampaignId: createResp.CampaignId,
		})
		require.NoError(t, err)
		assert.Equal(t, state, statusResp.Status)
	}

	// Verify campaign was completed
	getResp, err = updateClient.GetUpdateCampaign(ctx, &pb.GetUpdateCampaignRequest{
		CampaignId: createResp.CampaignId,
	})
	require.NoError(t, err)
	assert.Equal(t, pb.UpdateCampaignStatus_UPDATE_CAMPAIGN_STATUS_COMPLETED, getResp.Campaign.Status)
	assert.Equal(t, int32(1), getResp.Campaign.UpdatedDevices)
	assert.Equal(t, int32(0), getResp.Campaign.FailedDevices)
}
