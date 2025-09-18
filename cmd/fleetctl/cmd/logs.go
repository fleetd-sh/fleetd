package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

var (
	follow bool
	tail   int
)

func newLogsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logs [service]",
		Short: "View logs from Fleet services",
		Long: `View logs from Fleet services.

If no service is specified, shows logs from all services.
Use --follow to tail logs in real-time.`,
		RunE: runLogs,
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return []string{
				"platform-api",
				"device-api",
				"postgres",
				"valkey",
				"victoriametrics",
				"loki",
				"traefik",
				"studio",
			}, cobra.ShellCompDirectiveNoFileComp
		},
	}

	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Follow log output")
	cmd.Flags().IntVarP(&tail, "tail", "n", 100, "Number of lines to show from the end of logs")

	return cmd
}

func runLogs(cmd *cobra.Command, args []string) error {
	// Check Docker availability
	if err := checkDocker(); err != nil {
		return err
	}

	var service string
	if len(args) > 0 {
		service = args[0]
		printHeader(fmt.Sprintf("Logs for %s", service))
	} else {
		// Default to platform-api if no service specified
		service = "platform-api"
		printHeader("Logs for platform-api (use 'fleetctl logs <service>' to view other services)")
	}

	// Build container name
	containerName := fmt.Sprintf("fleetd-%s", service)

	// Build docker logs command
	cmdArgs := []string{"logs"}

	// Add tail option
	if tail > 0 {
		cmdArgs = append(cmdArgs, "--tail", fmt.Sprintf("%d", tail))
	}

	// Add follow option
	if follow {
		cmdArgs = append(cmdArgs, "-f")
	}

	// Add container name
	cmdArgs = append(cmdArgs, containerName)

	// Execute docker logs
	dockerCmd := exec.Command("docker", cmdArgs...)
	dockerCmd.Stdout = os.Stdout
	dockerCmd.Stderr = os.Stderr

	// Handle interrupt signal to stop following logs
	if follow {
		printInfo("Following logs... Press Ctrl+C to stop")
		fmt.Println()
	}

	if err := dockerCmd.Run(); err != nil {
		// Check if it's just a Ctrl+C interrupt
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 130 {
			fmt.Println()
			printInfo("Stopped following logs")
			return nil
		}
		printError("Failed to get logs for %s: %v", service, err)
		printInfo("Available services: platform-api, device-api, postgres, valkey, victoriametrics, loki, traefik, studio")
		return err
	}

	return nil
}
