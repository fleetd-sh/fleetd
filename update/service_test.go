package update_test

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/protobuf/types/known/timestamppb"

	updatepb "fleetd.sh/gen/update/v1"
	"fleetd.sh/internal/migrations"
	"fleetd.sh/internal/testutil"
	"fleetd.sh/update"
)

func TestUpdateService(t *testing.T) {
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
	service := update.NewUpdateService(db)

	// Test CreateUpdatePackage
	t.Run("CreateUpdatePackage", func(t *testing.T) {
		releaseDate := time.Now().UTC().Truncate(time.Second)
		req := connect.NewRequest(&updatepb.CreateUpdatePackageRequest{
			Id:          "update-001",
			Version:     "1.0.0",
			ReleaseDate: timestamppb.New(releaseDate),
			ChangeLog:   "Initial release",
			FileUrl:     "https://example.com/updates/1.0.0",
			DeviceTypes: []string{"SENSOR", "ACTUATOR"},
		})

		resp, err := service.CreateUpdatePackage(context.Background(), req)
		if err != nil {
			t.Fatalf("CreateUpdatePackage failed: %v", err)
		}

		if !resp.Msg.Success {
			t.Errorf("Expected success to be true, got false")
		}

		// Verify the update package was created in the database
		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM update_package WHERE id = ?", "update-001").Scan(&count)
		if err != nil {
			t.Fatalf("Failed to query database: %v", err)
		}
		if count != 1 {
			t.Errorf("Expected 1 update package, got %d", count)
		}
	})

	// Test GetAvailableUpdates
	t.Run("GetAvailableUpdates", func(t *testing.T) {
		// Insert a test update package
		_, err := db.Exec(`
			INSERT INTO update_package (id, version, release_date, change_log, file_url, device_types)
			VALUES (?, ?, ?, ?, ?, ?)
		`, "update-002", "1.1.0", time.Now().Add(-24*time.Hour), "Bug fixes", "https://example.com/updates/1.1.0", `["SENSOR"]`)
		if err != nil {
			t.Fatalf("Failed to insert test update package: %v", err)
		}

		req := connect.NewRequest(&updatepb.GetAvailableUpdatesRequest{
			DeviceType:     "SENSOR",
			LastUpdateDate: timestamppb.New(time.Now().Add(-48 * time.Hour)),
		})

		resp, err := service.GetAvailableUpdates(context.Background(), req)
		if err != nil {
			t.Fatalf("GetAvailableUpdates failed: %v", err)
		}

		if len(resp.Msg.Updates) != 2 {
			t.Errorf("Expected 2 available updates, got %d", len(resp.Msg.Updates))
		}

		// Verify the content of the updates
		expectedUpdates := []*updatepb.UpdatePackage{
			{
				Id:          "update-001",
				Version:     "1.0.0",
				ChangeLog:   "Initial release",
				FileUrl:     "https://example.com/updates/1.0.0",
				DeviceTypes: []string{"SENSOR", "ACTUATOR"},
			},
			{
				Id:          "update-002",
				Version:     "1.1.0",
				ChangeLog:   "Bug fixes",
				FileUrl:     "https://example.com/updates/1.1.0",
				DeviceTypes: []string{"SENSOR"},
			},
		}

		// Custom comparison function
		equalUpdatePackage := func(a, b *updatepb.UpdatePackage) bool {
			return a.Id == b.Id &&
				a.Version == b.Version &&
				a.ChangeLog == b.ChangeLog &&
				a.FileUrl == b.FileUrl &&
				cmp.Equal(a.DeviceTypes, b.DeviceTypes)
		}

		opts := []cmp.Option{
			cmp.Comparer(equalUpdatePackage),
			cmpopts.SortSlices(func(a, b *updatepb.UpdatePackage) bool {
				return a.Id < b.Id
			}),
		}

		if diff := cmp.Diff(expectedUpdates, resp.Msg.Updates, opts...); diff != "" {
			t.Errorf("Updates mismatch (-want +got):\n%s", diff)
		}
	})
}
