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
	if testing.Short() {
		t.Skip("Skipping e2e test")
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

	// Get the path to the testdata directory
	testDataDir := filepath.Join("testdata")

	// Build the agent binary for Linux
	cmd := exec.Command("go", "build", "-o", filepath.Join(testDataDir, "fleetd"), "../../cmd/fleetd")
	cmd.Env = append(os.Environ(),
		"GOOS=linux",
		"GOARCH=amd64",
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to build agent binary: %v\nOutput: %s", err, output)
	}
}

func createContainer(ctx context.Context, t *testing.T) (testcontainers.Container, error) {
	// Build agent first
	buildAgent(t)

	req := testcontainers.ContainerRequest{
		FromDockerfile: testcontainers.FromDockerfile{
			Context:    "testdata",
			Dockerfile: "Dockerfile.test",
		},
		ExposedPorts: []string{"8080/tcp"},
		WaitingFor:   wait.ForListeningPort("8080/tcp").WithStartupTimeout(5 * time.Second),
		Cmd: []string{
			"-storage-dir", "/var/lib/fleetd/state",
			"-rpc-port", "8080",
			"-disable-mdns",
		},
	}

	return testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
}
