package api

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"connectrpc.com/connect"
	pb "fleetd.sh/gen/fleetd/v1"
	rpc "fleetd.sh/gen/fleetd/v1/fleetpbconnect"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func parseTime(ts string) time.Time {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return time.Now()
	}
	return t
}

func calculateHealthScore(status string) float64 {
	switch status {
	case "healthy":
		return 1.0
	case "warning":
		return 0.8
	case "critical":
		return 0.0
	default:
		return 0.5
	}
}

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
	err := s.db.QueryRowContext(ctx, "SELECT 1 FROM device WHERE id = ?", req.Msg.DeviceId).Scan(&exists)
	if err == sql.ErrNoRows {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("device not found"))
	}
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to check device: %v", err))
	}

	// Build query for known metric columns
	query := `SELECT
		'cpu_usage' as metric_name, cpu_usage as value, timestamp FROM device_metric WHERE cpu_usage IS NOT NULL
		UNION ALL
		SELECT 'memory_usage' as metric_name, memory_usage as value, timestamp FROM device_metric WHERE memory_usage IS NOT NULL
		UNION ALL
		SELECT 'disk_usage' as metric_name, disk_usage as value, timestamp FROM device_metric WHERE disk_usage IS NOT NULL
		UNION ALL
		SELECT 'network_rx_bytes' as metric_name, CAST(network_rx_bytes as REAL) as value, timestamp FROM device_metric WHERE network_rx_bytes IS NOT NULL
		UNION ALL
		SELECT 'network_tx_bytes' as metric_name, CAST(network_tx_bytes as REAL) as value, timestamp FROM device_metric WHERE network_tx_bytes IS NOT NULL`

	query += ` AND device_id = ? AND timestamp BETWEEN ? AND ?`
	args := []any{req.Msg.DeviceId, req.Msg.TimeRange.StartTime.AsTime(), req.Msg.TimeRange.EndTime.AsTime()}

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
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to query metrics: %v", err))
	}
	defer rows.Close()

	// Group metrics by name
	metricsByName := make(map[string]*pb.MetricSeries)
	for rows.Next() {
		var (
			name         string
			value        float64
			timestampStr string
		)

		if err := rows.Scan(&name, &value, &timestampStr); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to scan metric: %v", err))
		}

		series, ok := metricsByName[name]
		if !ok {
			series = &pb.MetricSeries{
				Name:   name,
				Values: make([]*pb.MetricValue, 0),
			}
			metricsByName[name] = series
		}

		timestamp, err := time.Parse(time.RFC3339, timestampStr)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to parse timestamp: %v", err))
		}

		metricValue := &pb.MetricValue{
			Timestamp: timestamppb.New(timestamp),
			Value:     &pb.MetricValue_Numeric{Numeric: value},
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

func (s *AnalyticsService) GetUpdateAnalytics(ctx context.Context, req *connect.Request[pb.GetUpdateAnalyticsRequest]) (*connect.Response[pb.GetUpdateAnalyticsResponse], error) {
	query := `SELECT c.id, c.name, c.total_devices,
				c.updated_devices as successful,
				c.failed_devices as failed,
				COALESCE(m.avg_duration, 0) as avg_duration,
				COALESCE(m.failure_rate, 0) as failure_rate
			FROM update_campaign c
			LEFT JOIN (
				SELECT campaign_id,
					   AVG(avg_duration) as avg_duration,
					   AVG(failure_rate) as failure_rate
				FROM update_metric
				WHERE timestamp BETWEEN ? AND ?
				GROUP BY campaign_id
			) m ON c.id = m.campaign_id
			WHERE strftime('%Y-%m-%dT%H:%M:%SZ', c.created_at) BETWEEN ? AND ?`
	args := []any{
		req.Msg.TimeRange.StartTime.AsTime().UTC().Format(time.RFC3339),
		req.Msg.TimeRange.EndTime.AsTime().UTC().Format(time.RFC3339),
		req.Msg.TimeRange.StartTime.AsTime().UTC().Format(time.RFC3339),
		req.Msg.TimeRange.EndTime.AsTime().UTC().Format(time.RFC3339),
	}

	if req.Msg.CampaignId != "" {
		query += " AND c.id = ?"
		args = append(args, req.Msg.CampaignId)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to query update metrics: %v", err))
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
			failureRate sql.NullFloat64
		)

		if err := rows.Scan(
			&campaign.CampaignId,
			&campaign.Name,
			&campaign.TotalDevices,
			&campaign.SuccessfulUpdates,
			&campaign.FailedUpdates,
			&avgDuration,
			&failureRate,
		); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to scan update metrics: %v", err))
		}

		if campaign.TotalDevices > 0 {
			campaign.SuccessRate = float64(campaign.SuccessfulUpdates) / float64(campaign.TotalDevices)
		}
		if avgDuration.Valid {
			campaign.AverageDurationSeconds = avgDuration.Float64
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

	return &connect.Response[pb.GetUpdateAnalyticsResponse]{
		Msg: &pb.GetUpdateAnalyticsResponse{
			Campaigns:             campaigns,
			OverallSuccessRate:    overallSuccessRate,
			AverageCompletionTime: averageCompletionTime,
			FailuresByReason:      failuresByReason,
		},
	}, nil
}

func (s *AnalyticsService) GetDeviceHealth(ctx context.Context, req *connect.Request[pb.GetDeviceHealthRequest]) (*connect.Response[pb.GetDeviceHealthResponse], error) {
	// Get current health status
	var (
		status        string
		message       string
		lastHeartbeat sql.NullString
	)

	err := s.db.QueryRowContext(ctx,
		`SELECT status, message, strftime('%Y-%m-%dT%H:%M:%SZ', last_heartbeat) as last_heartbeat
		 FROM device_health
		 WHERE device_id = ? ORDER BY timestamp DESC LIMIT 1`,
		req.Msg.DeviceId).Scan(&status, &message, &lastHeartbeat)
	if err == sql.ErrNoRows {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("device health not found"))
	}
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get device health: %v", err))
	}

	// Handle NULL last_heartbeat
	var lastCheck *timestamppb.Timestamp
	if lastHeartbeat.Valid {
		lastCheck = timestamppb.New(parseTime(lastHeartbeat.String))
	}

	healthStatus := pb.DeviceHealthStatus{
		DeviceId:    req.Msg.DeviceId,
		Status:      status,
		Warnings:    []string{message},
		LastCheck:   lastCheck,
		HealthScore: calculateHealthScore(status),
	}

	// Get historical status if time range provided
	var historicalStatus []*pb.DeviceHealthStatus
	if req.Msg.TimeRange != nil {
		rows, err := s.db.QueryContext(ctx,
			`SELECT status, message, strftime('%Y-%m-%dT%H:%M:%SZ', last_heartbeat) as last_heartbeat
			 FROM device_health
			 WHERE device_id = ? AND timestamp BETWEEN ? AND ?
			 ORDER BY timestamp DESC`,
			req.Msg.DeviceId,
			req.Msg.TimeRange.StartTime.AsTime(),
			req.Msg.TimeRange.EndTime.AsTime())
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get historical health: %v", err))
		}
		defer rows.Close()

		for rows.Next() {
			var (
				status        string
				message       string
				lastHeartbeat sql.NullString
			)
			if err := rows.Scan(&status, &message, &lastHeartbeat); err != nil {
				return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to scan historical health: %v", err))
			}

			var lastCheck *timestamppb.Timestamp
			if lastHeartbeat.Valid {
				lastCheck = timestamppb.New(parseTime(lastHeartbeat.String))
			}

			historicalStatus = append(historicalStatus, &pb.DeviceHealthStatus{
				DeviceId:    req.Msg.DeviceId,
				Status:      status,
				Warnings:    []string{message},
				LastCheck:   lastCheck,
				HealthScore: calculateHealthScore(status),
			})
		}
	}

	return &connect.Response[pb.GetDeviceHealthResponse]{
		Msg: &pb.GetDeviceHealthResponse{
			CurrentStatus:    &healthStatus,
			HistoricalStatus: historicalStatus,
		},
	}, nil
}

func (s *AnalyticsService) GetPerformanceMetrics(ctx context.Context, req *connect.Request[pb.GetPerformanceMetricsRequest]) (*connect.Response[pb.GetPerformanceMetricsResponse], error) {
	query := `SELECT metric_name, value, unit, strftime('%Y-%m-%dT%H:%M:%SZ', timestamp) as timestamp
			  FROM performance_metric
			  WHERE timestamp BETWEEN ? AND ?`
	args := []any{
		req.Msg.TimeRange.StartTime.AsTime().UTC().Format(time.RFC3339),
		req.Msg.TimeRange.EndTime.AsTime().UTC().Format(time.RFC3339),
	}

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
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to query performance metrics: %v", err))
	}
	defer rows.Close()

	var (
		metrics           []*pb.PerformanceMetric
		aggregatedMetrics = make(map[string]float64)
		metricCounts      = make(map[string]int)
	)

	for rows.Next() {
		var (
			metric       pb.PerformanceMetric
			timestampStr string
		)

		if err := rows.Scan(&metric.Name, &metric.Value, &metric.Unit, &timestampStr); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to scan performance metric: %v", err))
		}

		timestamp, err := time.Parse(time.RFC3339, timestampStr)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to parse timestamp: %v", err))
		}

		metric.Timestamp = timestamppb.New(timestamp)
		metrics = append(metrics, &metric)

		// Calculate running average for aggregation
		aggregatedMetrics[metric.Name] = (aggregatedMetrics[metric.Name]*float64(metricCounts[metric.Name]) + metric.Value) /
			float64(metricCounts[metric.Name]+1)
		metricCounts[metric.Name]++
	}

	return &connect.Response[pb.GetPerformanceMetricsResponse]{
		Msg: &pb.GetPerformanceMetricsResponse{
			Metrics:           metrics,
			AggregatedMetrics: aggregatedMetrics,
		},
	}, nil
}
