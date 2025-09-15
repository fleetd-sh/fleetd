package database

import (
	"context"
	"database/sql"
	"log/slog"
	"sync"
	"time"

	"fleetd.sh/internal/ferrors"
	_ "github.com/lib/pq"           // PostgreSQL driver
	_ "github.com/mattn/go-sqlite3" // SQLite driver
)

// Config holds database configuration
type Config struct {
	Driver          string               `json:"driver"`             // postgres, sqlite3
	DSN             string               `json:"dsn"`                // Data source name
	MaxOpenConns    int                  `json:"max_open_conns"`     // Maximum open connections
	MaxIdleConns    int                  `json:"max_idle_conns"`     // Maximum idle connections
	ConnMaxLifetime time.Duration        `json:"conn_max_lifetime"`  // Connection max lifetime
	ConnMaxIdleTime time.Duration        `json:"conn_max_idle_time"` // Connection max idle time
	QueryTimeout    time.Duration        `json:"query_timeout"`      // Default query timeout
	MigrationsPath  string               `json:"migrations_path"`    // Path to migration files
	RetryConfig     *ferrors.RetryConfig `json:"-"`                  // Retry configuration
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
		RetryConfig: &ferrors.RetryConfig{
			MaxAttempts:  3,
			InitialDelay: 100 * time.Millisecond,
			MaxDelay:     2 * time.Second,
			Multiplier:   2.0,
			RetryableFunc: func(err error) bool {
				// Retry on connection errors and deadlocks
				code := ferrors.GetCode(err)
				return code == ferrors.ErrCodeUnavailable ||
					code == ferrors.ErrCodeTimeout ||
					code == ferrors.ErrCodeResourceExhausted
			},
		},
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
	config         *Config
	logger         *slog.Logger
	metrics        *Metrics
	circuitBreaker *ferrors.CircuitBreaker
	errorHandler   *ferrors.ErrorHandler
	mu             sync.RWMutex
	closed         bool
}

// Metrics tracks database metrics
type Metrics struct {
	QueryCount      int64
	ErrorCount      int64
	ConnectionsOpen int
	LastError       error
	LastErrorTime   time.Time
}

// New creates a new database connection with enhanced error handling
func New(config *Config) (*DB, error) {
	if config == nil {
		return nil, ferrors.New(ferrors.ErrCodeInvalidInput, "database config is nil")
	}

	// Validate configuration
	if err := validateConfig(config); err != nil {
		return nil, err
	}

	// Configure circuit breaker
	cbConfig := &ferrors.CircuitBreakerConfig{
		MaxFailures: 5,
		MaxRequests: 1,
		Interval:    60 * time.Second,
		Timeout:     30 * time.Second,
		OnStateChange: func(from, to ferrors.CircuitBreakerState) {
			slog.Warn("Database circuit breaker state changed",
				"from", from.String(),
				"to", to.String(),
				"driver", config.Driver,
			)
		},
	}

	cb := ferrors.NewCircuitBreaker(cbConfig)

	// Configure error handler
	errorHandler := &ferrors.ErrorHandler{
		OnError: func(err *ferrors.FleetError) {
			slog.Error("Database error",
				"code", err.Code,
				"message", err.Message,
				"severity", err.Severity,
				"retryable", err.Retryable,
			)
		},
		OnPanic: func(recovered any, stack string) {
			slog.Error("Database panic",
				"recovered", recovered,
				"stack", stack,
			)
		},
	}

	db := &DB{
		config:         config,
		logger:         slog.Default().With("component", "database"),
		metrics:        &Metrics{},
		circuitBreaker: cb,
		errorHandler:   errorHandler,
	}

	// Connect with retry
	err := ferrors.Retry(context.Background(), config.RetryConfig, func() error {
		return db.connect()
	})

	if err != nil {
		return nil, ferrors.Wrapf(err, ferrors.ErrCodeUnavailable,
			"failed to connect to database")
	}

	// Run migrations
	if config.MigrationsPath != "" {
		if err := db.runMigrations(); err != nil {
			// Log but don't fail - migrations might already be applied
			db.logger.Warn("Failed to run migrations", "error", err)
		}
	}

	// Start health check
	go db.healthCheck()

	return db, nil
}

func (db *DB) connect() error {
	sqlDB, err := sql.Open(db.config.Driver, db.config.DSN)
	if err != nil {
		return ferrors.Wrapf(err, ferrors.ErrCodeUnavailable,
			"failed to open database connection")
	}

	// Configure connection pool
	sqlDB.SetMaxOpenConns(db.config.MaxOpenConns)
	sqlDB.SetMaxIdleConns(db.config.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(db.config.ConnMaxLifetime)
	sqlDB.SetConnMaxIdleTime(db.config.ConnMaxIdleTime)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := sqlDB.PingContext(ctx); err != nil {
		sqlDB.Close()
		return ferrors.Wrapf(err, ferrors.ErrCodeUnavailable,
			"failed to ping database")
	}

	db.DB = sqlDB
	db.logger.Info("Database connected",
		"driver", db.config.Driver,
		"max_conns", db.config.MaxOpenConns,
	)

	return nil
}

// QueryContext executes a query with enhanced error handling
func (db *DB) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	// Check if closed
	db.mu.RLock()
	if db.closed {
		db.mu.RUnlock()
		return nil, ferrors.New(ferrors.ErrCodeUnavailable, "database is closed")
	}
	db.mu.RUnlock()

	// Apply query timeout
	if db.config.QueryTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, db.config.QueryTimeout)
		defer cancel()
	}

	// Execute with circuit breaker
	var rows *sql.Rows
	err := db.circuitBreaker.Execute(ctx, func() error {
		var execErr error
		rows, execErr = db.DB.QueryContext(ctx, query, args...)
		if execErr != nil {
			return db.wrapSQLError(execErr, "query failed")
		}
		return nil
	})

	if err != nil {
		db.recordError(err)
		return nil, err
	}

	db.recordSuccess()
	return rows, nil
}

// ExecContext executes a statement with enhanced error handling
func (db *DB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	// Check if closed
	db.mu.RLock()
	if db.closed {
		db.mu.RUnlock()
		return nil, ferrors.New(ferrors.ErrCodeUnavailable, "database is closed")
	}
	db.mu.RUnlock()

	// Apply query timeout
	if db.config.QueryTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, db.config.QueryTimeout)
		defer cancel()
	}

	// Execute with circuit breaker and retry
	var result sql.Result
	err := db.executeWithRetry(ctx, func() error {
		var execErr error
		result, execErr = db.DB.ExecContext(ctx, query, args...)
		if execErr != nil {
			return db.wrapSQLError(execErr, "exec failed")
		}
		return nil
	})

	if err != nil {
		db.recordError(err)
		return nil, err
	}

	db.recordSuccess()
	return result, nil
}

// Transaction executes a function within a transaction
func (db *DB) Transaction(ctx context.Context, fn func(*Tx) error) error {
	// Start transaction
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	// Wrap in our transaction type
	wrappedTx := &Tx{
		Tx:     tx,
		db:     db,
		logger: db.logger.With("transaction", true),
	}

	// Execute function with panic recovery
	defer func() {
		if r := recover(); r != nil {
			db.logger.Error("Panic in transaction", "recovered", r)
			if rollbackErr := wrappedTx.Rollback(); rollbackErr != nil {
				db.logger.Error("Failed to rollback after panic", "error", rollbackErr)
			}
			panic(r) // Re-panic after cleanup
		}
	}()

	// Execute function
	if err := fn(wrappedTx); err != nil {
		if rollbackErr := wrappedTx.Rollback(); rollbackErr != nil {
			db.logger.Error("Failed to rollback transaction", "error", rollbackErr)
		}
		return err
	}

	// Commit transaction
	if err := wrappedTx.Commit(); err != nil {
		return ferrors.Wrapf(err, ferrors.ErrCodeInternal,
			"failed to commit transaction")
	}

	return nil
}

// BeginTx starts a new transaction
func (db *DB) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	// Check if closed
	db.mu.RLock()
	if db.closed {
		db.mu.RUnlock()
		return nil, ferrors.New(ferrors.ErrCodeUnavailable, "database is closed")
	}
	db.mu.RUnlock()

	tx, err := db.DB.BeginTx(ctx, opts)
	if err != nil {
		return nil, db.wrapSQLError(err, "failed to begin transaction")
	}

	return tx, nil
}

// Tx wraps sql.Tx with enhanced error handling
type Tx struct {
	*sql.Tx
	db       *DB
	logger   *slog.Logger
	finished bool
	mu       sync.Mutex
}

// QueryContext executes a query within the transaction
func (tx *Tx) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	tx.mu.Lock()
	if tx.finished {
		tx.mu.Unlock()
		return nil, ferrors.New(ferrors.ErrCodeInternal, "transaction already finished")
	}
	tx.mu.Unlock()

	rows, err := tx.Tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, tx.db.wrapSQLError(err, "transaction query failed")
	}

	return rows, nil
}

// ExecContext executes a statement within the transaction
func (tx *Tx) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	tx.mu.Lock()
	if tx.finished {
		tx.mu.Unlock()
		return nil, ferrors.New(ferrors.ErrCodeInternal, "transaction already finished")
	}
	tx.mu.Unlock()

	result, err := tx.Tx.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, tx.db.wrapSQLError(err, "transaction exec failed")
	}

	return result, nil
}

// Commit commits the transaction
func (tx *Tx) Commit() error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.finished {
		return ferrors.New(ferrors.ErrCodeInternal, "transaction already finished")
	}

	err := tx.Tx.Commit()
	tx.finished = true

	if err != nil {
		return tx.db.wrapSQLError(err, "transaction commit failed")
	}

	tx.logger.Debug("Transaction committed")
	return nil
}

// Rollback rolls back the transaction
func (tx *Tx) Rollback() error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.finished {
		return nil // Already finished
	}

	err := tx.Tx.Rollback()
	tx.finished = true

	if err != nil && err != sql.ErrTxDone {
		return tx.db.wrapSQLError(err, "transaction rollback failed")
	}

	tx.logger.Debug("Transaction rolled back")
	return nil
}

// Helper methods

func (db *DB) executeWithRetry(ctx context.Context, fn func() error) error {
	return ferrors.Retry(ctx, db.config.RetryConfig, func() error {
		return db.circuitBreaker.Execute(ctx, fn)
	})
}

func (db *DB) wrapSQLError(err error, message string) error {
	if err == nil {
		return nil
	}

	// Map SQL errors to our error codes
	switch err {
	case sql.ErrNoRows:
		return ferrors.Wrap(err, ferrors.ErrCodeNotFound, message)
	case sql.ErrConnDone:
		return ferrors.Wrap(err, ferrors.ErrCodeUnavailable, "connection closed")
	case sql.ErrTxDone:
		return ferrors.Wrap(err, ferrors.ErrCodeInternal, "transaction closed")
	case context.DeadlineExceeded:
		return ferrors.Wrap(err, ferrors.ErrCodeTimeout, "query timeout")
	case context.Canceled:
		return ferrors.Wrap(err, ferrors.ErrCodeInternal, "query cancelled")
	}

	// Check for specific database errors
	errStr := err.Error()

	// Deadlock detection
	if contains(errStr, "deadlock") {
		return ferrors.Wrap(err, ferrors.ErrCodeResourceExhausted, "deadlock detected").
			WithRetryAfter(100 * time.Millisecond)
	}

	// Connection errors
	if contains(errStr, "connection refused") ||
		contains(errStr, "connection reset") ||
		contains(errStr, "broken pipe") {
		return ferrors.Wrap(err, ferrors.ErrCodeUnavailable, "connection error").
			WithRetryAfter(1 * time.Second)
	}

	// Constraint violations
	if contains(errStr, "unique constraint") ||
		contains(errStr, "duplicate key") {
		return ferrors.Wrap(err, ferrors.ErrCodeAlreadyExists, message)
	}

	if contains(errStr, "foreign key constraint") {
		return ferrors.Wrap(err, ferrors.ErrCodePreconditionFailed, message)
	}

	// Default to internal error
	return ferrors.Wrap(err, ferrors.ErrCodeInternal, message)
}

func (db *DB) recordSuccess() {
	db.metrics.QueryCount++
}

func (db *DB) recordError(err error) {
	db.metrics.ErrorCount++
	db.metrics.LastError = err
	db.metrics.LastErrorTime = time.Now()
	db.errorHandler.Handle(err)
}

func (db *DB) healthCheck() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err := db.DB.PingContext(ctx)
			cancel()

			if err != nil {
				db.logger.Error("Database health check failed", "error", err)
				db.recordError(db.wrapSQLError(err, "health check failed"))
			} else {
				// Update metrics
				stats := db.DB.Stats()
				db.metrics.ConnectionsOpen = stats.OpenConnections
			}
		}
	}
}

func (db *DB) runMigrations() error {
	// TODO: Implement migration logic
	// This would use a migration library like golang-migrate
	db.logger.Info("Running database migrations",
		"path", db.config.MigrationsPath,
	)
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

	if db.DB != nil {
		if err := db.DB.Close(); err != nil {
			return ferrors.Wrap(err, ferrors.ErrCodeInternal,
				"failed to close database")
		}
	}

	db.logger.Info("Database connection closed")
	return nil
}

// GetMetrics returns current database metrics
func (db *DB) GetMetrics() Metrics {
	stats := db.DB.Stats()

	db.metrics.ConnectionsOpen = stats.OpenConnections

	return *db.metrics
}

// Helper functions

func validateConfig(config *Config) error {
	if config.Driver == "" {
		return ferrors.New(ferrors.ErrCodeInvalidInput, "database driver is required")
	}

	if config.DSN == "" {
		return ferrors.New(ferrors.ErrCodeInvalidInput, "database DSN is required")
	}

	switch config.Driver {
	case "postgres", "sqlite3", "mysql":
		// Supported drivers
	default:
		return ferrors.Newf(ferrors.ErrCodeInvalidInput,
			"unsupported database driver: %s", config.Driver)
	}

	if config.MaxOpenConns < 1 {
		config.MaxOpenConns = 1
	}

	if config.MaxIdleConns < 0 {
		config.MaxIdleConns = 0
	}

	if config.MaxIdleConns > config.MaxOpenConns {
		config.MaxIdleConns = config.MaxOpenConns
	}

	return nil
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 &&
		(s == substr || len(s) > len(substr) &&
			(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
				containsMiddle(s, substr)))
}

func containsMiddle(s, substr string) bool {
	for i := 1; i < len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
