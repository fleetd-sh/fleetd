package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"

	"fleetd.sh/internal/agent/capability"
	"fleetd.sh/internal/agent/storage"
	"fleetd.sh/internal/agent/sync"
)

func main() {
	// Setup logging
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	slog.SetDefault(logger)

	// Detect device capabilities
	cap := capability.DetectCapabilities()
	logger.Info("Device capabilities detected",
		"tier", cap.Tier,
		"ram", formatBytes(cap.TotalRAM),
		"disk", formatBytes(cap.TotalDisk),
		"cores", cap.CPUCores,
		"has_sqlite", cap.HasSQLite,
	)

	// Create appropriate storage
	dataPath := "/var/lib/fleetd"
	os.MkdirAll(dataPath, 0o755)

	deviceStorage, err := cap.CreateStorage(dataPath)
	if err != nil {
		log.Fatal("Failed to create storage:", err)
	}
	defer deviceStorage.Close()

	// Create sync client
	serverURL := getEnv("FLEET_SERVER_URL", "http://localhost:8080")
	apiKey := getEnv("FLEET_API_KEY", "demo-device-key")
	deviceID := getEnv("DEVICE_ID", generateDeviceID())

	syncClient := sync.NewConnectSyncClient(
		serverURL,
		apiKey,
		sync.WithTimeout(30*time.Second),
		sync.WithMaxRetries(3),
	)
	defer syncClient.Close()

	// Create sync manager
	syncConfig := &sync.SyncConfig{
		DeviceID:           deviceID,
		OrgID:              "demo-org",
		ServerURL:          serverURL,
		SyncInterval:       cap.SyncInterval,
		BatchSize:          cap.BatchSize,
		CompressionEnabled: cap.CompressionEnabled,
		CompressionType:    cap.CompressionType,
		MaxRetries:         3,
		InitialBackoff:     1 * time.Second,
		MaxBackoff:         5 * time.Minute,
		BackoffMultiplier:  2.0,
		OfflineQueueSize:   10000,
	}

	syncManager := sync.NewManager(
		deviceStorage,
		syncClient,
		cap,
		syncConfig,
	)

	// Start sync manager
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := syncManager.Start(ctx); err != nil {
		log.Fatal("Failed to start sync manager:", err)
	}

	// Start metric generation
	go generateMetrics(deviceStorage, logger)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Periodically show status
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	logger.Info("Agent started, generating metrics...")

	for {
		select {
		case <-sigChan:
			logger.Info("Shutting down...")
			syncManager.Stop()
			return

		case <-ticker.C:
			// Show sync status
			metrics := syncManager.GetMetrics()
			storageInfo := deviceStorage.GetStorageInfo()

			logger.Info("Sync status",
				"metrics_synced", metrics.MetricsSynced,
				"bytes_sent", formatBytes(metrics.BytesSent),
				"compression_ratio", fmt.Sprintf("%.2f", metrics.CompressionRatio),
				"successful_syncs", metrics.SuccessfulSyncs,
				"failed_syncs", metrics.FailedSyncs,
				"unsynced_metrics", storageInfo.UnsyncedMetrics,
				"storage_used", formatBytes(storageInfo.StorageBytes),
			)
		}
	}
}

// generateMetrics simulates metric generation
func generateMetrics(store storage.DeviceStorage, logger *slog.Logger) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		// Generate random metrics
		metrics := []storage.Metric{
			{
				Name:      "cpu_usage",
				Value:     20 + rand.Float64()*60, // 20-80%
				Timestamp: time.Now(),
				Labels: map[string]string{
					"core": "all",
				},
			},
			{
				Name:      "memory_usage",
				Value:     30 + rand.Float64()*50, // 30-80%
				Timestamp: time.Now(),
				Labels: map[string]string{
					"type": "used",
				},
			},
			{
				Name:      "disk_usage",
				Value:     10 + rand.Float64()*70, // 10-80%
				Timestamp: time.Now(),
				Labels: map[string]string{
					"mount": "/",
				},
			},
			{
				Name:      "network_rx_bytes",
				Value:     rand.Float64() * 1000000, // 0-1MB
				Timestamp: time.Now(),
				Labels: map[string]string{
					"interface": "eth0",
				},
			},
			{
				Name:      "network_tx_bytes",
				Value:     rand.Float64() * 500000, // 0-500KB
				Timestamp: time.Now(),
				Labels: map[string]string{
					"interface": "eth0",
				},
			},
		}

		// Store metrics
		for _, metric := range metrics {
			if err := store.StoreMetric(metric); err != nil {
				logger.Error("Failed to store metric", "error", err)
			}
		}

		logger.Debug("Generated metrics", "count", len(metrics))
	}
}

// Helper functions
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func generateDeviceID() string {
	// In production, use MAC address or hardware ID
	return fmt.Sprintf("device-%d", time.Now().Unix())
}
