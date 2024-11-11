package runtime

import (
	"context"
	"fmt"
	"time"

	"github.com/shirou/gopsutil/v3/process"
)

func (r *Runtime) monitorResources(ctx context.Context, name string, proc *managedProcess) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	p, err := process.NewProcess(int32(proc.process.Pid))
	if err != nil {
		r.logger.Error("Failed to create process monitor",
			"process", name,
			"error", err)
		return
	}

	for {
		select {
		case <-ticker.C:
			stats, err := collectResourceStats(p)
			if err != nil {
				r.logger.Error("Failed to collect resource stats",
					"process", name,
					"error", err)
				continue
			}

			// Update process stats
			proc.stats = stats

			// Check limits
			if err := enforceResourceLimits(proc); err != nil {
				r.logger.Error("Resource limit exceeded",
					"process", name,
					"cpu", proc.stats.cpu,
					"memory", proc.stats.memory,
					"error", err)
				proc.cancel() // Stop the process
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

func collectResourceStats(p *process.Process) (*resourceStats, error) {
	cpu, err := p.CPUPercent()
	if err != nil {
		return nil, err
	}

	mem, err := p.MemoryInfo()
	if err != nil {
		return nil, err
	}

	return &resourceStats{
		cpu:    cpu,
		memory: mem.RSS,
	}, nil
}

func enforceResourceLimits(proc *managedProcess) error {
	if proc.stats.limits == nil {
		return nil
	}

	if proc.stats.limits.MaxCPU > 0 && proc.stats.cpu > proc.stats.limits.MaxCPU {
		return fmt.Errorf("CPU usage exceeded: %v%% > %v%%", proc.stats.cpu, proc.stats.limits.MaxCPU)
	}

	if proc.stats.limits.MaxMemory > 0 && proc.stats.memory > proc.stats.limits.MaxMemory {
		return fmt.Errorf("memory usage exceeded: %v > %v bytes", proc.stats.memory, proc.stats.limits.MaxMemory)
	}

	return nil
}
