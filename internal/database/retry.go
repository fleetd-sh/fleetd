package database

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"
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

// OpenWithRetry opens a database connection with retry logic
func OpenWithRetry(ctx context.Context, driver, dsn string, config RetryConfig) (*sql.DB, error) {
	delay := config.InitialDelay

	for attempt := 1; attempt <= config.MaxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("context cancelled while connecting to database")
		default:
		}

		slog.Info("Attempting database connection",
			"attempt", attempt,
			"max_attempts", config.MaxRetries,
			"driver", driver)

		// Try to open connection
		db, err := sql.Open(driver, dsn)
		if err != nil {
			slog.Error("Failed to open database",
				"error", err,
				"attempt", attempt)
			if attempt < config.MaxRetries {
				time.Sleep(delay)
				delay = calculateBackoff(delay, config.MaxDelay, config.BackoffFactor)
				continue
			}
			return nil, fmt.Errorf("failed to open database after %d attempts: %w", config.MaxRetries, err)
		}

		// Set connection pool settings
		db.SetMaxOpenConns(25)
		db.SetMaxIdleConns(10)
		db.SetConnMaxLifetime(5 * time.Minute)
		db.SetConnMaxIdleTime(10 * time.Minute)

		// Test the connection with timeout
		pingCtx, cancel := context.WithTimeout(ctx, config.ConnectTimeout)
		err = db.PingContext(pingCtx)
		cancel()

		if err != nil {
			db.Close()
			slog.Error("Database ping failed",
				"error", err,
				"attempt", attempt)
			if attempt < config.MaxRetries {
				time.Sleep(delay)
				delay = calculateBackoff(delay, config.MaxDelay, config.BackoffFactor)
				continue
			}
			return nil, fmt.Errorf("database ping failed after %d attempts: %w", config.MaxRetries, err)
		}

		slog.Info("Database connection established",
			"attempt", attempt,
			"driver", driver)
		return db, nil
	}

	return nil, fmt.Errorf("failed to connect to database after %d attempts", config.MaxRetries)
}

// calculateBackoff calculates the next delay with exponential backoff
func calculateBackoff(current, max time.Duration, factor float64) time.Duration {
	next := time.Duration(float64(current) * factor)
	if next > max {
		return max
	}
	return next
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
	delay := config.InitialDelay

	for attempt := 1; attempt <= config.MaxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled during database operation")
		default:
		}

		err := operation()
		if err == nil {
			return nil
		}

		// Check if error is retryable
		if !isRetryableError(err) {
			return err
		}

		slog.Warn("Database operation failed, retrying",
			"error", err,
			"attempt", attempt,
			"max_attempts", config.MaxRetries)

		if attempt < config.MaxRetries {
			time.Sleep(delay)
			delay = calculateBackoff(delay, config.MaxDelay, config.BackoffFactor)
			continue
		}

		return fmt.Errorf("database operation failed after %d attempts: %w", config.MaxRetries, err)
	}

	return nil
}

// isRetryableError determines if an error should trigger a retry
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Common retryable database errors
	errStr := err.Error()
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
		if containsIgnoreCase(errStr, pattern) {
			return true
		}
	}

	return false
}

// containsIgnoreCase checks if string contains substring case-insensitively
func containsIgnoreCase(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr ||
			containsStr(toLowerCase(s), toLowerCase(substr)))
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && indexOfStr(s, substr) >= 0
}

func indexOfStr(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func toLowerCase(s string) string {
	result := make([]byte, len(s))
	for i, b := range []byte(s) {
		if b >= 'A' && b <= 'Z' {
			result[i] = b + 32
		} else {
			result[i] = b
		}
	}
	return string(result)
}
