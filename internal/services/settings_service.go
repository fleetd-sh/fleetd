package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"connectrpc.com/connect"
	fleetpb "fleetd.sh/gen/fleetd/v1"
	"fleetd.sh/gen/fleetd/v1/fleetpbconnect"
	"fleetd.sh/internal/database"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type SettingsService struct {
	fleetpbconnect.UnimplementedSettingsServiceHandler
	db *database.DB
	// In-memory storage for demo purposes
	orgSettings    *fleetpb.OrganizationSettings
	secSettings    *fleetpb.SecuritySettings
	notifSettings  *fleetpb.NotificationSettings
	apiSettings    *fleetpb.APISettings
	advSettings    *fleetpb.AdvancedSettings
}

func NewSettingsService(db *database.DB) *SettingsService {
	// Initialize with default settings
	return &SettingsService{
		db: db,
		orgSettings: &fleetpb.OrganizationSettings{
			Name:         "Acme Corporation",
			ContactEmail: "admin@acme.com",
			Timezone:     "UTC",
			Language:     "en",
			LogoUrl:      "",
			CreatedAt:    timestamppb.Now(),
			UpdatedAt:    timestamppb.Now(),
		},
		secSettings: &fleetpb.SecuritySettings{
			TwoFactorRequired:    true,
			SessionTimeoutMinutes: 30,
			IpWhitelistEnabled:   false,
			AllowedIps:          []string{},
			AuditLoggingEnabled: true,
			PasswordPolicy: &fleetpb.PasswordPolicy{
				MinLength:           8,
				RequireUppercase:    true,
				RequireLowercase:    true,
				RequireNumbers:      true,
				RequireSpecialChars: false,
				ExpiryDays:         90,
			},
		},
		notifSettings: &fleetpb.NotificationSettings{
			EmailNotifications: &fleetpb.EmailNotifications{
				DeviceOfflineAlerts:     true,
				DeploymentStatusUpdates: true,
				SecurityAlerts:         true,
				WeeklySummary:          false,
				RecipientEmails:        []string{"admin@acme.com"},
			},
			WebhookSettings: &fleetpb.WebhookSettings{
				Enabled: false,
				Url:     "",
				Secret:  "",
				Events:  []string{},
			},
			AlertThresholds: &fleetpb.AlertThresholds{
				CpuUsagePercent:        80,
				MemoryUsagePercent:     85,
				DiskUsagePercent:       90,
				OfflineDurationMinutes: 5,
			},
		},
		apiSettings: &fleetpb.APISettings{
			ApiKey:             generateAPIKey(),
			RateLimitPerMinute: 60,
			RateLimitPerHour:   1000,
			CorsSettings: &fleetpb.CORSSettings{
				AllowedOrigins:   []string{"*"},
				AllowCredentials: true,
				AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
				AllowedHeaders:   []string{"*"},
			},
			ApiKeyCreatedAt: timestamppb.Now(),
		},
		advSettings: &fleetpb.AdvancedSettings{
			DataRetention: &fleetpb.DataRetention{
				TelemetryDays:  30,
				LogsDays:       90,
				AuditLogsDays:  365,
				BackupsDays:    30,
			},
			ExperimentalFeatures: &fleetpb.ExperimentalFeatures{
				BetaFeaturesEnabled: false,
				DebugModeEnabled:   false,
				EnabledFeatures:    []string{},
			},
		},
	}
}

// GetOrganizationSettings retrieves organization settings
func (s *SettingsService) GetOrganizationSettings(
	ctx context.Context,
	req *connect.Request[fleetpb.GetOrganizationSettingsRequest],
) (*connect.Response[fleetpb.GetOrganizationSettingsResponse], error) {
	return connect.NewResponse(&fleetpb.GetOrganizationSettingsResponse{
		Settings: s.orgSettings,
	}), nil
}

// UpdateOrganizationSettings updates organization settings
func (s *SettingsService) UpdateOrganizationSettings(
	ctx context.Context,
	req *connect.Request[fleetpb.UpdateOrganizationSettingsRequest],
) (*connect.Response[fleetpb.UpdateOrganizationSettingsResponse], error) {
	s.orgSettings = req.Msg.Settings
	s.orgSettings.UpdatedAt = timestamppb.Now()
	
	return connect.NewResponse(&fleetpb.UpdateOrganizationSettingsResponse{
		Settings: s.orgSettings,
	}), nil
}

// GetSecuritySettings retrieves security settings
func (s *SettingsService) GetSecuritySettings(
	ctx context.Context,
	req *connect.Request[fleetpb.GetSecuritySettingsRequest],
) (*connect.Response[fleetpb.GetSecuritySettingsResponse], error) {
	return connect.NewResponse(&fleetpb.GetSecuritySettingsResponse{
		Settings: s.secSettings,
	}), nil
}

// UpdateSecuritySettings updates security settings
func (s *SettingsService) UpdateSecuritySettings(
	ctx context.Context,
	req *connect.Request[fleetpb.UpdateSecuritySettingsRequest],
) (*connect.Response[fleetpb.UpdateSecuritySettingsResponse], error) {
	s.secSettings = req.Msg.Settings
	
	return connect.NewResponse(&fleetpb.UpdateSecuritySettingsResponse{
		Settings: s.secSettings,
	}), nil
}

// GetNotificationSettings retrieves notification settings
func (s *SettingsService) GetNotificationSettings(
	ctx context.Context,
	req *connect.Request[fleetpb.GetNotificationSettingsRequest],
) (*connect.Response[fleetpb.GetNotificationSettingsResponse], error) {
	return connect.NewResponse(&fleetpb.GetNotificationSettingsResponse{
		Settings: s.notifSettings,
	}), nil
}

// UpdateNotificationSettings updates notification settings
func (s *SettingsService) UpdateNotificationSettings(
	ctx context.Context,
	req *connect.Request[fleetpb.UpdateNotificationSettingsRequest],
) (*connect.Response[fleetpb.UpdateNotificationSettingsResponse], error) {
	s.notifSettings = req.Msg.Settings
	
	return connect.NewResponse(&fleetpb.UpdateNotificationSettingsResponse{
		Settings: s.notifSettings,
	}), nil
}

// GetAPISettings retrieves API settings
func (s *SettingsService) GetAPISettings(
	ctx context.Context,
	req *connect.Request[fleetpb.GetAPISettingsRequest],
) (*connect.Response[fleetpb.GetAPISettingsResponse], error) {
	return connect.NewResponse(&fleetpb.GetAPISettingsResponse{
		Settings: s.apiSettings,
	}), nil
}

// UpdateAPISettings updates API settings
func (s *SettingsService) UpdateAPISettings(
	ctx context.Context,
	req *connect.Request[fleetpb.UpdateAPISettingsRequest],
) (*connect.Response[fleetpb.UpdateAPISettingsResponse], error) {
	s.apiSettings = req.Msg.Settings
	
	if req.Msg.RegenerateKey {
		s.apiSettings.ApiKey = generateAPIKey()
		s.apiSettings.ApiKeyCreatedAt = timestamppb.Now()
	}
	
	return connect.NewResponse(&fleetpb.UpdateAPISettingsResponse{
		Settings: s.apiSettings,
	}), nil
}

// RegenerateAPIKey regenerates the API key
func (s *SettingsService) RegenerateAPIKey(
	ctx context.Context,
	req *connect.Request[fleetpb.RegenerateAPIKeyRequest],
) (*connect.Response[fleetpb.RegenerateAPIKeyResponse], error) {
	newKey := generateAPIKey()
	s.apiSettings.ApiKey = newKey
	s.apiSettings.ApiKeyCreatedAt = timestamppb.Now()
	
	return connect.NewResponse(&fleetpb.RegenerateAPIKeyResponse{
		NewApiKey: newKey,
		CreatedAt: s.apiSettings.ApiKeyCreatedAt,
	}), nil
}

// GetAdvancedSettings retrieves advanced settings
func (s *SettingsService) GetAdvancedSettings(
	ctx context.Context,
	req *connect.Request[fleetpb.GetAdvancedSettingsRequest],
) (*connect.Response[fleetpb.GetAdvancedSettingsResponse], error) {
	return connect.NewResponse(&fleetpb.GetAdvancedSettingsResponse{
		Settings: s.advSettings,
	}), nil
}

// UpdateAdvancedSettings updates advanced settings
func (s *SettingsService) UpdateAdvancedSettings(
	ctx context.Context,
	req *connect.Request[fleetpb.UpdateAdvancedSettingsRequest],
) (*connect.Response[fleetpb.UpdateAdvancedSettingsResponse], error) {
	s.advSettings = req.Msg.Settings
	
	return connect.NewResponse(&fleetpb.UpdateAdvancedSettingsResponse{
		Settings: s.advSettings,
	}), nil
}

// ExportData exports system data
func (s *SettingsService) ExportData(
	ctx context.Context,
	req *connect.Request[fleetpb.ExportDataRequest],
) (*connect.Response[fleetpb.ExportDataResponse], error) {
	// In production, this would generate an actual export
	// For now, return a mock response
	return connect.NewResponse(&fleetpb.ExportDataResponse{
		DownloadUrl: fmt.Sprintf("https://api.fleetd.io/exports/%s.%s", generateExportId(), req.Msg.Format),
		SizeBytes:   1024 * 1024 * 50, // 50MB mock size
		ExpiresAt:   timestamppb.New(time.Now().Add(24 * time.Hour)),
	}), nil
}

// DeleteAllData deletes all system data (danger zone)
func (s *SettingsService) DeleteAllData(
	ctx context.Context,
	req *connect.Request[fleetpb.DeleteAllDataRequest],
) (*connect.Response[fleetpb.DeleteAllDataResponse], error) {
	// Verify confirmation code
	if req.Msg.ConfirmationCode != "DELETE-ALL-DATA" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid confirmation code"))
	}
	
	// In production, this would actually delete data
	// For safety in demo, we don't actually delete anything
	
	return connect.NewResponse(&fleetpb.DeleteAllDataResponse{
		Success: true,
		Message: "All data has been marked for deletion. This process may take up to 24 hours to complete.",
	}), nil
}

// Helper functions

func generateAPIKey() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("fleetd_sk_%s", hex.EncodeToString(b))
}

func generateExportId() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}
