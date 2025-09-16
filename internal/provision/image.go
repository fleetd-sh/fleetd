package provision

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// ImageProvider defines the interface for OS image providers
type ImageProvider interface {
	// GetImageURL returns the download URL for the image
	GetImageURL(arch string) (string, error)

	// GetImageName returns a descriptive name for the image
	GetImageName() string

	// ValidateImage validates the downloaded image (checksum, etc)
	ValidateImage(imagePath string) error

	// GetBootPartitionLabel returns the label of the boot partition
	GetBootPartitionLabel() string

	// GetRootPartitionLabel returns the label of the root partition
	GetRootPartitionLabel() string

	// PostWriteSetup performs any OS-specific setup after writing to SD card
	PostWriteSetup(bootPath, rootPath string, config *Config) error
}

// ImageManager handles OS image downloads and caching
type ImageManager struct {
	cacheDir             string
	decompressedCacheDir string
	providers            map[string]ImageProvider
}

// NewImageManager creates a new image manager
func NewImageManager(cacheDir string) *ImageManager {
	if cacheDir == "" {
		home, _ := os.UserHomeDir()
		cacheDir = filepath.Join(home, ".fleetd", "images")
	}

	decompressedCacheDir := filepath.Join(cacheDir, "decompressed")

	return &ImageManager{
		cacheDir:             cacheDir,
		decompressedCacheDir: decompressedCacheDir,
		providers:            make(map[string]ImageProvider),
	}
}

// RegisterProvider registers an OS image provider
func (m *ImageManager) RegisterProvider(name string, provider ImageProvider) {
	m.providers[name] = provider
}

// GetProvider returns a provider by name
func (m *ImageManager) GetProvider(name string) (ImageProvider, error) {
	// Normalize the name
	name = strings.ToLower(name)

	provider, ok := m.providers[name]
	if !ok {
		return nil, fmt.Errorf("unknown OS provider: %s", name)
	}

	return provider, nil
}

// DownloadImage downloads an OS image if not cached
func (m *ImageManager) DownloadImage(ctx context.Context, providerName, arch string, progress func(downloaded, total int64)) (string, error) {
	provider, err := m.GetProvider(providerName)
	if err != nil {
		return "", err
	}

	// Get the download URL
	url, err := provider.GetImageURL(arch)
	if err != nil {
		return "", fmt.Errorf("failed to get image URL: %w", err)
	}

	// Check if it's a local file path
	if strings.HasPrefix(url, "/") || strings.HasPrefix(url, "./") || strings.HasPrefix(url, "../") {
		// It's a local file path
		if _, err := os.Stat(url); err != nil {
			return "", fmt.Errorf("local image file not found: %w", err)
		}

		// Validate the local file
		if err := provider.ValidateImage(url); err != nil {
			return "", fmt.Errorf("local image validation failed: %w", err)
		}

		if progress != nil {
			// Report file size for local file
			fi, _ := os.Stat(url)
			progress(fi.Size(), fi.Size())
		}

		return url, nil
	}

	// Create cache directory
	if err := os.MkdirAll(m.cacheDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Generate cache filename
	filename := fmt.Sprintf("%s-%s.img", provider.GetImageName(), arch)
	if strings.HasSuffix(url, ".xz") {
		filename += ".xz"
	} else if strings.HasSuffix(url, ".gz") {
		filename += ".gz"
	} else if strings.HasSuffix(url, ".zip") {
		filename += ".zip"
	}

	imagePath := filepath.Join(m.cacheDir, filename)

	// Check if already cached and valid
	if _, err := os.Stat(imagePath); err == nil {
		// Validate existing image
		if err := provider.ValidateImage(imagePath); err == nil {
			if progress != nil {
				// Report that we're using cached version
				fi, _ := os.Stat(imagePath)
				progress(fi.Size(), fi.Size())
			}
			return imagePath, nil
		}
		// Invalid cache, remove it
		os.Remove(imagePath)
	}

	// Download the image
	if err := m.downloadFile(ctx, url, imagePath, progress); err != nil {
		return "", fmt.Errorf("failed to download image: %w", err)
	}

	// Validate downloaded image
	if err := provider.ValidateImage(imagePath); err != nil {
		os.Remove(imagePath)
		return "", fmt.Errorf("image validation failed: %w", err)
	}

	return imagePath, nil
}

// downloadFile downloads a file with progress reporting
func (m *ImageManager) downloadFile(ctx context.Context, url, destPath string, progress func(downloaded, total int64)) error {
	// Create temporary file
	tmpPath := destPath + ".tmp"
	out, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	defer out.Close()

	// Clean up temp file on failure
	success := false
	defer func() {
		if !success {
			os.Remove(tmpPath)
		}
	}()

	// Create HTTP request with context
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	// Execute request
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	// Get total size
	totalSize := resp.ContentLength

	// Create progress reader if callback provided
	var reader io.Reader = resp.Body
	if progress != nil {
		reader = &progressReader{
			reader:   resp.Body,
			total:    totalSize,
			callback: progress,
		}
	}

	// Copy with progress
	_, err = io.Copy(out, reader)
	if err != nil {
		return err
	}

	// Close file before renaming
	out.Close()

	// Move to final location
	if err := os.Rename(tmpPath, destPath); err != nil {
		return err
	}

	success = true
	return nil
}

// progressReader wraps an io.Reader with progress reporting
type progressReader struct {
	reader     io.Reader
	downloaded int64
	total      int64
	callback   func(downloaded, total int64)
}

func (r *progressReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.downloaded += int64(n)
	if r.callback != nil {
		r.callback(r.downloaded, r.total)
	}
	return n, err
}

// GetDecompressedImage returns a path to a decompressed image, using cache if available
func (m *ImageManager) GetDecompressedImage(ctx context.Context, compressedPath string, progress func(status string)) (string, error) {
	// If the image is not compressed, return it as-is
	if !isCompressedImage(compressedPath) {
		return compressedPath, nil
	}

	// Create decompressed cache directory if it doesn't exist
	if err := os.MkdirAll(m.decompressedCacheDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create decompressed cache directory: %w", err)
	}

	// Generate cache key based on file path and modification time
	fi, err := os.Stat(compressedPath)
	if err != nil {
		return "", fmt.Errorf("failed to stat compressed image: %w", err)
	}

	// Create a hash of the file path and modification time
	h := sha256.New()
	h.Write([]byte(compressedPath))
	h.Write([]byte(fi.ModTime().Format("2006-01-02T15:04:05.999999999")))
	cacheKey := hex.EncodeToString(h.Sum(nil))[:16]

	// Generate decompressed filename
	baseName := filepath.Base(compressedPath)
	// Remove compression extension
	for _, ext := range []string{".xz", ".gz", ".7z", ".zip", ".zst"} {
		if strings.HasSuffix(baseName, ext) {
			baseName = strings.TrimSuffix(baseName, ext)
			break
		}
	}

	decompressedFileName := fmt.Sprintf("%s_%s", cacheKey, baseName)
	decompressedPath := filepath.Join(m.decompressedCacheDir, decompressedFileName)

	// Check if decompressed version exists in cache
	if _, err := os.Stat(decompressedPath); err == nil {
		if progress != nil {
			progress("Using cached decompressed image")
		}
		return decompressedPath, nil
	}

	// Need to decompress
	if progress != nil {
		progress("Decompressing image (this will be cached for future use)")
	}

	// Create an SDCardWriter instance to use its decompression methods
	writer := &SDCardWriter{}

	// Decompress the image
	resultPath, cleanup, err := writer.DecompressImage(ctx, compressedPath, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decompress image: %w", err)
	}

	// If we got a cleanup function, the decompression created a temp file
	if cleanup != nil {
		// Move the decompressed file to our cache
		if err := os.Rename(resultPath, decompressedPath); err != nil {
			// If rename fails (e.g., across filesystems), copy the file
			if err := copyFile(resultPath, decompressedPath); err != nil {
				cleanup()
				return "", fmt.Errorf("failed to cache decompressed image: %w", err)
			}
		}
		// Don't call cleanup since we moved the file
	} else {
		// No cleanup means the file wasn't compressed
		return resultPath, nil
	}

	if progress != nil {
		progress("Image decompressed and cached")
	}

	return decompressedPath, nil
}

// isCompressedImage checks if the image path indicates a compressed file
func isCompressedImage(path string) bool {
	compressedExts := []string{".xz", ".gz", ".7z", ".zip", ".zst"}
	for _, ext := range compressedExts {
		if strings.HasSuffix(strings.ToLower(path), ext) {
			return true
		}
	}
	return false
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}

// ClearDecompressedCache removes all decompressed images from cache
func (m *ImageManager) ClearDecompressedCache() error {
	return os.RemoveAll(m.decompressedCacheDir)
}

// GetDecompressedCacheSize returns the total size of decompressed cache in bytes
func (m *ImageManager) GetDecompressedCacheSize() (int64, error) {
	var totalSize int64

	err := filepath.Walk(m.decompressedCacheDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			totalSize += info.Size()
		}
		return nil
	})

	return totalSize, err
}

// InitializeProviders initializes default OS providers
func InitializeProviders(manager *ImageManager) {
	// Register Raspberry Pi OS provider
	manager.RegisterProvider("rpi", NewRaspberryPiOSProvider())
	manager.RegisterProvider("raspios", NewRaspberryPiOSProvider())
	manager.RegisterProvider("raspios", NewRaspberryPiOSProvider())

	// Future providers can be added here:
	// manager.RegisterProvider("debian", NewDebianProvider())
	// manager.RegisterProvider("ubuntu", NewUbuntuProvider())
	// manager.RegisterProvider("talos", NewTalosProvider())
}
