package testutil

import (
	"database/sql"
	"os"
	"path/filepath"

	_ "github.com/tursodatabase/libsql-client-go/libsql"
)

// NewDBTemp creates a new SQLite database in a temporary file and returns the database connection.
// It's the caller's responsibility to close the database and remove the temporary file.
func NewDBTemp() (*sql.DB, func(), error) {
	tempDir, err := os.MkdirTemp("", "fleettest")
	if err != nil {
		return nil, nil, err
	}

	dbPath := filepath.Join(tempDir, "test.db")
	db, err := sql.Open("libsql", "file:"+dbPath)
	if err != nil {
		os.RemoveAll(tempDir)
		return nil, nil, err
	}

	cleanup := func() {
		db.Close()
		os.RemoveAll(tempDir)
	}

	return db, cleanup, nil
}
