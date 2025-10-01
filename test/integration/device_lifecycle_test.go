package integration

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	pb "fleetd.sh/gen/fleetd/v1"
	"fleetd.sh/gen/fleetd/v1/fleetpbconnect"
	"fleetd.sh/internal/api"
	"github.com/stretchr/testify/suite"
	_ "modernc.org/sqlite"
)

// DeviceLifecycleTestSuite tests the complete device lifecycle
type DeviceLifecycleTestSuite struct {
	suite.Suite
	server       *httptest.Server
	db           *sql.DB
	deviceClient fleetpbconnect.DeviceServiceClient
	updateClient fleetpbconnect.UpdateServiceClient
	ctx          context.Context
	cancel       context.CancelFunc
}

// SetupSuite runs once before all tests
func (s *DeviceLifecycleTestSuite) SetupSuite() {
	// Create context
	s.ctx, s.cancel = context.WithTimeout(context.Background(), 5*time.Minute)

	// Setup test database
	s.db = s.setupTestDatabase()

	// Create test tables directly (simplified for SQLite)
	err := s.createTestTables()
	s.Require().NoError(err)

	// Setup test server
	s.server = s.setupTestServer()

	// Create clients
	s.deviceClient = fleetpbconnect.NewDeviceServiceClient(
		http.DefaultClient,
		s.server.URL,
	)

	s.updateClient = fleetpbconnect.NewUpdateServiceClient(
		http.DefaultClient,
		s.server.URL,
	)
}

// TearDownSuite runs once after all tests
func (s *DeviceLifecycleTestSuite) TearDownSuite() {
	s.cancel()
	if s.server != nil {
		s.server.Close()
	}
	if s.db != nil {
		s.db.Close()
	}
}

// setupTestDatabase creates an in-memory SQLite database for testing
func (s *DeviceLifecycleTestSuite) setupTestDatabase() *sql.DB {
	db, err := sql.Open("sqlite3", ":memory:")
	s.Require().NoError(err)

	// Set connection pool for testing
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	// Test connection
	err = db.Ping()
	s.Require().NoError(err)

	return db
}

// createTestTables creates simplified tables for SQLite testing
func (s *DeviceLifecycleTestSuite) createTestTables() error {
	// Create device table
	deviceTable := `
	CREATE TABLE IF NOT EXISTS device (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		type TEXT NOT NULL,
		version TEXT NOT NULL,
		api_key TEXT UNIQUE,
		certificate TEXT,
		last_seen DATETIME,
		status TEXT DEFAULT 'offline',
		metadata TEXT,
		system_info TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		deleted_at DATETIME
	);
	CREATE INDEX IF NOT EXISTS idx_device_type ON device(type);
	CREATE INDEX IF NOT EXISTS idx_device_status ON device(status);
	CREATE INDEX IF NOT EXISTS idx_device_last_seen ON device(last_seen);
	`

	if _, err := s.db.Exec(deviceTable); err != nil {
		return err
	}

	// Create update_campaign table
	updateTable := `
	CREATE TABLE IF NOT EXISTS update_campaign (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		version TEXT NOT NULL,
		description TEXT,
		strategy TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		started_at DATETIME,
		completed_at DATETIME,
		status TEXT DEFAULT 'pending'
	);
	`

	if _, err := s.db.Exec(updateTable); err != nil {
		return err
	}

	// Create metrics table
	metricsTable := `
	CREATE TABLE IF NOT EXISTS metrics (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		device_id TEXT NOT NULL,
		metric_name TEXT NOT NULL,
		metric_value TEXT,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (device_id) REFERENCES device(id)
	);
	CREATE INDEX IF NOT EXISTS idx_metrics_device ON metrics(device_id);
	CREATE INDEX IF NOT EXISTS idx_metrics_timestamp ON metrics(timestamp);
	`

	if _, err := s.db.Exec(metricsTable); err != nil {
		return err
	}

	// Create device_system_info table
	systemInfoTable := `
	CREATE TABLE IF NOT EXISTS device_system_info (
		device_id TEXT PRIMARY KEY,
		hostname TEXT,
		os TEXT,
		os_version TEXT,
		arch TEXT,
		cpu_model TEXT,
		cpu_cores INTEGER,
		memory_total INTEGER,
		storage_total INTEGER,
		kernel_version TEXT,
		platform TEXT,
		timezone TEXT,
		agent_version TEXT,
		serial_number TEXT,
		product_name TEXT,
		manufacturer TEXT,
		extra TEXT,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (device_id) REFERENCES device(id)
	);
	`

	if _, err := s.db.Exec(systemInfoTable); err != nil {
		return err
	}

	return nil
}

// setupTestServer creates a test HTTP server with all middleware
func (s *DeviceLifecycleTestSuite) setupTestServer() *httptest.Server {
	// Create a new mux for testing
	mux := http.NewServeMux()

	// Initialize services manually for testing
	deviceService := api.NewDeviceService(s.db)
	updateService := api.NewUpdateService(s.db)
	analyticsService := api.NewAnalyticsService(s.db)

	// Register Connect services
	path, handler := fleetpbconnect.NewDeviceServiceHandler(deviceService)
	mux.Handle(path, handler)

	path, handler = fleetpbconnect.NewUpdateServiceHandler(updateService)
	mux.Handle(path, handler)

	path, handler = fleetpbconnect.NewAnalyticsServiceHandler(analyticsService)
	mux.Handle(path, handler)

	// Start test server with the mux
	return httptest.NewServer(mux)
}

// TestDeviceRegistration tests device registration flow
func (s *DeviceLifecycleTestSuite) TestDeviceRegistration() {
	// Register a new device
	req := connect.NewRequest(&pb.RegisterRequest{
		Name:    "Test Device 001",
		Type:    "test",
		Version: "1.0.0",
		SystemInfo: &pb.SystemInfo{
			Os:            "Linux",
			Arch:          "x86_64",
			CpuCores:      4,
			MemoryTotal:   8192 * 1024 * 1024,       // Convert MB to bytes
			StorageTotal:  100 * 1024 * 1024 * 1024, // Convert GB to bytes
			Hostname:      "test-host",
			KernelVersion: "5.15.0",
		},
	})

	resp, err := s.deviceClient.Register(s.ctx, req)
	s.Require().NoError(err)
	s.Assert().NotNil(resp)
	s.Assert().NotEmpty(resp.Msg.DeviceId)
	s.Assert().NotEmpty(resp.Msg.ApiKey)

	// Store device ID for verification
	deviceID := resp.Msg.DeviceId

	// Verify device exists
	getReq := connect.NewRequest(&pb.GetDeviceRequest{
		DeviceId: deviceID,
	})

	getResp, err := s.deviceClient.GetDevice(s.ctx, getReq)
	s.Require().NoError(err)
	s.Assert().NotNil(getResp.Msg.Device)
	s.Assert().Equal(deviceID, getResp.Msg.Device.Id)
	s.Assert().Equal("Test Device 001", getResp.Msg.Device.Name)
	s.Assert().Equal("test", getResp.Msg.Device.Type)
}

// TestDuplicateRegistration tests duplicate device registration
func (s *DeviceLifecycleTestSuite) TestDuplicateRegistration() {
	// Register first device
	req := connect.NewRequest(&pb.RegisterRequest{
		Name:    "Test Device Duplicate",
		Type:    "test",
		Version: "1.0.0",
	})

	resp, err := s.deviceClient.Register(s.ctx, req)
	s.Require().NoError(err)
	deviceID := resp.Msg.DeviceId

	// Try to register with same device ID is not directly possible
	// as the ID is generated by the server
	// Instead test registering another device and ensure it gets a different ID
	req2 := connect.NewRequest(&pb.RegisterRequest{
		Name:    "Test Device Duplicate 2",
		Type:    "test",
		Version: "1.0.0",
	})

	resp2, err := s.deviceClient.Register(s.ctx, req2)
	s.Require().NoError(err)
	s.Assert().NotEqual(deviceID, resp2.Msg.DeviceId)
}

// TestDeviceHeartbeat tests device heartbeat functionality
func (s *DeviceLifecycleTestSuite) TestDeviceHeartbeat() {
	// Register device first
	registerReq := connect.NewRequest(&pb.RegisterRequest{
		Name:    "Heartbeat Test Device",
		Type:    "test",
		Version: "1.0.0",
	})

	registerResp, err := s.deviceClient.Register(s.ctx, registerReq)
	s.Require().NoError(err)
	deviceID := registerResp.Msg.DeviceId

	// Send heartbeat
	heartbeatReq := connect.NewRequest(&pb.HeartbeatRequest{
		DeviceId: deviceID,
		Metrics: map[string]string{
			"cpu_usage":    "25.5",
			"memory_usage": "2048",
			"disk_usage":   "50",
			"uptime":       "3600",
		},
	})

	// Add API key for authentication if needed
	heartbeatReq.Header().Set("X-API-Key", registerResp.Msg.ApiKey)

	heartbeatResp, err := s.deviceClient.Heartbeat(s.ctx, heartbeatReq)
	s.Require().NoError(err)
	s.Assert().NotNil(heartbeatResp)

	// Verify device last_seen is updated
	time.Sleep(100 * time.Millisecond) // Allow time for update

	getReq := connect.NewRequest(&pb.GetDeviceRequest{
		DeviceId: deviceID,
	})

	getResp, err := s.deviceClient.GetDevice(s.ctx, getReq)
	s.Require().NoError(err)
	s.Assert().NotNil(getResp.Msg.Device.LastSeen)
	s.Assert().WithinDuration(time.Now(), getResp.Msg.Device.LastSeen.AsTime(), 5*time.Second)
}

// TestDeviceList tests listing devices with filtering
func (s *DeviceLifecycleTestSuite) TestDeviceList() {
	// Register multiple devices
	devices := []struct {
		name  string
		dtype string
	}{
		{"Device 1", "raspberry-pi"},
		{"Device 2", "esp32"},
		{"Device 3", "raspberry-pi"},
		{"Device 4", "test"},
	}

	registeredIDs := make([]string, 0, len(devices))
	for _, d := range devices {
		req := connect.NewRequest(&pb.RegisterRequest{
			Name:    d.name,
			Type:    d.dtype,
			Version: "1.0.0",
		})
		resp, err := s.deviceClient.Register(s.ctx, req)
		s.Require().NoError(err)
		registeredIDs = append(registeredIDs, resp.Msg.DeviceId)
	}

	// Test listing all devices
	listReq := connect.NewRequest(&pb.ListDevicesRequest{
		PageSize: 10,
	})

	listResp, err := s.deviceClient.ListDevices(s.ctx, listReq)
	s.Require().NoError(err)
	s.Assert().GreaterOrEqual(len(listResp.Msg.Devices), 4)

	// Test filtering by type
	filterReq := connect.NewRequest(&pb.ListDevicesRequest{
		Type:     "raspberry-pi",
		PageSize: 10,
	})

	filterResp, err := s.deviceClient.ListDevices(s.ctx, filterReq)
	s.Require().NoError(err)

	// Verify raspberry-pi devices are included
	piCount := 0
	for _, device := range filterResp.Msg.Devices {
		if device.Type == "raspberry-pi" {
			piCount++
		}
	}
	s.Assert().GreaterOrEqual(piCount, 2) // We registered 2 raspberry-pi devices
}

// TestDeviceDelete tests device deletion
func (s *DeviceLifecycleTestSuite) TestDeviceDelete() {
	// Register device
	registerReq := connect.NewRequest(&pb.RegisterRequest{
		Name:    "Device to Delete",
		Type:    "test",
		Version: "1.0.0",
	})

	resp, err := s.deviceClient.Register(s.ctx, registerReq)
	s.Require().NoError(err)
	deviceID := resp.Msg.DeviceId

	// Delete device
	deleteReq := connect.NewRequest(&pb.DeleteDeviceRequest{
		DeviceId: deviceID,
	})

	deleteResp, err := s.deviceClient.DeleteDevice(s.ctx, deleteReq)
	s.Require().NoError(err)
	s.Assert().True(deleteResp.Msg.Success)

	// Verify device is deleted
	getReq := connect.NewRequest(&pb.GetDeviceRequest{
		DeviceId: deviceID,
	})

	_, err = s.deviceClient.GetDevice(s.ctx, getReq)
	s.Assert().Error(err)

	var connectErr *connect.Error
	s.Require().ErrorAs(err, &connectErr)
	s.Assert().Equal(connect.CodeNotFound, connectErr.Code())
}

// TestDeviceMetrics tests device metrics collection
func (s *DeviceLifecycleTestSuite) TestDeviceMetrics() {
	// Register device
	registerReq := connect.NewRequest(&pb.RegisterRequest{
		Name:    "Metrics Test Device",
		Type:    "test",
		Version: "1.0.0",
	})

	resp, err := s.deviceClient.Register(s.ctx, registerReq)
	s.Require().NoError(err)
	deviceID := resp.Msg.DeviceId

	// Send multiple heartbeats with metrics
	for i := 0; i < 3; i++ {
		heartbeatReq := connect.NewRequest(&pb.HeartbeatRequest{
			DeviceId: deviceID,
			Metrics: map[string]string{
				"cpu":    fmt.Sprintf("%d", 20+i*10),
				"memory": fmt.Sprintf("%d", 1000+i*500),
				"disk":   fmt.Sprintf("%d", 40+i*5),
			},
		})

		_, err := s.deviceClient.Heartbeat(s.ctx, heartbeatReq)
		s.Require().NoError(err)

		time.Sleep(100 * time.Millisecond)
	}

	// Get device
	getReq := connect.NewRequest(&pb.GetDeviceRequest{
		DeviceId: deviceID,
	})

	getResp, err := s.deviceClient.GetDevice(s.ctx, getReq)
	s.Require().NoError(err)
	s.Assert().NotNil(getResp.Msg.Device)
	// Verify device has been updated with heartbeats
	s.Assert().NotNil(getResp.Msg.Device.LastSeen)
}

// TestConcurrentOperations tests concurrent device operations
func (s *DeviceLifecycleTestSuite) TestConcurrentOperations() {
	const numDevices = 10
	errChan := make(chan error, numDevices)
	doneChan := make(chan string, numDevices)

	// Register devices concurrently
	for i := 0; i < numDevices; i++ {
		go func(idx int) {
			req := connect.NewRequest(&pb.RegisterRequest{
				Name:    fmt.Sprintf("Concurrent Device %d", idx),
				Type:    "test",
				Version: "1.0.0",
			})

			resp, err := s.deviceClient.Register(s.ctx, req)
			if err != nil {
				errChan <- err
				doneChan <- ""
			} else {
				doneChan <- resp.Msg.DeviceId
			}
		}(i)
	}

	// Collect registered device IDs
	registeredDevices := make([]string, 0, numDevices)
	timeout := time.After(10 * time.Second)

	for len(registeredDevices) < numDevices {
		select {
		case deviceID := <-doneChan:
			if deviceID != "" {
				registeredDevices = append(registeredDevices, deviceID)
			}
		case err := <-errChan:
			s.T().Logf("Concurrent registration error: %v", err)
		case <-timeout:
			s.T().Fatal("Timeout waiting for concurrent registrations")
		}
	}

	// Verify all devices were created
	s.Assert().Equal(numDevices, len(registeredDevices))

	// Send heartbeats for all registered devices
	for _, deviceID := range registeredDevices {
		req := connect.NewRequest(&pb.HeartbeatRequest{
			DeviceId: deviceID,
			Metrics: map[string]string{
				"status": "online",
			},
		})
		_, err := s.deviceClient.Heartbeat(s.ctx, req)
		s.Assert().NoError(err)
	}
}

// TestRun runs the test suite
func TestDeviceLifecycleTestSuite(t *testing.T) {
	suite.Run(t, new(DeviceLifecycleTestSuite))
}
