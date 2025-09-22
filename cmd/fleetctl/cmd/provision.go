package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"fleetd.sh/internal/provision"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

func newProvisionCmd() *cobra.Command {
	var (
		// Provision flags
		device         string
		name           string
		imageURL       string
		imageSHA256URL string
		osType         string
		wifiSSID       string
		wifiPass       string
		fleetServer    string
		sshKeyFile     string
		plugins        []string
		pluginOpts     []string
		pluginDir      string
		dryRun         bool
		configureOnly  bool
	)
	cmd := &cobra.Command{
		Use:   "provision",
		Short: "Provision edge devices with fleetd",
		Long: `Provision a device with the fleetd agent and optional plugins.

This command writes an OS image to an SD card or device, configures networking,
and sets up the fleetd agent for fleet management.`,
		Example: `  # Basic provisioning with Raspberry Pi OS Lite (default)
  fleet provision --device /dev/disk2 --wifi-ssid MyWiFi --wifi-pass secret

  # With custom image URL (e.g., Ubuntu Server for Raspberry Pi)
  fleet provision --device /dev/disk2 --wifi-ssid MyWiFi --wifi-pass secret \
    --image-url https://cdimage.ubuntu.com/releases/22.04/release/ubuntu-22.04.3-preinstalled-server-arm64+raspi.img.xz

  # With local image file
  fleet provision --device /dev/disk2 --wifi-ssid MyWiFi --wifi-pass secret \
    --image-url /path/to/raspios-lite-arm64.img.xz

  # With k3s plugin
  fleet provision --device /dev/disk2 --wifi-ssid MyWiFi --wifi-pass secret \
    --plugin k3s --plugin-opt k3s.role=server`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProvision(cmd, args, device, name, imageURL, imageSHA256URL, osType, wifiSSID, wifiPass, fleetServer, sshKeyFile, plugins, pluginOpts, pluginDir, dryRun, configureOnly)
		},
	}

	// Required flags
	cmd.Flags().StringVarP(&device, "device", "d", "", "Device path (e.g., /dev/disk2)")
	cmd.MarkFlagRequired("device")

	// Network flags (essential)
	cmd.Flags().StringVar(&wifiSSID, "wifi-ssid", "", "WiFi network name")
	cmd.Flags().StringVar(&wifiPass, "wifi-pass", "", "WiFi password")

	// Device configuration
	cmd.Flags().StringVarP(&name, "name", "n", "", "Device name (auto-generated if not specified)")
	cmd.Flags().StringVar(&fleetServer, "fleet-server", "", "Fleet server URL (uses mDNS discovery if not specified)")
	cmd.Flags().StringVar(&sshKeyFile, "ssh-key", "", "SSH public key file for remote access")

	// OS image selection
	cmd.Flags().StringVar(&osType, "os", "", "Operating system to install (raspios) - defaults to raspios")
	cmd.Flags().StringVar(&imageURL, "image-url", "", "Custom OS image URL or local file path (overrides --os)")
	cmd.Flags().StringVar(&imageSHA256URL, "image-sha256-url", "", "URL to SHA256 checksum for custom image")

	// Plugin support
	cmd.Flags().StringSliceVar(&plugins, "plugin", []string{}, "Load plugin (can be repeated)")
	cmd.Flags().StringSliceVar(&pluginOpts, "plugin-opt", []string{}, "Plugin option key=value (can be repeated)")
	cmd.Flags().StringVar(&pluginDir, "plugin-dir", "~/.fleetd/plugins", "Plugin directory")

	// Other options
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview actions without making changes")
	cmd.Flags().BoolVar(&configureOnly, "configure-only", false, "Only configure existing OS image (skip writing image)")

	return cmd
}

// provisionOptions holds all configuration for the provision command
type provisionOptions struct {
	device         string
	name           string
	imageURL       string
	imageSHA256URL string
	osType         string
	wifiSSID       string
	wifiPass       string
	fleetServer    string
	sshKeyFile     string
	plugins        []string
	pluginOpts     []string
	pluginDir      string
	dryRun         bool
	configureOnly  bool
}

func runProvision(cmd *cobra.Command, args []string, device, name, imageURL, imageSHA256URL, osType, wifiSSID, wifiPass, fleetServer, sshKeyFile string, plugins, pluginOpts []string, pluginDir string, dryRun, configureOnly bool) error {
	opts := &provisionOptions{
		device:         device,
		name:           name,
		imageURL:       imageURL,
		imageSHA256URL: imageSHA256URL,
		osType:         osType,
		wifiSSID:       wifiSSID,
		wifiPass:       wifiPass,
		fleetServer:    fleetServer,
		sshKeyFile:     sshKeyFile,
		plugins:        plugins,
		pluginOpts:     pluginOpts,
		pluginDir:      pluginDir,
		dryRun:         dryRun,
		configureOnly:  configureOnly,
	}

	// Verify admin access
	if err := verifyAdminAccess(opts.dryRun); err != nil {
		return err
	}

	// Build provisioning configuration
	config, err := buildProvisionConfig(opts)
	if err != nil {
		return err
	}

	// Create and setup provisioner
	provisioner, err := setupProvisioner(config, opts.pluginDir)
	if err != nil {
		return err
	}

	// Display provisioning summary
	displayProvisioningSummary(config, opts)

	// Execute provisioning
	return executeProvisioning(provisioner, config, opts)
}

// verifyAdminAccess checks for sudo/admin privileges
func verifyAdminAccess(dryRun bool) error {
	if !dryRun {
		if err := checkSudoAccess(); err != nil {
			return fmt.Errorf("this command requires sudo/administrator privileges: %w", err)
		}
		printSuccess("Administrator access verified")
	}
	return nil
}

// buildProvisionConfig creates the provision configuration
func buildProvisionConfig(opts *provisionOptions) (*provision.Config, error) {
	config := &provision.Config{
		DevicePath: opts.device,
		DeviceName: opts.name,
		DeviceID:   uuid.New().String(),
		Network: provision.NetworkConfig{
			WiFiSSID: opts.wifiSSID,
			WiFiPass: opts.wifiPass,
		},
		Fleet: provision.FleetConfig{
			ServerURL:    opts.fleetServer,
			AutoRegister: true,
		},
		Security: provision.SecurityConfig{
			EnableSSH: opts.sshKeyFile != "",
		},
	}

	// Auto-detect device type
	detectedType, err := provision.DetectDeviceType(opts.device)
	if err != nil {
		return nil, fmt.Errorf("could not detect device type: %w", err)
	}
	config.DeviceType = detectedType
	if verbose {
		printInfo("Detected device type: %s", detectedType)
	}

	// Set device name if not specified
	if config.DeviceName == "" {
		config.DeviceName = fmt.Sprintf("fleetd-%s-%s", config.DeviceType, config.DeviceID[:8])
	}

	// Load SSH key if specified
	if opts.sshKeyFile != "" {
		keyData, err := os.ReadFile(opts.sshKeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read SSH key: %w", err)
		}
		config.Security.SSHKey = string(keyData)
	}

	// Parse and configure plugins
	if err := configurePlugins(config, opts.plugins, opts.pluginOpts); err != nil {
		return nil, err
	}

	return config, nil
}

// configurePlugins parses and sets up plugin configuration
func configurePlugins(config *provision.Config, plugins []string, pluginOpts []string) error {
	if len(plugins) == 0 {
		return nil
	}

	config.Plugins = make(map[string]any)

	// Initialize each plugin
	for _, plugin := range plugins {
		config.Plugins[plugin] = make(map[string]any)
	}

	// Parse plugin options
	for _, opt := range pluginOpts {
		parts := strings.SplitN(opt, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid plugin option: %s", opt)
		}

		keyParts := strings.SplitN(parts[0], ".", 2)
		if len(keyParts) != 2 {
			return fmt.Errorf("plugin option must be in format plugin.key=value: %s", opt)
		}

		plugin := keyParts[0]
		key := keyParts[1]
		value := parts[1]

		// Create plugin map if needed
		if _, ok := config.Plugins[plugin]; !ok {
			config.Plugins[plugin] = make(map[string]any)
		}

		// Set the option
		if pluginMap, ok := config.Plugins[plugin].(map[string]any); ok {
			pluginMap[key] = value
		}
	}

	return nil
}

// setupProvisioner creates and configures the provisioner
func setupProvisioner(config *provision.Config, pluginDir string) (*provision.CoreProvisioner, error) {
	provisioner := provision.NewCoreProvisioner(config)

	// Load plugins
	pluginDir = expandPath(pluginDir)
	if err := provisioner.LoadPlugins(pluginDir); err != nil {
		if verbose {
			printWarning("Failed to load plugins from %s: %v", pluginDir, err)
		}
	}

	return provisioner, nil
}

// displayProvisioningSummary shows a summary of what will be provisioned
func displayProvisioningSummary(config *provision.Config, opts *provisionOptions) {
	printHeader("Device Provisioning")
	fmt.Printf("Device type: %s\n", config.DeviceType)
	fmt.Printf("Device name: %s\n", config.DeviceName)
	fmt.Printf("Device path: %s\n", config.DevicePath)

	if config.Network.WiFiSSID != "" {
		fmt.Printf("WiFi network: %s\n", config.Network.WiFiSSID)
	}

	if config.Fleet.ServerURL != "" {
		fmt.Printf("Fleet server: %s\n", config.Fleet.ServerURL)
	} else {
		fmt.Println("Fleet server: Will use mDNS discovery")
	}

	if config.Security.EnableSSH {
		fmt.Println("SSH access: Enabled")
	}

	if len(opts.plugins) > 0 {
		fmt.Printf("Plugins: %s\n", strings.Join(opts.plugins, ", "))
	}
}

// executeProvisioning performs the actual provisioning
func executeProvisioning(provisioner *provision.CoreProvisioner, config *provision.Config, opts *provisionOptions) error {
	progress := provision.NewProgressBarReporter(verbose)
	ctx := context.Background()

	// Handle configure-only mode
	if opts.configureOnly {
		return executeConfigureOnly(provisioner, ctx, opts, progress)
	}

	// Handle custom image or standard provisioning
	if opts.imageURL != "" {
		return executeCustomImageProvisioning(provisioner, ctx, opts, progress)
	}

	return executeStandardProvisioning(provisioner, ctx, config, opts, progress)
}

// executeConfigureOnly handles configuration-only mode
func executeConfigureOnly(provisioner *provision.CoreProvisioner, ctx context.Context, opts *provisionOptions, progress provision.ProgressReporter) error {
	printInfo("Configure-only mode: Will configure existing OS image")
	fmt.Println("\nConfiguring in progress...")

	if err := provisioner.ConfigureOnly(ctx, opts.osType, opts.dryRun, progress); err != nil {
		return err
	}

	printSuccess("Configuration complete!")
	displayPostProvisioningSteps()
	return nil
}

// executeCustomImageProvisioning handles custom image provisioning
func executeCustomImageProvisioning(provisioner *provision.CoreProvisioner, ctx context.Context, opts *provisionOptions, progress provision.ProgressReporter) error {
	fmt.Printf("Using custom image: %s\n", opts.imageURL)
	if opts.imageSHA256URL != "" {
		fmt.Printf("SHA256 verification: %s\n", opts.imageSHA256URL)
	}

	fmt.Println("\nProvisioning in progress...")

	if err := provisioner.ProvisionWithCustomImage(ctx, opts.imageURL, opts.imageSHA256URL, opts.dryRun, progress); err != nil {
		return err
	}

	printSuccess("Provisioning complete!")
	displayPostProvisioningSteps()
	return nil
}

// executeStandardProvisioning handles standard OS provisioning
func executeStandardProvisioning(provisioner *provision.CoreProvisioner, ctx context.Context, config *provision.Config, opts *provisionOptions, progress provision.ProgressReporter) error {
	// Determine OS type
	osType := determineOSType(opts.osType, config.DeviceType)
	arch := "arm64" // Default to arm64 for Raspberry Pi

	fmt.Printf("Operating System: %s\n", osType)
	fmt.Println("\nProvisioning in progress...")

	if err := provisioner.ProvisionWithImage(ctx, osType, arch, opts.dryRun, progress); err != nil {
		return err
	}

	printSuccess("Provisioning complete!")
	displayPostProvisioningSteps()
	return nil
}

// determineOSType determines the OS type to use
func determineOSType(requestedOS string, deviceType provision.DeviceType) string {
	if requestedOS != "" {
		return requestedOS
	}

	// Default to Raspberry Pi OS Lite for Raspberry Pi devices
	if deviceType == provision.DeviceTypeRaspberryPi {
		return "raspios"
	}

	// For other device types, use a sensible default
	return string(deviceType)
}

// displayPostProvisioningSteps shows next steps after provisioning
func displayPostProvisioningSteps() {
	fmt.Println("\nNext steps:")
	fmt.Println("1. Safely eject the SD card")
	fmt.Println("2. Insert it into your device")
	fmt.Println("3. Power on the device")
	fmt.Println("4. The device will appear in fleet discovery:")
	fmt.Println("   fleet discover")
}

// checkSudoAccess verifies sudo access by running a harmless command
func checkSudoAccess() error {
	// On macOS, try to run a simple diskutil command
	if _, err := os.Stat("/usr/sbin/diskutil"); err == nil {
		cmd := exec.Command("sudo", "-n", "diskutil", "list")
		if err := cmd.Run(); err != nil {
			// Need to prompt for password
			fmt.Println("This operation requires administrator privileges.")
			cmd = exec.Command("sudo", "diskutil", "list")
			cmd.Stdin = os.Stdin
			cmd.Stdout = nil // Don't show output
			cmd.Stderr = os.Stderr
			return cmd.Run()
		}
		return nil
	}

	// On Linux, try to run a simple ls command
	cmd := exec.Command("sudo", "-n", "ls", "/dev/null")
	if err := cmd.Run(); err != nil {
		// Need to prompt for password
		fmt.Println("This operation requires administrator privileges.")
		cmd = exec.Command("sudo", "ls", "/dev/null")
		cmd.Stdin = os.Stdin
		cmd.Stdout = nil // Don't show output
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}
	return nil
}

func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}
