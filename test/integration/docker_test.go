package integration

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"testing"
	"time"

	"connectrpc.com/connect"
	fleetpb "fleetd.sh/gen/fleetd/v1"
	"fleetd.sh/gen/fleetd/v1/fleetpbconnect"
	"fleetd.sh/internal/database"
	"fleetd.sh/internal/services"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestWithPostgresContainer tests the services with a real PostgreSQL container
func TestWithPostgresContainer(t *testing.T) {
	requireIntegrationMode(t)
	if testing.Short() {
		t.Skip("Skipping container test in short mode")
	}

	ctx := context.Background()

	// Start PostgreSQL container
	postgresContainer, err := postgres.RunContainer(ctx,
		testcontainers.WithImage("postgres:17-alpine"),
		postgres.WithDatabase("fleetd_test"),
		postgres.WithUsername("fleetd_test"),
		postgres.WithPassword("test_password"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	require.NoError(t, err)
	defer postgresContainer.Terminate(ctx)

	// Get connection string
	connStr, err := postgresContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	// Connect to the database
	db, err := sql.Open("postgres", connStr)
	require.NoError(t, err)
	defer safeCloseDB(db)

	// Create schema
	err = createPostgresSchema(db)
	require.NoError(t, err)

	// Seed test data
	seedPostgresTestData(t, db)

	// Now run tests with the PostgreSQL database
	t.Run("TelemetryWithPostgres", func(t *testing.T) {
		// First ensure the device exists
		_, err := db.Exec(`
			INSERT INTO device (id, name, status)
			VALUES ('docker-test-device', 'Docker Test Device', 'online')
			ON CONFLICT (id) DO NOTHING
		`)
		require.NoError(t, err)

		// Test telemetry operations work with PostgreSQL
		_, err = db.Exec(`
			INSERT INTO telemetry (device_id, cpu_usage, memory_usage, disk_usage)
			VALUES ('docker-test-device', 45.5, 62.3, 78.1)
		`)
		require.NoError(t, err)

		var cpuUsage float64
		err = db.QueryRow(`
			SELECT cpu_usage FROM telemetry
			WHERE device_id = 'docker-test-device'
			ORDER BY timestamp DESC LIMIT 1
		`).Scan(&cpuUsage)
		require.NoError(t, err)
		assert.Equal(t, 45.5, cpuUsage)
	})

	t.Run("SettingsWithPostgres", func(t *testing.T) {
		// Test settings operations work with PostgreSQL
		_, err := db.Exec(`
			INSERT INTO settings (key, value)
			VALUES ('test.setting', 'test-value')
			ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value
		`)
		require.NoError(t, err)

		var value string
		err = db.QueryRow(`
			SELECT value FROM settings WHERE key = 'test.setting'
		`).Scan(&value)
		require.NoError(t, err)
		assert.Equal(t, "test-value", value)
	})
}

// TestWithRedisContainer tests caching with a real Redis container
func TestWithRedisContainer(t *testing.T) {
	requireIntegrationMode(t)
	if testing.Short() {
		t.Skip("Skipping container test in short mode")
	}

	ctx := context.Background()

	// Start Redis container
	redisContainer, err := redis.RunContainer(ctx,
		testcontainers.WithImage("redis:7-alpine"),
		redis.WithLogLevel(redis.LogLevelDebug),
		testcontainers.WithWaitStrategy(
			wait.ForLog("Ready to accept connections").
				WithStartupTimeout(30*time.Second),
		),
	)
	require.NoError(t, err)
	defer redisContainer.Terminate(ctx)

	// Get Redis connection string
	connStr, err := redisContainer.ConnectionString(ctx)
	require.NoError(t, err)

	t.Log("Redis running at:", connStr)

	// Here you would test Redis-dependent functionality
	// For now, just verify the container is running
	endpoint, err := redisContainer.Endpoint(ctx, "")
	require.NoError(t, err)
	assert.NotEmpty(t, endpoint)
}

// TestFullStackWithContainers tests the full stack with all containers
func TestFullStackWithContainers(t *testing.T) {
	requireIntegrationMode(t)
	if testing.Short() {
		t.Skip("Skipping container test in short mode")
	}

	ctx := context.Background()

	// Start PostgreSQL
	postgresContainer, err := postgres.RunContainer(ctx,
		testcontainers.WithImage("postgres:17-alpine"),
		postgres.WithDatabase("fleetd_test"),
		postgres.WithUsername("fleetd_test"),
		postgres.WithPassword("test_password"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	require.NoError(t, err)
	defer postgresContainer.Terminate(ctx)

	// Start Redis
	redisContainer, err := redis.RunContainer(ctx,
		testcontainers.WithImage("redis:7-alpine"),
		redis.WithLogLevel(redis.LogLevelDebug),
		testcontainers.WithWaitStrategy(
			wait.ForLog("Ready to accept connections").
				WithStartupTimeout(30*time.Second),
		),
	)
	require.NoError(t, err)
	defer redisContainer.Terminate(ctx)

	// Get connection strings
	pgConnStr, err := postgresContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	redisEndpoint, err := redisContainer.Endpoint(ctx, "")
	require.NoError(t, err)

	// Connect to PostgreSQL
	db, err := sql.Open("postgres", pgConnStr)
	require.NoError(t, err)
	defer safeCloseDB(db)

	// Setup database
	err = createPostgresSchema(db)
	require.NoError(t, err)
	seedPostgresTestData(t, db)

	// Start test server with real database
	server, url := startTestServerWithDB(t, db)
	defer server.Close()

	// Create clients
	telemetryClient := fleetpbconnect.NewTelemetryServiceClient(
		http.DefaultClient,
		url,
	)
	settingsClient := fleetpbconnect.NewSettingsServiceClient(
		http.DefaultClient,
		url,
	)

	t.Log("Test server running with PostgreSQL at:", pgConnStr)
	t.Log("Redis available at:", redisEndpoint)

	// Run integration tests against real services
	t.Run("RealDatabaseIntegration", func(t *testing.T) {
		// Test telemetry with real PostgreSQL
		resp, err := telemetryClient.GetTelemetry(
			context.Background(),
			connect.NewRequest(&fleetpb.GetTelemetryRequest{
				DeviceId: "postgres-device",
				Limit:    10,
			}),
		)
		require.NoError(t, err)
		assert.NotNil(t, resp)

		// Test settings with real PostgreSQL
		settingsResp, err := settingsClient.GetOrganizationSettings(
			context.Background(),
			connect.NewRequest(&fleetpb.GetOrganizationSettingsRequest{}),
		)
		require.NoError(t, err)
		assert.NotNil(t, settingsResp)
	})
}

// TestDockerComposeWithExec tests docker-compose using exec.Command
func TestDockerComposeWithExec(t *testing.T) {
	requireIntegrationMode(t)
	if testing.Short() {
		t.Skip("Skipping container test in short mode")
	}

	// Check if docker-compose is available
	if err := exec.Command("docker-compose", "version").Run(); err != nil {
		t.Skip("docker-compose not available")
	}

	ctx := context.Background()
	composeFile := "../../test/docker-compose.test.yml"

	// Start the compose stack
	cmd := exec.CommandContext(ctx, "docker-compose", "-f", composeFile, "up", "-d", "postgres", "redis")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to start docker-compose: %v\nOutput: %s", err, output)
	}

	// Ensure cleanup
	defer func() {
		cmd := exec.Command("docker-compose", "-f", composeFile, "down", "-v")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Logf("Failed to stop docker-compose: %v\nOutput: %s", err, output)
		}
	}()

	// Wait for services to be ready
	time.Sleep(5 * time.Second)

	// Test PostgreSQL connectivity
	t.Run("PostgreSQLConnectivity", func(t *testing.T) {
		db, err := sql.Open("postgres", "postgres://fleetd_test:test_password@localhost:5433/fleetd_test?sslmode=disable")
		require.NoError(t, err)
		defer safeCloseDB(db)

		err = db.Ping()
		assert.NoError(t, err)
		t.Log("PostgreSQL is accessible via docker-compose")
	})

	// Test Redis connectivity using HTTP check (since we don't have a Redis client)
	t.Run("RedisHealthCheck", func(t *testing.T) {
		// We can't directly connect to Redis without a client, but we can verify the port is open
		conn, err := net.Dial("tcp", "localhost:6380")
		if err == nil {
			conn.Close()
			t.Log("Redis port is accessible via docker-compose")
		} else {
			t.Logf("Redis port check failed: %v", err)
		}
	})
}

// Helper functions

func createPostgresSchema(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS device (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		status TEXT DEFAULT 'offline',
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS telemetry (
		id SERIAL PRIMARY KEY,
		device_id TEXT NOT NULL REFERENCES device(id),
		timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		cpu_usage REAL,
		memory_usage REAL,
		disk_usage REAL,
		network_usage REAL,
		temperature REAL
	);

	CREATE TABLE IF NOT EXISTS logs (
		id SERIAL PRIMARY KEY,
		device_id TEXT NOT NULL REFERENCES device(id),
		timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		level TEXT,
		message TEXT,
		metadata JSONB
	);

	CREATE TABLE IF NOT EXISTS settings (
		key TEXT PRIMARY KEY,
		value TEXT,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS alerts (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		description TEXT,
		type TEXT,
		threshold REAL,
		condition TEXT,
		enabled BOOLEAN DEFAULT true,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_telemetry_device_time ON telemetry(device_id, timestamp DESC);
	CREATE INDEX IF NOT EXISTS idx_logs_device_time ON logs(device_id, timestamp DESC);
	`

	_, err := db.Exec(schema)
	return err
}

func seedPostgresTestData(t *testing.T, db *sql.DB) {
	// Insert test devices
	devices := []struct {
		id     string
		name   string
		status string
	}{
		{"postgres-device", "PostgreSQL Test Device", "online"},
		{"docker-device-001", "Docker Device 1", "online"},
		{"docker-device-002", "Docker Device 2", "offline"},
	}

	for _, d := range devices {
		_, err := db.Exec(
			"INSERT INTO device (id, name, status) VALUES ($1, $2, $3) ON CONFLICT (id) DO NOTHING",
			d.id, d.name, d.status,
		)
		require.NoError(t, err)
	}

	// Insert default settings
	defaultSettings := map[string]string{
		"org.name":         "Docker Test Organization",
		"org.email":        "docker@test.com",
		"security.2fa":     "false",
		"security.timeout": "30",
		"api.rate_limit":   "100",
	}

	for key, value := range defaultSettings {
		_, err := db.Exec(
			"INSERT INTO settings (key, value) VALUES ($1, $2) ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value",
			key, value,
		)
		require.NoError(t, err)
	}

	// Insert some test telemetry data
	for i := 0; i < 5; i++ {
		_, err := db.Exec(`
			INSERT INTO telemetry (device_id, cpu_usage, memory_usage, disk_usage)
			VALUES ($1, $2, $3, $4)
		`, "postgres-device", 20.0+float64(i*5), 40.0+float64(i*3), 60.0+float64(i*2))
		require.NoError(t, err)
		time.Sleep(10 * time.Millisecond) // Small delay to ensure different timestamps
	}
}

// startTestServerWithDB starts a test server with a specific database
func startTestServerWithDB(t *testing.T, db *sql.DB) (*httptest.Server, string) {
	// Create services with the provided database
	dbWrapper := &database.DB{DB: db}
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
		fmt.Fprint(w, "OK")
	})

	server := httptest.NewServer(mux)
	return server, server.URL
}
