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

var provisionCmd = &cobra.Command{
	Use:   "provision",
	Short: "Provision a device with fleetd",
	Long: `Provision a device with the fleetd agent and optional plugins.

This command writes an OS image to an SD card or device, configures networking,
and sets up the fleetd agent for fleet management.`,
	Example: `  # Basic provisioning with Raspberry Pi OS Lite (default)
  fleetp provision --device /dev/disk2 --wifi-ssid MyWiFi --wifi-pass secret

  # With DietPi instead
  fleetp provision --device /dev/disk2 --wifi-ssid MyWiFi --wifi-pass secret --os dietpi

  # With custom image URL (e.g., for Raspberry Pi 5)
  fleetp provision --device /dev/disk2 --wifi-ssid MyWiFi --wifi-pass secret \
    --image-url https://dietpi.com/downloads/images/DietPi_RPi5-ARMv8-Bookworm.img.xz \
    --image-sha256-url https://dietpi.com/downloads/images/DietPi_RPi5-ARMv8-Bookworm.img.xz.sha256

  # With local image file
  fleetp provision --device /dev/disk2 --wifi-ssid MyWiFi --wifi-pass secret \
    --image-url /path/to/raspios-lite-arm64.img.xz

  # With k3s plugin
  fleetp provision --device /dev/disk2 --wifi-ssid MyWiFi --wifi-pass secret \
    --plugin k3s --plugin-opt k3s.role=server`,
	RunE: runProvision,
}

func init() {
	// Required flags
	provisionCmd.Flags().StringVarP(&device, "device", "d", "", "Device path (e.g., /dev/disk2)")
	provisionCmd.MarkFlagRequired("device")

	// Network flags (essential)
	provisionCmd.Flags().StringVar(&wifiSSID, "wifi-ssid", "", "WiFi network name")
	provisionCmd.Flags().StringVar(&wifiPass, "wifi-pass", "", "WiFi password")

	// Device configuration
	provisionCmd.Flags().StringVarP(&name, "name", "n", "", "Device name (auto-generated if not specified)")
	provisionCmd.Flags().StringVar(&fleetServer, "fleet-server", "", "Fleet server URL (uses mDNS discovery if not specified)")
	provisionCmd.Flags().StringVar(&sshKeyFile, "ssh-key", "", "SSH public key file for remote access")

	// OS image selection
	provisionCmd.Flags().StringVar(&osType, "os", "", "Operating system to install (rpios, dietpi) - defaults to rpios")
	provisionCmd.Flags().StringVar(&imageURL, "image-url", "", "Custom OS image URL or local file path (overrides --os)")
	provisionCmd.Flags().StringVar(&imageSHA256URL, "image-sha256-url", "", "URL to SHA256 checksum for custom image")

	// Plugin support
	provisionCmd.Flags().StringSliceVar(&plugins, "plugin", []string{}, "Load plugin (can be repeated)")
	provisionCmd.Flags().StringSliceVar(&pluginOpts, "plugin-opt", []string{}, "Plugin option key=value (can be repeated)")
	provisionCmd.Flags().StringVar(&pluginDir, "plugin-dir", "~/.fleetd/plugins", "Plugin directory")

	// Other options
	provisionCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview actions without making changes")
	provisionCmd.Flags().BoolVar(&configureOnly, "configure-only", false, "Only configure existing OS image (skip writing image)")
}

func runProvision(cmd *cobra.Command, args []string) error {
	// Check for sudo/admin privileges early
	if !dryRun {
		// Try to run a harmless sudo command to trigger password prompt early
		if err := checkSudoAccess(); err != nil {
			return fmt.Errorf("this command requires sudo/administrator privileges: %w", err)
		}
		fmt.Println("✓ Administrator access verified")
	}

	// Create config
	config := &provision.Config{
		DevicePath: device,
		DeviceName: name,
		DeviceID:   uuid.New().String(),
		Network: provision.NetworkConfig{
			WiFiSSID: wifiSSID,
			WiFiPass: wifiPass,
		},
		Fleet: provision.FleetConfig{
			ServerURL:    fleetServer,
			AutoRegister: true,
		},
		Security: provision.SecurityConfig{
			EnableSSH: sshKeyFile != "",
		},
	}

	// Auto-detect device type
	detectedType, err := provision.DetectDeviceType(device)
	if err != nil {
		return fmt.Errorf("could not detect device type: %w", err)
	}
	config.DeviceType = detectedType
	if verbose {
		fmt.Printf("Detected device type: %s\n", detectedType)
	}

	// Set device name if not specified
	if config.DeviceName == "" {
		config.DeviceName = fmt.Sprintf("fleetd-%s-%s", config.DeviceType, config.DeviceID[:8])
	}

	// Load SSH key if specified
	if sshKeyFile != "" {
		keyData, err := os.ReadFile(sshKeyFile)
		if err != nil {
			return fmt.Errorf("failed to read SSH key: %w", err)
		}
		config.Security.SSHKey = string(keyData)
	}

	// Parse plugin options
	if len(plugins) > 0 {
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
	}

	// Create core provisioner
	provisioner := provision.NewCoreProvisioner(config)

	// Load plugins
	pluginDir = expandPath(pluginDir)
	if err := provisioner.LoadPlugins(pluginDir); err != nil {
		if verbose {
			fmt.Printf("Warning: failed to load plugins from %s: %v\n", pluginDir, err)
		}
	}

	// Display what we're doing
	fmt.Printf("Provisioning %s device: %s\n", config.DeviceType, config.DeviceName)
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
	if len(plugins) > 0 {
		fmt.Printf("Plugins: %s\n", strings.Join(plugins, ", "))
	}

	// Create progress reporter with better progress bars
	progress := provision.NewProgressBarReporter(verbose)

	// Handle configure-only mode
	if configureOnly {
		fmt.Println("Configure-only mode: Will configure existing OS image")
		fmt.Println("\nConfiguring in progress...")

		ctx := context.Background()
		if err := provisioner.ConfigureOnly(ctx, osType, dryRun, progress); err != nil {
			return err
		}

		fmt.Println("\n✓ Configuration complete!")
		fmt.Println("\nNext steps:")
		fmt.Println("1. Safely eject the SD card")
		fmt.Println("2. Insert it into your device")
		fmt.Println("3. Power on the device")
		fmt.Println("4. The device will appear in fleet discovery:")
		fmt.Println("   fleets discover")
		return nil
	}

	// Handle custom image URL
	if imageURL != "" {
		fmt.Printf("Using custom image: %s\n", imageURL)
		if imageSHA256URL != "" {
			fmt.Printf("SHA256 verification: %s\n", imageSHA256URL)
		}

		fmt.Println("\nProvisioning in progress...")

		// Use custom image provisioning
		ctx := context.Background()
		if err := provisioner.ProvisionWithCustomImage(ctx, imageURL, imageSHA256URL, dryRun, progress); err != nil {
			return err
		}
	} else {
		// Determine OS type
		if osType == "" {
			// Default to Raspberry Pi OS Lite for Raspberry Pi devices
			if config.DeviceType == provision.DeviceTypeRaspberryPi {
				osType = "rpios"
			} else {
				// For other device types, use a sensible default
				osType = string(config.DeviceType)
			}
		}

		// Default to arm64 for Raspberry Pi
		arch := "arm64"

		// Show which OS will be installed
		fmt.Printf("Operating System: %s\n", osType)

		fmt.Println("\nProvisioning in progress...")

		// Run the actual provisioning with OS image
		ctx := context.Background()
		if err := provisioner.ProvisionWithImage(ctx, osType, arch, dryRun, progress); err != nil {
			return err
		}
	}

	fmt.Println("\n✓ Provisioning complete!")
	fmt.Println("\nNext steps:")
	fmt.Println("1. Insert the SD card into your device")
	fmt.Println("2. Power on the device")
	fmt.Println("3. The device will appear in fleet discovery:")
	fmt.Println("   fleets discover")

	return nil
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
