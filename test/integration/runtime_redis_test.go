package integration

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fleetd.sh/internal/runtime"
	"github.com/go-redis/redis/v8"
)

func TestRedisDeployment(t *testing.T) {
	if testing.Short() || os.Getenv("INTEGRATION") == "" {
		t.Skip("Skipping integration test")
	}

	// Check for required build tools
	checkBuildDependencies(t)

	// Try to find existing redis-server binary first
	var redisBinaryPath string
	if existingPath, err := exec.LookPath("redis-server"); err == nil {
		t.Logf("Using existing Redis installation: %s", existingPath)
		redisBinaryPath = existingPath
	} else {
		// Download and build Redis only if not found
		redisURL := "https://download.redis.io/redis-stable.tar.gz"
		redisBinaryPath = downloadAndBuildRedis(t, redisURL)
	}

	tmpDir := t.TempDir()
	rt, err := runtime.New(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create runtime: %v", err)
	}

	// Deploy Redis
	redisData, err := os.ReadFile(redisBinaryPath)
	if err != nil {
		t.Fatalf("Failed to read Redis binary: %v", err)
	}

	if err := rt.Deploy("redis-server", bytes.NewReader(redisData)); err != nil {
		t.Fatalf("Failed to deploy Redis: %v", err)
	}

	// Start Redis with config
	config := []string{"--port", "6380", "--save", ""} // Non-default port, disable persistence
	if err := rt.Start("redis-server", config, &runtime.Config{}); err != nil {
		t.Fatalf("Failed to start Redis: %v", err)
	}

	// Ensure cleanup
	defer func() {
		procs, err := rt.List()
		if err != nil {
			t.Errorf("Failed to list processes: %v", err)
		} else {
			t.Logf("Processes: %v", procs)
		}

		running, err := rt.IsRunning("redis-server")
		if err != nil {
			t.Errorf("Failed to check if Redis is running: %v", err)
			return
		}
		if running {
			if err := rt.Stop("redis-server"); err != nil {
				t.Errorf("Failed to stop Redis: %v", err)
			}
			time.Sleep(50 * time.Millisecond)
		}
	}()

	// Test Redis connection
	client := redis.NewClient(&redis.Options{
		Addr: "localhost:6380",
	})
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Try to ping Redis
	for i := 0; i < 10; i++ {
		if err := client.Ping(ctx).Err(); err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Test basic Redis operations
	err = client.Set(ctx, "test_key", "test_value", 0).Err()
	if err != nil {
		t.Fatalf("Failed to set Redis key: %v", err)
	}

	val, err := client.Get(ctx, "test_key").Result()
	if err != nil {
		t.Fatalf("Failed to get Redis key: %v", err)
	}
	if val != "test_value" {
		t.Errorf("Unexpected value: got %q, want %q", val, "test_value")
	}

	// Stop Redis
	if err := rt.Stop("redis-server"); err != nil {
		t.Fatalf("Failed to stop Redis: %v", err)
	}

	// Verify Redis is stopped
	time.Sleep(50 * time.Millisecond)
	running, err := rt.IsRunning("redis-server")
	if err != nil {
		t.Fatalf("Failed to check if Redis is running: %v", err)
	}
	if running {
		t.Error("Redis still running after stop command")
	}
}

// downloadAndBuildRedis downloads, extracts, and builds Redis for testing
func downloadAndBuildRedis(t *testing.T, url string) string {
	t.Helper()

	// Create temp directory for Redis
	tmpDir := t.TempDir()

	// Download Redis archive
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("Failed to download Redis: %v", err)
	}
	defer resp.Body.Close()

	// Create temporary file for the download
	archivePath := filepath.Join(tmpDir, "redis.tar.gz")
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("Failed to create archive file: %v", err)
	}
	defer f.Close()

	// Copy download to file
	if _, err := io.Copy(f, resp.Body); err != nil {
		t.Fatalf("Failed to save Redis archive: %v", err)
	}
	f.Close() // Close before extraction

	// Extract archive
	cmd := exec.Command("tar", "xzf", archivePath, "-C", tmpDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to extract Redis archive: %v\nOutput: %s", err, out)
	}

	// Find Redis directory (it will be redis-stable or redis-x.x.x)
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("Failed to read temp directory: %v", err)
	}

	var redisDir string
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "redis-") {
			redisDir = filepath.Join(tmpDir, entry.Name())
			break
		}
	}
	if redisDir == "" {
		t.Fatal("Could not find Redis directory")
	}

	// Build Redis
	cmd = exec.Command("sudo", "make", "install")
	cmd.Dir = redisDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build Redis: %v\nOutput: %s", err, out)
	}

	// Check for redis-server binary
	redisBinary := filepath.Join(redisDir, "src", "redis-server")
	if _, err := os.Stat(redisBinary); err != nil {
		t.Fatalf("Redis binary not found: %v", err)
	}

	// Verify binary is executable
	if err := exec.Command(redisBinary, "--version").Run(); err != nil {
		t.Fatalf("Redis binary verification failed: %v", err)
	}

	return redisBinary
}

// Add helper function to check if required build tools are available
func checkBuildDependencies(t *testing.T) {
	t.Helper()

	required := []string{"make", "gcc", "tar"}

	for _, cmd := range required {
		if _, err := exec.LookPath(cmd); err != nil {
			t.Skipf("Required build tool %s not found: %v", cmd, err)
		}
	}
}
