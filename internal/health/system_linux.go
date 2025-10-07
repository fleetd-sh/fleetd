//go:build linux

package health

import (
	"fmt"
	"runtime"
	"syscall"
)

// DiskUsage represents disk usage statistics
type DiskUsage struct {
	Total       uint64  `json:"total"`
	Used        uint64  `json:"used"`
	Free        uint64  `json:"free"`
	UsedPercent float64 `json:"used_percent"`
}

// MemoryUsage represents memory usage statistics
type MemoryUsage struct {
	Total       uint64  `json:"total"`
	Used        uint64  `json:"used"`
	Available   uint64  `json:"available"`
	UsedPercent float64 `json:"used_percent"`
}

// CPUUsage represents CPU usage statistics
type CPUUsage struct {
	Cores       int     `json:"cores"`
	UsedPercent float64 `json:"used_percent"`
}

// GetDiskUsage returns disk usage for a path
func GetDiskUsage(paths []string) ([]DiskUsage, error) {
	usages := make([]DiskUsage, 0, len(paths))

	for _, path := range paths {
		var stat syscall.Statfs_t
		if err := syscall.Statfs(path, &stat); err != nil {
			return nil, fmt.Errorf("failed to get disk stats for %s: %w", path, err)
		}

		total := stat.Blocks * uint64(stat.Bsize)
		free := stat.Bavail * uint64(stat.Bsize)
		used := total - free
		usedPercent := float64(used) * 100.0 / float64(total)

		usages = append(usages, DiskUsage{
			Total:       total,
			Used:        used,
			Free:        free,
			UsedPercent: usedPercent,
		})
	}

	return usages, nil
}

// GetMemoryUsage returns current memory usage
func GetMemoryUsage() (*MemoryUsage, error) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// Get system memory info (platform-specific)
	sysInfo := new(syscall.Sysinfo_t)
	if err := syscall.Sysinfo(sysInfo); err != nil {
		// Fallback to runtime stats only
		return &MemoryUsage{
			Total:       m.Sys,
			Used:        m.Alloc,
			Available:   m.Sys - m.Alloc,
			UsedPercent: float64(m.Alloc) * 100.0 / float64(m.Sys),
		}, nil
	}

	totalMem := sysInfo.Totalram * uint64(sysInfo.Unit)
	freeMem := sysInfo.Freeram * uint64(sysInfo.Unit)
	usedMem := totalMem - freeMem
	usedPercent := float64(usedMem) * 100.0 / float64(totalMem)

	return &MemoryUsage{
		Total:       totalMem,
		Used:        usedMem,
		Available:   freeMem,
		UsedPercent: usedPercent,
	}, nil
}

// GetCPUUsage returns current CPU usage
func GetCPUUsage() (*CPUUsage, error) {
	// This is a simplified implementation
	// In production, you'd want to sample CPU usage over time
	return &CPUUsage{
		Cores:       runtime.NumCPU(),
		UsedPercent: 0.0, // Would need proper sampling
	}, nil
}
