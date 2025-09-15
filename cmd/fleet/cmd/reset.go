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

	// Build docker-compose command
	composeFiles := []string{
		filepath.Join(projectRoot, "docker-compose.yml"),
		filepath.Join(projectRoot, "docker-compose.dev.yml"),
	}

	// Check if gateway compose file exists
	gatewayFile := filepath.Join(projectRoot, "docker-compose.gateway.yml")
	if _, err := os.Stat(gatewayFile); err == nil {
		composeFiles = append(composeFiles, gatewayFile)
	}

	// Stop all services
	printInfo("Stopping all services...")
	cmdArgs := []string{"compose"}
	for _, file := range composeFiles {
		if _, err := os.Stat(file); err == nil {
			cmdArgs = append(cmdArgs, "-f", file)
		}
	}
	cmdArgs = append(cmdArgs, "down")

	if removeVolumes {
		cmdArgs = append(cmdArgs, "-v")
	}

	dockerCmd := exec.Command("docker", cmdArgs...)
	dockerCmd.Dir = projectRoot
	dockerCmd.Stdout = os.Stdout
	dockerCmd.Stderr = os.Stderr

	if err := dockerCmd.Run(); err != nil {
		printError("Failed to stop services: %v", err)
		return err
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
	printInfo("Run 'fleet start' to start fresh")

	return nil
}
