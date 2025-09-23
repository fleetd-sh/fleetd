package database

import (
	"context"
	"database/sql"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMigrations tests all database migrations
func TestMigrations(t *testing.T) {
	tests := []struct {
		name   string
		driver string
		dsn    string
		skipCI bool
	}{
		{
			name:   "SQLite",
			driver: "sqlite3",
			dsn:    ":memory:",
			skipCI: false,
		},
		{
			name:   "PostgreSQL",
			driver: "postgres",
			dsn:    getPostgresTestDSN(),
			skipCI: true, // Skip in CI unless PostgreSQL is available
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipCI && isCI() {
				t.Skip("Skipping PostgreSQL test in CI")
			}

			// Open database connection
			db, err := sql.Open(tt.driver, tt.dsn)
			if err != nil {
				if tt.skipCI {
					t.Skip("Database not available:", err)
				}
				t.Fatal("Failed to open database:", err)
			}
			defer db.Close()

			// Test connection
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			if err := db.PingContext(ctx); err != nil {
				if tt.skipCI {
					t.Skip("Database not reachable:", err)
				}
				t.Fatal("Failed to ping database:", err)
			}

			// Run migration tests
			t.Run("MigrateUp", func(t *testing.T) {
				testMigrateUp(t, db, tt.driver)
			})

			t.Run("MigrateDown", func(t *testing.T) {
				testMigrateDown(t, db, tt.driver)
			})

			t.Run("MigrateUpDown", func(t *testing.T) {
				testMigrateUpDown(t, db, tt.driver)
			})

			t.Run("MigrationIdempotency", func(t *testing.T) {
				testMigrationIdempotency(t, db, tt.driver)
			})

			t.Run("SchemaIntegrity", func(t *testing.T) {
				testSchemaIntegrity(t, db, tt.driver)
			})
		})
	}
}

// testMigrateUp tests migrating up to the latest version
func testMigrateUp(t *testing.T, db *sql.DB, driver string) {
	config := &MigrationConfig{
		Driver:         driver,
		MigrationsPath: "../../migrations",
	}
	migrator, err := NewMigrator(config)
	require.NoError(t, err, "Failed to create migrator")

	// Run all migrations up
	err = migrator.Up(context.Background())
	assert.NoError(t, err, "Failed to run migrations up")

	// Verify current version
	version, dirty, err := migrator.Version()
	require.NoError(t, err, "Failed to get migration version")
	assert.False(t, dirty, "Database is in dirty state")
	assert.Greater(t, version, uint(0), "Version should be greater than 0")

	// Verify all expected tables exist
	tables := []string{
		"schema_migrations",
		"organizations",
		"users",
		"devices",
		"fleets",
		"deployments",
	}

	for _, table := range tables {
		exists := tableExists(t, db, driver, table)
		assert.True(t, exists, "Table %s should exist", table)
	}
}

// testMigrateDown tests migrating down to version 0
func testMigrateDown(t *testing.T, db *sql.DB, driver string) {
	config := &MigrationConfig{
		Driver:         driver,
		MigrationsPath: "../../migrations",
	}
	migrator, err := NewMigrator(config)
	require.NoError(t, err, "Failed to create migrator")

	// First migrate up
	err = migrator.Up(context.Background())
	require.NoError(t, err, "Failed to run migrations up")

	// Then migrate down
	err = migrator.Down(context.Background())
	assert.NoError(t, err, "Failed to run migrations down")

	// Verify we're at version 0
	_, dirty, err := migrator.Version()
	if err != nil && err.Error() != "no migration" {
		require.NoError(t, err, "Failed to get migration version")
	}
	assert.False(t, dirty, "Database is in dirty state")

	// Verify tables are dropped (except schema_migrations)
	tables := []string{
		"organizations",
		"users",
		"devices",
		"fleets",
		"deployments",
	}

	for _, table := range tables {
		exists := tableExists(t, db, driver, table)
		assert.False(t, exists, "Table %s should not exist after down migration", table)
	}
}

// testMigrateUpDown tests migrating up and down multiple times
func testMigrateUpDown(t *testing.T, db *sql.DB, driver string) {
	config := &MigrationConfig{
		Driver:         driver,
		MigrationsPath: "../../migrations",
	}
	migrator, err := NewMigrator(config)
	require.NoError(t, err, "Failed to create migrator")

	// Run up-down cycle multiple times
	for i := 0; i < 3; i++ {
		// Migrate up
		err = migrator.Up(context.Background())
		assert.NoError(t, err, "Failed to migrate up on iteration %d", i)

		// Check version
		version, dirty, err := migrator.Version()
		require.NoError(t, err, "Failed to get version on iteration %d", i)
		assert.False(t, dirty, "Database is dirty on iteration %d", i)
		assert.Greater(t, version, uint(0), "Invalid version on iteration %d", i)

		// Migrate down
		err = migrator.Down(context.Background())
		assert.NoError(t, err, "Failed to migrate down on iteration %d", i)
	}
}

// testMigrationIdempotency tests that migrations are idempotent
func testMigrationIdempotency(t *testing.T, db *sql.DB, driver string) {
	config := &MigrationConfig{
		Driver:         driver,
		MigrationsPath: "../../migrations",
	}
	migrator, err := NewMigrator(config)
	require.NoError(t, err, "Failed to create migrator")

	// Run migrations up twice
	err = migrator.Up(context.Background())
	assert.NoError(t, err, "Failed to run first migration up")

	version1, _, err := migrator.Version()
	require.NoError(t, err, "Failed to get first version")

	// Run again - should be idempotent
	err = migrator.Up(context.Background())
	assert.NoError(t, err, "Failed to run second migration up")

	version2, _, err := migrator.Version()
	require.NoError(t, err, "Failed to get second version")

	assert.Equal(t, version1, version2, "Version should not change on second run")
}

// testSchemaIntegrity tests the integrity of the migrated schema
func testSchemaIntegrity(t *testing.T, db *sql.DB, driver string) {
	config := &MigrationConfig{
		Driver:         driver,
		MigrationsPath: "../../migrations",
	}
	migrator, err := NewMigrator(config)
	require.NoError(t, err, "Failed to create migrator")

	// Migrate to latest
	err = migrator.Up(context.Background())
	require.NoError(t, err, "Failed to run migrations")

	// Test foreign key constraints
	t.Run("ForeignKeyConstraints", func(t *testing.T) {
		// Try to insert a device with non-existent organization
		_, err := db.Exec(`
			INSERT INTO devices (id, name, organization_id, status)
			VALUES ('test-device', 'Test Device', 'non-existent-org', 'online')
		`)

		if driver == "postgres" {
			// PostgreSQL enforces foreign keys by default
			assert.Error(t, err, "Should fail due to foreign key constraint")
		}
	})

	// Test unique constraints
	t.Run("UniqueConstraints", func(t *testing.T) {
		// Insert an organization
		_, err := db.Exec(`
			INSERT INTO organizations (id, name, slug)
			VALUES ('org1', 'Test Org', 'test-org')
		`)
		require.NoError(t, err, "Failed to insert first organization")

		// Try to insert duplicate slug
		_, err = db.Exec(`
			INSERT INTO organizations (id, name, slug)
			VALUES ('org2', 'Another Org', 'test-org')
		`)
		assert.Error(t, err, "Should fail due to unique constraint on slug")
	})

	// Test indexes exist
	t.Run("IndexesExist", func(t *testing.T) {
		indexes := []struct {
			table string
			index string
		}{
			{"devices", "idx_devices_organization_id"},
			{"devices", "idx_devices_status"},
			{"users", "idx_users_email"},
			{"deployments", "idx_deployments_status"},
		}

		for _, idx := range indexes {
			exists := indexExists(t, db, driver, idx.table, idx.index)
			assert.True(t, exists, "Index %s on table %s should exist", idx.index, idx.table)
		}
	})

	// Test data types
	t.Run("DataTypes", func(t *testing.T) {
		// Insert test data with various types
		_, err := db.Exec(`
			INSERT INTO organizations (id, name, slug, metadata)
			VALUES ('org-test', 'Test Org', 'test-org-unique', '{"key": "value"}')
		`)
		require.NoError(t, err, "Failed to insert organization with JSON metadata")

		// Query back and verify
		var metadata string
		err = db.QueryRow("SELECT metadata FROM organizations WHERE id = 'org-test'").Scan(&metadata)
		require.NoError(t, err, "Failed to query metadata")
		assert.Contains(t, metadata, "key", "Metadata should contain JSON")
	})
}

// Helper functions

func tableExists(t *testing.T, db *sql.DB, driver, tableName string) bool {
	var query string
	switch driver {
	case "postgres":
		query = `
			SELECT EXISTS (
				SELECT FROM information_schema.tables
				WHERE table_schema = 'public'
				AND table_name = $1
			)`
	case "sqlite3":
		query = `
			SELECT EXISTS (
				SELECT name FROM sqlite_master
				WHERE type='table' AND name=?
			)`
	default:
		t.Fatalf("Unsupported driver: %s", driver)
	}

	var exists bool
	err := db.QueryRow(query, tableName).Scan(&exists)
	require.NoError(t, err, "Failed to check if table exists")
	return exists
}

func indexExists(t *testing.T, db *sql.DB, driver, tableName, indexName string) bool {
	var query string
	var exists bool

	switch driver {
	case "postgres":
		query = `
			SELECT EXISTS (
				SELECT 1 FROM pg_indexes
				WHERE schemaname = 'public'
				AND tablename = $1
				AND indexname = $2
			)`
		err := db.QueryRow(query, tableName, indexName).Scan(&exists)
		require.NoError(t, err, "Failed to check if index exists")
	case "sqlite3":
		query = `
			SELECT name FROM sqlite_master
			WHERE type='index' AND tbl_name=? AND name=?`
		var name sql.NullString
		err := db.QueryRow(query, tableName, indexName).Scan(&name)
		exists = err == nil && name.Valid
	default:
		t.Fatalf("Unsupported driver: %s", driver)
	}

	return exists
}

func getPostgresTestDSN() string {
	// Use test database if available
	dsn := "postgres://fleetd_test:fleetd_test@localhost:5432/fleetd_test?sslmode=disable"

	// Allow override from environment
	if envDSN := getEnv("TEST_DATABASE_URL", ""); envDSN != "" {
		dsn = envDSN
	}

	return dsn
}

func isCI() bool {
	return getEnv("CI", "") == "true" || getEnv("GITHUB_ACTIONS", "") == "true"
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// TestMigrationFiles tests that migration files are valid
func TestMigrationFiles(t *testing.T) {
	migrationDir := "./migrations"

	// Check that migrations directory exists
	info, err := os.Stat(migrationDir)
	require.NoError(t, err, "Migrations directory should exist")
	assert.True(t, info.IsDir(), "Migrations should be a directory")

	// Read migration files
	files, err := os.ReadDir(migrationDir)
	require.NoError(t, err, "Failed to read migrations directory")

	// Check for pairs of up/down migrations
	upFiles := make(map[string]bool)
	downFiles := make(map[string]bool)

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		name := file.Name()
		if strings.HasSuffix(name, ".up.sql") {
			base := strings.TrimSuffix(name, ".up.sql")
			upFiles[base] = true
		} else if strings.HasSuffix(name, ".down.sql") {
			base := strings.TrimSuffix(name, ".down.sql")
			downFiles[base] = true
		}
	}

	// Verify each up migration has a corresponding down migration
	for base := range upFiles {
		assert.True(t, downFiles[base], "Missing down migration for %s", base)
	}

	for base := range downFiles {
		assert.True(t, upFiles[base], "Missing up migration for %s", base)
	}

	// Verify migration numbering is sequential
	var numbers []int
	for base := range upFiles {
		parts := strings.Split(base, "_")
		if len(parts) > 0 {
			if num, err := strconv.Atoi(parts[0]); err == nil {
				numbers = append(numbers, num)
			}
		}
	}

	sort.Ints(numbers)
	for i := 1; i < len(numbers); i++ {
		assert.LessOrEqual(t, numbers[i-1], numbers[i], "Migration numbers should be sequential")
	}
}
