package e2e

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"connectrpc.com/connect"
	fleetpb "fleetd.sh/gen/fleetd/v1"
	"fleetd.sh/gen/fleetd/v1/fleetpbconnect"
	"fleetd.sh/internal/database"
	"fleetd.sh/internal/services"
	"github.com/stretchr/testify/suite"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// FullStackTestSuite tests the complete system with all services
type FullStackTestSuite struct {
	suite.Suite
	server          *httptest.Server
	db              *sql.DB
	baseURL         string
	telemetryClient fleetpbconnect.TelemetryServiceClient
	settingsClient  fleetpbconnect.SettingsServiceClient
}

func (s *FullStackTestSuite) SetupSuite() {
	// Skip if not in integration mode
	if os.Getenv("INTEGRATION") == "" {
		s.T().Skip("Skipping integration test - set INTEGRATION=1 to run")
	}

	// Setup database
	s.db = s.setupDatabase()

	// Create services
	dbWrapper := &database.DB{DB: s.db}
	telemetryService := services.NewTelemetryService(dbWrapper)
	settingsService := services.NewSettingsService(dbWrapper)

	// Create server mux
	mux := http.NewServeMux()

	// Register telemetry service
	telemetryPath, telemetryHandler := fleetpbconnect.NewTelemetryServiceHandler(telemetryService)
	mux.Handle(telemetryPath, telemetryHandler)

	// Register settings service
	settingsPath, settingsHandler := fleetpbconnect.NewSettingsServiceHandler(settingsService)
	mux.Handle(settingsPath, settingsHandler)

	// Add health endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Start test server
	s.server = httptest.NewServer(mux)
	s.baseURL = s.server.URL

	// Create clients
	s.telemetryClient = fleetpbconnect.NewTelemetryServiceClient(
		http.DefaultClient,
		s.baseURL,
	)
	s.settingsClient = fleetpbconnect.NewSettingsServiceClient(
		http.DefaultClient,
		s.baseURL,
	)
}

func (s *FullStackTestSuite) TearDownSuite() {
	if s.server != nil {
		s.server.Close()
	}
	if s.db != nil {
		s.db.Close()
	}
}

func (s *FullStackTestSuite) TestFullWorkflow() {
	// Test complete workflow across all services
	ctx := context.Background()

	// 1. Configure organization settings
	s.Run("ConfigureOrganization", func() {
		orgSettings := &fleetpb.OrganizationSettings{
			Name:         "E2E Test Corp",
			ContactEmail: "e2e@test.com",
			Timezone:     "UTC",
			Language:     "en",
		}

		_, err := s.settingsClient.UpdateOrganizationSettings(
			ctx,
			connect.NewRequest(&fleetpb.UpdateOrganizationSettingsRequest{
				Settings: orgSettings,
			}),
		)
		s.Require().NoError(err)
	})

	// 2. Configure security settings
	s.Run("ConfigureSecurity", func() {
		secSettings := &fleetpb.SecuritySettings{
			TwoFactorRequired:     false,
			SessionTimeoutMinutes: 30,
			AuditLoggingEnabled:   true,
			PasswordPolicy: &fleetpb.PasswordPolicy{
				MinLength:        8,
				RequireUppercase: true,
				RequireLowercase: true,
				RequireNumbers:   true,
			},
		}

		_, err := s.settingsClient.UpdateSecuritySettings(
			ctx,
			connect.NewRequest(&fleetpb.UpdateSecuritySettingsRequest{
				Settings: secSettings,
			}),
		)
		s.Require().NoError(err)
	})

	// 3. Set up telemetry alerts
	s.Run("ConfigureAlerts", func() {
		alerts := []struct {
			name      string
			alertType fleetpb.AlertType
			threshold float64
		}{
			{"High CPU Usage", fleetpb.AlertType_ALERT_TYPE_CPU, 80},
			{"High Memory Usage", fleetpb.AlertType_ALERT_TYPE_MEMORY, 85},
			{"Low Disk Space", fleetpb.AlertType_ALERT_TYPE_DISK, 90},
		}

		for _, alert := range alerts {
			_, err := s.telemetryClient.ConfigureAlert(
				ctx,
				connect.NewRequest(&fleetpb.ConfigureAlertRequest{
					Alert: &fleetpb.Alert{
						Name:        alert.name,
						Description: fmt.Sprintf("Alert for %s", alert.name),
						Type:        alert.alertType,
						Threshold:   alert.threshold,
						Condition:   fleetpb.AlertCondition_ALERT_CONDITION_GREATER_THAN,
						Enabled:     true,
					},
				}),
			)
			s.Require().NoError(err)
		}

		// Verify alerts were created
		listResp, err := s.telemetryClient.ListAlerts(
			ctx,
			connect.NewRequest(&fleetpb.ListAlertsRequest{
				EnabledOnly: true,
			}),
		)
		s.Require().NoError(err)
		s.Assert().GreaterOrEqual(len(listResp.Msg.Alerts), 3)
	})

	// 4. Collect and verify telemetry
	s.Run("CollectTelemetry", func() {
		deviceID := "e2e-test-device"

		// Get telemetry data
		telemetryResp, err := s.telemetryClient.GetTelemetry(
			ctx,
			connect.NewRequest(&fleetpb.GetTelemetryRequest{
				DeviceId: deviceID,
				Limit:    5,
			}),
		)
		s.Require().NoError(err)
		s.Assert().NotEmpty(telemetryResp.Msg.Data)

		// Get aggregated metrics
		metricsResp, err := s.telemetryClient.GetMetrics(
			ctx,
			connect.NewRequest(&fleetpb.GetMetricsRequest{
				DeviceIds:   []string{deviceID},
				MetricNames: []string{"cpu", "memory"},
				Aggregation: "avg",
			}),
		)
		s.Require().NoError(err)
		s.Assert().NotEmpty(metricsResp.Msg.Metrics)
	})

	// 5. Test real-time streaming
	s.Run("StreamTelemetry", func() {
		s.T().Skip("Skipping streaming test - requires real stream implementation")

		ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()

		stream, err := s.telemetryClient.StreamTelemetry(
			ctx,
			connect.NewRequest(&fleetpb.StreamTelemetryRequest{
				DeviceIds: []string{"e2e-test-device"},
			}),
		)
		s.Require().NoError(err)

		messageCount := 0
		for stream.Receive() && messageCount < 2 {
			msg := stream.Msg()
			s.Assert().NotNil(msg)
			s.Assert().Equal("e2e-test-device", msg.DeviceId)
			messageCount++
		}
		s.Assert().Greater(messageCount, 0)
	})

	// 6. Test data export
	s.Run("ExportData", func() {
		exportResp, err := s.settingsClient.ExportData(
			ctx,
			connect.NewRequest(&fleetpb.ExportDataRequest{
				DataTypes: []string{"telemetry", "settings"},
				Format:    "json",
			}),
		)
		s.Require().NoError(err)
		s.Assert().NotEmpty(exportResp.Msg.DownloadUrl)
		s.Assert().Greater(exportResp.Msg.SizeBytes, int64(0))
	})
}

func (s *FullStackTestSuite) TestConcurrentRequests() {
	// Test concurrent access to services
	ctx := context.Background()
	concurrency := 10
	errChan := make(chan error, concurrency*3)

	for i := 0; i < concurrency; i++ {
		// Concurrent telemetry requests
		go func(idx int) {
			_, err := s.telemetryClient.GetTelemetry(
				ctx,
				connect.NewRequest(&fleetpb.GetTelemetryRequest{
					DeviceId: fmt.Sprintf("device-%d", idx),
					Limit:    10,
				}),
			)
			errChan <- err
		}(i)

		// Concurrent settings reads
		go func() {
			_, err := s.settingsClient.GetOrganizationSettings(
				ctx,
				connect.NewRequest(&fleetpb.GetOrganizationSettingsRequest{}),
			)
			errChan <- err
		}()

		// Concurrent log queries
		go func(idx int) {
			_, err := s.telemetryClient.GetLogs(
				ctx,
				connect.NewRequest(&fleetpb.GetLogsRequest{
					DeviceIds: []string{fmt.Sprintf("device-%d", idx)},
					Limit:     20,
				}),
			)
			errChan <- err
		}(i)
	}

	// Collect errors
	for i := 0; i < concurrency*3; i++ {
		err := <-errChan
		s.Assert().NoError(err)
	}
}

func (s *FullStackTestSuite) TestErrorHandling() {
	ctx := context.Background()

	// Test invalid confirmation code for data deletion
	_, err := s.settingsClient.DeleteAllData(
		ctx,
		connect.NewRequest(&fleetpb.DeleteAllDataRequest{
			ConfirmationCode: "WRONG-CODE",
		}),
	)
	s.Assert().Error(err)
	s.Assert().Contains(err.Error(), "invalid")

	// Test telemetry with time range filter
	now := time.Now()
	_, err = s.telemetryClient.GetTelemetry(
		ctx,
		connect.NewRequest(&fleetpb.GetTelemetryRequest{
			DeviceId:  "test-device",
			StartTime: timestamppb.New(now.Add(24 * time.Hour)), // Future
			EndTime:   timestamppb.New(now),
		}),
	)
	// Should succeed but return empty data
	s.Assert().NoError(err)
}

// Helper methods

func (s *FullStackTestSuite) setupDatabase() *sql.DB {
	dbConfig := database.DefaultConfig("sqlite3")
	dbConfig.DSN = ":memory:"
	dbConfig.MigrationsPath = "../../migrations"

	dbInstance, err := database.New(dbConfig)
	s.Require().NoError(err)

	return dbInstance.DB
}

// waitForServer is no longer needed with httptest.Server

func TestFullStack(t *testing.T) {
	suite.Run(t, new(FullStackTestSuite))
}
