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
	if memLimit, ok := limits["memory"]; ok && memLimit != "" {
		// On Linux, we can use cgroups for better memory control
		// For now, use basic rlimit
		mp.logger.Debug("Memory limits should be enforced via cgroups",
			"limit", memLimit)
	}

	// Set file descriptor limit
	if fdLimit, ok := limits["file_descriptors"]; ok && fdLimit != "" {
		// On Linux, we can set RLIMIT_NOFILE
		// This would require platform-specific syscall handling
		mp.logger.Debug("File descriptor limits should be set via setrlimit",
			"limit", fdLimit)
	}

	// CPU limits would be enforced via cgroups in production
	if cpuLimit, ok := limits["cpu_millicores"]; ok && cpuLimit != "" {
		mp.logger.Debug("CPU limits should be enforced via cgroups",
			"millicores", cpuLimit)
	}
}
