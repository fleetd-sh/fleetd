package ferrors

import (
	"context"
	stderrors "errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCircuitBreakerStates(t *testing.T) {
	config := &CircuitBreakerConfig{
		MaxFailures: 3,
		MaxRequests: 1,
		Interval:    1 * time.Second,
		Timeout:     500 * time.Millisecond,
		ShouldTrip: func(err error) bool {
			return true // All errors trip the breaker
		},
	}

	cb := NewCircuitBreaker(config)

	// Initial state should be closed
	if cb.GetState() != StateClosed {
		t.Errorf("expected initial state to be CLOSED, got %s", cb.GetState())
	}

	// Simulate failures to open the circuit
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		err := cb.Execute(ctx, func() error {
			return stderrors.New("test failure")
		})
		if err == nil {
			t.Error("expected error from failing function")
		}
	}

	// Circuit should now be open
	if cb.GetState() != StateOpen {
		t.Errorf("expected state to be OPEN after failures, got %s", cb.GetState())
	}

	// Attempts should fail immediately when open
	err := cb.Execute(ctx, func() error {
		t.Error("function should not be called when circuit is open")
		return nil
	})

	if err == nil {
		t.Error("expected error when circuit is open")
	}

	var fleetErr *FleetError
	if As(err, &fleetErr) {
		if fleetErr.Code != ErrCodeUnavailable {
			t.Errorf("expected error code UNAVAILABLE, got %s", fleetErr.Code)
		}
	}

	// Wait for timeout to transition to half-open
	time.Sleep(600 * time.Millisecond)

	// Circuit should now be half-open
	if cb.GetState() != StateHalfOpen {
		t.Errorf("expected state to be HALF_OPEN after timeout, got %s", cb.GetState())
	}

	// Successful request should close the circuit
	err = cb.Execute(ctx, func() error {
		return nil
	})

	if err != nil {
		t.Errorf("expected no error in half-open state, got %v", err)
	}

	// Circuit should now be closed again
	if cb.GetState() != StateClosed {
		t.Errorf("expected state to be CLOSED after success, got %s", cb.GetState())
	}
}

func TestCircuitBreakerWithFallback(t *testing.T) {
	cb := NewCircuitBreaker(nil) // Use default config

	ctx := context.Background()
	fallbackCalled := false

	// Force circuit to open
	for i := 0; i < 6; i++ {
		cb.Execute(ctx, func() error {
			return stderrors.New("failure")
		})
	}

	// Execute with fallback when circuit is open
	err := cb.ExecuteWithFallback(ctx,
		func() error {
			t.Error("primary function should not be called")
			return stderrors.New("should not happen")
		},
		func() error {
			fallbackCalled = true
			return nil
		},
	)

	if err != nil {
		t.Errorf("expected no error with fallback, got %v", err)
	}

	if !fallbackCalled {
		t.Error("expected fallback to be called")
	}
}

func TestCircuitBreakerMetrics(t *testing.T) {
	cb := NewCircuitBreaker(nil)
	ctx := context.Background()

	// Execute some successful requests
	for i := 0; i < 3; i++ {
		cb.Execute(ctx, func() error {
			return nil
		})
	}

	// Execute some failures
	for i := 0; i < 2; i++ {
		cb.Execute(ctx, func() error {
			return stderrors.New("failure")
		})
	}

	metrics := cb.GetMetrics()

	if metrics["success_count"].(uint64) != 3 {
		t.Errorf("expected 3 successes, got %d", metrics["success_count"])
	}

	if metrics["failure_count"].(uint64) != 2 {
		t.Errorf("expected 2 failures, got %d", metrics["failure_count"])
	}

	if metrics["state"] != "CLOSED" {
		t.Errorf("expected state CLOSED, got %s", metrics["state"])
	}
}

func TestCircuitBreakerShouldTrip(t *testing.T) {
	tripCount := 0
	config := &CircuitBreakerConfig{
		MaxFailures: 3,
		Interval:    1 * time.Second, // Set interval to prevent resetting failures
		Timeout:     1 * time.Second,
		ShouldTrip: func(err error) bool {
			// Only trip on specific errors
			var fleetErr *FleetError
			if As(err, &fleetErr) {
				shouldTrip := fleetErr.Code == ErrCodeTimeout
				if shouldTrip {
					tripCount++
					t.Logf("ShouldTrip called %d times, error code: %s, returning true", tripCount, fleetErr.Code)
				}
				return shouldTrip
			}
			return false
		},
	}

	cb := NewCircuitBreaker(config)
	ctx := context.Background()

	// Non-tripping errors should not open circuit
	for i := 0; i < 5; i++ {
		cb.Execute(ctx, func() error {
			return New(ErrCodeInvalidInput, "bad input")
		})
	}

	if cb.GetState() != StateClosed {
		t.Errorf("expected circuit to remain CLOSED, got %s", cb.GetState())
	}

	// Tripping errors should open circuit
	for i := 0; i < 3; i++ {
		err := cb.Execute(ctx, func() error {
			return New(ErrCodeTimeout, "timeout")
		})
		if err == nil {
			t.Errorf("expected error on attempt %d", i+1)
		}
	}

	state := cb.GetState()
	if state != StateOpen {
		metrics := cb.GetMetrics()
		t.Errorf("expected circuit to be OPEN after 3 failures, got %s (failures: %v)", state, metrics["failures"])
	}
}

func TestCircuitBreakerReset(t *testing.T) {
	cb := NewCircuitBreaker(nil)
	ctx := context.Background()

	// Open the circuit
	for i := 0; i < 6; i++ {
		cb.Execute(ctx, func() error {
			return stderrors.New("failure")
		})
	}

	if cb.GetState() != StateOpen {
		t.Errorf("expected circuit to be OPEN, got %s", cb.GetState())
	}

	// Reset the circuit
	cb.Reset()

	if cb.GetState() != StateClosed {
		t.Errorf("expected circuit to be CLOSED after reset, got %s", cb.GetState())
	}

	// Should be able to execute again
	err := cb.Execute(ctx, func() error {
		return nil
	})

	if err != nil {
		t.Errorf("expected no error after reset, got %v", err)
	}
}

func TestCircuitBreakerConcurrency(t *testing.T) {
	config := &CircuitBreakerConfig{
		MaxFailures: 10,
		MaxRequests: 5,
		Interval:    1 * time.Second,
		Timeout:     1 * time.Second,
	}

	cb := NewCircuitBreaker(config)
	ctx := context.Background()

	var wg sync.WaitGroup
	var successCount atomic.Int32
	var failureCount atomic.Int32

	// Run concurrent requests
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			err := cb.Execute(ctx, func() error {
				if id%3 == 0 {
					return stderrors.New("failure")
				}
				return nil
			})

			if err != nil {
				failureCount.Add(1)
			} else {
				successCount.Add(1)
			}
		}(i)
	}

	wg.Wait()

	// Verify counts are reasonable
	total := successCount.Load() + failureCount.Load()
	if total != 50 {
		t.Errorf("expected 50 total requests, got %d", total)
	}
}

func TestCircuitBreakerGroup(t *testing.T) {
	group := NewCircuitBreakerGroup(nil)
	ctx := context.Background()

	// Get circuit breakers for different services
	cb1 := group.Get("service1")
	cb2 := group.Get("service2")

	// Verify they are different instances
	if cb1 == cb2 {
		t.Error("expected different circuit breakers for different services")
	}

	// Verify same instance is returned for same service
	cb1Again := group.Get("service1")
	if cb1 != cb1Again {
		t.Error("expected same circuit breaker instance for same service")
	}

	// Test execution through group
	err := group.Execute(ctx, "service3", func() error {
		return nil
	})

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	// Get metrics for all breakers
	metrics := group.GetMetrics()
	if len(metrics) < 3 {
		t.Errorf("expected at least 3 circuit breakers, got %d", len(metrics))
	}
}

func TestCircuitBreakerStateTransitions(t *testing.T) {
	var stateChanges []string
	var mu sync.Mutex

	config := &CircuitBreakerConfig{
		MaxFailures: 2,
		MaxRequests: 1,
		Interval:    100 * time.Millisecond,
		Timeout:     100 * time.Millisecond,
		OnStateChange: func(from, to CircuitBreakerState) {
			mu.Lock()
			stateChanges = append(stateChanges,
				from.String()+"->"+to.String())
			mu.Unlock()
		},
	}

	cb := NewCircuitBreaker(config)
	ctx := context.Background()

	// Trigger state changes
	for i := 0; i < 2; i++ {
		cb.Execute(ctx, func() error {
			return stderrors.New("failure")
		})
	}

	// Should transition from CLOSED to OPEN
	time.Sleep(10 * time.Millisecond)

	// Wait for timeout to half-open
	time.Sleep(150 * time.Millisecond)

	// Trigger transition by attempting execution
	cb.Execute(ctx, func() error {
		return nil
	})

	// Verify state changes were recorded
	mu.Lock()
	defer mu.Unlock()

	if len(stateChanges) < 1 {
		t.Error("expected at least one state change")
	}

	// Should have CLOSED->OPEN transition
	found := false
	for _, change := range stateChanges {
		if change == "CLOSED->OPEN" {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("expected CLOSED->OPEN transition, got %v", stateChanges)
	}
}

func TestCircuitBreakerMaxRequests(t *testing.T) {
	config := &CircuitBreakerConfig{
		MaxFailures: 3,
		MaxRequests: 2,               // Allow 2 requests in half-open
		Interval:    1 * time.Second, // Set interval to prevent resetting failures
		Timeout:     100 * time.Millisecond,
	}

	cb := NewCircuitBreaker(config)
	ctx := context.Background()

	// Open the circuit
	for i := 0; i < 3; i++ {
		cb.Execute(ctx, func() error {
			return stderrors.New("failure")
		})
	}

	// Wait for half-open
	time.Sleep(150 * time.Millisecond)

	// First two requests should be allowed
	var allowed atomic.Int32
	var rejected atomic.Int32

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := cb.Execute(ctx, func() error {
				allowed.Add(1)
				time.Sleep(50 * time.Millisecond) // Hold the request
				return nil
			})
			if err != nil {
				rejected.Add(1)
			}
		}()
	}

	wg.Wait()

	// Only MaxRequests should be allowed
	if allowed.Load() > int32(config.MaxRequests) {
		t.Errorf("expected at most %d requests, got %d",
			config.MaxRequests, allowed.Load())
	}
}

// Benchmark tests
func BenchmarkCircuitBreakerClosed(b *testing.B) {
	cb := NewCircuitBreaker(nil)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cb.Execute(ctx, func() error {
			return nil
		})
	}
}

func BenchmarkCircuitBreakerOpen(b *testing.B) {
	cb := NewCircuitBreaker(nil)
	ctx := context.Background()

	// Open the circuit
	for i := 0; i < 10; i++ {
		cb.Execute(ctx, func() error {
			return stderrors.New("failure")
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cb.Execute(ctx, func() error {
			return nil
		})
	}
}

func BenchmarkCircuitBreakerGetState(b *testing.B) {
	cb := NewCircuitBreaker(nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cb.GetState()
	}
}

func BenchmarkCircuitBreakerGroup(b *testing.B) {
	group := NewCircuitBreakerGroup(nil)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		group.Execute(ctx, "service", func() error {
			return nil
		})
	}
}
