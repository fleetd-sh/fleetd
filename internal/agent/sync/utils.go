package sync

import (
	"fmt"
	"math"
	"math/rand"
	"sync"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
)

// Backoff implements exponential backoff with jitter
type Backoff struct {
	mu         sync.Mutex
	initial    time.Duration
	max        time.Duration
	multiplier float64
	attempt    int
	rand       *rand.Rand
}

// NewBackoff creates a new backoff calculator
func NewBackoff(initial, max time.Duration, multiplier float64) *Backoff {
	return &Backoff{
		initial:    initial,
		max:        max,
		multiplier: multiplier,
		rand:       rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Next returns the next backoff duration
func (b *Backoff) Next() time.Duration {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.attempt++

	// Calculate exponential backoff
	backoff := float64(b.initial) * math.Pow(b.multiplier, float64(b.attempt-1))

	// Cap at maximum
	if backoff > float64(b.max) {
		backoff = float64(b.max)
	}

	// Add jitter (±25%)
	jitter := backoff * 0.25 * (2*b.rand.Float64() - 1)
	backoff += jitter

	// Ensure minimum duration
	if backoff < float64(b.initial) {
		backoff = float64(b.initial)
	}

	return time.Duration(backoff)
}

// Reset resets the backoff counter
func (b *Backoff) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.attempt = 0
}

// Attempt returns the current attempt number
func (b *Backoff) Attempt() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.attempt
}

// timestampToProto converts time.Time to protobuf timestamp
func timestampToProto(t time.Time) *timestamppb.Timestamp {
	return timestamppb.New(t)
}

// protoToTimestamp converts protobuf timestamp to time.Time
func protoToTimestamp(t *timestamppb.Timestamp) time.Time {
	if t == nil {
		return time.Time{}
	}
	return t.AsTime()
}

// RateLimiter implements token bucket rate limiting
type RateLimiter struct {
	tokens    float64
	capacity  float64
	rate      float64
	lastCheck time.Time
	mu        sync.Mutex
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(rate float64, capacity float64) *RateLimiter {
	return &RateLimiter{
		tokens:    capacity,
		capacity:  capacity,
		rate:      rate,
		lastCheck: time.Now(),
	}
}

// Allow checks if n tokens are available
func (r *RateLimiter) Allow(n float64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(r.lastCheck).Seconds()
	r.lastCheck = now

	// Add tokens based on elapsed time
	r.tokens += elapsed * r.rate
	if r.tokens > r.capacity {
		r.tokens = r.capacity
	}

	// Check if we have enough tokens
	if r.tokens >= n {
		r.tokens -= n
		return true
	}

	return false
}

// Wait blocks until n tokens are available
func (r *RateLimiter) Wait(n float64) {
	for !r.Allow(n) {
		time.Sleep(100 * time.Millisecond)
	}
}

// CircuitBreaker implements circuit breaker pattern
type CircuitBreaker struct {
	mu               sync.RWMutex
	failureThreshold int
	resetTimeout     time.Duration
	failures         int
	lastFailure      time.Time
	state            CircuitState
}

// CircuitState represents the circuit breaker state
type CircuitState int

const (
	CircuitClosed CircuitState = iota
	CircuitOpen
	CircuitHalfOpen
)

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(threshold int, resetTimeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		failureThreshold: threshold,
		resetTimeout:     resetTimeout,
		state:            CircuitClosed,
	}
}

// Call executes a function with circuit breaker protection
func (cb *CircuitBreaker) Call(fn func() error) error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	// Check state
	switch cb.state {
	case CircuitOpen:
		// Check if we should try half-open
		if time.Since(cb.lastFailure) > cb.resetTimeout {
			cb.state = CircuitHalfOpen
			cb.failures = 0
		} else {
			return ErrCircuitOpen
		}
	}

	// Execute function
	err := fn()
	if err != nil {
		cb.failures++
		cb.lastFailure = time.Now()

		if cb.failures >= cb.failureThreshold {
			cb.state = CircuitOpen
		}
		return err
	}

	// Success - reset or close circuit
	if cb.state == CircuitHalfOpen {
		cb.state = CircuitClosed
	}
	cb.failures = 0

	return nil
}

// State returns the current circuit state
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// ErrCircuitOpen is returned when circuit is open
var ErrCircuitOpen = fmt.Errorf("circuit breaker is open")

// BatchBuffer buffers items for batch processing
type BatchBuffer[T any] struct {
	items     []T
	maxSize   int
	maxWait   time.Duration
	flushFunc func([]T) error
	mu        sync.Mutex
	timer     *time.Timer
	lastFlush time.Time
}

// NewBatchBuffer creates a new batch buffer
func NewBatchBuffer[T any](maxSize int, maxWait time.Duration, flushFunc func([]T) error) *BatchBuffer[T] {
	return &BatchBuffer[T]{
		items:     make([]T, 0, maxSize),
		maxSize:   maxSize,
		maxWait:   maxWait,
		flushFunc: flushFunc,
		lastFlush: time.Now(),
	}
}

// Add adds an item to the buffer
func (b *BatchBuffer[T]) Add(item T) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.items = append(b.items, item)

	// Check if we should flush
	if len(b.items) >= b.maxSize {
		return b.flush()
	}

	// Set timer for time-based flush
	if b.timer == nil {
		b.timer = time.AfterFunc(b.maxWait, func() {
			b.mu.Lock()
			defer b.mu.Unlock()
			b.flush()
		})
	}

	return nil
}

// Flush forces a flush of the buffer
func (b *BatchBuffer[T]) Flush() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.flush()
}

// flush flushes the buffer (must be called with lock held)
func (b *BatchBuffer[T]) flush() error {
	if len(b.items) == 0 {
		return nil
	}

	// Cancel timer
	if b.timer != nil {
		b.timer.Stop()
		b.timer = nil
	}

	// Copy items for processing
	items := make([]T, len(b.items))
	copy(items, b.items)

	// Clear buffer
	b.items = b.items[:0]
	b.lastFlush = time.Now()

	// Process batch
	return b.flushFunc(items)
}

// Size returns the current buffer size
func (b *BatchBuffer[T]) Size() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.items)
}
