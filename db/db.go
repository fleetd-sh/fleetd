// Package db provides utilities and migrations for the platform database.
package db

import (
	"database/sql"
	"embed"

	"github.com/golang-migrate/migrate/v4/source"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	_ "github.com/tursodatabase/libsql-client-go/libsql"
)

//go:embed migrations/*.sql
var migrations embed.FS
var Migrations source.Driver

func init() {
	var err error
	Migrations, err = iofs.New(migrations, "migrations")
	if err != nil {
		panic(err)
	}
}

func New(dbURL string) (*sql.DB, error) {
	return sql.Open("libsql", dbURL)
}
