package cmd

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func newInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a new Fleet project",
		Long: `Initialize a new Fleet project in the current directory.

This command creates:
  - config.toml (project configuration)
  - .env (environment variables)
  - migrations/ (database migrations directory)`,
		RunE: runInit,
	}

	return cmd
}

func runInit(cmd *cobra.Command, args []string) error {
	printHeader("Initializing Fleet project")
	fmt.Println()

	// Check if already initialized
	if _, err := os.Stat("config.toml"); err == nil {
		printWarning("Project already initialized (config.toml exists)")
		fmt.Print("Overwrite existing configuration? (y/N): ")
		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')
		if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(response)), "y") {
			printInfo("Initialization cancelled")
			return nil
		}
	}

	// Get project name
	projectName := promptString("Project name", "fleet-project")

	// Select services
	printInfo("Select services to include:")
	services := promptServices()

	// Create config.toml
	printInfo("Creating config.toml...")
	if err := createConfigFile(projectName, services); err != nil {
		printError("Failed to create config.toml: %v", err)
		return err
	}
	printSuccess("Created config.toml")

	// Create .env file
	printInfo("Creating .env file...")
	if err := createEnvFile(projectName); err != nil {
		printError("Failed to create .env: %v", err)
		return err
	}
	printSuccess("Created .env")

	// Create directory structure
	printInfo("Creating project structure...")
	dirs := []string{
		"migrations",
		"configs",
		"scripts",
		"data",
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			printError("Failed to create directory %s: %v", dir, err)
			return err
		}
	}
	printSuccess("Created project directories")

	// Create .gitignore
	printInfo("Creating .gitignore...")
	if err := createGitignore(); err != nil {
		printError("Failed to create .gitignore: %v", err)
		return err
	}
	printSuccess("Created .gitignore")

	fmt.Println()
	printSuccess("Fleet project initialized successfully!")
	fmt.Println()
	printHeader("Next steps:")
	fmt.Println("  1. Review and edit config.toml")
	fmt.Println("  2. Set up environment variables in .env")
	fmt.Println("  3. Run 'fleetctl start' to start development stack")
	fmt.Println("  4. Run 'fleetctl migrate up' to set up database")

	return nil
}

func promptString(prompt, defaultValue string) string {
	fmt.Printf("%s [%s]: ", prompt, defaultValue)
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "" {
		return defaultValue
	}
	return input
}

func promptServices() []string {
	allServices := []string{
		"postgres",
		"victoriametrics",
		"loki",
		"valkey",
		"traefik",
		"clickhouse",
	}

	selected := make(map[string]bool)
	// Default selections
	selected["postgres"] = true
	selected["victoriametrics"] = true
	selected["loki"] = true
	selected["valkey"] = true
	selected["traefik"] = true

	fmt.Println("  [x] postgres       - Primary database")
	fmt.Println("  [x] victoriametrics - Metrics storage")
	fmt.Println("  [x] loki          - Log aggregation")
	fmt.Println("  [x] valkey        - Caching and rate limiting")
	fmt.Println("  [x] traefik       - API gateway")
	fmt.Println("  [ ] clickhouse    - Analytics database")
	fmt.Println()
	fmt.Print("Customize service selection? (y/N): ")

	reader := bufio.NewReader(os.Stdin)
	response, _ := reader.ReadString('\n')
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(response)), "y") {
		// Interactive service selection
		for _, service := range allServices {
			fmt.Printf("Include %s? (Y/n): ", service)
			response, _ := reader.ReadString('\n')
			response = strings.ToLower(strings.TrimSpace(response))
			if response == "" || response == "y" || response == "yes" {
				selected[service] = true
			} else {
				selected[service] = false
			}
		}
	}

	var services []string
	for service, include := range selected {
		if include {
			services = append(services, service)
		}
	}

	return services
}

// generateSecureSecret generates a cryptographically secure random secret
func generateSecureSecret(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

func createConfigFile(projectName string, services []string) error {
	// Generate secure secrets
	dbPassword, err := generateSecureSecret(32)
	if err != nil {
		return fmt.Errorf("failed to generate database password: %w", err)
	}

	jwtSecret, err := generateSecureSecret(48)
	if err != nil {
		return fmt.Errorf("failed to generate JWT secret: %w", err)
	}

	config := fmt.Sprintf(`# Fleet Project Configuration
# WARNING: This file contains generated secrets. Keep it secure and never commit to version control!
# For production, use environment variables or a secure secret management system.

[project]
id = "%s"
name = "%s"

[api]
enabled = true
port = 8080
host = "localhost"

[db]
port = 5432
host = "localhost"
name = "fleetd"
user = "fleetd"
# IMPORTANT: Change this password before production use!
# Generated password (base64 encoded):
password = "%s"

[stack]
services = [%s]

[gateway]
port = 80
dashboard_port = 8080

[telemetry]
victoria_metrics_port = 8428
loki_port = 3100
grafana_port = 3001

[auth]
# CRITICAL: This JWT secret is auto-generated. Store it securely!
# For production, use environment variable: FLEETD_JWT_SECRET
jwt_secret = "%s"
api_keys = []

[provisioning]
default_image = "raspios-lite"
default_user = "pi"

# Environment-specific overrides
[environments.staging]
api.url = "https://staging.fleet.example.com"

[environments.production]
api.url = "https://api.fleet.example.com"
`,
		strings.ReplaceAll(projectName, " ", "-"),
		projectName,
		dbPassword,
		formatServiceList(services),
		jwtSecret)

	return os.WriteFile("config.toml", []byte(config), 0o644)
}

func formatServiceList(services []string) string {
	var formatted []string
	for _, service := range services {
		formatted = append(formatted, fmt.Sprintf("\n  \"%s\"", service))
	}
	if len(formatted) > 0 {
		return strings.Join(formatted, ",") + "\n"
	}
	return ""
}

func createEnvFile(projectName string) error {
	// Generate secure secrets for development
	dbPassword, err := generateSecureSecret(32)
	if err != nil {
		return fmt.Errorf("failed to generate database password: %w", err)
	}

	jwtSecret, err := generateSecureSecret(48)
	if err != nil {
		return fmt.Errorf("failed to generate JWT secret: %w", err)
	}

	env := fmt.Sprintf(`# Fleet Environment Variables
# WARNING: This file contains generated secrets. Keep it secure and never commit to version control!
# For production, use a secure secret management system.

# Project
PROJECT_NAME=%s
ENVIRONMENT=development

# Database
DATABASE_URL=postgres://fleetd:%s@localhost:5432/fleetd?sslmode=disable
POSTGRES_HOST=localhost
POSTGRES_PORT=5432
POSTGRES_DB=fleetd
POSTGRES_USER=fleetd
POSTGRES_PASSWORD=%s

# API
API_PORT=8080
API_HOST=localhost

# Auth
JWT_SECRET=%s

# Telemetry
VICTORIA_METRICS_URL=http://localhost:8428
LOKI_URL=http://localhost:3100

# Gateway
GATEWAY_PORT=80
GATEWAY_DASHBOARD_PORT=8080
`, projectName, dbPassword, dbPassword, jwtSecret)

	return os.WriteFile(".env", []byte(env), 0o644)
}

func createGitignore() error {
	gitignore := `# Fleet
.env
*.log
data/

# OS
.DS_Store
Thumbs.db

# IDE
.vscode/
.idea/
*.swp
*.swo

# Build
dist/
build/
*.exe
fleetd
fleet

# Dependencies
node_modules/
vendor/

# Testing
coverage/
*.test
`

	return os.WriteFile(".gitignore", []byte(gitignore), 0o644)
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
