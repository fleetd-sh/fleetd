package metrics

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"

	"connectrpc.com/connect"
	metricspb "fleetd.sh/gen/metrics/v1"
)

type MetricsService struct {
	db *sql.DB
}

func NewMetricsService(db *sql.DB) *MetricsService {
	return &MetricsService{
		db: db,
	}
}

func (s *MetricsService) SendMetrics(
	ctx context.Context,
	req *connect.Request[metricspb.SendMetricsRequest],
) (*connect.Response[metricspb.SendMetricsResponse], error) {
	deviceID := req.Msg.DeviceId
	metrics := req.Msg.Metrics

	// Validate input
	if deviceID == "" {
		return connect.NewResponse(&metricspb.SendMetricsResponse{
			Success: false,
			Message: "Invalid device ID",
		}), nil
	}

	if len(metrics) == 0 {
		return connect.NewResponse(&metricspb.SendMetricsResponse{
			Success: true,
			Message: "No metrics to process",
		}), nil
	}

	for _, metric := range metrics {
		if metric.Name == "" || metric.Timestamp == nil {
			return connect.NewResponse(&metricspb.SendMetricsResponse{
				Success: false,
				Message: "Invalid metric data",
			}), nil
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		slog.Error("Failed to begin transaction", "error", err)
		return connect.NewResponse(&metricspb.SendMetricsResponse{
			Success: false,
			Message: "Failed to process metrics",
		}), nil
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO metric (device_id, name, value, timestamp, tags)
		VALUES (?, ?, ?, ?, ?)
	`)
	if err != nil {
		slog.Error("Failed to prepare statement", "error", err)
		return connect.NewResponse(&metricspb.SendMetricsResponse{
			Success: false,
			Message: "Failed to process metrics",
		}), nil
	}
	defer stmt.Close()

	for _, metric := range metrics {
		tags, err := json.Marshal(metric.Tags)
		if err != nil {
			slog.Error("Failed to marshal tags", "error", err, "metric", metric)
			return connect.NewResponse(&metricspb.SendMetricsResponse{
				Success: false,
				Message: "Failed to process metrics",
			}), nil
		}

		_, err = stmt.ExecContext(ctx, deviceID, metric.Name, metric.Value, metric.Timestamp.AsTime(), string(tags))
		if err != nil {
			slog.Error("Failed to insert metric", "error", err, "metric", metric)
			return connect.NewResponse(&metricspb.SendMetricsResponse{
				Success: false,
				Message: "Failed to process metrics",
			}), nil
		}
	}

	if err := tx.Commit(); err != nil {
		slog.Error("Failed to commit transaction", "error", err)
		return connect.NewResponse(&metricspb.SendMetricsResponse{
			Success: false,
			Message: "Failed to process metrics",
		}), nil
	}

	slog.Info("Metrics stored successfully", "deviceID", deviceID, "count", len(metrics))
	return connect.NewResponse(&metricspb.SendMetricsResponse{
		Success: true,
		Message: "Metrics stored successfully",
	}), nil
}
