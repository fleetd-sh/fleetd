package database

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"fleetd.sh/internal/retry"
)

// RetryConfig holds configuration for database retry logic
type RetryConfig struct {
	MaxRetries     int
	InitialDelay   time.Duration
	MaxDelay       time.Duration
	BackoffFactor  float64
	ConnectTimeout time.Duration
}

// DefaultRetryConfig returns default retry configuration
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:     10,
		InitialDelay:   1 * time.Second,
		MaxDelay:       30 * time.Second,
		BackoffFactor:  2.0,
		ConnectTimeout: 5 * time.Second,
	}
}

// toRetryConfig converts database RetryConfig to unified retry.Config
func (c RetryConfig) toRetryConfig() retry.Config {
	return retry.Config{
		MaxAttempts:    c.MaxRetries,
		InitialBackoff: c.InitialDelay,
		MaxBackoff:     c.MaxDelay,
		Multiplier:     c.BackoffFactor,
		Jitter:         true,
	}
}

// OpenWithRetry opens a database connection with retry logic
func OpenWithRetry(ctx context.Context, driver, dsn string, config RetryConfig) (*sql.DB, error) {
	var db *sql.DB
	var attempt int

	err := retry.DoWithRetryable(ctx, config.toRetryConfig(), isRetryableError, func(ctx context.Context) error {
		attempt++
		slog.Info("Attempting database connection",
			"attempt", attempt,
			"max_attempts", config.MaxRetries,
			"driver", driver)

		// Try to open connection
		var err error
		db, err = sql.Open(driver, dsn)
		if err != nil {
			slog.Error("Failed to open database", "error", err, "attempt", attempt)
			return err
		}

		// Set connection pool settings
		db.SetMaxOpenConns(25)
		db.SetMaxIdleConns(10)
		db.SetConnMaxLifetime(5 * time.Minute)
		db.SetConnMaxIdleTime(10 * time.Minute)

		// Test the connection with timeout
		pingCtx, cancel := context.WithTimeout(ctx, config.ConnectTimeout)
		defer cancel()

		err = db.PingContext(pingCtx)
		if err != nil {
			db.Close()
			slog.Error("Database ping failed", "error", err, "attempt", attempt)
			return err
		}

		slog.Info("Database connection established", "attempt", attempt, "driver", driver)
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	return db, nil
}

// WaitForDatabase waits for database to be ready with health checks
func WaitForDatabase(ctx context.Context, db *sql.DB, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled while waiting for database")
		case <-ticker.C:
			if time.Now().After(deadline) {
				return fmt.Errorf("timeout waiting for database to be ready")
			}

			if err := db.Ping(); err == nil {
				return nil
			}
		}
	}
}

// ExecuteWithRetry executes a database operation with retry logic
func ExecuteWithRetry(ctx context.Context, db *sql.DB, operation func() error, config RetryConfig) error {
	var attempt int

	return retry.DoWithRetryable(ctx, config.toRetryConfig(), isRetryableError, func(ctx context.Context) error {
		attempt++
		err := operation()
		if err != nil {
			slog.Warn("Database operation failed, retrying",
				"error", err,
				"attempt", attempt,
				"max_attempts", config.MaxRetries)
		}
		return err
	})
}

// isRetryableError determines if an error should trigger a retry
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())
	retryablePatterns := []string{
		"connection refused",
		"connection reset",
		"broken pipe",
		"no such host",
		"timeout",
		"temporary failure",
		"too many connections",
		"database is locked",
		"deadlock",
	}

	for _, pattern := range retryablePatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}

	return false
}
