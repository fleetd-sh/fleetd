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
  - Fleet server (platform & device APIs)
  - Studio (web UI dashboard)`,
		RunE: runStart,
	}

	cmd.Flags().StringSliceVarP(&exclude, "exclude", "e", []string{}, "Services to exclude (e.g., postgres,loki)")
	cmd.Flags().BoolVarP(&detach, "detach", "d", true, "Run services in background")
	cmd.Flags().BoolVar(&reset, "reset", false, "Reset all data and regenerate secrets")
	cmd.Flags().BoolVar(&noWeb, "no-studio", false, "Don't start the Studio web UI")
	cmd.Flags().BoolVar(&noServer, "no-server", false, "Don't start the fleetd server")
	cmd.Flags().IntVar(&exposePort, "expose-port", 8080, "Port to expose the fleetd API")

	return cmd
}

func runStart(cmd *cobra.Command, args []string) error {
	// Check Docker availability
	if err := checkDocker(); err != nil {
		return err
	}

	// Check Docker Compose availability
	if err := checkDockerCompose(); err != nil {
		return err
	}

	printHeader("Starting fleetd Platform")
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

	// Build Docker images if needed
	printInfo("Checking Docker images...")
	if err := buildDockerImages(projectRoot); err != nil {
		printWarning("Failed to build some images: %v", err)
	}

	// Determine compose files to use
	composeFiles := []string{
		filepath.Join(projectRoot, "docker", "docker-compose.yaml"),
	}

	// Add dev overrides if in development mode
	if viper.GetString("environment") != "production" {
		devCompose := filepath.Join(projectRoot, "docker", "docker-compose.dev.yaml")
		if _, err := os.Stat(devCompose); err == nil {
			composeFiles = append(composeFiles, devCompose)
		}
	}

	// Build docker-compose command
	composeArgs := []string{}
	for _, file := range composeFiles {
		composeArgs = append(composeArgs, "-f", file)
	}
	composeArgs = append(composeArgs, "--env-file", filepath.Join(projectRoot, ".env"))
	composeArgs = append(composeArgs, "up")

	if detach {
		composeArgs = append(composeArgs, "-d")
	}

	// Add services to start (or exclude)
	if len(exclude) > 0 {
		services := getServicesToStart(noServer, noWeb, exclude)
		composeArgs = append(composeArgs, services...)
	}

	// Start services with docker-compose
	printInfo("Starting services with Docker Compose...")
	dockerCmd := exec.Command("docker", append([]string{"compose"}, composeArgs...)...)
	dockerCmd.Stdout = os.Stdout
	dockerCmd.Stderr = os.Stderr
	dockerCmd.Dir = projectRoot

	if err := dockerCmd.Run(); err != nil {
		return fmt.Errorf("failed to start services: %w", err)
	}

	// Wait for services to be ready
	if detach {
		printInfo("Waiting for services to be ready...")
		time.Sleep(5 * time.Second)

		// Check service health
		if err := checkServicesHealthCompose(projectRoot); err != nil {
			printWarning("Some services may not be ready: %v", err)
		}
	}

	// Run database migrations
	printInfo("Running database migrations...")
	if err := runMigrations(projectRoot); err != nil {
		printError("Failed to run migrations: %v", err)
		return fmt.Errorf("database migration failed: %w", err)
	}

	// Display success message
	printSuccess("fleetd Platform is running!")
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

	config := fmt.Sprintf(`# fleetd Platform Configuration
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
	envContent.WriteString("# fleetd Platform Secrets\n")
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

func checkDockerCompose() error {
	cmd := exec.Command("docker", "compose", "version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker compose not found. Please install Docker with Compose plugin")
	}
	return nil
}

func buildDockerImages(projectRoot string) error {
	// Check if images exist
	images := []string{
		"ghcr.io/fleetd-sh/platform-api:latest",
		"ghcr.io/fleetd-sh/device-api:latest",
		"ghcr.io/fleetd-sh/studio:latest",
	}

	for _, image := range images {
		cmd := exec.Command("docker", "images", "-q", image)
		output, _ := cmd.Output()
		if strings.TrimSpace(string(output)) == "" {
			printInfo("Image %s not found, building...", image)
			// Build using Justfile
			serviceName := strings.Split(strings.Split(image, "/")[1], ":")[0]
			cmd = exec.Command("just", fmt.Sprintf("docker-build-%s", serviceName))
			cmd.Dir = projectRoot
			if err := cmd.Run(); err != nil {
				printWarning("Failed to build %s: %v", image, err)
			}
		}
	}
	return nil
}

func getServicesToStart(noServer, noWeb bool, exclude []string) []string {
	services := []string{
		"postgres",
		"valkey",
		"loki",
		"victoriametrics",
		"traefik",
	}

	if !noServer {
		services = append(services, "platform-api", "device-api")
	}
	if !noWeb {
		services = append(services, "studio")
	}

	// Remove excluded services
	var filtered []string
	for _, s := range services {
		excluded := false
		for _, e := range exclude {
			if s == e {
				excluded = true
				break
			}
		}
		if !excluded {
			filtered = append(filtered, s)
		}
	}

	return filtered
}

func checkServicesHealthCompose(projectRoot string) error {
	cmd := exec.Command("docker", "compose", "-f", filepath.Join(projectRoot, "docker", "docker-compose.yaml"), "ps", "--format", "json")
	_, err := cmd.Output()
	if err != nil {
		return err
	}

	// For now, just return nil
	return nil
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
		"postgres:17-alpine",
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
	// Find available port for Traefik dashboard
	traefikPort := findAvailablePort(8080)
	if traefikPort == 0 {
		printWarning("Could not find available port for Traefik dashboard, using default 8080")
		traefikPort = 8080
	}

	if traefikPort != 8080 {
		printInfo("Using port %d for Traefik dashboard (8080 was occupied)", traefikPort)
	}

	// Try with simplified configuration without Docker socket
	// This avoids permission issues on macOS and still works for basic routing
	err := runDockerCommand(
		"run", "-d",
		"--name", containerName,
		"--network", "fleetd-network",
		"-p", "80:80",
		"-p", fmt.Sprintf("%d:8080", traefikPort), // Map to available port
		"--restart", "unless-stopped",
		"traefik:v3.0",
		"--api.insecure=true",
		"--providers.docker=false",                        // Disable Docker provider to avoid socket issues
		"--providers.file.directory=/etc/traefik/dynamic", // Use file provider instead
		"--entrypoints.web.address=:80",
	)

	if err != nil {
		// Provide helpful error message
		if strings.Contains(err.Error(), "Unable to find image") {
			printInfo("Pulling Traefik image...")
			if pullErr := runDockerCommand("pull", "traefik:v3.0"); pullErr == nil {
				// Retry after pulling
				return startTraefik(containerName, projectRoot)
			}
		}
		return fmt.Errorf("%s", getDockerError(err, "Traefik"))
	}
	return nil
}

func startPlatformAPI(containerName string, secrets map[string]string, projectRoot string) error {
	cmd := exec.Command("docker", "run", "-d",
		"--name", containerName,
		"--network", "fleetd-network",
		"-p", "8090:8090",
		"-e", "DB_DRIVER=postgres",
		"-e", "DB_HOST=fleetd-postgres",
		"-e", "DB_PORT=5432",
		"-e", "DB_NAME=fleetd",
		"-e", "DB_USER=fleetd",
		"-e", fmt.Sprintf("DB_PASSWORD=%s", secrets["POSTGRES_PASSWORD"]),
		"-e", "DB_SSLMODE=disable",
		"-e", fmt.Sprintf("PLATFORM_API_SECRET_KEY=%s", secrets["JWT_SECRET"]),
		"-e", "DEVICE_API_URL=http://fleetd-device-api:8080",
		"-e", "FLEETD_AUTH_MODE=development",
		"-e", "LOG_LEVEL=info",
		"--restart", "unless-stopped",
		"ghcr.io/fleetd-sh/platform-api:latest",
	)
	return cmd.Run()
}

func startDeviceAPI(containerName string, secrets map[string]string, projectRoot string) error {
	cmd := exec.Command("docker", "run", "-d",
		"--name", containerName,
		"--network", "fleetd-network",
		"-p", "8082:8080",
		"-e", "DB_DRIVER=postgres",
		"-e", "DB_HOST=fleetd-postgres",
		"-e", "DB_PORT=5432",
		"-e", "DB_NAME=fleetd",
		"-e", "DB_USER=fleetd",
		"-e", fmt.Sprintf("DB_PASSWORD=%s", secrets["POSTGRES_PASSWORD"]),
		"-e", "DB_SSLMODE=disable",
		"-e", fmt.Sprintf("DEVICE_API_SECRET_KEY=%s", secrets["JWT_SECRET"]),
		"-e", "PLATFORM_API_URL=http://fleetd-platform-api:8090",
		"-e", "FLEETD_AUTH_MODE=development",
		"-e", "LOG_LEVEL=info",
		"--restart", "unless-stopped",
		"ghcr.io/fleetd-sh/device-api:latest",
	)
	return cmd.Run()
}

func startFleetWeb(containerName, projectRoot string) error {
	// Run the studio container
	cmd := exec.Command("docker", "run", "-d",
		"--name", containerName,
		"--network", "fleetd-network",
		"-p", "3000:3000",
		"-e", "NODE_ENV=production",
		"-e", "NEXT_PUBLIC_API_URL=http://localhost:8090",
		"-e", "BACKEND_URL=http://fleetd-platform-api:8090",
		"-e", "NEXT_PUBLIC_DEVICE_API_URL=http://localhost:8082",
		"-e", "DEVICE_API_URL=http://fleetd-device-api:8080",
		"--restart", "unless-stopped",
		"ghcr.io/fleetd-sh/studio:latest",
	)
	return cmd.Run()
}

func runMigrations(projectRoot string) error {
	// Get database connection info from environment/secrets
	secrets, err := loadOrCreateSecrets(projectRoot)
	if err != nil {
		return fmt.Errorf("failed to load secrets: %w", err)
	}

	// Set environment variables for the migration command
	os.Setenv("DB_HOST", "localhost")
	os.Setenv("DB_PORT", "5432")
	os.Setenv("DB_NAME", "fleetd")
	os.Setenv("DB_USER", "fleetd")
	os.Setenv("DB_PASSWORD", secrets["POSTGRES_PASSWORD"])
	os.Setenv("DB_SSLMODE", "disable")
	os.Setenv("DATABASE_TYPE", "postgres")

	// Wait for PostgreSQL to be ready
	printInfo("Waiting for PostgreSQL to be ready...")
	maxRetries := 30
	for i := 0; i < maxRetries; i++ {
		if err := checkPostgresConnection(); err == nil {
			break
		}
		if i == maxRetries-1 {
			return fmt.Errorf("PostgreSQL is not ready after %d attempts", maxRetries)
		}
		time.Sleep(1 * time.Second)
	}

	// Create and execute the migrate up command
	upCmd := newMigrateUpCmd()

	// Set command args to simulate CLI execution
	upCmd.SetArgs([]string{})

	// Execute the migration
	if err := upCmd.Execute(); err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	return nil
}

func displayStartupInfo(secrets map[string]string) {
	printHeader("Service URLs")
	fmt.Printf("  %s Studio Dashboard: %s\n", green("•"), cyan("http://localhost:3000"))
	fmt.Printf("  %s Platform API:     %s\n", green("•"), cyan("http://localhost:8090"))
	fmt.Printf("  %s Device API:       %s\n", green("•"), cyan("http://localhost:8082"))
	fmt.Printf("  %s API Gateway:      %s\n", green("•"), cyan("http://localhost:80"))
	fmt.Printf("  %s Metrics (Victoria): %s\n", green("•"), cyan("http://localhost:8428"))
	fmt.Printf("  %s Logs (Loki):      %s\n", green("•"), cyan("http://localhost:3100"))
	// Check which port Traefik is actually using
	traefikPort := 8080
	if !isPortAvailable(8080) {
		for _, port := range []int{8081, 8082, 8083} {
			if !isPortAvailable(port) {
				traefikPort = port
				break
			}
		}
	}
	fmt.Printf("  %s Traefik Dashboard: %s\n", green("•"), cyan(fmt.Sprintf("http://localhost:%d", traefikPort)))

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
