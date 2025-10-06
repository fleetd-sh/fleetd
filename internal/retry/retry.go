package retry

import (
	"context"
	"errors"
	"math/rand"
	"time"

	"connectrpc.com/connect"
)

// Config holds retry configuration
type Config struct {
	MaxAttempts    int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	Multiplier     float64
	Jitter         bool
}

// DefaultConfig returns sensible defaults
func DefaultConfig() Config {
	return Config{
		MaxAttempts:    3,
		InitialBackoff: 100 * time.Millisecond,
		MaxBackoff:     10 * time.Second,
		Multiplier:     2.0,
		Jitter:         true,
	}
}

// DatabaseConfig returns config optimized for database operations
func DatabaseConfig() Config {
	return Config{
		MaxAttempts:    5,
		InitialBackoff: 500 * time.Millisecond,
		MaxBackoff:     30 * time.Second,
		Multiplier:     2.0,
		Jitter:         true,
	}
}

// RPCConfig returns config optimized for RPC operations
func RPCConfig() Config {
	return Config{
		MaxAttempts:    3,
		InitialBackoff: 1 * time.Second,
		MaxBackoff:     10 * time.Second,
		Multiplier:     2.0,
		Jitter:         true,
	}
}

// IsRetryable checks if an error should be retried
type IsRetryable func(error) bool

// DefaultRetryable retries on temporary and timeout errors
func DefaultRetryable(err error) bool {
	if err == nil {
		return false
	}

	// Check for temporary errors
	type temporary interface {
		Temporary() bool
	}
	if te, ok := err.(temporary); ok && te.Temporary() {
		return true
	}

	// Check for timeout errors
	type timeout interface {
		Timeout() bool
	}
	if te, ok := err.(timeout); ok && te.Timeout() {
		return true
	}

	return false
}

// ConnectRetryable retries on specific Connect RPC error codes
func ConnectRetryable(err error) bool {
	if err == nil {
		return false
	}

	var connectErr *connect.Error
	if errors.As(err, &connectErr) {
		switch connectErr.Code() {
		case connect.CodeUnavailable,
			connect.CodeDeadlineExceeded,
			connect.CodeResourceExhausted,
			connect.CodeAborted:
			return true
		}
	}

	return DefaultRetryable(err)
}

// Do executes a function with retry logic
func Do(ctx context.Context, config Config, fn func(context.Context) error) error {
	return DoWithRetryable(ctx, config, DefaultRetryable, fn)
}

// DoWithRetryable executes a function with retry logic and custom retryability check
func DoWithRetryable(ctx context.Context, config Config, isRetryable IsRetryable, fn func(context.Context) error) error {
	var lastErr error
	backoff := config.InitialBackoff
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	for attempt := 1; attempt <= config.MaxAttempts; attempt++ {
		// Check context before attempt
		if err := ctx.Err(); err != nil {
			if lastErr != nil {
				return lastErr
			}
			return err
		}

		// Execute function
		err := fn(ctx)
		if err == nil {
			return nil
		}

		lastErr = err

		// Don't retry if not retryable or last attempt
		if !isRetryable(err) || attempt >= config.MaxAttempts {
			return err
		}

		// Calculate backoff with optional jitter
		delay := backoff
		if config.Jitter {
			// Add ±25% jitter
			jitter := time.Duration(float64(backoff) * 0.25 * (2*rng.Float64() - 1))
			delay = backoff + jitter
		}

		// Wait before retry
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return ctx.Err()
		}

		// Update backoff for next iteration
		backoff = time.Duration(float64(backoff) * config.Multiplier)
		if backoff > config.MaxBackoff {
			backoff = config.MaxBackoff
		}
	}

	return lastErr
}

// Backoff provides exponential backoff with jitter
type Backoff struct {
	config  Config
	attempt int
	rng     *rand.Rand
}

// NewBackoff creates a new Backoff
func NewBackoff(config Config) *Backoff {
	return &Backoff{
		config: config,
		rng:    rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Next returns the next backoff duration
func (b *Backoff) Next() time.Duration {
	b.attempt++

	if b.attempt > b.config.MaxAttempts {
		return 0
	}

	// Calculate exponential backoff
	backoff := b.config.InitialBackoff
	for i := 1; i < b.attempt; i++ {
		backoff = time.Duration(float64(backoff) * b.config.Multiplier)
		if backoff > b.config.MaxBackoff {
			backoff = b.config.MaxBackoff
			break
		}
	}

	// Add jitter if enabled
	if b.config.Jitter {
		jitter := time.Duration(float64(backoff) * 0.25 * (2*b.rng.Float64() - 1))
		backoff = backoff + jitter
	}

	return backoff
}

// Reset resets the backoff to initial state
func (b *Backoff) Reset() {
	b.attempt = 0
}

// Attempt returns the current attempt number
func (b *Backoff) Attempt() int {
	return b.attempt
}
