package provision

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// DiskImageMounter defines the interface for mounting disk images on different platforms
type DiskImageMounter interface {
	// Mount mounts the disk image and returns paths to boot and root partitions
	// Returns: bootPath, rootPath, cleanup function, error
	Mount(imagePath string) (string, string, func(), error)
}

// NewDiskImageMounter creates a platform-specific disk image mounter
func NewDiskImageMounter() (DiskImageMounter, error) {
	switch runtime.GOOS {
	case "darwin":
		return &macOSDiskImageMounter{}, nil
	case "linux":
		return &linuxDiskImageMounter{}, nil
	case "windows":
		return nil, fmt.Errorf("disk image mounting not yet implemented for Windows")
	default:
		return nil, fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

// macOSDiskImageMounter implements DiskImageMounter for macOS using hdiutil
type macOSDiskImageMounter struct{}

func (m *macOSDiskImageMounter) Mount(imagePath string) (string, string, func(), error) {
	fmt.Println("Attaching disk image...")

	// Attach the disk image
	cmd := exec.Command("hdiutil", "attach", "-nomount", imagePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", "", nil, fmt.Errorf("failed to attach disk image: %w\n%s", err, output)
	}

	// Parse output to find the disk device
	diskDevice := m.parseDiskDevice(string(output))
	if diskDevice == "" {
		return "", "", nil, fmt.Errorf("could not find disk device in hdiutil output")
	}

	// Mount partitions
	bootDev := diskDevice + "s1"
	rootDev := diskDevice + "s2"

	bootPath := filepath.Join(os.TempDir(), "fleetd-boot-image")
	rootPath := filepath.Join(os.TempDir(), "fleetd-root-image")

	// Create mount points
	if err := os.MkdirAll(bootPath, 0755); err != nil {
		m.detach(diskDevice)
		return "", "", nil, err
	}
	if err := os.MkdirAll(rootPath, 0755); err != nil {
		m.detach(diskDevice)
		return "", "", nil, err
	}

	// Mount boot partition (FAT32)
	if err := m.mountPartition(bootDev, bootPath); err != nil {
		fmt.Printf("Warning: Could not mount boot partition %s: %v\n", bootDev, err)
	}

	// Try to mount root partition (ext4) - will likely fail on macOS
	if err := m.mountPartition(rootDev, rootPath); err != nil {
		fmt.Printf("Note: Could not mount root partition %s (ext4 not supported on macOS)\n", rootDev)
		rootPath = "" // Signal that root is not available
	}

	cleanup := func() {
		// Unmount partitions
		if bootPath != "" {
			m.unmountPartition(bootPath)
		}
		if rootPath != "" {
			m.unmountPartition(rootPath)
		}
		// Detach disk image
		m.detach(diskDevice)
		// Clean up mount points
		os.RemoveAll(bootPath)
		if rootPath != "" {
			os.RemoveAll(rootPath)
		}
	}

	return bootPath, rootPath, cleanup, nil
}

func (m *macOSDiskImageMounter) parseDiskDevice(output string) string {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "/dev/disk") {
			fields := strings.Fields(line)
			if len(fields) > 0 {
				diskDevice := fields[0]
				if !strings.HasSuffix(diskDevice, "s1") && !strings.HasSuffix(diskDevice, "s2") {
					return diskDevice
				}
			}
		}
	}
	return ""
}

func (m *macOSDiskImageMounter) mountPartition(device, mountPath string) error {
	cmd := exec.Command("diskutil", "mount", "-mountPoint", mountPath, device)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("mount failed: %s", output)
	}
	return nil
}

func (m *macOSDiskImageMounter) unmountPartition(mountPath string) error {
	cmd := exec.Command("diskutil", "unmount", mountPath)
	return cmd.Run()
}

func (m *macOSDiskImageMounter) detach(diskDevice string) error {
	cmd := exec.Command("hdiutil", "detach", diskDevice)
	return cmd.Run()
}

// linuxDiskImageMounter implements DiskImageMounter for Linux using loopback devices
type linuxDiskImageMounter struct{}

func (m *linuxDiskImageMounter) Mount(imagePath string) (string, string, func(), error) {
	fmt.Println("Setting up loop device...")

	// Create a loop device for the image
	cmd := exec.Command("sudo", "losetup", "-f", "--show", "-P", imagePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", "", nil, fmt.Errorf("failed to create loop device: %w\n%s", err, output)
	}

	loopDevice := strings.TrimSpace(string(output))
	fmt.Printf("Created loop device: %s\n", loopDevice)

	// Wait for partitions to appear
	time.Sleep(1 * time.Second)

	// Partition devices
	bootDev := loopDevice + "p1"
	rootDev := loopDevice + "p2"

	bootPath := filepath.Join(os.TempDir(), "fleetd-boot-image")
	rootPath := filepath.Join(os.TempDir(), "fleetd-root-image")

	// Create mount points
	if err := os.MkdirAll(bootPath, 0755); err != nil {
		m.detachLoop(loopDevice)
		return "", "", nil, err
	}
	if err := os.MkdirAll(rootPath, 0755); err != nil {
		m.detachLoop(loopDevice)
		return "", "", nil, err
	}

	// Mount partitions
	if err := m.mountPartition(bootDev, bootPath); err != nil {
		fmt.Printf("Warning: Could not mount boot partition %s: %v\n", bootDev, err)
	}

	if err := m.mountPartition(rootDev, rootPath); err != nil {
		fmt.Printf("Warning: Could not mount root partition %s: %v\n", rootDev, err)
		rootPath = "" // Signal that root is not available
	}

	cleanup := func() {
		// Unmount partitions
		if bootPath != "" {
			m.unmountPartition(bootPath)
		}
		if rootPath != "" {
			m.unmountPartition(rootPath)
		}
		// Remove loop device
		m.detachLoop(loopDevice)
		// Clean up mount points
		os.RemoveAll(bootPath)
		if rootPath != "" {
			os.RemoveAll(rootPath)
		}
	}

	return bootPath, rootPath, cleanup, nil
}

func (m *linuxDiskImageMounter) mountPartition(device, mountPath string) error {
	cmd := exec.Command("sudo", "mount", device, mountPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("mount failed: %s", output)
	}
	return nil
}

func (m *linuxDiskImageMounter) unmountPartition(mountPath string) error {
	cmd := exec.Command("sudo", "umount", mountPath)
	return cmd.Run()
}

func (m *linuxDiskImageMounter) detachLoop(loopDevice string) error {
	cmd := exec.Command("sudo", "losetup", "-d", loopDevice)
	return cmd.Run()
}
