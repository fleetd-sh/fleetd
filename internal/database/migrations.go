package database

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log/slog"
	"time"

	"fleetd.sh/internal/ferrors"
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
func (m *Migrator) Initialize(db *sql.DB, driver string) error {
	// Create source from embedded filesystem
	source, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to create migration source")
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
		return ferrors.Newf(ferrors.ErrCodeInvalidInput, "unsupported database driver: %s", driver)
	}

	if err != nil {
		return ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to create database driver")
	}

	// Create migrator
	m.migrate, err = migrate.NewWithInstance("iofs", source, driver, dbDriver)
	if err != nil {
		return ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to create migrator")
	}

	return nil
}

// Up runs all pending migrations
func (m *Migrator) Up(ctx context.Context) error {
	if m.migrate == nil {
		return ferrors.New(ferrors.ErrCodeInternal, "migrator not initialized")
	}

	// Get current version
	version, dirty, err := m.migrate.Version()
	if err != nil && err != migrate.ErrNilVersion {
		return ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to get migration version")
	}

	if dirty {
		m.logger.Warn("Database is in dirty state, attempting to fix",
			"version", version)

		// Force version to clean state
		if err := m.migrate.Force(int(version)); err != nil {
			return ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to force migration version")
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
		return ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to run migrations")
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
		return ferrors.New(ferrors.ErrCodeInternal, "migrator not initialized")
	}

	// Get current version
	version, dirty, err := m.migrate.Version()
	if err != nil {
		return ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to get migration version")
	}

	if dirty {
		return ferrors.Newf(ferrors.ErrCodePreconditionFailed,
			"cannot rollback: database is in dirty state at version %d", version)
	}

	m.logger.Info("Rolling back migration", "current_version", version)

	// Rollback one migration
	startTime := time.Now()
	if err := m.migrate.Steps(-1); err != nil {
		return ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to rollback migration")
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
		return ferrors.New(ferrors.ErrCodeInternal, "migrator not initialized")
	}

	// Get current version
	currentVersion, dirty, err := m.migrate.Version()
	if err != nil && err != migrate.ErrNilVersion {
		return ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to get migration version")
	}

	if dirty {
		return ferrors.Newf(ferrors.ErrCodePreconditionFailed,
			"cannot migrate: database is in dirty state at version %d", currentVersion)
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
		return ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to migrate to version")
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
		return ferrors.New(ferrors.ErrCodeInternal, "migrator not initialized")
	}

	m.logger.Warn("Resetting database - all data will be lost")

	// Drop all tables
	if err := m.migrate.Drop(); err != nil {
		return ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to drop database")
	}

	// Run all migrations
	if err := m.Up(ctx); err != nil {
		return ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to run migrations after reset")
	}

	m.logger.Info("Database reset completed")
	return nil
}

// Version returns the current migration version
func (m *Migrator) Version() (uint, bool, error) {
	if m.migrate == nil {
		return 0, false, ferrors.New(ferrors.ErrCodeInternal, "migrator not initialized")
	}

	version, dirty, err := m.migrate.Version()
	if err != nil {
		if err == migrate.ErrNilVersion {
			return 0, false, nil
		}
		return 0, false, ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to get migration version")
	}

	return version, dirty, nil
}

// Force sets the migration version without running migrations
func (m *Migrator) Force(version int) error {
	if m.migrate == nil {
		return ferrors.New(ferrors.ErrCodeInternal, "migrator not initialized")
	}

	m.logger.Warn("Forcing migration version", "version", version)

	if err := m.migrate.Force(version); err != nil {
		return ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to force migration version")
	}

	m.logger.Info("Migration version forced", "version", version)
	return nil
}

// Close closes the migrator
func (m *Migrator) Close() error {
	if m.migrate != nil {
		sourceErr, dbErr := m.migrate.Close()
		if sourceErr != nil {
			return ferrors.Wrap(sourceErr, ferrors.ErrCodeInternal, "failed to close migration source")
		}
		if dbErr != nil {
			return ferrors.Wrap(dbErr, ferrors.ErrCodeInternal, "failed to close migration database")
		}
	}
	return nil
}

// RunMigrations is a convenience function to run migrations
func RunMigrations(ctx context.Context, db *sql.DB, driver string) error {
	migrator, err := NewMigrator(nil)
	if err != nil {
		return err
	}
	defer migrator.Close()

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
