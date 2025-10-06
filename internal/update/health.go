package update

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"fleetd.sh/internal/agent/metrics"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/process"
)

// HealthChecker performs system health checks
type HealthChecker struct {
	checks    []HealthCheck
	collector *metrics.Collector
	mu        sync.RWMutex
}

// HealthCheck represents a health check
type HealthCheck struct {
	Name        string
	Description string
	Critical    bool
	Timeout     time.Duration
	CheckFunc   func(ctx context.Context) error
}

// HealthStatus represents overall health status
type HealthStatus struct {
	Healthy   bool                   `json:"healthy"`
	Timestamp time.Time              `json:"timestamp"`
	Checks    []HealthCheckResult    `json:"checks"`
	Metrics   map[string]interface{} `json:"metrics"`
}

// HealthCheckResult represents the result of a health check
type HealthCheckResult struct {
	Name        string        `json:"name"`
	Healthy     bool          `json:"healthy"`
	Critical    bool          `json:"critical"`
	Error       string        `json:"error,omitempty"`
	Duration    time.Duration `json:"duration"`
	LastChecked time.Time     `json:"last_checked"`
}

// NewHealthChecker creates a new health checker
func NewHealthChecker() *HealthChecker {
	hc := &HealthChecker{
		collector: metrics.NewCollector(),
	}

	// Register default health checks
	hc.registerDefaultChecks()

	return hc
}

// registerDefaultChecks registers default health checks
func (hc *HealthChecker) registerDefaultChecks() {
	hc.checks = []HealthCheck{
		{
			Name:        "cpu_usage",
			Description: "Check CPU usage is below threshold",
			Critical:    false,
			Timeout:     5 * time.Second,
			CheckFunc:   hc.checkCPUUsage,
		},
		{
			Name:        "memory_usage",
			Description: "Check memory usage is below threshold",
			Critical:    false,
			Timeout:     5 * time.Second,
			CheckFunc:   hc.checkMemoryUsage,
		},
		{
			Name:        "disk_space",
			Description: "Check disk space availability",
			Critical:    true,
			Timeout:     5 * time.Second,
			CheckFunc:   hc.checkDiskSpace,
		},
		{
			Name:        "network_connectivity",
			Description: "Check network connectivity",
			Critical:    true,
			Timeout:     10 * time.Second,
			CheckFunc:   hc.checkNetworkConnectivity,
		},
		{
			Name:        "critical_services",
			Description: "Check critical services are running",
			Critical:    true,
			Timeout:     10 * time.Second,
			CheckFunc:   hc.checkCriticalServices,
		},
		{
			Name:        "agent_process",
			Description: "Check agent process health",
			Critical:    true,
			Timeout:     5 * time.Second,
			CheckFunc:   hc.checkAgentProcess,
		},
		{
			Name:        "file_system",
			Description: "Check file system integrity",
			Critical:    false,
			Timeout:     10 * time.Second,
			CheckFunc:   hc.checkFileSystem,
		},
	}
}

// CheckHealth performs all health checks
func (hc *HealthChecker) CheckHealth(ctx context.Context) error {
	status := hc.GetHealthStatus(ctx)

	if !status.Healthy {
		var criticalErrors []string
		for _, result := range status.Checks {
			if result.Critical && !result.Healthy {
				criticalErrors = append(criticalErrors, fmt.Sprintf("%s: %s", result.Name, result.Error))
			}
		}

		if len(criticalErrors) > 0 {
			return fmt.Errorf("critical health checks failed: %s", strings.Join(criticalErrors, "; "))
		}

		return fmt.Errorf("health checks failed")
	}

	return nil
}

// GetHealthStatus returns the current health status
func (hc *HealthChecker) GetHealthStatus(ctx context.Context) *HealthStatus {
	status := &HealthStatus{
		Healthy:   true,
		Timestamp: time.Now(),
		Checks:    []HealthCheckResult{},
		Metrics:   make(map[string]interface{}),
	}

	// Collect system metrics
	if sysMetrics, err := hc.collector.Collect(); err == nil {
		status.Metrics["cpu_usage"] = sysMetrics.CPU.UsagePercent
		status.Metrics["memory_usage"] = sysMetrics.Memory.UsedPercent
		status.Metrics["disk_usage"] = sysMetrics.Disk.UsedPercent
		status.Metrics["uptime"] = sysMetrics.System.Uptime
	}

	// Run health checks in parallel
	var wg sync.WaitGroup
	results := make(chan HealthCheckResult, len(hc.checks))

	for _, check := range hc.checks {
		wg.Add(1)
		go func(c HealthCheck) {
			defer wg.Done()

			checkCtx, cancel := context.WithTimeout(ctx, c.Timeout)
			defer cancel()

			start := time.Now()
			err := c.CheckFunc(checkCtx)
			duration := time.Since(start)

			result := HealthCheckResult{
				Name:        c.Name,
				Critical:    c.Critical,
				Healthy:     err == nil,
				Duration:    duration,
				LastChecked: time.Now(),
			}

			if err != nil {
				result.Error = err.Error()
				if c.Critical {
					status.Healthy = false
				}
			}

			results <- result
		}(check)
	}

	// Wait for all checks to complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	for result := range results {
		status.Checks = append(status.Checks, result)
	}

	return status
}

// checkCPUUsage checks CPU usage
func (hc *HealthChecker) checkCPUUsage(ctx context.Context) error {
	percentages, err := cpu.PercentWithContext(ctx, 2*time.Second, false)
	if err != nil {
		return fmt.Errorf("failed to get CPU usage: %w", err)
	}

	if len(percentages) > 0 && percentages[0] > 90 {
		return fmt.Errorf("CPU usage too high: %.2f%%", percentages[0])
	}

	return nil
}

// checkMemoryUsage checks memory usage
func (hc *HealthChecker) checkMemoryUsage(ctx context.Context) error {
	vmStat, err := mem.VirtualMemory()
	if err != nil {
		return fmt.Errorf("failed to get memory usage: %w", err)
	}

	if vmStat.UsedPercent > 95 {
		return fmt.Errorf("memory usage too high: %.2f%%", vmStat.UsedPercent)
	}

	// Check for memory pressure
	if vmStat.Available < 100*1024*1024 { // Less than 100MB available
		return fmt.Errorf("low available memory: %d MB", vmStat.Available/(1024*1024))
	}

	return nil
}

// checkDiskSpace checks disk space availability
func (hc *HealthChecker) checkDiskSpace(ctx context.Context) error {
	// Check root partition
	usage, err := disk.Usage("/")
	if err != nil {
		return fmt.Errorf("failed to get disk usage: %w", err)
	}

	if usage.UsedPercent > 95 {
		return fmt.Errorf("disk usage too high: %.2f%%", usage.UsedPercent)
	}

	if usage.Free < 500*1024*1024 { // Less than 500MB free
		return fmt.Errorf("low disk space: %d MB free", usage.Free/(1024*1024))
	}

	// Check /var partition if separate
	if varUsage, err := disk.Usage("/var"); err == nil && varUsage.Path != usage.Path {
		if varUsage.UsedPercent > 95 {
			return fmt.Errorf("/var disk usage too high: %.2f%%", varUsage.UsedPercent)
		}
	}

	return nil
}

// checkNetworkConnectivity checks network connectivity
func (hc *HealthChecker) checkNetworkConnectivity(ctx context.Context) error {
	// Check if we can resolve DNS
	resolver := &net.Resolver{}
	_, err := resolver.LookupHost(ctx, "google.com")
	if err != nil {
		return fmt.Errorf("DNS resolution failed: %w", err)
	}

	// Check if we can establish HTTP connection
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	req, err := http.NewRequestWithContext(ctx, "GET", "http://www.google.com/generate_204", nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("network connectivity test failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 204 && resp.StatusCode != 200 {
		return fmt.Errorf("unexpected response status: %d", resp.StatusCode)
	}

	return nil
}

// checkCriticalServices checks if critical services are running
func (hc *HealthChecker) checkCriticalServices(ctx context.Context) error {
	if runtime.GOOS != "linux" {
		return nil // Skip on non-Linux systems
	}

	// Check systemd services
	services := []string{"systemd-resolved", "systemd-networkd", "ssh"}

	for _, service := range services {
		cmd := exec.CommandContext(ctx, "systemctl", "is-active", service)
		output, err := cmd.Output()
		if err != nil {
			// Service might not exist, which is okay
			continue
		}

		status := strings.TrimSpace(string(output))
		if status != "active" && status != "activating" {
			log.Printf("Service %s is not active: %s", service, status)
			// Non-critical, just log
		}
	}

	return nil
}

// checkAgentProcess checks the agent process health
func (hc *HealthChecker) checkAgentProcess(ctx context.Context) error {
	pid := os.Getpid()
	proc, err := process.NewProcess(int32(pid))
	if err != nil {
		return fmt.Errorf("failed to get agent process: %w", err)
	}

	// Check CPU usage
	cpuPercent, err := proc.CPUPercent()
	if err == nil && cpuPercent > 50 {
		return fmt.Errorf("agent CPU usage too high: %.2f%%", cpuPercent)
	}

	// Check memory usage
	memInfo, err := proc.MemoryInfo()
	if err == nil && memInfo.RSS > 500*1024*1024 { // More than 500MB RSS
		return fmt.Errorf("agent memory usage too high: %d MB", memInfo.RSS/(1024*1024))
	}

	// Check open file descriptors
	if runtime.GOOS == "linux" {
		files, err := proc.OpenFiles()
		if err == nil && len(files) > 1000 {
			return fmt.Errorf("too many open files: %d", len(files))
		}
	}

	// Check thread count
	threads, err := proc.NumThreads()
	if err == nil && threads > 100 {
		return fmt.Errorf("too many threads: %d", threads)
	}

	return nil
}

// checkFileSystem checks file system integrity
func (hc *HealthChecker) checkFileSystem(ctx context.Context) error {
	// Check if critical directories are accessible
	criticalDirs := []string{
		"/etc/fleetd",
		"/var/lib/fleetd",
		"/var/log/fleetd",
	}

	for _, dir := range criticalDirs {
		if _, err := os.Stat(dir); err != nil {
			if os.IsNotExist(err) {
				// Directory doesn't exist, which might be okay
				continue
			}
			return fmt.Errorf("cannot access directory %s: %w", dir, err)
		}

		// Try to write a test file
		testFile := filepath.Join(dir, ".health_check")
		if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
			return fmt.Errorf("cannot write to directory %s: %w", dir, err)
		}
		os.Remove(testFile)
	}

	// Check for read-only file systems on Linux
	if runtime.GOOS == "linux" {
		mounts, err := os.ReadFile("/proc/mounts")
		if err == nil {
			lines := strings.Split(string(mounts), "\n")
			for _, line := range lines {
				fields := strings.Fields(line)
				if len(fields) >= 4 {
					mountPoint := fields[1]
					options := fields[3]
					if (mountPoint == "/" || mountPoint == "/var") && strings.Contains(options, "ro") {
						return fmt.Errorf("file system %s is read-only", mountPoint)
					}
				}
			}
		}
	}

	return nil
}

// AddCustomCheck adds a custom health check
func (hc *HealthChecker) AddCustomCheck(check HealthCheck) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	hc.checks = append(hc.checks, check)
}

// RemoveCheck removes a health check by name
func (hc *HealthChecker) RemoveCheck(name string) {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	var filtered []HealthCheck
	for _, check := range hc.checks {
		if check.Name != name {
			filtered = append(filtered, check)
		}
	}
	hc.checks = filtered
}

// RunCheck runs a specific health check by name
func (hc *HealthChecker) RunCheck(ctx context.Context, name string) (*HealthCheckResult, error) {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	for _, check := range hc.checks {
		if check.Name == name {
			checkCtx, cancel := context.WithTimeout(ctx, check.Timeout)
			defer cancel()

			start := time.Now()
			err := check.CheckFunc(checkCtx)
			duration := time.Since(start)

			result := &HealthCheckResult{
				Name:        check.Name,
				Critical:    check.Critical,
				Healthy:     err == nil,
				Duration:    duration,
				LastChecked: time.Now(),
			}

			if err != nil {
				result.Error = err.Error()
			}

			return result, nil
		}
	}

	return nil, fmt.Errorf("health check %s not found", name)
}
