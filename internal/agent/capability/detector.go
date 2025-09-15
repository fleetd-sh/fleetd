package capability

import (
	"runtime"
	"time"

	"fleetd.sh/internal/agent/storage"
)

// Tier represents device capability tier
type Tier int

const (
	TierMinimal     Tier = 3 // <64KB RAM, no storage
	TierConstrained Tier = 2 // ESP32-class, limited storage
	TierFull        Tier = 1 // Raspberry Pi, full features
)

// DeviceCapability describes device capabilities
type DeviceCapability struct {
	Tier          Tier
	TotalRAM      int64
	AvailableRAM  int64
	TotalDisk     int64
	AvailableDisk int64
	CPUCores      int
	Architecture  string
	OS            string

	// Storage capabilities
	HasSQLite          bool
	LocalStorageSize   int64
	MaxMetricsInMemory int

	// Sync capabilities
	SyncInterval       time.Duration
	BatchSize          int
	CompressionEnabled bool
	CompressionType    string

	// Network capabilities
	HasNetwork    bool
	BandwidthKbps int
	SupportsHTTP2 bool
}

// DetectCapabilities analyzes system and returns device capabilities
func DetectCapabilities() *DeviceCapability {
	cap := &DeviceCapability{
		OS:           runtime.GOOS,
		Architecture: runtime.GOARCH,
		CPUCores:     runtime.NumCPU(),
		HasNetwork:   true, // Assume network available by default
	}

	// Get memory info
	cap.TotalRAM = getTotalMemory()
	cap.AvailableRAM = getAvailableMemory()

	// Get disk info
	cap.TotalDisk = getTotalDisk()
	cap.AvailableDisk = getAvailableDisk()

	// Determine tier based on resources
	cap.determineTier()

	// Set tier-specific configurations
	cap.configureForTier()

	return cap
}

// determineTier sets the device tier based on available resources
func (cap *DeviceCapability) determineTier() {
	switch {
	case cap.TotalDisk > 1_000_000_000 && cap.TotalRAM > 512_000_000:
		// >1GB disk, >512MB RAM
		cap.Tier = TierFull

	case cap.TotalDisk > 10_000_000 && cap.TotalRAM > 64_000_000:
		// >10MB disk, >64MB RAM
		cap.Tier = TierConstrained

	default:
		// Minimal resources
		cap.Tier = TierMinimal
	}
}

// configureForTier sets configuration based on device tier
func (cap *DeviceCapability) configureForTier() {
	switch cap.Tier {
	case TierFull:
		cap.HasSQLite = true
		cap.LocalStorageSize = 100_000_000 // 100MB
		cap.MaxMetricsInMemory = 10000
		cap.SyncInterval = 5 * time.Minute
		cap.BatchSize = 1000
		cap.CompressionEnabled = true
		cap.CompressionType = "zstd"
		cap.SupportsHTTP2 = true

	case TierConstrained:
		cap.HasSQLite = true
		cap.LocalStorageSize = 5_000_000 // 5MB
		cap.MaxMetricsInMemory = 1000
		cap.SyncInterval = 1 * time.Minute
		cap.BatchSize = 100
		cap.CompressionEnabled = true
		cap.CompressionType = "gzip"
		cap.SupportsHTTP2 = false

	case TierMinimal:
		cap.HasSQLite = false
		cap.LocalStorageSize = 0
		cap.MaxMetricsInMemory = 100
		cap.SyncInterval = 10 * time.Second
		cap.BatchSize = 10
		cap.CompressionEnabled = false
		cap.CompressionType = "none"
		cap.SupportsHTTP2 = false
	}
}

// CreateStorage creates appropriate storage based on capabilities
func (cap *DeviceCapability) CreateStorage(dataPath string) (storage.DeviceStorage, error) {
	if !cap.HasSQLite {
		return storage.NewMemoryStorage(cap.MaxMetricsInMemory), nil
	}

	dbPath := dataPath + "/metrics.db"
	return storage.NewSQLiteStorage(
		dbPath,
		storage.WithMaxSize(cap.LocalStorageSize),
		storage.WithRetention(24*time.Hour*7), // 7 days
		storage.WithMaxMetrics(cap.MaxMetricsInMemory),
	)
}

// GetSyncConfig returns sync configuration for the device
func (cap *DeviceCapability) GetSyncConfig() SyncConfig {
	return SyncConfig{
		Interval:           cap.SyncInterval,
		BatchSize:          cap.BatchSize,
		CompressionEnabled: cap.CompressionEnabled,
		CompressionType:    cap.CompressionType,
		MaxRetries:         3,
		BackoffMultiplier:  2,
		MaxBackoff:         30 * time.Minute,
		SupportsHTTP2:      cap.SupportsHTTP2,
	}
}

// SyncConfig contains synchronization configuration
type SyncConfig struct {
	Interval           time.Duration
	BatchSize          int
	CompressionEnabled bool
	CompressionType    string
	MaxRetries         int
	BackoffMultiplier  int
	MaxBackoff         time.Duration
	SupportsHTTP2      bool
}

// Platform-specific implementations of memory and disk functions
// are in detector_linux.go and detector_other.go

// StorageRecommendation provides storage recommendations
type StorageRecommendation struct {
	MaxDatabaseSize    int64
	MaxMetricsRetained int
	RetentionDays      int
	BufferSize         int
	FlushInterval      time.Duration
}

// GetStorageRecommendation returns recommended storage settings
func (cap *DeviceCapability) GetStorageRecommendation() StorageRecommendation {
	rec := StorageRecommendation{}

	// Base recommendations on available disk (use 10% max)
	availableForStorage := cap.AvailableDisk / 10

	switch cap.Tier {
	case TierFull:
		rec.MaxDatabaseSize = min(availableForStorage, 500_000_000) // Max 500MB
		rec.MaxMetricsRetained = 1_000_000
		rec.RetentionDays = 7
		rec.BufferSize = 10000
		rec.FlushInterval = 5 * time.Second

	case TierConstrained:
		rec.MaxDatabaseSize = min(availableForStorage, 50_000_000) // Max 50MB
		rec.MaxMetricsRetained = 100_000
		rec.RetentionDays = 1
		rec.BufferSize = 1000
		rec.FlushInterval = 10 * time.Second

	case TierMinimal:
		rec.MaxDatabaseSize = 0 // Memory only
		rec.MaxMetricsRetained = 100
		rec.RetentionDays = 0
		rec.BufferSize = 10
		rec.FlushInterval = 30 * time.Second
	}

	return rec
}

func min(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

// CanRunService checks if device can run a specific service
func (cap *DeviceCapability) CanRunService(service string) bool {
	switch service {
	case "sqlite":
		return cap.HasSQLite
	case "compression":
		return cap.CompressionEnabled
	case "http2":
		return cap.SupportsHTTP2
	case "local-analytics":
		return cap.Tier == TierFull
	case "edge-compute":
		return cap.Tier == TierFull && cap.CPUCores >= 2
	default:
		return false
	}
}

// GetResourceLimits returns resource limits for the device
func (cap *DeviceCapability) GetResourceLimits() ResourceLimits {
	limits := ResourceLimits{}

	switch cap.Tier {
	case TierFull:
		limits.MaxCPUPercent = 25     // Use up to 25% CPU
		limits.MaxMemoryMB = 256      // Use up to 256MB RAM
		limits.MaxDiskIOps = 1000     // IOPS limit
		limits.MaxNetworkKbps = 10000 // 10 Mbps

	case TierConstrained:
		limits.MaxCPUPercent = 10    // Use up to 10% CPU
		limits.MaxMemoryMB = 32      // Use up to 32MB RAM
		limits.MaxDiskIOps = 100     // IOPS limit
		limits.MaxNetworkKbps = 1000 // 1 Mbps

	case TierMinimal:
		limits.MaxCPUPercent = 5    // Use up to 5% CPU
		limits.MaxMemoryMB = 8      // Use up to 8MB RAM
		limits.MaxDiskIOps = 10     // Minimal disk I/O
		limits.MaxNetworkKbps = 100 // 100 Kbps
	}

	return limits
}

// ResourceLimits defines resource usage limits
type ResourceLimits struct {
	MaxCPUPercent  int
	MaxMemoryMB    int
	MaxDiskIOps    int
	MaxNetworkKbps int
}
