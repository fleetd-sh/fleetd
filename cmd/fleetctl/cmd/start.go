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

	// Create docker network
	if err := createDockerNetwork(); err != nil {
		return fmt.Errorf("failed to create network: %w", err)
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
		services = append(services, "platform-api", "device-api")
	}
	if !noWeb {
		services = append(services, "studio")
	}

	// Filter excluded services
	activeServices := filterServices(services, exclude)

	// Start services
	for _, service := range activeServices {
		printInfo("Starting %s...", service)
		if err := startService(service, secrets, projectRoot); err != nil {
			printError("Failed to start %s: %v", service, err)
			return err
		}
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

	// Run database migrations
	if contains(activeServices, "postgres") && (contains(activeServices, "platform-api") || contains(activeServices, "device-api")) {
		printInfo("Running database migrations...")
		if err := runMigrations(projectRoot); err != nil {
			printWarning("Failed to run migrations: %v", err)
		}
	}

	// Display success message
	printSuccess("Fleet Platform is running!")
	fmt.Println()

	// Display credentials and URLs
	displayStartupInfo(secrets)

	fmt.Println()
	printHeader("Quick Commands:")
	printInfo("View logs:        fleetctl logs")
	printInfo("Check status:     fleetctl status")
	printInfo("Stop platform:    fleetctl stop")
	printInfo("Provision device: fleetctl provision --device /dev/diskX")

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
		containerName := fmt.Sprintf("fleetd-%s", service)

		// Check if container is running
		cmd := exec.Command("docker", "ps", "--filter", fmt.Sprintf("name=%s", containerName), "--format", "{{.Status}}")
		output, err := cmd.Output()
		if err != nil {
			printWarning("%s health check failed: %v", service, err)
			continue
		}

		if strings.Contains(string(output), "Up") {
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

func createDockerNetwork() error {
	// Check if network exists
	cmd := exec.Command("docker", "network", "ls", "--filter", "name=fleetd-network", "--format", "{{.Name}}")
	output, _ := cmd.Output()

	if strings.TrimSpace(string(output)) == "fleetd-network" {
		printInfo("Docker network 'fleetd-network' already exists")
		return nil
	}

	// Create network
	cmd = exec.Command("docker", "network", "create", "fleetd-network")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create docker network: %w", err)
	}
	printSuccess("Created Docker network 'fleetd-network'")
	return nil
}

func startService(service string, secrets map[string]string, projectRoot string) error {
	containerName := fmt.Sprintf("fleetd-%s", service)

	// Check if container already exists
	cmd := exec.Command("docker", "ps", "-a", "--filter", fmt.Sprintf("name=%s", containerName), "--format", "{{.Names}}")
	output, _ := cmd.Output()

	if strings.TrimSpace(string(output)) == containerName {
		// Container exists, try to start it
		cmd = exec.Command("docker", "start", containerName)
		if err := cmd.Run(); err == nil {
			printSuccess("Started existing %s container", service)
			return nil
		}
		// Remove failed container
		exec.Command("docker", "rm", "-f", containerName).Run()
	}

	// Start new container based on service type
	switch service {
	case "postgres":
		return startPostgres(containerName, secrets)
	case "valkey":
		return startValkey(containerName)
	case "victoriametrics":
		return startVictoriaMetrics(containerName)
	case "loki":
		return startLoki(containerName)
	case "traefik":
		return startTraefik(containerName, projectRoot)
	case "platform-api":
		return startPlatformAPI(containerName, secrets, projectRoot)
	case "device-api":
		return startDeviceAPI(containerName, secrets, projectRoot)
	case "studio":
		return startFleetWeb(containerName, projectRoot)
	default:
		return fmt.Errorf("unknown service: %s", service)
	}
}

func startPostgres(containerName string, secrets map[string]string) error {
	cmd := exec.Command("docker", "run", "-d",
		"--name", containerName,
		"--network", "fleetd-network",
		"-p", "5432:5432",
		"-e", fmt.Sprintf("POSTGRES_PASSWORD=%s", secrets["POSTGRES_PASSWORD"]),
		"-e", "POSTGRES_USER=fleetd",
		"-e", "POSTGRES_DB=fleetd",
		"-v", "fleetd-postgres-data:/var/lib/postgresql/data",
		"--restart", "unless-stopped",
		"postgres:15-alpine",
	)
	return cmd.Run()
}

func startValkey(containerName string) error {
	cmd := exec.Command("docker", "run", "-d",
		"--name", containerName,
		"--network", "fleetd-network",
		"-p", "6379:6379",
		"-v", "fleetd-valkey-data:/data",
		"--restart", "unless-stopped",
		"valkey/valkey:7-alpine",
		"valkey-server", "--appendonly", "yes", "--maxmemory", "256mb", "--maxmemory-policy", "allkeys-lru",
	)
	return cmd.Run()
}

func startVictoriaMetrics(containerName string) error {
	cmd := exec.Command("docker", "run", "-d",
		"--name", containerName,
		"--network", "fleetd-network",
		"-p", "8428:8428",
		"-v", "fleetd-metrics-data:/storage",
		"--restart", "unless-stopped",
		"victoriametrics/victoria-metrics:latest",
		"-storageDataPath=/storage",
		"-retentionPeriod=30d",
		"-httpListenAddr=:8428",
	)
	return cmd.Run()
}

func startLoki(containerName string) error {
	cmd := exec.Command("docker", "run", "-d",
		"--name", containerName,
		"--network", "fleetd-network",
		"-p", "3100:3100",
		"-v", "fleetd-loki-data:/loki",
		"--restart", "unless-stopped",
		"grafana/loki:2.9.0",
		"-config.file=/etc/loki/local-config.yaml",
	)
	return cmd.Run()
}

func startTraefik(containerName, projectRoot string) error {
	cmd := exec.Command("docker", "run", "-d",
		"--name", containerName,
		"--network", "fleetd-network",
		"-p", "80:80",
		"-p", "443:443",
		"-p", "8080:8080",
		"-v", "/var/run/docker.sock:/var/run/docker.sock:ro",
		"--restart", "unless-stopped",
		"traefik:v3.0",
		"--api.insecure=true",
		"--providers.docker=true",
		"--providers.docker.exposedbydefault=false",
		"--entrypoints.web.address=:80",
		"--entrypoints.websecure.address=:443",
	)
	return cmd.Run()
}

func startPlatformAPI(containerName string, secrets map[string]string, projectRoot string) error {
	// Build the platform API first
	printInfo("Building platform-api...")
	buildCmd := exec.Command("just", "build-platform")
	buildCmd.Dir = projectRoot
	if err := buildCmd.Run(); err != nil {
		return fmt.Errorf("failed to build platform-api: %w", err)
	}

	cmd := exec.Command("docker", "run", "-d",
		"--name", containerName,
		"--network", "fleetd-network",
		"-p", "8090:8090",
		"-v", fmt.Sprintf("%s/bin/platform-api:/app/platform-api:ro", projectRoot),
		"-v", fmt.Sprintf("%s/config.toml:/etc/fleetd/config.toml:ro", projectRoot),
		"-v", fmt.Sprintf("%s/internal/database/migrations:/app/migrations:ro", projectRoot),
		"-e", fmt.Sprintf("DB_PASSWORD=%s", secrets["POSTGRES_PASSWORD"]),
		"-e", fmt.Sprintf("JWT_SECRET=%s", secrets["JWT_SECRET"]),
		"-e", fmt.Sprintf("PLATFORM_API_SECRET_KEY=%s", secrets["JWT_SECRET"]),
		"-e", "FLEETD_AUTH_MODE=development",
		"-e", "DB_HOST=fleetd-postgres",
		"-e", "DB_PORT=5432",
		"-e", "DB_NAME=fleetd",
		"-e", "DB_USER=fleetd",
		"-e", "REDIS_ADDR=fleetd-valkey:6379",
		"-e", "VICTORIA_METRICS_ENDPOINT=http://fleetd-victoriametrics:8428/api/v1/write",
		"-e", "LOKI_ENDPOINT=http://fleetd-loki:3100/loki/api/v1/push",
		"--restart", "unless-stopped",
		"alpine:3.19",
		"/app/platform-api", "--config", "/etc/fleetd/config.toml",
	)
	return cmd.Run()
}

func startDeviceAPI(containerName string, secrets map[string]string, projectRoot string) error {
	// Build the device API first
	printInfo("Building device-api...")
	buildCmd := exec.Command("just", "build-device")
	buildCmd.Dir = projectRoot
	if err := buildCmd.Run(); err != nil {
		return fmt.Errorf("failed to build device-api: %w", err)
	}

	cmd := exec.Command("docker", "run", "-d",
		"--name", containerName,
		"--network", "fleetd-network",
		"-p", "8081:8081",
		"-v", fmt.Sprintf("%s/bin/device-api:/app/device-api:ro", projectRoot),
		"-v", fmt.Sprintf("%s/config.toml:/etc/fleetd/config.toml:ro", projectRoot),
		"-e", fmt.Sprintf("DB_PASSWORD=%s", secrets["POSTGRES_PASSWORD"]),
		"-e", fmt.Sprintf("JWT_SECRET=%s", secrets["JWT_SECRET"]),
		"-e", fmt.Sprintf("DEVICE_API_SECRET_KEY=%s", secrets["JWT_SECRET"]),
		"-e", "FLEETD_AUTH_MODE=development",
		"-e", "DB_HOST=fleetd-postgres",
		"-e", "DB_PORT=5432",
		"-e", "DB_NAME=fleetd",
		"-e", "DB_USER=fleetd",
		"-e", "REDIS_ADDR=fleetd-valkey:6379",
		"-e", "VICTORIA_METRICS_ENDPOINT=http://fleetd-victoriametrics:8428/api/v1/write",
		"-e", "LOKI_ENDPOINT=http://fleetd-loki:3100/loki/api/v1/push",
		"--restart", "unless-stopped",
		"alpine:3.19",
		"/app/device-api", "--config", "/etc/fleetd/config.toml",
	)
	return cmd.Run()
}

func startFleetWeb(containerName, projectRoot string) error {
	cmd := exec.Command("docker", "run", "-d",
		"--name", containerName,
		"--network", "fleetd-network",
		"-p", "3000:3000",
		"-v", fmt.Sprintf("%s/web:/app", projectRoot),
		"-w", "/app",
		"-e", "NEXT_PUBLIC_API_URL=http://localhost:8090",
		"--restart", "unless-stopped",
		"node:20-alpine",
		"sh", "-c", "npm install && npm run dev",
	)
	return cmd.Run()
}

func runMigrations(projectRoot string) error {
	cmd := exec.Command("docker", "run", "--rm",
		"--network", "fleetd-network",
		"-v", fmt.Sprintf("%s/bin/platform-api:/app/platform-api:ro", projectRoot),
		"-v", fmt.Sprintf("%s/internal/database/migrations:/app/migrations:ro", projectRoot),
		"-e", "DB_HOST=fleetd-postgres",
		"-e", "DB_PORT=5432",
		"-e", "DB_NAME=fleetd",
		"-e", "DB_USER=fleetd",
		"alpine:3.19",
		"/app/platform-api", "--migrate-only",
	)
	return cmd.Run()
}

func displayStartupInfo(secrets map[string]string) {
	printHeader("Service URLs")
	fmt.Printf("  %s Web Dashboard:    %s\n", green("•"), cyan("http://localhost:3000"))
	fmt.Printf("  %s Platform API:     %s\n", green("•"), cyan("http://localhost:8090"))
	fmt.Printf("  %s Device API:       %s\n", green("•"), cyan("http://localhost:8081"))
	fmt.Printf("  %s API Gateway:      %s\n", green("•"), cyan("http://localhost:80"))
	fmt.Printf("  %s Metrics (Victoria): %s\n", green("•"), cyan("http://localhost:8428"))
	fmt.Printf("  %s Logs (Loki):      %s\n", green("•"), cyan("http://localhost:3100"))
	fmt.Printf("  %s Traefik Dashboard: %s\n", green("•"), cyan("http://localhost:8080"))

	fmt.Println()
	printHeader("Credentials")
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
	printHeader("Configuration")
	fmt.Printf("  %s Config:        ./config.toml\n", blue("•"))
	fmt.Printf("  %s Secrets:       ./.env\n", blue("•"))
	fmt.Printf("  %s Database:      PostgreSQL in Docker\n", blue("•"))
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
