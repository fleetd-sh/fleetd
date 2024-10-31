package migrations

import (
	"database/sql"
	"embed"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite3"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed queries/*.sql
var Migrations embed.FS

func MigrateUp(d *sql.DB) (version int, dirty bool, err error) {
	source, err := iofs.New(Migrations, "queries")
	if err != nil {
		return -1, false, fmt.Errorf("failed to create source driver: %w", err)
	}

	driver, err := sqlite3.WithInstance(d, &sqlite3.Config{})
	if err != nil {
		return -1, false, fmt.Errorf("failed to create sqlite driver: %w", err)
	}

	if _, err := d.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return -1, false, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	m, err := migrate.NewWithInstance(
		"iofs",
		source,
		"sqlite3",
		driver,
	)
	if err != nil {
		return -1, false, fmt.Errorf("failed to create migrator: %w", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return -1, false, fmt.Errorf("failed to run migrations: %w", err)
	}

	version, dirty, err = driver.Version()
	if err != nil {
		// Return 0 since migrations have been applied
		return 0, false, fmt.Errorf("failed to get version: %w", err)
	}

	return version, dirty, nil
}

func MigrateDown(d *sql.DB) (version int, dirty bool, err error) {
	source, err := iofs.New(Migrations, "queries")
	if err != nil {
		return -1, false, fmt.Errorf("failed to create source driver: %w", err)
	}

	driver, err := sqlite3.WithInstance(d, &sqlite3.Config{})
	if err != nil {
		return -1, false, fmt.Errorf("failed to create sqlite driver: %w", err)
	}

	m, err := migrate.NewWithInstance(
		"iofs",
		source,
		"sqlite3",
		driver,
	)
	if err != nil {
		return -1, false, fmt.Errorf("failed to create migrator: %w", err)
	}

	if err := m.Down(); err != nil && err != migrate.ErrNoChange {
		return -1, false, fmt.Errorf("failed to run migrations: %w", err)
	}

	version, dirty, err = driver.Version()
	if err != nil {
		// Return 0 since migrations have been rolled back
		return 0, false, fmt.Errorf("failed to get version: %w", err)
	}

	return version, dirty, nil
}
