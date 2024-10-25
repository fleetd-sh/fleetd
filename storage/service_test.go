package storage_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"connectrpc.com/connect"
	storagepb "fleetd.sh/gen/storage/v1"
	"fleetd.sh/storage"
)

func TestStorageService(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "storage-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Initialize the storage service
	service := storage.NewStorageService(tempDir)

	// Test PutObject
	t.Run("PutObject", func(t *testing.T) {
		req := connect.NewRequest(&storagepb.PutObjectRequest{
			Bucket: "test-bucket",
			Key:    "test-key",
			Data:   []byte("test data"),
		})

		resp, err := service.PutObject(context.Background(), req)
		if err != nil {
			t.Fatalf("PutObject failed: %v", err)
		}

		if !resp.Msg.Success {
			t.Errorf("Expected success to be true, got false")
		}

		// Verify the file was created
		filePath := filepath.Join(tempDir, "test-bucket", "test-key")
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			t.Errorf("Expected file to exist at %s", filePath)
		}
	})

	// Test GetObject
	t.Run("GetObject", func(t *testing.T) {
		req := connect.NewRequest(&storagepb.GetObjectRequest{
			Bucket: "test-bucket",
			Key:    "test-key",
		})

		resp, err := service.GetObject(context.Background(), req)
		if err != nil {
			t.Fatalf("GetObject failed: %v", err)
		}

		if string(resp.Msg.Data) != "test data" {
			t.Errorf("Expected data 'test data', got '%s'", string(resp.Msg.Data))
		}
	})

	// Test ListObjects
	t.Run("ListObjects", func(t *testing.T) {
		// Put another object
		_, err := service.PutObject(context.Background(), connect.NewRequest(&storagepb.PutObjectRequest{
			Bucket: "test-bucket",
			Key:    "test-key2",
			Data:   []byte("test data 2"),
		}))
		if err != nil {
			t.Fatalf("Failed to put second object: %v", err)
		}

		req := connect.NewRequest(&storagepb.ListObjectsRequest{
			Bucket: "test-bucket",
			Prefix: "",
		})

		resp, err := service.ListObjects(context.Background(), req)
		if err != nil {
			t.Fatalf("ListObjects failed: %v", err)
		}

		if len(resp.Msg.Objects) != 2 {
			t.Errorf("Expected 2 objects, got %d", len(resp.Msg.Objects))
		}
	})

	// Test DeleteObject
	t.Run("DeleteObject", func(t *testing.T) {
		req := connect.NewRequest(&storagepb.DeleteObjectRequest{
			Bucket: "test-bucket",
			Key:    "test-key",
		})

		resp, err := service.DeleteObject(context.Background(), req)
		if err != nil {
			t.Fatalf("DeleteObject failed: %v", err)
		}

		if !resp.Msg.Success {
			t.Errorf("Expected success to be true, got false")
		}

		// Verify the file was deleted
		filePath := filepath.Join(tempDir, "test-bucket", "test-key")
		if _, err := os.Stat(filePath); !os.IsNotExist(err) {
			t.Errorf("Expected file to be deleted at %s", filePath)
		}
	})
}
