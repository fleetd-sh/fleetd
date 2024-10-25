package device_test

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"fleetd.sh/device"
	devicepb "fleetd.sh/gen/device/v1"
	"fleetd.sh/internal/migrations"
	"fleetd.sh/internal/testutil"
	"fleetd.sh/pkg/authclient"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestUpdateDevice(t *testing.T) {
	// Initialize the test database
	db, cleanup, err := testutil.NewDBTemp()
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer cleanup()

	// Run migrations
	if err := migrations.MigrateUp(db); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	// Initialize the service
	authClient := authclient.NewClient("http://localhost:8081") // Mock auth client URL
	service := device.NewDeviceService(db, authClient)

	// Insert a test device
	_, err = db.Exec(`
		INSERT INTO device (id, name, type, status, last_seen)
		VALUES (?, ?, ?, ?, ?)
	`, "device-123", "Test Device", "SENSOR", "ACTIVE", time.Now())
	if err != nil {
		t.Fatalf("Failed to insert test device: %v", err)
	}

	// Create the request
	req := connect.NewRequest(&devicepb.UpdateDeviceRequest{
		DeviceId: "device-123",
		Device: &devicepb.Device{
			Name:     "New Device Name",
			Type:     "New Device Type",
			Status:   "ACTIVE",
			LastSeen: timestamppb.Now(),
		},
	})

	// Call the method
	resp, err := service.UpdateDevice(context.Background(), req)
	if err != nil {
		t.Fatalf("UpdateDevice failed: %v", err)
	}

	// Verify response
	if !resp.Msg.Success {
		t.Errorf("expected success true, got false")
	}

	// Verify the device was updated in the database
	var updatedName, updatedType, updatedStatus string
	err = db.QueryRow("SELECT name, type, status FROM device WHERE id = ?", "device-123").Scan(&updatedName, &updatedType, &updatedStatus)
	if err != nil {
		t.Fatalf("Failed to query updated device: %v", err)
	}

	if updatedName != "New Device Name" {
		t.Errorf("expected name 'New Device Name', got '%s'", updatedName)
	}
	if updatedType != "New Device Type" {
		t.Errorf("expected type 'New Device Type', got '%s'", updatedType)
	}
	if updatedStatus != "ACTIVE" {
		t.Errorf("expected status 'ACTIVE', got '%s'", updatedStatus)
	}
}
