package cmd

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List available devices",
	Long:  `List available devices that can be used for provisioning.`,
	RunE:  runList,
}

func runList(cmd *cobra.Command, args []string) error {
	fmt.Println("Available devices:")

	switch runtime.GOOS {
	case "darwin":
		// On macOS, use diskutil
		return listDevicesMacOS()
	case "linux":
		// On Linux, use lsblk
		return listDevicesLinux()
	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}

func listDevicesMacOS() error {
	// Get disk list
	cmd := exec.Command("diskutil", "list", "-plist")
	output, err := cmd.Output()
	if err != nil {
		// Fallback to simple list
		cmd = exec.Command("diskutil", "list")
		output, err = cmd.Output()
		if err != nil {
			return fmt.Errorf("failed to list devices: %w", err)
		}
		fmt.Println(string(output))
		return nil
	}

	// Parse and display disk info
	// For now, just use the simple output
	cmd = exec.Command("diskutil", "list")
	output, err = cmd.Output()
	if err != nil {
		return err
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		// Filter to show only physical disks
		if strings.Contains(line, "/dev/disk") && !strings.Contains(line, "disk0") {
			fmt.Println(line)
		}
	}

	fmt.Println("\nNote: disk0 is typically your system disk and should not be used.")
	fmt.Println("Look for external disks (usually disk2, disk3, etc.)")

	return nil
}

func listDevicesLinux() error {
	// Use lsblk to list block devices
	cmd := exec.Command("lsblk", "-d", "-o", "NAME,SIZE,TYPE,MOUNTPOINT,MODEL")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to list devices: %w", err)
	}

	fmt.Println(string(output))
	fmt.Println("\nNote: Look for devices without mount points (typically sd* or mmcblk*)")

	return nil
}
