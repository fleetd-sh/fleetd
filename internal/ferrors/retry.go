package ferrors

import (
	"context"
	"math"
	"math/rand"
	"time"
)

// RetryConfig holds configuration for retry logic
type RetryConfig struct {
	// MaxAttempts is the maximum number of retry attempts
	MaxAttempts int
	// InitialDelay is the initial delay between retries
	InitialDelay time.Duration
	// MaxDelay is the maximum delay between retries
	MaxDelay time.Duration
	// Multiplier is the factor by which the delay increases
	Multiplier float64
	// Jitter adds randomization to delays to prevent thundering herd
	Jitter float64
	// RetryableFunc determines if an error is retryable
	RetryableFunc func(error) bool
	// OnRetry is called before each retry attempt
	OnRetry func(attempt int, err error, delay time.Duration)
}

// DefaultRetryConfig returns a default retry configuration
func DefaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxAttempts:   5,
		InitialDelay:  100 * time.Millisecond,
		MaxDelay:      30 * time.Second,
		Multiplier:    2.0,
		Jitter:        0.1,
		RetryableFunc: IsRetryable,
		OnRetry:       func(attempt int, err error, delay time.Duration) {},
	}
}

// Retry executes a function with retry logic
func Retry(ctx context.Context, config *RetryConfig, fn func() error) error {
	if config == nil {
		config = DefaultRetryConfig()
	}

	var lastErr error
	delay := config.InitialDelay

	for attempt := 0; attempt < config.MaxAttempts; attempt++ {
		if attempt > 0 {
			// Check context before retry
			select {
			case <-ctx.Done():
				return Wrap(ctx.Err(), ErrCodeTimeout, "retry cancelled")
			default:
			}

			// Apply jitter to delay
			jitteredDelay := ApplyJitter(delay, config.Jitter)

			// Call OnRetry callback
			if config.OnRetry != nil {
				config.OnRetry(attempt, lastErr, jitteredDelay)
			}

			// Sleep with context
			timer := time.NewTimer(jitteredDelay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return Wrap(ctx.Err(), ErrCodeTimeout, "retry cancelled during backoff")
			case <-timer.C:
			}

			// Calculate next delay with exponential backoff
			delay = time.Duration(float64(delay) * config.Multiplier)
			if delay > config.MaxDelay {
				delay = config.MaxDelay
			}
		}

		// Execute the function
		err := fn()
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if error is retryable
		if config.RetryableFunc != nil && !config.RetryableFunc(err) {
			return err
		}

		// If it's a FleetError with RetryAfter, respect it
		if fe, ok := err.(*FleetError); ok && fe.RetryAfter != nil {
			delay = *fe.RetryAfter
		}
	}

	// Wrap the last error with additional context
	return Wrapf(lastErr, ErrCodeResourceExhausted,
		"operation failed after %d attempts", config.MaxAttempts)
}

// RetryWithBackoff is a convenience function for common retry patterns
func RetryWithBackoff(ctx context.Context, fn func() error) error {
	return Retry(ctx, DefaultRetryConfig(), fn)
}

// RetryWithCustom allows inline configuration
func RetryWithCustom(ctx context.Context, maxAttempts int, delay time.Duration, fn func() error) error {
	config := &RetryConfig{
		MaxAttempts:   maxAttempts,
		InitialDelay:  delay,
		MaxDelay:      delay * 10,
		Multiplier:    2.0,
		Jitter:        0.1,
		RetryableFunc: IsRetryable,
	}
	return Retry(ctx, config, fn)
}

// ApplyJitter adds randomization to delay
func ApplyJitter(delay time.Duration, jitter float64) time.Duration {
	if jitter <= 0 {
		return delay
	}

	// Calculate jitter range
	jitterRange := float64(delay) * jitter
	// Random value between -jitterRange and +jitterRange
	jitterValue := (rand.Float64()*2 - 1) * jitterRange

	// Apply jitter
	newDelay := float64(delay) + jitterValue
	if newDelay < 0 {
		newDelay = 0
	}

	return time.Duration(newDelay)
}

// ExponentialBackoff calculates exponential backoff delay
func ExponentialBackoff(attempt int, initial time.Duration, max time.Duration, multiplier float64) time.Duration {
	if attempt <= 0 {
		return initial
	}

	delay := float64(initial) * math.Pow(multiplier, float64(attempt-1))
	if delay > float64(max) {
		return max
	}

	return time.Duration(delay)
}

// LinearBackoff calculates linear backoff delay
func LinearBackoff(attempt int, initial time.Duration, max time.Duration) time.Duration {
	delay := initial * time.Duration(attempt)
	if delay > max {
		return max
	}
	return delay
}

// RetryPolicy defines a reusable retry policy
type RetryPolicy struct {
	config         *RetryConfig
	circuitBreaker *CircuitBreaker
}

// NewRetryPolicy creates a new retry policy
func NewRetryPolicy(config *RetryConfig, cb *CircuitBreaker) *RetryPolicy {
	if config == nil {
		config = DefaultRetryConfig()
	}

	return &RetryPolicy{
		config:         config,
		circuitBreaker: cb,
	}
}

// Execute runs a function with the retry policy
func (p *RetryPolicy) Execute(ctx context.Context, fn func() error) error {
	// If circuit breaker is provided, wrap the function
	if p.circuitBreaker != nil {
		return Retry(ctx, p.config, func() error {
			return p.circuitBreaker.Execute(ctx, fn)
		})
	}

	return Retry(ctx, p.config, fn)
}

// AdaptiveRetry implements adaptive retry with backoff adjustment based on success rate
type AdaptiveRetry struct {
	config         *RetryConfig
	successCount   int64
	failureCount   int64
	lastAdjustment time.Time
	adjustInterval time.Duration
}

// NewAdaptiveRetry creates a new adaptive retry handler
func NewAdaptiveRetry(config *RetryConfig) *AdaptiveRetry {
	if config == nil {
		config = DefaultRetryConfig()
	}

	return &AdaptiveRetry{
		config:         config,
		adjustInterval: 1 * time.Minute,
		lastAdjustment: time.Now(),
	}
}

// Execute runs a function with adaptive retry
func (ar *AdaptiveRetry) Execute(ctx context.Context, fn func() error) error {
	// Adjust config based on success rate
	ar.maybeAdjust()

	err := Retry(ctx, ar.config, fn)

	// Track success/failure
	if err == nil {
		ar.successCount++
	} else {
		ar.failureCount++
	}

	return err
}

func (ar *AdaptiveRetry) maybeAdjust() {
	now := time.Now()
	if now.Sub(ar.lastAdjustment) < ar.adjustInterval {
		return
	}

	total := ar.successCount + ar.failureCount
	if total < 10 {
		return // Not enough data
	}

	successRate := float64(ar.successCount) / float64(total)

	// Adjust retry configuration based on success rate
	if successRate > 0.95 {
		// High success rate - reduce retries
		ar.config.MaxAttempts = max(2, ar.config.MaxAttempts-1)
		ar.config.InitialDelay = ar.config.InitialDelay * 2 / 3
	} else if successRate < 0.80 {
		// Low success rate - increase retries
		ar.config.MaxAttempts = min(10, ar.config.MaxAttempts+1)
		ar.config.InitialDelay = ar.config.InitialDelay * 3 / 2
	}

	// Reset counters
	ar.successCount = 0
	ar.failureCount = 0
	ar.lastAdjustment = now
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
