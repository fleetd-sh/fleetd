package provision

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCoreProvisioner_GetCoreFiles(t *testing.T) {
	config := &Config{
		DeviceID:   "test-device-123",
		DeviceName: "test-device",
		DeviceType: DeviceTypeRaspberryPi,
		Network: NetworkConfig{
			WiFiSSID: "TestNetwork",
			WiFiPass: "TestPassword",
		},
		Fleet: FleetConfig{
			ServerURL: "https://fleet.example.com",
			Token:     "test-token",
		},
		Security: SecurityConfig{
			EnableSSH: true,
			SSHKey:    "ssh-rsa AAAAB3NzaC1yc2E... test@example.com",
		},
	}

	provisioner := NewCoreProvisioner(config)
	files := provisioner.GetCoreFiles()

	// Check that all expected files are present
	expectedFiles := []string{
		"/boot/fleetd/config.yaml",
		"/boot/fleetd/fleetd.service",
		"/boot/wifi-config.txt",
		"/boot/ssh/authorized_keys",
		"/boot/fleetd-setup.sh",
	}

	for _, path := range expectedFiles {
		if _, ok := files[path]; !ok {
			t.Errorf("Expected file %s not found", path)
		}
	}

	// Check fleetd config content
	fleetdConfig := string(files["/boot/fleetd/config.yaml"])
	if !strings.Contains(fleetdConfig, "id: test-device-123") {
		t.Error("fleetd config missing device ID")
	}
	if !strings.Contains(fleetdConfig, "name: test-device") {
		t.Error("fleetd config missing device name")
	}
	if !strings.Contains(fleetdConfig, "url: https://fleet.example.com") {
		t.Error("fleetd config missing server URL")
	}

	// Check WiFi config
	wifiConfig := string(files["/boot/wifi-config.txt"])
	if !strings.Contains(wifiConfig, "SSID=TestNetwork") {
		t.Error("WiFi config missing SSID")
	}
	if !strings.Contains(wifiConfig, "PASSWORD=TestPassword") {
		t.Error("WiFi config missing password")
	}

	// Check SSH key
	sshKey := string(files["/boot/ssh/authorized_keys"])
	if !strings.Contains(sshKey, "ssh-rsa AAAAB3NzaC1yc2E") {
		t.Error("SSH key not properly stored")
	}

	// Check systemd service
	service := string(files["/boot/fleetd/fleetd.service"])
	if !strings.Contains(service, "Description=FleetD Agent") {
		t.Error("Service file missing description")
	}
	if !strings.Contains(service, "FLEETD_DEVICE_ID=test-device-123") {
		t.Error("Service file missing device ID environment variable")
	}
}

func TestCoreProvisioner_GetCoreFiles_NoWiFi(t *testing.T) {
	config := &Config{
		DeviceID:   "test-device-123",
		DeviceName: "test-device",
		DeviceType: DeviceTypeRaspberryPi,
		Network:    NetworkConfig{}, // No WiFi
	}

	provisioner := NewCoreProvisioner(config)
	files := provisioner.GetCoreFiles()

	// WiFi config should not be present
	if _, ok := files["/boot/wifi-config.txt"]; ok {
		t.Error("WiFi config should not be present when WiFi not configured")
	}
}

func TestCoreProvisioner_GetCoreFiles_NoSSH(t *testing.T) {
	config := &Config{
		DeviceID:   "test-device-123",
		DeviceName: "test-device",
		DeviceType: DeviceTypeRaspberryPi,
		Security:   SecurityConfig{EnableSSH: false},
	}

	provisioner := NewCoreProvisioner(config)
	files := provisioner.GetCoreFiles()

	// SSH key should not be present
	if _, ok := files["/boot/ssh/authorized_keys"]; ok {
		t.Error("SSH authorized_keys should not be present when SSH disabled")
	}
}

func TestCoreProvisioner_GetCoreFiles_MDNSDiscovery(t *testing.T) {
	config := &Config{
		DeviceID:   "test-device-123",
		DeviceName: "test-device",
		DeviceType: DeviceTypeRaspberryPi,
		Fleet:      FleetConfig{}, // No server URL - will use mDNS
	}

	provisioner := NewCoreProvisioner(config)
	files := provisioner.GetCoreFiles()

	fleetdConfig := string(files["/boot/fleetd/config.yaml"])
	if !strings.Contains(fleetdConfig, "discovery: mdns") {
		t.Error("fleetd config should use mDNS discovery when no server URL")
	}
	if strings.Contains(fleetdConfig, "url:") && !strings.Contains(fleetdConfig, "# Server") {
		t.Error("fleetd config should not have server URL when using mDNS")
	}
}

func TestCoreProvisioner_Provision(t *testing.T) {
	config := &Config{
		DeviceID:   "test-device-123",
		DeviceName: "test-device",
		DeviceType: DeviceTypeRaspberryPi,
	}

	provisioner := NewCoreProvisioner(config)

	// Register a test hook to verify it's called
	testHook := NewTestHook("test", 100)
	testHook.files["/test/file.txt"] = []byte("test content")
	provisioner.RegisterHook(testHook)

	ctx := context.Background()
	err := provisioner.Provision(ctx)
	if err != nil {
		t.Errorf("Provision failed: %v", err)
	}

	// Verify hooks were called
	if !testHook.preProvisionCalled {
		t.Error("PreProvision hook not called")
	}
	if !testHook.modifyConfigCalled {
		t.Error("ModifyConfig hook not called")
	}
	if !testHook.postProvisionCalled {
		t.Error("PostProvision hook not called")
	}
	if !testHook.addFilesCalled {
		t.Error("AddFiles hook not called")
	}
}

func TestCoreProvisioner_Provision_HookError(t *testing.T) {
	config := &Config{
		DeviceID:   "test-device-123",
		DeviceName: "test-device",
		DeviceType: DeviceTypeRaspberryPi,
	}

	provisioner := NewCoreProvisioner(config)

	// Register a hook that errors
	testHook := NewTestHook("test", 100)
	testHook.preProvisionError = context.DeadlineExceeded
	provisioner.RegisterHook(testHook)

	ctx := context.Background()
	err := provisioner.Provision(ctx)
	if err == nil {
		t.Error("Expected error from Provision when hook fails")
	}
	if !strings.Contains(err.Error(), "pre-provision failed") {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestCoreProvisioner_LoadPlugins(t *testing.T) {
	config := &Config{}
	provisioner := NewCoreProvisioner(config)

	// Should not error on non-existent directory
	err := provisioner.LoadPlugins("/tmp/non-existent")
	if err != nil {
		t.Errorf("LoadPlugins should not error on non-existent dir: %v", err)
	}
}

func TestVerifyProvisioning(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()

	// Create test config
	config := &Config{
		DeviceID:   "test-device-123",
		DeviceName: "test-device",
		DeviceType: DeviceTypeRaspberryPi,
		Network: NetworkConfig{
			WiFiSSID: "TestNetwork",
			WiFiPass: "TestPassword",
		},
	}

	provisioner := &CoreProvisioner{
		config: config,
	}

	// Test 1: No files written - should fail
	err := provisioner.verifyProvisioning(tempDir, "", nil)
	if err == nil {
		t.Error("Expected verification to fail when no files are present")
	}

	// Test 2: Write minimal Raspberry Pi OS files
	cmdlineFile := filepath.Join(tempDir, "cmdline.txt")
	if err := os.WriteFile(cmdlineFile, []byte("test cmdline config content that is long enough"), 0644); err != nil {
		t.Fatalf("Failed to write cmdline.txt: %v", err)
	}

	// Still missing fleetd binary - should fail
	err = provisioner.verifyProvisioning(tempDir, "", nil)
	if err == nil {
		t.Error("Expected verification to fail when fleetd binary is missing")
	}

	// Test 3: Add fleetd binary (simulate with a large file)
	fleetdFile := filepath.Join(tempDir, "fleetd")
	// Create a file larger than 1MB
	largeContent := make([]byte, 1024*1024+1)
	if err := os.WriteFile(fleetdFile, largeContent, 0755); err != nil {
		t.Fatalf("Failed to write fleetd: %v", err)
	}

	// Now verification should pass
	err = provisioner.verifyProvisioning(tempDir, "", nil)
	if err != nil {
		t.Errorf("Expected verification to pass, got error: %v", err)
	}

	// Test 4: Test with corrupted fleetd (too small)
	smallContent := []byte("too small")
	if err := os.WriteFile(fleetdFile, smallContent, 0755); err != nil {
		t.Fatalf("Failed to write small fleetd: %v", err)
	}

	err = provisioner.verifyProvisioning(tempDir, "", nil)
	if err == nil {
		t.Error("Expected verification to fail with corrupted fleetd binary")
	}
}

func TestVerifyGenericProvisioning(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()

	// Test config without WiFi
	config := &Config{
		DeviceID:   "test-device-123",
		DeviceName: "test-device",
		DeviceType: DeviceTypeRaspberryPi,
	}

	provisioner := &CoreProvisioner{
		config: config,
	}

	// Test 1: Missing files - should fail
	err := provisioner.verifyGenericProvisioning(tempDir)
	if err == nil {
		t.Error("Expected verification to fail when files are missing")
	}

	// Test 2: Add required files
	sshFile := filepath.Join(tempDir, "ssh")
	if err := os.WriteFile(sshFile, []byte(""), 0644); err != nil {
		t.Fatalf("Failed to write ssh file: %v", err)
	}

	userConfFile := filepath.Join(tempDir, "userconf.txt")
	if err := os.WriteFile(userConfFile, []byte("pi:encryptedpasswordhash"), 0644); err != nil {
		t.Fatalf("Failed to write userconf.txt: %v", err)
	}

	// Now verification should pass
	err = provisioner.verifyGenericProvisioning(tempDir)
	if err != nil {
		t.Errorf("Expected verification to pass, got error: %v", err)
	}

	// Test 3: With WiFi config
	config.Network.WiFiSSID = "TestNetwork"
	config.Network.WiFiPass = "TestPassword"

	// Should fail because WiFi config is missing
	err = provisioner.verifyGenericProvisioning(tempDir)
	if err == nil {
		t.Error("Expected verification to fail when WiFi config is missing")
	}

	// Add WiFi config
	wpaFile := filepath.Join(tempDir, "wpa_supplicant.conf")
	wpaContent := `ctrl_interface=DIR=/var/run/wpa_supplicant GROUP=netdev
update_config=1
country=US

network={
    ssid="TestNetwork"
    psk="TestPassword"
    key_mgmt=WPA-PSK
}`
	if err := os.WriteFile(wpaFile, []byte(wpaContent), 0644); err != nil {
		t.Fatalf("Failed to write wpa_supplicant.conf: %v", err)
	}

	// Now verification should pass
	err = provisioner.verifyGenericProvisioning(tempDir)
	if err != nil {
		t.Errorf("Expected verification to pass with WiFi config, got error: %v", err)
	}
}
