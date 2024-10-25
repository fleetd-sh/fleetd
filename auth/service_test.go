package auth_test

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"fleetd.sh/auth"
	authpb "fleetd.sh/gen/auth/v1"
	"fleetd.sh/internal/migrations"
	"fleetd.sh/internal/testutil"
	"github.com/google/uuid"
)

func TestAuthService_Authenticate(t *testing.T) {
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
	service, err := auth.NewAuthService(db)
	if err != nil {
		t.Fatalf("Failed to create auth service: %v", err)
	}

	// Insert a test API key
	testAPIKey := uuid.New().String()
	testDeviceID := uuid.New().String()
	_, err = db.Exec("INSERT INTO api_key (api_key, device_id) VALUES (?, ?)", testAPIKey, testDeviceID)
	if err != nil {
		t.Fatalf("Failed to insert test API key: %v", err)
	}

	// Test cases
	testCases := []struct {
		name           string
		apiKey         string
		expectAuth     bool
		expectDeviceID string
	}{
		{"Valid API Key", testAPIKey, true, testDeviceID},
		{"Invalid API Key", "invalid_key", false, ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := connect.NewRequest(&authpb.AuthenticateRequest{ApiKey: tc.apiKey})
			resp, err := service.Authenticate(context.Background(), req)

			if err != nil {
				t.Fatalf("Authenticate failed: %v", err)
			}

			if resp.Msg.Authenticated != tc.expectAuth {
				t.Errorf("Expected authenticated=%v, got %v", tc.expectAuth, resp.Msg.Authenticated)
			}

			if resp.Msg.DeviceId != tc.expectDeviceID {
				t.Errorf("Expected deviceID=%s, got %s", tc.expectDeviceID, resp.Msg.DeviceId)
			}
		})
	}
}

func TestAuthService_GenerateAPIKey(t *testing.T) {
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
	service, err := auth.NewAuthService(db)
	if err != nil {
		t.Fatalf("Failed to create auth service: %v", err)
	}

	// Test generating an API key
	testDeviceID := uuid.New().String()
	req := connect.NewRequest(&authpb.GenerateAPIKeyRequest{DeviceId: testDeviceID})
	resp, err := service.GenerateAPIKey(context.Background(), req)

	if err != nil {
		t.Fatalf("GenerateAPIKey failed: %v", err)
	}

	if resp.Msg.ApiKey == "" {
		t.Error("Expected non-empty API key")
	}

	// Verify the API key was inserted into the database
	var storedDeviceID string
	err = db.QueryRow("SELECT device_id FROM api_key WHERE api_key = ?", resp.Msg.ApiKey).Scan(&storedDeviceID)
	if err != nil {
		t.Fatalf("Failed to query generated API key: %v", err)
	}

	if storedDeviceID != testDeviceID {
		t.Errorf("Expected deviceID=%s, got %s", testDeviceID, storedDeviceID)
	}
}

func TestAuthService_RevokeAPIKey(t *testing.T) {
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
	service, err := auth.NewAuthService(db)
	if err != nil {
		t.Fatalf("Failed to create auth service: %v", err)
	}

	// Insert a test API key
	testAPIKey := uuid.New().String()
	testDeviceID := uuid.New().String()
	_, err = db.Exec("INSERT INTO api_key (api_key, device_id) VALUES (?, ?)", testAPIKey, testDeviceID)
	if err != nil {
		t.Fatalf("Failed to insert test API key: %v", err)
	}

	// Test revoking the API key
	req := connect.NewRequest(&authpb.RevokeAPIKeyRequest{DeviceId: testDeviceID})
	resp, err := service.RevokeAPIKey(context.Background(), req)

	if err != nil {
		t.Fatalf("RevokeAPIKey failed: %v", err)
	}

	if !resp.Msg.Success {
		t.Error("Expected successful revocation")
	}

	// Verify the API key was removed from the database
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM api_key WHERE device_id = ?", testDeviceID).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query API keys: %v", err)
	}

	if count != 0 {
		t.Errorf("Expected 0 API keys for device, got %d", count)
	}
}
