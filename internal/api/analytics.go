package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"connectrpc.com/connect"
	pb "fleetd.sh/gen/fleetd/v1"
	rpc "fleetd.sh/gen/fleetd/v1/fleetpbconnect"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type AnalyticsService struct {
	rpc.UnimplementedAnalyticsServiceHandler
	db *sql.DB
}

func NewAnalyticsService(db *sql.DB) *AnalyticsService {
	return &AnalyticsService{db: db}
}

func (s *AnalyticsService) GetDeviceMetrics(ctx context.Context, req *connect.Request[pb.GetDeviceMetricsRequest]) (*connect.Response[pb.GetDeviceMetricsResponse], error) {
	// Validate device exists
	var exists bool
	err := s.db.QueryRowContext(ctx, "SELECT 1 FROM devices WHERE id = ?", req.Msg.DeviceId).Scan(&exists)
	if err == sql.ErrNoRows {
		return nil, status.Errorf(codes.NotFound, "device not found")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to check device: %v", err)
	}

	// Build query
	query := `SELECT metric_name, metric_value, metric_text, timestamp
			  FROM device_metrics
			  WHERE device_id = ? AND timestamp BETWEEN ? AND ?`
	args := []interface{}{req.Msg.DeviceId, req.Msg.TimeRange.StartTime.AsTime(), req.Msg.TimeRange.EndTime.AsTime()}

	if len(req.Msg.MetricNames) > 0 {
		placeholders := make([]string, len(req.Msg.MetricNames))
		for i := range placeholders {
			placeholders[i] = "?"
			args = append(args, req.Msg.MetricNames[i])
		}
		query += fmt.Sprintf(" AND metric_name IN (%s)", strings.Join(placeholders, ","))
	}

	query += " ORDER BY timestamp ASC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to query metrics: %v", err)
	}
	defer rows.Close()

	// Group metrics by name
	metricsByName := make(map[string]*pb.MetricSeries)
	for rows.Next() {
		var (
			name      string
			value     sql.NullFloat64
			text      sql.NullString
			timestamp time.Time
		)

		if err := rows.Scan(&name, &value, &text, &timestamp); err != nil {
			return nil, status.Errorf(codes.Internal, "failed to scan metric: %v", err)
		}

		series, ok := metricsByName[name]
		if !ok {
			series = &pb.MetricSeries{
				Name:   name,
				Values: make([]*pb.MetricValue, 0),
			}
			metricsByName[name] = series
		}

		metricValue := &pb.MetricValue{
			Timestamp: timestamppb.New(timestamp),
		}
		if value.Valid {
			metricValue.Value = &pb.MetricValue_Numeric{Numeric: value.Float64}
		} else if text.Valid {
			metricValue.Value = &pb.MetricValue_Text{Text: text.String}
		}
		series.Values = append(series.Values, metricValue)
	}

	// Convert map to slice
	metrics := make([]*pb.MetricSeries, 0, len(metricsByName))
	for _, series := range metricsByName {
		metrics = append(metrics, series)
	}

	return &connect.Response[pb.GetDeviceMetricsResponse]{
		Msg: &pb.GetDeviceMetricsResponse{
			Metrics: metrics,
		},
	}, nil
}

func (s *AnalyticsService) GetUpdateAnalytics(ctx context.Context, req *pb.GetUpdateAnalyticsRequest) (*pb.GetUpdateAnalyticsResponse, error) {
	query := `SELECT c.id, c.name, c.total_devices,
				COUNT(CASE WHEN m.status = 'INSTALLED' THEN 1 END) as successful,
				COUNT(CASE WHEN m.status = 'FAILED' OR m.status = 'ROLLED_BACK' THEN 1 END) as failed,
				AVG(CASE WHEN m.status = 'INSTALLED' THEN m.duration_seconds END) as avg_duration,
				GROUP_CONCAT(DISTINCT CASE WHEN m.status = 'FAILED' THEN m.failure_reason END) as failure_reasons
			FROM update_campaigns c
			LEFT JOIN update_metrics m ON c.id = m.campaign_id
			WHERE m.timestamp BETWEEN ? AND ?`
	args := []interface{}{req.TimeRange.StartTime.AsTime(), req.TimeRange.EndTime.AsTime()}

	if req.CampaignId != "" {
		query += " AND c.id = ?"
		args = append(args, req.CampaignId)
	}

	query += " GROUP BY c.id, c.name, c.total_devices"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to query update metrics: %v", err)
	}
	defer rows.Close()

	var (
		campaigns        []*pb.UpdateMetrics
		totalSuccess     int32
		totalFailed      int32
		totalDuration    float64
		totalCampaigns   int32
		failuresByReason = make(map[string]int32)
	)

	for rows.Next() {
		var (
			campaign    pb.UpdateMetrics
			avgDuration sql.NullFloat64
			failureList sql.NullString
		)

		if err := rows.Scan(
			&campaign.CampaignId,
			&campaign.Name,
			&campaign.TotalDevices,
			&campaign.SuccessfulUpdates,
			&campaign.FailedUpdates,
			&avgDuration,
			&failureList,
		); err != nil {
			return nil, status.Errorf(codes.Internal, "failed to scan update metrics: %v", err)
		}

		if campaign.TotalDevices > 0 {
			campaign.SuccessRate = float64(campaign.SuccessfulUpdates) / float64(campaign.TotalDevices)
		}
		if avgDuration.Valid {
			campaign.AverageDurationSeconds = avgDuration.Float64
		}
		if failureList.Valid {
			campaign.CommonFailureReasons = strings.Split(failureList.String, ",")
			for _, reason := range campaign.CommonFailureReasons {
				failuresByReason[reason]++
			}
		}

		totalSuccess += campaign.SuccessfulUpdates
		totalFailed += campaign.FailedUpdates
		if avgDuration.Valid {
			totalDuration += avgDuration.Float64
		}
		totalCampaigns++

		campaigns = append(campaigns, &campaign)
	}

	var overallSuccessRate float64
	if totalSuccess+totalFailed > 0 {
		overallSuccessRate = float64(totalSuccess) / float64(totalSuccess+totalFailed)
	}

	var averageCompletionTime float64
	if totalCampaigns > 0 {
		averageCompletionTime = totalDuration / float64(totalCampaigns)
	}

	return &pb.GetUpdateAnalyticsResponse{
		Campaigns:             campaigns,
		OverallSuccessRate:    overallSuccessRate,
		AverageCompletionTime: averageCompletionTime,
		FailuresByReason:      failuresByReason,
	}, nil
}

func (s *AnalyticsService) GetDeviceHealth(ctx context.Context, req *pb.GetDeviceHealthRequest) (*pb.GetDeviceHealthResponse, error) {
	// Get current health status
	var (
		healthStatus pb.DeviceHealthStatus
		metrics      string
		warnings     string
		timestamp    time.Time
	)

	err := s.db.QueryRowContext(ctx,
		`SELECT status, health_metrics, warnings, timestamp FROM device_health
		 WHERE device_id = ? ORDER BY timestamp DESC LIMIT 1`,
		req.DeviceId).Scan(&healthStatus, &metrics, &warnings, &timestamp)
	if err == sql.ErrNoRows {
		return nil, status.Errorf(codes.NotFound, "device health not found")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get device health: %v", err)
	}

	if err := json.Unmarshal([]byte(metrics), &healthStatus.HealthMetrics); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to unmarshal health metrics: %v", err)
	}
	if err := json.Unmarshal([]byte(warnings), &healthStatus.Warnings); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to unmarshal warnings: %v", err)
	}
	healthStatus.LastCheck = timestamppb.New(timestamp)

	// Get historical health status
	var historicalStatus []*pb.DeviceHealthStatus
	rows, err := s.db.QueryContext(ctx,
		`SELECT status, health_metrics, warnings, timestamp FROM device_health
		 WHERE device_id = ? AND timestamp >= ? AND timestamp <= ?
		 ORDER BY timestamp DESC`,
		req.DeviceId, req.TimeRange.StartTime.AsTime(), req.TimeRange.EndTime.AsTime())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to query historical health: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			healthStatus pb.DeviceHealthStatus
			metrics      string
			warnings     string
			timestamp    time.Time
		)

		if err := rows.Scan(&healthStatus, &metrics, &warnings, &timestamp); err != nil {
			return nil, status.Errorf(codes.Internal, "failed to scan health status: %v", err)
		}

		if err := json.Unmarshal([]byte(metrics), &healthStatus.HealthMetrics); err != nil {
			return nil, status.Errorf(codes.Internal, "failed to unmarshal health metrics: %v", err)
		}
		if err := json.Unmarshal([]byte(warnings), &healthStatus.Warnings); err != nil {
			return nil, status.Errorf(codes.Internal, "failed to unmarshal warnings: %v", err)
		}
		healthStatus.LastCheck = timestamppb.New(timestamp)

		historicalStatus = append(historicalStatus, &healthStatus)
	}

	return &pb.GetDeviceHealthResponse{
		CurrentStatus:    &healthStatus,
		HistoricalStatus: historicalStatus,
	}, nil
}

func (s *AnalyticsService) GetPerformanceMetrics(ctx context.Context, req *pb.GetPerformanceMetricsRequest) (*pb.GetPerformanceMetricsResponse, error) {
	query := `SELECT name, value, unit, timestamp
			  FROM performance_metrics
			  WHERE timestamp BETWEEN ? AND ?`
	args := []interface{}{req.TimeRange.StartTime.AsTime(), req.TimeRange.EndTime.AsTime()}

	if len(req.MetricNames) > 0 {
		placeholders := make([]string, len(req.MetricNames))
		for i := range placeholders {
			placeholders[i] = "?"
			args = append(args, req.MetricNames[i])
		}
		query += fmt.Sprintf(" AND name IN (%s)", strings.Join(placeholders, ","))
	}

	query += " ORDER BY timestamp ASC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to query performance metrics: %v", err)
	}
	defer rows.Close()

	var (
		metrics           []*pb.PerformanceMetric
		aggregatedMetrics = make(map[string]float64)
		metricCounts      = make(map[string]int)
	)

	for rows.Next() {
		var metric pb.PerformanceMetric
		var timestamp time.Time

		if err := rows.Scan(&metric.Name, &metric.Value, &metric.Unit, &timestamp); err != nil {
			return nil, status.Errorf(codes.Internal, "failed to scan performance metric: %v", err)
		}

		metric.Timestamp = timestamppb.New(timestamp)
		metrics = append(metrics, &metric)

		// Calculate running average for aggregation
		aggregatedMetrics[metric.Name] = (aggregatedMetrics[metric.Name]*float64(metricCounts[metric.Name]) + metric.Value) /
			float64(metricCounts[metric.Name]+1)
		metricCounts[metric.Name]++
	}

	return &pb.GetPerformanceMetricsResponse{
		Metrics:           metrics,
		AggregatedMetrics: aggregatedMetrics,
	}, nil
}
