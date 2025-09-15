//go:build darwin

package process

import (
	"syscall"
)

// setResourceLimits sets process resource limits on macOS
func (mp *ManagedProcess) setResourceLimits() {
	// On macOS, we can only set basic resource limits
	mp.Cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true, // Create new process group for easier cleanup
	}

	// Note: Memory and CPU limits would need to be enforced differently on macOS
	// Consider using sandbox-exec or other macOS-specific mechanisms
	if mp.config.Resources != nil && mp.config.Resources.Limits != nil {
		mp.logger.Debug("Resource limits are not fully supported on macOS")
	}
}
