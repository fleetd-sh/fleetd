package build

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"
)

type NixpackConfig struct {
	Providers []string
	Name      string
	Version   string
}

type NixpackBuilder struct {
	workDir string
}

func NewNixpackBuilder() *NixpackBuilder {
	return &NixpackBuilder{
		workDir: os.TempDir(),
	}
}

func (b *NixpackBuilder) Build(ctx context.Context, spec *BuildSpec) (*BuildResult, error) {
	buildID := generateBuildID()
	buildDir, err := os.MkdirTemp(b.workDir, fmt.Sprintf("nixpack-build-%s-*", buildID))
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

	// Clone/download source
	if err := fetchSource(ctx, buildDir, &spec.Source); err != nil {
		result.Status = BuildStatusFailed
		result.Error = fmt.Errorf("failed to fetch source: %w", err)
		return result, result.Error
	}

	// Build using nixpacks
	nixpackCmd := []string{
		"nixpacks",
		"build",
		buildDir,
		"--name", spec.Config["name"],
	}

	// Add providers if specified
	if providers, ok := spec.Config["nixpack.providers"]; ok {
		nixpackCmd = append(nixpackCmd, "--provider", providers)
	}

	// Execute nixpacks build
	cmd := exec.CommandContext(ctx, nixpackCmd[0], nixpackCmd[1:]...)
	cmd.Env = append(os.Environ(), mapToEnvSlice(spec.Env)...)

	if out, err := cmd.CombinedOutput(); err != nil {
		result.Status = BuildStatusFailed
		result.Error = fmt.Errorf("nixpacks build failed: %s: %w", out, err)
		return result, result.Error
	}

	// Create artifact
	result.Status = BuildStatusSuccess
	result.Artifacts = []Artifact{
		{
			Type: "nixpack",
			Path: fmt.Sprintf("%s:%s", spec.Config["name"], spec.Version),
			Metadata: map[string]string{
				"builder": "nixpack",
				"version": spec.Version,
			},
		},
	}
	result.EndTime = time.Now()

	return result, nil
}

func (b *NixpackBuilder) Status(ctx context.Context, id string) (*BuildResult, error) {
	// Nixpack builds are synchronous, so we don't need to implement status checking
	return nil, fmt.Errorf("status checking not supported for nixpack builds")
}
