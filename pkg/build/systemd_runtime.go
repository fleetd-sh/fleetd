package build

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/coreos/go-systemd/v22/dbus"
)

type SystemdRuntime struct {
	conn     *dbus.Conn
	unitName string
	options  *RuntimeOptions
}

func NewSystemdRuntime(ctx context.Context, opts *RuntimeOptions) (*SystemdRuntime, error) {
	conn, err := dbus.NewSystemdConnectionContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to systemd: %w", err)
	}
	return &SystemdRuntime{conn: conn, options: opts}, nil
}

func (r *SystemdRuntime) Deploy(ctx context.Context, result *BuildResult) error {
	if len(result.Artifacts) == 0 {
		return fmt.Errorf("no artifacts to deploy")
	}

	// Find executable artifact
	var executablePath string
	for _, artifact := range result.Artifacts {
		if isExecutable(artifact.Path) {
			executablePath = artifact.Path
			break
		}
	}
	if executablePath == "" {
		return fmt.Errorf("no executable artifact found")
	}

	// Create systemd unit
	unitName := fmt.Sprintf("fleetd-%s.service", result.ID)
	unitContent, err := r.createUnitFile(&result.Spec, executablePath)
	if err != nil {
		return fmt.Errorf("failed to create unit file: %w", err)
	}

	// Install and start unit
	if _, err := r.conn.LinkUnitFilesContext(ctx,
		[]string{unitContent},
		true,
		true,
	); err != nil {
		return fmt.Errorf("failed to link unit file: %w", err)
	}

	if err := r.conn.ReloadContext(ctx); err != nil {
		return fmt.Errorf("failed to reload systemd: %w", err)
	}

	if _, err := r.conn.StartUnitContext(ctx, unitName, "replace", nil); err != nil {
		return fmt.Errorf("failed to start unit: %w", err)
	}

	r.unitName = unitName
	return nil
}

func (r *SystemdRuntime) Status(ctx context.Context) (*RuntimeStatus, error) {
	if r.unitName == "" {
		return nil, fmt.Errorf("no unit deployed")
	}

	props, err := r.conn.GetAllPropertiesContext(ctx, r.unitName)
	if err != nil {
		return nil, fmt.Errorf("failed to get unit properties: %w", err)
	}

	return &RuntimeStatus{
		State:     props["ActiveState"].(string),
		Pid:       int(props["MainPID"].(uint32)),
		Memory:    props["MemoryCurrent"].(uint64),
		CPU:       float64(props["CPUUsageNSec"].(uint64)) / 1e9,
		Restarts:  int(props["NRestarts"].(uint32)),
		LastError: props["Result"].(string),
		StartTime: time.Unix(int64(props["StartTimestamp"].(uint64))/1e6, 0),
	}, nil
}

func (r *SystemdRuntime) Close() error {
	r.conn.Close()
	return nil
}

func (r *SystemdRuntime) createUnitFile(spec *BuildSpec, executablePath string) (string, error) {
	// Create a temporary directory for the unit file
	unitDir, err := os.MkdirTemp("", "fleetd-unit-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}

	unitName := fmt.Sprintf("fleetd-%s.service", spec.Version)
	unitPath := filepath.Join(unitDir, unitName)

	// Build unit file content
	unitContent := r.generateUnitContent(spec, executablePath)

	// Write unit file using shared function
	if err := os.WriteFile(unitPath, []byte(unitContent), 0644); err != nil {
		os.RemoveAll(unitDir) // Cleanup on error
		return "", fmt.Errorf("failed to write unit file: %w", err)
	}

	return unitPath, nil
}

// generateResourceLimits creates systemd resource limit directives based on spec configuration
func (r *SystemdRuntime) generateResourceLimits(spec *BuildSpec) string {
	var limits []string

	// CPU limits
	if cpuLimit, ok := spec.Config["cpu.limit"]; ok {
		limits = append(limits, fmt.Sprintf("CPUQuota=%s%%", cpuLimit))
	}
	if cpuShares, ok := spec.Config["cpu.shares"]; ok {
		limits = append(limits, fmt.Sprintf("CPUShares=%s", cpuShares))
	}

	// Memory limits
	if memoryLimit, ok := spec.Config["memory.limit"]; ok {
		limits = append(limits, fmt.Sprintf("MemoryLimit=%s", memoryLimit))
	}
	if memorySwap, ok := spec.Config["memory.swap"]; ok {
		limits = append(limits, fmt.Sprintf("MemorySwapMax=%s", memorySwap))
	}

	// Task limits
	if taskLimit, ok := spec.Config["tasks.max"]; ok {
		limits = append(limits, fmt.Sprintf("TasksMax=%s", taskLimit))
	}

	// IO limits
	if ioWeight, ok := spec.Config["io.weight"]; ok {
		limits = append(limits, fmt.Sprintf("IOWeight=%s", ioWeight))
	}

	// Default limits if none specified
	if len(limits) == 0 {
		limits = append(limits,
			"CPUQuota=100%",
			"MemoryLimit=512M",
			"TasksMax=32",
		)
	}

	return strings.Join(limits, "\n")
}

func (r *SystemdRuntime) Rollback(ctx context.Context) error {
	result := &RollbackResult{
		StartTime: time.Now(),
		State:     RollbackStateStarted,
	}
	defer func() {
		result.EndTime = time.Now()
		if result.Error != nil {
			result.State = RollbackStateFailed
		}
	}()

	if r.unitName == "" {
		return nil
	}

	// Create backup if enabled
	if r.options.CreateBackup {
		backupPath, err := r.createUnitBackup(ctx)
		if err != nil {
			result.Error = fmt.Errorf("backup failed: %w", err)
			return result.Error
		}
		result.BackupPath = backupPath
	}

	// Graceful shutdown with timeout
	shutdownStart := time.Now()
	if err := r.gracefulUnitStop(ctx); err != nil {
		if !r.options.Force {
			result.Error = fmt.Errorf("graceful shutdown failed: %w", err)
			return result.Error
		}
		// Force stop if graceful shutdown fails
		if err := r.forceUnitStop(ctx); err != nil {
			result.Error = fmt.Errorf("force stop failed: %w", err)
			return result.Error
		}
	}
	result.Metrics.ShutdownDuration = time.Since(shutdownStart)

	// Cleanup resources
	cleanupStart := time.Now()
	stats, err := r.cleanupUnit(ctx)
	if err != nil {
		result.Error = fmt.Errorf("cleanup failed: %w", err)
		return result.Error
	}
	result.Metrics.CleanupDuration = time.Since(cleanupStart)
	result.Metrics.ResourcesFreed = stats

	// Health check if enabled
	if r.options.HealthCheck {
		if err := r.verifyUnitRollback(ctx); err != nil {
			result.Error = fmt.Errorf("health check failed: %w", err)
			return result.Error
		}
	}

	// Notify metrics if enabled
	if r.options.NotifyMetrics {
		if err := r.notifyRollbackMetrics(result); err != nil {
			slog.With("error", err).Error("Failed to notify metrics")
		}
	}

	r.unitName = ""
	result.Success = true
	result.State = RollbackStateComplete
	return nil
}

func (r *SystemdRuntime) gracefulUnitStop(ctx context.Context) error {
	ch := make(chan string)
	if _, err := r.conn.StopUnitContext(ctx, r.unitName, "replace", ch); err != nil {
		return fmt.Errorf("failed to stop unit: %w", err)
	}

	select {
	case result := <-ch:
		if result != "done" {
			return fmt.Errorf("failed to stop unit: %s", result)
		}
	case <-time.After(r.options.Timeout):
		return fmt.Errorf("stop timed out after %v", r.options.Timeout)
	case <-ctx.Done():
		return ctx.Err()
	}

	return nil
}

func (r *SystemdRuntime) forceUnitStop(ctx context.Context) error {
	ch := make(chan string)
	r.conn.KillUnitContext(ctx, r.unitName, int32(syscall.SIGKILL))

	select {
	case result := <-ch:
		if result != "done" {
			return fmt.Errorf("failed to kill unit: %s", result)
		}
	case <-ctx.Done():
		return ctx.Err()
	}

	return nil
}

func (r *SystemdRuntime) cleanupUnit(ctx context.Context) (ResourceStats, error) {
	stats := ResourceStats{}

	// Get initial disk usage of unit files
	unitPath := filepath.Join("/etc/systemd/system", r.unitName)
	initialSize, err := getDiskUsage(unitPath)
	if err == nil {
		stats.DiskSpaceFreed = initialSize
	}

	// Disable and remove unit file
	if _, err := r.conn.DisableUnitFilesContext(ctx, []string{r.unitName}, true); err != nil {
		return stats, fmt.Errorf("failed to disable unit: %w", err)
	}

	// Remove unit file if cleanup is enabled
	if r.options.CleanupFiles {
		if err := os.Remove(unitPath); err == nil {
			stats.FilesRemoved++
		}
	}

	// Reload systemd
	if err := r.conn.ReloadContext(ctx); err != nil {
		return stats, fmt.Errorf("failed to reload systemd: %w", err)
	}

	return stats, nil
}

func (r *SystemdRuntime) createUnitBackup(ctx context.Context) (string, error) {
	backupDir := filepath.Join("/var/lib/fleetd/backups", fmt.Sprintf("unit-%s-%s", r.unitName, time.Now().Format("20060102-150405")))
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return "", err
	}

	// Copy unit file
	unitPath := filepath.Join("/etc/systemd/system", r.unitName)
	backupPath := filepath.Join(backupDir, r.unitName)
	if err := copyFile(unitPath, backupPath); err != nil {
		return "", fmt.Errorf("failed to backup unit file: %w", err)
	}

	// Save unit properties using shared function
	properties, err := r.conn.GetUnitPropertiesContext(ctx, r.unitName)
	if err == nil {
		metadataPath := filepath.Join(backupDir, "metadata.json")
		if err := saveJSON(metadataPath, properties); err != nil {
			return "", fmt.Errorf("failed to save metadata: %w", err)
		}
	}

	return backupDir, nil
}

func (r *SystemdRuntime) verifyUnitRollback(ctx context.Context) error {
	// Check if unit is actually stopped
	props, err := r.conn.GetUnitPropertiesContext(ctx, r.unitName)
	if err != nil {
		return fmt.Errorf("failed to get unit properties: %w", err)
	}

	activeState, ok := props["ActiveState"].(string)
	if !ok {
		return fmt.Errorf("failed to get unit active state")
	}

	if activeState != "inactive" && activeState != "failed" {
		return fmt.Errorf("unit is still active: %s", activeState)
	}

	// Verify unit file is removed if cleanup was enabled
	if r.options.CleanupFiles {
		unitPath := filepath.Join("/etc/systemd/system", r.unitName)
		if _, err := os.Stat(unitPath); !os.IsNotExist(err) {
			return fmt.Errorf("unit file still exists: %s", unitPath)
		}
	}

	return nil
}

func (r *SystemdRuntime) notifyRollbackMetrics(result *RollbackResult) error {
	// Create metrics event
	event := map[string]interface{}{
		"type":              "systemd_rollback",
		"unit_name":         r.unitName,
		"success":           result.Success,
		"duration_ms":       result.EndTime.Sub(result.StartTime).Milliseconds(),
		"shutdown_duration": result.Metrics.ShutdownDuration.Milliseconds(),
		"cleanup_duration":  result.Metrics.CleanupDuration.Milliseconds(),
		"files_removed":     result.Metrics.ResourcesFreed.FilesRemoved,
		"has_backup":        result.BackupPath != "",
		"error":             "",
	}
	if result.Error != nil {
		event["error"] = result.Error.Error()
	}

	// Log metrics event
	slog.With("event", event).Info("Rollback metrics")

	// TODO: Send to metrics collection system
	// This could be implemented to send to Prometheus, InfluxDB, etc.
	return nil
}

func (r *SystemdRuntime) saveJSON(path string, data interface{}) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(data); err != nil {
		return fmt.Errorf("failed to encode JSON: %w", err)
	}

	return nil
}

func (r *SystemdRuntime) generateUnitContent(spec *BuildSpec, executablePath string) string {
	// Build environment variables
	envVars := []string{}
	for k, v := range spec.Env {
		envVars = append(envVars, fmt.Sprintf("%s=%s", k, v))
	}

	// Build command arguments
	execStart := executablePath
	if len(spec.Commands) > 0 {
		execStart = fmt.Sprintf("%s %s", executablePath, strings.Join(spec.Commands, " "))
	}

	// Generate unit file content
	unitContent := fmt.Sprintf(`[Unit]
Description=Fleetd Daemon Service - %s
Documentation=https://fleetd.sh/docs
After=network.target

[Service]
Type=simple
ExecStart=%s
WorkingDirectory=%s
Environment=%s
Restart=always
RestartSec=10
TimeoutStartSec=30
TimeoutStopSec=30

# Resource limits
%s

# Security settings
NoNewPrivileges=yes
ProtectSystem=full
ProtectHome=read-only
PrivateTmp=yes
PrivateDevices=yes
ProtectKernelTunables=yes
ProtectKernelModules=yes
ProtectControlGroups=yes
RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX
RestrictNamespaces=yes

# Logging
StandardOutput=journal
StandardError=journal
SyslogIdentifier=fleetd-%s

[Install]
WantedBy=multi-user.target
`,
		spec.Version,
		execStart,
		filepath.Dir(executablePath),
		strings.Join(envVars, " "),
		r.generateResourceLimits(spec),
		spec.Version,
	)

	return unitContent
}
