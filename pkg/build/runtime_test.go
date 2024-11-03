package build

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/docker/docker/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"fleetd.sh/internal/testutil/containers"
)

func init() {
	// Get absolute path to testdata directory
	wd, err := os.Getwd()
	if err != nil {
		panic(fmt.Sprintf("failed to get working directory: %v", err))
	}

	testDataDir := filepath.Join(wd, "testdata")
	testExecutablePath := filepath.Join(testDataDir, "dummy-executable")

	// Create testdata directory if it doesn't exist
	if err := os.MkdirAll(testDataDir, 0755); err != nil {
		panic(fmt.Sprintf("failed to create testdata directory: %v", err))
	}

	// Create a simple test executable
	content := []byte(`#!/bin/sh
while true; do
    echo "Running test executable"
    sleep 1
done
`)
	if err := os.WriteFile(testExecutablePath, content, 0755); err != nil {
		panic(fmt.Sprintf("failed to create test executable: %v", err))
	}

	// Verify the file exists and is executable
	info, err := os.Stat(testExecutablePath)
	if err != nil {
		panic(fmt.Sprintf("failed to stat test executable: %v", err))
	}
	if info.Mode()&0111 == 0 {
		if err := os.Chmod(testExecutablePath, 0755); err != nil {
			panic(fmt.Sprintf("failed to make file executable: %v", err))
		}
	}
}

func TestDefaultRuntimeOptions(t *testing.T) {
	opts := DefaultRuntimeOptions()
	assert.Equal(t, 30*time.Second, opts.Timeout)
	assert.True(t, opts.CleanupFiles)
	assert.True(t, opts.KeepLogs)
	assert.Equal(t, "info", opts.LogLevel)
	assert.Equal(t, 3, opts.MaxRetries)
}

func TestRuntimeOptionsValidation(t *testing.T) {
	tests := []struct {
		name    string
		opts    *RuntimeOptions
		wantErr bool
	}{
		{
			name:    "nil options uses defaults",
			opts:    nil,
			wantErr: false,
		},
		{
			name: "valid options",
			opts: &RuntimeOptions{
				Timeout:    30 * time.Second,
				MaxRetries: 3,
				LogLevel:   "info",
			},
			wantErr: false,
		},
		{
			name: "invalid timeout",
			opts: &RuntimeOptions{
				Timeout:  -1 * time.Second,
				LogLevel: "info",
			},
			wantErr: true,
		},
		{
			name: "invalid max retries",
			opts: &RuntimeOptions{
				MaxRetries: -1,
				LogLevel:   "info",
			},
			wantErr: true,
		},
		{
			name: "zero timeout is valid",
			opts: &RuntimeOptions{
				Timeout:  0,
				LogLevel: "info",
			},
			wantErr: false,
		},
		{
			name: "zero max retries is valid",
			opts: &RuntimeOptions{
				MaxRetries: 0,
				LogLevel:   "info",
			},
			wantErr: false,
		},
		{
			name: "invalid log level",
			opts: &RuntimeOptions{
				LogLevel: "invalid",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRuntimeOptions(tt.opts)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestNewRuntime(t *testing.T) {
	tests := []struct {
		name        string
		runtimeType RuntimeType
		opts        *RuntimeOptions
		skipOnOS    string
		skipOnEnv   string
		wantErr     bool
	}{
		{
			name:        "native runtime",
			runtimeType: RuntimeTypeNative,
			opts:        DefaultRuntimeOptions(),
			wantErr:     false,
		},
		{
			name:        "oci runtime",
			runtimeType: RuntimeTypeOCI,
			opts:        DefaultRuntimeOptions(),
			wantErr:     false,
		},
		{
			name:        "systemd runtime",
			runtimeType: RuntimeTypeSystemd,
			opts:        DefaultRuntimeOptions(),
			skipOnOS:    "darwin",
			skipOnEnv:   "CI",
			wantErr:     false,
		},
		{
			name:        "invalid runtime type",
			runtimeType: "invalid",
			opts:        DefaultRuntimeOptions(),
			wantErr:     true,
		},
		{
			name:        "nil options",
			runtimeType: RuntimeTypeNative,
			opts:        nil,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipOnOS != "" && runtime.GOOS == tt.skipOnOS {
				t.Skipf("Skipping on %s", tt.skipOnOS)
			} else if tt.skipOnEnv != "" && os.Getenv(tt.skipOnEnv) != "" {
				t.Skipf("Skipping on %s", tt.skipOnEnv)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			runtime, err := NewRuntime(ctx, tt.runtimeType, tt.opts)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, runtime)
			} else {
				if assert.NoError(t, err) {
					assert.NotNil(t, runtime)
					assert.Implements(t, (*Runtime)(nil), runtime)
				}
			}
		})
	}
}

func TestRuntimeStatus(t *testing.T) {
	ctx := context.Background()

	// Start nginx container for OCI tests
	nginxContainer, err := containers.NewNginxContainer(ctx)
	if err != nil {
		t.Skipf("Skipping OCI tests: failed to start nginx container: %v", err)
	}
	t.Cleanup(func() {
		if err := nginxContainer.Terminate(ctx); err != nil {
			t.Logf("failed to terminate nginx container: %v", err)
		}
	})

	// Get absolute path to test executable
	wd, err := os.Getwd()
	require.NoError(t, err)
	execPath := filepath.Join(wd, "testdata", "dummy-executable")

	// Verify test executable exists and is executable
	info, err := os.Stat(execPath)
	require.NoError(t, err)
	require.True(t, info.Mode()&0111 != 0, "File must be executable")

	tests := []struct {
		name        string
		runtimeType RuntimeType
		artifact    Artifact
		config      map[string]string
		wantState   string
		skipOnOS    string
		wantErr     bool
	}{
		{
			name:        "native runtime executable",
			runtimeType: RuntimeTypeNative,
			artifact: Artifact{
				Type: ArtifactTypeExecutable,
				Path: execPath,
			},
			wantState: "running",
			wantErr:   false,
		},
		{
			name:        "oci runtime container",
			runtimeType: RuntimeTypeOCI,
			artifact: Artifact{
				Type: ArtifactTypeOCI,
				Path: "nginx:alpine",
			},
			config: map[string]string{
				"ports": `["80:80"]`,
				"healthcheck": `{
					"test": ["CMD-SHELL", "wget -q --spider http://localhost/ || exit 1"],
					"interval": "1s",
					"timeout": "1s",
					"retries": 3
				}`,
			},
			wantState: "running",
			wantErr:   false,
		},
		{
			name:        "oci runtime with invalid image",
			runtimeType: RuntimeTypeOCI,
			artifact: Artifact{
				Type: ArtifactTypeOCI,
				Path: "invalid/image:tag",
			},
			wantState: "",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipOnOS != "" && runtime.GOOS == tt.skipOnOS {
				t.Skipf("Skipping on %s", tt.skipOnOS)
			}

			// Create temp directory for test
			tempDir, err := os.MkdirTemp("", "fleetd-test-*")
			require.NoError(t, err)
			defer os.RemoveAll(tempDir)

			// Create runtime with temp directory
			opts := DefaultRuntimeOptions()
			opts.WorkDir = tempDir

			ctx := context.Background()
			runtime, err := NewRuntime(ctx, tt.runtimeType, opts)
			require.NoError(t, err)
			defer runtime.Close()

			// Test status before deployment
			status, err := runtime.Status(ctx)
			assert.Error(t, err)
			assert.Nil(t, status)

			// Deploy
			result := &BuildResult{
				ID: "test",
				Spec: BuildSpec{
					Version: "v1.0.0",
					Config:  tt.config,
				},
				Artifacts: []Artifact{tt.artifact},
			}

			err = runtime.Deploy(ctx, result)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)

			// Add a longer delay to ensure container is running and healthcheck passes
			time.Sleep(5 * time.Second)

			// Test status after deployment
			status, err = runtime.Status(ctx)
			assert.NoError(t, err)
			assert.NotNil(t, status)
			if tt.runtimeType == RuntimeTypeOCI {
				assert.NotEmpty(t, status.ContainerID)
			} else {
				assert.NotEmpty(t, status.Pid)
			}
			assert.Equal(t, tt.wantState, status.State)

			// Cleanup
			err = runtime.Close()
			if err != nil {
				t.Logf("Warning: cleanup failed: %v", err)
			}

			// Wait for process/container to stop with timeout
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			stopped := false
			for !stopped {
				select {
				case <-ctx.Done():
					t.Fatalf("Timeout waiting for %s to stop", tt.runtimeType)
				case <-time.After(100 * time.Millisecond):
					status, err = runtime.Status(ctx)
					if err != nil {
						// Any error likely means the process/container is gone
						stopped = true
						continue
					}
					if status == nil || (tt.runtimeType == RuntimeTypeOCI &&
						(status.State == "stopped" || status.State == "exited")) {
						stopped = true
					}
				}
			}

			// Final verification - we expect either an error or a stopped state
			status, err = runtime.Status(ctx)
			if err == nil && status != nil {
				if tt.runtimeType == RuntimeTypeOCI {
					assert.Contains(t, []string{"stopped", "exited"}, status.State)
				} else {
					t.Error("Expected error for stopped native process")
				}
			}
		})
	}
}

// Helper function to pull test images
func pullTestImage(image string) error {
	cmd := exec.Command("docker", "pull", image)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func TestRuntimeAbruptTermination(t *testing.T) {
	// Get absolute path to test executable
	wd, err := os.Getwd()
	require.NoError(t, err)
	execPath := filepath.Join(wd, "testdata", "dummy-executable")

	tests := []struct {
		name        string
		runtimeType RuntimeType
		artifact    Artifact
		killFunc    func(t *testing.T, status *RuntimeStatus) error
	}{
		{
			name:        "kill native process",
			runtimeType: RuntimeTypeNative,
			artifact: Artifact{
				Type: ArtifactTypeExecutable,
				Path: execPath,
			},
			killFunc: func(t *testing.T, status *RuntimeStatus) error {
				proc, err := os.FindProcess(status.Pid)
				if err != nil {
					return err
				}
				return proc.Signal(os.Kill)
			},
		},
		{
			name:        "kill container",
			runtimeType: RuntimeTypeOCI,
			artifact: Artifact{
				Type: ArtifactTypeOCI,
				Path: "nginx:alpine",
			},
			killFunc: func(t *testing.T, status *RuntimeStatus) error {
				cli, err := client.NewClientWithOpts(client.FromEnv)
				if err != nil {
					return err
				}
				defer cli.Close()
				return cli.ContainerKill(context.Background(), status.ContainerID, "SIGKILL")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory for test
			tempDir, err := os.MkdirTemp("", "fleetd-test-*")
			require.NoError(t, err)
			defer os.RemoveAll(tempDir)

			// Create runtime
			opts := DefaultRuntimeOptions()
			opts.WorkDir = tempDir
			ctx := context.Background()
			runtime, err := NewRuntime(ctx, tt.runtimeType, opts)
			require.NoError(t, err)
			defer runtime.Close()

			// Deploy
			result := &BuildResult{
				ID: "test",
				Spec: BuildSpec{
					Version: "v1.0.0",
				},
				Artifacts: []Artifact{tt.artifact},
			}
			err = runtime.Deploy(ctx, result)
			require.NoError(t, err)

			// Wait for process/container to be running
			var status *RuntimeStatus
			require.Eventually(t, func() bool {
				status, err = runtime.Status(ctx)
				return err == nil && status != nil && status.State == "running"
			}, 5*time.Second, 100*time.Millisecond, "Process/container did not start")

			// Kill the process/container abruptly
			err = tt.killFunc(t, status)
			require.NoError(t, err)

			// Verify runtime detects the termination
			require.Eventually(t, func() bool {
				status, err = runtime.Status(ctx)

				if tt.runtimeType == RuntimeTypeNative {
					// For native processes, we expect an error when the process is gone
					return err != nil
				}

				// For OCI containers, we expect either an error or an exited/dead state
				return err != nil || (status != nil &&
					(status.State == "exited" || status.State == "dead"))
			}, 5*time.Second, 100*time.Millisecond, "Failed to detect termination")

			// Cleanup should still work
			err = runtime.Close()
			assert.NoError(t, err)

			// Final status check should show no running process/container
			status, err = runtime.Status(ctx)
			if tt.runtimeType == RuntimeTypeNative {
				assert.Error(t, err, "Expected error for killed native process")
			} else {
				if err == nil {
					assert.Contains(t, []string{"exited", "dead", "removed"}, status.State,
						"Container should be in terminal state")
				}
			}
		})
	}
}
