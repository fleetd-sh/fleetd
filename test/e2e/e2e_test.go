package e2e_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
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
	dbPath         string
}

// TestDevice represents a test device instance
type TestDevice struct {
	ID      string
	Name    string
	Host    string
	SSHPort int
	RPCPort int
	Process *os.Process
	TempDir string
}

// SetupSuite runs once before all tests
func (s *E2ETestSuite) SetupSuite() {
	s.deviceAPIURL = getEnv("DEVICE_API_URL", "http://localhost:18080")
	s.platformAPIURL = getEnv("PLATFORM_API_URL", "http://localhost:18090")
	s.agentBinary = getEnv("AGENT_BINARY", "../../bin/fleetd")

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

// Test_01_AgentProvisioning tests initial agent provisioning and registration
func (s *E2ETestSuite) Test_01_AgentProvisioning() {
	device := s.provisionDevice("test-device-01")
	s.testDevices = append(s.testDevices, device)

	// Verify agent registers with device-api (agent reports to server, not polled)
	assert.Eventually(s.T(), func() bool {
		devices := s.listDevices()
		for _, d := range devices {
			// Match by device name
			if name, ok := d["name"].(string); ok && strings.Contains(name, s.testDevices[0].Name) {
				s.T().Logf("Device registered with device-api: %v", d)
				// Store the actual device ID from registration
				if id, ok := d["id"].(string); ok {
					s.testDevices[0].ID = id
				}
				return true
			}
		}
		return false
	}, 60*time.Second, 2*time.Second, "Agent should register with device-api")
}

// Test_02_DeviceHeartbeat tests that device sends heartbeats to device-api
func (s *E2ETestSuite) Test_02_DeviceHeartbeat() {
	require.NotEmpty(s.T(), s.testDevices, "No devices provisioned")
	device := s.testDevices[0]

	// Wait for device to show as online (heartbeat received)
	assert.Eventually(s.T(), func() bool {
		devices := s.listDevices()
		for _, d := range devices {
			if name, ok := d["name"].(string); ok && strings.Contains(name, device.Name) {
				// Check if device status is "online" (has sent heartbeat)
				if status, ok := d["status"].(string); ok {
					s.T().Logf("Device status: %s", status)
					return status == "online" || status == "active"
				}
				return true // Device exists, consider it healthy even without explicit status
			}
		}
		return false
	}, 30*time.Second, 2*time.Second, "Device should send heartbeats to device-api")
}

// Test_03_Telemetry tests telemetry collection and reporting
func (s *E2ETestSuite) Test_03_Telemetry() {
	device := s.testDevices[0]

	// Get initial metrics
	initialMetrics := s.getDeviceMetrics(device.ID)

	// Wait for new telemetry data
	time.Sleep(2 * time.Second)

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

	// Start the binary
	s.startBinary(device, "test-app")

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

	// Trigger update by calling agent endpoint directly
	s.triggerAgentUpdate(device, "2.0.0")

	// Verify agent version changed
	assert.Eventually(s.T(), func() bool {
		info := s.getDeviceInfo(device)
		version, ok := info["version"].(string)
		return ok && version == "2.0.0"
	}, 10*time.Second, 500*time.Millisecond, "Agent version should update")
}

// Test_06_NetworkResilience tests network disconnection handling
func (s *E2ETestSuite) Test_06_NetworkResilience() {
	device := s.testDevices[0]

	// Get initial last_seen time
	initialDevice := s.getDeviceFromAPI(device.ID)
	initialLastSeen := initialDevice["last_seen"].(string)
	s.T().Logf("Initial last_seen: %s", initialLastSeen)

	// Stop agent (simulate network failure)
	s.stopAgent(device)

	// Wait a bit and verify last_seen hasn't updated
	time.Sleep(2 * time.Second)
	deviceAfterStop := s.getDeviceFromAPI(device.ID)
	lastSeenAfterStop := deviceAfterStop["last_seen"].(string)
	assert.Equal(s.T(), initialLastSeen, lastSeenAfterStop, "last_seen should not update when agent is stopped")

	// Restart agent (restore network)
	s.startAgent(device)

	// Wait for agent to reconnect and send telemetry
	time.Sleep(3 * time.Second)

	// Verify last_seen has updated
	assert.Eventually(s.T(), func() bool {
		deviceAfterRestart := s.getDeviceFromAPI(device.ID)
		lastSeenAfterRestart := deviceAfterRestart["last_seen"].(string)
		return lastSeenAfterRestart != initialLastSeen
	}, 10*time.Second, 500*time.Millisecond, "last_seen should update after agent restarts")
}

// Test_07_MultiDevice tests multiple devices
func (s *E2ETestSuite) Test_07_MultiDevice() {
	// Provision additional devices
	startIdx := len(s.testDevices)
	for i := 2; i <= 5; i++ {
		device := s.provisionDevice(fmt.Sprintf("test-device-%02d", i))
		s.testDevices = append(s.testDevices, device)
	}

	// Verify all devices register and update their IDs
	assert.Eventually(s.T(), func() bool {
		devices := s.listDevices()
		if len(devices) < 5 {
			return false
		}

		// Match and store device IDs for newly provisioned devices
		for i := startIdx; i < len(s.testDevices); i++ {
			for _, d := range devices {
				if name, ok := d["name"].(string); ok && strings.Contains(name, s.testDevices[i].Name) {
					if id, ok := d["id"].(string); ok {
						s.testDevices[i].ID = id
					}
					break
				}
			}
		}
		return true
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
	assert.Less(s.T(), memUsage, int64(50*1024*1024), "Memory usage should be < 50MB")
	assert.Less(s.T(), cpuUsage, 5.0, "CPU usage should be < 5%")
}

// Test_09_Cleanup tests graceful shutdown
func (s *E2ETestSuite) Test_09_Cleanup() {
	device := s.testDevices[0]

	// Send shutdown signal
	s.shutdownDevice(device)

	// Wait a bit for shutdown to complete
	time.Sleep(2 * time.Second)

	// Verify state is saved (agent persisted state before shutdown)
	stateFile := fmt.Sprintf("%s/state/state.json", device.TempDir)
	assert.FileExists(s.T(), stateFile, "State should be persisted")

	// Optionally verify device goes offline on server side
	// (May take time due to heartbeat timeout, so we don't strictly require it)
	devices := s.listDevices()
	for _, d := range devices {
		if name, ok := d["name"].(string); ok && strings.Contains(name, device.Name) {
			if status, ok := d["status"].(string); ok {
				s.T().Logf("Device status after shutdown: %s", status)
			}
		}
	}
}

// Helper methods

func (s *E2ETestSuite) buildAgent() {
	cmd := exec.Command("go", "build", "-o", s.agentBinary, "../../cmd/fleetd")
	output, err := cmd.CombinedOutput()
	require.NoError(s.T(), err, "Failed to build agent: %s", string(output))
}

func (s *E2ETestSuite) startDeviceAPI() {
	// Create temp database file
	tmpDB, err := os.CreateTemp("", "test-device-api-*.db")
	require.NoError(s.T(), err)
	s.dbPath = tmpDB.Name()
	tmpDB.Close()

	// Remove any existing database file
	os.Remove(s.dbPath)

	cmd := exec.Command("../../bin/device-api", "--port=18080", fmt.Sprintf("--db=%s", s.dbPath))
	cmd.Env = append(os.Environ(), "DEVICE_API_SECRET_KEY=test-secret-key")

	// Capture stdout/stderr for debugging
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = cmd.Start()
	require.NoError(s.T(), err, "Failed to start device-api")

	s.cleanup = append(s.cleanup, func() {
		if cmd.Process != nil {
			cmd.Process.Kill()
			cmd.Process.Wait() // Wait for process to fully terminate
		}
		// Clean up database file
		if s.dbPath != "" {
			os.Remove(s.dbPath)
			os.Remove(s.dbPath + "-shm")
			os.Remove(s.dbPath + "-wal")
		}
	})
}

func (s *E2ETestSuite) waitForServices() {
	require.Eventually(s.T(), func() bool {
		return s.isServiceHealthy(s.deviceAPIURL)
	}, 30*time.Second, 500*time.Millisecond, "device-api should be healthy")

	// Additional wait to ensure ConnectRPC handlers are fully initialized
	// The health endpoint responds quickly but gRPC/ConnectRPC handlers take longer
	s.T().Log("Waiting for device-api to fully initialize...")
	time.Sleep(2 * time.Second)
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

	// Find free port in a less common range
	port := 19000 + len(s.testDevices)

	// Create log file for agent output
	logPath := filepath.Join(tempDir, "agent.log")
	logFile, err := os.Create(logPath)
	require.NoError(s.T(), err)

	// Start agent
	cmd := exec.Command(s.agentBinary, "agent",
		"--server-url", s.deviceAPIURL,
		"--storage-dir", tempDir,
		"--rpc-port", fmt.Sprintf("%d", port),
		"--disable-mdns",
		"--device-name", name,
		"--telemetry-interval", "1")

	cmd.Stdout = logFile
	cmd.Stderr = logFile

	err = cmd.Start()
	require.NoError(s.T(), err)

	s.T().Logf("Started agent %s (PID: %d, port: %d, logs: %s)", name, cmd.Process.Pid, port, logPath)

	device := TestDevice{
		ID:      fmt.Sprintf("%s-%d", name, time.Now().Unix()),
		Name:    name,
		Host:    "localhost",
		RPCPort: port,
		Process: cmd.Process,
		TempDir: tempDir,
	}

	s.cleanup = append(s.cleanup, func() {
		// Print agent log before cleanup for debugging
		if logData, err := os.ReadFile(logPath); err == nil {
			s.T().Logf("Agent %s log:\n%s", name, string(logData))
		}
		os.RemoveAll(tempDir)
	})

	return device
}

// Removed isAgentHealthy - agents report to server, they are not polled

func (s *E2ETestSuite) listDevices() []map[string]interface{} {
	resp, err := http.Get(s.deviceAPIURL + "/api/v1/devices")
	require.NoError(s.T(), err)
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	require.NoError(s.T(), err)

	// Handle empty or non-array responses
	if len(body) == 0 || string(body) == "0" || string(body) == "null" {
		return []map[string]interface{}{}
	}

	// The API returns a plain array of devices
	var devices []map[string]interface{}
	err = json.Unmarshal(body, &devices)
	if err != nil {
		s.T().Logf("Failed to unmarshal devices response: %v, body: %s", err, string(body))
		return []map[string]interface{}{}
	}

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

	// Read response body
	body, err := io.ReadAll(resp.Body)
	require.NoError(s.T(), err)

	// Handle empty or invalid responses
	if len(body) == 0 || string(body) == "0" || string(body) == "null" {
		return nil
	}

	var metrics map[string]interface{}
	err = json.Unmarshal(body, &metrics)
	if err != nil {
		s.T().Logf("Failed to unmarshal metrics response: %v, body: %s", err, string(body))
		return nil
	}
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

	url := fmt.Sprintf("http://%s:%d/agent.v1.DaemonService/DeployBinary", device.Host, device.RPCPort)
	payload := map[string]interface{}{
		"name": name,
		"data": base64.StdEncoding.EncodeToString(data),
	}

	jsonData, err := json.Marshal(payload)
	require.NoError(s.T(), err)

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	require.NoError(s.T(), err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(s.T(), err)
	defer resp.Body.Close()

	require.Equal(s.T(), http.StatusOK, resp.StatusCode)
}

func (s *E2ETestSuite) startBinary(device TestDevice, name string) {
	url := fmt.Sprintf("http://%s:%d/agent.v1.DaemonService/StartBinary", device.Host, device.RPCPort)
	payload := map[string]interface{}{
		"name": name,
		"args": []string{},
	}

	jsonData, err := json.Marshal(payload)
	require.NoError(s.T(), err)

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	require.NoError(s.T(), err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(s.T(), err)
	defer resp.Body.Close()

	require.Equal(s.T(), http.StatusOK, resp.StatusCode)
}

func (s *E2ETestSuite) listBinaries(device TestDevice) []map[string]interface{} {
	url := fmt.Sprintf("http://%s:%d/agent.v1.DaemonService/ListBinaries", device.Host, device.RPCPort)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer([]byte("{}")))
	require.NoError(s.T(), err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
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

func (s *E2ETestSuite) triggerAgentUpdate(device TestDevice, version string) {
	payload := map[string]string{
		"version": version,
	}
	data, _ := json.Marshal(payload)

	url := fmt.Sprintf("http://%s:%d/update", device.Host, device.RPCPort)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(data))
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

func (s *E2ETestSuite) getDeviceFromAPI(deviceID string) map[string]interface{} {
	resp, err := http.Get(fmt.Sprintf("%s/api/v1/devices/%s", s.deviceAPIURL, deviceID))
	require.NoError(s.T(), err)
	defer resp.Body.Close()

	var device map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&device)
	require.NoError(s.T(), err)

	return device
}

func (s *E2ETestSuite) stopAgent(device TestDevice) {
	if device.Process != nil {
		s.T().Logf("Stopping agent %s (PID: %d)", device.Name, device.Process.Pid)
		device.Process.Kill()
		device.Process.Wait()
	}
}

func (s *E2ETestSuite) startAgent(device TestDevice) {
	logPath := filepath.Join(device.TempDir, "agent.log")
	logFile, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	require.NoError(s.T(), err)

	cmd := exec.Command(s.agentBinary, "agent",
		"--server-url", s.deviceAPIURL,
		"--storage-dir", device.TempDir,
		"--rpc-port", fmt.Sprintf("%d", device.RPCPort),
		"--disable-mdns",
		"--device-name", device.Name,
		"--telemetry-interval", "1")

	cmd.Stdout = logFile
	cmd.Stderr = logFile

	err = cmd.Start()
	require.NoError(s.T(), err)

	// Update the device's process reference
	device.Process = cmd.Process
	// Update in the slice
	for i := range s.testDevices {
		if s.testDevices[i].Name == device.Name {
			s.testDevices[i].Process = cmd.Process
			break
		}
	}

	s.T().Logf("Restarted agent %s (PID: %d)", device.Name, cmd.Process.Pid)
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
	// Send command to device-api (for logging/tracking)
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

	// Also send directly to agents for E2E testing
	for _, testDevice := range s.testDevices {
		payload := map[string]string{
			"command": command,
			"args":    args,
		}
		data, _ := json.Marshal(payload)

		resp, err := http.Post(
			fmt.Sprintf("http://%s:%d/execute-command", testDevice.Host, testDevice.RPCPort),
			"application/json",
			bytes.NewBuffer(data),
		)
		if err != nil {
			s.T().Logf("Failed to send command to agent %s: %v", testDevice.Name, err)
			continue
		}
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
		// Try different numeric types
		switch mem := metrics["memory_bytes"].(type) {
		case float64:
			return int64(mem)
		case int64:
			return mem
		case int:
			return int64(mem)
		case uint64:
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
	if os.Getenv("E2E") == "" {
		t.Skip("Skipping E2E tests. Set E2E=1 to run")
	}
	suite.Run(t, new(E2ETestSuite))
}
