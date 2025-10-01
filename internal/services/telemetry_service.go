package services

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"connectrpc.com/connect"
	fleetpb "fleetd.sh/gen/fleetd/v1"
	"fleetd.sh/gen/fleetd/v1/fleetpbconnect"
	"fleetd.sh/internal/database"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type TelemetryService struct {
	fleetpbconnect.UnimplementedTelemetryServiceHandler
	db *database.DB
}

func NewTelemetryService(db *database.DB) *TelemetryService {
	return &TelemetryService{
		db: db,
	}
}

// GetTelemetry retrieves telemetry data for devices
func (s *TelemetryService) GetTelemetry(
	ctx context.Context,
	req *connect.Request[fleetpb.GetTelemetryRequest],
) (*connect.Response[fleetpb.GetTelemetryResponse], error) {
	// For now, return mock data
	data := generateMockTelemetryData(req.Msg.DeviceId, 20)

	// Apply time filtering if provided
	if req.Msg.StartTime != nil || req.Msg.EndTime != nil {
		var filtered []*fleetpb.TelemetryData
		for _, d := range data {
			if req.Msg.StartTime != nil && d.Timestamp.AsTime().Before(req.Msg.StartTime.AsTime()) {
				continue
			}
			if req.Msg.EndTime != nil && d.Timestamp.AsTime().After(req.Msg.EndTime.AsTime()) {
				continue
			}
			filtered = append(filtered, d)
		}
		data = filtered
	}

	// Apply limit
	if req.Msg.Limit > 0 && int(req.Msg.Limit) < len(data) {
		data = data[:req.Msg.Limit]
	}

	return connect.NewResponse(&fleetpb.GetTelemetryResponse{
		Data: data,
	}), nil
}

// GetMetrics retrieves aggregated metrics
func (s *TelemetryService) GetMetrics(
	ctx context.Context,
	req *connect.Request[fleetpb.GetMetricsRequest],
) (*connect.Response[fleetpb.GetMetricsResponse], error) {
	// Generate mock aggregated metrics
	var metrics []*fleetpb.MetricData

	deviceIds := req.Msg.DeviceIds
	if len(deviceIds) == 0 {
		deviceIds = []string{"device-001", "device-002", "device-003"}
	}

	metricNames := req.Msg.MetricNames
	if len(metricNames) == 0 {
		metricNames = []string{"cpu", "memory", "disk", "network"}
	}

	for _, deviceId := range deviceIds {
		for _, metricName := range metricNames {
			metrics = append(metrics, &fleetpb.MetricData{
				Name:      metricName,
				Timestamp: timestamppb.Now(),
				Value:     rand.Float64() * 100,
				DeviceId:  deviceId,
				Labels: map[string]string{
					"aggregation": req.Msg.Aggregation,
				},
			})
		}
	}

	return connect.NewResponse(&fleetpb.GetMetricsResponse{
		Metrics: metrics,
	}), nil
}

// StreamTelemetry streams real-time telemetry data
func (s *TelemetryService) StreamTelemetry(
	ctx context.Context,
	req *connect.Request[fleetpb.StreamTelemetryRequest],
	stream *connect.ServerStream[fleetpb.TelemetryData],
) error {
	deviceIds := req.Msg.DeviceIds
	if len(deviceIds) == 0 {
		deviceIds = []string{"device-001"}
	}

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			for _, deviceId := range deviceIds {
				data := &fleetpb.TelemetryData{
					DeviceId:      deviceId,
					Timestamp:     timestamppb.Now(),
					CpuUsage:      rand.Float64() * 100,
					MemoryUsage:   rand.Float64() * 100,
					DiskUsage:     20 + rand.Float64()*60,
					NetworkUsage:  rand.Float64() * 100,
					Temperature:   35 + rand.Float64()*15,
					CustomMetrics: map[string]float64{},
				}

				if err := stream.Send(data); err != nil {
					return err
				}
			}
		}
	}
}

// GetLogs retrieves system logs
func (s *TelemetryService) GetLogs(
	ctx context.Context,
	req *connect.Request[fleetpb.GetLogsRequest],
) (*connect.Response[fleetpb.GetLogsResponse], error) {
	// Generate mock logs
	logs := generateMockLogs(req.Msg.DeviceIds, 50)

	// Apply filtering
	var filtered []*fleetpb.TelemetryLogEntry
	for _, log := range logs {
		// Filter by level
		if len(req.Msg.Levels) > 0 {
			found := false
			for _, level := range req.Msg.Levels {
				if log.Level == level {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		// Filter by text
		if req.Msg.Filter != "" {
			// Simple text matching
			// In production, this would be more sophisticated
			if !contains(log.Message, req.Msg.Filter) {
				continue
			}
		}

		filtered = append(filtered, log)
	}

	// Apply limit
	if req.Msg.Limit > 0 && int(req.Msg.Limit) < len(filtered) {
		filtered = filtered[:req.Msg.Limit]
	}

	return connect.NewResponse(&fleetpb.GetLogsResponse{
		Logs: filtered,
	}), nil
}

// StreamLogs streams real-time logs
func (s *TelemetryService) StreamLogs(
	ctx context.Context,
	req *connect.Request[fleetpb.StreamLogsRequest],
	stream *connect.ServerStream[fleetpb.TelemetryLogEntry],
) error {
	deviceIds := req.Msg.DeviceIds
	if len(deviceIds) == 0 {
		deviceIds = []string{"device-001", "device-002"}
	}

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	logMessages := []string{
		"System health check passed",
		"Configuration updated successfully",
		"Telemetry data transmitted",
		"Cache cleared",
		"Service restarted",
		"Connection established",
		"Deployment initiated",
		"Update check completed",
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			deviceId := deviceIds[rand.Intn(len(deviceIds))]
			log := &fleetpb.TelemetryLogEntry{
				Id:        fmt.Sprintf("log-%d", time.Now().Unix()),
				DeviceId:  deviceId,
				Timestamp: timestamppb.Now(),
				Level:     randomLogLevel(),
				Message:   logMessages[rand.Intn(len(logMessages))],
				Metadata:  map[string]string{},
			}

			if err := stream.Send(log); err != nil {
				return err
			}
		}
	}
}

// ConfigureAlert configures an alert
func (s *TelemetryService) ConfigureAlert(
	ctx context.Context,
	req *connect.Request[fleetpb.ConfigureAlertRequest],
) (*connect.Response[fleetpb.ConfigureAlertResponse], error) {
	alert := req.Msg.Alert
	if alert.Id == "" {
		alert.Id = fmt.Sprintf("alert-%d", time.Now().Unix())
		alert.CreatedAt = timestamppb.Now()
	}
	alert.UpdatedAt = timestamppb.Now()

	// In production, this would save to database

	return connect.NewResponse(&fleetpb.ConfigureAlertResponse{
		Alert: alert,
	}), nil
}

// ListAlerts lists configured alerts
func (s *TelemetryService) ListAlerts(
	ctx context.Context,
	req *connect.Request[fleetpb.ListAlertsRequest],
) (*connect.Response[fleetpb.ListAlertsResponse], error) {
	alerts := []*fleetpb.Alert{
		{
			Id:          "alert-cpu",
			Name:        "High CPU Usage",
			Description: "Alert when CPU usage exceeds 80%",
			Type:        fleetpb.AlertType_ALERT_TYPE_CPU,
			Threshold:   80,
			Condition:   fleetpb.AlertCondition_ALERT_CONDITION_GREATER_THAN,
			Enabled:     true,
			DeviceIds:   req.Msg.DeviceIds,
			CreatedAt:   timestamppb.Now(),
			UpdatedAt:   timestamppb.Now(),
		},
		{
			Id:          "alert-disk",
			Name:        "Low Disk Space",
			Description: "Alert when disk usage exceeds 90%",
			Type:        fleetpb.AlertType_ALERT_TYPE_DISK,
			Threshold:   90,
			Condition:   fleetpb.AlertCondition_ALERT_CONDITION_GREATER_THAN,
			Enabled:     true,
			DeviceIds:   req.Msg.DeviceIds,
			CreatedAt:   timestamppb.Now(),
			UpdatedAt:   timestamppb.Now(),
		},
		{
			Id:          "alert-offline",
			Name:        "Device Offline",
			Description: "Alert when device goes offline",
			Type:        fleetpb.AlertType_ALERT_TYPE_DEVICE_OFFLINE,
			Threshold:   5,
			Condition:   fleetpb.AlertCondition_ALERT_CONDITION_GREATER_THAN,
			Enabled:     true,
			DeviceIds:   req.Msg.DeviceIds,
			CreatedAt:   timestamppb.Now(),
			UpdatedAt:   timestamppb.Now(),
		},
	}

	if req.Msg.EnabledOnly {
		var enabled []*fleetpb.Alert
		for _, alert := range alerts {
			if alert.Enabled {
				enabled = append(enabled, alert)
			}
		}
		alerts = enabled
	}

	return connect.NewResponse(&fleetpb.ListAlertsResponse{
		Alerts: alerts,
	}), nil
}

// DeleteAlert deletes an alert
func (s *TelemetryService) DeleteAlert(
	ctx context.Context,
	req *connect.Request[fleetpb.DeleteAlertRequest],
) (*connect.Response[fleetpb.DeleteAlertResponse], error) {
	// In production, this would delete from database
	return connect.NewResponse(&fleetpb.DeleteAlertResponse{}), nil
}

// Helper functions

func generateMockTelemetryData(deviceId string, count int) []*fleetpb.TelemetryData {
	data := make([]*fleetpb.TelemetryData, count)
	now := time.Now()

	for i := 0; i < count; i++ {
		data[i] = &fleetpb.TelemetryData{
			DeviceId:     deviceId,
			Timestamp:    timestamppb.New(now.Add(time.Duration(-count+i) * time.Minute)),
			CpuUsage:     rand.Float64() * 100,
			MemoryUsage:  rand.Float64() * 100,
			DiskUsage:    20 + rand.Float64()*60,
			NetworkUsage: rand.Float64() * 100,
			Temperature:  35 + rand.Float64()*15,
			CustomMetrics: map[string]float64{
				"requests_per_second": rand.Float64() * 1000,
				"active_connections":  rand.Float64() * 100,
			},
		}
	}

	return data
}

func generateMockLogs(deviceIds []string, count int) []*fleetpb.TelemetryLogEntry {
	if len(deviceIds) == 0 {
		deviceIds = []string{"device-001", "device-002", "device-003"}
	}

	messages := []string{
		"Device connected successfully",
		"Telemetry data transmitted",
		"Health check passed",
		"Configuration updated",
		"High memory usage detected",
		"Network latency spike",
		"Deployment completed",
		"Cache cleared",
		"Service restarted",
	}

	logs := make([]*fleetpb.TelemetryLogEntry, count)
	now := time.Now()

	for i := 0; i < count; i++ {
		logs[i] = &fleetpb.TelemetryLogEntry{
			Id:        fmt.Sprintf("log-%d", i),
			DeviceId:  deviceIds[rand.Intn(len(deviceIds))],
			Timestamp: timestamppb.New(now.Add(time.Duration(-count+i) * 30 * time.Second)),
			Level:     randomLogLevel(),
			Message:   messages[rand.Intn(len(messages))],
			Metadata:  map[string]string{},
		}
	}

	return logs
}

func randomLogLevel() fleetpb.LogLevel {
	levels := []fleetpb.LogLevel{
		fleetpb.LogLevel_LOG_LEVEL_DEBUG,
		fleetpb.LogLevel_LOG_LEVEL_INFO,
		fleetpb.LogLevel_LOG_LEVEL_INFO,
		fleetpb.LogLevel_LOG_LEVEL_INFO,
		fleetpb.LogLevel_LOG_LEVEL_WARN,
		fleetpb.LogLevel_LOG_LEVEL_ERROR,
	}
	return levels[rand.Intn(len(levels))]
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && s[0:len(substr)] == substr) ||
		(len(s) > len(substr) && contains(s[1:], substr)))
}
