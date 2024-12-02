package container

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDockerManager_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Create Docker manager
	factory := &DockerFactory{}
	manager, err := factory.Create("docker", nil)
	require.NoError(t, err)
	defer manager.Close()

	ctx := context.Background()

	// Initialize manager
	err = manager.Initialize(ctx)
	require.NoError(t, err)

	// Test container lifecycle
	t.Run("ContainerLifecycle", func(t *testing.T) {
		// Create container
		config := ContainerConfig{
			Name:  "test-container",
			Image: "alpine:latest",
			Command: []string{
				"sh", "-c", "while true; do echo 'test'; sleep 1; done",
			},
			Labels: map[string]string{
				"test": "true",
			},
			Network: NetworkConfig{
				NetworkMode: "bridge",
			},
			RestartPolicy: "no",
		}

		id, err := manager.CreateContainer(ctx, config)
		require.NoError(t, err)
		assert.NotEmpty(t, id)

		// Start container
		err = manager.StartContainer(ctx, id)
		require.NoError(t, err)

		// Wait for container to start
		time.Sleep(2 * time.Second)

		// Get container info
		info, err := manager.InspectContainer(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, id, info.ID)
		assert.Equal(t, "test-container", info.Name)
		assert.Equal(t, ContainerStateRunning, info.State)

		// Get container logs
		logs, err := manager.GetContainerLogs(ctx, id, time.Now().Add(-time.Minute))
		require.NoError(t, err)
		defer logs.Close()

		// Read some logs
		buf := make([]byte, 1024)
		n, err := logs.Read(buf)
		require.NoError(t, err)
		assert.Contains(t, string(buf[:n]), "test")

		// Get container stats
		stats, err := manager.GetContainerStats(ctx, id)
		require.NoError(t, err)
		assert.NotZero(t, stats.CPUPercentage)
		assert.NotZero(t, stats.MemoryUsage)

		// Execute command in container
		exitCode, err := manager.ExecInContainer(ctx, id, []string{"echo", "hello"}, true)
		require.NoError(t, err)
		assert.Equal(t, 0, exitCode)

		// Pause container
		err = manager.PauseContainer(ctx, id)
		require.NoError(t, err)

		// Verify container is paused
		info, err = manager.InspectContainer(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, ContainerStatePaused, info.State)

		// Unpause container
		err = manager.UnpauseContainer(ctx, id)
		require.NoError(t, err)

		// Verify container is running
		info, err = manager.InspectContainer(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, ContainerStateRunning, info.State)

		// Stop container
		err = manager.StopContainer(ctx, id, 10*time.Second)
		require.NoError(t, err)

		// Verify container is stopped
		info, err = manager.InspectContainer(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, ContainerStateExited, info.State)

		// Remove container
		err = manager.RemoveContainer(ctx, id, true)
		require.NoError(t, err)

		// Verify container is removed
		_, err = manager.InspectContainer(ctx, id)
		assert.Error(t, err)
	})

	t.Run("ContainerEvents", func(t *testing.T) {
		// Subscribe to events
		events, err := manager.Events(ctx)
		require.NoError(t, err)

		// Create container
		config := ContainerConfig{
			Name:  "test-events",
			Image: "alpine:latest",
			Command: []string{
				"sh", "-c", "echo 'test' && sleep 1",
			},
		}

		id, err := manager.CreateContainer(ctx, config)
		require.NoError(t, err)

		// Start container
		err = manager.StartContainer(ctx, id)
		require.NoError(t, err)

		// Wait for container to exit
		time.Sleep(2 * time.Second)

		// Remove container
		err = manager.RemoveContainer(ctx, id, true)
		require.NoError(t, err)

		// Collect events
		var receivedEvents []ContainerEvent
		timeout := time.After(5 * time.Second)
	collectEvents:
		for {
			select {
			case event := <-events:
				if event.ID == id {
					receivedEvents = append(receivedEvents, event)
					if event.Type == "destroy" {
						break collectEvents
					}
				}
			case <-timeout:
				break collectEvents
			}
		}

		// Verify events
		assert.NotEmpty(t, receivedEvents)
		eventTypes := make([]string, len(receivedEvents))
		for i, e := range receivedEvents {
			eventTypes[i] = e.Type
		}
		assert.Contains(t, eventTypes, "create")
		assert.Contains(t, eventTypes, "start")
		assert.Contains(t, eventTypes, "die")
		assert.Contains(t, eventTypes, "destroy")
	})

	t.Run("ContainerCopy", func(t *testing.T) {
		// Create container
		config := ContainerConfig{
			Name:  "test-copy",
			Image: "alpine:latest",
			Command: []string{
				"sh", "-c", "while true; do sleep 1; done",
			},
		}

		id, err := manager.CreateContainer(ctx, config)
		require.NoError(t, err)

		// Start container
		err = manager.StartContainer(ctx, id)
		require.NoError(t, err)

		// Copy file to container
		content := strings.NewReader("test content")
		err = manager.CopyToContainer(ctx, id, "/tmp/test.txt", content)
		require.NoError(t, err)

		// Verify file exists
		exitCode, err := manager.ExecInContainer(ctx, id, []string{"cat", "/tmp/test.txt"}, true)
		require.NoError(t, err)
		assert.Equal(t, 0, exitCode)

		// Copy file from container
		reader, err := manager.CopyFromContainer(ctx, id, "/tmp/test.txt")
		require.NoError(t, err)
		defer reader.Close()

		// Read file content
		buf := make([]byte, 1024)
		n, err := reader.Read(buf)
		require.NoError(t, err)
		assert.Contains(t, string(buf[:n]), "test content")

		// Cleanup
		err = manager.RemoveContainer(ctx, id, true)
		require.NoError(t, err)
	})

	t.Run("ContainerPrune", func(t *testing.T) {
		// Create stopped container
		config := ContainerConfig{
			Name:  "test-prune",
			Image: "alpine:latest",
			Command: []string{
				"sh", "-c", "exit 0",
			},
		}

		id, err := manager.CreateContainer(ctx, config)
		require.NoError(t, err)

		// Start and wait for container to exit
		err = manager.StartContainer(ctx, id)
		require.NoError(t, err)
		time.Sleep(2 * time.Second)

		// Prune containers
		err = manager.PruneContainers(ctx)
		require.NoError(t, err)

		// Verify container is removed
		_, err = manager.InspectContainer(ctx, id)
		assert.Error(t, err)
	})
}

func TestDockerManager_Unit(t *testing.T) {
	t.Run("StateConversion", func(t *testing.T) {
		tests := []struct {
			dockerState string
			want        ContainerState
		}{
			{"created", ContainerStateCreated},
			{"running", ContainerStateRunning},
			{"paused", ContainerStatePaused},
			{"restarting", ContainerStateRestarting},
			{"removing", ContainerStateRemoving},
			{"exited", ContainerStateExited},
			{"dead", ContainerStateDead},
			{"unknown", ContainerStateStopped},
		}

		for _, tt := range tests {
			t.Run(tt.dockerState, func(t *testing.T) {
				got := dockerStateToContainerState(tt.dockerState)
				assert.Equal(t, tt.want, got)
			})
		}
	})

	t.Run("ResourceConversion", func(t *testing.T) {
		resources := Resources{
			CPUShares:         1024,
			CPUQuota:          100000,
			CPUPeriod:         100000,
			Memory:            1024 * 1024 * 1024,
			MemorySwap:        2 * 1024 * 1024 * 1024,
			MemoryReservation: 512 * 1024 * 1024,
			OOMKillDisable:    true,
			PidsLimit:         1000,
		}

		dockerRes := dockerResources(resources)
		assert.Equal(t, resources.CPUShares, dockerRes.CPUShares)
		assert.Equal(t, resources.CPUQuota, dockerRes.CPUQuota)
		assert.Equal(t, resources.CPUPeriod, dockerRes.CPUPeriod)
		assert.Equal(t, resources.Memory, dockerRes.Memory)
		assert.Equal(t, resources.MemorySwap, dockerRes.MemorySwap)
		assert.Equal(t, resources.MemoryReservation, dockerRes.MemoryReservation)
		assert.Equal(t, resources.OOMKillDisable, *dockerRes.OomKillDisable)
		assert.Equal(t, resources.PidsLimit, *dockerRes.PidsLimit)
	})
}

func TestDockerFactory_Create(t *testing.T) {
	factory := &DockerFactory{}

	t.Run("ValidRuntime", func(t *testing.T) {
		manager, err := factory.Create("docker", map[string]interface{}{
			"host":    "unix:///var/run/docker.sock",
			"version": "1.41",
		})
		require.NoError(t, err)
		assert.NotNil(t, manager)
		manager.Close()
	})

	t.Run("InvalidRuntime", func(t *testing.T) {
		manager, err := factory.Create("invalid", nil)
		assert.Error(t, err)
		assert.Nil(t, manager)
	})
}
