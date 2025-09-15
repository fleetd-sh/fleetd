//go:build !linux

package capability

import "syscall"

// getTotalMemory returns total system memory in bytes
func getTotalMemory() int64 {
	// Default values for non-Linux platforms
	// In production, implement platform-specific code
	return 512 * 1024 * 1024 // Default 512MB
}

// getAvailableMemory returns available system memory in bytes
func getAvailableMemory() int64 {
	return 256 * 1024 * 1024 // Default 256MB
}

// getTotalDisk returns total disk space in bytes
func getTotalDisk() int64 {
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/", &stat); err != nil {
		return 1024 * 1024 * 1024 // Default 1GB
	}
	// Works on macOS and other Unix-like systems
	return int64(stat.Blocks) * int64(stat.Bsize)
}

// getAvailableDisk returns available disk space in bytes
func getAvailableDisk() int64 {
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/", &stat); err != nil {
		return 512 * 1024 * 1024 // Default 512MB
	}
	// Works on macOS and other Unix-like systems
	return int64(stat.Bavail) * int64(stat.Bsize)
}
