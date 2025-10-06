package framework

import (
	"context"
	"encoding/json"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"
)

// MetricsCollector collects and aggregates performance metrics
type MetricsCollector struct {
	mu                 sync.RWMutex
	ctx                context.Context
	cancel             context.CancelFunc
	wg                 sync.WaitGroup
	logger             *slog.Logger
	collectionInterval time.Duration
	retentionPeriod    time.Duration

	// Metrics storage
	performanceMetrics []PerformanceMetrics
	latencyMetrics     []LatencyMetrics
	throughputMetrics  []ThroughputMetrics
	systemMetrics      []SystemMetrics
	errorMetrics       []ErrorMetrics

	// Aggregated metrics
	summaryMetrics MetricsSummary

	// Real-time metrics
	realTimeMetrics RealTimeMetrics

	// Callbacks for metrics events
	metricsCallbacks []MetricsCallback
}

// PerformanceMetrics represents system performance at a point in time
type PerformanceMetrics struct {
	Timestamp         time.Time              `json:"timestamp"`
	CPUUsage          float64                `json:"cpu_usage"`
	MemoryUsage       float64                `json:"memory_usage"`
	MemoryUsedBytes   uint64                 `json:"memory_used_bytes"`
	MemoryTotalBytes  uint64                 `json:"memory_total_bytes"`
	NetworkBytesIn    uint64                 `json:"network_bytes_in"`
	NetworkBytesOut   uint64                 `json:"network_bytes_out"`
	NetworkPacketsIn  uint64                 `json:"network_packets_in"`
	NetworkPacketsOut uint64                 `json:"network_packets_out"`
	ProcessCount      int                    `json:"process_count"`
	ThreadCount       int                    `json:"thread_count"`
	OpenConnections   int64                  `json:"open_connections"`
	Custom            map[string]interface{} `json:"custom,omitempty"`
}

// LatencyMetrics tracks latency measurements
type LatencyMetrics struct {
	Timestamp        time.Time     `json:"timestamp"`
	OperationType    string        `json:"operation_type"`
	Latency          time.Duration `json:"latency"`
	P50Latency       time.Duration `json:"p50_latency"`
	P95Latency       time.Duration `json:"p95_latency"`
	P99Latency       time.Duration `json:"p99_latency"`
	MaxLatency       time.Duration `json:"max_latency"`
	MinLatency       time.Duration `json:"min_latency"`
	MeasurementCount int64         `json:"measurement_count"`
}

// ThroughputMetrics tracks throughput measurements
type ThroughputMetrics struct {
	Timestamp             time.Time `json:"timestamp"`
	RequestsPerSecond     float64   `json:"requests_per_second"`
	MetricsPerSecond      float64   `json:"metrics_per_second"`
	HeartbeatsPerSecond   float64   `json:"heartbeats_per_second"`
	BytesPerSecond        uint64    `json:"bytes_per_second"`
	TransactionsPerSecond float64   `json:"transactions_per_second"`
}

// SystemMetrics tracks system-level metrics
type SystemMetrics struct {
	Timestamp       time.Time `json:"timestamp"`
	LoadAverage1    float64   `json:"load_average_1"`
	LoadAverage5    float64   `json:"load_average_5"`
	LoadAverage15   float64   `json:"load_average_15"`
	DiskUsage       float64   `json:"disk_usage"`
	DiskIOPS        float64   `json:"disk_iops"`
	NetworkIOPS     float64   `json:"network_iops"`
	ContextSwitches uint64    `json:"context_switches"`
	SystemCalls     uint64    `json:"system_calls"`
}

// ErrorMetrics tracks error rates and types
type ErrorMetrics struct {
	Timestamp        time.Time        `json:"timestamp"`
	TotalErrors      int64            `json:"total_errors"`
	ErrorRate        float64          `json:"error_rate"`
	ErrorsByType     map[string]int64 `json:"errors_by_type"`
	TimeoutErrors    int64            `json:"timeout_errors"`
	ConnectionErrors int64            `json:"connection_errors"`
	AuthErrors       int64            `json:"auth_errors"`
	ServerErrors     int64            `json:"server_errors"`
	ClientErrors     int64            `json:"client_errors"`
}

// MetricsSummary provides aggregated metrics over time periods
type MetricsSummary struct {
	StartTime time.Time     `json:"start_time"`
	EndTime   time.Time     `json:"end_time"`
	Duration  time.Duration `json:"duration"`

	// Performance summary
	AvgCPUUsage     float64 `json:"avg_cpu_usage"`
	MaxCPUUsage     float64 `json:"max_cpu_usage"`
	AvgMemoryUsage  float64 `json:"avg_memory_usage"`
	MaxMemoryUsage  float64 `json:"max_memory_usage"`
	PeakConnections int64   `json:"peak_connections"`

	// Latency summary
	AvgLatency time.Duration `json:"avg_latency"`
	P50Latency time.Duration `json:"p50_latency"`
	P95Latency time.Duration `json:"p95_latency"`
	P99Latency time.Duration `json:"p99_latency"`
	MaxLatency time.Duration `json:"max_latency"`

	// Throughput summary
	AvgThroughput  float64 `json:"avg_throughput"`
	PeakThroughput float64 `json:"peak_throughput"`
	TotalRequests  int64   `json:"total_requests"`

	// Error summary
	TotalErrors       int64            `json:"total_errors"`
	OverallErrorRate  float64          `json:"overall_error_rate"`
	ErrorDistribution map[string]int64 `json:"error_distribution"`

	// Resource utilization
	ResourceUsage ResourceUsageSummary `json:"resource_usage"`
}

// ResourceUsageSummary summarizes resource utilization
type ResourceUsageSummary struct {
	CPUEfficiency      float64 `json:"cpu_efficiency"`
	MemoryEfficiency   float64 `json:"memory_efficiency"`
	NetworkUtilization float64 `json:"network_utilization"`
	IOUtilization      float64 `json:"io_utilization"`
}

// RealTimeMetrics provides current real-time metrics
type RealTimeMetrics struct {
	mu                sync.RWMutex
	LastUpdated       time.Time     `json:"last_updated"`
	CurrentCPU        float64       `json:"current_cpu"`
	CurrentMemory     float64       `json:"current_memory"`
	CurrentThroughput float64       `json:"current_throughput"`
	CurrentLatency    time.Duration `json:"current_latency"`
	CurrentErrors     int64         `json:"current_errors"`
	ConnectionCount   int64         `json:"connection_count"`
	ActiveDevices     int64         `json:"active_devices"`
}

// MetricsCallback is called when metrics are collected
type MetricsCallback func(metrics interface{})

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector() *MetricsCollector {
	ctx, cancel := context.WithCancel(context.Background())

	logger := slog.Default().With("component", "metrics_collector")

	return &MetricsCollector{
		ctx:                ctx,
		cancel:             cancel,
		logger:             logger,
		collectionInterval: 5 * time.Second,
		retentionPeriod:    24 * time.Hour,
		performanceMetrics: make([]PerformanceMetrics, 0),
		latencyMetrics:     make([]LatencyMetrics, 0),
		throughputMetrics:  make([]ThroughputMetrics, 0),
		systemMetrics:      make([]SystemMetrics, 0),
		errorMetrics:       make([]ErrorMetrics, 0),
		metricsCallbacks:   make([]MetricsCallback, 0),
	}
}

// Start begins metrics collection
func (mc *MetricsCollector) Start() error {
	mc.logger.Info("Starting metrics collection",
		"collection_interval", mc.collectionInterval,
		"retention_period", mc.retentionPeriod,
	)

	mc.wg.Add(4)
	go mc.collectPerformanceMetrics()
	go mc.collectSystemMetrics()
	go mc.cleanupOldMetrics()
	go mc.updateRealTimeMetrics()

	return nil
}

// Stop stops metrics collection
func (mc *MetricsCollector) Stop() error {
	mc.logger.Info("Stopping metrics collection")

	mc.cancel()
	mc.wg.Wait()

	return nil
}

// SetCollectionInterval sets how often metrics are collected
func (mc *MetricsCollector) SetCollectionInterval(interval time.Duration) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.collectionInterval = interval
}

// SetRetentionPeriod sets how long metrics are retained
func (mc *MetricsCollector) SetRetentionPeriod(period time.Duration) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.retentionPeriod = period
}

// AddMetricsCallback adds a callback for metrics events
func (mc *MetricsCollector) AddMetricsCallback(callback MetricsCallback) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.metricsCallbacks = append(mc.metricsCallbacks, callback)
}

// collectPerformanceMetrics collects system performance metrics
func (mc *MetricsCollector) collectPerformanceMetrics() {
	defer mc.wg.Done()

	ticker := time.NewTicker(mc.collectionInterval)
	defer ticker.Stop()

	for {
		select {
		case <-mc.ctx.Done():
			return
		case <-ticker.C:
			metrics := mc.gatherPerformanceMetrics()

			mc.mu.Lock()
			mc.performanceMetrics = append(mc.performanceMetrics, metrics)
			mc.mu.Unlock()

			// Notify callbacks
			mc.notifyCallbacks(metrics)
		}
	}
}

// collectSystemMetrics collects system-level metrics
func (mc *MetricsCollector) collectSystemMetrics() {
	defer mc.wg.Done()

	ticker := time.NewTicker(mc.collectionInterval)
	defer ticker.Stop()

	for {
		select {
		case <-mc.ctx.Done():
			return
		case <-ticker.C:
			metrics := mc.gatherSystemMetrics()

			mc.mu.Lock()
			mc.systemMetrics = append(mc.systemMetrics, metrics)
			mc.mu.Unlock()

			// Notify callbacks
			mc.notifyCallbacks(metrics)
		}
	}
}

// updateRealTimeMetrics updates real-time metrics display
func (mc *MetricsCollector) updateRealTimeMetrics() {
	defer mc.wg.Done()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-mc.ctx.Done():
			return
		case <-ticker.C:
			mc.refreshRealTimeMetrics()
		}
	}
}

// gatherPerformanceMetrics collects current performance metrics
func (mc *MetricsCollector) gatherPerformanceMetrics() PerformanceMetrics {
	metrics := PerformanceMetrics{
		Timestamp: time.Now(),
		Custom:    make(map[string]interface{}),
	}

	// CPU usage
	if cpuPercent, err := cpu.Percent(0, false); err == nil && len(cpuPercent) > 0 {
		metrics.CPUUsage = cpuPercent[0]
	}

	// Memory usage
	if memInfo, err := mem.VirtualMemory(); err == nil {
		metrics.MemoryUsage = memInfo.UsedPercent
		metrics.MemoryUsedBytes = memInfo.Used
		metrics.MemoryTotalBytes = memInfo.Total
	}

	// Network statistics
	if netStats, err := net.IOCounters(false); err == nil && len(netStats) > 0 {
		metrics.NetworkBytesIn = netStats[0].BytesRecv
		metrics.NetworkBytesOut = netStats[0].BytesSent
		metrics.NetworkPacketsIn = netStats[0].PacketsRecv
		metrics.NetworkPacketsOut = netStats[0].PacketsSent
	}

	// Process count
	if processes, err := process.Processes(); err == nil {
		metrics.ProcessCount = len(processes)

		// Count threads
		threadCount := 0
		for _, p := range processes {
			if numThreads, err := p.NumThreads(); err == nil {
				threadCount += int(numThreads)
			}
		}
		metrics.ThreadCount = threadCount
	}

	return metrics
}

// gatherSystemMetrics collects system-level metrics
func (mc *MetricsCollector) gatherSystemMetrics() SystemMetrics {
	metrics := SystemMetrics{
		Timestamp: time.Now(),
	}

	// Load averages (Linux/macOS)
	if loadInfo, err := load.Avg(); err == nil {
		metrics.LoadAverage1 = loadInfo.Load1
		metrics.LoadAverage5 = loadInfo.Load5
		metrics.LoadAverage15 = loadInfo.Load15
	}

	return metrics
}

// refreshRealTimeMetrics updates the real-time metrics
func (mc *MetricsCollector) refreshRealTimeMetrics() {
	mc.realTimeMetrics.mu.Lock()
	defer mc.realTimeMetrics.mu.Unlock()

	mc.realTimeMetrics.LastUpdated = time.Now()

	// Get latest performance metrics
	mc.mu.RLock()
	if len(mc.performanceMetrics) > 0 {
		latest := mc.performanceMetrics[len(mc.performanceMetrics)-1]
		mc.realTimeMetrics.CurrentCPU = latest.CPUUsage
		mc.realTimeMetrics.CurrentMemory = latest.MemoryUsage
		mc.realTimeMetrics.ConnectionCount = latest.OpenConnections
	}

	// Get latest throughput metrics
	if len(mc.throughputMetrics) > 0 {
		latest := mc.throughputMetrics[len(mc.throughputMetrics)-1]
		mc.realTimeMetrics.CurrentThroughput = latest.RequestsPerSecond
	}

	// Get latest latency metrics
	if len(mc.latencyMetrics) > 0 {
		latest := mc.latencyMetrics[len(mc.latencyMetrics)-1]
		mc.realTimeMetrics.CurrentLatency = latest.P95Latency
	}

	// Get latest error metrics
	if len(mc.errorMetrics) > 0 {
		latest := mc.errorMetrics[len(mc.errorMetrics)-1]
		mc.realTimeMetrics.CurrentErrors = latest.TotalErrors
	}
	mc.mu.RUnlock()
}

// cleanupOldMetrics removes old metrics to prevent memory leaks
func (mc *MetricsCollector) cleanupOldMetrics() {
	defer mc.wg.Done()

	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-mc.ctx.Done():
			return
		case <-ticker.C:
			mc.performCleanup()
		}
	}
}

// performCleanup removes metrics older than retention period
func (mc *MetricsCollector) performCleanup() {
	cutoff := time.Now().Add(-mc.retentionPeriod)

	mc.mu.Lock()
	defer mc.mu.Unlock()

	// Clean performance metrics
	mc.performanceMetrics = mc.filterMetricsByTime(mc.performanceMetrics, cutoff)

	// Clean latency metrics
	mc.latencyMetrics = mc.filterLatencyMetricsByTime(mc.latencyMetrics, cutoff)

	// Clean throughput metrics
	mc.throughputMetrics = mc.filterThroughputMetricsByTime(mc.throughputMetrics, cutoff)

	// Clean system metrics
	mc.systemMetrics = mc.filterSystemMetricsByTime(mc.systemMetrics, cutoff)

	// Clean error metrics
	mc.errorMetrics = mc.filterErrorMetricsByTime(mc.errorMetrics, cutoff)

	mc.logger.Debug("Cleaned up old metrics", "cutoff_time", cutoff)
}

// filterMetricsByTime filters performance metrics by timestamp
func (mc *MetricsCollector) filterMetricsByTime(metrics []PerformanceMetrics, cutoff time.Time) []PerformanceMetrics {
	var filtered []PerformanceMetrics
	for _, m := range metrics {
		if m.Timestamp.After(cutoff) {
			filtered = append(filtered, m)
		}
	}
	return filtered
}

// filterLatencyMetricsByTime filters latency metrics by timestamp
func (mc *MetricsCollector) filterLatencyMetricsByTime(metrics []LatencyMetrics, cutoff time.Time) []LatencyMetrics {
	var filtered []LatencyMetrics
	for _, m := range metrics {
		if m.Timestamp.After(cutoff) {
			filtered = append(filtered, m)
		}
	}
	return filtered
}

// filterThroughputMetricsByTime filters throughput metrics by timestamp
func (mc *MetricsCollector) filterThroughputMetricsByTime(metrics []ThroughputMetrics, cutoff time.Time) []ThroughputMetrics {
	var filtered []ThroughputMetrics
	for _, m := range metrics {
		if m.Timestamp.After(cutoff) {
			filtered = append(filtered, m)
		}
	}
	return filtered
}

// filterSystemMetricsByTime filters system metrics by timestamp
func (mc *MetricsCollector) filterSystemMetricsByTime(metrics []SystemMetrics, cutoff time.Time) []SystemMetrics {
	var filtered []SystemMetrics
	for _, m := range metrics {
		if m.Timestamp.After(cutoff) {
			filtered = append(filtered, m)
		}
	}
	return filtered
}

// filterErrorMetricsByTime filters error metrics by timestamp
func (mc *MetricsCollector) filterErrorMetricsByTime(metrics []ErrorMetrics, cutoff time.Time) []ErrorMetrics {
	var filtered []ErrorMetrics
	for _, m := range metrics {
		if m.Timestamp.After(cutoff) {
			filtered = append(filtered, m)
		}
	}
	return filtered
}

// notifyCallbacks calls all registered callbacks with the metrics
func (mc *MetricsCollector) notifyCallbacks(metrics interface{}) {
	mc.mu.RLock()
	callbacks := make([]MetricsCallback, len(mc.metricsCallbacks))
	copy(callbacks, mc.metricsCallbacks)
	mc.mu.RUnlock()

	for _, callback := range callbacks {
		go func(cb MetricsCallback) {
			defer func() {
				if r := recover(); r != nil {
					mc.logger.Error("Metrics callback panicked", "error", r)
				}
			}()
			cb(metrics)
		}(callback)
	}
}

// RecordLatency records a latency measurement
func (mc *MetricsCollector) RecordLatency(operationType string, latency time.Duration) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	// Find or create latency metrics for this operation type
	now := time.Now()
	var latencyMetric *LatencyMetrics

	// Look for recent metrics for this operation type
	for i := len(mc.latencyMetrics) - 1; i >= 0; i-- {
		if mc.latencyMetrics[i].OperationType == operationType &&
			now.Sub(mc.latencyMetrics[i].Timestamp) < mc.collectionInterval {
			latencyMetric = &mc.latencyMetrics[i]
			break
		}
	}

	// Create new metrics if not found
	if latencyMetric == nil {
		mc.latencyMetrics = append(mc.latencyMetrics, LatencyMetrics{
			Timestamp:        now,
			OperationType:    operationType,
			Latency:          latency,
			MinLatency:       latency,
			MaxLatency:       latency,
			MeasurementCount: 1,
		})
		return
	}

	// Update existing metrics
	latencyMetric.MeasurementCount++
	if latency < latencyMetric.MinLatency {
		latencyMetric.MinLatency = latency
	}
	if latency > latencyMetric.MaxLatency {
		latencyMetric.MaxLatency = latency
	}
}

// RecordThroughput records throughput measurements
func (mc *MetricsCollector) RecordThroughput(requestsPerSec, metricsPerSec, heartbeatsPerSec float64, bytesPerSec uint64) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	metrics := ThroughputMetrics{
		Timestamp:           time.Now(),
		RequestsPerSecond:   requestsPerSec,
		MetricsPerSecond:    metricsPerSec,
		HeartbeatsPerSecond: heartbeatsPerSec,
		BytesPerSecond:      bytesPerSec,
	}

	mc.throughputMetrics = append(mc.throughputMetrics, metrics)

	// Notify callbacks
	mc.notifyCallbacks(metrics)
}

// RecordErrors records error metrics
func (mc *MetricsCollector) RecordErrors(totalErrors int64, errorsByType map[string]int64, totalRequests int64) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	var errorRate float64
	if totalRequests > 0 {
		errorRate = float64(totalErrors) / float64(totalRequests)
	}

	metrics := ErrorMetrics{
		Timestamp:    time.Now(),
		TotalErrors:  totalErrors,
		ErrorRate:    errorRate,
		ErrorsByType: make(map[string]int64),
	}

	// Copy error types
	for errorType, count := range errorsByType {
		metrics.ErrorsByType[errorType] = count

		// Categorize errors
		switch errorType {
		case "timeout":
			metrics.TimeoutErrors = count
		case "connection":
			metrics.ConnectionErrors = count
		case "auth":
			metrics.AuthErrors = count
		case "server":
			metrics.ServerErrors = count
		case "client":
			metrics.ClientErrors = count
		}
	}

	mc.errorMetrics = append(mc.errorMetrics, metrics)

	// Notify callbacks
	mc.notifyCallbacks(metrics)
}

// GetSummary generates a summary of all collected metrics
func (mc *MetricsCollector) GetSummary() MetricsSummary {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	summary := MetricsSummary{
		ErrorDistribution: make(map[string]int64),
	}

	// Determine time range
	if len(mc.performanceMetrics) > 0 {
		summary.StartTime = mc.performanceMetrics[0].Timestamp
		summary.EndTime = mc.performanceMetrics[len(mc.performanceMetrics)-1].Timestamp
		summary.Duration = summary.EndTime.Sub(summary.StartTime)
	}

	// Calculate performance summary
	mc.calculatePerformanceSummary(&summary)

	// Calculate latency summary
	mc.calculateLatencySummary(&summary)

	// Calculate throughput summary
	mc.calculateThroughputSummary(&summary)

	// Calculate error summary
	mc.calculateErrorSummary(&summary)

	// Calculate resource usage summary
	mc.calculateResourceUsageSummary(&summary)

	return summary
}

// calculatePerformanceSummary calculates performance metrics summary
func (mc *MetricsCollector) calculatePerformanceSummary(summary *MetricsSummary) {
	if len(mc.performanceMetrics) == 0 {
		return
	}

	var totalCPU, totalMemory float64
	var maxCPU, maxMemory float64
	var maxConnections int64

	for _, m := range mc.performanceMetrics {
		totalCPU += m.CPUUsage
		totalMemory += m.MemoryUsage

		if m.CPUUsage > maxCPU {
			maxCPU = m.CPUUsage
		}
		if m.MemoryUsage > maxMemory {
			maxMemory = m.MemoryUsage
		}
		if m.OpenConnections > maxConnections {
			maxConnections = m.OpenConnections
		}
	}

	count := float64(len(mc.performanceMetrics))
	summary.AvgCPUUsage = totalCPU / count
	summary.MaxCPUUsage = maxCPU
	summary.AvgMemoryUsage = totalMemory / count
	summary.MaxMemoryUsage = maxMemory
	summary.PeakConnections = maxConnections
}

// calculateLatencySummary calculates latency metrics summary
func (mc *MetricsCollector) calculateLatencySummary(summary *MetricsSummary) {
	if len(mc.latencyMetrics) == 0 {
		return
	}

	var allLatencies []time.Duration
	var totalLatency time.Duration
	var maxLatency time.Duration

	for _, m := range mc.latencyMetrics {
		allLatencies = append(allLatencies, m.Latency)
		totalLatency += m.Latency
		if m.MaxLatency > maxLatency {
			maxLatency = m.MaxLatency
		}
	}

	if len(allLatencies) > 0 {
		summary.AvgLatency = totalLatency / time.Duration(len(allLatencies))
		summary.MaxLatency = maxLatency

		// Calculate percentiles
		sort.Slice(allLatencies, func(i, j int) bool {
			return allLatencies[i] < allLatencies[j]
		})

		summary.P50Latency = allLatencies[len(allLatencies)/2]
		summary.P95Latency = allLatencies[int(float64(len(allLatencies))*0.95)]
		summary.P99Latency = allLatencies[int(float64(len(allLatencies))*0.99)]
	}
}

// calculateThroughputSummary calculates throughput metrics summary
func (mc *MetricsCollector) calculateThroughputSummary(summary *MetricsSummary) {
	if len(mc.throughputMetrics) == 0 {
		return
	}

	var totalThroughput float64
	var maxThroughput float64

	for _, m := range mc.throughputMetrics {
		totalThroughput += m.RequestsPerSecond
		if m.RequestsPerSecond > maxThroughput {
			maxThroughput = m.RequestsPerSecond
		}
	}

	summary.AvgThroughput = totalThroughput / float64(len(mc.throughputMetrics))
	summary.PeakThroughput = maxThroughput
}

// calculateErrorSummary calculates error metrics summary
func (mc *MetricsCollector) calculateErrorSummary(summary *MetricsSummary) {
	if len(mc.errorMetrics) == 0 {
		return
	}

	var totalErrors int64
	var totalErrorRate float64
	errorCounts := make(map[string]int64)

	for _, m := range mc.errorMetrics {
		totalErrors += m.TotalErrors
		totalErrorRate += m.ErrorRate

		for errorType, count := range m.ErrorsByType {
			errorCounts[errorType] += count
		}
	}

	summary.TotalErrors = totalErrors
	summary.OverallErrorRate = totalErrorRate / float64(len(mc.errorMetrics))
	summary.ErrorDistribution = errorCounts
}

// calculateResourceUsageSummary calculates resource usage summary
func (mc *MetricsCollector) calculateResourceUsageSummary(summary *MetricsSummary) {
	// This is a simplified calculation
	summary.ResourceUsage = ResourceUsageSummary{
		CPUEfficiency:      summary.AvgCPUUsage / 100.0,
		MemoryEfficiency:   summary.AvgMemoryUsage / 100.0,
		NetworkUtilization: 0.5, // Placeholder
		IOUtilization:      0.3, // Placeholder
	}
}

// GetRealTimeMetrics returns current real-time metrics
func (mc *MetricsCollector) GetRealTimeMetrics() *RealTimeMetrics {
	mc.realTimeMetrics.mu.RLock()
	defer mc.realTimeMetrics.mu.RUnlock()
	return &RealTimeMetrics{
		LastUpdated:       mc.realTimeMetrics.LastUpdated,
		CurrentCPU:        mc.realTimeMetrics.CurrentCPU,
		CurrentMemory:     mc.realTimeMetrics.CurrentMemory,
		CurrentThroughput: mc.realTimeMetrics.CurrentThroughput,
		CurrentLatency:    mc.realTimeMetrics.CurrentLatency,
		CurrentErrors:     mc.realTimeMetrics.CurrentErrors,
		ConnectionCount:   mc.realTimeMetrics.ConnectionCount,
		ActiveDevices:     mc.realTimeMetrics.ActiveDevices,
	}
}

// ExportMetrics exports all metrics to JSON
func (mc *MetricsCollector) ExportMetrics() ([]byte, error) {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	export := struct {
		Summary            MetricsSummary       `json:"summary"`
		PerformanceMetrics []PerformanceMetrics `json:"performance_metrics"`
		LatencyMetrics     []LatencyMetrics     `json:"latency_metrics"`
		ThroughputMetrics  []ThroughputMetrics  `json:"throughput_metrics"`
		SystemMetrics      []SystemMetrics      `json:"system_metrics"`
		ErrorMetrics       []ErrorMetrics       `json:"error_metrics"`
	}{
		Summary:            mc.GetSummary(),
		PerformanceMetrics: mc.performanceMetrics,
		LatencyMetrics:     mc.latencyMetrics,
		ThroughputMetrics:  mc.throughputMetrics,
		SystemMetrics:      mc.systemMetrics,
		ErrorMetrics:       mc.errorMetrics,
	}

	return json.MarshalIndent(export, "", "  ")
}

// SetActiveDevices updates the active device count for real-time metrics
func (mc *MetricsCollector) SetActiveDevices(count int64) {
	mc.realTimeMetrics.mu.Lock()
	defer mc.realTimeMetrics.mu.Unlock()
	mc.realTimeMetrics.ActiveDevices = count
}

// SetConnectionCount updates the connection count for real-time metrics
func (mc *MetricsCollector) SetConnectionCount(count int64) {
	mc.realTimeMetrics.mu.Lock()
	defer mc.realTimeMetrics.mu.Unlock()
	mc.realTimeMetrics.ConnectionCount = count
}
