package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	_ "github.com/lib/pq"  // PostgreSQL driver
	_ "modernc.org/sqlite" // SQLite driver
)

// Config holds database configuration
type Config struct {
	Driver          string        `json:"driver"`             // postgres, sqlite3
	DSN             string        `json:"dsn"`                // Data source name
	MaxOpenConns    int           `json:"max_open_conns"`     // Maximum open connections
	MaxIdleConns    int           `json:"max_idle_conns"`     // Maximum idle connections
	ConnMaxLifetime time.Duration `json:"conn_max_lifetime"`  // Connection max lifetime
	ConnMaxIdleTime time.Duration `json:"conn_max_idle_time"` // Connection max idle time
	QueryTimeout    time.Duration `json:"query_timeout"`      // Default query timeout
	MigrationsPath  string        `json:"migrations_path"`    // Path to migration files
}

// DefaultConfig returns default database configuration
func DefaultConfig(driver string) *Config {
	config := &Config{
		Driver:          driver,
		MaxOpenConns:    25,
		MaxIdleConns:    5,
		ConnMaxLifetime: 5 * time.Minute,
		ConnMaxIdleTime: 1 * time.Minute,
		QueryTimeout:    30 * time.Second,
		MigrationsPath:  "/internal/migrations",
	}

	// Adjust for driver specifics
	switch driver {
	case "sqlite3":
		config.MaxOpenConns = 1 // SQLite doesn't handle concurrency well
		config.MaxIdleConns = 1
	case "postgres":
		config.MaxOpenConns = 50
		config.MaxIdleConns = 10
	}

	return config
}

// DB wraps sql.DB with enhanced error handling and monitoring
type DB struct {
	*sql.DB
	config       *Config
	logger       *slog.Logger
	metrics      *Metrics
	mu           sync.RWMutex
	closed       bool
	healthCancel context.CancelFunc // Cancel function for health check goroutine
}

// Metrics tracks database metrics
type Metrics struct {
	QueryCount      int64
	ErrorCount      int64
	ConnectionsOpen int
	LastError       error
	LastErrorTime   time.Time
}

// New creates a new database connection
func New(config *Config) (*DB, error) {
	if config == nil {
		return nil, errors.New("database config is nil")
	}

	// Validate configuration
	if err := validateConfig(config); err != nil {
		return nil, err
	}

	db := &DB{
		config:  config,
		logger:  slog.Default(),
		metrics: &Metrics{},
	}

	// Connect to database
	if err := db.connect(); err != nil {
		return nil, err
	}

	// Run migrations
	if config.MigrationsPath != "" {
		if err := db.runMigrations(); err != nil {
			if db.DB != nil {
				db.DB.Close()
			}
			return nil, fmt.Errorf("failed to run database migrations: %w", err)
		}
	}

	// Start health check
	healthCtx, healthCancel := context.WithCancel(context.Background())
	db.healthCancel = healthCancel
	go db.healthCheck(healthCtx)

	return db, nil
}

func (db *DB) connect() error {
	sqlDB, err := sql.Open(db.config.Driver, db.config.DSN)
	if err != nil {
		return fmt.Errorf("failed to open database connection: %w", err)
	}

	// Set connection pool configuration
	if db.config.MaxOpenConns > 0 {
		sqlDB.SetMaxOpenConns(db.config.MaxOpenConns)
	}
	if db.config.MaxIdleConns > 0 {
		sqlDB.SetMaxIdleConns(db.config.MaxIdleConns)
	}
	if db.config.ConnMaxLifetime > 0 {
		sqlDB.SetConnMaxLifetime(db.config.ConnMaxLifetime)
	}
	if db.config.ConnMaxIdleTime > 0 {
		sqlDB.SetConnMaxIdleTime(db.config.ConnMaxIdleTime)
	}

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		sqlDB.Close()
		return fmt.Errorf("failed to ping database: %w", err)
	}

	db.DB = sqlDB
	db.logger.Info("Database connection established", "driver", db.config.Driver)
	return nil
}

// Close closes the database connection
func (db *DB) Close() error {
	db.mu.Lock()
	defer db.mu.Unlock()

	if db.closed {
		return nil
	}
	db.closed = true

	// Cancel health check goroutine
	if db.healthCancel != nil {
		db.healthCancel()
	}

	if db.DB != nil {
		if err := db.DB.Close(); err != nil {
			return fmt.Errorf("failed to close database: %w", err)
		}
	}

	db.logger.Info("Database connection closed")
	return nil
}

// GetMetrics returns current database metrics
func (db *DB) GetMetrics() Metrics {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return *db.metrics
}

// Helper functions

func validateConfig(config *Config) error {
	if config.Driver == "" {
		return errors.New("database driver is required")
	}

	if config.DSN == "" {
		return errors.New("database DSN is required")
	}

	switch config.Driver {
	case "postgres", "sqlite3", "mysql":
		// Supported drivers
	default:
		return errors.New("unsupported database driver")
	}

	if config.MaxOpenConns < 1 {
		config.MaxOpenConns = 1
	}

	if config.MaxIdleConns < 0 {
		config.MaxIdleConns = 0
	}

	return nil
}

func (db *DB) healthCheck(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := db.Ping(); err != nil {
				db.logger.Error("Database health check failed", "error", err)
				db.mu.Lock()
				db.metrics.LastError = err
				db.metrics.LastErrorTime = time.Now()
				db.mu.Unlock()
			}
		}
	}
}

func (db *DB) runMigrations() error {
	// Simplified migration - just log for now
	db.logger.Info("Migrations would run here", "path", db.config.MigrationsPath)
	return nil
}
