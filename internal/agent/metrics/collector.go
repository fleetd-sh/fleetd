package metrics

import (
	"time"
)

// SystemMetrics contains comprehensive system metrics
type SystemMetrics struct {
	Timestamp   time.Time              `json:"timestamp"`
	CPU         CPUMetrics             `json:"cpu"`
	Memory      MemoryMetrics          `json:"memory"`
	Disk        DiskMetrics            `json:"disk"`
	Network     NetworkMetrics         `json:"network"`
	Process     ProcessMetrics         `json:"process"`
	System      SystemInfo             `json:"system"`
	Temperature *TemperatureMetrics    `json:"temperature,omitempty"`
	Custom      map[string]interface{} `json:"custom,omitempty"`
}

// CPUMetrics contains CPU-related metrics
type CPUMetrics struct {
	UsagePercent  float64   `json:"usage_percent"`
	UserPercent   float64   `json:"user_percent"`
	SystemPercent float64   `json:"system_percent"`
	IdlePercent   float64   `json:"idle_percent"`
	LoadAvg1      float64   `json:"load_avg_1"`
	LoadAvg5      float64   `json:"load_avg_5"`
	LoadAvg15     float64   `json:"load_avg_15"`
	Cores         int       `json:"cores"`
	PerCoreUsage  []float64 `json:"per_core_usage,omitempty"`
}

// MemoryMetrics contains memory-related metrics
type MemoryMetrics struct {
	Total       uint64  `json:"total"`
	Used        uint64  `json:"used"`
	Free        uint64  `json:"free"`
	Available   uint64  `json:"available"`
	UsedPercent float64 `json:"used_percent"`
	SwapTotal   uint64  `json:"swap_total"`
	SwapUsed    uint64  `json:"swap_used"`
	SwapFree    uint64  `json:"swap_free"`
	SwapPercent float64 `json:"swap_percent"`
	Cached      uint64  `json:"cached"`
	Buffers     uint64  `json:"buffers"`
}

// DiskMetrics contains disk-related metrics
type DiskMetrics struct {
	Total       uint64             `json:"total"`
	Used        uint64             `json:"used"`
	Free        uint64             `json:"free"`
	UsedPercent float64            `json:"used_percent"`
	Partitions  []PartitionMetrics `json:"partitions"`
	IOCounters  map[string]IOStats `json:"io_counters,omitempty"`
}

// PartitionMetrics contains metrics for a disk partition
type PartitionMetrics struct {
	Device      string  `json:"device"`
	Mountpoint  string  `json:"mountpoint"`
	Fstype      string  `json:"fstype"`
	Total       uint64  `json:"total"`
	Used        uint64  `json:"used"`
	Free        uint64  `json:"free"`
	UsedPercent float64 `json:"used_percent"`
}

// IOStats contains I/O statistics
type IOStats struct {
	ReadCount  uint64 `json:"read_count"`
	WriteCount uint64 `json:"write_count"`
	ReadBytes  uint64 `json:"read_bytes"`
	WriteBytes uint64 `json:"write_bytes"`
	ReadTime   uint64 `json:"read_time_ms"`
	WriteTime  uint64 `json:"write_time_ms"`
}

// NetworkMetrics contains network-related metrics
type NetworkMetrics struct {
	Interfaces []InterfaceMetrics `json:"interfaces"`
	TotalSent  uint64             `json:"total_sent"`
	TotalRecv  uint64             `json:"total_recv"`
}

// InterfaceMetrics contains metrics for a network interface
type InterfaceMetrics struct {
	Name        string `json:"name"`
	BytesSent   uint64 `json:"bytes_sent"`
	BytesRecv   uint64 `json:"bytes_recv"`
	PacketsSent uint64 `json:"packets_sent"`
	PacketsRecv uint64 `json:"packets_recv"`
	ErrorsIn    uint64 `json:"errors_in"`
	ErrorsOut   uint64 `json:"errors_out"`
	DropsIn     uint64 `json:"drops_in"`
	DropsOut    uint64 `json:"drops_out"`
}

// ProcessMetrics contains process-related metrics
type ProcessMetrics struct {
	Total    int32   `json:"total"`
	Running  int32   `json:"running"`
	Sleeping int32   `json:"sleeping"`
	Stopped  int32   `json:"stopped"`
	Zombie   int32   `json:"zombie"`
	AgentPID int32   `json:"agent_pid"`
	AgentCPU float64 `json:"agent_cpu_percent"`
	AgentMem float32 `json:"agent_mem_percent"`
	AgentRSS uint64  `json:"agent_rss"`
	AgentVMS uint64  `json:"agent_vms"`
}

// SystemInfo contains system information
type SystemInfo struct {
	Hostname        string `json:"hostname"`
	Uptime          uint64 `json:"uptime"`
	OS              string `json:"os"`
	Platform        string `json:"platform"`
	PlatformVersion string `json:"platform_version"`
	KernelVersion   string `json:"kernel_version"`
	Architecture    string `json:"architecture"`
}

// TemperatureMetrics contains temperature sensors data
type TemperatureMetrics struct {
	CPU     float64            `json:"cpu,omitempty"`
	GPU     float64            `json:"gpu,omitempty"`
	Sensors map[string]float64 `json:"sensors,omitempty"`
}

// Collector collects system metrics
type Collector struct {
	lastNetStats  map[string]interface{}
	lastDiskStats map[string]interface{}
}

// NewCollector creates a new metrics collector
func NewCollector() *Collector {
	return &Collector{
		lastNetStats:  make(map[string]interface{}),
		lastDiskStats: make(map[string]interface{}),
	}
}

// Collect gathers all system metrics
func (c *Collector) Collect() (*SystemMetrics, error) {
	metrics := &SystemMetrics{
		Timestamp: time.Now(),
	}

	// Collect CPU metrics
	if cpuMetrics, err := c.collectCPU(); err == nil {
		metrics.CPU = *cpuMetrics
	}

	// Collect Memory metrics
	if memMetrics, err := c.collectMemory(); err == nil {
		metrics.Memory = *memMetrics
	}

	// Collect Disk metrics
	if diskMetrics, err := c.collectDisk(); err == nil {
		metrics.Disk = *diskMetrics
	}

	// Collect Network metrics
	if netMetrics, err := c.collectNetwork(); err == nil {
		metrics.Network = *netMetrics
	}

	// Collect Process metrics
	if procMetrics, err := c.collectProcess(); err == nil {
		metrics.Process = *procMetrics
	}

	// Collect System info
	if sysInfo, err := c.collectSystem(); err == nil {
		metrics.System = *sysInfo
	}

	// Collect Temperature (if available)
	if tempMetrics, err := c.collectTemperature(); err == nil && tempMetrics != nil {
		metrics.Temperature = tempMetrics
	}

	return metrics, nil
}

// Platform-specific collection methods are implemented in:
// - collector_windows.go for Windows
// - collector_darwin.go for macOS
// - collector_generic.go for Linux and other Unix-like systems


// Utility functions

// GetDiskUsageForPath gets disk usage for a specific path
func GetDiskUsageForPath(path string) (map[string]uint64, error) {
	// Platform-specific implementation would be needed
	return nil, nil
}

// GetNetworkLatency measures network latency to a host
func GetNetworkLatency(host string) (time.Duration, error) {
	// Platform-specific implementation would be needed
	return 0, nil
}