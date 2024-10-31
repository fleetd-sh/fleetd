package build

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Core types used across multiple files
type BuildStrategy string

const (
	BuildStrategyNative  BuildStrategy = "native"
	BuildStrategyOCI     BuildStrategy = "oci"
	BuildStrategyNixpack BuildStrategy = "nixpack"
)

// Core interfaces used across the package
type Builder interface {
	Build(ctx context.Context, spec *BuildSpec) (*BuildResult, error)
	Status(ctx context.Context, id string) (*BuildResult, error)
}

// Common structs used by multiple components
type BuildSpec struct {
	Source   Source
	Strategy BuildStrategy
	Runtime  RuntimeConfig
	Version  string
	Commands []string
	Env      map[string]string
	Config   map[string]string
}

type BuildResult struct {
	ID        string
	Spec      BuildSpec
	Status    BuildStatus
	Artifacts []Artifact
	StartTime time.Time
	EndTime   time.Time
	Error     error
}

// generateBuildID creates a unique build identifier
func generateBuildID() string {
	return uuid.New().String()
}
