package integration

import (
	"database/sql"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// setupTestDatabase creates a test database for integration tests
func setupTestDatabase(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)

	// Create basic schema
	schema := `
	CREATE TABLE IF NOT EXISTS device (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		status TEXT DEFAULT 'offline',
		labels TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS telemetry (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		device_id TEXT NOT NULL,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		cpu_usage REAL,
		memory_usage REAL,
		disk_usage REAL,
		network_usage REAL,
		temperature REAL,
		FOREIGN KEY (device_id) REFERENCES device(id)
	);

	CREATE TABLE IF NOT EXISTS logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		device_id TEXT NOT NULL,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		level TEXT,
		message TEXT,
		metadata TEXT,
		FOREIGN KEY (device_id) REFERENCES device(id)
	);

	CREATE TABLE IF NOT EXISTS settings (
		key TEXT PRIMARY KEY,
		value TEXT,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS alerts (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		description TEXT,
		type TEXT,
		threshold REAL,
		condition TEXT,
		enabled BOOLEAN DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
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
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS device_deployment (
		deployment_id TEXT,
		device_id TEXT,
		status TEXT,
		progress INTEGER,
		message TEXT,
		started_at DATETIME,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (deployment_id, device_id),
		FOREIGN KEY (deployment_id) REFERENCES deployment(id),
		FOREIGN KEY (device_id) REFERENCES device(id)
	);

	`

	_, err = db.Exec(schema)
	require.NoError(t, err)

	// Insert test data
	seedTestData(t, db)

	return db
}

// setupBenchDatabase creates a database for benchmarks
func setupBenchDatabase(b *testing.B) *sql.DB {
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(b, err)

	// Use same schema as test database
	schema := `
	CREATE TABLE IF NOT EXISTS device (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		status TEXT DEFAULT 'offline',
		labels TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
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
			"INSERT INTO device (id, name, status) VALUES (?, ?, ?)",
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
			"INSERT INTO settings (key, value) VALUES (?, ?)",
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
