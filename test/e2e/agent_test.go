package e2e

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestAgentInContainer(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping e2e test")
	}

	// Build agent binary for Linux
	tmpDir := t.TempDir()
	agentBinary := filepath.Join(tmpDir, "fleetd")

	cmd := exec.Command("go", "build", "-o", agentBinary)
	cmd.Dir = "../../cmd/fleetd" // Set working directory
	cmd.Env = append(os.Environ(),
		"GOOS=linux",
		"GOARCH=amd64",
		"CGO_ENABLED=0", // Disable CGO for cross-compilation
	)

	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build agent: %v\n%s", err, out)
	}

	// Create container
	ctx := context.Background()
	req := testcontainers.ContainerRequest{
		Image:      "alpine:latest",
		Cmd:        []string{"/usr/local/bin/fleetd", "--disable-mdns"},
		WaitingFor: wait.ForLog("Agent started"),
		Files: []testcontainers.ContainerFile{
			{
				HostFilePath:      agentBinary,
				ContainerFilePath: "/usr/local/bin/fleetd",
				FileMode:          0755,
			},
		},
		ExposedPorts: []string{"8080/tcp"},
		Mounts: []testcontainers.ContainerMount{
			{
				Source: testcontainers.GenericBindMountSource{
					HostPath: "/tmp/fleetd-test",
				},
				Target: "/var/lib/fleetd",
			},
		},
	}

	// Ensure test directory exists
	if err := os.MkdirAll("/tmp/fleetd-test", 0755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}
	defer os.RemoveAll("/tmp/fleetd-test")

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("Failed to create container: %v", err)
	}
	defer container.Terminate(ctx)

	// Get container IP
	ip, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("Failed to get container host: %v", err)
	}

	// Wait for agent to be ready
	time.Sleep(500 * time.Millisecond)

	// Test binary deployment
	testBinary := []byte(`#!/bin/sh
while true; do
	echo "Running test binary"
	sleep 1
done`)

	// Deploy binary through agent API
	url := fmt.Sprintf("http://%s:8080/api/v1/binaries/test-binary", ip)
	resp, err := http.Post(url, "application/octet-stream", bytes.NewReader(testBinary))
	if err != nil {
		t.Fatalf("Failed to deploy binary: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Deploy failed with status %d: %s", resp.StatusCode, body)
	}

	// Start binary
	startURL := fmt.Sprintf("http://%s:8080/api/v1/binaries/test-binary/start", ip)
	resp, err = http.Post(startURL, "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("Failed to start binary: %v", err)
	}
	defer resp.Body.Close()

	// Verify logs exist
	logs, err := container.Logs(ctx)
	if err != nil {
		t.Fatalf("Failed to get container logs: %v", err)
	}
	defer logs.Close()

	logContent, err := io.ReadAll(logs)
	if err != nil {
		t.Fatalf("Failed to read logs: %v", err)
	}

	if !strings.Contains(string(logContent), "Running test binary") {
		t.Error("Expected log message not found")
	}

	// Check state persistence
	statePath := "/var/lib/fleetd/state/state.json"
	state, err := container.CopyFileFromContainer(ctx, statePath)
	if err != nil {
		t.Fatalf("Failed to get state file: %v", err)
	}

	// Read state file
	stateBytes, err := io.ReadAll(state)
	if err != nil {
		t.Fatalf("Failed to read state file: %v", err)
	}

	if len(stateBytes) == 0 {
		t.Error("State file is empty")
	}
}

func buildAgent(t *testing.T) {
	cmd := exec.Command("go", "build", "-o", "testdata/fleetd", "../../cmd/fleetd")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build agent: %v\nOutput: %s", err, out)
	}
}

func createContainer(ctx context.Context, t *testing.T) (testcontainers.Container, error) {
	req := testcontainers.ContainerRequest{
		FromDockerfile: testcontainers.FromDockerfile{
			Context:    "testdata",
			Dockerfile: "Dockerfile.test",
		},
		ExposedPorts: []string{"8080/tcp"},
		WaitingFor:   wait.ForHTTP("/health").WithPort("8080/tcp"),
	}

	return testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
}
