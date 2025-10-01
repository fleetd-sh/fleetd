package integration

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/timestamppb"

	publicpb "fleetd.sh/gen/public/v1"
	"fleetd.sh/gen/public/v1/publicv1connect"
)

// TestEnvironment encapsulates the test infrastructure
type TestEnvironment struct {
	TempDir      string
	DatabasePath string
	ConfigPath   string
	ArtifactDir  string
	LogDir       string

	// Server addresses
	PlatformAddr string
	DeviceAddr   string
	HealthAddr   string

	// gRPC connections
	PlatformConn *grpc.ClientConn
	DeviceConn   *grpc.ClientConn
	HealthConn   *grpc.ClientConn

	// gRPC clients
	// Note: DeploymentClient removed - proto was deleted as unused
	FleetClient      publicv1connect.FleetServiceClient
	// Note: HealthClient import removed as it causes compilation errors
}

// NewTestEnvironment creates a new test environment
func NewTestEnvironment(t *testing.T) *TestEnvironment {
	tempDir := t.TempDir()

	env := &TestEnvironment{
		TempDir:      tempDir,
		DatabasePath: filepath.Join(tempDir, "test.db"),
		ConfigPath:   filepath.Join(tempDir, "config.toml"),
		ArtifactDir:  filepath.Join(tempDir, "artifacts"),
		LogDir:       filepath.Join(tempDir, "logs"),
	}

	// Create directories
	require.NoError(t, os.MkdirAll(env.ArtifactDir, 0755))
	require.NoError(t, os.MkdirAll(env.LogDir, 0755))

	return env
}

// Connect establishes gRPC connections to test servers
func (env *TestEnvironment) Connect(t *testing.T) {
	var err error

	if env.PlatformAddr != "" {
		env.PlatformConn, err = grpc.Dial(env.PlatformAddr,
			grpc.WithTransportCredentials(insecure.NewCredentials()))
		require.NoError(t, err)
		// TODO: Update to use Connect-RPC client instead of gRPC
		// env.FleetClient = publicv1connect.NewFleetServiceClient(...)
	}

	if env.DeviceAddr != "" {
		env.DeviceConn, err = grpc.Dial(env.DeviceAddr,
			grpc.WithTransportCredentials(insecure.NewCredentials()))
		require.NoError(t, err)
		// DeploymentClient removed - proto was deleted as unused
	}

	if env.HealthAddr != "" {
		env.HealthConn, err = grpc.Dial(env.HealthAddr,
			grpc.WithTransportCredentials(insecure.NewCredentials()))
		require.NoError(t, err)
		// TODO: Update to use Connect-RPC client instead of gRPC
		// env.HealthClient = healthpbconnect.NewHealthServiceClient(...)
	}
}

// Cleanup closes all connections and cleans up resources
func (env *TestEnvironment) Cleanup() {
	if env.PlatformConn != nil {
		env.PlatformConn.Close()
	}
	if env.DeviceConn != nil {
		env.DeviceConn.Close()
	}
	if env.HealthConn != nil {
		env.HealthConn.Close()
	}
}

// TestFixtures provides test data generation utilities
type TestFixtures struct {
	env *TestEnvironment
}

// NewTestFixtures creates a new test fixtures helper
func NewTestFixtures(env *TestEnvironment) *TestFixtures {
	return &TestFixtures{env: env}
}

// CreateBinaryArtifact creates a test binary artifact
func (f *TestFixtures) CreateBinaryArtifact(t *testing.T, name string, size int) *TestArtifact {
	data := make([]byte, size)
	_, err := rand.Read(data)
	require.NoError(t, err)

	// Add ELF header for Linux binary
	if size >= 4 {
		copy(data, []byte{0x7f, 'E', 'L', 'F'})
	}

	return &TestArtifact{
		Name:     name,
		Version:  "1.0.0",
		Type:     "binary",
		Data:     data,
		Checksum: calculateChecksum(data),
	}
}

// CreateScriptArtifact creates a test script artifact
func (f *TestFixtures) CreateScriptArtifact(t *testing.T, name, script string) *TestArtifact {
	data := []byte(script)

	return &TestArtifact{
		Name:     name,
		Version:  "1.0.0",
		Type:     "script",
		Data:     data,
		Checksum: calculateChecksum(data),
	}
}

// CreateContainerArtifact creates a test container image artifact
func (f *TestFixtures) CreateContainerArtifact(t *testing.T, name, dockerfile string) *TestArtifact {
	data := []byte(dockerfile)

	return &TestArtifact{
		Name:     name,
		Version:  "1.0.0",
		Type:     "container",
		Data:     data,
		Checksum: calculateChecksum(data),
	}
}

// CreateConfigArtifact creates a test configuration artifact
func (f *TestFixtures) CreateConfigArtifact(t *testing.T, name string, config map[string]interface{}) *TestArtifact {
	data := []byte(fmt.Sprintf("%v", config))

	return &TestArtifact{
		Name:     name,
		Version:  "1.0.0",
		Type:     "config",
		Data:     data,
		Checksum: calculateChecksum(data),
	}
}

// TestArtifact represents a test artifact
type TestArtifact struct {
	Name     string
	Version  string
	Type     string
	Data     []byte
	Checksum string
	Metadata map[string]string
}

// Upload uploads the artifact to the control service
// NOTE: Commented out - DeploymentServiceClient no longer exists after proto cleanup
/*
func (a *TestArtifact) Upload(t *testing.T, client publicpb.DeploymentServiceClient) string {
	ctx := context.Background()

	stream, err := client.UploadArtifact(ctx)
	require.NoError(t, err)

	// Send metadata
	err = stream.Send(&publicpb.UploadArtifactRequest{
		Data: &publicpb.UploadArtifactRequest_Metadata{
			Metadata: &publicpb.ArtifactMetadata{
				Name:      a.Name,
				Version:   a.Version,
				Type:      a.Type,
				SizeBytes: int64(len(a.Data)),
				Checksum:  a.Checksum,
				Metadata:  a.Metadata,
			},
		},
	})
	require.NoError(t, err)

	// Send data in chunks
	chunkSize := 64 * 1024 // 64KB chunks
	for offset := 0; offset < len(a.Data); offset += chunkSize {
		end := offset + chunkSize
		if end > len(a.Data) {
			end = len(a.Data)
		}

		err = stream.Send(&publicpb.UploadArtifactRequest{
			Data: &publicpb.UploadArtifactRequest_Chunk{
				Chunk: a.Data[offset:end],
			},
		})
		require.NoError(t, err)
	}

	resp, err := stream.CloseAndRecv()
	require.NoError(t, err)

	return resp.ArtifactId
}
*/

// SaveToFile saves the artifact to a file
func (a *TestArtifact) SaveToFile(t *testing.T, path string) {
	err := os.WriteFile(path, a.Data, 0644)
	require.NoError(t, err)
}

// TestDevice represents a mock device for testing
type TestDevice struct {
	ID           string
	Name         string
	Type         string
	Labels       map[string]string
	Groups       []string
	Platform     string
	Architecture string
	Version      string
	Status       string
}

// NewTestDevice creates a new test device
func NewTestDevice(id string) *TestDevice {
	return &TestDevice{
		ID:           id,
		Name:         fmt.Sprintf("test-device-%s", id),
		Labels:       make(map[string]string),
		Groups:       []string{"test"},
		Platform:     "linux",
		Architecture: "amd64",
		Version:      "1.0.0",
		Status:       "online",
	}
}

// ToProto converts the test device to a protobuf message
func (d *TestDevice) ToProto() *publicpb.Device {
	// Convert status string to enum
	status := publicpb.DeviceStatus_DEVICE_STATUS_UNSPECIFIED
	switch d.Status {
	case "online":
		status = publicpb.DeviceStatus_DEVICE_STATUS_ONLINE
	case "offline":
		status = publicpb.DeviceStatus_DEVICE_STATUS_OFFLINE
	}

	return &publicpb.Device{
		Id:       d.ID,
		Name:     d.Name,
		Type:     d.Type,
		Version:  d.Version,
		Status:   status,
		LastSeen: timestamppb.Now(),
		Tags:     d.Groups, // Use Groups as Tags
		SystemInfo: &publicpb.SystemInfo{
			Os:        d.Platform,
			OsVersion: d.Architecture,
		},
	}
}

// TestDeploymentBuilder helps build test deployments
type TestDeploymentBuilder struct {
	deployment *publicpb.CreateDeploymentRequest
}

// NewTestDeployment creates a new test deployment builder
func NewTestDeployment(name string) *TestDeploymentBuilder {
	return &TestDeploymentBuilder{
		deployment: &publicpb.CreateDeploymentRequest{
			Name:        name,
			Description: fmt.Sprintf("Test deployment %s", name),
			Type:        publicpb.DeploymentType_DEPLOYMENT_TYPE_BINARY,
			Strategy:    publicpb.DeploymentStrategy_DEPLOYMENT_STRATEGY_ROLLING,
			Target: &publicpb.DeploymentTarget{
				Selector: &publicpb.DeploymentTarget_Devices{
					Devices: &publicpb.DeviceSelector{},
				},
			},
			Config: &publicpb.DeploymentConfig{},
		},
	}
}

// WithDescription sets the deployment description
func (b *TestDeploymentBuilder) WithDescription(desc string) *TestDeploymentBuilder {
	b.deployment.Description = desc
	return b
}

// WithArtifact sets the deployment artifact
func (b *TestDeploymentBuilder) WithArtifact(artifactID string) *TestDeploymentBuilder {
	// TODO: Update when DeploymentPayload structure is clearer
	// b.deployment.Payload = ...
	return b
}

// WithStrategy sets the deployment strategy
func (b *TestDeploymentBuilder) WithStrategy(strategy publicpb.DeploymentStrategy) *TestDeploymentBuilder {
	b.deployment.Strategy = strategy
	return b
}

// WithTargetDevices sets target devices
func (b *TestDeploymentBuilder) WithTargetDevices(deviceIDs ...string) *TestDeploymentBuilder {
	b.deployment.Target.Selector = &publicpb.DeploymentTarget_Devices{
		Devices: &publicpb.DeviceSelector{
			DeviceIds: deviceIDs,
		},
	}
	return b
}

// WithTargetGroups sets target groups
func (b *TestDeploymentBuilder) WithTargetGroups(groupIDs ...string) *TestDeploymentBuilder {
	b.deployment.Target.Selector = &publicpb.DeploymentTarget_Groups{
		Groups: &publicpb.GroupSelector{
			GroupIds: groupIDs,
		},
	}
	return b
}

// WithLabelSelectors sets label selectors
func (b *TestDeploymentBuilder) WithLabelSelectors(labels map[string]string) *TestDeploymentBuilder {
	b.deployment.Target.Selector = &publicpb.DeploymentTarget_Labels{
		Labels: &publicpb.LabelSelector{
			MatchLabels: labels,
		},
	}
	return b
}

// WithRolloutPolicy sets the rollout policy
func (b *TestDeploymentBuilder) WithRolloutPolicy(batchSize int, autoRollback bool) *TestDeploymentBuilder {
	if b.deployment.Config == nil {
		b.deployment.Config = &publicpb.DeploymentConfig{}
	}
	if b.deployment.Config.Rollout == nil {
		b.deployment.Config.Rollout = &publicpb.RolloutConfig{}
	}
	b.deployment.Config.Rollout.BatchSize = int32(batchSize)
	// TODO: Set auto_rollback when field is available in RolloutConfig
	return b
}

// WithHealthCheck enables health checks
func (b *TestDeploymentBuilder) WithHealthCheck(timeout int32) *TestDeploymentBuilder {
	// Note: ValidationPolicy is not yet implemented in the proto
	// This is a placeholder for future health check feature
	// TODO: Implement when ValidationPolicy is added to proto
	return b
}

// Build returns the deployment request
func (b *TestDeploymentBuilder) Build() *publicpb.CreateDeploymentRequest {
	return b.deployment
}

// Monitoring utilities

// NOTE: DeploymentMonitor commented out - DeploymentServiceClient no longer exists after proto cleanup
/*
// DeploymentMonitor monitors deployment progress
type DeploymentMonitor struct {
	client       publicpb.DeploymentServiceClient
	deploymentID string
	interval     time.Duration
	timeout      time.Duration
}

// NewDeploymentMonitor creates a new deployment monitor
func NewDeploymentMonitor(client publicpb.DeploymentServiceClient, deploymentID string) *DeploymentMonitor {
	return &DeploymentMonitor{
		client:       client,
		deploymentID: deploymentID,
		interval:     100 * time.Millisecond,
		timeout:      30 * time.Second,
	}
}
*/

// NOTE: DeploymentMonitor methods commented out - DeploymentServiceClient no longer exists
/*
// WaitForCompletion waits for the deployment to complete
func (m *DeploymentMonitor) WaitForCompletion(t *testing.T) *publicpb.GetDeploymentStatusResponse {
	ctx := context.Background()
	deadline := time.Now().Add(m.timeout)

	for time.Now().Before(deadline) {
		resp, err := m.client.GetDeploymentStatus(ctx, &publicpb.GetDeploymentStatusRequest{
			DeploymentId: m.deploymentID,
		})
		require.NoError(t, err)

		// Check if deployment is complete
		if resp.Status.UpdatedDevices+resp.Status.FailedDevices == resp.Status.TotalDevices {
			return resp.Status
		}

		time.Sleep(m.interval)
	}

	t.Fatalf("deployment %s did not complete within timeout", m.deploymentID)
	return nil
}

// WaitForState waits for the deployment to reach a specific state
func (m *DeploymentMonitor) WaitForState(t *testing.T, state publicpb.DeploymentState) {
	ctx := context.Background()
	deadline := time.Now().Add(m.timeout)

	for time.Now().Before(deadline) {
		resp, err := m.client.GetDeployment(ctx, &publicpb.GetDeploymentRequest{
			DeploymentId: m.deploymentID,
		})
		require.NoError(t, err)

		if resp.Deployment.State == state {
			return
		}

		time.Sleep(m.interval)
	}

	t.Fatalf("deployment %s did not reach state %v within timeout", m.deploymentID, state)
}

// GetProgress returns current deployment progress
func (m *DeploymentMonitor) GetProgress(t *testing.T) float64 {
	ctx := context.Background()

	resp, err := m.client.GetDeploymentStatus(ctx, &publicpb.GetDeploymentStatusRequest{
		DeploymentId: m.deploymentID,
	})
	require.NoError(t, err)

	return resp.Status.ProgressPercentage
}
*/

// Helper functions

// calculateChecksum calculates SHA256 checksum of data
func calculateChecksum(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// GenerateRandomID generates a random ID
func GenerateRandomID(prefix string) string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%s-%x", prefix, b)
}

// CreateTestFile creates a test file with content
func CreateTestFile(t *testing.T, dir, name string, content []byte) string {
	path := filepath.Join(dir, name)
	err := os.WriteFile(path, content, 0644)
	require.NoError(t, err)
	return path
}

// AssertFileContains checks if file contains expected content
func AssertFileContains(t *testing.T, path string, expected string) {
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(content), expected)
}

// AssertFileExists checks if file exists
func AssertFileExists(t *testing.T, path string) {
	_, err := os.Stat(path)
	require.NoError(t, err)
}

// AssertFileNotExists checks if file does not exist
func AssertFileNotExists(t *testing.T, path string) {
	_, err := os.Stat(path)
	require.True(t, os.IsNotExist(err))
}

// WaitForCondition waits for a condition to be true
func WaitForCondition(t *testing.T, timeout time.Duration, check func() bool, message string) {
	deadline := time.Now().Add(timeout)
	interval := 100 * time.Millisecond

	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(interval)
	}

	t.Fatal(message)
}

// StreamReader helps read streaming responses
type StreamReader struct {
	stream interface {
		Recv() (interface{}, error)
	}
	responses []interface{}
}

// NewStreamReader creates a new stream reader
func NewStreamReader(stream interface{}) *StreamReader {
	return &StreamReader{
		stream:    stream.(interface{ Recv() (interface{}, error) }),
		responses: make([]interface{}, 0),
	}
}

// ReadAll reads all responses from the stream
func (r *StreamReader) ReadAll(t *testing.T) []interface{} {
	for {
		resp, err := r.stream.Recv()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		r.responses = append(r.responses, resp)
	}
	return r.responses
}

// ReadN reads N responses from the stream
func (r *StreamReader) ReadN(t *testing.T, n int) []interface{} {
	for i := 0; i < n; i++ {
		resp, err := r.stream.Recv()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		r.responses = append(r.responses, resp)
	}
	return r.responses
}

// MockHealthChecker provides a mock health checker for tests
type MockHealthChecker struct {
	healthy bool
	message string
}

// NewMockHealthChecker creates a new mock health checker
func NewMockHealthChecker(healthy bool) *MockHealthChecker {
	return &MockHealthChecker{
		healthy: healthy,
		message: "mock health check",
	}
}

// Check performs a health check
func (m *MockHealthChecker) Check(ctx context.Context) error {
	if !m.healthy {
		return fmt.Errorf("health check failed: %s", m.message)
	}
	return nil
}

// SetHealthy sets the health status
func (m *MockHealthChecker) SetHealthy(healthy bool) {
	m.healthy = healthy
}

// TestLogger provides a test logger
type TestLogger struct {
	t *testing.T
}

// NewTestLogger creates a new test logger
func NewTestLogger(t *testing.T) *TestLogger {
	return &TestLogger{t: t}
}

// Info logs an info message
func (l *TestLogger) Info(msg string, args ...interface{}) {
	l.t.Logf("[INFO] "+msg, args...)
}

// Error logs an error message
func (l *TestLogger) Error(msg string, args ...interface{}) {
	l.t.Logf("[ERROR] "+msg, args...)
}

// Debug logs a debug message
func (l *TestLogger) Debug(msg string, args ...interface{}) {
	if testing.Verbose() {
		l.t.Logf("[DEBUG] "+msg, args...)
	}
}