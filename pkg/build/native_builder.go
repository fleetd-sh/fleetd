package build

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type NativeBuilder struct {
	workDir string
}

func NewNativeBuilder() *NativeBuilder {
	return &NativeBuilder{
		workDir: os.TempDir(),
	}
}

func (b *NativeBuilder) Build(ctx context.Context, spec *BuildSpec) (*BuildResult, error) {
	buildID := generateBuildID()
	buildDir, err := os.MkdirTemp(b.workDir, fmt.Sprintf("build-%s-*", buildID))
	if err != nil {
		return nil, fmt.Errorf("failed to create build directory: %w", err)
	}
	defer os.RemoveAll(buildDir)

	result := &BuildResult{
		ID:        buildID,
		Spec:      *spec,
		Status:    BuildStatusRunning,
		StartTime: time.Now(),
	}

	// Fetch source
	if err := fetchSource(ctx, buildDir, &spec.Source); err != nil {
		result.Status = BuildStatusFailed
		result.Error = err
		return result, fmt.Errorf("failed to fetch source: %w", err)
	}

	// Execute build commands
	for _, cmd := range spec.Commands {
		command := exec.CommandContext(ctx, "sh", "-c", cmd)
		command.Dir = buildDir
		command.Env = append(os.Environ(), mapToEnvSlice(spec.Env)...)

		if out, err := command.CombinedOutput(); err != nil {
			result.Status = BuildStatusFailed
			result.Error = fmt.Errorf("build command failed: %s: %w", out, err)
			return result, result.Error
		}
	}

	// Package artifacts
	artifacts, err := packageArtifacts(buildDir)
	if err != nil {
		result.Status = BuildStatusFailed
		result.Error = err
		return result, fmt.Errorf("failed to package artifacts: %w", err)
	}

	result.Status = BuildStatusSuccess
	result.Artifacts = artifacts
	result.EndTime = time.Now()
	return result, nil
}

// packageBinary creates a deployable artifact from the build output
func packageBinary(buildDir string) (*ArtifactInfo, error) {
	// Find the main binary
	var binaryPath string
	err := filepath.Walk(buildDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// Skip directories and non-executable files
		if info.IsDir() || (info.Mode()&0111) == 0 {
			return nil
		}
		// Use the first executable found
		binaryPath = path
		return filepath.SkipAll
	})
	if err != nil {
		return nil, fmt.Errorf("failed to find binary: %w", err)
	}
	if binaryPath == "" {
		return nil, fmt.Errorf("no binary found in build directory")
	}

	// Create artifacts directory if it doesn't exist
	artifactsDir := filepath.Join(os.TempDir(), "fleetd", "artifacts")
	if err := os.MkdirAll(artifactsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create artifacts directory: %w", err)
	}

	// Create artifact file with timestamp
	timestamp := time.Now().Unix()
	artifactName := fmt.Sprintf("%s-%d", filepath.Base(binaryPath), timestamp)
	artifactPath := filepath.Join(artifactsDir, artifactName)

	// Copy binary to artifacts directory
	if err := copyFile(binaryPath, artifactPath); err != nil {
		return nil, fmt.Errorf("failed to copy binary: %w", err)
	}

	// Calculate checksum
	checksum, err := calculateChecksum(artifactPath)
	if err != nil {
		os.Remove(artifactPath)
		return nil, fmt.Errorf("failed to calculate checksum: %w", err)
	}

	return &ArtifactInfo{
		URL:    artifactPath,
		Digest: checksum,
		Type:   "binary",
	}, nil
}

// packageArtifacts collects all files in the build directory as artifacts
func packageArtifacts(buildDir string) ([]Artifact, error) {
	var artifacts []Artifact
	err := filepath.Walk(buildDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		checksum, err := calculateChecksum(path)
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(buildDir, path)
		if err != nil {
			return err
		}

		artifacts = append(artifacts, Artifact{
			Path:     relPath,
			Type:     "file",
			Checksum: checksum,
			Metadata: map[string]string{
				"executable": fmt.Sprintf("%t", isExecutable(path)),
				"size":       fmt.Sprintf("%d", info.Size()),
			},
		})
		return nil
	})
	return artifacts, err
}
