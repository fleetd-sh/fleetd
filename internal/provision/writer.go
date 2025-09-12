package provision

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"
)

// SDCardWriter handles writing images to SD cards
type SDCardWriter struct {
	devicePath string
	dryRun     bool
}

// NewSDCardWriter creates a new SD card writer
func NewSDCardWriter(devicePath string, dryRun bool) *SDCardWriter {
	return &SDCardWriter{
		devicePath: devicePath,
		dryRun:     dryRun,
	}
}

// WriteImage writes an OS image to the SD card
func (w *SDCardWriter) WriteImage(ctx context.Context, imagePath string, progress func(written, total int64)) error {
	// Decompress image if needed
	decompressedPath, cleanup, err := w.decompressImage(ctx, imagePath, progress)
	if err != nil {
		return fmt.Errorf("failed to decompress image: %w", err)
	}
	if cleanup != nil {
		defer cleanup()
	}
	
	// Get image size for progress reporting
	fi, err := os.Stat(decompressedPath)
	if err != nil {
		return fmt.Errorf("failed to stat image: %w", err)
	}
	totalSize := fi.Size()
	
	if w.dryRun {
		fmt.Printf("[DRY RUN] Would write %d bytes from %s to %s\n", totalSize, decompressedPath, w.devicePath)
		if progress != nil {
			progress(totalSize, totalSize)
		}
		return nil
	}
	
	// Unmount device before writing
	if err := w.unmountDevice(); err != nil {
		return fmt.Errorf("failed to unmount device: %w", err)
	}
	
	// Open source image
	source, err := os.Open(decompressedPath)
	if err != nil {
		return fmt.Errorf("failed to open image: %w", err)
	}
	defer source.Close()
	
	// Open destination device
	dest, err := os.OpenFile(w.devicePath, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("failed to open device %s: %w (try with sudo)", w.devicePath, err)
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
	written, err := io.CopyN(writer, source, totalSize)
	if err != nil && err != io.EOF {
		return fmt.Errorf("failed to write image: %w", err)
	}
	
	// Sync to ensure all data is written
	if err := dest.Sync(); err != nil {
		return fmt.Errorf("failed to sync device: %w", err)
	}
	
	fmt.Printf("Successfully wrote %d bytes to %s\n", written, w.devicePath)
	
	return nil
}

// MountPartitions mounts the boot and root partitions
func (w *SDCardWriter) MountPartitions(bootLabel, rootLabel string) (bootPath, rootPath string, cleanup func(), err error) {
	if w.dryRun {
		// In dry run, create temporary directories
		bootPath = filepath.Join(os.TempDir(), "fleetd-boot")
		rootPath = filepath.Join(os.TempDir(), "fleetd-root")
		os.MkdirAll(bootPath, 0755)
		os.MkdirAll(rootPath, 0755)
		
		cleanup = func() {
			os.RemoveAll(bootPath)
			os.RemoveAll(rootPath)
		}
		
		fmt.Printf("[DRY RUN] Would mount partitions to %s and %s\n", bootPath, rootPath)
		return bootPath, rootPath, cleanup, nil
	}
	
	// Create mount points
	bootPath = filepath.Join(os.TempDir(), "fleetd-boot")
	rootPath = filepath.Join(os.TempDir(), "fleetd-root")
	
	if err := os.MkdirAll(bootPath, 0755); err != nil {
		return "", "", nil, err
	}
	if err := os.MkdirAll(rootPath, 0755); err != nil {
		return "", "", nil, err
	}
	
	// Find partition devices
	bootDev, rootDev, err := w.findPartitions()
	if err != nil {
		return "", "", nil, err
	}
	
	// Mount boot partition
	if err := w.mountPartition(bootDev, bootPath); err != nil {
		os.RemoveAll(bootPath)
		os.RemoveAll(rootPath)
		return "", "", nil, fmt.Errorf("failed to mount boot partition: %w", err)
	}
	
	// Mount root partition
	if err := w.mountPartition(rootDev, rootPath); err != nil {
		w.unmountPartition(bootPath)
		os.RemoveAll(bootPath)
		os.RemoveAll(rootPath)
		return "", "", nil, fmt.Errorf("failed to mount root partition: %w", err)
	}
	
	// Create cleanup function
	cleanup = func() {
		w.unmountPartition(bootPath)
		w.unmountPartition(rootPath)
		os.RemoveAll(bootPath)
		os.RemoveAll(rootPath)
	}
	
	return bootPath, rootPath, cleanup, nil
}

// decompressImage decompresses an image if needed
func (w *SDCardWriter) decompressImage(ctx context.Context, imagePath string, progress func(written, total int64)) (string, func(), error) {
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
	tempFile.Close()
	
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
	tempPath := tempFile.Name()
	
	// Decompress
	_, err = io.Copy(tempFile, gzReader)
	tempFile.Close()
	
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
	tempPath := tempFile.Name()
	
	// Decompress
	_, err = io.Copy(tempFile, decoder)
	tempFile.Close()
	
	if err != nil {
		os.Remove(tempPath)
		return "", nil, err
	}
	
	cleanup := func() {
		os.Remove(tempPath)
	}
	
	return tempPath, cleanup, nil
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
		// On Linux, use mount command
		cmd := exec.Command("mount", device, mountPath)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("mount failed: %s", output)
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