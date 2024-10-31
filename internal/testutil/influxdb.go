package testutil

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
)

const (
	InfluxDBOrg        = "my-org"
	InfluxDBBucket     = "my-bucket"
	InfluxDBAdminToken = "my-super-secret-admin-token"
	InfluxDBUsername   = "admin"
	InfluxDBPassword   = "password123"
)

type InfluxDBContainer struct {
	URL          string
	Token        string
	Client       influxdb2.Client
	ContainerID  string
	DockerClient *client.Client
	Organization string
	Bucket       string
}

func StartInfluxDB(t *testing.T) (*InfluxDBContainer, error) {
	// Set up Docker client
	dockerClient, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}

	// Pull and start container
	resp, url, token, err := startInfluxDBContainer(t, dockerClient)
	if err != nil {
		dockerClient.Close()
		return nil, err
	}

	// Create InfluxDB client
	influxClient := influxdb2.NewClient(url, token)

	// Test connection
	health, err := influxClient.Health(context.Background())
	if err != nil {
		dockerClient.Close()
		return nil, fmt.Errorf("failed to check InfluxDB health: %w", err)
	}
	if health.Status != "pass" {
		dockerClient.Close()
		return nil, fmt.Errorf("unhealthy InfluxDB status: %s", health.Status)
	}

	return &InfluxDBContainer{
		URL:          url,
		Token:        token,
		Client:       influxClient,
		ContainerID:  resp.ID,
		DockerClient: dockerClient,
		Organization: InfluxDBOrg,
		Bucket:       InfluxDBBucket,
	}, nil
}

func (c *InfluxDBContainer) Close() error {
	c.Client.Close()
	ctx := context.Background()
	if err := c.DockerClient.ContainerStop(ctx, c.ContainerID, container.StopOptions{}); err != nil {
		return fmt.Errorf("failed to stop container: %w", err)
	}
	c.DockerClient.Close()

	return nil
}

func startInfluxDBContainer(t *testing.T, dockerClient *client.Client) (container.CreateResponse, string, string, error) {
	t.Log("Starting container setup")
	ctx := context.Background()

	// Pull image with timeout
	t.Log("Pulling InfluxDB image")
	pullCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	_, err := dockerClient.ImagePull(pullCtx, "influxdb:2.7.10", image.PullOptions{})
	if err != nil {
		t.Log("Image pull failed:", err)
		return container.CreateResponse{}, "", "", fmt.Errorf("failed to pull InfluxDB image: %w", err)
	}
	t.Log("Image pull completed")

	// Create container with timeout
	t.Log("Creating container")
	createCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	resp, err := dockerClient.ContainerCreate(createCtx, &container.Config{
		Image: "influxdb:2.7.10",
		ExposedPorts: nat.PortSet{
			"8086/tcp": struct{}{},
		},
		Env: []string{
			"DOCKER_INFLUXDB_INIT_MODE=setup",
			"DOCKER_INFLUXDB_INIT_USERNAME=" + InfluxDBUsername,
			"DOCKER_INFLUXDB_INIT_PASSWORD=" + InfluxDBPassword,
			"DOCKER_INFLUXDB_INIT_ORG=" + InfluxDBOrg,
			"DOCKER_INFLUXDB_INIT_BUCKET=" + InfluxDBBucket,
			"DOCKER_INFLUXDB_INIT_ADMIN_TOKEN=" + InfluxDBAdminToken,
		},
	}, &container.HostConfig{
		PortBindings: nat.PortMap{
			"8086/tcp": []nat.PortBinding{{HostIP: "127.0.0.1", HostPort: "0"}},
		},
	}, nil, nil, "")
	if err != nil {
		t.Log("Container creation failed:", err)
		return container.CreateResponse{}, "", "", err
	}
	t.Log("Container created:", resp.ID)

	// Start container with timeout
	t.Log("Starting container")
	startCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := dockerClient.ContainerStart(startCtx, resp.ID, container.StartOptions{}); err != nil {
		t.Log("Container start failed:", err)
		return container.CreateResponse{}, "", "", err
	}

	// Get container info with timeout
	t.Log("Inspecting container")
	inspectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	containerJSON, err := dockerClient.ContainerInspect(inspectCtx, resp.ID)
	if err != nil {
		t.Log("Container inspect failed:", err)
		return container.CreateResponse{}, "", "", err
	}

	portBindings := containerJSON.NetworkSettings.Ports["8086/tcp"]
	if len(portBindings) == 0 {
		return container.CreateResponse{}, "", "", fmt.Errorf("no port bindings found for container")
	}

	influxdbURL := fmt.Sprintf("http://127.0.0.1:%s", portBindings[0].HostPort)

	// Wait for InfluxDB to be ready
	t.Log("Waiting for InfluxDB to become ready")
	httpClient := &http.Client{Timeout: 2 * time.Second}
	ready := false
	for i := 0; i < 30; i++ {
		req, err := http.NewRequest("GET", influxdbURL+"/health", nil)
		if err != nil {
			t.Log("Health check request creation failed", "error", err)
			time.Sleep(1 * time.Second)
			continue
		}

		resp, err := httpClient.Do(req)
		if err != nil {
			t.Log("Health check failed", "attempt", i+1, "error", err)
			time.Sleep(1 * time.Second)
			continue
		}
		resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			ready = true
			t.Log("InfluxDB is ready")
			break
		}
		time.Sleep(1 * time.Second)
	}

	if !ready {
		return container.CreateResponse{}, "", "", fmt.Errorf("timeout waiting for InfluxDB to be ready")
	}

	// Wait a bit more after health check passes
	time.Sleep(2 * time.Second)

	// Create an all-access token using the influx CLI inside the container
	execConfig := container.ExecOptions{
		Cmd: []string{
			"influx", "auth", "create",
			"--all-access",
			"-o", InfluxDBOrg,
			"--json",
		},
		AttachStdout: true,
		AttachStderr: true,
	}

	// Add timeout for exec create
	execCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	execIDResp, err := dockerClient.ContainerExecCreate(execCtx, resp.ID, execConfig)
	if err != nil {
		return container.CreateResponse{}, "", "", fmt.Errorf("failed to create exec: %w", err)
	}

	// Run the exec and capture output with timeout
	execStartCheck := container.ExecStartOptions{}
	execAttachResp, err := dockerClient.ContainerExecAttach(execCtx, execIDResp.ID, execStartCheck)
	if err != nil {
		return container.CreateResponse{}, "", "", fmt.Errorf("failed to attach to exec: %w", err)
	}
	defer execAttachResp.Close()

	// Add timeout for reading output
	outputCh := make(chan []byte)
	errCh := make(chan error)
	go func() {
		output, err := io.ReadAll(execAttachResp.Reader)
		if err != nil {
			errCh <- err
			return
		}
		outputCh <- output
	}()

	// Wait for output with timeout
	select {
	case output := <-outputCh:
		// Skip the first 8 bytes (Docker stream header) and take the rest as JSON
		cleanOutput := output[8:]

		var tokenResp struct {
			Token string `json:"token"`
		}
		if err := json.Unmarshal(cleanOutput, &tokenResp); err != nil {
			t.Log("Failed to parse JSON:", err)
			t.Log("Clean output:", string(cleanOutput))
			return container.CreateResponse{}, "", "", fmt.Errorf("failed to parse token response: %w", err)
		}
		return resp, influxdbURL, tokenResp.Token, nil

	case err := <-errCh:
		return container.CreateResponse{}, "", "", fmt.Errorf("failed to read exec output: %w", err)

	case <-time.After(10 * time.Second):
		return container.CreateResponse{}, "", "", fmt.Errorf("timeout waiting for exec output")
	}
}
