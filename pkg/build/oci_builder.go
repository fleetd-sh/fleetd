package build

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
)

type OCIBuilder struct {
	client       *client.Client
	registry     string
	registryAuth string
}

func NewOCIBuilder(registry string, registryAuth string) (*OCIBuilder, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}
	return &OCIBuilder{
		client:       cli,
		registry:     registry,
		registryAuth: registryAuth,
	}, nil
}

func (b *OCIBuilder) Build(ctx context.Context, spec *BuildSpec) (*BuildResult, error) {
	buildID := generateBuildID()
	result := &BuildResult{
		ID:        buildID,
		Spec:      *spec,
		Status:    BuildStatusRunning,
		StartTime: time.Now(),
	}

	buildDir, err := os.MkdirTemp("", fmt.Sprintf("build-%s-*", buildID))
	if err != nil {
		return nil, fmt.Errorf("failed to create build directory: %w", err)
	}
	defer os.RemoveAll(buildDir)

	// Create Dockerfile
	if err := b.createDockerfile(buildDir, spec); err != nil {
		result.Status = BuildStatusFailed
		result.Error = err
		return result, err
	}

	// Build image
	tag := fmt.Sprintf("%s/%s:%s", b.registry, buildID, "latest")
	buildDirReader, err := os.Open(buildDir)
	if err != nil {
		result.Status = BuildStatusFailed
		result.Error = err
		return result, fmt.Errorf("failed to open build directory: %w", err)
	}
	buildResponse, err := b.client.ImageBuild(ctx, buildDirReader, types.ImageBuildOptions{
		Dockerfile: "Dockerfile",
		Tags:       []string{tag},
		Remove:     true,
		Context:    buildDirReader,
	})
	if err != nil {
		result.Status = BuildStatusFailed
		result.Error = err
		return result, fmt.Errorf("failed to build image: %w", err)
	}
	defer buildResponse.Body.Close()

	result.Status = BuildStatusSuccess
	result.Artifacts = []Artifact{{
		Type:     "oci",
		Path:     tag,
		Metadata: map[string]string{"registry": b.registry},
	}}
	result.EndTime = time.Now()
	return result, nil
}

func (b *OCIBuilder) createDockerfile(buildDir string, spec *BuildSpec) error {
	dockerfile := []string{
		"FROM alpine:latest", // Default base image
		"WORKDIR /app",
	}

	// Add build commands
	for _, cmd := range spec.Commands {
		dockerfile = append(dockerfile, "RUN "+cmd)
	}

	// Add environment variables
	for k, v := range spec.Env {
		dockerfile = append(dockerfile, fmt.Sprintf("ENV %s=%s", k, v))
	}

	return os.WriteFile(
		filepath.Join(buildDir, "Dockerfile"),
		[]byte(strings.Join(dockerfile, "\n")),
		0644,
	)
}

func (b *OCIBuilder) pushImage(ctx context.Context, tag string) error {
	resp, err := b.client.ImagePush(ctx, tag, image.PushOptions{
		RegistryAuth: b.registryAuth,
	})
	if err != nil {
		return fmt.Errorf("push failed: %w", err)
	}
	defer resp.Close()

	// Monitor push progress
	decoder := json.NewDecoder(resp)
	for decoder.More() {
		var status struct{ Error string }
		if err := decoder.Decode(&status); err != nil {
			return err
		}
		if status.Error != "" {
			return fmt.Errorf("push error: %s", status.Error)
		}
	}
	return nil
}
