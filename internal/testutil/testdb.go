package testutil

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	_ "github.com/tursodatabase/libsql-client-go/libsql"
)

// TestDB wraps a database connection with test-specific functionality
type TestDB struct {
	DB   *sql.DB
	path string
	t    *testing.T
}

// NewTestDB creates a new test database and returns a TestDB instance.
func NewTestDB(t *testing.T) *TestDB {
	t.Helper()

	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	connStr := fmt.Sprintf("file:%s?_journal=WAL&_synchronous=NORMAL&_foreign_keys=on", dbPath)

	db, err := sql.Open("libsql", connStr)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		t.Fatalf("Failed to ping database: %v", err)
	}

	tdb := &TestDB{
		DB:   db,
		path: dbPath,
		t:    t,
	}

	t.Cleanup(func() {
		if err := tdb.Close(); err != nil {
			t.Errorf("Failed to close database: %v", err)
		}
	})

	return tdb
}

// Close closes the database connection
func (tdb *TestDB) Close() error {
	return tdb.DB.Close()
}

// GetDB returns the underlying *sql.DB
func (tdb *TestDB) GetDB() *sql.DB {
	return tdb.DB
}
