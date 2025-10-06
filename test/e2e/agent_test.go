package e2e

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	agentpb "fleetd.sh/gen/agent/v1"
	agentrpc "fleetd.sh/gen/agent/v1/agentpbconnect"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestAgentInContainer(t *testing.T) {
	if os.Getenv("INTEGRATION") != "1" {
		t.Skip("set INTEGRATION=1 to run integration tests")
	}

	ctx := context.Background()

	// Create container
	container, err := createContainer(ctx, t)
	if err != nil {
		t.Fatalf("Failed to create container: %v", err)
	}
	defer container.Terminate(ctx)

	// Get container IP
	ip, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("Failed to get container host: %v", err)
	}

	// Get mapped port
	mappedPort, err := container.MappedPort(ctx, "8080/tcp")
	require.NoError(t, err)

	// Create Connect client
	client := agentrpc.NewDaemonServiceClient(
		http.DefaultClient,
		fmt.Sprintf("http://%s:%s", ip, mappedPort.Port()),
	)

	// Basic connectivity test using ListBinaries
	listCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err = client.ListBinaries(listCtx, connect.NewRequest(&agentpb.ListBinariesRequest{}))
	if err != nil {
		// Get container logs for debugging
		logs, logErr := container.Logs(ctx)
		if logErr == nil {
			logContent, _ := io.ReadAll(logs)
			t.Errorf("Container logs:\n%s", string(logContent))
			logs.Close()
		}

		t.Fatal("Basic connectivity test failed")
	}

	t.Log("Basic connectivity test passed")
}

func buildAgent(t *testing.T) {
	t.Helper()

	// Get the absolute path to the testdata directory
	// The test file is in test/e2e/, so testdata should be in test/e2e/testdata
	testFile, err := filepath.Abs(os.Args[0])
	if err != nil {
		// Fallback to working directory
		wd, err := os.Getwd()
		if err != nil {
			t.Fatalf("Failed to get working directory: %v", err)
		}
		testFile = wd
	}

	// The testdata directory should be relative to the test file location
	testDataDir := filepath.Join(filepath.Dir(testFile), "test", "e2e", "testdata")
	if _, err := os.Stat(testDataDir); os.IsNotExist(err) {
		// Try relative to current directory
		testDataDir = filepath.Join("test", "e2e", "testdata")
		if _, err := os.Stat(testDataDir); os.IsNotExist(err) {
			// Try just testdata in current directory (when running from test/e2e)
			testDataDir = "testdata"
			if _, err := os.Stat(testDataDir); os.IsNotExist(err) {
				// Create the directory if it doesn't exist
				if err := os.MkdirAll(testDataDir, 0o755); err != nil {
					t.Fatalf("Failed to create testdata directory: %v", err)
				}
			}
		}
	}

	t.Logf("Using testdata directory: %s", testDataDir)

	// Build the agent binary for Linux
	// Determine the architecture - use amd64 by default, but allow override
	goarch := "amd64"
	if envArch := os.Getenv("GOARCH"); envArch != "" {
		goarch = envArch
	}

	binaryPath := filepath.Join(testDataDir, "fleetd")
	cmd := exec.Command("go", "build", "-o", binaryPath, "fleetd.sh/cmd/fleetd")
	cmd.Env = append(os.Environ(),
		"GOOS=linux",
		"GOARCH="+goarch,
		"CGO_ENABLED=0", // Ensure static binary for Alpine
	)

	t.Logf("Building binary: GOOS=linux GOARCH=%s CGO_ENABLED=0 go build -o %s fleetd.sh/cmd/fleetd", goarch, binaryPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to build agent binary: %v\nOutput: %s", err, output)
	}

	// Verify the binary was created
	if _, err := os.Stat(binaryPath); err != nil {
		t.Fatalf("Binary not found after build at %s: %v", binaryPath, err)
	}

	t.Logf("Binary built successfully at %s", binaryPath)
}

func createContainer(ctx context.Context, t *testing.T) (testcontainers.Container, error) {
	// Build agent first
	buildAgent(t)

	// Get the correct testdata directory path using same logic as buildAgent
	testFile, err := filepath.Abs(os.Args[0])
	if err != nil {
		wd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to get working directory: %v", err)
		}
		testFile = wd
	}

	testDataDir := filepath.Join(filepath.Dir(testFile), "test", "e2e", "testdata")
	if _, err := os.Stat(testDataDir); os.IsNotExist(err) {
		testDataDir = filepath.Join("test", "e2e", "testdata")
		if _, err := os.Stat(testDataDir); os.IsNotExist(err) {
			testDataDir = "testdata"
		}
	}

	// Verify the binary exists in the context directory
	binaryPath := filepath.Join(testDataDir, "fleetd")
	if _, err := os.Stat(binaryPath); err != nil {
		return nil, fmt.Errorf("binary not found at %s: %v", binaryPath, err)
	}

	// Also verify Dockerfile exists
	dockerfilePath := filepath.Join(testDataDir, "Dockerfile.test")
	if _, err := os.Stat(dockerfilePath); err != nil {
		return nil, fmt.Errorf("Dockerfile not found at %s: %v", dockerfilePath, err)
	}

	t.Logf("Container build context: %s", testDataDir)
	t.Logf("Binary exists at: %s", binaryPath)
	t.Logf("Dockerfile exists at: %s", dockerfilePath)

	req := testcontainers.ContainerRequest{
		FromDockerfile: testcontainers.FromDockerfile{
			Context:    testDataDir,
			Dockerfile: "Dockerfile.test",
		},
		ExposedPorts: []string{"8080/tcp"},
		WaitingFor:   wait.ForListeningPort("8080/tcp").WithStartupTimeout(5 * time.Second),
		Cmd: []string{
			"--storage-dir", "/var/lib/fleetd/state",
			"--rpc-port", "8080",
			"--disable-mdns",
		},
	}

	return testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
}
