package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	exclude    []string
	detach     bool
	reset      bool
	noWeb      bool
	noServer   bool
	exposePort int
)

func newStartCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the Fleet development stack",
		Long: `Start all Fleet services locally for development.

This command starts:
  - PostgreSQL (database)
  - VictoriaMetrics (metrics storage)
  - Loki (log aggregation)
  - Valkey (caching and rate limiting)
  - Traefik (API gateway)
  - Fleet server`,
		RunE: runStart,
	}

	cmd.Flags().StringSliceVarP(&exclude, "exclude", "e", []string{}, "Services to exclude (e.g., postgres,loki)")
	cmd.Flags().BoolVarP(&detach, "detach", "d", true, "Run services in background")
	cmd.Flags().BoolVar(&reset, "reset", false, "Reset all data and regenerate secrets")
	cmd.Flags().BoolVar(&noWeb, "no-web", false, "Don't start the web UI")
	cmd.Flags().BoolVar(&noServer, "no-server", false, "Don't start the Fleet server")
	cmd.Flags().IntVar(&exposePort, "expose-port", 8080, "Port to expose the Fleet API")

	return cmd
}

func runStart(cmd *cobra.Command, args []string) error {
	// Check Docker availability
	if err := checkDocker(); err != nil {
		return err
	}

	if err := checkDockerCompose(); err != nil {
		return err
	}

	printHeader("Starting Fleet Platform")
	fmt.Println()

	// Get project root
	projectRoot := getProjectRoot()

	// Initialize configuration and secrets
	if err := initializeFleetConfig(projectRoot, reset); err != nil {
		return fmt.Errorf("failed to initialize configuration: %w", err)
	}

	// Load secrets from .env
	secrets, err := loadOrCreateSecrets(projectRoot)
	if err != nil {
		return fmt.Errorf("failed to load secrets: %w", err)
	}

	// Create docker-compose.fleet.yml for Fleet services
	if err := createFleetComposeFile(projectRoot, secrets); err != nil {
		return fmt.Errorf("failed to create Fleet compose file: %w", err)
	}

	// Load services from config
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

	// Add Fleet services unless excluded
	if !noServer {
		services = append(services, "fleet-server")
	}
	if !noWeb {
		services = append(services, "fleet-web")
	}

	// Filter excluded services
	activeServices := filterServices(services, exclude)

	// Build docker-compose command
	composeFiles := []string{
		filepath.Join(projectRoot, "docker-compose.yml"),
	}

	// Add development overrides if they exist
	devFile := filepath.Join(projectRoot, "docker-compose.dev.yml")
	if _, err := os.Stat(devFile); err == nil {
		composeFiles = append(composeFiles, devFile)
	}

	// Add Fleet services compose file
	fleetFile := filepath.Join(projectRoot, "docker-compose.fleet.yml")
	composeFiles = append(composeFiles, fleetFile)

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
	cmdArgs = append(cmdArgs, "up")
	if detach {
		cmdArgs = append(cmdArgs, "-d")
	}
	cmdArgs = append(cmdArgs, activeServices...)

	// Execute docker-compose up
	dockerCmd := exec.Command("docker", cmdArgs...)
	dockerCmd.Dir = projectRoot
	dockerCmd.Stdout = os.Stdout
	dockerCmd.Stderr = os.Stderr

	if err := dockerCmd.Run(); err != nil {
		printError("Failed to start services: %v", err)
		return err
	}

	// Wait for services to be ready
	if detach {
		printInfo("Waiting for services to be ready...")
		time.Sleep(3 * time.Second)
	}

	// Check service health
	if err := checkServicesHealth(activeServices); err != nil {
		printWarning("Some services may not be ready: %v", err)
	}

	// Display success message
	printSuccess("Fleet Platform is running!")
	fmt.Println()

	// Display credentials and URLs
	displayStartupInfo(secrets)

	fmt.Println()
	printHeader("Quick Commands:")
	printInfo("View logs:        fleet logs")
	printInfo("Check status:     fleet status")
	printInfo("Stop platform:    fleet stop")
	printInfo("Provision device: fleet provision --device /dev/diskX")

	return nil
}

func filterServices(services, exclude []string) []string {
	if len(exclude) == 0 {
		return services
	}

	excludeMap := make(map[string]bool)
	for _, s := range exclude {
		excludeMap[strings.ToLower(s)] = true
	}

	var filtered []string
	for _, service := range services {
		if !excludeMap[strings.ToLower(service)] {
			filtered = append(filtered, service)
		}
	}

	return filtered
}

func checkServicesHealth(services []string) error {
	// Basic health check for services
	for _, service := range services {
		printInfo("Checking %s...", service)

		// Check if container is running
		cmd := exec.Command("docker", "compose", "ps", service, "--format", "json")
		cmd.Dir = getProjectRoot()

		output, err := cmd.Output()
		if err != nil {
			return fmt.Errorf("%s health check failed: %v", service, err)
		}

		if strings.Contains(string(output), "running") || strings.Contains(string(output), "Up") {
			printSuccess("%s is running", service)
		} else {
			printWarning("%s may not be ready", service)
		}
	}

	return nil
}

func initializeFleetConfig(projectRoot string, reset bool) error {
	configPath := filepath.Join(projectRoot, "config.toml")

	// Check if config exists and we're not resetting
	if !reset {
		if _, err := os.Stat(configPath); err == nil {
			printInfo("Using existing config.toml")
			return nil
		}
	}

	printInfo("Creating config.toml...")

	// Generate secure secrets
	dbPassword, _ := generateSecureSecret(32)
	jwtSecret, _ := generateSecureSecret(48)

	config := fmt.Sprintf(`# Fleet Platform Configuration
# Generated: %s

[project]
name = "fleet-platform"
environment = "development"

[api]
enabled = true
port = 8080
host = "0.0.0.0"

[database]
url = "postgres://fleetd:%s@postgres:5432/fleetd?sslmode=disable"
max_connections = 25

[auth]
jwt_secret = "%s"
jwt_access_ttl = "15m"
jwt_refresh_ttl = "168h"

[telemetry]
victoria_metrics_url = "http://victoriametrics:8428"
loki_url = "http://loki:3100"

[gateway]
enabled = true
port = 80
dashboard_port = 8080

[stack]
services = ["postgres", "victoriametrics", "loki", "valkey", "traefik"]
`, time.Now().Format(time.RFC3339), dbPassword, jwtSecret)

	return os.WriteFile(configPath, []byte(config), 0o644)
}

func loadOrCreateSecrets(projectRoot string) (map[string]string, error) {
	envPath := filepath.Join(projectRoot, ".env")
	secrets := make(map[string]string)

	// Try to load existing .env
	if file, err := os.Open(envPath); err == nil {
		defer file.Close()
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "#") || line == "" {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				secrets[parts[0]] = parts[1]
			}
		}
	}

	// Generate missing secrets
	if secrets["POSTGRES_PASSWORD"] == "" {
		secrets["POSTGRES_PASSWORD"], _ = generateSecureSecret(32)
	}
	if secrets["JWT_SECRET"] == "" {
		secrets["JWT_SECRET"], _ = generateSecureSecret(48)
	}
	if secrets["GRAFANA_PASSWORD"] == "" {
		secrets["GRAFANA_PASSWORD"], _ = generateSecureSecret(24)
	}
	if secrets["API_KEY"] == "" {
		secrets["API_KEY"], _ = generateSecureSecret(32)
	}

	// Save updated .env
	var envContent strings.Builder
	envContent.WriteString("# Fleet Platform Secrets\n")
	envContent.WriteString("# Generated: " + time.Now().Format(time.RFC3339) + "\n")
	envContent.WriteString("# WARNING: Keep this file secure!\n\n")

	envContent.WriteString("# Database\n")
	envContent.WriteString(fmt.Sprintf("POSTGRES_PASSWORD=%s\n", secrets["POSTGRES_PASSWORD"]))
	envContent.WriteString("POSTGRES_USER=fleetd\n")
	envContent.WriteString("POSTGRES_DB=fleetd\n\n")

	envContent.WriteString("# Authentication\n")
	envContent.WriteString(fmt.Sprintf("JWT_SECRET=%s\n", secrets["JWT_SECRET"]))
	envContent.WriteString(fmt.Sprintf("API_KEY=%s\n", secrets["API_KEY"]))
	envContent.WriteString(fmt.Sprintf("FLEETD_SECRET_KEY=%s\n\n", secrets["JWT_SECRET"]))

	envContent.WriteString("# Monitoring\n")
	envContent.WriteString("GRAFANA_USER=admin\n")
	envContent.WriteString(fmt.Sprintf("GRAFANA_PASSWORD=%s\n\n", secrets["GRAFANA_PASSWORD"]))

	envContent.WriteString("# URLs\n")
	envContent.WriteString("API_URL=http://fleet-server:8080\n")
	envContent.WriteString("WEB_URL=http://localhost:3000\n")

	return secrets, os.WriteFile(envPath, []byte(envContent.String()), 0o600)
}

func createFleetComposeFile(projectRoot string, secrets map[string]string) error {
	compose := fmt.Sprintf(`# Fleet Platform Services
# Auto-generated - do not edit directly
version: '3.8'

services:
  fleet-server:
    image: golang:1.21-alpine
    container_name: fleet-server
    working_dir: /app
    volumes:
      - %s:/app
      - ./fleet.db:/app/fleet.db
    environment:
      - JWT_SECRET=${JWT_SECRET}
      - FLEETD_SECRET_KEY=${FLEETD_SECRET_KEY}
      - DATABASE_URL=postgres://fleetd:${POSTGRES_PASSWORD}@postgres:5432/fleetd?sslmode=disable
      - VALKEY_ADDR=valkey:6379
      - VICTORIA_METRICS_URL=http://victoriametrics:8428
      - LOKI_URL=http://loki:3100
    command: >
      sh -c "
        apk add --no-cache git build-base &&
        go build -o /tmp/fleets ./cmd/fleets &&
        /tmp/fleets server --port 8080 --db /app/fleet.db
      "
    ports:
      - "8080:8080"
    depends_on:
      - postgres
      - valkey
      - victoriametrics
      - loki
    networks:
      - fleet-network
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "wget", "--quiet", "--tries=1", "--spider", "http://localhost:8080/health"]
      interval: 30s
      timeout: 10s
      retries: 3

  fleet-web:
    image: node:20-alpine
    container_name: fleet-web
    working_dir: /app
    volumes:
      - %s/web:/app
    environment:
      - NEXT_PUBLIC_API_URL=http://localhost:8080
      - API_URL=http://fleet-server:8080
    command: >
      sh -c "
        npm install &&
        npm run dev
      "
    ports:
      - "3000:3000"
    depends_on:
      - fleet-server
    networks:
      - fleet-network
    restart: unless-stopped

  # Optional: Fleet Agent for local testing
  fleet-agent:
    image: golang:1.21-alpine
    container_name: fleet-agent
    profiles: ["agent"]
    working_dir: /app
    volumes:
      - %s:/app
      - /var/run/docker.sock:/var/run/docker.sock
    environment:
      - SERVER_URL=http://fleet-server:8080
    command: >
      sh -c "
        apk add --no-cache git build-base &&
        go build -o /tmp/fleetd ./cmd/fleetd &&
        /tmp/fleetd agent --server-url http://fleet-server:8080
      "
    depends_on:
      - fleet-server
    networks:
      - fleet-network
    restart: unless-stopped

networks:
  fleet-network:
    external: true
    name: fleet_default
`, projectRoot, projectRoot, projectRoot)

	fleetComposePath := filepath.Join(projectRoot, "docker-compose.fleet.yml")
	return os.WriteFile(fleetComposePath, []byte(compose), 0o644)
}

func displayStartupInfo(secrets map[string]string) {
	printHeader("🚀 Service URLs")
	fmt.Printf("  %s Dashboard:        %s\n", green("•"), cyan("http://localhost:3000"))
	fmt.Printf("  %s API:              %s\n", green("•"), cyan("http://localhost:8080"))
	fmt.Printf("  %s API Gateway:      %s\n", green("•"), cyan("http://localhost:80"))
	fmt.Printf("  %s Metrics (Victoria): %s\n", green("•"), cyan("http://localhost:8428"))
	fmt.Printf("  %s Logs (Loki):      %s\n", green("•"), cyan("http://localhost:3100"))
	fmt.Printf("  %s Traefik Dashboard: %s\n", green("•"), cyan("http://localhost:8080"))

	fmt.Println()
	printHeader("🔐 Credentials")
	fmt.Printf("  %s JWT Secret:    %s...%s (first/last 4 chars)\n",
		yellow("•"),
		secrets["JWT_SECRET"][:4],
		secrets["JWT_SECRET"][len(secrets["JWT_SECRET"])-4:])
	fmt.Printf("  %s API Key:       %s...%s\n",
		yellow("•"),
		secrets["API_KEY"][:4],
		secrets["API_KEY"][len(secrets["API_KEY"])-4:])
	if grafanaPass, ok := secrets["GRAFANA_PASSWORD"]; ok {
		fmt.Printf("  %s Grafana:       admin / %s\n", yellow("•"), grafanaPass)
	}

	fmt.Println()
	printHeader("📝 Configuration")
	fmt.Printf("  %s Config:        ./config.toml\n", blue("•"))
	fmt.Printf("  %s Secrets:       ./.env\n", blue("•"))
	fmt.Printf("  %s Database:      ./fleet.db\n", blue("•"))
	fmt.Printf("  %s Compose:       ./docker-compose.fleet.yml\n", blue("•"))
}

func displayServiceURLs() {
	// Legacy function - keeping for compatibility
	displayStartupInfo(make(map[string]string))
}

// Helper function to run a command and return error
func runCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
