package build_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"fleetd.sh/pkg/build"
)

func TestRuntimeIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}

	tests := []struct {
		name        string
		runtimeType build.RuntimeType
		setup       func(t *testing.T) *build.BuildResult
		verify      func(t *testing.T, status *build.RuntimeStatus)
	}{
		{
			name:        "native runtime success",
			runtimeType: build.RuntimeTypeNative,
			setup: func(t *testing.T) *build.BuildResult {
				execPath := filepath.Join(t.TempDir(), "test-executable")
				require.NoError(t, os.WriteFile(execPath, []byte("#!/bin/sh\necho test\n"), 0755))
				return &build.BuildResult{
					ID: "test",
					Artifacts: []build.Artifact{{
						Type: build.ArtifactTypeExecutable,
						Path: execPath,
					}},
				}
			},
			verify: func(t *testing.T, status *build.RuntimeStatus) {
				require.NotNil(t, status)
				require.Equal(t, "running", status.State)
				require.Greater(t, status.Pid, 0)
			},
		},
		{
			name:        "oci runtime success",
			runtimeType: build.RuntimeTypeOCI,
			setup: func(t *testing.T) *build.BuildResult {
				return &build.BuildResult{
					ID: "test",
					Artifacts: []build.Artifact{{
						Type: build.ArtifactTypeOCI,
						Path: "nginx:alpine",
					}},
				}
			},
			verify: func(t *testing.T, status *build.RuntimeStatus) {
				require.NotNil(t, status)
				require.Equal(t, "running", status.State)
				require.NotEmpty(t, status.ContainerID)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Use a temp dir for the runtime
			opts := build.DefaultRuntimeOptions()
			opts.WorkDir = t.TempDir()

			// Create runtime
			runtime, err := build.NewRuntime(ctx, tt.runtimeType, opts)
			require.NoError(t, err)

			// Ensure cleanup
			defer func() {
				if err := runtime.Close(); err != nil {
					t.Logf("Warning: cleanup failed: %v", err)
				}
			}()

			// Deploy
			result := tt.setup(t)
			err = runtime.Deploy(ctx, result)
			require.NoError(t, err)

			// Verify running state
			status, err := runtime.Status(ctx)
			require.NoError(t, err)
			tt.verify(t, status)

			// Test rollback
			err = runtime.Rollback(ctx)
			require.NoError(t, err)

			// Verify stopped state
			status, err = runtime.Status(ctx)
			if tt.runtimeType == build.RuntimeTypeNative {
				require.Error(t, err)
			} else {
				if err == nil {
					require.Contains(t, []string{"exited", "dead", "removed"}, status.State)
				}
			}
		})
	}
}
