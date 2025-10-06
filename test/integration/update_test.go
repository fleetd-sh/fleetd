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

	pb "fleetd.sh/gen/fleetd/v1"
	rpc "fleetd.sh/gen/fleetd/v1/fleetpbconnect"
	"fleetd.sh/internal/api"
	"fleetd.sh/internal/migrations"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

func setupUpdateServer(t *testing.T) (*http.Server, *httptest.Server, *sql.DB, func()) {
	// Create temporary directory
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	storagePath := filepath.Join(tmpDir, "binaries")

	// Setup database
	db, err := sql.Open("sqlite", dbPath)
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

	// Create test server with HTTP/2 support
	server := httptest.NewServer(h2c.NewHandler(mux, &http2.Server{}))

	cleanup := func() {
		server.Close()
		safeCloseDB(db)
		os.RemoveAll(tmpDir)
	}

	return &http.Server{Handler: mux}, server, db, cleanup
}

func TestUpdateServiceIntegration(t *testing.T) {
	// Run subtests for each client type
	clients := []struct {
		name   string
		client func(string) rpc.UpdateServiceClient
	}{
		{
			name: "connect",
			client: func(url string) rpc.UpdateServiceClient {
				return rpc.NewUpdateServiceClient(
					http.DefaultClient,
					url,
				)
			},
		},
		{
			name: "grpc",
			client: func(url string) rpc.UpdateServiceClient {
				return rpc.NewUpdateServiceClient(
					http.DefaultClient,
					url,
					connect.WithGRPC(),
				)
			},
		},
	}

	for _, c := range clients {
		c := c // capture range variable
		t.Run(c.name, func(t *testing.T) {
			// Setup server and cleanup for this test
			_, server, db, cleanup := setupUpdateServer(t)
			defer cleanup()

			// Setup test data
			deviceID := "test-device"
			setupTestDevice(t, db, deviceID)
			binaryID := uploadTestUpdateBinary(t, server.URL)

			// Create client
			client := c.client(server.URL)

			// Run test
			ctx := context.Background()

			// Create campaign
			campaign := createTestCampaign(t, ctx, client, binaryID)

			// Verify initial campaign state
			verifyInitialCampaignState(t, ctx, client, campaign.CampaignId, deviceID)

			// Test update progression
			testUpdateProgression(t, ctx, client, campaign.CampaignId, deviceID)

			// Verify final campaign state
			verifyFinalCampaignState(t, ctx, client, campaign.CampaignId)
		})
	}
}

func setupTestDevice(t *testing.T, db *sql.DB, deviceID string) {
	_, err := db.Exec(
		`INSERT INTO device (id, name, type, version, api_key)
		 VALUES (?, ?, ?, ?, ?)`,
		deviceID, "test", "raspberry-pi", "1.0.0", "test-key")
	require.NoError(t, err)
}

func createTestCampaign(t *testing.T, ctx context.Context, client rpc.UpdateServiceClient, binaryID string) *pb.CreateUpdateCampaignResponse {
	resp, err := client.CreateUpdateCampaign(ctx, connect.NewRequest(&pb.CreateUpdateCampaignRequest{
		Name:                "Test Update",
		Description:         "Integration test update campaign",
		BinaryId:            binaryID,
		TargetVersion:       "2.0.0",
		TargetPlatforms:     []string{"raspberry-pi"},
		TargetArchitectures: []string{"arm64"},
		Strategy:            pb.UpdateStrategy_UPDATE_STRATEGY_IMMEDIATE,
	}))
	require.NoError(t, err)
	assert.NotEmpty(t, resp.Msg.CampaignId)
	return resp.Msg
}

func verifyInitialCampaignState(t *testing.T, ctx context.Context, client rpc.UpdateServiceClient, campaignID, deviceID string) {
	// Verify campaign status
	campaign, err := client.GetUpdateCampaign(ctx, connect.NewRequest(&pb.GetUpdateCampaignRequest{
		CampaignId: campaignID,
	}))
	require.NoError(t, err)
	assert.Equal(t, pb.UpdateCampaignStatus_UPDATE_CAMPAIGN_STATUS_IN_PROGRESS, campaign.Msg.Campaign.Status)
	assert.Equal(t, int32(1), campaign.Msg.Campaign.TotalDevices)

	// Verify device status
	status, err := client.GetDeviceUpdateStatus(ctx, connect.NewRequest(&pb.GetDeviceUpdateStatusRequest{
		DeviceId:   deviceID,
		CampaignId: campaignID,
	}))
	require.NoError(t, err)
	assert.Equal(t, pb.DeviceUpdateStatus_DEVICE_UPDATE_STATUS_PENDING, status.Msg.Status)
}

func testUpdateProgression(t *testing.T, ctx context.Context, client rpc.UpdateServiceClient, campaignID, deviceID string) {
	updateStates := []pb.DeviceUpdateStatus{
		pb.DeviceUpdateStatus_DEVICE_UPDATE_STATUS_DOWNLOADING,
		pb.DeviceUpdateStatus_DEVICE_UPDATE_STATUS_DOWNLOADED,
		pb.DeviceUpdateStatus_DEVICE_UPDATE_STATUS_INSTALLING,
		pb.DeviceUpdateStatus_DEVICE_UPDATE_STATUS_INSTALLED,
	}

	for _, state := range updateStates {
		reportResp, err := client.ReportUpdateStatus(ctx, connect.NewRequest(&pb.ReportUpdateStatusRequest{
			DeviceId:   deviceID,
			CampaignId: campaignID,
			Status:     state,
		}))
		require.NoError(t, err)
		assert.True(t, reportResp.Msg.Success)

		// Verify status was updated
		statusResp, err := client.GetDeviceUpdateStatus(ctx, connect.NewRequest(&pb.GetDeviceUpdateStatusRequest{
			DeviceId:   deviceID,
			CampaignId: campaignID,
		}))
		require.NoError(t, err)
		assert.Equal(t, state, statusResp.Msg.Status)
	}
}

func verifyFinalCampaignState(t *testing.T, ctx context.Context, client rpc.UpdateServiceClient, campaignID string) {
	campaign, err := client.GetUpdateCampaign(ctx, connect.NewRequest(&pb.GetUpdateCampaignRequest{
		CampaignId: campaignID,
	}))
	require.NoError(t, err)
	assert.Equal(t, pb.UpdateCampaignStatus_UPDATE_CAMPAIGN_STATUS_COMPLETED, campaign.Msg.Campaign.Status)
	assert.Equal(t, int32(1), campaign.Msg.Campaign.TotalDevices)
	assert.Equal(t, int32(1), campaign.Msg.Campaign.UpdatedDevices)
	assert.Equal(t, int32(0), campaign.Msg.Campaign.FailedDevices)
}

func uploadTestUpdateBinary(t *testing.T, serverURL string) string {
	// Create a client to interact with the BinaryService
	client := rpc.NewBinaryServiceClient(http.DefaultClient, serverURL)

	// Start the upload stream
	stream := client.UploadBinary(context.Background())

	// Send metadata first
	err := stream.Send(&pb.UploadBinaryRequest{
		Data: &pb.UploadBinaryRequest_Metadata{
			Metadata: &pb.BinaryMetadata{
				Name:         "test-binary",
				Version:      "1.0.0",
				Platform:     "raspberry-pi",
				Architecture: "arm64",
			},
		},
	})
	require.NoError(t, err)

	// Send binary data
	err = stream.Send(&pb.UploadBinaryRequest{
		Data: &pb.UploadBinaryRequest_Chunk{
			Chunk: []byte("binary data"),
		},
	})
	require.NoError(t, err)

	// Close stream and get response
	resp, err := stream.CloseAndReceive()
	require.NoError(t, err)
	return resp.Msg.Id
}

// Add a helper function to parse timestamps from SQLite string format
func parseTimestamp(s string) (*time.Time, error) {
	if s == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil, err
	}
	return &t, nil
}
