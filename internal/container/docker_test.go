package container

import (
	"context"
	"fmt"
	"io"
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

	ctx := context.Background()
	manager, err := NewDockerManager()
	require.NoError(t, err)

	// Clean up any existing containers
	containers, err := manager.ListContainers(ctx, nil)
	require.NoError(t, err)
	for _, c := range containers {
		if strings.HasPrefix(c.Name, "test-container-") {
			_ = manager.RemoveContainer(ctx, c.ID, false)
		}
	}

	t.Run("ContainerLifecycle", func(t *testing.T) {
		containerName := fmt.Sprintf("test-container-%d", time.Now().UnixNano())

		// Create container
		id, err := manager.CreateContainer(ctx, ContainerConfig{
			Name:    containerName,
			Image:   "alpine:latest",
			Command: []string{"sleep", "300"},
		})
		require.NoError(t, err)

		// Start container
		err = manager.StartContainer(ctx, id)
		require.NoError(t, err)

		// Get container state
		state, err := manager.GetContainerState(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, ContainerStateRunning, state)

		// Stop container
		err = manager.StopContainer(ctx, id, 10*time.Second)
		require.NoError(t, err)

		// Verify exited state
		state, err = manager.GetContainerState(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, ContainerStateExited, state)

		// Remove container
		err = manager.RemoveContainer(ctx, id, false)
		require.NoError(t, err)
	})

	t.Run("ContainerEvents", func(t *testing.T) {
		containerName := fmt.Sprintf("test-container-%d", time.Now().UnixNano())

		// Create event channel
		events := make(chan ContainerEvent)
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		// Start event monitoring
		go func() {
			err := manager.MonitorEvents(ctx, events)
			require.NoError(t, err)
		}()

		// Create and start container
		id, err := manager.CreateContainer(ctx, ContainerConfig{
			Name:    containerName,
			Image:   "alpine:latest",
			Command: []string{"sleep", "300"},
		})
		require.NoError(t, err)

		// Wait for create event
		event := <-events
		assert.Equal(t, id, event.ID)
		assert.Equal(t, ContainerEventType("create"), ContainerEventType(event.Type))

		// Start container
		err = manager.StartContainer(ctx, id)
		require.NoError(t, err)

		// Wait for start event
		event = <-events
		assert.Equal(t, id, event.ID)
		assert.Equal(t, ContainerEventType("start"), ContainerEventType(event.Type))

		// Cleanup
		_ = manager.StopContainer(ctx, id, 10*time.Second)
		_ = manager.RemoveContainer(ctx, id, false)
	})

	t.Run("ContainerCopy", func(t *testing.T) {
		containerName := fmt.Sprintf("test-container-%d", time.Now().UnixNano())

		// Create container with test file
		id, err := manager.CreateContainer(ctx, ContainerConfig{
			Name:    containerName,
			Image:   "alpine:latest",
			Command: []string{"sh", "-c", "echo 'test data' > /test.txt && sleep 300"},
		})
		require.NoError(t, err)

		// Start container
		err = manager.StartContainer(ctx, id)
		require.NoError(t, err)

		// Wait for file creation
		time.Sleep(5 * time.Millisecond)

		// Copy file from container
		reader, err := manager.CopyFromContainer(ctx, id, "/test.txt")
		require.NoError(t, err)
		defer reader.Close()

		data, err := io.ReadAll(reader)
		require.NoError(t, err)
		assert.Contains(t, string(data), "test data")

		// Cleanup
		_ = manager.StopContainer(ctx, id, 10*time.Second)
		_ = manager.RemoveContainer(ctx, id, false)
	})

	t.Run("ContainerPrune", func(t *testing.T) {
		containerName := fmt.Sprintf("test-container-%d", time.Now().UnixNano())

		// Create stopped container
		id, err := manager.CreateContainer(ctx, ContainerConfig{
			Name:    containerName,
			Image:   "alpine:latest",
			Command: []string{"echo", "test"},
		})
		require.NoError(t, err)

		// Start and wait for container to exit
		err = manager.StartContainer(ctx, id)
		require.NoError(t, err)

		// Wait for container to exit
		time.Sleep(5 * time.Millisecond)

		// Prune containers
		err = manager.PruneContainers(ctx)
		require.NoError(t, err)

		// Verify container was removed
		_, err = manager.GetContainerState(ctx, id)
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
			{"unknown", ContainerStateUnknown},
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
