package process

import (
	"fmt"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
)

const MaxProcesses = 32

type ProcessPriority int

const (
	PriorityCritical ProcessPriority = iota
	PriorityNormal
	PriorityBackground
)

type ProcessStatus struct {
	State ProcessState
	Pid   int
	Error string
}

type ProcessState int

const (
	StateStarting ProcessState = iota
	StateRunning
	StateStopped
	StateFailed
)

type ProcessInfo struct {
	ID          string
	Executable  string
	Args        []string
	Status      ProcessStatus
	MemoryLimit int64
	Priority    ProcessPriority
}

type ResourceLimits struct {
	MaxMemoryKB   int64
	MaxCPUPercent float64
	MaxRuntime    time.Duration
	CheckInterval time.Duration
}

type ProcessManager struct {
	processes      map[string]*ProcessInfo
	resourceLimits ResourceLimits
	mu             sync.RWMutex
	runningCount   atomic.Int32
}

func NewProcessManager() *ProcessManager {
	return &ProcessManager{
		processes: make(map[string]*ProcessInfo),
		resourceLimits: ResourceLimits{
			MaxMemoryKB:   64 * 1024, // 64MB default
			MaxCPUPercent: 80.0,      // 80% CPU default
			CheckInterval: 5 * time.Second,
		},
	}
}

func (pm *ProcessManager) StartProcess(id string, executable string, args []string, priority ProcessPriority) error {
	if pm.runningCount.Load() >= MaxProcesses {
		return fmt.Errorf("resource limit reached: maximum processes")
	}

	memLimit := int64(0)
	switch priority {
	case PriorityCritical:
		memLimit = 64 * 1024 * 1024
	case PriorityNormal:
		memLimit = 32 * 1024 * 1024
	case PriorityBackground:
		memLimit = 16 * 1024 * 1024
	}

	cmd := exec.Command(executable, args...)

	// Set process group and nice value based on priority
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	switch priority {
	case PriorityCritical:
		cmd.SysProcAttr.Nice = -10
	case PriorityBackground:
		cmd.SysProcAttr.Nice = 10
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start process: %w", err)
	}

	processInfo := &ProcessInfo{
		ID:          id,
		Executable:  executable,
		Args:        args,
		Status:      ProcessStatus{State: StateRunning, Pid: cmd.Process.Pid},
		MemoryLimit: memLimit,
		Priority:    priority,
	}

	pm.mu.Lock()
	pm.processes[id] = processInfo
	pm.mu.Unlock()

	pm.runningCount.Add(1)

	// Start monitoring goroutine
	go pm.monitorProcess(id, cmd.Process.Pid)

	return nil
}

func (pm *ProcessManager) StopProcess(id string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	proc, exists := pm.processes[id]
	if !exists {
		return fmt.Errorf("process not found: %s", id)
	}

	if proc.Status.State != StateRunning {
		return nil
	}

	process, err := os.FindProcess(proc.Status.Pid)
	if err != nil {
		return fmt.Errorf("failed to find process: %w", err)
	}

	if err := process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("failed to stop process: %w", err)
	}

	proc.Status.State = StateStopped
	pm.runningCount.Add(-1)
	return nil
}

func (pm *ProcessManager) monitorProcess(id string, pid int) {
	ticker := time.NewTicker(pm.resourceLimits.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			usage, err := getProcessResources(pid)
			if err != nil {
				if err == os.ErrProcessDone {
					pm.mu.Lock()
					if proc, exists := pm.processes[id]; exists {
						proc.Status.State = StateStopped
						pm.runningCount.Add(-1)
					}
					pm.mu.Unlock()
					return
				}
				logrus.Warnf("Failed to get process resources: %v", err)
				continue
			}

			if usage.MemoryKB > pm.resourceLimits.MaxMemoryKB {
				logrus.Warnf("Process %s exceeded memory limit: %d KB > %d KB",
					id, usage.MemoryKB, pm.resourceLimits.MaxMemoryKB)
				if err := pm.StopProcess(id); err != nil {
					logrus.Errorf("Failed to stop process %s: %v", id, err)
				}
				return
			}

			if usage.CPUPercent > pm.resourceLimits.MaxCPUPercent {
				logrus.Warnf("Process %s exceeded CPU limit: %.1f%% > %.1f%%",
					id, usage.CPUPercent, pm.resourceLimits.MaxCPUPercent)
			}
		}
	}
}

type ResourceUsage struct {
	MemoryKB   int64
	CPUPercent float64
}

// Linux implementation of getProcessResources
func getProcessResources(pid int) (*ResourceUsage, error) {
	// Read memory from /proc/[pid]/statm
	statmPath := fmt.Sprintf("/proc/%d/statm", pid)
	statmBytes, err := os.ReadFile(statmPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, os.ErrProcessDone
		}
		return nil, fmt.Errorf("failed to read statm: %w", err)
	}

	var pages int64
	_, err = fmt.Sscanf(string(statmBytes), "%d", &pages)
	if err != nil {
		return nil, fmt.Errorf("failed to parse statm: %w", err)
	}

	// Convert pages to KB (assuming 4KB pages)
	memoryKB := pages * 4

	// Read CPU usage from /proc/[pid]/stat
	// This is more complex and requires two readings to calculate percentage
	// For now, returning just memory usage
	return &ResourceUsage{
		MemoryKB:   memoryKB,
		CPUPercent: 0, // TODO: Implement CPU percentage calculation
	}, nil
}
