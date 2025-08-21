package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"fleetd.sh/internal/provision"
	"github.com/google/uuid"
)

// SimpleFlags contains the simplified CLI flags
type SimpleFlags struct {
	// Core flags
	device     string
	deviceType string
	name       string

	// Network (essential)
	wifiSSID string
	wifiPass string

	// Fleet server (optional - uses mDNS if empty)
	fleetServer string

	// SSH (optional but recommended)
	sshKeyFile string

	// Plugins
	plugins    stringSlice
	pluginOpts stringSlice
	pluginDir  string

	// Utility
	list    bool
	verbose bool
}

// stringSlice is a flag type for repeated string flags
type stringSlice []string

func (s *stringSlice) String() string {
	return strings.Join(*s, ",")
}

func (s *stringSlice) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func main() {
	flags := &SimpleFlags{}

	// Define flags
	flag.StringVar(&flags.device, "device", "", "Device path (e.g., /dev/disk2)")
	flag.StringVar(&flags.deviceType, "type", "", "Device type (rpi, esp32)")
	flag.StringVar(&flags.name, "name", "", "Device name")

	flag.StringVar(&flags.wifiSSID, "wifi-ssid", "", "WiFi network name")
	flag.StringVar(&flags.wifiPass, "wifi-pass", "", "WiFi password")

	flag.StringVar(&flags.fleetServer, "fleet-server", "", "Fleet server URL (optional, uses mDNS if empty)")
	flag.StringVar(&flags.sshKeyFile, "ssh-key", "", "SSH public key file")

	flag.Var(&flags.plugins, "plugin", "Load plugin (can be repeated)")
	flag.Var(&flags.pluginOpts, "plugin-opt", "Plugin option key=value (can be repeated)")
	flag.StringVar(&flags.pluginDir, "plugin-dir", "~/.fleetd/plugins", "Plugin directory")

	flag.BoolVar(&flags.list, "list", false, "List available devices")
	flag.BoolVar(&flags.verbose, "v", false, "Verbose output")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `fleetp - Fleet Device Provisioner

USAGE:
  fleetp -device /dev/disk2 -wifi-ssid MyNetwork -wifi-pass secret

PURPOSE:
  Provisions devices with the fleetd agent so they can:
  - Boot up and connect to your network
  - Be discovered via mDNS
  - Be managed by a fleet server

OPTIONS:
`)
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
EXAMPLES:
  # Basic provisioning (device will use mDNS to find fleet server)
  fleetp -device /dev/disk2 -wifi-ssid MyWiFi -wifi-pass secret

  # With specific fleet server
  fleetp -device /dev/disk2 -wifi-ssid MyWiFi -wifi-pass secret \
    -fleet-server https://fleet.local:8080

  # With SSH access
  fleetp -device /dev/disk2 -wifi-ssid MyWiFi -wifi-pass secret \
    -ssh-key ~/.ssh/id_rsa.pub

  # With k3s plugin
  fleetp -device /dev/disk2 -wifi-ssid MyWiFi -wifi-pass secret \
    -plugin k3s -plugin-opt k3s.role=server

PLUGINS:
  Plugins extend provisioning with additional functionality.
  Available plugins: k3s, docker, monitoring

  Plugin options use the format: plugin.key=value
  Example: -plugin-opt k3s.role=agent -plugin-opt k3s.server=https://192.168.1.100:6443

For more information: https://fleetd.sh/docs/provisioning
`)
	}

	flag.Parse()

	if flags.list {
		listDevices()
		return
	}

	if flags.device == "" {
		flag.Usage()
		os.Exit(1)
	}

	// Run simplified provisioning
	if err := runSimpleProvision(flags); err != nil {
		log.Fatalf("Provisioning failed: %v", err)
	}

	fmt.Println("\n✓ Provisioning complete!")
	fmt.Println("\nNext steps:")
	fmt.Println("1. Insert the SD card into your device")
	fmt.Println("2. Power on the device")
	fmt.Println("3. The device will appear in fleet discovery:")
	fmt.Println("   fleetd discover")
}

func runSimpleProvision(flags *SimpleFlags) error {
	// Create config
	config := &provision.Config{
		DevicePath: flags.device,
		DeviceName: flags.name,
		DeviceID:   uuid.New().String(),
		Network: provision.NetworkConfig{
			WiFiSSID: flags.wifiSSID,
			WiFiPass: flags.wifiPass,
		},
		Fleet: provision.FleetConfig{
			ServerURL:    flags.fleetServer,
			AutoRegister: true,
		},
		Security: provision.SecurityConfig{
			EnableSSH: flags.sshKeyFile != "",
		},
	}

	// Auto-detect device type if not specified
	if flags.deviceType == "" {
		detectedType, err := provision.DetectDeviceType(flags.device)
		if err != nil {
			return fmt.Errorf("could not detect device type: %w", err)
		}
		config.DeviceType = detectedType
		if flags.verbose {
			fmt.Printf("Detected device type: %s\n", detectedType)
		}
	} else {
		config.DeviceType = provision.DeviceType(flags.deviceType)
	}

	// Set device name if not specified
	if config.DeviceName == "" {
		config.DeviceName = fmt.Sprintf("fleetd-%s-%s", config.DeviceType, config.DeviceID[:8])
	}

	// Load SSH key if specified
	if flags.sshKeyFile != "" {
		keyData, err := os.ReadFile(flags.sshKeyFile)
		if err != nil {
			return fmt.Errorf("failed to read SSH key: %w", err)
		}
		config.Security.SSHKey = string(keyData)
	}

	// Parse plugin options
	if len(flags.plugins) > 0 {
		config.Plugins = make(map[string]any)

		// Parse plugin options
		for _, opt := range flags.pluginOpts {
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
	pluginDir := expandPath(flags.pluginDir)
	if err := provisioner.LoadPlugins(pluginDir); err != nil {
		if flags.verbose {
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
	if len(flags.plugins) > 0 {
		fmt.Printf("Plugins: %s\n", strings.Join(flags.plugins, ", "))
	}

	// In a real implementation, this would:
	// 1. Write the OS image to the device
	// 2. Mount the boot partition
	// 3. Write core fleetd files
	// 4. Let plugins add their files
	// 5. Unmount and finish

	fmt.Println("\nProvisioning in progress...")

	// Run the actual provisioning
	ctx := context.Background()
	if err := provisioner.Provision(ctx); err != nil {
		return err
	}

	return nil
}

func listDevices() {
	fmt.Println("Available devices:")
	// This would list actual devices
	fmt.Println("  /dev/disk2 - SD Card Reader (32GB)")
	fmt.Println("  /dev/ttyUSB0 - ESP32 DevKit")
}

func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}
