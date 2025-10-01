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

	cmd.Flags().BoolVar(&removeVolumes, "volumes", false, "Remove volumes (data will be lost)")

	return cmd
}

func runStop(cmd *cobra.Command, args []string, removeVolumes bool) error {
	// Check Docker availability
	if err := checkDocker(); err != nil {
		return err
	}

	// Check Docker Compose availability
	if err := checkDockerCompose(); err != nil {
		return err
	}

	printHeader("Stopping Fleet development stack...")

	// Get project root
	projectRoot := getProjectRoot()
	composeFile := filepath.Join(projectRoot, "docker", "docker-compose.yaml")

	// Build docker-compose command
	composeArgs := []string{"compose", "-f", composeFile, "down"}

	if removeVolumes {
		printWarning("Removing volumes - all data will be lost!")
		composeArgs = append(composeArgs, "-v")
	}

	// Stop services with docker-compose
	stopCmd := exec.Command("docker", composeArgs...)
	stopCmd.Stdout = os.Stdout
	stopCmd.Stderr = os.Stderr
	stopCmd.Dir = projectRoot

	if err := stopCmd.Run(); err != nil {
		// Fallback to manual cleanup if docker-compose fails
		printWarning("Docker Compose failed, attempting manual cleanup...")
		return manualCleanup(removeVolumes)
	}

	if !removeVolumes {
		printInfo("Data volumes preserved. Use 'fleetctl stop --volumes' to remove them")
	}

	printSuccess("Fleet development stack stopped")
	return nil
}

func manualCleanup(removeVolumes bool) error {
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

	// Stop all fleetd containers
	for _, service := range services {
		containerName := "fleetd-" + service

		// Check if container exists
		checkCmd := exec.Command("docker", "ps", "-a", "--filter", "name="+containerName, "--format", "{{.Names}}")
		output, _ := checkCmd.Output()

		if output != nil && len(output) > 0 {
			printInfo("Stopping %s...", service)
			stopCmd := exec.Command("docker", "stop", containerName)
			stopCmd.Run()

			// Remove container
			rmCmd := exec.Command("docker", "rm", containerName)
			rmCmd.Run()
		}
	}

	// Remove network
	printInfo("Removing Docker network...")
	exec.Command("docker", "network", "rm", "fleetd-network").Run()

	if removeVolumes {
		volumes := []string{
			"fleetd_postgres_data",
			"fleetd_valkey_data",
			"fleetd_victoria_data",
			"fleetd_loki_data",
		}

		for _, volume := range volumes {
			printInfo("Removing volume %s...", volume)
			exec.Command("docker", "volume", "rm", volume).Run()
		}
	}

	return nil
}
