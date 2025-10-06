package database

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/database/sqlite3"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// MigrationConfig holds migration configuration
type MigrationConfig struct {
	Driver         string
	DSN            string
	MigrationsPath string
	TableName      string
	Logger         *slog.Logger
}

// DefaultMigrationConfig returns default migration configuration
func DefaultMigrationConfig() *MigrationConfig {
	return &MigrationConfig{
		TableName: "schema_migrations",
		Logger:    slog.Default().With("component", "migrations"),
	}
}

// Migrator handles database migrations
type Migrator struct {
	config  *MigrationConfig
	migrate *migrate.Migrate
	logger  *slog.Logger
}

// NewMigrator creates a new migrator
func NewMigrator(config *MigrationConfig) (*Migrator, error) {
	if config == nil {
		config = DefaultMigrationConfig()
	}

	if config.Logger == nil {
		config.Logger = slog.Default().With("component", "migrations")
	}

	return &Migrator{
		config: config,
		logger: config.Logger,
	}, nil
}

// Initialize initializes the migrator with database connection
//
// NOTE: The migrator does NOT take ownership of the database connection.
// The caller remains responsible for closing the connection when appropriate.
// Calling migrator.Close() will close the connection, so only do that if
// you want to close everything.
func (m *Migrator) Initialize(db *sql.DB, driver string) error {
	// Create source from embedded filesystem
	source, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("failed to create migration source: %w", err)
	}

	// Create database driver
	var dbDriver database.Driver
	switch driver {
	case "postgres":
		dbDriver, err = postgres.WithInstance(db, &postgres.Config{
			MigrationsTable: m.config.TableName,
		})
	case "sqlite3":
		dbDriver, err = sqlite3.WithInstance(db, &sqlite3.Config{
			MigrationsTable: m.config.TableName,
		})
	default:
		return fmt.Errorf("unsupported database driver: %s", driver)
	}

	if err != nil {
		return fmt.Errorf("failed to create database driver: %w", err)
	}

	// Create migrator
	m.migrate, err = migrate.NewWithInstance("iofs", source, driver, dbDriver)
	if err != nil {
		return fmt.Errorf("failed to create migrator: %w", err)
	}

	return nil
}

// Up runs all pending migrations
func (m *Migrator) Up(ctx context.Context) error {
	if m.migrate == nil {
		return errors.New("migrator not initialized")
	}

	// Get current version
	version, dirty, err := m.migrate.Version()
	if err != nil && err != migrate.ErrNilVersion {
		return fmt.Errorf("failed to get migration version: %w", err)
	}

	if dirty {
		m.logger.Warn("Database is in dirty state, marking as clean to continue",
			"version", version)

		// When dirty, Force() just marks the current version as clean without running migrations
		// This allows Up() to determine if there's anything left to do
		if err := m.migrate.Force(int(version)); err != nil {
			return fmt.Errorf("failed to clean dirty state: %w", err)
		}
	}

	m.logger.Info("Running migrations", "current_version", version)

	// Run migrations
	startTime := time.Now()
	err = m.migrate.Up()
	if err != nil {
		if err == migrate.ErrNoChange {
			m.logger.Info("No migrations to run")
			return nil
		}
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	// Get new version
	newVersion, _, _ := m.migrate.Version()
	duration := time.Since(startTime)

	m.logger.Info("Migrations completed",
		"from_version", version,
		"to_version", newVersion,
		"duration", duration,
	)

	return nil
}

// Down rolls back one migration
func (m *Migrator) Down(ctx context.Context) error {
	if m.migrate == nil {
		return errors.New("migrator not initialized")
	}

	// Get current version
	version, dirty, err := m.migrate.Version()
	if err != nil {
		return fmt.Errorf("failed to get migration version: %w", err)
	}

	if dirty {
		return fmt.Errorf("cannot rollback: database is in dirty state at version %d", version)
	}

	m.logger.Info("Rolling back migration", "current_version", version)

	// Rollback one migration
	startTime := time.Now()
	if err := m.migrate.Steps(-1); err != nil {
		return fmt.Errorf("failed to rollback migration: %w", err)
	}

	// Get new version
	newVersion, _, _ := m.migrate.Version()
	duration := time.Since(startTime)

	m.logger.Info("Migration rolled back",
		"from_version", version,
		"to_version", newVersion,
		"duration", duration,
	)

	return nil
}

// Migrate runs migrations to a specific version
func (m *Migrator) Migrate(ctx context.Context, targetVersion uint) error {
	if m.migrate == nil {
		return errors.New("migrator not initialized")
	}

	// Get current version
	currentVersion, dirty, err := m.migrate.Version()
	if err != nil && err != migrate.ErrNilVersion {
		return fmt.Errorf("failed to get migration version: %w", err)
	}

	if dirty {
		return fmt.Errorf("cannot migrate: database is in dirty state at version %d", currentVersion)
	}

	m.logger.Info("Migrating to version",
		"current_version", currentVersion,
		"target_version", targetVersion,
	)

	// Migrate to target version
	startTime := time.Now()
	if err := m.migrate.Migrate(targetVersion); err != nil {
		if err == migrate.ErrNoChange {
			m.logger.Info("Already at target version")
			return nil
		}
		return fmt.Errorf("failed to migrate to version: %w", err)
	}

	duration := time.Since(startTime)

	m.logger.Info("Migration completed",
		"from_version", currentVersion,
		"to_version", targetVersion,
		"duration", duration,
	)

	return nil
}

// Reset drops all tables and reruns all migrations
func (m *Migrator) Reset(ctx context.Context) error {
	if m.migrate == nil {
		return errors.New("migrator not initialized")
	}

	m.logger.Warn("Resetting database - all data will be lost")

	// Drop all tables
	if err := m.migrate.Drop(); err != nil {
		return fmt.Errorf("failed to drop database: %w", err)
	}

	// Run all migrations
	if err := m.Up(ctx); err != nil {
		return fmt.Errorf("failed to run migrations after reset: %w", err)
	}

	m.logger.Info("Database reset completed")
	return nil
}

// Version returns the current migration version
func (m *Migrator) Version() (uint, bool, error) {
	if m.migrate == nil {
		return 0, false, errors.New("migrator not initialized")
	}

	version, dirty, err := m.migrate.Version()
	if err != nil {
		if err == migrate.ErrNilVersion {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("failed to get migration version: %w", err)
	}

	return version, dirty, nil
}

// Force sets the migration version without running migrations
func (m *Migrator) Force(version int) error {
	if m.migrate == nil {
		return errors.New("migrator not initialized")
	}

	m.logger.Warn("Forcing migration version", "version", version)

	if err := m.migrate.Force(version); err != nil {
		return fmt.Errorf("failed to force migration version: %w", err)
	}

	m.logger.Info("Migration version forced", "version", version)
	return nil
}

// Close closes the migrator and its underlying database connection
//
// WARNING: This will close the database connection passed to Initialize().
// Only call this when you're completely done with both migrations AND
// the database connection. In most cases, you should manage the database
// connection lifecycle separately and not call this method.
func (m *Migrator) Close() error {
	if m.migrate != nil {
		sourceErr, dbErr := m.migrate.Close()
		if sourceErr != nil {
			return fmt.Errorf("failed to close migration source: %w", sourceErr)
		}
		if dbErr != nil {
			return fmt.Errorf("failed to close migration database: %w", dbErr)
		}
	}
	return nil
}

// RunMigrations is a convenience function to run migrations
//
// IMPORTANT: This function does NOT close the database connection.
// The caller is responsible for managing the database connection lifecycle.
// The migrator.Close() method is intentionally not called here because it
// would close the underlying database connection, which should remain open
// for the application to use after migrations are complete.
func RunMigrations(ctx context.Context, db *sql.DB, driver string) error {
	migrator, err := NewMigrator(nil)
	if err != nil {
		return err
	}

	// IMPORTANT: Do NOT defer migrator.Close() here!
	// Closing the migrator would close the database connection,
	// but the caller expects to continue using the connection.

	if err := migrator.Initialize(db, driver); err != nil {
		return err
	}

	return migrator.Up(ctx)
}

// RunMigrationsAndClose runs migrations and closes everything
//
// This function runs migrations and then closes both the migrator and
// the database connection. Use this only when you want to run migrations
// as a one-off operation and don't need the database connection afterwards.
func RunMigrationsAndClose(ctx context.Context, db *sql.DB, driver string) error {
	migrator, err := NewMigrator(nil)
	if err != nil {
		return err
	}
	defer migrator.Close() // This WILL close the database connection

	if err := migrator.Initialize(db, driver); err != nil {
		return err
	}

	return migrator.Up(ctx)
}

// MigrationStatus represents the status of migrations
type MigrationStatus struct {
	CurrentVersion uint      `json:"current_version"`
	Dirty          bool      `json:"dirty"`
	Applied        []Applied `json:"applied"`
	Pending        []Pending `json:"pending"`
}

// Applied represents an applied migration
type Applied struct {
	Version   uint      `json:"version"`
	Name      string    `json:"name"`
	AppliedAt time.Time `json:"applied_at"`
}

// Pending represents a pending migration
type Pending struct {
	Version uint   `json:"version"`
	Name    string `json:"name"`
}

// GetStatus returns the current migration status
func (m *Migrator) GetStatus(ctx context.Context) (*MigrationStatus, error) {
	version, dirty, err := m.Version()
	if err != nil {
		return nil, err
	}

	status := &MigrationStatus{
		CurrentVersion: version,
		Dirty:          dirty,
		Applied:        []Applied{},
		Pending:        []Pending{},
	}

	// Get current migration version and status from migrate tool
	if m.migrate != nil {
		version, dirty, err := m.migrate.Version()
		if err != nil && err != migrate.ErrNilVersion {
			return nil, fmt.Errorf("failed to get migration version: %w", err)
		}
		status.CurrentVersion = version
		status.Dirty = dirty
	}

	return status, nil
}

// Validate checks if migrations are valid
func (m *Migrator) Validate() error {
	// Validation is handled by the migrate library
	// Basic check: ensure migrations can be loaded
	if m.migrate == nil {
		return fmt.Errorf("migrator not initialized")
	}

	// The migrate library validates migrations during initialization
	// Additional validation can be added here if needed
	return nil
}
