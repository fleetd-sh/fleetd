package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

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
				"postgres",
				"victoriametrics",
				"loki",
				"valkey",
				"traefik",
				"fleet-server",
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

	if err := checkDockerCompose(); err != nil {
		return err
	}

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
	cmdArgs = append(cmdArgs, "logs")

	// Add tail option
	if tail > 0 {
		cmdArgs = append(cmdArgs, "--tail", fmt.Sprintf("%d", tail))
	}

	// Add follow option
	if follow {
		cmdArgs = append(cmdArgs, "-f")
	}

	// Add service name if specified
	if len(args) > 0 {
		cmdArgs = append(cmdArgs, args[0])
		printHeader(fmt.Sprintf("Logs for %s", args[0]))
	} else {
		printHeader("Logs for all services")
	}

	// Execute docker-compose logs
	dockerCmd := exec.Command("docker", cmdArgs...)
	dockerCmd.Dir = projectRoot
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
		printError("Failed to get logs: %v", err)
		return err
	}

	return nil
}
