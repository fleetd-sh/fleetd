package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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

	if err := checkDockerCompose(); err != nil {
		return err
	}

	printHeader("Fleet Service Status")
	fmt.Println()

	// Get project root
	projectRoot := getProjectRoot()

	// Get list of expected services
	services := viper.GetStringSlice("stack.services")
	if len(services) == 0 {
		services = []string{
			"postgres",
			"victoriametrics",
			"loki",
			"valkey",
			"traefik",
		}
	}

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

	// Build command to get container status
	cmdArgs := []string{"compose"}
	for _, file := range composeFiles {
		if _, err := os.Stat(file); err == nil {
			cmdArgs = append(cmdArgs, "-f", file)
		}
	}
	cmdArgs = append(cmdArgs, "ps", "--format", "json")

	// Execute docker-compose ps
	dockerCmd := exec.Command("docker", cmdArgs...)
	dockerCmd.Dir = projectRoot

	output, err := dockerCmd.Output()
	if err != nil {
		printError("Failed to get service status: %v", err)
		return err
	}

	// Parse JSON output
	var containers []ContainerStatus
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		var container ContainerStatus
		if err := json.Unmarshal([]byte(line), &container); err == nil {
			containers = append(containers, container)
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
	fmt.Println("  • View logs:        fleet logs [service]")
	fmt.Println("  • Restart service:  fleet restart [service]")
	fmt.Println("  • Stop all:         fleet stop")
	fmt.Println("  • Reset all:        fleet reset")

	return nil
}

func displayServiceStatus(name string, container ContainerStatus) {
	var statusIcon, statusColor string

	switch strings.ToLower(container.State) {
	case "running":
		statusIcon = green("✓")
		statusColor = green(container.State)
	case "restarting":
		statusIcon = yellow("↻")
		statusColor = yellow(container.State)
	case "exited", "dead":
		statusIcon = red("✗")
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
	fmt.Printf("%s %-20s %s\n", red("✗"), bold(name), red("not running"))
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
			fmt.Printf("  %s %-15s %s %d\n", green("✓"), service, green("listening on"), port)
		} else {
			fmt.Printf("  %s %-15s %s %d\n", yellow("⚠"), service, yellow("not available on"), port)
		}
	}
}

func isPortOpen(port int) bool {
	// Simple check using nc (netcat)
	cmd := exec.Command("nc", "-z", "localhost", fmt.Sprintf("%d", port))
	err := cmd.Run()
	return err == nil
}
