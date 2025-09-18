package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

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

	// Stop and remove all fleetd containers
	for _, service := range services {
		containerName := "fleetd-" + service

		// Check if container exists
		checkCmd := exec.Command("docker", "ps", "-a", "--filter", "name="+containerName, "--format", "{{.Names}}")
		output, _ := checkCmd.Output()

		if output != nil && len(output) > 0 {
			printInfo("Removing %s...", service)
			exec.Command("docker", "stop", containerName).Run()
			exec.Command("docker", "rm", containerName).Run()
		}
	}

	// Remove network
	printInfo("Removing Docker network...")
	exec.Command("docker", "network", "rm", "fleetd-network").Run()

	if removeVolumes {
		printInfo("Removing Docker volumes...")
		volumes := []string{
			"fleetd-postgres-data",
			"fleetd-valkey-data",
			"fleetd-metrics-data",
			"fleetd-loki-data",
		}
		for _, volume := range volumes {
			exec.Command("docker", "volume", "rm", volume).Run()
		}
	}

	// Clean up dangling images
	printInfo("Cleaning up dangling images...")
	pruneCmd := exec.Command("docker", "image", "prune", "-f")
	if err := pruneCmd.Run(); err != nil {
		printWarning("Failed to prune images: %v", err)
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
