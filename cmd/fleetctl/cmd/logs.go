package cmd

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
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

	// Create Docker client
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("failed to create Docker client: %w", err)
	}
	defer cli.Close()

	// Configure log options
	logOptions := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Timestamps: false,
		Follow:     follow,
	}

	if tail > 0 {
		logOptions.Tail = fmt.Sprintf("%d", tail)
	}

	// Handle interrupt signal to stop following logs
	if follow {
		printInfo("Following logs... Press Ctrl+C to stop")
		fmt.Println()
	}

	// Get logs from container
	logReader, err := cli.ContainerLogs(ctx, containerName, logOptions)
	if err != nil {
		printError("Failed to get logs for %s: %v", service, err)
		printInfo("Available services: platform-api, device-api, postgres, valkey, victoriametrics, loki, traefik, studio")
		return err
	}
	defer logReader.Close()

	// Copy logs to stdout
	_, err = io.Copy(os.Stdout, logReader)
	if err != nil && err != io.EOF {
		return fmt.Errorf("error reading logs: %w", err)
	}

	return nil
}
