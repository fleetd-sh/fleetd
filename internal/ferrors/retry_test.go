package ferrors

import (
	"context"
	stderrors "errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestRetryBasic(t *testing.T) {
	attempts := 0
	config := &RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
		RetryableFunc: func(err error) bool {
			return true
		},
	}

	ctx := context.Background()
	err := Retry(ctx, config, func() error {
		attempts++
		if attempts < 3 {
			return stderrors.New("temporary failure")
		}
		return nil
	})

	if err != nil {
		t.Errorf("expected successful retry, got error: %v", err)
	}

	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestRetryMaxAttempts(t *testing.T) {
	attempts := 0
	config := &RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 1 * time.Millisecond,
		RetryableFunc: func(err error) bool {
			return true
		},
	}

	ctx := context.Background()
	err := Retry(ctx, config, func() error {
		attempts++
		return stderrors.New("permanent failure")
	})

	if err == nil {
		t.Error("expected error after max attempts")
	}

	if attempts != 3 {
		t.Errorf("expected exactly 3 attempts, got %d", attempts)
	}

	// Check error wrapping
	var fleetErr *FleetError
	if As(err, &fleetErr) {
		if fleetErr.Code != ErrCodeResourceExhausted {
			t.Errorf("expected error code RESOURCE_EXHAUSTED, got %s", fleetErr.Code)
		}
	}
}

func TestRetryExponentialBackoff(t *testing.T) {
	var delays []time.Duration
	config := &RetryConfig{
		MaxAttempts:  4,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
		Jitter:       0, // No jitter for predictable testing
		OnRetry: func(attempt int, err error, delay time.Duration) {
			delays = append(delays, delay)
		},
		RetryableFunc: func(err error) bool {
			return true
		},
	}

	ctx := context.Background()
	attempts := 0
	Retry(ctx, config, func() error {
		attempts++
		if attempts < 4 {
			return stderrors.New("retry me")
		}
		return nil
	})

	// Verify exponential backoff
	expectedDelays := []time.Duration{
		10 * time.Millisecond, // Initial
		20 * time.Millisecond, // Initial * 2
		40 * time.Millisecond, // Initial * 4
	}

	if len(delays) != 3 {
		t.Fatalf("expected 3 retry delays, got %d", len(delays))
	}

	for i, expected := range expectedDelays {
		if delays[i] != expected {
			t.Errorf("delay %d: expected %v, got %v", i, expected, delays[i])
		}
	}
}

func TestRetryWithJitter(t *testing.T) {
	config := &RetryConfig{
		MaxAttempts:  5,
		InitialDelay: 100 * time.Millisecond,
		Jitter:       0.5,
		RetryableFunc: func(err error) bool {
			return true
		},
	}

	ctx := context.Background()
	var delays []time.Duration

	config.OnRetry = func(attempt int, err error, delay time.Duration) {
		delays = append(delays, delay)
	}

	attempts := 0
	Retry(ctx, config, func() error {
		attempts++
		if attempts < 5 {
			return stderrors.New("retry")
		}
		return nil
	})

	// Verify jitter is applied (delays should vary)
	if len(delays) < 2 {
		t.Fatal("expected at least 2 delays")
	}

	// With jitter, not all delays should be exactly the same or follow exact pattern
	allSame := true
	for i := 1; i < len(delays); i++ {
		if delays[i] != delays[0] {
			allSame = false
			break
		}
	}

	if allSame && len(delays) > 1 {
		t.Error("expected delays to vary with jitter")
	}
}

func TestRetryContextCancellation(t *testing.T) {
	config := &RetryConfig{
		MaxAttempts:  10,
		InitialDelay: 100 * time.Millisecond,
		RetryableFunc: func(err error) bool {
			return true
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	attempts := 0
	err := Retry(ctx, config, func() error {
		attempts++
		return stderrors.New("retry")
	})

	if err == nil {
		t.Error("expected error from context cancellation")
	}

	// Should have attempted at least once but not all 10 times
	if attempts == 0 {
		t.Error("expected at least one attempt")
	}
	if attempts >= 10 {
		t.Error("expected context to cancel before max attempts")
	}

	// Check for timeout error
	var fleetErr *FleetError
	if As(err, &fleetErr) {
		if fleetErr.Code != ErrCodeTimeout {
			t.Errorf("expected timeout error code, got %s", fleetErr.Code)
		}
	}
}

func TestRetryNonRetryableError(t *testing.T) {
	attempts := 0
	config := &RetryConfig{
		MaxAttempts: 5,
		RetryableFunc: func(err error) bool {
			var fleetErr *FleetError
			if As(err, &fleetErr) {
				return fleetErr.Retryable
			}
			return false
		},
	}

	ctx := context.Background()
	nonRetryableErr := New(ErrCodeInvalidInput, "bad input")
	nonRetryableErr.Retryable = false

	err := Retry(ctx, config, func() error {
		attempts++
		return nonRetryableErr
	})

	if err == nil {
		t.Error("expected error to be returned")
	}

	if attempts != 1 {
		t.Errorf("expected 1 attempt for non-retryable error, got %d", attempts)
	}
}

func TestRetryWithRetryAfter(t *testing.T) {
	config := &RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 10 * time.Millisecond,
		RetryableFunc: func(err error) bool {
			return true
		},
	}

	ctx := context.Background()
	retryAfter := 50 * time.Millisecond
	attempts := 0

	start := time.Now()
	err := Retry(ctx, config, func() error {
		attempts++
		if attempts < 3 {
			fleetErr := New(ErrCodeRateLimited, "rate limited")
			fleetErr.RetryAfter = &retryAfter
			return fleetErr
		}
		return nil
	})

	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("expected successful retry, got: %v", err)
	}

	// Should have respected RetryAfter delay
	expectedMinDelay := retryAfter * 2 // Two retries with RetryAfter
	if elapsed < expectedMinDelay {
		t.Errorf("expected delay of at least %v, got %v", expectedMinDelay, elapsed)
	}
}

func TestRetryWithBackoff(t *testing.T) {
	attempts := 0
	ctx := context.Background()

	err := RetryWithBackoff(ctx, func() error {
		attempts++
		if attempts < 3 {
			return stderrors.New("retry")
		}
		return nil
	})

	if err != nil {
		t.Errorf("expected success, got: %v", err)
	}

	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestRetryWithCustom(t *testing.T) {
	attempts := 0
	ctx := context.Background()

	err := RetryWithCustom(ctx, 2, 5*time.Millisecond, func() error {
		attempts++
		if attempts < 2 {
			return stderrors.New("retry")
		}
		return nil
	})

	if err != nil {
		t.Errorf("expected success, got: %v", err)
	}

	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}

func TestRetryPolicy(t *testing.T) {
	config := &RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 1 * time.Millisecond,
		RetryableFunc: func(err error) bool {
			return true
		},
	}

	// Test without circuit breaker
	policy := NewRetryPolicy(config, nil)
	attempts := 0

	ctx := context.Background()
	err := policy.Execute(ctx, func() error {
		attempts++
		if attempts < 2 {
			return stderrors.New("retry")
		}
		return nil
	})

	if err != nil {
		t.Errorf("expected success, got: %v", err)
	}

	// Test with circuit breaker
	cb := NewCircuitBreaker(nil)
	policyWithCB := NewRetryPolicy(config, cb)

	attempts = 0
	err = policyWithCB.Execute(ctx, func() error {
		attempts++
		return nil
	})

	if err != nil {
		t.Errorf("expected success with circuit breaker, got: %v", err)
	}
}

func TestAdaptiveRetry(t *testing.T) {
	config := &RetryConfig{
		MaxAttempts:  5,
		InitialDelay: 10 * time.Millisecond,
		RetryableFunc: func(err error) bool {
			return true
		},
	}

	ar := NewAdaptiveRetry(config)
	ctx := context.Background()

	// Simulate high success rate
	for i := 0; i < 20; i++ {
		ar.Execute(ctx, func() error {
			return nil
		})
	}

	// Force adjustment
	ar.lastAdjustment = time.Now().Add(-2 * time.Minute)
	ar.maybeAdjust()

	// Max attempts should be reduced for high success rate
	if ar.config.MaxAttempts >= 5 {
		t.Error("expected MaxAttempts to be reduced for high success rate")
	}

	// Simulate low success rate
	ar.successCount = 2
	ar.failureCount = 18
	ar.maybeAdjust()

	// Max attempts should be increased for low success rate
	if ar.config.MaxAttempts <= 2 {
		t.Error("expected MaxAttempts to be increased for low success rate")
	}
}

func TestExponentialBackoffCalculation(t *testing.T) {
	tests := []struct {
		attempt  int
		initial  time.Duration
		max      time.Duration
		multi    float64
		expected time.Duration
	}{
		{1, 100 * time.Millisecond, 10 * time.Second, 2.0, 100 * time.Millisecond},
		{2, 100 * time.Millisecond, 10 * time.Second, 2.0, 200 * time.Millisecond},
		{3, 100 * time.Millisecond, 10 * time.Second, 2.0, 400 * time.Millisecond},
		{10, 100 * time.Millisecond, 1 * time.Second, 2.0, 1 * time.Second}, // Capped at max
	}

	for _, tt := range tests {
		result := ExponentialBackoff(tt.attempt, tt.initial, tt.max, tt.multi)
		if result != tt.expected {
			t.Errorf("attempt %d: expected %v, got %v", tt.attempt, tt.expected, result)
		}
	}
}

func TestLinearBackoff(t *testing.T) {
	tests := []struct {
		attempt  int
		initial  time.Duration
		max      time.Duration
		expected time.Duration
	}{
		{1, 100 * time.Millisecond, 10 * time.Second, 100 * time.Millisecond},
		{2, 100 * time.Millisecond, 10 * time.Second, 200 * time.Millisecond},
		{3, 100 * time.Millisecond, 10 * time.Second, 300 * time.Millisecond},
		{200, 100 * time.Millisecond, 10 * time.Second, 10 * time.Second}, // Capped at max
	}

	for _, tt := range tests {
		result := LinearBackoff(tt.attempt, tt.initial, tt.max)
		if result != tt.expected {
			t.Errorf("attempt %d: expected %v, got %v", tt.attempt, tt.expected, result)
		}
	}
}

func TestApplyJitter(t *testing.T) {
	delay := 1000 * time.Millisecond
	jitter := 0.1

	// Test multiple times to ensure randomness
	results := make(map[time.Duration]bool)
	for i := 0; i < 100; i++ {
		result := ApplyJitter(delay, jitter)
		results[result] = true

		// Result should be within jitter range
		minDelay := time.Duration(float64(delay) * (1 - jitter))
		maxDelay := time.Duration(float64(delay) * (1 + jitter))

		if result < minDelay || result > maxDelay {
			t.Errorf("jittered delay %v outside expected range [%v, %v]",
				result, minDelay, maxDelay)
		}
	}

	// Should have different values due to randomness
	if len(results) < 10 {
		t.Error("expected more variation in jittered delays")
	}

	// Test with zero jitter
	noJitter := ApplyJitter(delay, 0)
	if noJitter != delay {
		t.Errorf("expected no change with zero jitter, got %v", noJitter)
	}

	// Test with negative jitter (should be treated as zero)
	negJitter := ApplyJitter(delay, -0.1)
	if negJitter != delay {
		t.Errorf("expected no change with negative jitter, got %v", negJitter)
	}
}

func TestRetryConcurrency(t *testing.T) {
	config := &RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 1 * time.Millisecond,
		RetryableFunc: func(err error) bool {
			return true
		},
	}

	ctx := context.Background()
	var totalAttempts atomic.Int32

	// Run multiple concurrent retries
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			localAttempts := 0
			Retry(ctx, config, func() error {
				localAttempts++
				totalAttempts.Add(1)
				if localAttempts < 2 {
					return stderrors.New("retry")
				}
				return nil
			})
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Each goroutine should have made 2 attempts
	expected := int32(20)
	if totalAttempts.Load() != expected {
		t.Errorf("expected %d total attempts, got %d", expected, totalAttempts.Load())
	}
}

// Benchmark tests
func BenchmarkRetrySuccess(b *testing.B) {
	config := &RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 1 * time.Microsecond,
		RetryableFunc: func(err error) bool {
			return true
		},
	}

	ctx := context.Background()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		Retry(ctx, config, func() error {
			return nil
		})
	}
}

func BenchmarkRetryWithBackoff(b *testing.B) {
	ctx := context.Background()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		RetryWithBackoff(ctx, func() error {
			return nil
		})
	}
}

func BenchmarkApplyJitter(b *testing.B) {
	delay := 100 * time.Millisecond
	jitter := 0.1

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ApplyJitter(delay, jitter)
	}
}
