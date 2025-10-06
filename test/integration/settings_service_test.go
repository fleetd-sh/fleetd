package integration

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	fleetpb "fleetd.sh/gen/fleetd/v1"
	"fleetd.sh/gen/fleetd/v1/fleetpbconnect"
	"fleetd.sh/internal/database"
	"fleetd.sh/internal/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSettingsService(t *testing.T) {
	requireIntegrationMode(t)
	// Create test database
	db := setupTestDatabase(t)
	defer safeCloseDB(db)

	// Create service
	dbWrapper := &database.DB{DB: db}
	service := services.NewSettingsService(dbWrapper)

	// Create test server
	mux := http.NewServeMux()
	path, handler := fleetpbconnect.NewSettingsServiceHandler(service)
	mux.Handle(path, handler)
	server := httptest.NewServer(mux)
	defer server.Close()

	// Create client
	client := fleetpbconnect.NewSettingsServiceClient(
		http.DefaultClient,
		server.URL,
	)

	t.Run("OrganizationSettings", func(t *testing.T) {
		// Get initial settings
		getResp, err := client.GetOrganizationSettings(
			context.Background(),
			connect.NewRequest(&fleetpb.GetOrganizationSettingsRequest{}),
		)
		require.NoError(t, err)
		assert.NotNil(t, getResp.Msg.Settings)

		initialName := getResp.Msg.Settings.Name

		// Update settings
		updatedSettings := getResp.Msg.Settings
		updatedSettings.Name = "Updated Corp"
		updatedSettings.ContactEmail = "updated@example.com"
		updatedSettings.Timezone = "America/New_York"

		updateResp, err := client.UpdateOrganizationSettings(
			context.Background(),
			connect.NewRequest(&fleetpb.UpdateOrganizationSettingsRequest{
				Settings: updatedSettings,
			}),
		)
		require.NoError(t, err)
		assert.Equal(t, "Updated Corp", updateResp.Msg.Settings.Name)
		assert.Equal(t, "updated@example.com", updateResp.Msg.Settings.ContactEmail)

		// Verify persistence
		verifyResp, err := client.GetOrganizationSettings(
			context.Background(),
			connect.NewRequest(&fleetpb.GetOrganizationSettingsRequest{}),
		)
		require.NoError(t, err)
		assert.Equal(t, "Updated Corp", verifyResp.Msg.Settings.Name)

		// Restore original
		updatedSettings.Name = initialName
		_, err = client.UpdateOrganizationSettings(
			context.Background(),
			connect.NewRequest(&fleetpb.UpdateOrganizationSettingsRequest{
				Settings: updatedSettings,
			}),
		)
		require.NoError(t, err)
	})

	t.Run("SecuritySettings", func(t *testing.T) {
		// Get initial settings
		getResp, err := client.GetSecuritySettings(
			context.Background(),
			connect.NewRequest(&fleetpb.GetSecuritySettingsRequest{}),
		)
		require.NoError(t, err)
		assert.NotNil(t, getResp.Msg.Settings)

		// Update settings
		updatedSettings := getResp.Msg.Settings
		updatedSettings.TwoFactorRequired = true
		updatedSettings.SessionTimeoutMinutes = 60
		updatedSettings.AuditLoggingEnabled = true

		if updatedSettings.PasswordPolicy == nil {
			updatedSettings.PasswordPolicy = &fleetpb.PasswordPolicy{}
		}
		updatedSettings.PasswordPolicy.MinLength = 12
		updatedSettings.PasswordPolicy.RequireUppercase = true
		updatedSettings.PasswordPolicy.RequireNumbers = true

		updateResp, err := client.UpdateSecuritySettings(
			context.Background(),
			connect.NewRequest(&fleetpb.UpdateSecuritySettingsRequest{
				Settings: updatedSettings,
			}),
		)
		require.NoError(t, err)
		assert.True(t, updateResp.Msg.Settings.TwoFactorRequired)
		assert.Equal(t, int32(60), updateResp.Msg.Settings.SessionTimeoutMinutes)
		assert.Equal(t, int32(12), updateResp.Msg.Settings.PasswordPolicy.MinLength)
	})

	t.Run("NotificationSettings", func(t *testing.T) {
		// Get initial settings
		getResp, err := client.GetNotificationSettings(
			context.Background(),
			connect.NewRequest(&fleetpb.GetNotificationSettingsRequest{}),
		)
		require.NoError(t, err)
		assert.NotNil(t, getResp.Msg.Settings)

		// Update email notifications
		updatedSettings := getResp.Msg.Settings
		if updatedSettings.EmailNotifications == nil {
			updatedSettings.EmailNotifications = &fleetpb.EmailNotifications{}
		}
		updatedSettings.EmailNotifications.DeviceOfflineAlerts = true
		updatedSettings.EmailNotifications.DeploymentStatusUpdates = true
		updatedSettings.EmailNotifications.SecurityAlerts = true
		updatedSettings.EmailNotifications.RecipientEmails = []string{
			"admin@test.com",
			"ops@test.com",
		}

		// Update alert thresholds
		if updatedSettings.AlertThresholds == nil {
			updatedSettings.AlertThresholds = &fleetpb.AlertThresholds{}
		}
		updatedSettings.AlertThresholds.CpuUsagePercent = 85
		updatedSettings.AlertThresholds.MemoryUsagePercent = 90
		updatedSettings.AlertThresholds.DiskUsagePercent = 95

		updateResp, err := client.UpdateNotificationSettings(
			context.Background(),
			connect.NewRequest(&fleetpb.UpdateNotificationSettingsRequest{
				Settings: updatedSettings,
			}),
		)
		require.NoError(t, err)
		assert.True(t, updateResp.Msg.Settings.EmailNotifications.DeviceOfflineAlerts)
		assert.Equal(t, float64(85), updateResp.Msg.Settings.AlertThresholds.CpuUsagePercent)
	})

	t.Run("APISettings", func(t *testing.T) {
		// Get initial settings
		getResp, err := client.GetAPISettings(
			context.Background(),
			connect.NewRequest(&fleetpb.GetAPISettingsRequest{}),
		)
		require.NoError(t, err)
		assert.NotNil(t, getResp.Msg.Settings)
		assert.NotEmpty(t, getResp.Msg.Settings.ApiKey)

		originalKey := getResp.Msg.Settings.ApiKey

		// Regenerate API key
		regenResp, err := client.RegenerateAPIKey(
			context.Background(),
			connect.NewRequest(&fleetpb.RegenerateAPIKeyRequest{}),
		)
		require.NoError(t, err)
		assert.NotEmpty(t, regenResp.Msg.NewApiKey)
		assert.NotEqual(t, originalKey, regenResp.Msg.NewApiKey)

		// Verify new key is stored
		verifyResp, err := client.GetAPISettings(
			context.Background(),
			connect.NewRequest(&fleetpb.GetAPISettingsRequest{}),
		)
		require.NoError(t, err)
		assert.Equal(t, regenResp.Msg.NewApiKey, verifyResp.Msg.Settings.ApiKey)

		// Update rate limits
		updatedSettings := verifyResp.Msg.Settings
		updatedSettings.RateLimitPerMinute = 120
		updatedSettings.RateLimitPerHour = 5000

		updateResp, err := client.UpdateAPISettings(
			context.Background(),
			connect.NewRequest(&fleetpb.UpdateAPISettingsRequest{
				Settings: updatedSettings,
			}),
		)
		require.NoError(t, err)
		assert.Equal(t, int32(120), updateResp.Msg.Settings.RateLimitPerMinute)
		assert.Equal(t, int32(5000), updateResp.Msg.Settings.RateLimitPerHour)
	})

	t.Run("AdvancedSettings", func(t *testing.T) {
		// Get initial settings
		getResp, err := client.GetAdvancedSettings(
			context.Background(),
			connect.NewRequest(&fleetpb.GetAdvancedSettingsRequest{}),
		)
		require.NoError(t, err)
		assert.NotNil(t, getResp.Msg.Settings)

		// Update data retention
		updatedSettings := getResp.Msg.Settings
		if updatedSettings.DataRetention == nil {
			updatedSettings.DataRetention = &fleetpb.DataRetention{}
		}
		updatedSettings.DataRetention.TelemetryDays = 60
		updatedSettings.DataRetention.LogsDays = 180
		updatedSettings.DataRetention.AuditLogsDays = 730

		// Update experimental features
		if updatedSettings.ExperimentalFeatures == nil {
			updatedSettings.ExperimentalFeatures = &fleetpb.ExperimentalFeatures{}
		}
		updatedSettings.ExperimentalFeatures.BetaFeaturesEnabled = true
		updatedSettings.ExperimentalFeatures.EnabledFeatures = []string{
			"advanced-telemetry",
			"multi-region",
		}

		updateResp, err := client.UpdateAdvancedSettings(
			context.Background(),
			connect.NewRequest(&fleetpb.UpdateAdvancedSettingsRequest{
				Settings: updatedSettings,
			}),
		)
		require.NoError(t, err)
		assert.Equal(t, int32(60), updateResp.Msg.Settings.DataRetention.TelemetryDays)
		assert.True(t, updateResp.Msg.Settings.ExperimentalFeatures.BetaFeaturesEnabled)
		assert.Contains(t, updateResp.Msg.Settings.ExperimentalFeatures.EnabledFeatures, "advanced-telemetry")
	})

	t.Run("DataExport", func(t *testing.T) {
		req := &fleetpb.ExportDataRequest{
			DataTypes: []string{"devices", "telemetry", "logs"},
			Format:    "json",
		}

		resp, err := client.ExportData(
			context.Background(),
			connect.NewRequest(req),
		)
		require.NoError(t, err)
		assert.NotEmpty(t, resp.Msg.DownloadUrl)
		assert.Greater(t, resp.Msg.SizeBytes, int64(0))
		assert.NotNil(t, resp.Msg.ExpiresAt)
	})

	t.Run("DeleteAllData_InvalidCode", func(t *testing.T) {
		req := &fleetpb.DeleteAllDataRequest{
			ConfirmationCode: "INVALID-CODE",
		}

		_, err := client.DeleteAllData(
			context.Background(),
			connect.NewRequest(req),
		)
		assert.Error(t, err)
	})

	t.Run("DeleteAllData_ValidCode", func(t *testing.T) {
		t.Skip("Skipping destructive test")

		req := &fleetpb.DeleteAllDataRequest{
			ConfirmationCode: "DELETE-ALL-DATA",
		}

		resp, err := client.DeleteAllData(
			context.Background(),
			connect.NewRequest(req),
		)
		require.NoError(t, err)
		assert.True(t, resp.Msg.Success)
		assert.NotEmpty(t, resp.Msg.Message)
	})
}
