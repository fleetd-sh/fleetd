package build

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Artifact type enum
type ArtifactType string

const (
	ArtifactTypeExecutable ArtifactType = "executable"
	ArtifactTypeOCI        ArtifactType = "oci"
)

type Artifact struct {
	Path     string
	Type     ArtifactType
	Checksum string
	Metadata map[string]string
}

type ArtifactInfo struct {
	URL    string
	Digest string
	Type   ArtifactType
}

type ArtifactMetadata struct {
	BuildID      string
	Version      string
	CreatedAt    time.Time
	Files        []ArtifactFile
	Dependencies map[string]string
	Config       map[string]string
	Checksums    map[string]string
}

type ArtifactFile struct {
	Path       string
	Size       int64
	Mode       os.FileMode
	Executable bool
	Checksum   string
}

type BuildStatus string

const (
	BuildStatusPending BuildStatus = "pending"
	BuildStatusRunning BuildStatus = "running"
	BuildStatusSuccess BuildStatus = "success"
	BuildStatusFailed  BuildStatus = "failed"
)

type ArtifactManager struct {
	baseDir     string
	maxAge      time.Duration
	compression bool
}

func NewArtifactManager(baseDir string, opts ...ArtifactOption) (*ArtifactManager, error) {
	am := &ArtifactManager{
		baseDir:     baseDir,
		maxAge:      24 * time.Hour,
		compression: true,
	}

	for _, opt := range opts {
		opt(am)
	}

	return am, os.MkdirAll(baseDir, 0755)
}

type ArtifactOption func(*ArtifactManager)

func WithMaxAge(d time.Duration) ArtifactOption {
	return func(am *ArtifactManager) {
		am.maxAge = d
	}
}

func WithCompression(enabled bool) ArtifactOption {
	return func(am *ArtifactManager) {
		am.compression = enabled
	}
}

func (am *ArtifactManager) Package(ctx context.Context, buildDir string, spec *BuildSpec) (*ArtifactInfo, error) {
	buildID := generateBuildID()
	artifactDir := filepath.Join(am.baseDir, buildID)

	if err := os.MkdirAll(artifactDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create artifact directory: %w", err)
	}

	// Create metadata
	metadata := &ArtifactMetadata{
		BuildID:   buildID,
		Version:   spec.Version,
		CreatedAt: time.Now(),
		Files:     make([]ArtifactFile, 0),
		Checksums: make(map[string]string),
	}

	// Walk build directory and collect files
	err := filepath.Walk(buildDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(buildDir, path)
		if err != nil {
			return fmt.Errorf("failed to get relative path: %w", err)
		}

		checksum, err := calculateChecksum(path)
		if err != nil {
			return fmt.Errorf("failed to calculate checksum: %w", err)
		}

		metadata.Files = append(metadata.Files, ArtifactFile{
			Path:       relPath,
			Size:       info.Size(),
			Mode:       info.Mode(),
			Executable: info.Mode()&0111 != 0,
			Checksum:   checksum,
		})

		// Copy file to artifact directory
		destPath := filepath.Join(artifactDir, relPath)
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return fmt.Errorf("failed to create directories: %w", err)
		}

		if err := copyFile(path, destPath); err != nil {
			return fmt.Errorf("failed to copy file: %w", err)
		}

		return nil
	})

	if err != nil {
		os.RemoveAll(artifactDir)
		return nil, fmt.Errorf("failed to collect files: %w", err)
	}

	// Write metadata
	metadataPath := filepath.Join(artifactDir, "metadata.json")
	metadataFile, err := os.Create(metadataPath)
	if err != nil {
		os.RemoveAll(artifactDir)
		return nil, fmt.Errorf("failed to create metadata file: %w", err)
	}
	defer metadataFile.Close()

	if err := json.NewEncoder(metadataFile).Encode(metadata); err != nil {
		os.RemoveAll(artifactDir)
		return nil, fmt.Errorf("failed to write metadata: %w", err)
	}

	// Create archive if compression is enabled
	var artifactPath string
	if am.compression {
		artifactPath = filepath.Join(am.baseDir, fmt.Sprintf("%s.tar.gz", buildID))
		if err := am.createArchive(artifactDir, artifactPath); err != nil {
			os.RemoveAll(artifactDir)
			return nil, fmt.Errorf("failed to create archive: %w", err)
		}
		os.RemoveAll(artifactDir)
	} else {
		artifactPath = artifactDir
	}

	return &ArtifactInfo{
		URL:    artifactPath,
		Digest: metadata.Checksums["metadata.json"],
		Type:   "artifact",
	}, nil
}

func (am *ArtifactManager) createArchive(sourceDir, destPath string) error {
	f, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create archive file: %w", err)
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	return filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return fmt.Errorf("failed to get relative path: %w", err)
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return fmt.Errorf("failed to create tar header: %w", err)
		}
		header.Name = relPath

		if err := tw.WriteHeader(header); err != nil {
			return fmt.Errorf("failed to write tar header: %w", err)
		}

		if !info.IsDir() {
			file, err := os.Open(path)
			if err != nil {
				return fmt.Errorf("failed to open file: %w", err)
			}
			defer file.Close()

			if _, err := io.Copy(tw, file); err != nil {
				return fmt.Errorf("failed to write file to tar: %w", err)
			}
		}

		return nil
	})
}

// downloadArtifact downloads a build artifact to a local path
func downloadArtifact(ctx context.Context, artifact ArtifactInfo) (string, error) {
	resp, err := http.Get(artifact.URL)
	if err != nil {
		return "", fmt.Errorf("failed to download artifact: %w", err)
	}
	defer resp.Body.Close()

	// Create destination directory
	destDir := filepath.Join(os.TempDir(), "fleetd", "artifacts")
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create artifact directory: %w", err)
	}

	// Create artifact file
	destPath := filepath.Join(destDir, fmt.Sprintf("%s-%d", filepath.Base(artifact.URL), time.Now().Unix()))
	f, err := os.Create(destPath)
	if err != nil {
		return "", fmt.Errorf("failed to create artifact file: %w", err)
	}
	defer f.Close()

	// Copy and calculate checksum
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(f, h), resp.Body); err != nil {
		return "", fmt.Errorf("failed to write artifact: %w", err)
	}

	// Verify checksum if provided
	if artifact.Digest != "" {
		sum := hex.EncodeToString(h.Sum(nil))
		if sum != artifact.Digest {
			os.Remove(destPath)
			return "", fmt.Errorf("checksum mismatch: expected %s, got %s", artifact.Digest, sum)
		}
	}

	return destPath, nil
}

// calculateChecksum calculates SHA256 checksum of a file
func calculateChecksum(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("failed to calculate hash: %w", err)
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}
