package provision

import (
	"context"
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
	cacheDir  string
	providers map[string]ImageProvider
}

// NewImageManager creates a new image manager
func NewImageManager(cacheDir string) *ImageManager {
	if cacheDir == "" {
		home, _ := os.UserHomeDir()
		cacheDir = filepath.Join(home, ".fleetd", "images")
	}
	
	return &ImageManager{
		cacheDir:  cacheDir,
		providers: make(map[string]ImageProvider),
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
	
	// Create cache directory
	if err := os.MkdirAll(m.cacheDir, 0755); err != nil {
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
	defer os.Remove(tmpPath) // Clean up on error
	
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
	return os.Rename(tmpPath, destPath)
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

// InitializeProviders initializes default OS providers
func InitializeProviders(manager *ImageManager) {
	// Register DietPi provider
	manager.RegisterProvider("dietpi", NewDietPiProvider())
	
	// Register Raspberry Pi OS provider
	manager.RegisterProvider("rpi", NewRaspberryPiOSProvider())
	manager.RegisterProvider("raspios", NewRaspberryPiOSProvider())
	
	// Future providers can be added here:
	// manager.RegisterProvider("debian", NewDebianProvider())
	// manager.RegisterProvider("ubuntu", NewUbuntuProvider())
	// manager.RegisterProvider("talos", NewTalosProvider())
}