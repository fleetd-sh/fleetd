package health

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"sync"
	"time"

	"log/slog"
)

// Status represents health check status
type Status string

const (
	StatusHealthy   Status = "healthy"
	StatusDegraded  Status = "degraded"
	StatusUnhealthy Status = "unhealthy"
	StatusUnknown   Status = "unknown"
)

// Check represents a single health check result
type Check struct {
	Name      string                 `json:"name"`
	Status    Status                 `json:"status"`
	Message   string                 `json:"message,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
	Duration  time.Duration          `json:"duration"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// Report represents overall health status
type Report struct {
	Status     Status              `json:"status"`
	Checks     []Check             `json:"checks"`
	Version    string              `json:"version"`
	Uptime     time.Duration       `json:"uptime"`
	Timestamp  time.Time           `json:"timestamp"`
	Thresholds HealthThresholds    `json:"thresholds"`
}

// HealthThresholds defines health check thresholds
type HealthThresholds struct {
	CPULimit           float64       `json:"cpu_limit"`
	MemoryLimit        float64       `json:"memory_limit"`
	DiskLimit          float64       `json:"disk_limit"`
	ResponseTimeLimit  time.Duration `json:"response_time_limit"`
	ErrorRateLimit     float64       `json:"error_rate_limit"`
}

// Checker performs health checks
type Checker struct {
	db          *sql.DB
	httpClient  *http.Client
	checks      map[string]CheckFunc
	thresholds  HealthThresholds
	startTime   time.Time
	version     string
	mu          sync.RWMutex
	lastReport  *Report
	subscribers []chan *Report
}

// CheckFunc is a function that performs a health check
type CheckFunc func(ctx context.Context) Check

// NewChecker creates a new health checker
func NewChecker(db *sql.DB, version string) *Checker {
	return &Checker{
		db:         db,
		httpClient: &http.Client{Timeout: 5 * time.Second},
		checks:     make(map[string]CheckFunc),
		thresholds: DefaultThresholds(),
		startTime:  time.Now(),
		version:    version,
	}
}

// DefaultThresholds returns default health thresholds
func DefaultThresholds() HealthThresholds {
	return HealthThresholds{
		CPULimit:          80.0,  // 80% CPU usage
		MemoryLimit:       90.0,  // 90% memory usage
		DiskLimit:         85.0,  // 85% disk usage
		ResponseTimeLimit: 5 * time.Second,
		ErrorRateLimit:    5.0,   // 5% error rate
	}
}

// RegisterCheck registers a health check
func (c *Checker) RegisterCheck(name string, checkFunc CheckFunc) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.checks[name] = checkFunc
}

// RunChecks runs all registered health checks
func (c *Checker) RunChecks(ctx context.Context) *Report {
	c.mu.RLock()
	checks := make(map[string]CheckFunc)
	for name, check := range c.checks {
		checks[name] = check
	}
	c.mu.RUnlock()

	var wg sync.WaitGroup
	checkResults := make([]Check, 0, len(checks))
	resultChan := make(chan Check, len(checks))

	// Run checks in parallel
	for name, checkFunc := range checks {
		wg.Add(1)
		go func(n string, cf CheckFunc) {
			defer wg.Done()

			// Create timeout context for individual check
			checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()

			start := time.Now()
			check := cf(checkCtx)
			check.Name = n
			check.Duration = time.Since(start)
			check.Timestamp = time.Now()

			resultChan <- check
		}(name, checkFunc)
	}

	// Wait for all checks to complete
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Collect results
	overallStatus := StatusHealthy
	for check := range resultChan {
		checkResults = append(checkResults, check)
		if check.Status == StatusUnhealthy {
			overallStatus = StatusUnhealthy
		} else if check.Status == StatusDegraded && overallStatus != StatusUnhealthy {
			overallStatus = StatusDegraded
		}
	}

	report := &Report{
		Status:     overallStatus,
		Checks:     checkResults,
		Version:    c.version,
		Uptime:     time.Since(c.startTime),
		Timestamp:  time.Now(),
		Thresholds: c.thresholds,
	}

	// Store last report
	c.mu.Lock()
	c.lastReport = report
	c.mu.Unlock()

	// Notify subscribers
	c.notifySubscribers(report)

	return report
}

// GetLastReport returns the last health report
func (c *Checker) GetLastReport() *Report {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastReport
}

// Subscribe subscribes to health check updates
func (c *Checker) Subscribe() <-chan *Report {
	c.mu.Lock()
	defer c.mu.Unlock()

	ch := make(chan *Report, 1)
	c.subscribers = append(c.subscribers, ch)

	// Send current report if available
	if c.lastReport != nil {
		ch <- c.lastReport
	}

	return ch
}

// notifySubscribers sends report to all subscribers
func (c *Checker) notifySubscribers(report *Report) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, ch := range c.subscribers {
		select {
		case ch <- report:
		default:
			// Don't block if subscriber is not reading
		}
	}
}

// StartMonitoring starts continuous health monitoring
func (c *Checker) StartMonitoring(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			report := c.RunChecks(ctx)
			if report.Status != StatusHealthy {
				slog.Warn("Health check detected issues",
					"status", report.Status,
					"failed_checks", c.getFailedChecks(report))
			}
		}
	}
}

// getFailedChecks returns names of failed checks
func (c *Checker) getFailedChecks(report *Report) []string {
	var failed []string
	for _, check := range report.Checks {
		if check.Status != StatusHealthy {
			failed = append(failed, check.Name)
		}
	}
	return failed
}

// Built-in health checks

// DatabaseCheck checks database connectivity
func (c *Checker) DatabaseCheck() CheckFunc {
	return func(ctx context.Context) Check {
		check := Check{Name: "database"}

		// Ping database
		if err := c.db.PingContext(ctx); err != nil {
			check.Status = StatusUnhealthy
			check.Message = fmt.Sprintf("Database ping failed: %v", err)
			return check
		}

		// Check connection pool
		stats := c.db.Stats()
		check.Metadata = map[string]interface{}{
			"open_connections": stats.OpenConnections,
			"in_use":          stats.InUse,
			"idle":            stats.Idle,
		}

		if stats.OpenConnections > 0 {
			check.Status = StatusHealthy
			check.Message = "Database is healthy"
		} else {
			check.Status = StatusDegraded
			check.Message = "No database connections available"
		}

		return check
	}
}

// DiskSpaceCheck checks available disk space
func (c *Checker) DiskSpaceCheck(path string) CheckFunc {
	return func(ctx context.Context) Check {
		check := Check{Name: "disk_space"}

		usages, err := GetDiskUsage([]string{path})
		if err != nil {
			check.Status = StatusUnhealthy
			check.Message = fmt.Sprintf("Failed to check disk usage: %v", err)
			return check
		}

		if len(usages) == 0 {
			check.Status = StatusUnknown
			check.Message = "No disk usage data available"
			return check
		}

		usage := usages[0]
		check.Metadata = map[string]interface{}{
			"total_gb":     usage.Total / (1 << 30),
			"used_gb":      usage.Used / (1 << 30),
			"free_gb":      usage.Free / (1 << 30),
			"used_percent": usage.UsedPercent,
		}

		if usage.UsedPercent > c.thresholds.DiskLimit {
			check.Status = StatusUnhealthy
			check.Message = fmt.Sprintf("Disk usage %.1f%% exceeds threshold %.1f%%",
				usage.UsedPercent, c.thresholds.DiskLimit)
		} else if usage.UsedPercent > c.thresholds.DiskLimit*0.9 {
			check.Status = StatusDegraded
			check.Message = fmt.Sprintf("Disk usage %.1f%% approaching threshold", usage.UsedPercent)
		} else {
			check.Status = StatusHealthy
			check.Message = fmt.Sprintf("Disk usage %.1f%% is healthy", usage.UsedPercent)
		}

		return check
	}
}

// MemoryCheck checks memory usage
func (c *Checker) MemoryCheck() CheckFunc {
	return func(ctx context.Context) Check {
		check := Check{Name: "memory"}

		usage, err := GetMemoryUsage()
		if err != nil {
			check.Status = StatusUnhealthy
			check.Message = fmt.Sprintf("Failed to check memory: %v", err)
			return check
		}

		check.Metadata = map[string]interface{}{
			"total_gb":     usage.Total / (1 << 30),
			"used_gb":      usage.Used / (1 << 30),
			"free_gb":      usage.Available / (1 << 30),
			"used_percent": usage.UsedPercent,
		}

		if usage.UsedPercent > c.thresholds.MemoryLimit {
			check.Status = StatusUnhealthy
			check.Message = fmt.Sprintf("Memory usage %.1f%% exceeds threshold %.1f%%",
				usage.UsedPercent, c.thresholds.MemoryLimit)
		} else if usage.UsedPercent > c.thresholds.MemoryLimit*0.9 {
			check.Status = StatusDegraded
			check.Message = fmt.Sprintf("Memory usage %.1f%% approaching threshold", usage.UsedPercent)
		} else {
			check.Status = StatusHealthy
			check.Message = fmt.Sprintf("Memory usage %.1f%% is healthy", usage.UsedPercent)
		}

		return check
	}
}

// ServiceCheck checks if a service endpoint is reachable
func (c *Checker) ServiceCheck(name, url string, expectedStatus int) CheckFunc {
	return func(ctx context.Context) Check {
		check := Check{Name: name}

		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			check.Status = StatusUnhealthy
			check.Message = fmt.Sprintf("Failed to create request: %v", err)
			return check
		}

		start := time.Now()
		resp, err := c.httpClient.Do(req)
		responseTime := time.Since(start)

		if err != nil {
			check.Status = StatusUnhealthy
			check.Message = fmt.Sprintf("Request failed: %v", err)
			return check
		}
		defer resp.Body.Close()

		check.Metadata = map[string]interface{}{
			"status_code":   resp.StatusCode,
			"response_time": responseTime.Milliseconds(),
		}

		if resp.StatusCode != expectedStatus {
			check.Status = StatusUnhealthy
			check.Message = fmt.Sprintf("Unexpected status code %d (expected %d)",
				resp.StatusCode, expectedStatus)
		} else if responseTime > c.thresholds.ResponseTimeLimit {
			check.Status = StatusDegraded
			check.Message = fmt.Sprintf("Response time %v exceeds threshold %v",
				responseTime, c.thresholds.ResponseTimeLimit)
		} else {
			check.Status = StatusHealthy
			check.Message = fmt.Sprintf("Service is healthy (response time %v)", responseTime)
		}

		return check
	}
}

// DeploymentHealthCheck checks deployment health
func (c *Checker) DeploymentHealthCheck(deploymentID string) CheckFunc {
	return func(ctx context.Context) Check {
		check := Check{Name: fmt.Sprintf("deployment_%s", deploymentID)}

		// Query deployment status
		var status string
		var errorRate float64
		err := c.db.QueryRowContext(ctx, `
			SELECT status,
			       (SELECT COUNT(*) * 100.0 / NULLIF(COUNT(*), 0)
			        FROM device_deployment
			        WHERE deployment_id = ? AND status = 'failed')
			FROM deployment
			WHERE id = ?`,
			deploymentID, deploymentID).Scan(&status, &errorRate)

		if err != nil {
			check.Status = StatusUnhealthy
			check.Message = fmt.Sprintf("Failed to query deployment: %v", err)
			return check
		}

		check.Metadata = map[string]interface{}{
			"deployment_status": status,
			"error_rate":       errorRate,
		}

		if status == "failed" || errorRate > c.thresholds.ErrorRateLimit {
			check.Status = StatusUnhealthy
			check.Message = fmt.Sprintf("Deployment unhealthy: status=%s, error_rate=%.1f%%",
				status, errorRate)
		} else if status == "running" && errorRate > c.thresholds.ErrorRateLimit*0.5 {
			check.Status = StatusDegraded
			check.Message = fmt.Sprintf("Deployment degraded: error_rate=%.1f%%", errorRate)
		} else {
			check.Status = StatusHealthy
			check.Message = "Deployment is healthy"
		}

		return check
	}
}