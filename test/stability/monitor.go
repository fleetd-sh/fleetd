package stability

import (
	"bufio"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"
)

// SystemMonitor monitors system resources and detects issues
type SystemMonitor struct {
	config         *Config
	logger         *logrus.Logger
	mu             sync.RWMutex
	pid            int32
	process        *process.Process
	baselineMemory uint64
	baselineTime   time.Time
	cpuHistory     []float64
	memoryHistory  []uint64
	maxHistorySize int
}

// NewSystemMonitor creates a new system monitor
func NewSystemMonitor(config *Config, logger *logrus.Logger) (*SystemMonitor, error) {
	pid := int32(os.Getpid())
	proc, err := process.NewProcess(pid)
	if err != nil {
		return nil, fmt.Errorf("failed to create process monitor: %w", err)
	}

	// Get baseline memory
	memInfo, err := proc.MemoryInfo()
	if err != nil {
		return nil, fmt.Errorf("failed to get baseline memory: %w", err)
	}

	return &SystemMonitor{
		config:         config,
		logger:         logger,
		pid:            pid,
		process:        proc,
		baselineMemory: memInfo.RSS,
		baselineTime:   time.Now(),
		cpuHistory:     make([]float64, 0),
		memoryHistory:  make([]uint64, 0),
		maxHistorySize: 1000, // Keep last 1000 measurements
	}, nil
}

// CollectMetrics collects current system metrics
func (sm *SystemMonitor) CollectMetrics() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Collect CPU metrics
	if err := sm.collectCPUMetrics(); err != nil {
		sm.logger.WithError(err).Error("Failed to collect CPU metrics")
	}

	// Collect memory metrics
	if err := sm.collectMemoryMetrics(); err != nil {
		sm.logger.WithError(err).Error("Failed to collect memory metrics")
		return err
	}

	// Check for resource leaks
	if err := sm.checkResourceLeaks(); err != nil {
		sm.logger.WithError(err).Warn("Resource leak detected")
		return err
	}

	// Check thresholds
	if err := sm.checkThresholds(); err != nil {
		sm.logger.WithError(err).Warn("Resource threshold exceeded")
		return err
	}

	return nil
}

// collectCPUMetrics collects CPU usage metrics
func (sm *SystemMonitor) collectCPUMetrics() error {
	// Get process CPU usage
	cpuPercent, err := sm.process.CPUPercent()
	if err != nil {
		return fmt.Errorf("failed to get process CPU usage: %w", err)
	}

	// Add to history
	sm.cpuHistory = append(sm.cpuHistory, cpuPercent)
	if len(sm.cpuHistory) > sm.maxHistorySize {
		sm.cpuHistory = sm.cpuHistory[1:]
	}

	return nil
}

// collectMemoryMetrics collects memory usage metrics
func (sm *SystemMonitor) collectMemoryMetrics() error {
	// Get process memory info
	memInfo, err := sm.process.MemoryInfo()
	if err != nil {
		return fmt.Errorf("failed to get process memory info: %w", err)
	}

	// Add to history
	sm.memoryHistory = append(sm.memoryHistory, memInfo.RSS)
	if len(sm.memoryHistory) > sm.maxHistorySize {
		sm.memoryHistory = sm.memoryHistory[1:]
	}

	// Log memory usage periodically
	if len(sm.memoryHistory)%10 == 0 {
		sm.logger.WithFields(logrus.Fields{
			"rss_mb":     memInfo.RSS / 1024 / 1024,
			"vms_mb":     memInfo.VMS / 1024 / 1024,
			"baseline_mb": sm.baselineMemory / 1024 / 1024,
			"increase_mb": (memInfo.RSS - sm.baselineMemory) / 1024 / 1024,
		}).Info("Memory usage")
	}

	return nil
}

// checkResourceLeaks checks for various types of resource leaks
func (sm *SystemMonitor) checkResourceLeaks() error {
	// Check memory leak
	if err := sm.checkMemoryLeak(); err != nil {
		return fmt.Errorf("memory leak detected: %w", err)
	}

	// Check goroutine leak
	if err := sm.checkGoroutineLeak(); err != nil {
		return fmt.Errorf("goroutine leak detected: %w", err)
	}

	// Check file descriptor leak
	if err := sm.checkFileDescriptorLeak(); err != nil {
		return fmt.Errorf("file descriptor leak detected: %w", err)
	}

	return nil
}

// checkMemoryLeak detects memory leaks using trend analysis
func (sm *SystemMonitor) checkMemoryLeak() error {
	if len(sm.memoryHistory) < 10 {
		return nil // Not enough data
	}

	// Check if memory usage has consistently increased
	windowSize := min(len(sm.memoryHistory), 50) // Use last 50 measurements
	recent := sm.memoryHistory[len(sm.memoryHistory)-windowSize:]

	// Calculate linear regression to detect trend
	trend := sm.calculateTrend(recent)

	// Convert to percentage increase per hour
	timeSinceBaseline := time.Since(sm.baselineTime).Hours()
	if timeSinceBaseline == 0 {
		return nil
	}

	currentMemory := recent[len(recent)-1]
	increasePercent := float64(currentMemory-sm.baselineMemory) / float64(sm.baselineMemory) * 100

	if increasePercent > sm.config.MemoryLeakThreshold {
		return fmt.Errorf("memory usage increased by %.2f%% (threshold: %.2f%%), trend: %.2f bytes/measurement",
			increasePercent, sm.config.MemoryLeakThreshold, trend)
	}

	return nil
}

// checkGoroutineLeak checks for goroutine leaks
func (sm *SystemMonitor) checkGoroutineLeak() error {
	numGoroutines := runtime.NumGoroutine()

	if numGoroutines > sm.config.MaxGoroutines {
		return fmt.Errorf("goroutine count (%d) exceeds threshold (%d)",
			numGoroutines, sm.config.MaxGoroutines)
	}

	return nil
}

// checkFileDescriptorLeak checks for file descriptor leaks
func (sm *SystemMonitor) checkFileDescriptorLeak() error {
	openFiles := sm.GetOpenFileCount()

	if openFiles > sm.config.MaxOpenFiles {
		return fmt.Errorf("open file count (%d) exceeds threshold (%d)",
			openFiles, sm.config.MaxOpenFiles)
	}

	return nil
}

// checkThresholds checks if current usage exceeds configured thresholds
func (sm *SystemMonitor) checkThresholds() error {
	// Check CPU threshold
	if len(sm.cpuHistory) > 0 {
		currentCPU := sm.cpuHistory[len(sm.cpuHistory)-1]
		if currentCPU > sm.config.MaxCPUPercent {
			return fmt.Errorf("CPU usage (%.2f%%) exceeds threshold (%.2f%%)",
				currentCPU, sm.config.MaxCPUPercent)
		}
	}

	// Check memory threshold
	if len(sm.memoryHistory) > 0 {
		currentMemoryMB := sm.memoryHistory[len(sm.memoryHistory)-1] / 1024 / 1024
		if currentMemoryMB > uint64(sm.config.MaxMemoryMB) {
			return fmt.Errorf("memory usage (%d MB) exceeds threshold (%d MB)",
				currentMemoryMB, sm.config.MaxMemoryMB)
		}
	}

	return nil
}

// calculateTrend calculates the trend (slope) of memory usage
func (sm *SystemMonitor) calculateTrend(values []uint64) float64 {
	n := len(values)
	if n < 2 {
		return 0
	}

	// Calculate linear regression slope
	var sumX, sumY, sumXY, sumXX float64
	for i, value := range values {
		x := float64(i)
		y := float64(value)
		sumX += x
		sumY += y
		sumXY += x * y
		sumXX += x * x
	}

	// Slope = (n*sumXY - sumX*sumY) / (n*sumXX - sumX*sumX)
	denominator := float64(n)*sumXX - sumX*sumX
	if denominator == 0 {
		return 0
	}

	slope := (float64(n)*sumXY - sumX*sumY) / denominator
	return slope
}

// GetCPUUsage returns current CPU usage
func (sm *SystemMonitor) GetCPUUsage() float64 {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if len(sm.cpuHistory) == 0 {
		return 0
	}
	return sm.cpuHistory[len(sm.cpuHistory)-1]
}

// GetMemoryUsage returns current memory usage in bytes
func (sm *SystemMonitor) GetMemoryUsage() uint64 {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if len(sm.memoryHistory) == 0 {
		return 0
	}
	return sm.memoryHistory[len(sm.memoryHistory)-1]
}

// GetOpenFileCount returns the number of open file descriptors
func (sm *SystemMonitor) GetOpenFileCount() int {
	// On Unix systems, count files in /proc/PID/fd/
	fdDir := fmt.Sprintf("/proc/%d/fd", sm.pid)
	entries, err := os.ReadDir(fdDir)
	if err != nil {
		// Fallback method for non-Unix systems
		return sm.getOpenFileCountFallback()
	}
	return len(entries)
}

// getOpenFileCountFallback provides fallback method for counting open files
func (sm *SystemMonitor) getOpenFileCountFallback() int {
	// This is a simplified fallback - in a real implementation,
	// you'd use platform-specific methods
	return 0
}

// GetConnectionCount returns the number of active network connections
func (sm *SystemMonitor) GetConnectionCount() int {
	connections, err := net.ConnectionsPid("all", sm.pid)
	if err != nil {
		sm.logger.WithError(err).Debug("Failed to get connection count")
		return 0
	}
	return len(connections)
}

// GetSystemInfo returns detailed system information
func (sm *SystemMonitor) GetSystemInfo() (*SystemInfo, error) {
	// Memory info
	vmStat, err := mem.VirtualMemory()
	if err != nil {
		return nil, fmt.Errorf("failed to get virtual memory info: %w", err)
	}

	// CPU info
	cpuStats, err := cpu.Info()
	if err != nil {
		return nil, fmt.Errorf("failed to get CPU info: %w", err)
	}

	// Process info
	procMemInfo, err := sm.process.MemoryInfo()
	if err != nil {
		return nil, fmt.Errorf("failed to get process memory info: %w", err)
	}

	procCPUPercent, err := sm.process.CPUPercent()
	if err != nil {
		return nil, fmt.Errorf("failed to get process CPU percent: %w", err)
	}

	return &SystemInfo{
		// System memory
		TotalMemoryMB:    vmStat.Total / 1024 / 1024,
		AvailableMemoryMB: vmStat.Available / 1024 / 1024,
		UsedMemoryMB:     vmStat.Used / 1024 / 1024,
		MemoryPercent:    vmStat.UsedPercent,

		// Process memory
		ProcessMemoryMB:  procMemInfo.RSS / 1024 / 1024,
		ProcessVMemoryMB: procMemInfo.VMS / 1024 / 1024,

		// CPU
		CPUCount:         len(cpuStats),
		ProcessCPUPercent: procCPUPercent,

		// Goroutines
		Goroutines:       runtime.NumGoroutine(),

		// File descriptors
		OpenFiles:        sm.GetOpenFileCount(),

		// Network connections
		Connections:      sm.GetConnectionCount(),

		// Uptime
		Uptime:           time.Since(sm.baselineTime),
	}, nil
}

// SystemInfo contains comprehensive system information
type SystemInfo struct {
	// Memory statistics
	TotalMemoryMB     uint64  `json:"total_memory_mb"`
	AvailableMemoryMB uint64  `json:"available_memory_mb"`
	UsedMemoryMB      uint64  `json:"used_memory_mb"`
	MemoryPercent     float64 `json:"memory_percent"`

	// Process memory
	ProcessMemoryMB   uint64  `json:"process_memory_mb"`
	ProcessVMemoryMB  uint64  `json:"process_vmemory_mb"`

	// CPU statistics
	CPUCount          int     `json:"cpu_count"`
	ProcessCPUPercent float64 `json:"process_cpu_percent"`

	// Resource counts
	Goroutines        int     `json:"goroutines"`
	OpenFiles         int     `json:"open_files"`
	Connections       int     `json:"connections"`

	// Uptime
	Uptime            time.Duration `json:"uptime"`
}

// DumpGoroutines dumps goroutine stack traces to a file
func (sm *SystemMonitor) DumpGoroutines(filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create goroutine dump file: %w", err)
	}
	defer file.Close()

	// Get stack traces for all goroutines
	buf := make([]byte, 1<<20) // 1MB buffer
	stackSize := runtime.Stack(buf, true)

	_, err = file.Write(buf[:stackSize])
	if err != nil {
		return fmt.Errorf("failed to write goroutine dump: %w", err)
	}

	sm.logger.WithField("file", filename).Info("Goroutine dump created")
	return nil
}

// DumpMemoryProfile dumps memory profile to a file
func (sm *SystemMonitor) DumpMemoryProfile(filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create memory profile file: %w", err)
	}
	defer file.Close()

	// Force garbage collection before profiling
	runtime.GC()

	// Write memory profile
	if err := runtime.WriteMemProfile(file); err != nil {
		return fmt.Errorf("failed to write memory profile: %w", err)
	}

	sm.logger.WithField("file", filename).Info("Memory profile created")
	return nil
}

// GetLoadAverage returns system load average on Unix systems
func (sm *SystemMonitor) GetLoadAverage() ([]float64, error) {
	// Read /proc/loadavg on Linux
	file, err := os.Open("/proc/loadavg")
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		return nil, fmt.Errorf("failed to read load average")
	}

	fields := strings.Fields(scanner.Text())
	if len(fields) < 3 {
		return nil, fmt.Errorf("invalid load average format")
	}

	loadAvg := make([]float64, 3)
	for i := 0; i < 3; i++ {
		loadAvg[i], err = strconv.ParseFloat(fields[i], 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse load average: %w", err)
		}
	}

	return loadAvg, nil
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}