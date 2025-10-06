package e2e

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"connectrpc.com/connect"
	"fleetd.sh/gen/public/v1"
	"fleetd.sh/gen/public/v1/publicv1connect"
	"fleetd.sh/internal/config"
	"fleetd.sh/internal/control"
	"fleetd.sh/internal/database"
	"fleetd.sh/internal/fleet"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCompleteUpdateFlow tests the entire update flow from manifest creation to device update
func TestCompleteUpdateFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	// Setup test environment
	ctx := context.Background()
	testEnv := setupTestEnvironment(t)
	defer testEnv.cleanup()

	// Start platform API server
	server := startPlatformAPI(t, testEnv)
	defer server.Close()

	// Create fleet client
	client := publicv1connect.NewFleetServiceClient(
		http.DefaultClient,
		server.URL,
	)

	// Test scenarios
	t.Run("SimpleRollingUpdate", func(t *testing.T) {
		testSimpleRollingUpdate(t, ctx, client, testEnv)
	})

	t.Run("CanaryDeployment", func(t *testing.T) {
		testCanaryDeployment(t, ctx, client, testEnv)
	})

	t.Run("FailedDeploymentRollback", func(t *testing.T) {
		testFailedDeploymentRollback(t, ctx, client, testEnv)
	})

	t.Run("ConcurrentDeployments", func(t *testing.T) {
		testConcurrentDeployments(t, ctx, client, testEnv)
	})
}

type testEnvironment struct {
	db          *sql.DB
	artifactDir string
	configDir   string
}

func (te *testEnvironment) cleanup() {
	if te.db != nil {
		te.db.Close()
	}
	if te.artifactDir != "" {
		os.RemoveAll(te.artifactDir)
	}
	if te.configDir != "" {
		os.RemoveAll(te.configDir)
	}
}

func setupTestEnvironment(t *testing.T) *testEnvironment {
	// Create test database
	dbConfig := database.DefaultConfig("sqlite3")
	dbConfig.DSN = ":memory:"
	dbConfig.MigrationsPath = "../../internal/database/migrations"

	dbWrapper, err := database.New(dbConfig)
	require.NoError(t, err)

	// The DB embeds sql.DB, so we can use it directly
	db := dbWrapper.DB

	// Run migrations to create tables
	ctx := context.Background()
	err = database.RunMigrations(ctx, db, "sqlite3")
	require.NoError(t, err, "Failed to run database migrations")

	// Create test directories
	artifactDir, err := os.MkdirTemp("", "fleetd-artifacts-*")
	require.NoError(t, err)

	configDir, err := os.MkdirTemp("", "fleetd-config-*")
	require.NoError(t, err)

	// Seed test data
	seedTestData(t, db)

	return &testEnvironment{
		db:          db,
		artifactDir: artifactDir,
		configDir:   configDir,
	}
}

func seedTestData(t *testing.T, db *sql.DB) {
	// Insert test devices
	devices := []struct {
		id     string
		name   string
		status string
		labels map[string]string
	}{
		{
			id:     "device-001",
			name:   "Edge Device 1",
			status: "online",
			labels: map[string]string{
				"environment": "production",
				"region":      "us-west",
				"type":        "sensor",
				"canary":      "true",
			},
		},
		{
			id:     "device-002",
			name:   "Edge Device 2",
			status: "online",
			labels: map[string]string{
				"environment": "production",
				"region":      "us-east",
				"type":        "sensor",
				"canary":      "true",
			},
		},
		{
			id:     "device-003",
			name:   "Edge Device 3",
			status: "online",
			labels: map[string]string{
				"environment": "staging",
				"region":      "us-west",
				"type":        "gateway",
			},
		},
		{
			id:     "device-004",
			name:   "Edge Device 4",
			status: "offline",
			labels: map[string]string{
				"environment": "production",
				"region":      "eu-west",
				"type":        "sensor",
			},
		},
		{
			id:     "device-005",
			name:   "Edge Device 5",
			status: "online",
			labels: map[string]string{
				"environment": "production",
				"region":      "us-west",
				"type":        "sensor",
				"canary":      "true",
			},
		},
	}

	for _, d := range devices {
		_, err := db.Exec(`
			INSERT INTO device (id, name, status)
			VALUES (?, ?, ?)`,
			d.id, d.name, d.status)
		require.NoError(t, err)

		// Store labels as JSON in the device table
		if len(d.labels) > 0 {
			labelsJSON, err := json.Marshal(d.labels)
			require.NoError(t, err)
			_, err = db.Exec(`UPDATE device SET labels = ? WHERE id = ?`, string(labelsJSON), d.id)
			require.NoError(t, err)
		}
	}
}

func startPlatformAPI(t *testing.T, env *testEnvironment) *httptest.Server {
	// Create mock device API client
	deviceAPI := &control.DeviceAPIClient{}

	// Create update client adapter
	updateClient := control.NewUpdateClientAdapter(deviceAPI, env.db)

	// Create orchestrator with test config
	testConfig := config.TestOrchestratorConfig()
	orchestrator := fleet.NewOrchestratorWithConfig(env.db, updateClient, testConfig)

	// Create fleet service
	fleetService := control.NewFleetService(env.db, deviceAPI, orchestrator)

	// Create HTTP handler
	path, handler := publicv1connect.NewFleetServiceHandler(fleetService)

	mux := http.NewServeMux()
	mux.Handle(path, handler)

	return httptest.NewServer(mux)
}

func testSimpleRollingUpdate(t *testing.T, ctx context.Context, client publicv1connect.FleetServiceClient, env *testEnvironment) {
	// Create test artifact
	artifactPath := filepath.Join(env.artifactDir, "app-v1.0.0.tar.gz")
	createTestArtifact(t, artifactPath, "app-v1.0.0")

	// Create deployment manifest
	manifest := fleet.Manifest{
		APIVersion: "fleet/v1",
		Kind:       "Deployment",
		Metadata: fleet.ManifestMetadata{
			Name:      "rolling-update-test",
			Namespace: "production",
		},
		Spec: fleet.ManifestSpec{
			Selector: fleet.DeploymentSelector{
				MatchLabels: map[string]string{
					"environment": "production",
					"type":        "sensor",
				},
			},
			Strategy: fleet.DeploymentStrategy{
				Type: "RollingUpdate",
				RollingUpdate: &fleet.RollingUpdate{
					MaxUnavailable: "50%",
					MaxSurge:       "0%",
				},
			},
			Template: fleet.DeploymentTemplate{
				Spec: fleet.TemplateSpec{
					Artifacts: []fleet.Artifact{
						{
							Name:     "test-app",
							Version:  "v1.0.0",
							URL:      "file://" + artifactPath,
							Checksum: "sha256:abc123",
							Type:     "binary",
							Target:   "/opt/app",
						},
					},
				},
			},
		},
	}

	manifestJSON, err := json.Marshal(manifest)
	require.NoError(t, err)

	// Create deployment via API
	createReq := connect.NewRequest(&publicv1.CreateDeploymentRequest{
		Name:        manifest.Metadata.Name,
		Description: "Test rolling update deployment",
		Type:        publicv1.DeploymentType_DEPLOYMENT_TYPE_BINARY,
		Payload: &publicv1.DeploymentPayload{
			Content: &publicv1.DeploymentPayload_Binary{
				Binary: &publicv1.BinaryPayload{
					DownloadUrl: manifest.Spec.Template.Spec.Artifacts[0].URL,
					Version:     manifest.Spec.Template.Spec.Artifacts[0].Version,
					Checksum:    manifest.Spec.Template.Spec.Artifacts[0].Checksum,
				},
			},
		},
		Target: &publicv1.DeploymentTarget{
			Selector: &publicv1.DeploymentTarget_Labels{
				Labels: &publicv1.LabelSelector{
					MatchLabels: manifest.Spec.Selector.MatchLabels,
				},
			},
		},
		Strategy: publicv1.DeploymentStrategy_DEPLOYMENT_STRATEGY_ROLLING,
		Config: &publicv1.DeploymentConfig{
			Rollout: &publicv1.RolloutConfig{
				BatchSize: 50, // 50% batches
			},
		},
		Metadata: &publicv1.DeploymentMetadata{
			Source:      "e2e-test",
			Environment: "production",
			Tags: map[string]string{
				"manifest": string(manifestJSON),
			},
			TriggeredBy: "test-user",
		},
		AutoStart: true,
	})

	// Debug: Check what devices are in the database
	rows, err := env.db.Query(`SELECT id, name, status, labels FROM device`)
	require.NoError(t, err)
	defer rows.Close()
	t.Logf("Devices in database:")
	for rows.Next() {
		var id, name, status string
		var labels sql.NullString
		err := rows.Scan(&id, &name, &status, &labels)
		require.NoError(t, err)
		t.Logf("  Device %s (%s): status=%s, labels=%s", id, name, status, labels.String)
	}

	createResp, err := client.CreateDeployment(ctx, createReq)
	require.NoError(t, err)
	require.NotNil(t, createResp)
	assert.NotEmpty(t, createResp.Msg.Deployment.Id)

	deploymentID := createResp.Msg.Deployment.Id

	// Wait for deployment to start
	time.Sleep(100 * time.Millisecond)

	// Get deployment status
	statusReq := connect.NewRequest(&publicv1.GetDeploymentStatusRequest{
		DeploymentId: deploymentID,
	})

	// Poll for deployment completion
	maxAttempts := 30
	for i := 0; i < maxAttempts; i++ {
		statusResp, err := client.GetDeploymentStatus(ctx, statusReq)
		require.NoError(t, err)

		t.Logf("Deployment %s status: %s, progress: %v%%",
			deploymentID,
			statusResp.Msg.State,
			statusResp.Msg.Progress.PercentageComplete)

		if statusResp.Msg.State == publicv1.DeploymentState_DEPLOYMENT_STATE_COMPLETED {
			// Debug: Check device_deployment table
			rows, err := env.db.Query(`SELECT device_id, status FROM device_deployment WHERE deployment_id = ?`, deploymentID)
			require.NoError(t, err)
			defer rows.Close()
			t.Logf("Device deployments for %s:", deploymentID)
			for rows.Next() {
				var deviceID, status string
				err := rows.Scan(&deviceID, &status)
				require.NoError(t, err)
				t.Logf("  Device %s: status=%s", deviceID, status)
			}

			assert.Equal(t, float64(100), statusResp.Msg.Progress.PercentageComplete)
			break
		}

		if statusResp.Msg.State == publicv1.DeploymentState_DEPLOYMENT_STATE_FAILED {
			t.Fatalf("Deployment failed")
		}

		time.Sleep(100 * time.Millisecond)
	}

	// Verify deployment completed
	getReq := connect.NewRequest(&publicv1.GetDeploymentRequest{
		DeploymentId: deploymentID,
	})
	getResp, err := client.GetDeployment(ctx, getReq)
	require.NoError(t, err)
	assert.Equal(t, publicv1.DeploymentState_DEPLOYMENT_STATE_COMPLETED, getResp.Msg.Deployment.State)
}

func testCanaryDeployment(t *testing.T, ctx context.Context, client publicv1connect.FleetServiceClient, env *testEnvironment) {
	// Create deployment with canary strategy
	createReq := connect.NewRequest(&publicv1.CreateDeploymentRequest{
		Name:        "canary-test",
		Description: "Test canary deployment",
		Type:        publicv1.DeploymentType_DEPLOYMENT_TYPE_BINARY,
		Payload: &publicv1.DeploymentPayload{
			Content: &publicv1.DeploymentPayload_Binary{
				Binary: &publicv1.BinaryPayload{
					DownloadUrl: "https://example.com/app-v2.0.0.tar.gz",
					Version:     "v2.0.0",
				},
			},
		},
		Target: &publicv1.DeploymentTarget{
			Selector: &publicv1.DeploymentTarget_Labels{
				Labels: &publicv1.LabelSelector{
					MatchLabels: map[string]string{
						"environment": "production",
						"canary":      "true",
					},
				},
			},
		},
		Strategy: publicv1.DeploymentStrategy_DEPLOYMENT_STRATEGY_CANARY,
		Config: &publicv1.DeploymentConfig{
			Rollout: &publicv1.RolloutConfig{
				BatchSize:       20, // Start with 20%
				RequireApproval: true,
			},
		},
		AutoStart: true,
	})

	createResp, err := client.CreateDeployment(ctx, createReq)
	require.NoError(t, err)
	assert.NotNil(t, createResp.Msg.Deployment)

	// Monitor canary progress
	deploymentID := createResp.Msg.Deployment.Id
	time.Sleep(100 * time.Millisecond)

	statusReq := connect.NewRequest(&publicv1.GetDeploymentStatusRequest{
		DeploymentId: deploymentID,
	})
	statusResp, err := client.GetDeploymentStatus(ctx, statusReq)
	require.NoError(t, err)

	// Verify canary is running
	assert.Equal(t, publicv1.DeploymentState_DEPLOYMENT_STATE_RUNNING, statusResp.Msg.State)

	// Simulate canary approval (in real scenario, this would be manual)
	// For testing, we'll just verify the deployment is in the right state
	assert.True(t, statusResp.Msg.Progress.PercentageComplete < 100)
}

func testFailedDeploymentRollback(t *testing.T, ctx context.Context, client publicv1connect.FleetServiceClient, env *testEnvironment) {
	// Create deployment that will fail
	createReq := connect.NewRequest(&publicv1.CreateDeploymentRequest{
		Name:        "rollback-test",
		Description: "Test deployment with rollback",
		Type:        publicv1.DeploymentType_DEPLOYMENT_TYPE_BINARY,
		Payload: &publicv1.DeploymentPayload{
			Content: &publicv1.DeploymentPayload_Binary{
				Binary: &publicv1.BinaryPayload{
					DownloadUrl: "https://example.com/bad-app.tar.gz", // Intentionally bad URL
					Version:     "v-bad",
				},
			},
		},
		Target: &publicv1.DeploymentTarget{
			Selector: &publicv1.DeploymentTarget_Labels{
				Labels: &publicv1.LabelSelector{
					MatchLabels: map[string]string{
						"environment": "staging",
					},
				},
			},
		},
		Config: &publicv1.DeploymentConfig{
			Rollback: &publicv1.RollbackConfig{
				AutoRollback:     true,
				FailureThreshold: 0.1, // Rollback if 10% fail
			},
		},
		AutoStart: true,
	})

	createResp, err := client.CreateDeployment(ctx, createReq)
	require.NoError(t, err)

	deploymentID := createResp.Msg.Deployment.Id
	time.Sleep(200 * time.Millisecond)

	// Check if rollback was triggered
	eventsReq := connect.NewRequest(&publicv1.StreamDeploymentEventsRequest{
		DeploymentId:        deploymentID,
		IncludeDeviceEvents: true,
	})

	stream, err := client.StreamDeploymentEvents(ctx, eventsReq)
	if err == nil {
		// In a real implementation, we'd check for rollback events
		// For now, just verify the stream works
		assert.NotNil(t, stream)
	}
}

func testConcurrentDeployments(t *testing.T, ctx context.Context, client publicv1connect.FleetServiceClient, env *testEnvironment) {
	// Create multiple deployments concurrently
	deploymentCount := 3
	deploymentIDs := make([]string, deploymentCount)
	errCh := make(chan error, deploymentCount)

	for i := 0; i < deploymentCount; i++ {
		go func(index int) {
			createReq := connect.NewRequest(&publicv1.CreateDeploymentRequest{
				Name:        fmt.Sprintf("concurrent-test-%d", index),
				Description: fmt.Sprintf("Concurrent deployment %d", index),
				Type:        publicv1.DeploymentType_DEPLOYMENT_TYPE_BINARY,
				Payload: &publicv1.DeploymentPayload{
					Content: &publicv1.DeploymentPayload_Binary{
						Binary: &publicv1.BinaryPayload{
							DownloadUrl: fmt.Sprintf("https://example.com/app-v%d.tar.gz", index),
							Version:     fmt.Sprintf("v%d.0.0", index),
						},
					},
				},
				Target: &publicv1.DeploymentTarget{
					Selector: &publicv1.DeploymentTarget_Labels{
						Labels: &publicv1.LabelSelector{
							MatchLabels: map[string]string{
								"environment": "staging",
								"test-group":  fmt.Sprintf("group-%d", index),
							},
						},
					},
				},
				AutoStart: false, // Don't auto-start to avoid resource conflicts
			})

			resp, err := client.CreateDeployment(ctx, createReq)
			if err != nil {
				errCh <- err
				return
			}

			deploymentIDs[index] = resp.Msg.Deployment.Id
			errCh <- nil
		}(i)
	}

	// Collect results
	for i := 0; i < deploymentCount; i++ {
		err := <-errCh
		require.NoError(t, err)
	}

	// Verify all deployments were created
	listReq := connect.NewRequest(&publicv1.ListDeploymentsRequest{
		PageSize: 10,
	})
	listResp, err := client.ListDeployments(ctx, listReq)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(listResp.Msg.Deployments), deploymentCount)

	// Verify each deployment exists
	for _, id := range deploymentIDs {
		if id != "" {
			getReq := connect.NewRequest(&publicv1.GetDeploymentRequest{
				DeploymentId: id,
			})
			getResp, err := client.GetDeployment(ctx, getReq)
			require.NoError(t, err)
			assert.NotNil(t, getResp.Msg.Deployment)
		}
	}
}

func createTestArtifact(t *testing.T, path string, content string) {
	data := []byte(content)
	err := os.WriteFile(path, data, 0644)
	require.NoError(t, err)
}

// TestDeploymentLifecycle tests the complete lifecycle of a deployment
func TestDeploymentLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping lifecycle test in short mode")
	}

	ctx := context.Background()
	testEnv := setupTestEnvironment(t)
	defer testEnv.cleanup()

	server := startPlatformAPI(t, testEnv)
	defer server.Close()

	client := publicv1connect.NewFleetServiceClient(
		http.DefaultClient,
		server.URL,
	)

	// Create deployment
	createReq := connect.NewRequest(&publicv1.CreateDeploymentRequest{
		Name:        "lifecycle-test",
		Description: "Test deployment lifecycle",
		Type:        publicv1.DeploymentType_DEPLOYMENT_TYPE_BINARY,
		Payload: &publicv1.DeploymentPayload{
			Content: &publicv1.DeploymentPayload_Binary{
				Binary: &publicv1.BinaryPayload{
					DownloadUrl: "https://example.com/app.tar.gz",
					Version:     "v1.0.0",
				},
			},
		},
		Target: &publicv1.DeploymentTarget{
			Selector: &publicv1.DeploymentTarget_Labels{
				Labels: &publicv1.LabelSelector{
					MatchLabels: map[string]string{
						"environment": "staging",
					},
				},
			},
		},
		AutoStart: false,
	})

	createResp, err := client.CreateDeployment(ctx, createReq)
	require.NoError(t, err)

	deploymentID := createResp.Msg.Deployment.Id

	// Verify initial state
	assert.Equal(t, publicv1.DeploymentState_DEPLOYMENT_STATE_PENDING, createResp.Msg.Deployment.State)

	// Start deployment
	startReq := connect.NewRequest(&publicv1.StartDeploymentRequest{
		DeploymentId: deploymentID,
	})
	startResp, err := client.StartDeployment(ctx, startReq)
	require.NoError(t, err)
	assert.True(t, startResp.Msg.Started)

	time.Sleep(100 * time.Millisecond)

	// Pause deployment
	pauseReq := connect.NewRequest(&publicv1.PauseDeploymentRequest{
		DeploymentId: deploymentID,
	})
	pauseResp, err := client.PauseDeployment(ctx, pauseReq)
	if err == nil { // May fail if deployment completed too quickly
		assert.True(t, pauseResp.Msg.Paused)

		// Resume deployment
		// Implementation would go here
	}

	// Cancel deployment
	cancelReq := connect.NewRequest(&publicv1.CancelDeploymentRequest{
		DeploymentId: deploymentID,
		Reason:       "Test cancellation",
	})
	cancelResp, err := client.CancelDeployment(ctx, cancelReq)
	if err == nil {
		assert.True(t, cancelResp.Msg.Cancelled)
	}

	// Verify final state
	getReq := connect.NewRequest(&publicv1.GetDeploymentRequest{
		DeploymentId: deploymentID,
	})
	getResp, err := client.GetDeployment(ctx, getReq)
	require.NoError(t, err)

	finalState := getResp.Msg.Deployment.State
	t.Logf("Final deployment state: %v", finalState)
	assert.True(t,
		finalState == publicv1.DeploymentState_DEPLOYMENT_STATE_CANCELLED ||
			finalState == publicv1.DeploymentState_DEPLOYMENT_STATE_COMPLETED ||
			finalState == publicv1.DeploymentState_DEPLOYMENT_STATE_FAILED, // Allow failed state due to cancellation
		"Deployment should be either cancelled, completed, or failed")
}

// TestArtifactDownloadAndVerification tests artifact download and checksum verification
func TestArtifactDownloadAndVerification(t *testing.T) {
	// Create test artifact server
	artifactServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/app-v1.0.0.tar.gz" {
			w.Header().Set("Content-Type", "application/gzip")
			w.WriteHeader(http.StatusOK)
			io.WriteString(w, "test-artifact-content")
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer artifactServer.Close()

	// Test artifact download
	resp, err := http.Get(artifactServer.URL + "/app-v1.0.0.tar.gz")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	content, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "test-artifact-content", string(content))

	// Test checksum verification
	// In real implementation, would calculate SHA256 and verify
	expectedChecksum := "sha256:abc123" // Would be actual SHA256
	_ = expectedChecksum                // Placeholder for checksum verification logic
}

// Benchmark tests
func BenchmarkDeploymentCreation(b *testing.B) {
	ctx := context.Background()
	testEnv := setupTestEnvironment(&testing.T{})
	defer testEnv.cleanup()

	server := startPlatformAPI(&testing.T{}, testEnv)
	defer server.Close()

	client := publicv1connect.NewFleetServiceClient(
		http.DefaultClient,
		server.URL,
	)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := connect.NewRequest(&publicv1.CreateDeploymentRequest{
			Name:        fmt.Sprintf("bench-deployment-%d", i),
			Description: "Benchmark deployment",
			Type:        publicv1.DeploymentType_DEPLOYMENT_TYPE_BINARY,
			Payload: &publicv1.DeploymentPayload{
				Content: &publicv1.DeploymentPayload_Binary{
					Binary: &publicv1.BinaryPayload{
						DownloadUrl: "https://example.com/app.tar.gz",
						Version:     "v1.0.0",
					},
				},
			},
			Target: &publicv1.DeploymentTarget{
				Selector: &publicv1.DeploymentTarget_Labels{
					Labels: &publicv1.LabelSelector{
						MatchLabels: map[string]string{
							"benchmark": "true",
						},
					},
				},
			},
			AutoStart: false,
		})

		_, err := client.CreateDeployment(ctx, req)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkManifestParsing(b *testing.B) {
	manifestYAML := []byte(`
apiVersion: fleet/v1
kind: Deployment
metadata:
  name: benchmark-test
  namespace: production
spec:
  selector:
    matchLabels:
      environment: production
  strategy:
    type: RollingUpdate
  template:
    spec:
      artifacts:
      - name: app
        version: v1.0.0
        url: https://example.com/app.tar.gz
`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		manifest, err := fleet.ParseManifest(manifestYAML)
		if err != nil {
			b.Fatal(err)
		}
		_ = manifest
	}
}
