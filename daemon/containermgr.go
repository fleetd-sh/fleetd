package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
)

type ContainerManager struct {
	client *client.Client
	config *Config
	stopCh chan struct{}
}

func NewContainerManager(cfg *Config) (*ContainerManager, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return nil, err
	}

	return &ContainerManager{
		client: cli,
		config: cfg,
		stopCh: make(chan struct{}),
	}, nil
}

func (cm *ContainerManager) Start() {
	slog.Info("Container manager started")
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := cm.ensureContainerRunning(); err != nil {
				slog.With("error", err).Error("Error ensuring container is running")
			}
		case <-cm.stopCh:
			slog.Info("Container manager stopped")
			return
		}
	}
}

func (cm *ContainerManager) Stop() {
	close(cm.stopCh)
}

func (cm *ContainerManager) ensureContainerRunning() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	containers, err := cm.client.ContainerList(ctx, container.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list containers: %w", err)
	}

	for _, c := range containers {
		if c.Image == cm.config.ContainerImage {
			if c.State == "running" {
				return nil // Container is already running
			}
			break
		}
	}

	// Container not found or not running, start it
	resp, err := cm.client.ContainerCreate(ctx, &container.Config{
		Image: cm.config.ContainerImage,
	}, nil, nil, nil, "")
	if err != nil {
		return fmt.Errorf("failed to create container: %w", err)
	}

	if err := cm.client.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	return nil
}

func (cm *ContainerManager) UpdateContainer(ctx context.Context, containerName, newImage string) error {
	// Pull the new image
	_, err := cm.client.ImagePull(ctx, newImage, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("failed to pull new image: %w", err)
	}

	// Stop the existing container
	if err := cm.client.ContainerStop(ctx, containerName, container.StopOptions{}); err != nil {
		return fmt.Errorf("failed to stop container: %w", err)
	}

	// Remove the existing container
	if err := cm.client.ContainerRemove(ctx, containerName, container.RemoveOptions{}); err != nil {
		return fmt.Errorf("failed to remove container: %w", err)
	}

	// Create a new container with the updated image
	resp, err := cm.client.ContainerCreate(ctx, &container.Config{
		Image: newImage,
	}, nil, nil, nil, containerName)
	if err != nil {
		return fmt.Errorf("failed to create new container: %w", err)
	}

	// Start the new container
	if err := cm.client.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("failed to start new container: %w", err)
	}

	slog.With("container_name", containerName, "new_image", newImage).Info("Container updated successfully")
	return nil
}

func (cm *ContainerManager) ManageTenantContainer(ctx context.Context, tenantID, containerImage string) error {
	containerName := fmt.Sprintf("tenant-%s", tenantID)

	// Check if the container already exists
	containers, err := cm.client.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return fmt.Errorf("failed to list containers: %w", err)
	}

	var existingContainer types.Container
	for _, c := range containers {
		if c.Names[0] == "/"+containerName {
			existingContainer = c
			break
		}
	}

	if existingContainer.ID != "" {
		// Container exists, update if necessary
		if existingContainer.Image != containerImage {
			return cm.UpdateContainer(ctx, containerName, containerImage)
		}
		// Ensure the container is running
		if existingContainer.State != "running" {
			slog.With("tenantID", tenantID, "containerName", containerName).Info("Starting existing container")
			return cm.client.ContainerStart(ctx, existingContainer.ID, container.StartOptions{})
		}
		slog.With("tenantID", tenantID, "containerName", containerName).Info("Container already running and up-to-date")
		return nil
	}

	// Create and start a new container
	resp, err := cm.client.ContainerCreate(ctx, &container.Config{
		Image: containerImage,
	}, nil, nil, nil, containerName)
	if err != nil {
		return fmt.Errorf("failed to create tenant container: %w", err)
	}

	if err := cm.client.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("failed to start tenant container: %w", err)
	}

	slog.With("tenant_id", tenantID, "container_image", containerImage).Info("Tenant container created and started")
	return nil
}
