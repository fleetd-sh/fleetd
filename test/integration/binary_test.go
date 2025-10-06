package integration

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	pb "fleetd.sh/gen/fleetd/v1"
	rpc "fleetd.sh/gen/fleetd/v1/fleetpbconnect"

	"connectrpc.com/connect"
	"fleetd.sh/internal/api"
	"fleetd.sh/internal/migrations"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	_ "modernc.org/sqlite"
)

func setupBinaryServer(t *testing.T) (*http.Server, *httptest.Server, func()) {
	// Create temporary directories
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	storagePath := filepath.Join(tmpDir, "binaries")

	// Setup database
	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)

	// Run migrations
	_, _, err = migrations.MigrateUp(db)
	require.NoError(t, err)

	// Setup binary service
	binaryService, err := api.NewBinaryService(db, storagePath)
	require.NoError(t, err)

	// Setup HTTP mux with Connect handler
	mux := http.NewServeMux()
	mux.Handle(rpc.NewBinaryServiceHandler(
		binaryService,
		connect.WithCompressMinBytes(1024),
	))

	// Create test server
	server := httptest.NewServer(h2c.NewHandler(mux, &http2.Server{}))

	cleanup := func() {
		server.Close()
		safeCloseDB(db)
		os.RemoveAll(tmpDir)
	}

	return &http.Server{Handler: mux}, server, cleanup
}

func uploadTestBinary(t *testing.T, serverURL string) string {
	client := rpc.NewBinaryServiceClient(
		http.DefaultClient,
		serverURL,
	)

	stream := client.UploadBinary(context.Background())

	// Send metadata first
	err := stream.Send(&pb.UploadBinaryRequest{
		Data: &pb.UploadBinaryRequest_Metadata{
			Metadata: &pb.BinaryMetadata{
				Name:         "test-binary",
				Version:      "1.0.0",
				Platform:     "linux",
				Architecture: "amd64",
			},
		},
	})
	require.NoError(t, err)

	// Send binary data
	err = stream.Send(&pb.UploadBinaryRequest{
		Data: &pb.UploadBinaryRequest_Chunk{
			Chunk: []byte("test binary data"),
		},
	})
	require.NoError(t, err)

	resp, err := stream.CloseAndReceive()
	require.NoError(t, err)
	return resp.Msg.Id
}

func TestBinaryServiceIntegration(t *testing.T) {
	_, server, cleanup := setupBinaryServer(t)
	defer cleanup()

	// Create Connect client
	client := rpc.NewBinaryServiceClient(
		http.DefaultClient,
		server.URL,
	)

	// Test uploading a binary
	stream := client.UploadBinary(context.Background())

	// Send metadata
	err := stream.Send(&pb.UploadBinaryRequest{
		Data: &pb.UploadBinaryRequest_Metadata{
			Metadata: &pb.BinaryMetadata{
				Name:         "test-app",
				Version:      "1.0.0",
				Platform:     "linux",
				Architecture: "amd64",
				Metadata:     map[string]string{"type": "server"},
			},
		},
	})
	require.NoError(t, err)

	// Send binary data
	testData := []byte("test binary data")
	hasher := sha256.New()
	hasher.Write(testData)
	expectedSHA256 := hex.EncodeToString(hasher.Sum(nil))

	err = stream.Send(&pb.UploadBinaryRequest{
		Data: &pb.UploadBinaryRequest_Chunk{
			Chunk: testData,
		},
	})
	require.NoError(t, err)

	uploadResp, err := stream.CloseAndReceive()
	require.NoError(t, err)
	assert.NotEmpty(t, uploadResp.Msg.Id)
	assert.Equal(t, expectedSHA256, uploadResp.Msg.Sha256)

	// Test getting binary info
	getBinaryResp, err := client.GetBinary(context.Background(), connect.NewRequest(&pb.GetBinaryRequest{
		Id: uploadResp.Msg.Id,
	}))
	require.NoError(t, err)
	assert.Equal(t, "test-app", getBinaryResp.Msg.Binary.Name)
	assert.Equal(t, "1.0.0", getBinaryResp.Msg.Binary.Version)
	assert.Equal(t, "linux", getBinaryResp.Msg.Binary.Platform)
	assert.Equal(t, "amd64", getBinaryResp.Msg.Binary.Architecture)
	assert.Equal(t, int64(len(testData)), getBinaryResp.Msg.Binary.Size)
	assert.Equal(t, expectedSHA256, getBinaryResp.Msg.Binary.Sha256)
	assert.Equal(t, "server", getBinaryResp.Msg.Binary.Metadata["type"])

	// Test downloading binary
	downloadStream, err := client.DownloadBinary(context.Background(), connect.NewRequest(&pb.DownloadBinaryRequest{
		Id: uploadResp.Msg.Id,
	}))
	require.NoError(t, err)

	var downloadedData []byte
	for {
		ok := downloadStream.Receive()
		if !ok {
			break
		}
		chunk := downloadStream.Msg()
		downloadedData = append(downloadedData, chunk.Chunk...)
	}
	assert.Equal(t, testData, downloadedData)

	// Test listing binaries
	listResp, err := client.ListBinaries(context.Background(), connect.NewRequest(&pb.ListBinariesRequest{
		Platform: "linux",
	}))
	require.NoError(t, err)
	assert.Len(t, listResp.Msg.Binaries, 1)
	assert.Equal(t, "test-app", listResp.Msg.Binaries[0].Name)
}
