//go:build darwin

package metrics

import (
	"os"
	"runtime"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"
)

// Darwin (macOS) metrics collection using gopsutil

func (c *Collector) collectCPU() (*CPUMetrics, error) {
	cpuMetrics := &CPUMetrics{
		Cores: runtime.NumCPU(),
	}

	// Overall CPU usage
	percentages, err := cpu.Percent(time.Second, false)
	if err == nil && len(percentages) > 0 {
		cpuMetrics.UsagePercent = percentages[0]
	}

	// Per-core CPU usage
	perCore, err := cpu.Percent(time.Second, true)
	if err == nil {
		cpuMetrics.PerCoreUsage = perCore
	}

	// CPU times
	times, err := cpu.Times(false)
	if err == nil && len(times) > 0 {
		total := times[0].User + times[0].System + times[0].Idle + times[0].Nice
		if total > 0 {
			cpuMetrics.UserPercent = (times[0].User / total) * 100
			cpuMetrics.SystemPercent = (times[0].System / total) * 100
			cpuMetrics.IdlePercent = (times[0].Idle / total) * 100
		}
	}

	// Load average - not available on macOS via gopsutil
	// Set to 0 for now - can be enhanced later using syscall
	cpuMetrics.LoadAvg1 = 0
	cpuMetrics.LoadAvg5 = 0
	cpuMetrics.LoadAvg15 = 0

	return cpuMetrics, nil
}

func (c *Collector) collectMemory() (*MemoryMetrics, error) {
	vmStat, err := mem.VirtualMemory()
	if err != nil {
		return nil, err
	}

	swap, _ := mem.SwapMemory()

	return &MemoryMetrics{
		Total:       vmStat.Total,
		Used:        vmStat.Used,
		Free:        vmStat.Free,
		Available:   vmStat.Available,
		UsedPercent: vmStat.UsedPercent,
		SwapTotal:   swap.Total,
		SwapUsed:    swap.Used,
		SwapFree:    swap.Free,
	}, nil
}

func (c *Collector) collectDisk() (*DiskMetrics, error) {
	usage, err := disk.Usage("/")
	if err != nil {
		return nil, err
	}

	return &DiskMetrics{
		Total:       usage.Total,
		Used:        usage.Used,
		Free:        usage.Free,
		UsedPercent: usage.UsedPercent,
		Partitions: []PartitionMetrics{
			{
				Device:      "/dev/disk1",
				Mountpoint:  "/",
				Fstype:      "apfs",
				Total:       usage.Total,
				Used:        usage.Used,
				Free:        usage.Free,
				UsedPercent: usage.UsedPercent,
			},
		},
	}, nil
}

func (c *Collector) collectNetwork() (*NetworkMetrics, error) {
	netMetrics := &NetworkMetrics{}

	ioCounters, err := net.IOCounters(false)
	if err == nil && len(ioCounters) > 0 {
		netMetrics.TotalSent = ioCounters[0].BytesSent
		netMetrics.TotalRecv = ioCounters[0].BytesRecv
	}

	return netMetrics, nil
}

func (c *Collector) collectProcess() (*ProcessMetrics, error) {
	proc, err := process.NewProcess(int32(os.Getpid()))
	if err != nil {
		return nil, err
	}

	cpuPercent, _ := proc.CPUPercent()
	memInfo, _ := proc.MemoryInfo()
	memPercent, _ := proc.MemoryPercent()

	// Get all processes count
	allProcs, _ := process.Processes()

	return &ProcessMetrics{
		Total:    int32(len(allProcs)),
		AgentPID: int32(os.Getpid()),
		AgentCPU: cpuPercent,
		AgentMem: memPercent,
		AgentRSS: memInfo.RSS,
		AgentVMS: memInfo.VMS,
	}, nil
}

func (c *Collector) collectSystem() (*SystemInfo, error) {
	hostInfo, err := host.Info()
	if err != nil {
		return nil, err
	}

	return &SystemInfo{
		OS:              hostInfo.OS,
		Platform:        hostInfo.Platform,
		PlatformVersion: hostInfo.PlatformVersion,
		Hostname:        hostInfo.Hostname,
		Uptime:          hostInfo.Uptime,
	}, nil
}

func (c *Collector) collectTemperature() (*TemperatureMetrics, error) {
	// Temperature sensors not readily available on macOS via gopsutil
	// Return nil to indicate no temperature data available
	return nil, nil
}
