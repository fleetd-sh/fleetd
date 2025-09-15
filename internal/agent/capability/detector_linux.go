//go:build linux

package capability

import "syscall"

// getTotalMemory returns total system memory in bytes
func getTotalMemory() int64 {
	var stat syscall.Sysinfo_t
	if err := syscall.Sysinfo(&stat); err != nil {
		return 512 * 1024 * 1024 // Default 512MB
	}
	return int64(stat.Totalram) * int64(stat.Unit)
}

// getAvailableMemory returns available system memory in bytes
func getAvailableMemory() int64 {
	var stat syscall.Sysinfo_t
	if err := syscall.Sysinfo(&stat); err != nil {
		return 256 * 1024 * 1024 // Default 256MB
	}
	return int64(stat.Freeram) * int64(stat.Unit)
}

// getTotalDisk returns total disk space in bytes
func getTotalDisk() int64 {
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/", &stat); err != nil {
		return 1024 * 1024 * 1024 // Default 1GB
	}
	return int64(stat.Blocks) * int64(stat.Bsize)
}

// getAvailableDisk returns available disk space in bytes
func getAvailableDisk() int64 {
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/", &stat); err != nil {
		return 512 * 1024 * 1024 // Default 512MB
	}
	return int64(stat.Bavail) * int64(stat.Bsize)
}
