//go:build !windows && !darwin

package metrics

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"
)

// Generic (Linux and other Unix-like) metrics collection

// collectCPUGeneric collects CPU metrics using gopsutil
func (c *Collector) collectCPUGeneric() (*CPUMetrics, error) {
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
		total := times[0].User + times[0].System + times[0].Idle + times[0].Nice +
			times[0].Iowait + times[0].Irq + times[0].Softirq + times[0].Steal

		if total > 0 {
			cpuMetrics.UserPercent = (times[0].User / total) * 100
			cpuMetrics.SystemPercent = (times[0].System / total) * 100
			cpuMetrics.IdlePercent = (times[0].Idle / total) * 100
		}
	}

	// Load average (Unix-like systems)
	if runtime.GOOS != "windows" {
		if loadAvg, err := getLoadAverage(); err == nil {
			cpuMetrics.LoadAvg1 = loadAvg[0]
			cpuMetrics.LoadAvg5 = loadAvg[1]
			cpuMetrics.LoadAvg15 = loadAvg[2]
		}
	}

	return cpuMetrics, nil
}

// collectMemoryGeneric collects memory metrics using gopsutil
func (c *Collector) collectMemoryGeneric() (*MemoryMetrics, error) {
	memMetrics := &MemoryMetrics{}

	// Virtual memory
	vmStat, err := mem.VirtualMemory()
	if err != nil {
		return nil, err
	}

	memMetrics.Total = vmStat.Total
	memMetrics.Used = vmStat.Used
	memMetrics.Free = vmStat.Free
	memMetrics.Available = vmStat.Available
	memMetrics.UsedPercent = vmStat.UsedPercent
	memMetrics.Cached = vmStat.Cached
	memMetrics.Buffers = vmStat.Buffers

	// Swap memory
	swapStat, err := mem.SwapMemory()
	if err == nil {
		memMetrics.SwapTotal = swapStat.Total
		memMetrics.SwapUsed = swapStat.Used
		memMetrics.SwapFree = swapStat.Free
		memMetrics.SwapPercent = swapStat.UsedPercent
	}

	return memMetrics, nil
}

// collectDiskGeneric collects disk metrics using gopsutil
func (c *Collector) collectDiskGeneric() (*DiskMetrics, error) {
	diskMetrics := &DiskMetrics{
		Partitions: []PartitionMetrics{},
		IOCounters: make(map[string]IOStats),
	}

	// Disk partitions
	partitions, err := disk.Partitions(false)
	if err != nil {
		return nil, err
	}

	var totalUsed, totalFree uint64
	for _, partition := range partitions {
		usage, err := disk.Usage(partition.Mountpoint)
		if err != nil {
			continue
		}

		partMetric := PartitionMetrics{
			Device:      partition.Device,
			Mountpoint:  partition.Mountpoint,
			Fstype:      partition.Fstype,
			Total:       usage.Total,
			Used:        usage.Used,
			Free:        usage.Free,
			UsedPercent: usage.UsedPercent,
		}

		diskMetrics.Partitions = append(diskMetrics.Partitions, partMetric)

		// Accumulate totals (only for main partitions)
		if partition.Mountpoint == "/" || (runtime.GOOS == "windows" && strings.HasSuffix(partition.Mountpoint, ":\\")) {
			diskMetrics.Total = usage.Total
			diskMetrics.Used = usage.Used
			diskMetrics.Free = usage.Free
			diskMetrics.UsedPercent = usage.UsedPercent
		}

		totalUsed += usage.Used
		totalFree += usage.Free
	}

	// I/O counters
	ioCounters, err := disk.IOCounters()
	if err == nil {
		for name, counter := range ioCounters {
			diskMetrics.IOCounters[name] = IOStats{
				ReadCount:  counter.ReadCount,
				WriteCount: counter.WriteCount,
				ReadBytes:  counter.ReadBytes,
				WriteBytes: counter.WriteBytes,
				ReadTime:   counter.ReadTime,
				WriteTime:  counter.WriteTime,
			}
		}
	}

	return diskMetrics, nil
}

// collectNetworkGeneric collects network metrics using gopsutil
func (c *Collector) collectNetworkGeneric() (*NetworkMetrics, error) {
	netMetrics := &NetworkMetrics{
		Interfaces: []InterfaceMetrics{},
	}

	// Network I/O counters
	ioCounters, err := net.IOCounters(true)
	if err != nil {
		return nil, err
	}

	for _, counter := range ioCounters {
		// Skip loopback interface
		if counter.Name == "lo" || strings.HasPrefix(counter.Name, "lo") {
			continue
		}

		ifaceMetric := InterfaceMetrics{
			Name:        counter.Name,
			BytesSent:   counter.BytesSent,
			BytesRecv:   counter.BytesRecv,
			PacketsSent: counter.PacketsSent,
			PacketsRecv: counter.PacketsRecv,
			ErrorsIn:    counter.Errin,
			ErrorsOut:   counter.Errout,
			DropsIn:     counter.Dropin,
			DropsOut:    counter.Dropout,
		}

		netMetrics.Interfaces = append(netMetrics.Interfaces, ifaceMetric)
		netMetrics.TotalSent += counter.BytesSent
		netMetrics.TotalRecv += counter.BytesRecv
	}

	return netMetrics, nil
}

// collectProcessGeneric collects process metrics using gopsutil
func (c *Collector) collectProcessGeneric() (*ProcessMetrics, error) {
	procMetrics := &ProcessMetrics{
		AgentPID: int32(os.Getpid()),
	}

	// Get all processes
	processes, err := process.Processes()
	if err == nil {
		procMetrics.Total = int32(len(processes))

		for _, p := range processes {
			status, err := p.Status()
			if err != nil {
				continue
			}

			switch status[0] {
			case "R", "running":
				procMetrics.Running++
			case "S", "sleeping":
				procMetrics.Sleeping++
			case "T", "stopped":
				procMetrics.Stopped++
			case "Z", "zombie":
				procMetrics.Zombie++
			}
		}
	}

	// Get agent process metrics
	agentProc, err := process.NewProcess(procMetrics.AgentPID)
	if err == nil {
		if cpuPercent, err := agentProc.CPUPercent(); err == nil {
			procMetrics.AgentCPU = cpuPercent
		}

		if memPercent, err := agentProc.MemoryPercent(); err == nil {
			procMetrics.AgentMem = memPercent
		}

		if memInfo, err := agentProc.MemoryInfo(); err == nil {
			procMetrics.AgentRSS = memInfo.RSS
			procMetrics.AgentVMS = memInfo.VMS
		}
	}

	return procMetrics, nil
}

// collectSystemGeneric collects system information using gopsutil
func (c *Collector) collectSystemGeneric() (*SystemInfo, error) {
	sysInfo := &SystemInfo{
		OS:           runtime.GOOS,
		Architecture: runtime.GOARCH,
	}

	// Host info
	hostInfo, err := host.Info()
	if err == nil {
		sysInfo.Hostname = hostInfo.Hostname
		sysInfo.Uptime = hostInfo.Uptime
		sysInfo.Platform = hostInfo.Platform
		sysInfo.PlatformVersion = hostInfo.PlatformVersion
		sysInfo.KernelVersion = hostInfo.KernelVersion
	}

	return sysInfo, nil
}

// collectTemperatureGeneric collects temperature metrics (Linux with thermal zones)
func (c *Collector) collectTemperatureGeneric() (*TemperatureMetrics, error) {
	if runtime.GOOS != "linux" {
		return nil, nil
	}

	temps := &TemperatureMetrics{
		Sensors: make(map[string]float64),
	}

	// Try to read CPU temperature from thermal zones
	thermalZone := "/sys/class/thermal/thermal_zone0/temp"
	if data, err := os.ReadFile(thermalZone); err == nil {
		if temp, err := strconv.ParseFloat(strings.TrimSpace(string(data)), 64); err == nil {
			temps.CPU = temp / 1000.0 // Convert from millidegrees
		}
	}

	// Try to read Raspberry Pi specific temperature
	if runtime.GOARCH == "arm" || runtime.GOARCH == "arm64" {
		if temp, err := getRPiTemperature(); err == nil {
			temps.CPU = temp
		}
	}

	if len(temps.Sensors) == 0 && temps.CPU == 0 {
		return nil, fmt.Errorf("no temperature sensors available")
	}

	return temps, nil
}

// getRPiTemperature gets Raspberry Pi CPU temperature
func getRPiTemperature() (float64, error) {
	data, err := os.ReadFile("/sys/class/thermal/thermal_zone0/temp")
	if err != nil {
		return 0, err
	}

	temp, err := strconv.ParseFloat(strings.TrimSpace(string(data)), 64)
	if err != nil {
		return 0, err
	}

	return temp / 1000.0, nil
}

// getLoadAverage gets system load average (Unix-like systems)
func getLoadAverage() ([3]float64, error) {
	var loadAvg [3]float64

	if runtime.GOOS == "windows" {
		return loadAvg, fmt.Errorf("load average not available on Windows")
	}

	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return loadAvg, err
	}

	fields := strings.Fields(string(data))
	if len(fields) < 3 {
		return loadAvg, fmt.Errorf("invalid loadavg format")
	}

	for i := 0; i < 3; i++ {
		if val, err := strconv.ParseFloat(fields[i], 64); err == nil {
			loadAvg[i] = val
		}
	}

	return loadAvg, nil
}

// Default implementations that will be overridden by platform-specific files
func (c *Collector) collectCPU() (*CPUMetrics, error) {
	return c.collectCPUGeneric()
}

func (c *Collector) collectMemory() (*MemoryMetrics, error) {
	return c.collectMemoryGeneric()
}

func (c *Collector) collectDisk() (*DiskMetrics, error) {
	return c.collectDiskGeneric()
}

func (c *Collector) collectNetwork() (*NetworkMetrics, error) {
	return c.collectNetworkGeneric()
}

func (c *Collector) collectProcess() (*ProcessMetrics, error) {
	return c.collectProcessGeneric()
}

func (c *Collector) collectSystem() (*SystemInfo, error) {
	return c.collectSystemGeneric()
}

func (c *Collector) collectTemperature() (*TemperatureMetrics, error) {
	return c.collectTemperatureGeneric()
}
