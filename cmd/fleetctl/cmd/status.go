package cmd

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type ContainerStatus struct {
	Name    string `json:"Name"`
	State   string `json:"State"`
	Status  string `json:"Status"`
	Service string `json:"Service"`
}

func newStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show status of Fleet services",
		Long:  `Display the status of all Fleet services including health checks and resource usage.`,
		RunE:  runStatus,
	}

	return cmd
}

func runStatus(cmd *cobra.Command, args []string) error {
	// Check Docker availability
	if err := checkDocker(); err != nil {
		return err
	}

	printHeader("Fleet Service Status")
	fmt.Println()

	// List of expected services
	services := []string{
		"platform-api",
		"device-api",
		"postgres",
		"valkey",
		"victoriametrics",
		"loki",
		"traefik",
		"studio",
	}

	// Create Docker client
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("failed to create Docker client: %w", err)
	}
	defer cli.Close()

	// Get status of each container
	var containers []ContainerStatus
	for _, service := range services {
		containerName := fmt.Sprintf("fleetd-%s", service)

		// List containers with filter
		filterArgs := filters.NewArgs()
		filterArgs.Add("name", containerName)
		containerList, err := cli.ContainerList(ctx, container.ListOptions{
			All:     true,
			Filters: filterArgs,
		})

		if err == nil && len(containerList) > 0 {
			container := containerList[0]
			containers = append(containers, ContainerStatus{
				Name:    strings.TrimPrefix(container.Names[0], "/"),
				State:   container.State,
				Status:  container.Status,
				Service: service,
			})
		}
	}

	// Display status for each service
	serviceStatus := make(map[string]ContainerStatus)
	for _, container := range containers {
		serviceStatus[container.Service] = container
	}

	// Check each expected service
	for _, service := range services {
		if container, exists := serviceStatus[service]; exists {
			displayServiceStatus(service, container)
		} else {
			displayServiceMissing(service)
		}
	}

	fmt.Println()

	// Check port availability
	printHeader("Port Status")
	checkPorts()

	fmt.Println()

	// Display quick actions
	printHeader("Quick Actions")
	fmt.Println("  • View logs:        fleetctl logs [service]")
	fmt.Println("  • Start services:   fleetctl start")
	fmt.Println("  • Stop all:         fleetctl stop")
	fmt.Println("  • Reset all:        fleetctl stop --volumes")

	return nil
}

func displayServiceStatus(name string, container ContainerStatus) {
	var statusIcon, statusColor string

	switch strings.ToLower(container.State) {
	case "running":
		statusIcon = green("[OK]")
		statusColor = green(container.State)
	case "restarting":
		statusIcon = yellow("↻")
		statusColor = yellow(container.State)
	case "exited", "dead":
		statusIcon = red("[X]")
		statusColor = red(container.State)
	default:
		statusIcon = yellow("?")
		statusColor = yellow(container.State)
	}

	fmt.Printf("%s %-20s %s", statusIcon, bold(name), statusColor)

	if container.Status != "" {
		fmt.Printf(" (%s)", container.Status)
	}

	fmt.Println()
}

func displayServiceMissing(name string) {
	fmt.Printf("%s %-20s %s\n", red("[X]"), bold(name), red("not running"))
}

func checkPorts() {
	ports := map[string]int{
		"API":      viper.GetInt("api.port"),
		"Gateway":  viper.GetInt("gateway.port"),
		"Metrics":  viper.GetInt("telemetry.victoria_metrics_port"),
		"Logs":     viper.GetInt("telemetry.loki_port"),
		"Grafana":  viper.GetInt("telemetry.grafana_port"),
		"Postgres": viper.GetInt("db.port"),
	}

	// Set defaults if not configured
	if ports["API"] == 0 {
		ports["API"] = 8080
	}
	if ports["Gateway"] == 0 {
		ports["Gateway"] = 80
	}
	if ports["Metrics"] == 0 {
		ports["Metrics"] = 8428
	}
	if ports["Logs"] == 0 {
		ports["Logs"] = 3100
	}
	if ports["Grafana"] == 0 {
		ports["Grafana"] = 3001
	}
	if ports["Postgres"] == 0 {
		ports["Postgres"] = 5432
	}

	for service, port := range ports {
		if isPortOpen(port) {
			fmt.Printf("  %s %-15s %s %d\n", green("[OK]"), service, green("listening on"), port)
		} else {
			fmt.Printf("  %s %-15s %s %d\n", yellow("[WARN]"), service, yellow("not available on"), port)
		}
	}
}

func isPortOpen(port int) bool {
	// Check if port is open by attempting to connect
	addr := fmt.Sprintf("localhost:%d", port)
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}
