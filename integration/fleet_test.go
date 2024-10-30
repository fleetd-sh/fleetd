package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "github.com/tursodatabase/libsql-client-go/libsql"
	"golang.org/x/exp/rand"
	"google.golang.org/protobuf/types/known/timestamppb"

	"io"

	"fleetd.sh/auth"
	"fleetd.sh/device"
	authrpc "fleetd.sh/gen/auth/v1/authv1connect"
	devicerpc "fleetd.sh/gen/device/v1/devicev1connect"
	metricspb "fleetd.sh/gen/metrics/v1"
	metricsrpc "fleetd.sh/gen/metrics/v1/metricsv1connect"
	storagerpc "fleetd.sh/gen/storage/v1/storagev1connect"
	updatepb "fleetd.sh/gen/update/v1"
	updaterpc "fleetd.sh/gen/update/v1/updatev1connect"
	"fleetd.sh/internal/migrations"
	"fleetd.sh/metrics"
	"fleetd.sh/pkg/authclient"
	"fleetd.sh/pkg/deviceclient"
	"fleetd.sh/pkg/metricsclient"
	"fleetd.sh/pkg/updateclient"
	"fleetd.sh/storage"
	"fleetd.sh/update"
)

const (
	InfluxDBOrg        = "my-org"
	InfluxDBBucket     = "my-bucket"
	InfluxDBAdminToken = "my-super-secret-admin-token"
	InfluxDBUsername   = "admin"
	InfluxDBPassword   = "password123"
)

type Stack struct {
	AuthService       *auth.AuthService
	DeviceService     *device.DeviceService
	MetricsService    *metrics.MetricsService
	UpdateService     *update.UpdateService
	StorageService    *storage.StorageService
	AuthServiceURL    string
	DeviceServiceURL  string
	MetricsServiceURL string
	UpdateServiceURL  string
	StorageServiceURL string
	cleanup           func()
}

type TestDevice struct {
	Name       string
	ID         string
	Version    string
	DeviceType string
	Cleanup    func()
}

func TestFleetdIntegration(t *testing.T) {
	// Create a temporary directory for the test
	tempDir, err := os.MkdirTemp("", "fleetd-integration-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Initialize the test stack with dynamic ports
	stack, err := setupStack(t)
	if err != nil {
		t.Fatalf("Failed to set up stack: %v", err)
	}
	if stack != nil {
		defer stack.cleanup()
	}

	// Set up a test device with fleetd
	testDevice, err := setupTestDevice()
	if err != nil {
		t.Fatalf("Failed to set up test device: %v", err)
	}
	if testDevice != nil {
		defer testDevice.Cleanup()
	}

	// Register a test device
	t.Run("DeviceRegistration", func(t *testing.T) {
		ctx := context.Background()
		client := deviceclient.NewClient(stack.DeviceServiceURL)
		id, _, err := client.RegisterDevice(ctx, testDevice.Name, testDevice.DeviceType)
		assert.NoError(t, err)
		testDevice.ID = id

		// Verify device is registered
		deviceCh, errorCh := client.ListDevices(ctx)
		foundDevice := false
		for device := range deviceCh {
			if device.Id == testDevice.ID {
				foundDevice = true
				assert.Equal(t, testDevice.Name, device.Name)
				assert.Equal(t, testDevice.DeviceType, device.Type)
				break
			}
		}
		assert.NoError(t, <-errorCh)
		assert.True(t, foundDevice, "Registered device not found in device list")
	})

	// Test metric collection
	t.Run("MetricCollection", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		client := metricsclient.NewClient(stack.MetricsServiceURL)

		// Create metric with proper fields
		metric := &metricspb.Metric{
			Measurement: "temperature",
			Fields: map[string]float64{
				"value": 25.5,
			},
			Tags: map[string]string{
				"device_id": testDevice.ID,
				"type":      "temperature",
			},
			Timestamp: timestamppb.Now(),
		}

		// Send metrics with proper device ID
		t.Log("Sending metric to InfluxDB")
		success, err := client.SendMetrics(ctx, testDevice.ID, []*metricspb.Metric{metric})
		require.NoError(t, err)
		assert.True(t, success)

		// Wait for metrics to be processed
		t.Log("Waiting for metrics to be processed")
		time.Sleep(2 * time.Second)

		// Query metrics with proper authentication and timeout
		t.Log("Querying metrics")
		metricsCh, errCh := client.GetMetrics(
			ctx,
			testDevice.ID,
			"temperature",
			timestamppb.New(time.Now().Add(-1*time.Hour)),
			timestamppb.Now(),
		)

		// Add timeout for reading metrics
		done := make(chan bool)
		go func() {
			for metric := range metricsCh {
				t.Logf("Received metric: %+v", metric)
			}
			done <- true
		}()

		// Wait for either completion or timeout
		select {
		case err := <-errCh:
			require.NoError(t, err, "Error reading metrics")
		case <-done:
			t.Log("Successfully read all metrics")
		case <-time.After(5 * time.Second):
			t.Fatal("Timeout waiting for metrics")
		}
	})

	// Test update management
	t.Run("UpdateManagement", func(t *testing.T) {
		ctx := context.Background()
		client := updateclient.NewClient(stack.UpdateServiceURL)
		// Create update package with matching device type
		updateReq := &updatepb.CreateUpdatePackageRequest{
			Version:     "1.0.1",
			ReleaseDate: timestamppb.Now(),
			ChangeLog:   "Test update",
			DeviceTypes: []string{testDevice.DeviceType}, // Match the device type used in registration
		}

		success, err := client.CreateUpdatePackage(ctx, updateReq)
		require.NoError(t, err)
		assert.True(t, success)

		// Wait for update to be processed
		time.Sleep(2 * time.Second)

		// Check for updates with matching device type
		updates, err := client.GetAvailableUpdates(
			ctx,
			"TEST_DEVICE",
			timestamppb.New(time.Now().Add(-24*time.Hour)),
		)
		require.NoError(t, err)
		require.NotEmpty(t, updates)
		assert.Equal(t, "1.0.1", updates[0].Version)
	})
}

func setupStack(t *testing.T) (*Stack, error) {
	t.Log("Starting stack setup")

	// Create a temporary directory for the test
	tempDir, err := os.MkdirTemp("", "fleetd-integration-test")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}

	// Set up Docker client for container management
	t.Log("Setting up Docker client")
	dockerClient, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}
	defer dockerClient.Close()

	// Start InfluxDB container
	t.Log("Starting InfluxDB container")
	containerResp, influxdbURL, token, err := startInfluxDBContainer(t, dockerClient)
	if err != nil {
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("failed to start InfluxDB container: %w", err)
	}

	t.Log("Setting up InfluxDB client",
		"url", influxdbURL,
		"token", token[:8]+"...",
		"org", InfluxDBOrg,
		"bucket", InfluxDBBucket,
	)

	// Set up InfluxDB client with the all-access token
	influxClient := influxdb2.NewClient(influxdbURL, token)
	metricsService := metrics.NewMetricsService(influxClient, InfluxDBOrg, InfluxDBBucket)

	// Test the connection
	health, err := influxClient.Health(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to check InfluxDB health: %w", err)
	}
	t.Log("InfluxDB health check", "status", health.Status)

	// Set up SQLite database
	dbpath := "file://" + tempDir + "/fleetd.db"
	d, err := sql.Open("libsql", dbpath)
	if err != nil {
		return nil, fmt.Errorf("failed to create database: %w", err)
	}

	if err := migrations.MigrateUp(d); err != nil {
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}
	t.Log("Database URL", "url", dbpath)

	// Start the auth service server first
	// TODO: decouple clients from auth service server
	authService := auth.NewAuthService(d)
	authPath, authHandler := authrpc.NewAuthServiceHandler(authService)
	authMux := http.NewServeMux()
	authMux.Handle(authPath, authHandler)
	authServer := httptest.NewServer(authMux)

	authClient := authclient.NewClient(authServer.URL)
	deviceService := device.NewDeviceService(d, authClient)
	updateService := update.NewUpdateService(d)
	storageService := storage.NewStorageService(fmt.Sprintf("%s/storage", tempDir))

	// Create ConnectRPC handlers for each service
	devicePath, deviceHandler := devicerpc.NewDeviceServiceHandler(deviceService)
	metricsPath, metricsHandler := metricsrpc.NewMetricsServiceHandler(metricsService)
	updatePath, updateHandler := updaterpc.NewUpdateServiceHandler(updateService)
	storagePath, storageHandler := storagerpc.NewStorageServiceHandler(storageService)

	// Start remainingHTTP test servers with ConnectRPC handlers
	deviceMux := http.NewServeMux()
	deviceMux.Handle(devicePath, deviceHandler)
	deviceServer := httptest.NewServer(deviceMux)

	metricsMux := http.NewServeMux()
	metricsMux.Handle(metricsPath, metricsHandler)
	metricsServer := httptest.NewServer(metricsMux)

	updateMux := http.NewServeMux()
	updateMux.Handle(updatePath, updateHandler)
	updateServer := httptest.NewServer(updateMux)

	storageMux := http.NewServeMux()
	storageMux.Handle(storagePath, storageHandler)
	storageServer := httptest.NewServer(storageMux)

	cleanup := func() {
		os.RemoveAll(tempDir)
		authServer.Close()
		deviceServer.Close()
		metricsServer.Close()
		updateServer.Close()
		storageServer.Close()
		influxClient.Close()
		ctx := context.Background()
		if err := dockerClient.ContainerStop(ctx, containerResp.ID, container.StopOptions{}); err != nil {
			t.Error("Failed to stop container", "error", err)
		}
		if err := dockerClient.ContainerRemove(ctx, containerResp.ID, container.RemoveOptions{}); err != nil {
			t.Error("Failed to remove container", "error", err)
		}
	}

	t.Log("AuthServiceURL", "url", authServer.URL)
	t.Log("DeviceServiceURL", "url", deviceServer.URL)
	t.Log("MetricsServiceURL", "url", metricsServer.URL)
	t.Log("UpdateServiceURL", "url", updateServer.URL)
	t.Log("StorageServiceURL", "url", storageServer.URL)

	return &Stack{
		AuthService:       authService,
		DeviceService:     deviceService,
		MetricsService:    metricsService,
		UpdateService:     updateService,
		StorageService:    storageService,
		AuthServiceURL:    authServer.URL,
		DeviceServiceURL:  deviceServer.URL,
		MetricsServiceURL: metricsServer.URL,
		UpdateServiceURL:  updateServer.URL,
		StorageServiceURL: storageServer.URL,
		cleanup:           cleanup,
	}, nil
}

func setupTestDevice() (*TestDevice, error) {
	// In a real scenario, this would set up a container running fleetd
	// For this example, we'll simulate a device
	return &TestDevice{
		Name:       "test-device-001",
		Version:    "1.0.0",
		DeviceType: "TEST_DEVICE",
		Cleanup: func() {
			// Clean up resources if needed
		},
	}, nil
}

func (d *TestDevice) SendMetrics(url string) error {
	client := metricsclient.NewClient(url, metricsclient.WithLogger(slog.Default()))

	_, err := client.SendMetrics(context.Background(), d.ID, []*metricspb.Metric{
		{
			Measurement: "temperature",
			Fields: map[string]float64{
				"value": float64(rand.Intn(100)),
			},
		},
	})

	return err
}

func (d *TestDevice) CheckForUpdates(url string) error {
	client := updateclient.NewClient(url)
	lastUpdateDate := timestamppb.New(time.Now().Add(-1 * time.Hour))
	_, err := client.GetAvailableUpdates(context.Background(), d.DeviceType, lastUpdateDate)
	return err
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
	t.Log("Container started")

	// Get container info with timeout
	t.Log("Inspecting container")
	inspectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	containerJSON, err := dockerClient.ContainerInspect(inspectCtx, resp.ID)
	if err != nil {
		t.Log("Container inspect failed:", err)
		return container.CreateResponse{}, "", "", err
	}
	t.Log("Container inspection complete")

	portBindings := containerJSON.NetworkSettings.Ports["8086/tcp"]
	if len(portBindings) == 0 {
		return container.CreateResponse{}, "", "", fmt.Errorf("no port bindings found for container")
	}

	influxdbURL := fmt.Sprintf("http://127.0.0.1:%s", portBindings[0].HostPort)

	// Wait for InfluxDB to be ready
	t.Log("Waiting for InfluxDB to be ready...")
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

	t.Log("Creating exec", execConfig.Cmd)

	// Add timeout for exec create
	execCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	execIDResp, err := dockerClient.ContainerExecCreate(execCtx, resp.ID, execConfig)
	if err != nil {
		return container.CreateResponse{}, "", "", fmt.Errorf("failed to create exec: %w", err)
	}

	t.Log("Starting exec", execIDResp.ID)

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
		t.Log("Got output length:", len(output))
		if len(output) > 0 {
			t.Log("First few bytes:", output[:min(len(output), 20)])
		}
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

func stopInfluxDBContainer(dockerClient *client.Client, res container.CreateResponse) error {
	ctx := context.Background()
	return dockerClient.ContainerStop(ctx, res.ID, container.StopOptions{Timeout: nil}) // 10 seconds timeout
}
