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
	timestamppb "google.golang.org/protobuf/types/known/timestamppb"
)

type MetricsService struct {
	writeAPI api.WriteAPIBlocking
	queryAPI api.QueryAPI
	org      string
	bucket   string
}

func NewMetricsService(client influxdb2.Client, org, bucket string) *MetricsService {
	writeAPI := client.WriteAPIBlocking(org, bucket)
	queryAPI := client.QueryAPI(org)
	return &MetricsService{
		writeAPI: writeAPI,
		queryAPI: queryAPI,
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

func (s *MetricsService) GetMetrics(
	ctx context.Context,
	req *connect.Request[metricspb.GetMetricsRequest],
	stream *connect.ServerStream[metricspb.GetMetricsResponse],
) error {
	startTime, endTime := req.Msg.StartTime, req.Msg.EndTime
	deviceID := req.Msg.DeviceId
	measurement := req.Msg.Measurement

	if startTime == nil || endTime == nil {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("start time and end time are required"))
	}

	// Construct the base Flux query
	query := fmt.Sprintf(`
		from(bucket:"%s")
			|> range(start: %s, stop: %s)
	`, s.bucket, startTime.AsTime().Format(time.RFC3339), endTime.AsTime().Format(time.RFC3339))

	// Add optional filters
	if measurement != "" {
		query += fmt.Sprintf(`|> filter(fn: (r) => r._measurement == "%s")`, measurement)
	}
	if deviceID != "" {
		query += fmt.Sprintf(`|> filter(fn: (r) => r.device_id == "%s")`, deviceID)
	}

	// Execute the query
	result, err := s.queryAPI.Query(ctx, query)
	if err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to query metrics: %w", err))
	}
	defer result.Close()

	// Process and stream the results
	for result.Next() {
		record := result.Record()
		metric := &metricspb.Metric{
			Measurement: record.Measurement(),
			Tags:        make(map[string]string),
			Fields:      make(map[string]float64),
			Timestamp:   timestamppb.New(record.Time()),
		}

		// Add tags
		for k, v := range record.Values() {
			if k != "_value" && k != "_field" {
				metric.Tags[k] = fmt.Sprintf("%v", v)
			}
		}

		// Add field
		fieldName := record.Field()
		fieldValue, ok := record.Value().(float64)
		if !ok {
			slog.Warn("Unexpected field value type", "field", fieldName, "value", record.Value())
			continue
		}
		metric.Fields[fieldName] = fieldValue

		// Stream the metric
		if err := stream.Send(&metricspb.GetMetricsResponse{Metric: metric}); err != nil {
			return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to send metric: %w", err))
		}
	}

	if result.Err() != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("error processing query results: %w", result.Err()))
	}

	return nil
}
