package ferrors

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// CircuitBreakerState represents the state of a circuit breaker
type CircuitBreakerState int32

const (
	// StateClosed allows all requests through
	StateClosed CircuitBreakerState = iota
	// StateOpen blocks all requests
	StateOpen
	// StateHalfOpen allows limited requests through for testing
	StateHalfOpen
)

func (s CircuitBreakerState) String() string {
	switch s {
	case StateClosed:
		return "CLOSED"
	case StateOpen:
		return "OPEN"
	case StateHalfOpen:
		return "HALF_OPEN"
	default:
		return "UNKNOWN"
	}
}

// CircuitBreakerConfig holds configuration for a circuit breaker
type CircuitBreakerConfig struct {
	// MaxFailures is the number of failures before opening the circuit
	MaxFailures uint32
	// MaxRequests is the number of requests allowed in half-open state
	MaxRequests uint32
	// Interval is the cyclic period for resetting failure count in closed state
	Interval time.Duration
	// Timeout is the duration of open state before switching to half-open
	Timeout time.Duration
	// OnStateChange is called when the state changes
	OnStateChange func(from, to CircuitBreakerState)
	// ShouldTrip determines if an error should count as a failure
	ShouldTrip func(error) bool
}

// DefaultCircuitBreakerConfig returns a default configuration
func DefaultCircuitBreakerConfig() *CircuitBreakerConfig {
	return &CircuitBreakerConfig{
		MaxFailures: 5,
		MaxRequests: 1,
		Interval:    60 * time.Second,
		Timeout:     30 * time.Second,
		OnStateChange: func(from, to CircuitBreakerState) {
			// Default no-op
		},
		ShouldTrip: func(err error) bool {
			// By default, trip on non-retryable errors
			return !IsRetryable(err)
		},
	}
}

// CircuitBreaker implements the circuit breaker pattern
type CircuitBreaker struct {
	config *CircuitBreakerConfig

	state           atomic.Int32
	failures        atomic.Uint32
	requests        atomic.Uint32
	successCount    atomic.Uint64
	failureCount    atomic.Uint64
	lastFailureTime atomic.Int64
	nextBackOff     atomic.Int64

	mu            sync.RWMutex
	lastStateTime time.Time
	generation    uint64
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(config *CircuitBreakerConfig) *CircuitBreaker {
	if config == nil {
		config = DefaultCircuitBreakerConfig()
	}

	cb := &CircuitBreaker{
		config:        config,
		lastStateTime: time.Now(),
	}
	cb.state.Store(int32(StateClosed))

	return cb
}

// Execute runs a function through the circuit breaker
func (cb *CircuitBreaker) Execute(ctx context.Context, fn func() error) error {
	if err := cb.beforeRequest(); err != nil {
		return err
	}

	err := fn()
	cb.afterRequest(err)

	return err
}

// ExecuteWithFallback runs a function with a fallback on circuit open
func (cb *CircuitBreaker) ExecuteWithFallback(ctx context.Context, fn func() error, fallback func() error) error {
	if err := cb.beforeRequest(); err != nil {
		if fallback != nil {
			return fallback()
		}
		return err
	}

	err := fn()
	cb.afterRequest(err)

	if err != nil && fallback != nil {
		return fallback()
	}

	return err
}

// GetState returns the current state of the circuit breaker
func (cb *CircuitBreaker) GetState() CircuitBreakerState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	state := CircuitBreakerState(cb.state.Load())
	now := time.Now()

	switch state {
	case StateOpen:
		if now.After(cb.lastStateTime.Add(cb.config.Timeout)) {
			cb.mu.RUnlock()
			cb.mu.Lock()
			cb.setState(StateHalfOpen)
			cb.mu.Unlock()
			cb.mu.RLock()
			return StateHalfOpen
		}
	case StateClosed:
		if now.After(cb.lastStateTime.Add(cb.config.Interval)) {
			cb.mu.RUnlock()
			cb.mu.Lock()
			cb.failures.Store(0)
			cb.lastStateTime = now
			cb.generation++
			cb.mu.Unlock()
			cb.mu.RLock()
		}
	}

	return state
}

// GetMetrics returns circuit breaker metrics
func (cb *CircuitBreaker) GetMetrics() map[string]any {
	return map[string]any{
		"state":         cb.GetState().String(),
		"failures":      cb.failures.Load(),
		"requests":      cb.requests.Load(),
		"success_count": cb.successCount.Load(),
		"failure_count": cb.failureCount.Load(),
		"last_failure":  time.Unix(0, cb.lastFailureTime.Load()),
	}
}

// Reset resets the circuit breaker to closed state
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.setState(StateClosed)
	cb.failures.Store(0)
	cb.requests.Store(0)
	cb.lastStateTime = time.Now()
	cb.generation++
}

func (cb *CircuitBreaker) beforeRequest() error {
	state := cb.GetState()

	switch state {
	case StateOpen:
		return &FleetError{
			Code:      ErrCodeUnavailable,
			Message:   "Circuit breaker is open",
			Severity:  SeverityWarning,
			Retryable: true,
			Metadata: map[string]any{
				"circuit_state": "OPEN",
				"retry_after":   cb.lastStateTime.Add(cb.config.Timeout).Sub(time.Now()),
			},
		}
	case StateHalfOpen:
		if cb.requests.Add(1) > cb.config.MaxRequests {
			return &FleetError{
				Code:      ErrCodeUnavailable,
				Message:   "Circuit breaker is half-open but max requests exceeded",
				Severity:  SeverityWarning,
				Retryable: true,
				Metadata: map[string]any{
					"circuit_state": "HALF_OPEN",
					"max_requests":  cb.config.MaxRequests,
				},
			}
		}
	}

	return nil
}

func (cb *CircuitBreaker) afterRequest(err error) {
	state := CircuitBreakerState(cb.state.Load())

	if err == nil {
		cb.onSuccess(state)
	} else {
		cb.onFailure(state, err)
	}
}

func (cb *CircuitBreaker) onSuccess(state CircuitBreakerState) {
	cb.successCount.Add(1)

	switch state {
	case StateHalfOpen:
		cb.mu.Lock()
		defer cb.mu.Unlock()

		if cb.requests.Load() >= cb.config.MaxRequests {
			cb.setState(StateClosed)
			cb.failures.Store(0)
			cb.requests.Store(0)
		}
	}
}

func (cb *CircuitBreaker) onFailure(state CircuitBreakerState, err error) {
	cb.failureCount.Add(1)
	cb.lastFailureTime.Store(time.Now().UnixNano())

	// Check if this error should trip the circuit
	shouldTrip := true
	if cb.config.ShouldTrip != nil {
		shouldTrip = cb.config.ShouldTrip(err)
	}

	if !shouldTrip {
		return
	}

	switch state {
	case StateClosed:
		newFailures := cb.failures.Add(1)
		if newFailures >= cb.config.MaxFailures {
			cb.mu.Lock()
			defer cb.mu.Unlock()
			cb.setState(StateOpen)
		}
	case StateHalfOpen:
		cb.mu.Lock()
		defer cb.mu.Unlock()
		cb.setState(StateOpen)
	}
}

func (cb *CircuitBreaker) setState(state CircuitBreakerState) {
	oldState := CircuitBreakerState(cb.state.Load())

	if oldState == state {
		return
	}

	cb.state.Store(int32(state))
	cb.lastStateTime = time.Now()

	if state == StateOpen {
		cb.requests.Store(0)
	}

	if cb.config.OnStateChange != nil {
		cb.config.OnStateChange(oldState, state)
	}
}

// CircuitBreakerGroup manages multiple circuit breakers
type CircuitBreakerGroup struct {
	mu       sync.RWMutex
	breakers map[string]*CircuitBreaker
	config   *CircuitBreakerConfig
}

// NewCircuitBreakerGroup creates a new circuit breaker group
func NewCircuitBreakerGroup(config *CircuitBreakerConfig) *CircuitBreakerGroup {
	if config == nil {
		config = DefaultCircuitBreakerConfig()
	}

	return &CircuitBreakerGroup{
		breakers: make(map[string]*CircuitBreaker),
		config:   config,
	}
}

// Get returns a circuit breaker for the given name
func (g *CircuitBreakerGroup) Get(name string) *CircuitBreaker {
	g.mu.RLock()
	cb, exists := g.breakers[name]
	g.mu.RUnlock()

	if exists {
		return cb
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	// Double-check after acquiring write lock
	if cb, exists = g.breakers[name]; exists {
		return cb
	}

	cb = NewCircuitBreaker(g.config)
	g.breakers[name] = cb

	return cb
}

// Execute runs a function through a named circuit breaker
func (g *CircuitBreakerGroup) Execute(ctx context.Context, name string, fn func() error) error {
	return g.Get(name).Execute(ctx, fn)
}

// GetMetrics returns metrics for all circuit breakers
func (g *CircuitBreakerGroup) GetMetrics() map[string]map[string]any {
	g.mu.RLock()
	defer g.mu.RUnlock()

	metrics := make(map[string]map[string]any)
	for name, cb := range g.breakers {
		metrics[name] = cb.GetMetrics()
	}

	return metrics
}

// Reset resets all circuit breakers
func (g *CircuitBreakerGroup) Reset() {
	g.mu.RLock()
	defer g.mu.RUnlock()

	for _, cb := range g.breakers {
		cb.Reset()
	}
}
