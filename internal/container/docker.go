package container

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

// DockerManager implements ContainerManager for Docker
type DockerManager struct {
	client *client.Client
}

// DockerFactory creates Docker container managers
type DockerFactory struct{}

func init() {
	RegisterContainerFactory("docker", &DockerFactory{})
}

// Create implements ContainerFactory
func (f *DockerFactory) Create(runtime string, options map[string]any) (ContainerManager, error) {
	if runtime != "docker" {
		return nil, fmt.Errorf("unsupported runtime: %s", runtime)
	}

	// Get Docker client options
	var opts []client.Opt
	if host, ok := options["host"].(string); ok {
		opts = append(opts, client.WithHost(host))
	}
	if version, ok := options["version"].(string); ok {
		opts = append(opts, client.WithVersion(version))
	}

	// Create Docker client
	cli, err := client.NewClientWithOpts(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %v", err)
	}

	return &DockerManager{client: cli}, nil
}

// Initialize implements ContainerManager
func (m *DockerManager) Initialize(ctx context.Context) error {
	// Ping Docker daemon to verify connection
	if _, err := m.client.Ping(ctx); err != nil {
		return fmt.Errorf("failed to ping Docker daemon: %v", err)
	}
	return nil
}

// ListContainers implements ContainerManager
func (m *DockerManager) ListContainers(ctx context.Context, filterMap map[string]string) ([]ContainerInfo, error) {
	// Convert filters to Docker format
	filterArgs := filters.NewArgs()
	for k, v := range filterMap {
		filterArgs.Add(k, v)
	}

	// List containers
	containers, err := m.client.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filterArgs,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %v", err)
	}

	// Convert to ContainerInfo
	result := make([]ContainerInfo, len(containers))
	for i, c := range containers {
		info, err := m.containerToInfo(ctx, c)
		if err != nil {
			return nil, err
		}
		result[i] = *info
	}

	return result, nil
}

// PullImage pulls a Docker image from a registry
func (m *DockerManager) PullImage(ctx context.Context, imageName string) error {
	reader, err := m.client.ImagePull(ctx, imageName, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("failed to pull image %s: %v", imageName, err)
	}
	defer reader.Close()

	// Read the output to ensure the pull completes
	_, err = io.Copy(io.Discard, reader)
	if err != nil {
		return fmt.Errorf("failed to read pull output: %v", err)
	}

	return nil
}

// InspectContainer implements ContainerManager
func (m *DockerManager) InspectContainer(ctx context.Context, nameOrID string) (*ContainerInfo, error) {
	// Get container details
	c, err := m.client.ContainerInspect(ctx, nameOrID)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect container: %v", err)
	}

	// Convert to ContainerInfo
	info := &ContainerInfo{
		ID:            c.ID,
		Name:          strings.TrimPrefix(c.Name, "/"),
		Image:         c.Config.Image,
		State:         dockerStateToContainerState(c.State.Status),
		Health:        dockerHealthToHealthState(c.State.Health),
		CreatedAt:     parseDockerTime(c.Created),
		StartedAt:     parseDockerTime(c.State.StartedAt),
		FinishedAt:    parseDockerTime(c.State.FinishedAt),
		ExitCode:      c.State.ExitCode,
		RestartCount:  c.RestartCount,
		Platform:      c.Platform,
		Driver:        c.Driver,
		NetworkMode:   string(c.HostConfig.NetworkMode),
		Labels:        c.Config.Labels,
		RestartPolicy: string(c.HostConfig.RestartPolicy.Name),
	}

	// Get IP address and ports
	if c.NetworkSettings != nil {
		for _, network := range c.NetworkSettings.Networks {
			info.IPAddress = network.IPAddress
			break
		}
		info.Ports = make(map[string]string)
		for port, bindings := range c.NetworkSettings.Ports {
			if len(bindings) > 0 {
				info.Ports[port.Port()] = bindings[0].HostPort
			}
		}
	}

	// Get mounts
	info.Mounts = make([]Mount, len(c.Mounts))
	for i, m := range c.Mounts {
		info.Mounts[i] = Mount{
			Source:   m.Source,
			Target:   m.Destination,
			Type:     string(m.Type),
			ReadOnly: !m.RW,
		}
	}

	return info, nil
}

// CreateContainer implements ContainerManager
func (m *DockerManager) CreateContainer(ctx context.Context, config ContainerConfig) (string, error) {
	// Convert mounts
	mounts := make([]mount.Mount, len(config.Mounts))
	for i, m := range config.Mounts {
		mounts[i] = mount.Mount{
			Type:     mount.Type(m.Type),
			Source:   m.Source,
			Target:   m.Target,
			ReadOnly: m.ReadOnly,
		}
	}

	// Convert ports
	portBindings := nat.PortMap{}
	exposedPorts := nat.PortSet{}
	for container, host := range config.Network.Ports {
		port := nat.Port(container)
		portBindings[port] = []nat.PortBinding{{HostPort: host}}
		exposedPorts[port] = struct{}{}
	}

	// Create container
	resp, err := m.client.ContainerCreate(ctx,
		&container.Config{
			Image:        config.Image,
			Cmd:          config.Command,
			Entrypoint:   config.Entrypoint,
			Env:          config.Env,
			Labels:       config.Labels,
			ExposedPorts: exposedPorts,
		},
		&container.HostConfig{
			Mounts:        mounts,
			PortBindings:  portBindings,
			NetworkMode:   container.NetworkMode(config.Network.NetworkMode),
			DNS:           config.Network.DNS,
			DNSSearch:     config.Network.DNSSearch,
			ExtraHosts:    config.Network.ExtraHosts,
			Resources:     dockerResources(config.Resources),
			RestartPolicy: container.RestartPolicy{Name: container.RestartPolicyMode(config.RestartPolicy)},
		},
		&network.NetworkingConfig{},
		nil,
		config.Name,
	)
	if err != nil {
		return "", fmt.Errorf("failed to create container: %v", err)
	}

	return resp.ID, nil
}

// StartContainer implements ContainerManager
func (m *DockerManager) StartContainer(ctx context.Context, nameOrID string) error {
	if err := m.client.ContainerStart(ctx, nameOrID, container.StartOptions{}); err != nil {
		return fmt.Errorf("failed to start container: %v", err)
	}
	return nil
}

// StopContainer implements ContainerManager
func (m *DockerManager) StopContainer(ctx context.Context, nameOrID string, timeout time.Duration) error {
	timeoutSeconds := int(timeout.Seconds())
	if err := m.client.ContainerStop(ctx, nameOrID, container.StopOptions{Timeout: &timeoutSeconds}); err != nil {
		return fmt.Errorf("failed to stop container: %v", err)
	}
	return nil
}

// RestartContainer implements ContainerManager
func (m *DockerManager) RestartContainer(ctx context.Context, nameOrID string, timeout time.Duration) error {
	timeoutSeconds := int(timeout.Seconds())
	if err := m.client.ContainerRestart(ctx, nameOrID, container.StopOptions{Timeout: &timeoutSeconds}); err != nil {
		return fmt.Errorf("failed to restart container: %v", err)
	}
	return nil
}

// PauseContainer implements ContainerManager
func (m *DockerManager) PauseContainer(ctx context.Context, nameOrID string) error {
	if err := m.client.ContainerPause(ctx, nameOrID); err != nil {
		return fmt.Errorf("failed to pause container: %v", err)
	}
	return nil
}

// UnpauseContainer implements ContainerManager
func (m *DockerManager) UnpauseContainer(ctx context.Context, nameOrID string) error {
	if err := m.client.ContainerUnpause(ctx, nameOrID); err != nil {
		return fmt.Errorf("failed to unpause container: %v", err)
	}
	return nil
}

// RemoveContainer implements ContainerManager
func (m *DockerManager) RemoveContainer(ctx context.Context, nameOrID string, force bool) error {
	if err := m.client.ContainerRemove(ctx, nameOrID, container.RemoveOptions{Force: force}); err != nil {
		return fmt.Errorf("failed to remove container: %v", err)
	}
	return nil
}

// GetContainerLogs implements ContainerManager
func (m *DockerManager) GetContainerLogs(ctx context.Context, nameOrID string, since time.Time) (io.ReadCloser, error) {
	logs, err := m.client.ContainerLogs(ctx, nameOrID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Since:      since.Format(time.RFC3339),
		Follow:     false,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get container logs: %v", err)
	}
	return logs, nil
}

// GetContainerStats implements ContainerManager
func (m *DockerManager) GetContainerStats(ctx context.Context, nameOrID string) (*ContainerStats, error) {
	stats, err := m.client.ContainerStats(ctx, nameOrID, false)
	if err != nil {
		return nil, fmt.Errorf("failed to get container stats: %v", err)
	}
	defer stats.Body.Close()

	var dockerStats container.StatsResponse
	if err := json.NewDecoder(stats.Body).Decode(&dockerStats); err != nil {
		return nil, fmt.Errorf("failed to decode stats: %v", err)
	}

	return &ContainerStats{
		CPUPercentage:    calculateCPUPercentage(&dockerStats),
		MemoryUsage:      dockerStats.MemoryStats.Usage,
		MemoryLimit:      dockerStats.MemoryStats.Limit,
		MemoryPercentage: calculateMemoryPercentage(&dockerStats),
		NetworkRx:        calculateNetworkRx(&dockerStats),
		NetworkTx:        calculateNetworkTx(&dockerStats),
		BlockRead:        dockerStats.BlkioStats.IoServiceBytesRecursive[0].Value,
		BlockWrite:       dockerStats.BlkioStats.IoServiceBytesRecursive[1].Value,
		PIDs:             dockerStats.PidsStats.Current,
		Timestamp:        time.Now(),
	}, nil
}

// ExecInContainer implements ContainerManager
func (m *DockerManager) ExecInContainer(ctx context.Context, nameOrID string, cmd []string, attachStdio bool) (int, error) {
	// Create exec instance
	exec, err := m.client.ContainerExecCreate(ctx, nameOrID, container.ExecOptions{
		Cmd:          cmd,
		AttachStdout: attachStdio,
		AttachStderr: attachStdio,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to create exec: %v", err)
	}

	// Start exec instance
	if err := m.client.ContainerExecStart(ctx, exec.ID, container.ExecAttachOptions{}); err != nil {
		return 0, fmt.Errorf("failed to start exec: %v", err)
	}

	// Get exec result
	result, err := m.client.ContainerExecInspect(ctx, exec.ID)
	if err != nil {
		return 0, fmt.Errorf("failed to inspect exec: %v", err)
	}

	return result.ExitCode, nil
}

// CopyToContainer implements ContainerManager
func (m *DockerManager) CopyToContainer(ctx context.Context, nameOrID, path string, content io.Reader) error {
	if err := m.client.CopyToContainer(ctx, nameOrID, path, content, container.CopyToContainerOptions{}); err != nil {
		return fmt.Errorf("failed to copy to container: %v", err)
	}
	return nil
}

// CopyFromContainer implements ContainerManager
func (m *DockerManager) CopyFromContainer(ctx context.Context, nameOrID, path string) (io.ReadCloser, error) {
	reader, _, err := m.client.CopyFromContainer(ctx, nameOrID, path)
	if err != nil {
		return nil, fmt.Errorf("failed to copy from container: %v", err)
	}
	return reader, nil
}

// PruneContainers implements ContainerManager
func (m *DockerManager) PruneContainers(ctx context.Context) error {
	_, err := m.client.ContainersPrune(ctx, filters.Args{})
	if err != nil {
		return fmt.Errorf("failed to prune containers: %v", err)
	}
	return nil
}

// Events implements ContainerManager
func (m *DockerManager) Events(ctx context.Context) (<-chan ContainerEvent, error) {
	dockerEvents, errs := m.client.Events(ctx, events.ListOptions{
		Filters: filters.NewArgs(filters.Arg("type", "container")),
	})

	events := make(chan ContainerEvent)
	go func() {
		defer close(events)
		for {
			select {
			case event := <-dockerEvents:
				events <- ContainerEvent{
					Type:       string(event.Action),
					ID:         event.Actor.ID,
					Name:       event.Actor.Attributes["name"],
					Image:      event.Actor.Attributes["image"],
					Time:       time.Unix(event.Time, 0),
					Status:     dockerStateToContainerState(string(event.Action)),
					ExitCode:   0, // Not available in event
					Error:      event.Actor.Attributes["error"],
					Attributes: event.Actor.Attributes,
				}
			case err := <-errs:
				if err != nil {
					// Log error
					slog.Error("Error receiving Docker events", "error", err)
				}
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	return events, nil
}

// Add this method to the DockerManager type
func (m *DockerManager) GetContainerState(ctx context.Context, containerID string) (ContainerState, error) {
	container, err := m.client.ContainerInspect(ctx, containerID)
	if err != nil {
		return ContainerStateUnknown, err
	}

	switch container.State.Status {
	case "created":
		return ContainerStateCreated, nil
	case "running":
		return ContainerStateRunning, nil
	case "paused":
		return ContainerStatePaused, nil
	case "restarting":
		return ContainerStateRestarting, nil
	case "removing":
		return ContainerStateRemoving, nil
	case "exited":
		return ContainerStateExited, nil
	case "dead":
		return ContainerStateDead, nil
	default:
		return ContainerStateUnknown, nil
	}
}

// Close implements ContainerManager
func (m *DockerManager) Close() error {
	return m.client.Close()
}

func (m *DockerManager) MonitorEvents(ctx context.Context, eventsChan chan<- ContainerEvent) error {
	msgCh, errCh := m.client.Events(ctx, events.ListOptions{
		Filters: filters.NewArgs(filters.Arg("type", "container")),
	})

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errCh:
			return err
		case msg := <-msgCh:
			event := ContainerEvent{
				ID:   msg.Actor.ID,
				Type: string(msg.Action),
			}
			eventsChan <- event
		}
	}
}

// Helper functions
func dockerStateToContainerState(state string) ContainerState {
	switch state {
	case "created":
		return ContainerStateCreated
	case "running":
		return ContainerStateRunning
	case "paused":
		return ContainerStatePaused
	case "restarting":
		return ContainerStateRestarting
	case "removing":
		return ContainerStateRemoving
	case "exited":
		return ContainerStateExited
	case "dead":
		return ContainerStateDead
	default:
		return ContainerStateUnknown
	}
}

func dockerHealthToHealthState(health *types.Health) HealthState {
	if health == nil {
		return HealthStateNone
	}
	switch health.Status {
	case "healthy":
		return HealthStateHealthy
	case "unhealthy":
		return HealthStateUnhealthy
	case "starting":
		return HealthStateStarting
	default:
		return HealthStateNone
	}
}

func dockerResources(res Resources) container.Resources {
	return container.Resources{
		CPUShares:         res.CPUShares,
		CPUQuota:          res.CPUQuota,
		CPUPeriod:         res.CPUPeriod,
		Memory:            res.Memory,
		MemorySwap:        res.MemorySwap,
		MemoryReservation: res.MemoryReservation,
		OomKillDisable:    &res.OOMKillDisable,
		PidsLimit:         &res.PidsLimit,
	}
}

func calculateCPUPercentage(stats *container.StatsResponse) float64 {
	cpuPercent := 0.0
	cpuDelta := float64(stats.CPUStats.CPUUsage.TotalUsage - stats.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(stats.CPUStats.SystemUsage - stats.PreCPUStats.SystemUsage)

	if systemDelta > 0.0 && cpuDelta > 0.0 {
		cpuPercent = (cpuDelta / systemDelta) * float64(len(stats.CPUStats.CPUUsage.PercpuUsage)) * 100.0
	}
	return cpuPercent
}

func calculateMemoryPercentage(stats *container.StatsResponse) float64 {
	if stats.MemoryStats.Limit > 0 {
		return float64(stats.MemoryStats.Usage) / float64(stats.MemoryStats.Limit) * 100.0
	}
	return 0.0
}

func calculateNetworkRx(stats *container.StatsResponse) uint64 {
	var rx uint64
	for _, network := range stats.Networks {
		rx += network.RxBytes
	}
	return rx
}

func calculateNetworkTx(stats *container.StatsResponse) uint64 {
	var tx uint64
	for _, network := range stats.Networks {
		tx += network.TxBytes
	}
	return tx
}

func (m *DockerManager) containerToInfo(ctx context.Context, c types.Container) (*ContainerInfo, error) {
	createdAt := time.Unix(c.Created, 0)
	info := &ContainerInfo{
		ID:          c.ID,
		Name:        strings.TrimPrefix(c.Names[0], "/"),
		Image:       c.Image,
		State:       dockerStateToContainerState(c.State),
		CreatedAt:   createdAt,
		Labels:      c.Labels,
		NetworkMode: string(c.HostConfig.NetworkMode),
	}

	// Get detailed info for additional fields
	details, err := m.client.ContainerInspect(ctx, c.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect container: %v", err)
	}

	info.Health = dockerHealthToHealthState(details.State.Health)
	info.StartedAt = parseDockerTime(details.State.StartedAt)
	info.FinishedAt = parseDockerTime(details.State.FinishedAt)
	info.ExitCode = details.State.ExitCode
	info.RestartCount = details.RestartCount
	info.Platform = details.Platform
	info.Driver = details.Driver
	info.RestartPolicy = string(details.HostConfig.RestartPolicy.Name)

	// Get IP address and ports
	if details.NetworkSettings != nil {
		for _, network := range details.NetworkSettings.Networks {
			info.IPAddress = network.IPAddress
			break
		}
		info.Ports = make(map[string]string)
		for port, bindings := range details.NetworkSettings.Ports {
			if len(bindings) > 0 {
				info.Ports[port.Port()] = bindings[0].HostPort
			}
		}
	}

	// Get mounts
	info.Mounts = make([]Mount, len(details.Mounts))
	for i, m := range details.Mounts {
		info.Mounts[i] = Mount{
			Source:   m.Source,
			Target:   m.Destination,
			Type:     string(m.Type),
			ReadOnly: !m.RW,
		}
	}

	return info, nil
}

func parseDockerTime(t string) time.Time {
	parsed, _ := time.Parse(time.RFC3339Nano, t)
	return parsed
}

// NewDockerManager creates a new Docker container manager
func NewDockerManager() (*DockerManager, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}
	return &DockerManager{client: cli}, nil
}
