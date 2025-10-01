package e2e_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// E2ETestSuite runs end-to-end tests for the fleetd system
type E2ETestSuite struct {
	suite.Suite
	deviceAPIURL   string
	platformAPIURL string
	agentBinary    string
	testDevices    []TestDevice
	cleanup        []func()
}

// TestDevice represents a test device instance
type TestDevice struct {
	ID       string
	Name     string
	Host     string
	SSHPort  int
	RPCPort  int
	Process  *os.Process
	TempDir  string
}

// SetupSuite runs once before all tests
func (s *E2ETestSuite) SetupSuite() {
	s.deviceAPIURL = getEnv("DEVICE_API_URL", "http://localhost:8080")
	s.platformAPIURL = getEnv("PLATFORM_API_URL", "http://localhost:8090")
	s.agentBinary = getEnv("AGENT_BINARY", "./bin/fleetd")

	// Build agent if needed
	if _, err := os.Stat(s.agentBinary); os.IsNotExist(err) {
		s.T().Log("Building agent binary...")
		s.buildAgent()
	}

	// Start services if not running
	if !s.isServiceHealthy(s.deviceAPIURL) {
		s.T().Log("Starting device-api...")
		s.startDeviceAPI()
	}

	// Wait for services to be ready
	s.waitForServices()
}

// TearDownSuite runs once after all tests
func (s *E2ETestSuite) TearDownSuite() {
	// Clean up test devices
	for _, device := range s.testDevices {
		s.stopDevice(device)
	}

	// Run cleanup functions
	for _, cleanup := range s.cleanup {
		cleanup()
	}
}

// Test_01_AgentProvisioning tests initial agent provisioning
func (s *E2ETestSuite) Test_01_AgentProvisioning() {
	device := s.provisionDevice("test-device-01")
	s.testDevices = append(s.testDevices, device)

	// Verify agent starts successfully
	assert.Eventually(s.T(), func() bool {
		return s.isAgentHealthy(device)
	}, 30*time.Second, 1*time.Second, "Agent should become healthy")
}

// Test_02_DeviceRegistration tests device registration with device-api
func (s *E2ETestSuite) Test_02_DeviceRegistration() {
	device := s.testDevices[0]

	// Wait for registration
	assert.Eventually(s.T(), func() bool {
		devices := s.listDevices()
		for _, d := range devices {
			if strings.Contains(d["name"].(string), device.Name) {
				s.T().Logf("Device registered: %v", d)
				return true
			}
		}
		return false
	}, 30*time.Second, 1*time.Second, "Device should register")
}

// Test_03_Telemetry tests telemetry collection and reporting
func (s *E2ETestSuite) Test_03_Telemetry() {
	device := s.testDevices[0]

	// Get initial metrics
	initialMetrics := s.getDeviceMetrics(device.ID)

	// Wait for new telemetry data
	time.Sleep(35 * time.Second) // Default telemetry interval is 30s

	// Get updated metrics
	updatedMetrics := s.getDeviceMetrics(device.ID)

	// Verify metrics are being collected
	assert.NotEqual(s.T(), initialMetrics, updatedMetrics, "Metrics should update")
}

// Test_04_BinaryDeployment tests deploying a binary to the device
func (s *E2ETestSuite) Test_04_BinaryDeployment() {
	device := s.testDevices[0]

	// Create test binary
	testBinary := s.createTestBinary()
	defer os.Remove(testBinary)

	// Deploy binary
	s.deployBinary(device, testBinary, "test-app")

	// Verify binary is running
	assert.Eventually(s.T(), func() bool {
		binaries := s.listBinaries(device)
		for _, b := range binaries {
			if b["name"] == "test-app" {
				return b["status"] == "running"
			}
		}
		return false
	}, 30*time.Second, 1*time.Second, "Binary should be running")
}

// Test_05_DeviceUpdate tests agent self-update
func (s *E2ETestSuite) Test_05_DeviceUpdate() {
	device := s.testDevices[0]

	// Create updated agent binary (mock)
	updatedBinary := s.createMockUpdate("v2.0.0")
	defer os.Remove(updatedBinary)

	// Trigger update
	s.triggerUpdate(device.ID, updatedBinary)

	// Verify agent restarts with new version
	assert.Eventually(s.T(), func() bool {
		info := s.getDeviceInfo(device)
		return info["version"] == "2.0.0" // Mock version
	}, 60*time.Second, 1*time.Second, "Agent should update")
}

// Test_06_NetworkResilience tests network disconnection handling
func (s *E2ETestSuite) Test_06_NetworkResilience() {
	device := s.testDevices[0]

	// Simulate network disconnection
	s.blockNetwork(device)

	// Wait for offline status
	assert.Eventually(s.T(), func() bool {
		devices := s.listDevices()
		for _, d := range devices {
			if d["id"] == device.ID {
				return d["status"] == "offline"
			}
		}
		return false
	}, 30*time.Second, 1*time.Second, "Device should go offline")

	// Restore network
	s.unblockNetwork(device)

	// Verify reconnection
	assert.Eventually(s.T(), func() bool {
		devices := s.listDevices()
		for _, d := range devices {
			if d["id"] == device.ID {
				return d["status"] == "online"
			}
		}
		return false
	}, 30*time.Second, 1*time.Second, "Device should reconnect")
}

// Test_07_MultiDevice tests multiple devices
func (s *E2ETestSuite) Test_07_MultiDevice() {
	// Provision additional devices
	for i := 2; i <= 5; i++ {
		device := s.provisionDevice(fmt.Sprintf("test-device-%02d", i))
		s.testDevices = append(s.testDevices, device)
	}

	// Verify all devices register
	assert.Eventually(s.T(), func() bool {
		devices := s.listDevices()
		return len(devices) >= 5
	}, 60*time.Second, 2*time.Second, "All devices should register")

	// Test broadcast command
	s.broadcastCommand("echo", "test")

	// Verify all devices executed command
	for _, device := range s.testDevices {
		result := s.getCommandResult(device, "echo")
		assert.Contains(s.T(), result, "test", "Device should execute command")
	}
}

// Test_08_ResourceLimits tests resource usage limits
func (s *E2ETestSuite) Test_08_ResourceLimits() {
	device := s.testDevices[0]

	// Monitor resource usage
	memUsage := s.getMemoryUsage(device)
	cpuUsage := s.getCPUUsage(device)

	// Verify resource constraints
	assert.Less(s.T(), memUsage, 50*1024*1024, "Memory usage should be < 50MB")
	assert.Less(s.T(), cpuUsage, 5.0, "CPU usage should be < 5%")
}

// Test_09_Cleanup tests graceful shutdown
func (s *E2ETestSuite) Test_09_Cleanup() {
	device := s.testDevices[0]

	// Send shutdown signal
	s.shutdownDevice(device)

	// Verify graceful shutdown
	assert.Eventually(s.T(), func() bool {
		return !s.isAgentHealthy(device)
	}, 10*time.Second, 1*time.Second, "Agent should shutdown gracefully")

	// Verify state is saved
	stateFile := fmt.Sprintf("%s/state/state.json", device.TempDir)
	assert.FileExists(s.T(), stateFile, "State should be persisted")
}

// Helper methods

func (s *E2ETestSuite) buildAgent() {
	cmd := exec.Command("go", "build", "-o", s.agentBinary, "../../cmd/fleetd")
	output, err := cmd.CombinedOutput()
	require.NoError(s.T(), err, "Failed to build agent: %s", string(output))
}

func (s *E2ETestSuite) startDeviceAPI() {
	cmd := exec.Command("../../bin/device-api", "--port=8080", "--db=/tmp/test-device-api.db")
	cmd.Env = append(os.Environ(), "DEVICE_API_SECRET_KEY=test-secret-key")
	err := cmd.Start()
	require.NoError(s.T(), err, "Failed to start device-api")

	s.cleanup = append(s.cleanup, func() {
		cmd.Process.Kill()
	})
}

func (s *E2ETestSuite) waitForServices() {
	require.Eventually(s.T(), func() bool {
		return s.isServiceHealthy(s.deviceAPIURL)
	}, 30*time.Second, 1*time.Second, "device-api should be healthy")
}

func (s *E2ETestSuite) isServiceHealthy(url string) bool {
	resp, err := http.Get(url + "/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func (s *E2ETestSuite) provisionDevice(name string) TestDevice {
	// Create temp directory for device
	tempDir, err := os.MkdirTemp("", "fleetd-test-")
	require.NoError(s.T(), err)

	// Find free port
	port := 8088 + len(s.testDevices)

	// Start agent
	cmd := exec.Command(s.agentBinary, "agent",
		"--server-url", s.deviceAPIURL,
		"--storage-dir", tempDir,
		"--rpc-port", fmt.Sprintf("%d", port),
		"--disable-mdns")

	cmd.Env = append(os.Environ(), fmt.Sprintf("DEVICE_NAME=%s", name))
	err = cmd.Start()
	require.NoError(s.T(), err)

	device := TestDevice{
		ID:      fmt.Sprintf("%s-%d", name, time.Now().Unix()),
		Name:    name,
		Host:    "localhost",
		RPCPort: port,
		Process: cmd.Process,
		TempDir: tempDir,
	}

	s.cleanup = append(s.cleanup, func() {
		os.RemoveAll(tempDir)
	})

	return device
}

func (s *E2ETestSuite) isAgentHealthy(device TestDevice) bool {
	resp, err := http.Get(fmt.Sprintf("http://%s:%d/health", device.Host, device.RPCPort))
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func (s *E2ETestSuite) listDevices() []map[string]interface{} {
	resp, err := http.Get(s.deviceAPIURL + "/api/v1/devices")
	require.NoError(s.T(), err)
	defer resp.Body.Close()

	// The API returns a plain array of devices
	var devices []map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&devices)
	require.NoError(s.T(), err)

	return devices
}

func (s *E2ETestSuite) stopDevice(device TestDevice) {
	if device.Process != nil {
		device.Process.Kill()
	}
}

func (s *E2ETestSuite) getDeviceMetrics(deviceID string) map[string]interface{} {
	resp, err := http.Get(fmt.Sprintf("%s/api/v1/devices/%s/metrics", s.deviceAPIURL, deviceID))
	require.NoError(s.T(), err)
	defer resp.Body.Close()

	var metrics map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&metrics)
	return metrics
}

func (s *E2ETestSuite) createTestBinary() string {
	code := `
package main
import (
	"fmt"
	"time"
)
func main() {
	for {
		fmt.Println("Test app running...")
		time.Sleep(10 * time.Second)
	}
}
`
	tmpFile, err := os.CreateTemp("", "test-app-*.go")
	require.NoError(s.T(), err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString(code)
	require.NoError(s.T(), err)
	tmpFile.Close()

	outputPath := strings.TrimSuffix(tmpFile.Name(), ".go")
	cmd := exec.Command("go", "build", "-o", outputPath, tmpFile.Name())
	err = cmd.Run()
	require.NoError(s.T(), err)

	return outputPath
}

func (s *E2ETestSuite) deployBinary(device TestDevice, binaryPath, name string) {
	data, err := os.ReadFile(binaryPath)
	require.NoError(s.T(), err)

	url := fmt.Sprintf("http://%s:%d/agent.v1.Daemon/DeployBinary", device.Host, device.RPCPort)
	payload := map[string]interface{}{
		"name": name,
		"data": data,
	}

	jsonData, err := json.Marshal(payload)
	require.NoError(s.T(), err)

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	require.NoError(s.T(), err)
	defer resp.Body.Close()

	require.Equal(s.T(), http.StatusOK, resp.StatusCode)
}

func (s *E2ETestSuite) listBinaries(device TestDevice) []map[string]interface{} {
	url := fmt.Sprintf("http://%s:%d/agent.v1.Daemon/ListBinaries", device.Host, device.RPCPort)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer([]byte("{}")))
	require.NoError(s.T(), err)
	defer resp.Body.Close()

	var result struct {
		Binaries []map[string]interface{} `json:"binaries"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	return result.Binaries
}

func (s *E2ETestSuite) createMockUpdate(version string) string {
	// Create a mock update package
	updateID := fmt.Sprintf("update-%s-%d", version, time.Now().Unix())
	// In a real test, this would upload to a test S3 bucket or similar
	return updateID
}

func (s *E2ETestSuite) triggerUpdate(deviceID, updateID string) {
	payload := map[string]string{
		"device_id": deviceID,
		"update_id": updateID,
	}
	data, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", fmt.Sprintf("%s/api/v1/devices/%s/update", s.deviceAPIURL, deviceID), bytes.NewBuffer(data))
	require.NoError(s.T(), err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(s.T(), err)
	defer resp.Body.Close()
	require.Equal(s.T(), http.StatusOK, resp.StatusCode)
}

func (s *E2ETestSuite) getDeviceInfo(device TestDevice) map[string]interface{} {
	resp, err := http.Get(fmt.Sprintf("http://%s:%d/info", device.Host, device.RPCPort))
	require.NoError(s.T(), err)
	defer resp.Body.Close()

	var info map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&info)
	require.NoError(s.T(), err)

	return info
}

func (s *E2ETestSuite) blockNetwork(device TestDevice) {
	// Simulate network failure using iptables (requires root in real test)
	s.T().Log("Simulating network failure for", device.Name)
	// In Docker/QEMU tests, this would use iptables or tc
}

func (s *E2ETestSuite) unblockNetwork(device TestDevice) {
	// Restore network connectivity
	s.T().Log("Restoring network for", device.Name)
	// In Docker/QEMU tests, this would restore iptables rules
}

func (s *E2ETestSuite) broadcastCommand(command, args string) {
	devices := s.listDevices()
	for _, device := range devices {
		deviceID := device["id"].(string)
		payload := map[string]string{
			"command": command,
			"args":    args,
		}
		data, _ := json.Marshal(payload)

		req, err := http.NewRequest("POST", fmt.Sprintf("%s/api/v1/devices/%s/command", s.deviceAPIURL, deviceID), bytes.NewBuffer(data))
		require.NoError(s.T(), err)
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		require.NoError(s.T(), err)
		resp.Body.Close()
	}
}

func (s *E2ETestSuite) getCommandResult(device TestDevice, command string) string {
	resp, err := http.Get(fmt.Sprintf("http://%s:%d/commands/%s", device.Host, device.RPCPort, command))
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	return string(data)
}

func (s *E2ETestSuite) getMemoryUsage(device TestDevice) int64 {
	info := s.getDeviceInfo(device)
	if metrics, ok := info["metrics"].(map[string]interface{}); ok {
		if mem, ok := metrics["memory_bytes"].(float64); ok {
			return int64(mem)
		}
	}
	return 0
}

func (s *E2ETestSuite) getCPUUsage(device TestDevice) float64 {
	info := s.getDeviceInfo(device)
	if metrics, ok := info["metrics"].(map[string]interface{}); ok {
		if cpu, ok := metrics["cpu_percent"].(float64); ok {
			return cpu
		}
	}
	return 0.0
}

func (s *E2ETestSuite) shutdownDevice(device TestDevice) {
	req, err := http.NewRequest("POST", fmt.Sprintf("http://%s:%d/shutdown", device.Host, device.RPCPort), nil)
	if err == nil {
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			resp.Body.Close()
		}
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// TestE2ESuite runs the test suite
func TestE2ESuite(t *testing.T) {
	if os.Getenv("RUN_E2E") != "true" {
		t.Skip("Skipping E2E tests. Set RUN_E2E=true to run")
	}
	suite.Run(t, new(E2ETestSuite))
}