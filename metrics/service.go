package metrics

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"connectrpc.com/connect"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"
	"github.com/influxdata/influxdb-client-go/v2/api/write"

	metricspb "fleetd.sh/gen/metrics/v1"
)

type MetricsService struct {
	writeAPI api.WriteAPIBlocking
	org      string
	bucket   string
}

func NewMetricsService(client influxdb2.Client, org, bucket string) *MetricsService {
	writeAPI := client.WriteAPIBlocking(org, bucket)
	return &MetricsService{
		writeAPI: writeAPI,
		org:      org,
		bucket:   bucket,
	}
}

func (s *MetricsService) SendMetrics(
	ctx context.Context,
	req *connect.Request[metricspb.SendMetricsRequest],
) (*connect.Response[metricspb.SendMetricsResponse], error) {
	deviceID := req.Msg.DeviceId
	metrics := req.Msg.Metrics

	if deviceID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid device ID"))
	}

	if len(metrics) == 0 {
		return connect.NewResponse(&metricspb.SendMetricsResponse{
			Success: true,
			Message: "No metrics to process",
		}), nil
	}

	points := make([]*write.Point, 0, len(metrics))
	for _, metric := range metrics {
		if metric.Measurement == "" || metric.Timestamp == nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid metric data"))
		}

		fields := make(map[string]interface{}, len(metric.Fields))
		for k, v := range metric.Fields {
			fields[k] = v
		}
		p := write.NewPoint(
			metric.Measurement,
			metric.Tags,
			fields,
			metric.Timestamp.AsTime(),
		)
		points = append(points, p)
	}

	// Set a timeout for the write operation
	writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := s.writeAPI.WritePoint(writeCtx, points...); err != nil {
		slog.With(
			"error", err,
			"count", len(points),
		).Error("Failed to write points")

		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to process metrics: %w", err))
	}

	slog.With(
		"deviceID", deviceID,
		"count", len(metrics),
	).Info("Metrics stored successfully")

	return connect.NewResponse(&metricspb.SendMetricsResponse{
		Success: true,
		Message: "Metrics stored successfully",
	}), nil
}
