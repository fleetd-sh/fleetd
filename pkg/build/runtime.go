package build

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type RuntimeType string

const (
	RuntimeTypeNative  RuntimeType = "native"
	RuntimeTypeOCI     RuntimeType = "oci"
	RuntimeTypeSystemd RuntimeType = "systemd"
)

type Runtime interface {
	Deploy(ctx context.Context, result *BuildResult) error
	Status(ctx context.Context) (*RuntimeStatus, error)
	Rollback(ctx context.Context) error
	Close() error
}

type RuntimeConfig struct {
	Type        RuntimeType
	Resources   ResourceLimits
	AutoRestart bool
	Monitoring  *MonitoringConfig // nil means no monitoring
}

type ResourceLimits struct {
	MemoryBytes uint64
	CPUCores    float64
}

type MonitoringConfig struct {
	Port     int
	Path     string
	Interval time.Duration
}

type RuntimeStatus struct {
	State       string
	Pid         int
	Memory      uint64
	CPU         float64
	Restarts    int
	LastError   string
	ContainerID string
	StartTime   time.Time
}

// RollbackState tracks the state of a rollback operation
type RollbackState string

const (
	RollbackStatePending  RollbackState = "pending"
	RollbackStateStarted  RollbackState = "started"
	RollbackStateShutdown RollbackState = "shutdown"
	RollbackStateCleanup  RollbackState = "cleanup"
	RollbackStateComplete RollbackState = "complete"
	RollbackStateFailed   RollbackState = "failed"
)

// RollbackOptions configures how the rollback should be performed
type RollbackOptions struct {
	Force         bool          // Force immediate shutdown
	Timeout       time.Duration // Maximum time to wait for graceful shutdown
	CleanupFiles  bool          // Remove associated files
	KeepLogs      bool          // Preserve log files
	CreateBackup  bool          // Create backup before rollback
	HealthCheck   bool          // Verify health after rollback
	NotifyMetrics bool          // Send metrics about rollback
}

// RollbackResult provides detailed information about the rollback operation
type RollbackResult struct {
	Success    bool
	Error      error
	State      RollbackState
	StartTime  time.Time
	EndTime    time.Time
	BackupPath string
	Metrics    *RollbackMetrics
}

// RollbackMetrics captures performance metrics during rollback
type RollbackMetrics struct {
	ShutdownDuration time.Duration
	CleanupDuration  time.Duration
	ResourcesFreed   ResourceStats
}

// ResourceStats tracks resource usage
type ResourceStats struct {
	DiskSpaceFreed    int64
	MemoryFreed       int64
	ConnectionsClosed int
	FilesRemoved      int
}

// DefaultRollbackOptions provides sensible defaults
func DefaultRollbackOptions() *RollbackOptions {
	return &RollbackOptions{
		Force:         false,
		Timeout:       30 * time.Second,
		CleanupFiles:  true,
		KeepLogs:      true,
		CreateBackup:  true,
		HealthCheck:   true,
		NotifyMetrics: true,
	}
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("failed to copy file contents: %w", err)
	}

	// Copy permissions
	srcInfo, err := srcFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to get source file info: %w", err)
	}

	if err := os.Chmod(dst, srcInfo.Mode()); err != nil {
		return fmt.Errorf("failed to set file permissions: %w", err)
	}

	return nil
}

// isExecutable checks if a file is executable
func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.Mode()&0111 != 0
}

// RuntimeOptions configures how the runtime operates
type RuntimeOptions struct {
	// Rollback options
	Force         bool          // Force immediate shutdown
	Timeout       time.Duration // Maximum time to wait for graceful shutdown
	CleanupFiles  bool          // Remove associated files
	KeepLogs      bool          // Preserve log files
	CreateBackup  bool          // Create backup before rollback
	HealthCheck   bool          // Verify health after rollback
	NotifyMetrics bool          // Send metrics about rollback

	// Runtime specific options
	WorkDir    string            // Working directory for the runtime
	EnvVars    map[string]string // Environment variables
	Labels     map[string]string // Labels/tags for the runtime
	LogLevel   string            // Logging verbosity
	MaxRetries int               // Maximum number of retry attempts
}

// DefaultRuntimeOptions provides sensible defaults for runtime options
func DefaultRuntimeOptions() *RuntimeOptions {
	return &RuntimeOptions{
		Force:         false,
		Timeout:       30 * time.Second,
		CleanupFiles:  true,
		KeepLogs:      true,
		CreateBackup:  true,
		HealthCheck:   true,
		NotifyMetrics: true,
		LogLevel:      "info",
		MaxRetries:    3,
	}
}

// Shared utility functions
func saveJSON(path string, data interface{}) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(data); err != nil {
		return fmt.Errorf("failed to encode JSON: %w", err)
	}

	return nil
}

func createTarArchive(srcDir, destPath string) error {
	file, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create tar file: %w", err)
	}
	defer file.Close()

	gw := gzip.NewWriter(file)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		header, err := tar.FileInfoHeader(info, info.Name())
		if err != nil {
			return fmt.Errorf("failed to create tar header: %w", err)
		}

		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return fmt.Errorf("failed to get relative path: %w", err)
		}
		header.Name = relPath

		if err := tw.WriteHeader(header); err != nil {
			return fmt.Errorf("failed to write tar header: %w", err)
		}

		if !info.Mode().IsRegular() {
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("failed to open file: %w", err)
		}
		defer file.Close()

		if _, err := io.Copy(tw, file); err != nil {
			return fmt.Errorf("failed to write file to tar: %w", err)
		}

		return nil
	})
}

func getDiskUsage(path string) (int64, error) {
	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size, err
}

func cleanupFiles(dir string) (int, error) {
	var count int
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			if err := os.Remove(path); err != nil {
				return err
			}
			count++
		}
		return nil
	})
	return count, err
}

func NewRuntime(ctx context.Context, runtimeType RuntimeType, opts *RuntimeOptions) (Runtime, error) {
	switch runtimeType {
	case RuntimeTypeNative:
		return NewNativeRuntime(opts), nil
	case RuntimeTypeOCI:
		runtime, err := NewOCIRuntime(opts)
		if err != nil {
			return nil, err
		}
		return runtime, nil
	case RuntimeTypeSystemd:
		runtime, err := NewSystemdRuntime(ctx, opts)
		if err != nil {
			return nil, err
		}
		return runtime, nil
	default:
		return nil, fmt.Errorf("unsupported runtime type: %s", runtimeType)
	}
}

func ValidateRuntimeOptions(opts *RuntimeOptions) error {
	// Nil options are valid - defaults will be used
	if opts == nil {
		return nil
	}

	// Validate timeout
	if opts.Timeout < 0 {
		return fmt.Errorf("timeout must be non-negative, got %v", opts.Timeout)
	}

	// Validate max retries
	if opts.MaxRetries < 0 {
		return fmt.Errorf("max retries must be non-negative, got %d", opts.MaxRetries)
	}

	// Validate log level
	switch strings.ToLower(opts.LogLevel) {
	case "debug", "info", "warn", "error":
		// Valid log levels
	default:
		return fmt.Errorf("invalid log level: %s, must be one of: debug, info, warn, error", opts.LogLevel)
	}

	// Validate working directory if specified
	if opts.WorkDir != "" {
		if !filepath.IsAbs(opts.WorkDir) {
			return fmt.Errorf("working directory must be an absolute path: %s", opts.WorkDir)
		}
		// Check if directory exists and is accessible
		if _, err := os.Stat(opts.WorkDir); err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("working directory does not exist: %s", opts.WorkDir)
			}
			return fmt.Errorf("working directory is not accessible: %s", opts.WorkDir)
		}
	}

	return nil
}
