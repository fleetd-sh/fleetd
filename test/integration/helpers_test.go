package integration

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

var (
	// Shared PostgreSQL container and connection pool
	sharedDB       *sql.DB
	sharedConnStr  string
	containerMutex sync.Mutex
	containerSetup sync.Once
	schemaSetup    sync.Once
	containerCtx   = context.Background()
	pgContainer    testcontainers.Container
)

// TestMain sets up shared resources for all integration tests
func TestMain(m *testing.M) {
	var cleanup func()

	// Check if we should use shared container
	if os.Getenv("INTEGRATION") != "" {
		sharedDB, cleanup = setupSharedPostgres()
		if sharedDB != nil {
			defer cleanup()
		}
	}

	// Run tests
	code := m.Run()
	os.Exit(code)
}

// setupSharedPostgres creates a single PostgreSQL container for all tests
func setupSharedPostgres() (*sql.DB, func()) {
	var err error

	// Start PostgreSQL container once
	pgContainer, err = postgres.RunContainer(containerCtx,
		testcontainers.WithImage("postgres:17-alpine"),
		postgres.WithDatabase("fleetd_test"),
		postgres.WithUsername("fleetd"),
		postgres.WithPassword("testpass"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		fmt.Printf("Failed to start shared PostgreSQL container: %v\n", err)
		return nil, func() {}
	}

	// Get connection string
	pgContainerTyped := pgContainer.(*postgres.PostgresContainer)
	sharedConnStr, err = pgContainerTyped.ConnectionString(containerCtx, "sslmode=disable")
	if err != nil {
		fmt.Printf("Failed to get connection string: %v\n", err)
		pgContainer.Terminate(containerCtx)
		return nil, func() {}
	}

	// Connect to database
	sharedDB, err = sql.Open("postgres", sharedConnStr)
	if err != nil {
		fmt.Printf("Failed to connect to shared database: %v\n", err)
		pgContainer.Terminate(containerCtx)
		return nil, func() {}
	}

	// Configure connection pool for parallel tests
	sharedDB.SetMaxOpenConns(25)
	sharedDB.SetMaxIdleConns(10)
	sharedDB.SetConnMaxLifetime(time.Hour)

	// Setup schema once
	if err := setupSharedSchema(sharedDB); err != nil {
		fmt.Printf("Failed to setup schema: %v\n", err)
		sharedDB.Close()
		pgContainer.Terminate(containerCtx)
		return nil, func() {}
	}

	fmt.Println("✓ Shared PostgreSQL container ready for integration tests")

	// Return cleanup function
	cleanup := func() {
		if sharedDB != nil {
			sharedDB.Close()
		}
		if pgContainer != nil {
			if err := pgContainer.Terminate(containerCtx); err != nil {
				fmt.Printf("Failed to terminate container: %v\n", err)
			}
		}
	}

	return sharedDB, cleanup
}

// setupSharedSchema creates the schema once for the shared database
func setupSharedSchema(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS device (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		status TEXT DEFAULT 'offline',
		labels TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS telemetry (
		id SERIAL PRIMARY KEY,
		device_id TEXT NOT NULL,
		timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		cpu_usage REAL,
		memory_usage REAL,
		disk_usage REAL,
		network_usage REAL,
		temperature REAL,
		FOREIGN KEY (device_id) REFERENCES device(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS logs (
		id SERIAL PRIMARY KEY,
		device_id TEXT NOT NULL,
		timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		level TEXT,
		message TEXT,
		metadata TEXT,
		FOREIGN KEY (device_id) REFERENCES device(id) ON DELETE CASCADE
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

	CREATE TABLE IF NOT EXISTS device_auth_request (
		id TEXT PRIMARY KEY,
		device_code TEXT UNIQUE NOT NULL,
		user_code TEXT UNIQUE NOT NULL,
		verification_url TEXT NOT NULL,
		expires_at TIMESTAMP NOT NULL,
		interval_seconds INTEGER DEFAULT 5,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		user_id TEXT,
		approved_at TIMESTAMP,
		client_id TEXT DEFAULT 'fleetctl',
		client_name TEXT,
		ip_address TEXT
	);

	CREATE TABLE IF NOT EXISTS user_account (
		id TEXT PRIMARY KEY,
		email TEXT UNIQUE NOT NULL,
		password_hash TEXT,
		name TEXT,
		role TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS access_token (
		id TEXT PRIMARY KEY,
		token TEXT UNIQUE NOT NULL,
		user_id TEXT NOT NULL,
		device_auth_id TEXT,
		client_id TEXT,
		expires_at TIMESTAMP NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		revoked_at TIMESTAMP,
		last_used TIMESTAMP,
		FOREIGN KEY (user_id) REFERENCES user_account(id) ON DELETE CASCADE,
		FOREIGN KEY (device_auth_id) REFERENCES device_auth_request(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS deployment (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		namespace TEXT DEFAULT 'default',
		description TEXT,
		type TEXT,
		status TEXT,
		manifest TEXT,
		payload TEXT,
		target TEXT,
		strategy TEXT,
		selector TEXT,
		config TEXT,
		metadata TEXT,
		created_by TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS device_deployment (
		deployment_id TEXT,
		device_id TEXT,
		status TEXT,
		progress INTEGER,
		message TEXT,
		started_at TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (deployment_id, device_id),
		FOREIGN KEY (deployment_id) REFERENCES deployment(id) ON DELETE CASCADE,
		FOREIGN KEY (device_id) REFERENCES device(id) ON DELETE CASCADE
	);
	`

	_, err := db.Exec(schema)
	return err
}

// setupTestDatabase creates a test database for integration tests
// Uses the shared container with simple cleanup-based isolation
// NOTE: For true parallel test isolation, tests should NOT run in parallel (remove t.Parallel())
// OR we accept that tests run sequentially but share the same container startup cost
func setupTestDatabase(t *testing.T) *sql.DB {
	// If shared DB is available, use it with simple cleanup
	if sharedDB != nil {
		// Clean all tables before test starts
		// This is fast enough for sequential tests
		cleanupTestData(t, sharedDB)

		// Seed test data
		seedTestData(t, sharedDB)

		// Register cleanup to clean tables after test
		t.Cleanup(func() {
			cleanupTestData(t, sharedDB)
		})

		return sharedDB
	}

	// Fallback: Create isolated container (for tests run outside TestMain)
	ctx := context.Background()

	pgContainer, err := postgres.RunContainer(ctx,
		testcontainers.WithImage("postgres:17-alpine"),
		postgres.WithDatabase("fleetd_test"),
		postgres.WithUsername("fleetd"),
		postgres.WithPassword("testpass"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	require.NoError(t, err)

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	t.Cleanup(func() {
		if err := pgContainer.Terminate(ctx); err != nil {
			t.Logf("Failed to terminate postgres container: %v", err)
		}
	})

	db, err := sql.Open("postgres", connStr)
	require.NoError(t, err)

	// Setup schema for isolated container
	err = setupSharedSchema(db)
	require.NoError(t, err)

	seedTestData(t, db)

	return db
}

// sanitizeSchemaName converts test name to valid PostgreSQL schema name
func sanitizeSchemaName(name string) string {
	// Replace invalid characters with underscores
	result := ""
	for _, ch := range name {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' {
			result += string(ch)
		} else {
			result += "_"
		}
	}
	// Ensure it starts with a letter or underscore
	if len(result) > 0 && result[0] >= '0' && result[0] <= '9' {
		result = "t_" + result
	}
	// Limit length to 63 characters (PostgreSQL identifier limit)
	if len(result) > 50 {
		result = result[:50]
	}
	return result
}

// cleanupTestData removes all data from tables (but keeps schema)
func cleanupTestData(t *testing.T, db *sql.DB) {
	// Delete in order to respect foreign keys
	tables := []string{
		"device_deployment",
		"deployment",
		"access_token",
		"device_auth_request",
		"user_account",
		"telemetry",
		"logs",
		"alerts",
		"settings",
		"device",
	}

	for _, table := range tables {
		_, err := db.Exec(fmt.Sprintf("DELETE FROM %s", table))
		if err != nil {
			t.Logf("Warning: failed to clean table %s: %v", table, err)
		}
	}

	// Clean migration state to avoid dirty state issues
	_, _ = db.Exec("DROP TABLE IF EXISTS schema_migrations CASCADE")
}

// safeCloseDB closes database only if it's not the shared database
// Use this instead of db.Close() in tests to avoid closing shared container DB
func safeCloseDB(db *sql.DB) {
	if db != nil && db != sharedDB {
		db.Close()
	}
}

// setupBenchDatabase creates a database for benchmarks
func setupBenchDatabase(b *testing.B) *sql.DB {
	ctx := context.Background()

	// Start PostgreSQL container
	pgContainer, err := postgres.RunContainer(ctx,
		testcontainers.WithImage("postgres:17-alpine"),
		postgres.WithDatabase("fleetd_bench"),
		postgres.WithUsername("fleetd"),
		postgres.WithPassword("testpass"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	require.NoError(b, err)

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(b, err)

	b.Cleanup(func() {
		if err := pgContainer.Terminate(ctx); err != nil {
			b.Logf("Failed to terminate postgres container: %v", err)
		}
	})

	db, err := sql.Open("postgres", connStr)
	require.NoError(b, err)

	// Use same schema as test database
	schema := `
	CREATE TABLE IF NOT EXISTS device (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		status TEXT DEFAULT 'offline',
		labels TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	`

	_, err = db.Exec(schema)
	require.NoError(b, err)

	return db
}

// seedTestData adds initial test data to the database
func seedTestData(t *testing.T, db *sql.DB) {
	// Insert test devices
	devices := []struct {
		id     string
		name   string
		status string
	}{
		{"device-001", "Test Device 1", "online"},
		{"device-002", "Test Device 2", "online"},
		{"device-003", "Test Device 3", "offline"},
	}

	for _, d := range devices {
		_, err := db.Exec(
			"INSERT INTO device (id, name, status) VALUES ($1, $2, $3)",
			d.id, d.name, d.status,
		)
		require.NoError(t, err)
	}

	// Insert default settings
	defaultSettings := map[string]string{
		"org.name":         "Test Organization",
		"org.email":        "test@example.com",
		"security.2fa":     "false",
		"security.timeout": "30",
		"api.rate_limit":   "100",
	}

	for key, value := range defaultSettings {
		_, err := db.Exec(
			"INSERT INTO settings (key, value) VALUES ($1, $2)",
			key, value,
		)
		require.NoError(t, err)
	}
}

// getTestAPIURL returns the API URL for testing
func getTestAPIURL() string {
	if url := os.Getenv("PLATFORM_API_URL"); url != "" {
		return url
	}
	return "http://localhost:8090"
}

// skipIfShort skips the test if running in short mode
func skipIfShort(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
}

// requireDocker checks if Docker is available
func requireDocker(t *testing.T) {
	if _, err := exec.Command("docker", "version").Output(); err != nil {
		t.Skip("Docker not available")
	}
}

// requireIntegrationMode checks if integration tests are enabled
func requireIntegrationMode(t *testing.T) {
	if os.Getenv("INTEGRATION") == "" {
		t.Skip("Integration tests not enabled (set INTEGRATION=true)")
	}
}
