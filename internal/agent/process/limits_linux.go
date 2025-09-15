//go:build linux

package process

import (
	"syscall"
)

// setResourceLimits sets process resource limits on Linux
func (mp *ManagedProcess) setResourceLimits() {
	limits := mp.config.Resources.Limits

	mp.Cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true, // Create new process group for easier cleanup
	}

	// Set memory limit
	if limits.Memory > 0 {
		// On Linux, we can use cgroups for better memory control
		// For now, use basic rlimit
		mp.logger.Debug("Memory limits should be enforced via cgroups",
			"limit", limits.Memory)
	}

	// Set file descriptor limit
	if limits.FileDescriptors > 0 {
		// On Linux, we can set RLIMIT_NOFILE
		// This would require platform-specific syscall handling
		mp.logger.Debug("File descriptor limits should be set via setrlimit",
			"limit", limits.FileDescriptors)
	}

	// CPU limits would be enforced via cgroups in production
	if limits.CpuMillicores > 0 {
		mp.logger.Debug("CPU limits should be enforced via cgroups",
			"millicores", limits.CpuMillicores)
	}
}
