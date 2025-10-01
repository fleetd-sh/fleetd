package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/spf13/cobra"
)

func newResetCmd() *cobra.Command {
	var removeVolumes bool

	cmd := &cobra.Command{
		Use:   "reset",
		Short: "Reset Fleet development environment",
		Long: `Reset the Fleet development environment by stopping all services,
removing containers, volumes, and optionally cleaning up data.

This is useful when you want a fresh start or are experiencing issues.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReset(cmd, args, removeVolumes)
		},
	}

	cmd.Flags().BoolVar(&removeVolumes, "volumes", true, "Remove volumes (data will be lost)")

	return cmd
}

func runReset(cmd *cobra.Command, args []string, removeVolumes bool) error {
	printHeader("Resetting Fleet Development Environment")
	printWarning("This will stop all services and remove containers")

	if removeVolumes {
		printWarning("All data volumes will be removed!")
	}

	fmt.Print("\nContinue? (y/N): ")
	var confirm string
	fmt.Scanln(&confirm)
	if confirm != "y" && confirm != "Y" {
		printInfo("Reset cancelled")
		return nil
	}

	fmt.Println()

	// Get project root
	projectRoot := getProjectRoot()

	// Stop all services
	printInfo("Stopping all services...")

	// List of services to stop
	services := []string{
		"platform-api",
		"device-api",
		"studio",
		"postgres",
		"valkey",
		"victoriametrics",
		"loki",
		"traefik",
	}

	// Create Docker client
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("failed to create Docker client: %w", err)
	}
	defer cli.Close()

	// Stop and remove all fleetd containers
	for _, service := range services {
		containerName := "fleetd-" + service

		// List containers with filter
		filterArgs := filters.NewArgs()
		filterArgs.Add("name", containerName)
		containers, _ := cli.ContainerList(ctx, container.ListOptions{
			All:     true,
			Filters: filterArgs,
		})

		for _, cnt := range containers {
			printInfo("Removing %s...", service)
			// Stop container if running
			if cnt.State == "running" {
				if err := cli.ContainerStop(ctx, cnt.ID, container.StopOptions{}); err != nil {
					printWarning("Failed to stop %s: %v", service, err)
				}
			}
			// Remove container
			if err := cli.ContainerRemove(ctx, cnt.ID, container.RemoveOptions{Force: true}); err != nil {
				printWarning("Failed to remove %s: %v", service, err)
			}
		}
	}

	// Remove network
	printInfo("Removing Docker network...")
	if err := cli.NetworkRemove(ctx, "fleetd-network"); err != nil {
		if !strings.Contains(err.Error(), "not found") {
			printWarning("Failed to remove network: %v", err)
		}
	}

	if removeVolumes {
		printInfo("Removing Docker volumes...")
		volumes := []string{
			"fleetd_postgres_data",
			"fleetd_valkey_data",
			"fleetd_victoria_data",
			"fleetd_loki_data",
		}
		for _, volumeName := range volumes {
			if err := cli.VolumeRemove(ctx, volumeName, true); err != nil {
				if !strings.Contains(err.Error(), "no such volume") {
					printWarning("Failed to remove volume %s: %v", volumeName, err)
				}
			}
		}
	}

	// Clean up dangling images
	printInfo("Cleaning up dangling images...")
	pruneReport, err := cli.ImagesPrune(ctx, filters.NewArgs())
	if err != nil {
		printWarning("Failed to prune images: %v", err)
	} else if len(pruneReport.ImagesDeleted) > 0 {
		printInfo("Removed %d dangling images", len(pruneReport.ImagesDeleted))
	}

	// Clean up data directory if it exists
	dataDir := filepath.Join(projectRoot, "data")
	if _, err := os.Stat(dataDir); err == nil {
		printInfo("Removing data directory...")
		if err := os.RemoveAll(dataDir); err != nil {
			printWarning("Failed to remove data directory: %v", err)
		}
	}

	printSuccess("Fleet environment reset complete!")
	fmt.Println()
	printInfo("Run 'fleetctl start' to start fresh")

	return nil
}
