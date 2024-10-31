package build

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

type NativeRuntime struct {
	Runtime
	processCmd *exec.Cmd
	workDir    string
	options    *RuntimeOptions
	startTime  time.Time
	deployed   bool // Track deployment state
	pgid       int  // Process group ID
}

func NewNativeRuntime(opts *RuntimeOptions) *NativeRuntime {
	if opts == nil {
		opts = DefaultRuntimeOptions()
	}

	var workDir string
	if opts.WorkDir != "" {
		workDir = opts.WorkDir
	} else {
		workDir = "/opt/fleetd/runtime"
	}

	return &NativeRuntime{
		workDir: workDir,
		options: opts,
	}
}

func (r *NativeRuntime) Deploy(ctx context.Context, result *BuildResult) error {
	if r.deployed {
		return fmt.Errorf("runtime already has a deployment")
	}

	// Find executable artifact
	var executablePath string
	for _, artifact := range result.Artifacts {
		if artifact.Type == ArtifactTypeExecutable {
			executablePath = artifact.Path
			break
		}
	}
	if executablePath == "" {
		return fmt.Errorf("no executable artifact found")
	}

	// Create runtime directory if it doesn't exist
	if err := os.MkdirAll(r.workDir, 0755); err != nil {
		return fmt.Errorf("failed to create runtime directory: %w", err)
	}

	// Create a new process group
	cmd := exec.CommandContext(ctx, executablePath)
	cmd.Dir = r.workDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true, // Create new process group
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start process: %w", err)
	}

	r.processCmd = cmd
	r.deployed = true
	r.startTime = time.Now()
	r.pgid = cmd.Process.Pid // Store process group ID

	return nil
}

func (r *NativeRuntime) Status(ctx context.Context) (*RuntimeStatus, error) {
	if r.processCmd == nil || r.processCmd.Process == nil {
		return nil, fmt.Errorf("no process running")
	}

	// Create a channel for the process state
	done := make(chan error, 1)

	// Check process state in a non-blocking way
	go func() {
		if _, err := r.processCmd.Process.Wait(); err != nil {
			done <- err
			return
		}
		done <- nil
	}()

	// Wait for either process check or timeout
	select {
	case err := <-done:
		if err != nil {
			// Process is already gone
			if strings.Contains(err.Error(), "no child") {
				return nil, fmt.Errorf("process has terminated")
			}
			return nil, fmt.Errorf("error checking process: %w", err)
		}
		// Process has exited normally
		return nil, fmt.Errorf("process has terminated")

	case <-time.After(100 * time.Millisecond):
		// Process is still running (Wait() is blocking)
		return &RuntimeStatus{
			State:     "running",
			Pid:       r.processCmd.Process.Pid,
			StartTime: r.startTime,
		}, nil
	}
}

func (r *NativeRuntime) Rollback(ctx context.Context) error {
	if r.processCmd == nil || r.processCmd.Process == nil {
		return nil
	}

	// Try graceful shutdown of process group first
	if err := r.signalGroup(syscall.SIGTERM); err != nil {
		if !strings.Contains(err.Error(), "process already finished") {
			slog.Debug("failed to send SIGTERM to process group", "error", err)
		}
	} else {
		// Wait for graceful shutdown with timeout
		gracefulDone := make(chan error, 1)
		go func() {
			gracefulDone <- r.waitForGroup(5 * time.Second)
		}()

		select {
		case err := <-gracefulDone:
			if err == nil {
				// Process group exited gracefully
				r.processCmd = nil
				return nil
			}
			slog.Debug("error waiting for graceful shutdown", "error", err)
		case <-time.After(5 * time.Second):
			slog.Debug("graceful shutdown timeout, proceeding with force kill")
		}
	}

	// Force kill the process group
	if err := r.signalGroup(syscall.SIGKILL); err != nil {
		if !strings.Contains(err.Error(), "process already finished") {
			slog.Debug("failed to kill process group", "error", err)
		}
	}

	// Wait for final cleanup
	if err := r.waitForGroup(2 * time.Second); err != nil {
		slog.Debug("error waiting for process group to exit", "error", err)
	}

	r.processCmd = nil
	return nil
}

// Helper functions for process group management
func (r *NativeRuntime) signalGroup(sig syscall.Signal) error {
	if r.pgid <= 0 {
		return fmt.Errorf("invalid process group ID")
	}
	// Negative PID means signal the process group
	return syscall.Kill(-r.pgid, sig)
}

func (r *NativeRuntime) waitForGroup(timeout time.Duration) error {
	if r.pgid <= 0 {
		return fmt.Errorf("invalid process group ID")
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		// Check if process group still exists
		if err := syscall.Kill(-r.pgid, 0); err != nil {
			if err == syscall.ESRCH {
				return nil // Process group is gone
			}
			return err
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for process group")
}

func (r *NativeRuntime) Close() error {
	if r.processCmd == nil || r.processCmd.Process == nil {
		return nil
	}

	// Force kill the process group immediately
	if err := r.signalGroup(syscall.SIGKILL); err != nil {
		if !strings.Contains(err.Error(), "process already finished") {
			slog.Debug("failed to kill process group", "error", err)
		}
	}

	// Wait for the process group to exit with timeout
	if err := r.waitForGroup(2 * time.Second); err != nil {
		slog.Debug("error waiting for process group to exit", "error", err)
	}

	// Clear the command
	r.processCmd = nil
	return nil
}

func (r *NativeRuntime) forceKill() error {
	if err := r.processCmd.Process.Kill(); err != nil {
		return fmt.Errorf("failed to kill process: %w", err)
	}
	return nil
}

func (r *NativeRuntime) verifyRollback() error {
	// Check if process is actually terminated
	if r.processCmd != nil && r.processCmd.Process != nil {
		if err := r.processCmd.Process.Signal(syscall.Signal(0)); err == nil {
			return fmt.Errorf("process is still running")
		}
	}

	// Verify runtime directory is cleaned up if cleanup was enabled
	if r.options.CleanupFiles {
		if _, err := os.Stat(r.workDir); !os.IsNotExist(err) {
			return fmt.Errorf("runtime directory still exists: %s", r.workDir)
		}
	}

	return nil
}

func (r *NativeRuntime) notifyRollbackMetrics(result *RollbackResult) error {
	event := map[string]interface{}{
		"type":              "native_rollback",
		"pid":               r.processCmd.Process.Pid,
		"success":           result.Success,
		"duration_ms":       result.EndTime.Sub(result.StartTime).Milliseconds(),
		"shutdown_duration": result.Metrics.ShutdownDuration.Milliseconds(),
		"cleanup_duration":  result.Metrics.CleanupDuration.Milliseconds(),
		"disk_freed":        result.Metrics.ResourcesFreed.DiskSpaceFreed,
		"files_removed":     result.Metrics.ResourcesFreed.FilesRemoved,
		"has_backup":        result.BackupPath != "",
		"error":             "",
	}
	if result.Error != nil {
		event["error"] = result.Error.Error()
	}

	slog.With("event", event).Info("Rollback metrics")
	return nil
}

func (r *NativeRuntime) cleanupFiles(stats *ResourceStats) error {
	count, err := cleanupFiles(r.workDir)
	if err != nil {
		return fmt.Errorf("failed to cleanup files: %w", err)
	}
	stats.FilesRemoved = count
	return nil
}
