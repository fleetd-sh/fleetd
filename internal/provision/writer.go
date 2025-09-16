package provision

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"
)

// SDCardWriter handles writing images to SD cards
type SDCardWriter struct {
	devicePath string
	dryRun     bool
}

// NewSDCardWriter creates a new SD card writer
func NewSDCardWriter(devicePath string, dryRun bool) (*SDCardWriter, error) {
	// Validate device path
	if err := ValidateDevicePath(devicePath); err != nil {
		return nil, fmt.Errorf("invalid device path: %w", err)
	}

	return &SDCardWriter{
		devicePath: devicePath,
		dryRun:     dryRun,
	}, nil
}

// WriteImage writes an OS image to the SD card
func (w *SDCardWriter) WriteImage(ctx context.Context, imagePath string, progress func(written, total int64)) error {
	// Validate image path
	if err := ValidateImagePath(imagePath); err != nil {
		return fmt.Errorf("invalid image path: %w", err)
	}

	// Use the provided path directly - assume caller handles decompression
	return w.WriteDecompressedImage(ctx, imagePath, progress)
}

// WriteDecompressedImage writes a decompressed image to the SD card
func (w *SDCardWriter) WriteDecompressedImage(ctx context.Context, imagePath string, progress func(written, total int64)) error {
	// Validate image path
	if err := ValidateImagePath(imagePath); err != nil {
		return fmt.Errorf("invalid image path: %w", err)
	}

	// Check if we can open the device first
	if !w.dryRun {
		// Check device access
		testFile, err := os.OpenFile(w.devicePath, os.O_RDWR, 0)
		if err != nil {
			if os.IsPermission(err) {
				slog.Info("need sudo access to write to device", "device", w.devicePath)
				fmt.Println("Will prompt for sudo password when ready to write...")
			} else {
				return fmt.Errorf("device not accessible: %w", err)
			}
		} else {
			testFile.Close()
		}
	}

	decompressedPath := imagePath

	// Get image size for progress reporting
	fi, err := os.Stat(decompressedPath)
	if err != nil {
		return fmt.Errorf("failed to stat image: %w", err)
	}
	totalSize := fi.Size()

	if w.dryRun {
		slog.Info("dry run: would write image to device",
			"size_bytes", totalSize,
			"source", decompressedPath,
			"target", w.devicePath)
		if progress != nil {
			progress(totalSize, totalSize)
		}
		return nil
	}

	// Unmount device before writing (skip for disk images)
	if !isDiskImage(w.devicePath) {
		if err := w.unmountDevice(); err != nil {
			return fmt.Errorf("failed to unmount device: %w", err)
		}
	}

	// Open source image
	source, err := os.Open(decompressedPath)
	if err != nil {
		return fmt.Errorf("failed to open image: %w", err)
	}
	defer source.Close()

	// Open destination device with sudo if needed
	dest, err := w.openDeviceForWriting()
	if err != nil {
		return fmt.Errorf("failed to open device %s: %w", w.devicePath, err)
	}
	defer dest.Close()

	// Create progress writer
	var writer io.Writer = dest
	if progress != nil {
		writer = &progressWriter{
			writer:   dest,
			total:    totalSize,
			callback: progress,
		}
	}

	// Write image to device
	written, err := io.Copy(writer, source)
	if err != nil {
		return fmt.Errorf("failed to write image: %w", err)
	}

	// Sync to ensure all data is written
	if err := dest.Sync(); err != nil {
		return fmt.Errorf("failed to sync device: %w", err)
	}

	slog.Info("successfully wrote image to device",
		"bytes_written", written,
		"device", w.devicePath)

	return nil
}

// MountPartitions mounts the boot and root partitions
func (w *SDCardWriter) MountPartitions(bootLabel, rootLabel string) (bootPath, rootPath string, cleanup func(), err error) {
	// For disk images, we can't mount partitions directly
	// Instead, we'll extract files using loopback mounting (Linux) or return temp dirs
	if isDiskImage(w.devicePath) {
		return w.mountDiskImagePartitions(bootLabel, rootLabel)
	}

	if w.dryRun {
		// In dry run, create temporary directories
		bootPath = filepath.Join(os.TempDir(), "fleetd-boot")
		rootPath = filepath.Join(os.TempDir(), "fleetd-root")
		os.MkdirAll(bootPath, 0o755)
		os.MkdirAll(rootPath, 0o755)

		cleanup = func() {
			os.RemoveAll(bootPath)
			os.RemoveAll(rootPath)
		}

		slog.Info("dry run: would mount partitions",
			"boot_path", bootPath,
			"root_path", rootPath)
		return bootPath, rootPath, cleanup, nil
	}

	// Wait for device to be ready after writing
	fmt.Println("Waiting for device to be ready...")
	time.Sleep(3 * time.Second)

	// Re-read partition table on macOS
	if runtime.GOOS == "darwin" {
		cmd := exec.Command("diskutil", "list", w.devicePath)
		cmd.Run()
	}

	// Find partition devices
	bootDev, rootDev, err := w.findPartitions()
	if err != nil {
		return "", "", nil, err
	}

	// Check if partitions are already mounted (common on macOS)
	bootPath, rootPath = w.findExistingMounts(bootDev, rootDev)
	slog.Info("found existing mounts",
		"boot_path", bootPath,
		"root_path", rootPath)

	// If not already mounted, create mount points and mount
	if bootPath == "" {
		bootPath = filepath.Join(os.TempDir(), "fleetd-boot")
		if err := os.MkdirAll(bootPath, 0o755); err != nil {
			return "", "", nil, err
		}

		// Mount boot partition
		if err := w.mountPartition(bootDev, bootPath); err != nil {
			os.RemoveAll(bootPath)
			return "", "", nil, fmt.Errorf("failed to mount boot partition: %w", err)
		}
	} else {
		slog.Info("using existing mount point", "boot_path", bootPath)
	}

	// Handle root partition
	if rootPath == "" && rootDev != "" {
		rootPath = filepath.Join(os.TempDir(), "fleetd-root")
		if err := os.MkdirAll(rootPath, 0o755); err != nil {
			// Cleanup boot if we mounted it
			if strings.Contains(bootPath, "fleetd-boot") {
				w.unmountPartition(bootPath)
				os.RemoveAll(bootPath)
			}
			return "", "", nil, err
		}

		// Mount root partition (might be ext4, which macOS can't mount)
		if err := w.mountPartition(rootDev, rootPath); err != nil {
			// On macOS, we might not be able to mount ext4 root partition
			if runtime.GOOS == "darwin" && strings.Contains(err.Error(), "failed to mount") {
				fmt.Println("Note: Cannot mount root partition on macOS (likely ext4 filesystem)")
				// Continue with just boot partition mounted
				cleanup = func() {
					// Only unmount if we mounted it
					if strings.Contains(bootPath, "fleetd-boot") {
						w.unmountPartition(bootPath)
						os.RemoveAll(bootPath)
					}
					os.RemoveAll(rootPath)
				}
				return bootPath, "", cleanup, nil
			}
			// Cleanup on error
			if strings.Contains(bootPath, "fleetd-boot") {
				w.unmountPartition(bootPath)
				os.RemoveAll(bootPath)
			}
			os.RemoveAll(rootPath)
			return "", "", nil, fmt.Errorf("failed to mount root partition: %w", err)
		}
	}

	// Create cleanup function
	cleanup = func() {
		// Only unmount if we mounted it (temp directory)
		if strings.Contains(bootPath, "fleetd-boot") {
			w.unmountPartition(bootPath)
			os.RemoveAll(bootPath)
		}
		if strings.Contains(rootPath, "fleetd-root") {
			w.unmountPartition(rootPath)
			os.RemoveAll(rootPath)
		}
		// For existing mounts, we don't unmount them
	}

	return bootPath, rootPath, cleanup, nil
}

// DecompressImage decompresses an image if needed
func (w *SDCardWriter) DecompressImage(ctx context.Context, imagePath string, progress func(written, total int64)) (string, func(), error) {
	// Check if image is compressed
	if strings.HasSuffix(imagePath, ".xz") {
		return w.decompressXZ(ctx, imagePath, progress)
	} else if strings.HasSuffix(imagePath, ".gz") {
		return w.decompressGZ(ctx, imagePath, progress)
	} else if strings.HasSuffix(imagePath, ".7z") {
		return w.decompress7z(ctx, imagePath, progress)
	} else if strings.HasSuffix(imagePath, ".zip") {
		return w.decompressZip(ctx, imagePath, progress)
	} else if strings.HasSuffix(imagePath, ".zst") {
		return w.decompressZstd(ctx, imagePath, progress)
	}

	// Not compressed, return as-is
	return imagePath, nil, nil
}

// decompressXZ decompresses an XZ compressed image
func (w *SDCardWriter) decompressXZ(ctx context.Context, imagePath string, progress func(written, total int64)) (string, func(), error) {
	source, err := os.Open(imagePath)
	if err != nil {
		return "", nil, err
	}
	defer source.Close()

	// Create XZ reader
	xzReader, err := xz.NewReader(source)
	if err != nil {
		return "", nil, err
	}

	// Create temporary file for decompressed image
	tempFile, err := os.CreateTemp("", "fleetd-image-*.img")
	if err != nil {
		return "", nil, err
	}
	defer tempFile.Close()
	tempPath := tempFile.Name()

	// Create progress reader if callback provided
	var reader io.Reader = xzReader
	if progress != nil {
		// For compressed files, we can't know the total size ahead of time
		// so we'll report based on compressed file size
		fi, _ := os.Stat(imagePath)
		reader = &progressReader{
			reader:   xzReader,
			total:    fi.Size() * 4, // Estimate 4x compression ratio
			callback: progress,
		}
	}

	// Decompress
	_, err = io.Copy(tempFile, reader)

	if err != nil {
		os.Remove(tempPath)
		return "", nil, err
	}

	cleanup := func() {
		os.Remove(tempPath)
	}

	return tempPath, cleanup, nil
}

// decompressGZ decompresses a GZIP compressed image
func (w *SDCardWriter) decompressGZ(ctx context.Context, imagePath string, progress func(written, total int64)) (string, func(), error) {
	source, err := os.Open(imagePath)
	if err != nil {
		return "", nil, err
	}
	defer source.Close()

	// Create GZIP reader
	gzReader, err := gzip.NewReader(source)
	if err != nil {
		return "", nil, err
	}
	defer gzReader.Close()

	// Create temporary file
	tempFile, err := os.CreateTemp("", "fleetd-image-*.img")
	if err != nil {
		return "", nil, err
	}
	defer tempFile.Close()
	tempPath := tempFile.Name()

	// Decompress
	_, err = io.Copy(tempFile, gzReader)

	if err != nil {
		os.Remove(tempPath)
		return "", nil, err
	}

	cleanup := func() {
		os.Remove(tempPath)
	}

	return tempPath, cleanup, nil
}

// decompress7z decompresses a 7-Zip compressed image
func (w *SDCardWriter) decompress7z(ctx context.Context, imagePath string, progress func(written, total int64)) (string, func(), error) {
	// Check for 7z command
	if _, err := exec.LookPath("7z"); err != nil {
		return "", nil, fmt.Errorf("7z command not found. Please install p7zip: brew install p7zip (macOS) or apt-get install p7zip-full (Linux)")
	}

	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "fleetd-extract-")
	if err != nil {
		return "", nil, err
	}

	// Extract using 7z
	cmd := exec.CommandContext(ctx, "7z", "x", "-o"+tempDir, imagePath, "-y")
	if err := cmd.Run(); err != nil {
		os.RemoveAll(tempDir)
		return "", nil, fmt.Errorf("failed to extract 7z archive: %w", err)
	}

	// Find the .img file
	files, err := filepath.Glob(filepath.Join(tempDir, "*.img"))
	if err != nil || len(files) == 0 {
		os.RemoveAll(tempDir)
		return "", nil, fmt.Errorf("no .img file found in archive")
	}

	cleanup := func() {
		os.RemoveAll(tempDir)
	}

	return files[0], cleanup, nil
}

// decompressZip decompresses a ZIP compressed image
func (w *SDCardWriter) decompressZip(ctx context.Context, imagePath string, progress func(written, total int64)) (string, func(), error) {
	// Use unzip command line tool
	if _, err := exec.LookPath("unzip"); err != nil {
		return "", nil, fmt.Errorf("unzip command not found")
	}

	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "fleetd-extract-")
	if err != nil {
		return "", nil, err
	}

	// Extract using unzip
	cmd := exec.CommandContext(ctx, "unzip", "-q", imagePath, "-d", tempDir)
	if err := cmd.Run(); err != nil {
		os.RemoveAll(tempDir)
		return "", nil, fmt.Errorf("failed to extract zip archive: %w", err)
	}

	// Find the .img file
	files, err := filepath.Glob(filepath.Join(tempDir, "*.img"))
	if err != nil || len(files) == 0 {
		os.RemoveAll(tempDir)
		return "", nil, fmt.Errorf("no .img file found in archive")
	}

	cleanup := func() {
		os.RemoveAll(tempDir)
	}

	return files[0], cleanup, nil
}

// decompressZstd decompresses a Zstandard compressed image
func (w *SDCardWriter) decompressZstd(ctx context.Context, imagePath string, progress func(written, total int64)) (string, func(), error) {
	source, err := os.Open(imagePath)
	if err != nil {
		return "", nil, err
	}
	defer source.Close()

	// Create Zstd decoder
	decoder, err := zstd.NewReader(source)
	if err != nil {
		return "", nil, err
	}
	defer decoder.Close()

	// Create temporary file
	tempFile, err := os.CreateTemp("", "fleetd-image-*.img")
	if err != nil {
		return "", nil, err
	}
	defer tempFile.Close()
	tempPath := tempFile.Name()

	// Decompress
	_, err = io.Copy(tempFile, decoder)

	if err != nil {
		os.Remove(tempPath)
		return "", nil, err
	}

	cleanup := func() {
		os.Remove(tempPath)
	}

	return tempPath, cleanup, nil
}

// deviceWriter is an interface for writing to a device
type deviceWriter interface {
	io.WriteCloser
	Sync() error
}

// openDeviceForWriting opens the device for writing, using sudo if needed
func (w *SDCardWriter) openDeviceForWriting() (deviceWriter, error) {
	// Check if this is a regular file (disk image) vs block device
	if isDiskImage(w.devicePath) {
		// For disk images, just open as a regular file
		file, err := os.OpenFile(w.devicePath, os.O_RDWR|os.O_CREATE, 0o644)
		if err != nil {
			return nil, fmt.Errorf("failed to open disk image: %w", err)
		}
		return file, nil
	}

	// For block devices, try to open directly first
	file, err := os.OpenFile(w.devicePath, os.O_RDWR, 0)
	if err == nil {
		return file, nil
	}

	// If permission denied, try with sudo
	if os.IsPermission(err) {
		slog.Info("need sudo access to write to device", "device", w.devicePath)

		// Use dd with sudo for the actual writing
		// We'll return a pipe that dd will read from
		cmd := exec.Command("sudo", "dd", "of="+w.devicePath, "bs=4M", "status=progress")

		// Get stdin pipe
		stdin, err := cmd.StdinPipe()
		if err != nil {
			return nil, fmt.Errorf("failed to create pipe: %w", err)
		}

		// Start the command
		if err := cmd.Start(); err != nil {
			return nil, fmt.Errorf("failed to start dd: %w", err)
		}

		// Return a wrapper that will handle the process
		return &sudoWriter{
			cmd:   cmd,
			stdin: stdin,
		}, nil
	}

	return nil, err
}

type sudoWriter struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser
}

func (s *sudoWriter) Write(p []byte) (n int, err error) {
	return s.stdin.Write(p)
}

func (s *sudoWriter) Sync() error {
	// Close stdin to signal EOF to dd
	s.stdin.Close()
	// Wait for dd to complete
	return s.cmd.Wait()
}

func (s *sudoWriter) Close() error {
	s.stdin.Close()
	return s.cmd.Wait()
}

// unmountDevice unmounts all partitions of the device
func (w *SDCardWriter) unmountDevice() error {
	switch runtime.GOOS {
	case "darwin":
		// On macOS, use diskutil
		cmd := exec.Command("diskutil", "unmountDisk", w.devicePath)
		if output, err := cmd.CombinedOutput(); err != nil {
			// Check if already unmounted
			if !strings.Contains(string(output), "was already unmounted") {
				return fmt.Errorf("unmount failed: %s", output)
			}
		}
	case "linux":
		// On Linux, unmount all partitions
		cmd := exec.Command("umount", w.devicePath+"*")
		cmd.Run() // Ignore error if not mounted
	}
	return nil
}

// findPartitions finds the boot and root partition devices
func (w *SDCardWriter) findPartitions() (bootDev, rootDev string, err error) {
	switch runtime.GOOS {
	case "darwin":
		// On macOS, partitions are like /dev/disk2s1, /dev/disk2s2
		bootDev = w.devicePath + "s1"
		rootDev = w.devicePath + "s2"
	case "linux":
		// On Linux, partitions are like /dev/sdb1, /dev/sdb2
		// or /dev/mmcblk0p1, /dev/mmcblk0p2
		if strings.Contains(w.devicePath, "mmcblk") {
			bootDev = w.devicePath + "p1"
			rootDev = w.devicePath + "p2"
		} else {
			bootDev = w.devicePath + "1"
			rootDev = w.devicePath + "2"
		}
	default:
		return "", "", fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}

	return bootDev, rootDev, nil
}

// mountPartition mounts a partition to a path
func (w *SDCardWriter) mountPartition(device, mountPath string) error {
	switch runtime.GOOS {
	case "darwin":
		// On macOS, use diskutil
		cmd := exec.Command("diskutil", "mount", "-mountPoint", mountPath, device)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("mount failed: %s", output)
		}
	case "linux":
		// On Linux, use mount command (may need sudo)
		cmd := exec.Command("mount", device, mountPath)
		if _, err := cmd.CombinedOutput(); err != nil {
			// Try with sudo
			cmd = exec.Command("sudo", "mount", device, mountPath)
			if output, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("mount failed: %s", output)
			}
		}
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}

	return nil
}

// unmountPartition unmounts a partition
func (w *SDCardWriter) unmountPartition(mountPath string) error {
	switch runtime.GOOS {
	case "darwin":
		cmd := exec.Command("diskutil", "unmount", mountPath)
		cmd.Run() // Ignore error
	case "linux":
		cmd := exec.Command("umount", mountPath)
		cmd.Run() // Ignore error
	}
	return nil
}

// findExistingMounts checks if partitions are already mounted
func (w *SDCardWriter) findExistingMounts(bootDev, rootDev string) (bootPath, rootPath string) {
	switch runtime.GOOS {
	case "darwin":
		// On macOS, check mount output for existing mounts
		cmd := exec.Command("mount")
		output, err := cmd.Output()
		if err != nil {
			return "", ""
		}

		slog.Debug("looking for devices in mount output",
			"boot_device", bootDev,
			"root_device", rootDev)
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			// Look for our boot device
			if strings.Contains(line, bootDev) {
				slog.Debug("found boot device in mount line", "line", line)
				// Extract mount point
				// Format: /dev/disk14s1 on /Volumes/bootfs (msdos, ...)
				// or: /dev/disk14s1 on /Volumes/boot 1 (msdos, ...)
				onIndex := strings.Index(line, " on ")
				if onIndex != -1 {
					afterOn := line[onIndex+4:]
					// Find the opening parenthesis which marks end of path
					parenIndex := strings.Index(afterOn, " (")
					if parenIndex != -1 {
						bootPath = afterOn[:parenIndex]
						slog.Debug("extracted boot mount path", "path", bootPath)
					}
				}
			}
			// Look for root device
			if rootDev != "" && strings.Contains(line, rootDev) {
				slog.Debug("found root device in mount line", "line", line)
				// Extract mount point (same logic as boot)
				onIndex := strings.Index(line, " on ")
				if onIndex != -1 {
					afterOn := line[onIndex+4:]
					parenIndex := strings.Index(afterOn, " (")
					if parenIndex != -1 {
						rootPath = afterOn[:parenIndex]
						slog.Debug("extracted root mount path", "path", rootPath)
					}
				}
			}
		}
	case "linux":
		// On Linux, check /proc/mounts
		data, err := os.ReadFile("/proc/mounts")
		if err != nil {
			return "", ""
		}

		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				if fields[0] == bootDev {
					bootPath = fields[1]
				}
				if rootDev != "" && fields[0] == rootDev {
					rootPath = fields[1]
				}
			}
		}
	}

	return bootPath, rootPath
}

// SyncPartitions ensures all data is written to disk before unmounting
func (w *SDCardWriter) SyncPartitions(bootPath, rootPath string) error {
	switch runtime.GOOS {
	case "darwin":
		// On macOS, sync specific mount points and the device
		fmt.Println("Syncing data to SD card...")

		// Sync the boot partition
		if bootPath != "" {
			cmd := exec.Command("sync")
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("failed to sync: %w", err)
			}
		}

		// Force disk sync using diskutil
		cmd := exec.Command("diskutil", "synchronizeDisk", w.devicePath)
		if output, err := cmd.CombinedOutput(); err != nil {
			// Try alternative sync method
			cmd = exec.Command("sync")
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("failed to sync disk: %s", output)
			}
		}

		// Give the system a moment to complete the sync
		time.Sleep(2 * time.Second)

	case "linux":
		// On Linux, sync all filesystems
		cmd := exec.Command("sync")
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to sync: %w", err)
		}

		// Also try to sync specific partitions
		bootDev, rootDev, _ := w.findPartitions()
		if bootDev != "" {
			cmd = exec.Command("sync", bootDev)
			cmd.Run() // Ignore error
		}
		if rootDev != "" {
			cmd = exec.Command("sync", rootDev)
			cmd.Run() // Ignore error
		}
	}

	return nil
}

// progressWriter wraps an io.Writer with progress reporting
type progressWriter struct {
	writer   io.Writer
	written  int64
	total    int64
	callback func(written, total int64)
}

func (w *progressWriter) Write(p []byte) (int, error) {
	n, err := w.writer.Write(p)
	w.written += int64(n)
	if w.callback != nil {
		w.callback(w.written, w.total)
	}
	return n, err
}

// mountDiskImagePartitions handles mounting for disk image files
func (w *SDCardWriter) mountDiskImagePartitions(bootLabel, rootLabel string) (string, string, func(), error) {
	mounter, err := NewDiskImageMounter()
	if err != nil {
		return "", "", nil, err
	}

	return mounter.Mount(w.devicePath)
}
