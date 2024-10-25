package migrations

import (
	"database/sql"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite3"
	_ "github.com/golang-migrate/migrate/v4/source/file"

	"fleetd.sh/db"
)

func MigrateUp(d *sql.DB) error {
	driver, err := sqlite3.WithInstance(d, &sqlite3.Config{})
	if err != nil {
		return err
	}

	m, err := migrate.NewWithInstance(
		"embed",
		db.Migrations,
		"libsql",
		driver,
	)
	if err != nil {
		return err
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}

	return nil
}

func MigrateDown(d *sql.DB) error {
	driver, err := sqlite3.WithInstance(d, &sqlite3.Config{})
	if err != nil {
		return err
	}

	m, err := migrate.NewWithInstance(
		"embed",
		db.Migrations,
		"libsql",
		driver,
	)
	if err != nil {
		return err
	}

	if err := m.Down(); err != nil && err != migrate.ErrNoChange {
		return err
	}

	return nil
}
