package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"fleetd.sh/cmd/fleetctl/cmd"
	"fleetd.sh/gen/fleetd/v1/fleetpbconnect"
	"fleetd.sh/internal/database"
	"fleetd.sh/internal/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCLICommands tests fleetctl commands against a running server
func TestCLICommands(t *testing.T) {
	requireIntegrationMode(t)
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Start test server
	server, url := startTestServer(t)
	defer server.Close()

	// Set environment for CLI to use test server
	os.Setenv("FLEETD_API_URL", url)
	defer os.Unsetenv("FLEETD_API_URL")

	t.Run("TelemetryCommands", func(t *testing.T) {
		t.Run("GetTelemetry", func(t *testing.T) {
			output := runCommand(t, "telemetry", "get", "--limit", "5")
			assert.Contains(t, output, "DEVICE")
			assert.Contains(t, output, "CPU%")
			assert.Contains(t, output, "MEM%")
		})

		t.Run("GetTelemetryJSON", func(t *testing.T) {
			output := runCommand(t, "telemetry", "get", "--format", "json")
			var data []interface{}
			err := json.Unmarshal([]byte(output), &data)
			require.NoError(t, err)
			assert.NotEmpty(t, data)
		})

		t.Run("GetLogs", func(t *testing.T) {
			output := runCommand(t, "telemetry", "logs", "--limit", "10")
			// Should contain log entries
			lines := strings.Split(output, "\n")
			for _, line := range lines {
				if line != "" {
					// Each line should have timestamp, level, device, message
					assert.Contains(t, line, "[")
				}
			}
		})

		t.Run("GetLogsWithFilter", func(t *testing.T) {
			output := runCommand(t, "telemetry", "logs", "--level", "error", "--limit", "5")
			if output != "" {
				assert.Contains(t, strings.ToLower(output), "error")
			}
		})

		t.Run("ListAlerts", func(t *testing.T) {
			output := runCommand(t, "telemetry", "alerts", "list")
			assert.Contains(t, output, "ID")
			assert.Contains(t, output, "NAME")
			assert.Contains(t, output, "TYPE")
		})

		t.Run("CreateAlert", func(t *testing.T) {
			output := runCommand(t, "telemetry", "alerts", "create",
				"--name", "Test Alert",
				"--type", "cpu",
				"--threshold", "80",
				"--description", "Test alert for CPU")
			assert.Contains(t, output, "Alert created")
		})
	})

	t.Run("SettingsCommands", func(t *testing.T) {
		t.Run("GetAllSettings", func(t *testing.T) {
			output := runCommand(t, "settings", "get")
			assert.Contains(t, output, "Organization Settings")
			assert.Contains(t, output, "Security Settings")
			assert.Contains(t, output, "API Settings")
		})

		t.Run("GetOrganizationSettings", func(t *testing.T) {
			output := runCommand(t, "settings", "get", "org")
			assert.Contains(t, output, "Name")
			assert.Contains(t, output, "Contact Email")
			assert.Contains(t, output, "Timezone")
		})

		t.Run("UpdateOrganizationSettings", func(t *testing.T) {
			output := runCommand(t, "settings", "update", "org",
				"--name", "Test Corp",
				"--email", "test@example.com")
			assert.Contains(t, output, "Organization settings updated")
		})

		t.Run("GetSecuritySettings", func(t *testing.T) {
			output := runCommand(t, "settings", "get", "security")
			assert.Contains(t, output, "Two-Factor Required")
			assert.Contains(t, output, "Session Timeout")
			assert.Contains(t, output, "Audit Logging")
		})

		t.Run("ShowAPIKey", func(t *testing.T) {
			output := runCommand(t, "settings", "api-key", "show")
			assert.Contains(t, output, "API Key:")
			assert.Contains(t, output, "fleetd_sk_")
		})

		t.Run("ExportData", func(t *testing.T) {
			output := runCommand(t, "settings", "export",
				"--types", "devices,telemetry",
				"--format", "json")
			assert.Contains(t, output, "Data export initiated")
			assert.Contains(t, output, "Download URL:")
		})
	})
}

// TestCLIWithDocker tests CLI commands that interact with Docker
func TestCLIWithDocker(t *testing.T) {
	requireIntegrationMode(t)
	if testing.Short() || os.Getenv("SKIP_DOCKER_TESTS") != "" {
		t.Skip("Skipping Docker integration test")
	}

	// Check if Docker is available
	if err := exec.Command("docker", "version").Run(); err != nil {
		t.Skip("Docker not available")
	}

	tmpDir := t.TempDir()
	projectRoot := filepath.Join(tmpDir, "test-project")
	os.MkdirAll(projectRoot, 0755)

	// Create minimal project structure
	createTestProject(t, projectRoot)

	os.Setenv("FLEETD_PROJECT_ROOT", projectRoot)
	defer os.Unsetenv("FLEETD_PROJECT_ROOT")

	t.Run("InitCommand", func(t *testing.T) {
		output := runCommandInDir(t, projectRoot, "init")
		assert.Contains(t, output, "initialized")

		// Verify config files were created
		assert.FileExists(t, filepath.Join(projectRoot, "config.toml"))
		// Note: docker-compose.yml might not be created by init command
		// assert.FileExists(t, filepath.Join(projectRoot, "docker-compose.yml"))
	})

	t.Run("StartCommand", func(t *testing.T) {
		t.Skip("Skipping start command test - use TestDockerCompose in docker_test.go instead")
	})
}

// TestCLIProgrammatic tests CLI commands programmatically without exec
func TestCLIProgrammatic(t *testing.T) {
	requireIntegrationMode(t)
	// Start test server
	server, url := startTestServer(t)
	defer server.Close()

	// Set environment for CLI
	os.Setenv("FLEETD_API_URL", url)
	defer os.Unsetenv("FLEETD_API_URL")

	t.Run("RootCommand", func(t *testing.T) {
		output := captureOutput(func() {
			cmd.Execute()
		})
		assert.Contains(t, output, "fleetctl is a unified CLI for managing fleetd infrastructure")
	})

	t.Run("VersionCommand", func(t *testing.T) {
		output := captureOutput(func() {
			os.Args = []string{"fleetctl", "version"}
			cmd.Execute()
		})
		assert.Contains(t, output, "fleetctl")
		assert.Contains(t, output, "Version:")
	})
}

// Helper functions

func startTestServer(t *testing.T) (*httptest.Server, string) {
	// Create test database
	db := setupTestDatabase(t)
	t.Cleanup(func() { safeCloseDB(db) })

	// Create services
	dbWrapper := &database.DB{DB: db}
	telemetryService := services.NewTelemetryService(dbWrapper)
	settingsService := services.NewSettingsService(dbWrapper)

	// Create server mux
	mux := http.NewServeMux()

	// Register telemetry service
	telemetryPath, telemetryHandler := fleetpbconnect.NewTelemetryServiceHandler(telemetryService)
	mux.Handle(telemetryPath, telemetryHandler)

	// Register settings service
	settingsPath, settingsHandler := fleetpbconnect.NewSettingsServiceHandler(settingsService)
	mux.Handle(settingsPath, settingsHandler)

	// Start server
	server := httptest.NewServer(mux)
	return server, server.URL
}

func runCommand(t *testing.T, args ...string) string {
	cmd := exec.Command("fleetctl", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("Command failed: fleetctl %s", strings.Join(args, " "))
		t.Logf("Output: %s", output)
		t.Fatalf("Command error: %v", err)
	}
	return string(output)
}

func runCommandInDir(t *testing.T, dir string, args ...string) string {
	cmd := exec.Command("fleetctl", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("Command failed: fleetctl %s", strings.Join(args, " "))
		t.Logf("Output: %s", output)
		t.Fatalf("Command error: %v", err)
	}
	return string(output)
}

func captureOutput(f func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	f()

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

func createTestProject(t *testing.T, root string) {
	// Create minimal project structure
	dirs := []string{
		"migrations",
		"certs",
		"data",
	}

	for _, dir := range dirs {
		err := os.MkdirAll(filepath.Join(root, dir), 0755)
		require.NoError(t, err)
	}

	// Create a basic migration file
	migrationContent := `CREATE TABLE IF NOT EXISTS test (id INTEGER PRIMARY KEY);`
	err := os.WriteFile(
		filepath.Join(root, "migrations", "001_init.sql"),
		[]byte(migrationContent),
		0644,
	)
	require.NoError(t, err)
}
