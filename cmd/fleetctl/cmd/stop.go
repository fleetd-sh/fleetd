package cmd

import (
	"os/exec"

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

	printHeader("Stopping Fleet development stack...")

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
		printWarning("Removing volumes - all data will be lost!")

		volumes := []string{
			"fleetd-postgres-data",
			"fleetd-valkey-data",
			"fleetd-metrics-data",
			"fleetd-loki-data",
		}

		for _, volume := range volumes {
			printInfo("Removing volume %s...", volume)
			exec.Command("docker", "volume", "rm", volume).Run()
		}

		printInfo("All volumes have been removed")
	} else {
		printInfo("Data volumes preserved. Use 'fleetctl stop --volumes' to remove them")
	}

	printSuccess("Fleet development stack stopped")
	return nil
}
