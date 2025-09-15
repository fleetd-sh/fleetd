package cmd

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

func newStopCmd() *cobra.Command {
	var removeVolumes bool

	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop the Fleet development stack",
		Long:  `Stop all running Fleet services.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStop(cmd, args, removeVolumes)
		},
	}

	cmd.Flags().BoolVarP(&removeVolumes, "volumes", "v", false, "Remove volumes (data will be lost)")

	return cmd
}

func runStop(cmd *cobra.Command, args []string, removeVolumes bool) error {
	// Check Docker availability
	if err := checkDocker(); err != nil {
		return err
	}

	if err := checkDockerCompose(); err != nil {
		return err
	}

	printHeader("Stopping Fleet development stack...")

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

	// Build command
	cmdArgs := []string{"compose"}
	for _, file := range composeFiles {
		if _, err := os.Stat(file); err == nil {
			cmdArgs = append(cmdArgs, "-f", file)
		}
	}
	cmdArgs = append(cmdArgs, "down")

	if removeVolumes {
		cmdArgs = append(cmdArgs, "-v")
		printWarning("Removing volumes - all data will be lost!")
	}

	// Execute docker-compose down
	dockerCmd := exec.Command("docker", cmdArgs...)
	dockerCmd.Dir = projectRoot
	dockerCmd.Stdout = os.Stdout
	dockerCmd.Stderr = os.Stderr

	if err := dockerCmd.Run(); err != nil {
		printError("Failed to stop services: %v", err)
		return err
	}

	printSuccess("Fleet development stack stopped")

	if removeVolumes {
		printInfo("All volumes have been removed")
	} else {
		printInfo("Data volumes preserved. Use 'fleet stop --volumes' to remove them")
	}

	return nil
}
