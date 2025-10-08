package api

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"connectrpc.com/connect"
	pb "fleetd.sh/gen/fleetd/v1"
	"fleetd.sh/gen/fleetd/v1/fleetpbconnect"
	"fleetd.sh/internal/compression"
	"fleetd.sh/internal/telemetry"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// SyncService handles device data synchronization
type SyncService struct {
	fleetpbconnect.UnimplementedSyncServiceHandler

	db         *sql.DB
	vmClient   *telemetry.VictoriaMetricsClient
	lokiClient *telemetry.LokiClient
	logger     *slog.Logger

	// Metrics
	metricsReceived int64
	logsReceived    int64
	bytesReceived   int64
}

// NewSyncService creates a new sync service
func NewSyncService(
	db *sql.DB,
	vmClient *telemetry.VictoriaMetricsClient,
	lokiClient *telemetry.LokiClient,
) *SyncService {
	return &SyncService{
		db:         db,
		vmClient:   vmClient,
		lokiClient: lokiClient,
		logger:     slog.Default().With("component", "sync-service"),
	}
}

// SyncMetrics handles metrics synchronization from devices
func (s *SyncService) SyncMetrics(
	ctx context.Context,
	req *connect.Request[pb.SyncMetricsRequest],
) (*connect.Response[pb.SyncMetricsResponse], error) {
	msg := req.Msg
	meta := msg.Metadata

	// Validate device
	device, err := s.validateDevice(ctx, meta.DeviceId)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}

	// Decompress batch if needed
	batch := msg.Batch
	if batch.Compression != "" && batch.Compression != "none" {
		decompressed, err := s.decompressBatch(batch)
		if err != nil {
			s.logger.Error("Failed to decompress batch",
				"device_id", meta.DeviceId,
				"compression", batch.Compression,
				"error", err,
			)
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		batch = decompressed
	}

	// Convert and send to VictoriaMetrics
	vmMetrics := telemetry.DeviceMetrics{
		DeviceID: meta.DeviceId,
		OrgID:    meta.OrgId,
		Points:   make([]telemetry.MetricPoint, 0, len(batch.Metrics)),
	}

	for _, metric := range batch.Metrics {
		vmMetrics.Points = append(vmMetrics.Points, telemetry.MetricPoint{
			Timestamp: metric.Timestamp.AsTime(),
			Name:      metric.Name,
			Value:     metric.Value,
			Labels:    metric.Labels,
		})
	}

	// Send to VictoriaMetrics
	if err := s.vmClient.IngestMetrics(ctx, vmMetrics); err != nil {
		s.logger.Error("Failed to ingest metrics to VictoriaMetrics",
			"device_id", meta.DeviceId,
			"error", err,
		)
		// Don't fail the sync - we can retry later
	}

	// Store aggregates in PostgreSQL for quick queries
	go s.storeMetricAggregates(context.Background(), device.ID, vmMetrics)

	// Update device last seen
	go s.updateDeviceLastSeen(context.Background(), device.ID)

	// Update metrics
	s.metricsReceived += int64(len(batch.Metrics))
	if batch.CompressedData != nil {
		s.bytesReceived += int64(len(batch.CompressedData))
	} else {
		// Estimate uncompressed size
		for _, m := range batch.Metrics {
			s.bytesReceived += int64(proto.Size(m))
		}
	}

	// Build response
	resp := &pb.SyncMetricsResponse{
		Success:         true,
		LastSequenceAck: meta.SequenceNumber,
		ServerTime:      timestamppb.Now(),
	}

	// Check if we should update device config
	if s.shouldUpdateConfig(meta.Capability) {
		resp.ConfigUpdate = s.getConfigForCapability(meta.Capability)
	}

	return connect.NewResponse(resp), nil
}

// SyncLogs handles log synchronization from devices
func (s *SyncService) SyncLogs(
	ctx context.Context,
	req *connect.Request[pb.SyncLogsRequest],
) (*connect.Response[pb.SyncLogsResponse], error) {
	msg := req.Msg
	meta := msg.Metadata

	// Validate device
	device, err := s.validateDevice(ctx, meta.DeviceId)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}

	// Decompress batch if needed
	batch := msg.Batch
	if batch.Compression != "" && batch.Compression != "none" {
		decompressed, err := s.decompressLogsBatch(batch)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		batch = decompressed
	}

	// Send to Loki
	for _, log := range batch.Logs {
		s.lokiClient.Log(telemetry.LogEntry{
			Timestamp: log.Timestamp.AsTime(),
			Level:     log.Level,
			DeviceID:  meta.DeviceId,
			OrgID:     meta.OrgId,
			Source:    log.Source,
			Message:   log.Message,
			Labels:    log.Fields,
		})
	}

	// Store log metadata in PostgreSQL
	go s.storeLogMetadata(context.Background(), device.ID, batch)

	// Update metrics
	s.logsReceived += int64(len(batch.Logs))

	resp := &pb.SyncLogsResponse{
		Success:         true,
		LastSequenceAck: meta.SequenceNumber,
	}

	return connect.NewResponse(resp), nil
}

// GetSyncConfig returns sync configuration for a device
func (s *SyncService) GetSyncConfig(
	ctx context.Context,
	req *connect.Request[pb.GetSyncConfigRequest],
) (*connect.Response[pb.GetSyncConfigResponse], error) {
	msg := req.Msg

	// Validate device
	_, err := s.validateDevice(ctx, msg.DeviceId)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}

	config := s.getConfigForCapability(msg.Capability)

	resp := &pb.GetSyncConfigResponse{
		Config:              config,
		MetricsEndpoint:     "/api/v1/sync/metrics",
		LogsEndpoint:        "/api/v1/sync/logs",
		EventsEndpoint:      "/api/v1/sync/events",
		EnableCompression:   msg.Capability.Tier <= 2, // Enable for Tier 1 and 2
		EnableBatching:      true,
		EnableEdgeAnalytics: msg.Capability.Tier == 1, // Only for Tier 1
	}

	return connect.NewResponse(resp), nil
}

// StreamSync handles bidirectional streaming sync
func (s *SyncService) StreamSync(
	ctx context.Context,
	stream *connect.BidiStream[pb.StreamSyncRequest, pb.StreamSyncResponse],
) error {
	// This would implement real-time bidirectional sync
	// TODO: Implement streaming sync
	return connect.NewError(connect.CodeUnimplemented, fmt.Errorf("streaming sync not yet implemented"))
}

// validateDevice validates a device exists and is authorized
func (s *SyncService) validateDevice(ctx context.Context, deviceID string) (*Device, error) {
	var device Device
	err := s.db.QueryRowContext(ctx, `
		SELECT id, device_id, name, api_key, organization_id
		FROM devices
		WHERE device_id = $1
	`, deviceID).Scan(&device.ID, &device.DeviceID, &device.Name, &device.APIKey, &device.OrgID)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("device not found")
	}
	if err != nil {
		return nil, fmt.Errorf("database error: %w", err)
	}

	// Validate API key from request headers
	if err := s.validateAPIKey(ctx, device.APIKey); err != nil {
		return nil, fmt.Errorf("unauthorized: %w", err)
	}

	return &device, nil
}

// Device represents a device record
type Device struct {
	ID       string
	DeviceID string
	Name     string
	APIKey   string
	OrgID    string
}

// validateAPIKey validates the API key from the request context
func (s *SyncService) validateAPIKey(ctx context.Context, expectedKey string) error {
	// Get the API key that was validated by the middleware
	if providedKey, ok := getAPIKeyFromContext(ctx); ok {
		// Use constant-time comparison to prevent timing attacks
		if subtle.ConstantTimeCompare([]byte(providedKey), []byte(expectedKey)) == 1 {
			return nil
		}
		return fmt.Errorf("API key mismatch for device")
	}

	// Check if JWT claims are present (alternative auth method)
	if claims, ok := getClaimsFromContext(ctx); ok {
		// Device operations require specific permissions
		if hasDevicePermission(claims) {
			return nil
		}
		return fmt.Errorf("insufficient permissions for device operations")
	}

	return fmt.Errorf("no valid authentication found in request")
}

// getAPIKeyFromContext retrieves the API key from the request context
func getAPIKeyFromContext(ctx context.Context) (string, bool) {
	// This should match the key used by the auth middleware
	type contextKey string
	const APIKeyContextKey contextKey = "api_key"

	apiKey, ok := ctx.Value(APIKeyContextKey).(string)
	return apiKey, ok
}

// getClaimsFromContext retrieves JWT claims from the request context
func getClaimsFromContext(ctx context.Context) (map[string]interface{}, bool) {
	// This should match the key used by the auth middleware
	type contextKey string
	const ClaimsContextKey contextKey = "claims"

	claims, ok := ctx.Value(ClaimsContextKey).(map[string]interface{})
	return claims, ok
}

// hasDevicePermission checks if the claims have permission for device operations
func hasDevicePermission(claims map[string]interface{}) bool {
	// Check for device-related permissions or roles
	if roles, ok := claims["roles"].([]interface{}); ok {
		for _, role := range roles {
			if roleStr, ok := role.(string); ok {
				if roleStr == "admin" || roleStr == "device" || roleStr == "operator" {
					return true
				}
			}
		}
	}
	return false
}

// storeMetricAggregates stores metric aggregates in PostgreSQL
func (s *SyncService) storeMetricAggregates(ctx context.Context, deviceID string, metrics telemetry.DeviceMetrics) {
	// Group metrics by name and calculate aggregates
	aggregates := make(map[string]struct {
		min, max, sum float64
		count         int
	})

	for _, point := range metrics.Points {
		agg, exists := aggregates[point.Name]
		if !exists {
			agg = struct {
				min, max, sum float64
				count         int
			}{
				min: point.Value,
				max: point.Value,
			}
		}

		if point.Value < agg.min {
			agg.min = point.Value
		}
		if point.Value > agg.max {
			agg.max = point.Value
		}
		agg.sum += point.Value
		agg.count++

		aggregates[point.Name] = agg
	}

	// Store in PostgreSQL (using TimescaleDB hypertable)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		s.logger.Error("Failed to begin transaction", "error", err)
		return
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO device_metrics (time, device_id, metric_name, value, labels)
		VALUES ($1, $2, $3, $4, $5)
	`)
	if err != nil {
		s.logger.Error("Failed to prepare statement", "error", err)
		return
	}
	defer stmt.Close()

	for name, agg := range aggregates {
		avg := agg.sum / float64(agg.count)
		labels := map[string]any{
			"min":   agg.min,
			"max":   agg.max,
			"count": agg.count,
		}
		labelsJSON, _ := json.Marshal(labels)

		_, err = stmt.ExecContext(ctx,
			time.Now(),
			deviceID,
			name,
			avg,
			labelsJSON,
		)
		if err != nil {
			s.logger.Error("Failed to insert aggregate", "error", err)
		}
	}

	if err = tx.Commit(); err != nil {
		s.logger.Error("Failed to commit transaction", "error", err)
	}
}

// storeLogMetadata stores log metadata in PostgreSQL
func (s *SyncService) storeLogMetadata(ctx context.Context, deviceID string, batch *pb.LogsBatch) {
	// Count logs by level
	levelCounts := make(map[string]int)
	for _, log := range batch.Logs {
		levelCounts[log.Level]++
	}

	// Store counts
	for level, count := range levelCounts {
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO device_logs (time, device_id, log_level, count)
			VALUES ($1, $2, $3, $4)
		`, time.Now(), deviceID, level, count)
		if err != nil {
			s.logger.Error("Failed to store log metadata", "error", err)
		}
	}
}

// updateDeviceLastSeen updates the device's last seen timestamp
func (s *SyncService) updateDeviceLastSeen(ctx context.Context, deviceID string) {
	_, err := s.db.ExecContext(ctx, `
		UPDATE device
		SET last_seen = $1, updated_at = $1
		WHERE id = $2
	`, time.Now(), deviceID)
	if err != nil {
		s.logger.Error("Failed to update device last seen", "error", err)
	}
}

// shouldUpdateConfig determines if device config should be updated
func (s *SyncService) shouldUpdateConfig(cap *pb.SyncDeviceCapability) bool {
	// Update config if device capabilities have changed
	// or periodically (every 100th sync)
	// This is simplified - real implementation would track state
	return false
}

// getConfigForCapability returns config based on device capabilities
func (s *SyncService) getConfigForCapability(cap *pb.SyncDeviceCapability) *pb.SyncConfig {
	config := &pb.SyncConfig{
		MaxRetries:        3,
		BackoffMultiplier: 2.0,
	}

	switch cap.Tier {
	case 1: // Full featured
		config.BatchSize = 1000
		config.SyncIntervalSeconds = 300 // 5 minutes
		config.RetentionHours = 168      // 7 days
		config.CompressionEnabled = true
		config.CompressionType = "zstd"
		config.MaxMetricsPerSecond = 1000
		config.MaxLogsPerSecond = 100
		config.MaxLocalStorageBytes = 100_000_000 // 100MB
		config.MaxMetricsRetained = 100_000
		config.InitialBackoffSeconds = 1
		config.MaxBackoffSeconds = 300

	case 2: // Constrained
		config.BatchSize = 100
		config.SyncIntervalSeconds = 60 // 1 minute
		config.RetentionHours = 24      // 1 day
		config.CompressionEnabled = true
		config.CompressionType = "gzip"
		config.MaxMetricsPerSecond = 100
		config.MaxLogsPerSecond = 10
		config.MaxLocalStorageBytes = 5_000_000 // 5MB
		config.MaxMetricsRetained = 10_000
		config.InitialBackoffSeconds = 2
		config.MaxBackoffSeconds = 60

	case 3: // Minimal
		config.BatchSize = 10
		config.SyncIntervalSeconds = 10 // 10 seconds
		config.RetentionHours = 0       // No retention
		config.CompressionEnabled = false
		config.CompressionType = "none"
		config.MaxMetricsPerSecond = 10
		config.MaxLogsPerSecond = 1
		config.MaxLocalStorageBytes = 0 // Memory only
		config.MaxMetricsRetained = 100
		config.InitialBackoffSeconds = 5
		config.MaxBackoffSeconds = 30
	}

	return config
}

// decompressData is a generic helper for decompressing protobuf messages
func decompressData[T any](compressionType string, compressedData []byte, result *T) error {
	compressor, err := compression.New(compressionType)
	if err != nil {
		return err
	}

	// For zstd, close resources when done
	if zc, ok := compressor.(*compression.ZstdCompressor); ok {
		defer zc.Close()
	}

	data, err := compressor.Decompress(compressedData)
	if err != nil {
		return err
	}

	msg, ok := any(result).(proto.Message)
	if !ok {
		return fmt.Errorf("result must be a proto.Message")
	}

	if err := proto.Unmarshal(data, msg); err != nil {
		return err
	}

	return nil
}

// decompressBatch decompresses a metrics batch
func (s *SyncService) decompressBatch(batch *pb.MetricsBatch) (*pb.MetricsBatch, error) {
	var decompressed pb.MetricsBatch
	if err := decompressData(batch.Compression, batch.CompressedData, &decompressed); err != nil {
		return nil, fmt.Errorf("failed to decompress metrics batch: %w", err)
	}
	return &decompressed, nil
}

// decompressLogsBatch decompresses a logs batch
func (s *SyncService) decompressLogsBatch(batch *pb.LogsBatch) (*pb.LogsBatch, error) {
	var decompressed pb.LogsBatch
	if err := decompressData(batch.Compression, batch.CompressedData, &decompressed); err != nil {
		return nil, fmt.Errorf("failed to decompress logs batch: %w", err)
	}
	return &decompressed, nil
}
