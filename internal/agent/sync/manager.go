package sync

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	pb "fleetd.sh/gen/fleetd/v1"
	"fleetd.sh/internal/agent/capability"
	"fleetd.sh/internal/agent/storage"
	"fleetd.sh/internal/retry"
)

// Manager handles all data synchronization for the device
type Manager struct {
	// Core components
	storage    storage.DeviceStorage
	client     SyncClient
	capability *capability.DeviceCapability
	config     *SyncConfig
	logger     *slog.Logger

	// Sync state
	sequenceNum  atomic.Int64
	lastSyncTime atomic.Value // time.Time
	syncErrors   atomic.Int32
	isSyncing    atomic.Bool

	// Control channels
	triggerChan chan struct{}
	stopChan    chan struct{}
	wg          sync.WaitGroup

	// Metrics
	metrics *SyncMetrics
}

// SyncConfig contains sync configuration
type SyncConfig struct {
	DeviceID           string
	OrgID              string
	ServerURL          string
	SyncInterval       time.Duration
	BatchSize          int
	CompressionEnabled bool
	CompressionType    string
	MaxRetries         int
	InitialBackoff     time.Duration
	MaxBackoff         time.Duration
	BackoffMultiplier  float64
	OfflineQueueSize   int
}

// SyncMetrics tracks sync performance
type SyncMetrics struct {
	MetricsSynced       atomic.Int64
	LogsSynced          atomic.Int64
	BytesSent           atomic.Int64
	BytesCompressed     atomic.Int64
	SyncDuration        atomic.Int64 // nanoseconds
	FailedSyncs         atomic.Int32
	SuccessfulSyncs     atomic.Int32
	CompressionRatio    atomic.Value // float64
	LastError           atomic.Value // error
	ConsecutiveFailures atomic.Int32
}

// NewManager creates a new sync manager
func NewManager(
	storage storage.DeviceStorage,
	client SyncClient,
	cap *capability.DeviceCapability,
	config *SyncConfig,
) *Manager {
	m := &Manager{
		storage:     storage,
		client:      client,
		capability:  cap,
		config:      config,
		logger:      slog.Default().With("component", "sync-manager"),
		triggerChan: make(chan struct{}, 1),
		stopChan:    make(chan struct{}),
		metrics:     &SyncMetrics{},
	}

	// Set initial values
	m.lastSyncTime.Store(time.Now())
	m.sequenceNum.Store(time.Now().UnixNano())

	// Apply capability-based config overrides
	m.applyCapabilityConfig()

	return m
}

// Start begins the sync manager
func (m *Manager) Start(ctx context.Context) error {
	m.logger.Info("Starting sync manager",
		"device_id", m.config.DeviceID,
		"tier", m.capability.Tier,
		"interval", m.config.SyncInterval,
		"compression", m.config.CompressionType,
	)

	// Start sync worker
	m.wg.Add(1)
	go m.syncWorker(ctx)

	// Start monitor worker
	m.wg.Add(1)
	go m.monitorWorker(ctx)

	return nil
}

// Stop gracefully stops the sync manager
func (m *Manager) Stop() error {
	m.logger.Info("Stopping sync manager")

	// Trigger final sync
	m.TriggerSync()

	// Signal stop
	close(m.stopChan)

	// Wait for workers with timeout
	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		m.logger.Info("Sync manager stopped gracefully")
	case <-time.After(30 * time.Second):
		m.logger.Warn("Sync manager stop timeout")
	}

	return nil
}

// TriggerSync manually triggers a sync cycle
func (m *Manager) TriggerSync() {
	select {
	case m.triggerChan <- struct{}{}:
		m.logger.Debug("Sync triggered")
	default:
		// Already triggered
	}
}

// syncWorker is the main sync loop
func (m *Manager) syncWorker(ctx context.Context) {
	defer m.wg.Done()

	ticker := time.NewTicker(m.config.SyncInterval)
	defer ticker.Stop()

	backoff := retry.NewBackoff(retry.Config{
		MaxAttempts:    m.config.MaxRetries,
		InitialBackoff: m.config.InitialBackoff,
		MaxBackoff:     m.config.MaxBackoff,
		Multiplier:     m.config.BackoffMultiplier,
		Jitter:         true,
	})

	for {
		select {
		case <-ctx.Done():
			m.performFinalSync()
			return

		case <-m.stopChan:
			m.performFinalSync()
			return

		case <-ticker.C:
			if err := m.performSync(ctx); err != nil {
				m.handleSyncError(err, backoff)
			} else {
				backoff.Reset()
				m.metrics.ConsecutiveFailures.Store(0)
			}

		case <-m.triggerChan:
			if err := m.performSync(ctx); err != nil {
				m.handleSyncError(err, backoff)
			} else {
				backoff.Reset()
				m.metrics.ConsecutiveFailures.Store(0)
			}
		}
	}
}

// performSync executes a sync cycle
func (m *Manager) performSync(ctx context.Context) error {
	// Check if already syncing
	if !m.isSyncing.CompareAndSwap(false, true) {
		return nil // Already syncing
	}
	defer m.isSyncing.Store(false)

	startTime := time.Now()
	defer func() {
		m.metrics.SyncDuration.Store(time.Since(startTime).Nanoseconds())
		m.lastSyncTime.Store(time.Now())
	}()

	m.logger.Debug("Starting sync cycle")

	// Get storage info
	info := m.storage.GetStorageInfo()
	if info.UnsyncedMetrics == 0 {
		m.logger.Debug("No unsynced data")
		return nil
	}

	// Sync metrics
	if err := m.syncMetrics(ctx); err != nil {
		return fmt.Errorf("failed to sync metrics: %w", err)
	}

	// Sync logs if supported
	if m.capability.Tier == capability.TierFull {
		if err := m.syncLogs(ctx); err != nil {
			m.logger.Warn("Failed to sync logs", "error", err)
			// Don't fail the whole sync for logs
		}
	}

	m.metrics.SuccessfulSyncs.Add(1)
	m.logger.Debug("Sync cycle completed",
		"duration", time.Since(startTime),
		"metrics_synced", m.metrics.MetricsSynced.Load(),
	)

	return nil
}

// syncMetrics syncs metrics to the server
func (m *Manager) syncMetrics(ctx context.Context) error {
	// Get unsynced metrics
	metrics, err := m.storage.GetUnsynced(m.config.BatchSize)
	if err != nil {
		return fmt.Errorf("failed to get unsynced metrics: %w", err)
	}

	if len(metrics) == 0 {
		return nil
	}

	// Create batch
	batch := &pb.MetricsBatch{
		Metrics: make([]*pb.Metric, len(metrics)),
	}

	for i, metric := range metrics {
		batch.Metrics[i] = &pb.Metric{
			Name:      metric.Name,
			Value:     metric.Value,
			Timestamp: timestampToProto(metric.Timestamp),
			Labels:    metric.Labels,
		}
	}

	// Compress if enabled
	if m.config.CompressionEnabled {
		compressedBatch, err := m.compressBatch(batch)
		if err != nil {
			m.logger.Warn("Compression failed, sending uncompressed", "error", err)
		} else {
			batch = compressedBatch
			ratio := float64(batch.OriginalSize) / float64(len(batch.CompressedData))
			m.metrics.CompressionRatio.Store(ratio)
		}
	}

	// Create sync request
	req := &pb.SyncMetricsRequest{
		Metadata: &pb.SyncMetadata{
			DeviceId:       m.config.DeviceID,
			OrgId:          m.config.OrgID,
			SequenceNumber: m.sequenceNum.Add(1),
			ClientTime:     timestampToProto(time.Now()),
			Capability:     m.capabilityToProto(),
		},
		Batch: batch,
	}

	// Send to server
	resp, err := m.client.SyncMetrics(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to sync metrics: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("sync failed: %s", resp.ErrorMessage)
	}

	// Mark as synced
	ids := make([]int64, len(metrics))
	for i, metric := range metrics {
		ids[i] = metric.ID
	}

	if err := m.storage.MarkSynced(ids); err != nil {
		m.logger.Warn("Failed to mark metrics as synced", "error", err)
	}

	// Update metrics
	m.metrics.MetricsSynced.Add(int64(len(metrics)))
	if batch.CompressedData != nil {
		m.metrics.BytesSent.Add(int64(len(batch.CompressedData)))
		m.metrics.BytesCompressed.Add(int64(batch.OriginalSize))
	}

	// Apply config updates from server
	if resp.ConfigUpdate != nil {
		m.applyConfigUpdate(resp.ConfigUpdate)
	}

	return nil
}

// syncLogs syncs logs to the server
func (m *Manager) syncLogs(ctx context.Context) error {
	// Implementation would be similar to syncMetrics
	// but pulling from log storage/buffer
	return nil
}

// performFinalSync does a final sync before shutdown
func (m *Manager) performFinalSync() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	m.logger.Info("Performing final sync before shutdown")

	if err := m.performSync(ctx); err != nil {
		m.logger.Error("Final sync failed", "error", err)
	}
}

// handleSyncError handles sync errors with backoff
func (m *Manager) handleSyncError(err error, backoff *retry.Backoff) {
	m.metrics.FailedSyncs.Add(1)
	m.metrics.LastError.Store(err)
	m.metrics.ConsecutiveFailures.Add(1)

	m.logger.Error("Sync failed",
		"error", err,
		"consecutive_failures", m.metrics.ConsecutiveFailures.Load(),
	)

	// Calculate next retry
	nextRetry := backoff.Next()
	m.logger.Info("Will retry sync",
		"retry_in", nextRetry,
		"attempt", backoff.Attempt(),
	)

	// Schedule retry
	time.AfterFunc(nextRetry, func() {
		m.TriggerSync()
	})
}

// monitorWorker monitors sync health and triggers syncs as needed
func (m *Manager) monitorWorker(ctx context.Context) {
	defer m.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopChan:
			return
		case <-ticker.C:
			m.checkSyncHealth()
		}
	}
}

// checkSyncHealth checks sync health and triggers if needed
func (m *Manager) checkSyncHealth() {
	info := m.storage.GetStorageInfo()

	// Check if we need to sync based on buffer size
	if info.UnsyncedMetrics > int64(m.config.BatchSize*2) {
		m.logger.Info("Large buffer detected, triggering sync",
			"unsynced", info.UnsyncedMetrics,
		)
		m.TriggerSync()
		return
	}

	// Check if storage is getting full
	if m.capability.HasSQLite && info.StorageBytes > m.capability.LocalStorageSize*8/10 {
		m.logger.Warn("Storage nearly full, triggering sync",
			"used", info.StorageBytes,
			"limit", m.capability.LocalStorageSize,
		)
		m.TriggerSync()
		return
	}

	// Check if it's been too long since last sync
	lastSync := m.lastSyncTime.Load().(time.Time)
	if time.Since(lastSync) > m.config.SyncInterval*3 {
		m.logger.Warn("Sync overdue, triggering",
			"last_sync", lastSync,
		)
		m.TriggerSync()
	}
}

// applyCapabilityConfig applies capability-based configuration
func (m *Manager) applyCapabilityConfig() {
	syncConfig := m.capability.GetSyncConfig()

	// Override config with capability-based settings
	if m.config.SyncInterval == 0 {
		m.config.SyncInterval = syncConfig.Interval
	}
	if m.config.BatchSize == 0 {
		m.config.BatchSize = syncConfig.BatchSize
	}
	if m.config.CompressionType == "" {
		m.config.CompressionType = syncConfig.CompressionType
		m.config.CompressionEnabled = syncConfig.CompressionEnabled
	}
}

// applyConfigUpdate applies configuration updates from server
func (m *Manager) applyConfigUpdate(config *pb.SyncConfig) {
	m.logger.Info("Applying config update from server",
		"batch_size", config.BatchSize,
		"interval", config.SyncIntervalSeconds,
	)

	if config.BatchSize > 0 {
		m.config.BatchSize = int(config.BatchSize)
	}
	if config.SyncIntervalSeconds > 0 {
		m.config.SyncInterval = time.Duration(config.SyncIntervalSeconds) * time.Second
	}
	if config.CompressionType != "" {
		m.config.CompressionType = config.CompressionType
		m.config.CompressionEnabled = config.CompressionEnabled
	}
}

// GetMetrics returns current sync metrics
func (m *Manager) GetMetrics() SyncMetricsSnapshot {
	return SyncMetricsSnapshot{
		MetricsSynced:       m.metrics.MetricsSynced.Load(),
		LogsSynced:          m.metrics.LogsSynced.Load(),
		BytesSent:           m.metrics.BytesSent.Load(),
		BytesCompressed:     m.metrics.BytesCompressed.Load(),
		LastSyncDuration:    time.Duration(m.metrics.SyncDuration.Load()),
		FailedSyncs:         m.metrics.FailedSyncs.Load(),
		SuccessfulSyncs:     m.metrics.SuccessfulSyncs.Load(),
		ConsecutiveFailures: m.metrics.ConsecutiveFailures.Load(),
		LastSyncTime:        m.lastSyncTime.Load().(time.Time),
	}
}

// SyncMetricsSnapshot is a point-in-time snapshot of sync metrics
type SyncMetricsSnapshot struct {
	MetricsSynced       int64
	LogsSynced          int64
	BytesSent           int64
	BytesCompressed     int64
	LastSyncDuration    time.Duration
	FailedSyncs         int32
	SuccessfulSyncs     int32
	ConsecutiveFailures int32
	LastSyncTime        time.Time
	CompressionRatio    float64
}

// capabilityToProto converts capability to protobuf
func (m *Manager) capabilityToProto() *pb.SyncDeviceCapability {
	return &pb.SyncDeviceCapability{
		Tier:               int32(m.capability.Tier),
		TotalRam:           m.capability.TotalRAM,
		AvailableRam:       m.capability.AvailableRAM,
		TotalDisk:          m.capability.TotalDisk,
		AvailableDisk:      m.capability.AvailableDisk,
		CpuCores:           int32(m.capability.CPUCores),
		Architecture:       m.capability.Architecture,
		Os:                 m.capability.OS,
		HasSqlite:          m.capability.HasSQLite,
		LocalStorageSize:   m.capability.LocalStorageSize,
		MaxMetricsInMemory: int32(m.capability.MaxMetricsInMemory),
		HasNetwork:         m.capability.HasNetwork,
		BandwidthKbps:      int32(m.capability.BandwidthKbps),
		SupportsHttp2:      m.capability.SupportsHTTP2,
	}
}
