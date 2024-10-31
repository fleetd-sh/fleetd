package build

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
)

type OCIRuntime struct {
	client      *client.Client
	containerID string
	options     *RollbackOptions
}

func NewOCIRuntime(opts *RuntimeOptions) (*OCIRuntime, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}
	return &OCIRuntime{client: cli}, nil
}

func (r *OCIRuntime) Deploy(ctx context.Context, result *BuildResult) error {
	if len(result.Artifacts) == 0 {
		return fmt.Errorf("no artifacts to deploy")
	}

	// Find OCI artifact
	var imageURL string
	for _, artifact := range result.Artifacts {
		if artifact.Type == "oci" {
			imageURL = artifact.Path
			break
		}
	}
	if imageURL == "" {
		return fmt.Errorf("no OCI artifact found")
	}

	// Pull image if needed
	if _, err := r.client.ImagePull(ctx, imageURL, image.PullOptions{}); err != nil {
		return fmt.Errorf("failed to pull image: %w", err)
	}

	// Create container
	resp, err := r.client.ContainerCreate(ctx, &container.Config{
		Image: imageURL,
		Env:   mapToEnvSlice(result.Spec.Config),
	}, nil, nil, nil, "")
	if err != nil {
		return fmt.Errorf("failed to create container: %w", err)
	}

	// Start container
	if err := r.client.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	r.containerID = resp.ID
	return nil
}

func (r *OCIRuntime) Status(ctx context.Context) (*RuntimeStatus, error) {
	if r.containerID == "" {
		return nil, fmt.Errorf("no container deployed")
	}

	inspect, err := r.client.ContainerInspect(ctx, r.containerID)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect container: %w", err)
	}

	stats, err := r.client.ContainerStats(ctx, r.containerID, false)
	if err != nil {
		return nil, fmt.Errorf("failed to get container stats: %w", err)
	}
	defer stats.Body.Close()

	var statsJSON container.StatsResponse
	if err := json.NewDecoder(stats.Body).Decode(&statsJSON); err != nil {
		return nil, fmt.Errorf("failed to decode stats: %w", err)
	}

	// Calculate CPU usage percentage
	cpuDelta := float64(statsJSON.CPUStats.CPUUsage.TotalUsage - statsJSON.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(statsJSON.CPUStats.SystemUsage - statsJSON.PreCPUStats.SystemUsage)
	cpuPercent := 0.0
	if systemDelta > 0 && cpuDelta > 0 {
		cpuPercent = (cpuDelta / systemDelta) * float64(len(statsJSON.CPUStats.CPUUsage.PercpuUsage)) * 100.0
	}

	startTime, err := time.Parse(time.RFC3339Nano, inspect.State.StartedAt)
	if err != nil {
		startTime = time.Time{}
	}

	return &RuntimeStatus{
		State:       inspect.State.Status,
		Pid:         inspect.State.Pid,
		Memory:      statsJSON.MemoryStats.Usage,
		CPU:         cpuPercent,
		Restarts:    inspect.RestartCount,
		LastError:   inspect.State.Error,
		StartTime:   startTime,
		ContainerID: r.containerID,
	}, nil
}

func (r *OCIRuntime) Rollback(ctx context.Context) error {
	if r == nil {
		return fmt.Errorf("runtime is nil")
	}

	// If we have no container ID, nothing to rollback
	if r.containerID == "" {
		return nil
	}

	// Try to stop the container first
	timeout := 10
	if err := r.client.ContainerStop(ctx, r.containerID, container.StopOptions{
		Timeout: &timeout,
	}); err != nil && !client.IsErrNotFound(err) {
		slog.Error("failed to stop container during rollback",
			"error", err,
			"container", r.containerID)
	}

	// Remove the container
	if err := r.client.ContainerRemove(ctx, r.containerID, container.RemoveOptions{
		Force: true,
	}); err != nil && !client.IsErrNotFound(err) {
		return fmt.Errorf("failed to remove container during rollback: %w", err)
	}

	// Clear the container ID
	r.containerID = ""

	return nil
}

func (r *OCIRuntime) gracefulContainerStop(ctx context.Context) error {
	if r == nil {
		return fmt.Errorf("runtime is nil")
	}
	if r.client == nil {
		return fmt.Errorf("docker client is not initialized")
	}
	if r.containerID == "" {
		return fmt.Errorf("no container ID set")
	}

	timeout := int(r.options.Timeout.Seconds())
	err := r.client.ContainerStop(ctx, r.containerID, container.StopOptions{
		Timeout: &timeout,
	})
	if err != nil {
		return fmt.Errorf("failed to stop container: %w", err)
	}

	return nil
}

func (r *OCIRuntime) forceContainerRemove(ctx context.Context) error {
	return r.client.ContainerRemove(ctx, r.containerID, container.RemoveOptions{
		Force: true,
	})
}

func (r *OCIRuntime) cleanupContainer(ctx context.Context) (ResourceStats, error) {
	stats := ResourceStats{}

	// Get container stats before cleanup
	containerStats, err := r.client.ContainerStats(ctx, r.containerID, false)
	if err == nil {
		defer containerStats.Body.Close()
		var statsJSON container.StatsResponse
		if err := json.NewDecoder(containerStats.Body).Decode(&statsJSON); err == nil {
			stats.MemoryFreed = int64(statsJSON.MemoryStats.Usage)
		}
	}

	// Remove container volumes
	if err := r.client.ContainerRemove(ctx, r.containerID, container.RemoveOptions{
		RemoveVolumes: true,
		RemoveLinks:   true,
	}); err != nil {
		return stats, fmt.Errorf("failed to remove container: %w", err)
	}

	// Cleanup unused images if enabled
	if r.options.CleanupFiles {
		pruneReport, err := r.client.ImagesPrune(ctx, filters.NewArgs())
		if err == nil {
			stats.DiskSpaceFreed = int64(pruneReport.SpaceReclaimed)
		}
	}

	return stats, nil
}

func (r *OCIRuntime) createContainerBackup(ctx context.Context) (string, error) {
	backupDir := filepath.Join(os.TempDir(), fmt.Sprintf("container-backup-%s-%s", r.containerID[:12], time.Now().Format("20060102-150405")))
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return "", err
	}

	// Export container
	reader, err := r.client.ContainerExport(ctx, r.containerID)
	if err != nil {
		return "", fmt.Errorf("failed to export container: %w", err)
	}
	defer reader.Close()

	backupPath := filepath.Join(backupDir, "container.tar")
	file, err := os.Create(backupPath)
	if err != nil {
		return "", fmt.Errorf("failed to create backup file: %w", err)
	}
	defer file.Close()

	if _, err := io.Copy(file, reader); err != nil {
		return "", fmt.Errorf("failed to save backup: %w", err)
	}

	// Save container metadata
	inspect, err := r.client.ContainerInspect(ctx, r.containerID)
	if err == nil {
		metadataPath := filepath.Join(backupDir, "metadata.json")
		if err := saveJSON(metadataPath, inspect); err != nil {
			return "", fmt.Errorf("failed to save metadata: %w", err)
		}
	}

	return backupDir, nil
}

func (r *OCIRuntime) Close() error {
	if r == nil {
		return nil
	}

	var errs []error
	ctx := context.Background()

	// Stop container if we have an ID and client
	if r.containerID != "" && r.client != nil {
		timeout := 30
		if err := r.client.ContainerStop(ctx, r.containerID, container.StopOptions{
			Timeout: &timeout,
		}); err != nil && !client.IsErrNotFound(err) {
			errs = append(errs, fmt.Errorf("failed to stop container: %w", err))
		}
	}

	// Remove container if we have an ID and client
	if r.containerID != "" && r.client != nil {
		if err := r.client.ContainerRemove(ctx, r.containerID, container.RemoveOptions{
			Force: true,
		}); err != nil && !client.IsErrNotFound(err) {
			errs = append(errs, fmt.Errorf("failed to remove container: %w", err))
		}
	}

	// Close client if we have one
	if r.client != nil {
		if err := r.client.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close docker client: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("multiple errors during cleanup: %v", errs)
	}
	return nil
}

func (r *OCIRuntime) verifyContainerRollback(ctx context.Context) error {
	// Check if container still exists
	_, err := r.client.ContainerInspect(ctx, r.containerID)
	if err == nil {
		return fmt.Errorf("container still exists after rollback")
	}
	if !client.IsErrNotFound(err) {
		return fmt.Errorf("unexpected error checking container: %w", err)
	}

	// Check if container's volumes are cleaned up
	volumes, err := r.client.VolumeList(ctx, volume.ListOptions{
		Filters: filters.NewArgs(
			filters.Arg("label", fmt.Sprintf("container=%s", r.containerID)),
		),
	})
	if err != nil {
		return fmt.Errorf("failed to list volumes: %w", err)
	}
	if len(volumes.Volumes) > 0 {
		return fmt.Errorf("container volumes still exist")
	}

	// Check if container's networks are cleaned up
	networks, err := r.client.NetworkList(ctx, network.ListOptions{
		Filters: filters.NewArgs(
			filters.Arg("label", fmt.Sprintf("container=%s", r.containerID)),
		),
	})
	if err != nil {
		return fmt.Errorf("failed to list networks: %w", err)
	}
	if len(networks) > 0 {
		return fmt.Errorf("container networks still exist")
	}

	return nil
}

func (r *OCIRuntime) notifyRollbackMetrics(result *RollbackResult) error {
	// Create metrics event
	event := map[string]interface{}{
		"type":              "oci_rollback",
		"container_id":      r.containerID,
		"success":           result.Success,
		"duration_ms":       result.EndTime.Sub(result.StartTime).Milliseconds(),
		"shutdown_duration": result.Metrics.ShutdownDuration.Milliseconds(),
		"cleanup_duration":  result.Metrics.CleanupDuration.Milliseconds(),
		"memory_freed":      result.Metrics.ResourcesFreed.MemoryFreed,
		"disk_freed":        result.Metrics.ResourcesFreed.DiskSpaceFreed,
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

func (r *OCIRuntime) saveJSON(path string, data interface{}) error {
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
