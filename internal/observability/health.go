package observability

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// HealthStatus represents the health status of a component
type HealthStatus string

const (
	HealthStatusHealthy   HealthStatus = "healthy"
	HealthStatusDegraded  HealthStatus = "degraded"
	HealthStatusUnhealthy HealthStatus = "unhealthy"
)

// HealthCheck represents a single health check
type HealthCheck struct {
	Name        string        `json:"name"`
	Status      HealthStatus  `json:"status"`
	Message     string        `json:"message,omitempty"`
	LastChecked time.Time     `json:"last_checked"`
	Duration    time.Duration `json:"duration_ms"`
	Metadata    any           `json:"metadata,omitempty"`
}

// HealthChecker performs health checks
type HealthChecker interface {
	Check(ctx context.Context) HealthCheck
}

// HealthCheckFunc is a function that performs a health check
type HealthCheckFunc func(ctx context.Context) HealthCheck

// Check implements HealthChecker
func (f HealthCheckFunc) Check(ctx context.Context) HealthCheck {
	return f(ctx)
}

// HealthService manages health checks
type HealthService struct {
	checks   map[string]HealthChecker
	mu       sync.RWMutex
	logger   *slog.Logger
	provider *Provider
	cache    map[string]*HealthCheck
	cacheTTL time.Duration
}

// NewHealthService creates a new health service
func NewHealthService(provider *Provider) *HealthService {
	return &HealthService{
		checks:   make(map[string]HealthChecker),
		cache:    make(map[string]*HealthCheck),
		cacheTTL: 5 * time.Second,
		logger:   slog.Default().With("component", "health"),
		provider: provider,
	}
}

// RegisterCheck registers a health check
func (s *HealthService) RegisterCheck(name string, checker HealthChecker) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.checks[name] = checker
	s.logger.Info("Health check registered", "name", name)
}

// RegisterCheckFunc registers a health check function
func (s *HealthService) RegisterCheckFunc(name string, fn func(ctx context.Context) error) {
	s.RegisterCheck(name, HealthCheckFunc(func(ctx context.Context) HealthCheck {
		startTime := time.Now()
		err := fn(ctx)
		duration := time.Since(startTime)

		check := HealthCheck{
			Name:        name,
			LastChecked: time.Now(),
			Duration:    duration,
		}

		if err != nil {
			check.Status = HealthStatusUnhealthy
			check.Message = err.Error()
		} else {
			check.Status = HealthStatusHealthy
			check.Message = "OK"
		}

		return check
	}))
}

// Check performs all health checks
func (s *HealthService) Check(ctx context.Context) map[string]HealthCheck {
	s.mu.RLock()
	checkers := make(map[string]HealthChecker, len(s.checks))
	for name, checker := range s.checks {
		checkers[name] = checker
	}
	s.mu.RUnlock()

	results := make(map[string]HealthCheck)
	var wg sync.WaitGroup

	for name, checker := range checkers {
		// Check cache first
		if cached := s.getCached(name); cached != nil {
			results[name] = *cached
			continue
		}

		wg.Add(1)
		go func(name string, checker HealthChecker) {
			defer wg.Done()

			// Create context with timeout
			checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			// Start span
			checkCtx, span := s.provider.Tracer("health").Start(checkCtx, fmt.Sprintf("health.check.%s", name))
			defer span.End()

			// Perform check
			check := checker.Check(checkCtx)

			// Record metrics
			attrs := []attribute.KeyValue{
				attribute.String("check", name),
				attribute.String("status", string(check.Status)),
			}

			if s.provider.metrics != nil {
				// Record check duration
				checkDuration, _ := s.provider.Meter("health").Float64Histogram(
					"health_check_duration_seconds",
					metric.WithDescription("Health check duration in seconds"),
				)
				checkDuration.Record(checkCtx, check.Duration.Seconds(), metric.WithAttributes(attrs...))

				// Record check status
				checkStatus, _ := s.provider.Meter("health").Int64Counter(
					"health_check_total",
					metric.WithDescription("Total number of health checks"),
				)
				checkStatus.Add(checkCtx, 1, metric.WithAttributes(attrs...))
			}

			// Update cache
			s.updateCache(name, &check)

			// Set span attributes
			span.SetAttributes(attrs...)

			s.mu.Lock()
			results[name] = check
			s.mu.Unlock()
		}(name, checker)
	}

	wg.Wait()
	return results
}

// CheckSingle performs a single health check
func (s *HealthService) CheckSingle(ctx context.Context, name string) (*HealthCheck, error) {
	s.mu.RLock()
	checker, exists := s.checks[name]
	s.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("health check not found: %s", name)
	}

	// Check cache first
	if cached := s.getCached(name); cached != nil {
		return cached, nil
	}

	// Perform check
	check := checker.Check(ctx)
	s.updateCache(name, &check)

	return &check, nil
}

// OverallStatus returns the overall health status
func (s *HealthService) OverallStatus(ctx context.Context) HealthStatus {
	checks := s.Check(ctx)

	hasUnhealthy := false
	hasDegraded := false

	for _, check := range checks {
		switch check.Status {
		case HealthStatusUnhealthy:
			hasUnhealthy = true
		case HealthStatusDegraded:
			hasDegraded = true
		}
	}

	if hasUnhealthy {
		return HealthStatusUnhealthy
	}
	if hasDegraded {
		return HealthStatusDegraded
	}
	return HealthStatusHealthy
}

// getCached returns a cached health check if it's still valid
func (s *HealthService) getCached(name string) *HealthCheck {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cached, exists := s.cache[name]
	if !exists {
		return nil
	}

	if time.Since(cached.LastChecked) > s.cacheTTL {
		return nil
	}

	return cached
}

// updateCache updates the health check cache
func (s *HealthService) updateCache(name string, check *HealthCheck) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cache[name] = check
}

// HTTPHandler returns an HTTP handler for health checks
func (s *HealthService) HTTPHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Check for specific check
		checkName := r.URL.Query().Get("check")
		if checkName != "" {
			check, err := s.CheckSingle(ctx, checkName)
			if err != nil {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}

			s.writeJSONResponse(w, check, check.Status)
			return
		}

		// Perform all checks
		checks := s.Check(ctx)
		overallStatus := s.OverallStatus(ctx)

		response := struct {
			Status HealthStatus           `json:"status"`
			Checks map[string]HealthCheck `json:"checks"`
			Time   time.Time              `json:"time"`
		}{
			Status: overallStatus,
			Checks: checks,
			Time:   time.Now(),
		}

		s.writeJSONResponse(w, response, overallStatus)
	}
}

// LivenessHandler returns an HTTP handler for liveness checks
func (s *HealthService) LivenessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Simple liveness check - service is running
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}
}

// ReadinessHandler returns an HTTP handler for readiness checks
func (s *HealthService) ReadinessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		overallStatus := s.OverallStatus(ctx)

		switch overallStatus {
		case HealthStatusHealthy:
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("READY"))
		case HealthStatusDegraded:
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("DEGRADED"))
		case HealthStatusUnhealthy:
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("NOT READY"))
		}
	}
}

// writeJSONResponse writes a JSON response with appropriate status code
func (s *HealthService) writeJSONResponse(w http.ResponseWriter, data any, status HealthStatus) {
	w.Header().Set("Content-Type", "application/json")

	// Set status code based on health status
	switch status {
	case HealthStatusHealthy:
		w.WriteHeader(http.StatusOK)
	case HealthStatusDegraded:
		w.WriteHeader(http.StatusOK) // Still OK but degraded
	case HealthStatusUnhealthy:
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	if err := json.NewEncoder(w).Encode(data); err != nil {
		s.logger.Error("Failed to encode health response", "error", err)
	}
}

// Common health check implementations

// DatabaseHealthCheck creates a database health check
func DatabaseHealthCheck(name string, pingFunc func(context.Context) error) HealthChecker {
	return HealthCheckFunc(func(ctx context.Context) HealthCheck {
		startTime := time.Now()
		err := pingFunc(ctx)
		duration := time.Since(startTime)

		check := HealthCheck{
			Name:        name,
			LastChecked: time.Now(),
			Duration:    duration,
		}

		if err != nil {
			check.Status = HealthStatusUnhealthy
			check.Message = fmt.Sprintf("Database ping failed: %v", err)
		} else {
			check.Status = HealthStatusHealthy
			check.Message = "Database connection healthy"
		}

		return check
	})
}

// HTTPHealthCheck creates an HTTP endpoint health check
func HTTPHealthCheck(name, url string, expectedStatus int, timeout time.Duration) HealthChecker {
	return HealthCheckFunc(func(ctx context.Context) HealthCheck {
		startTime := time.Now()

		client := &http.Client{Timeout: timeout}
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return HealthCheck{
				Name:        name,
				Status:      HealthStatusUnhealthy,
				Message:     fmt.Sprintf("Failed to create request: %v", err),
				LastChecked: time.Now(),
				Duration:    time.Since(startTime),
			}
		}

		resp, err := client.Do(req)
		duration := time.Since(startTime)

		check := HealthCheck{
			Name:        name,
			LastChecked: time.Now(),
			Duration:    duration,
		}

		if err != nil {
			check.Status = HealthStatusUnhealthy
			check.Message = fmt.Sprintf("Request failed: %v", err)
			return check
		}
		defer resp.Body.Close()

		if resp.StatusCode != expectedStatus {
			check.Status = HealthStatusUnhealthy
			check.Message = fmt.Sprintf("Unexpected status code: %d", resp.StatusCode)
		} else {
			check.Status = HealthStatusHealthy
			check.Message = fmt.Sprintf("Endpoint healthy (status: %d)", resp.StatusCode)
		}

		return check
	})
}

// DiskSpaceHealthCheck creates a disk space health check
func DiskSpaceHealthCheck(path string, minFreeBytes uint64) HealthChecker {
	return HealthCheckFunc(func(ctx context.Context) HealthCheck {
		startTime := time.Now()

		// TODO: Implement disk space check using syscall or library
		// For now, return healthy
		return HealthCheck{
			Name:        "disk_space",
			Status:      HealthStatusHealthy,
			Message:     "Disk space check not yet implemented",
			LastChecked: time.Now(),
			Duration:    time.Since(startTime),
		}
	})
}

// MemoryHealthCheck creates a memory usage health check
func MemoryHealthCheck(maxUsagePercent float64) HealthChecker {
	return HealthCheckFunc(func(ctx context.Context) HealthCheck {
		startTime := time.Now()

		// TODO: Implement memory check using runtime or gopsutil
		// For now, return healthy
		return HealthCheck{
			Name:        "memory",
			Status:      HealthStatusHealthy,
			Message:     "Memory check not yet implemented",
			LastChecked: time.Now(),
			Duration:    time.Since(startTime),
		}
	})
}
