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
	"google.golang.org/protobuf/types/known/timestamppb"

	metricspb "fleetd.sh/gen/metrics/v1"
	"fleetd.sh/internal/telemetry"
)

type MetricsService struct {
	writeAPI api.WriteAPIBlocking
	queryAPI api.QueryAPI
	org      string
	bucket   string
}

func NewMetricsService(client influxdb2.Client, org, bucket string) *MetricsService {
	return &MetricsService{
		writeAPI: client.WriteAPIBlocking(org, bucket),
		queryAPI: client.QueryAPI(org),
		org:      org,
		bucket:   bucket,
	}
}

func (s *MetricsService) SendMetrics(
	ctx context.Context,
	req *connect.Request[metricspb.SendMetricsRequest],
) (*connect.Response[metricspb.SendMetricsResponse], error) {
	defer telemetry.TrackSQLOperation(ctx, "SendMetrics")(nil)

	points := make([]*write.Point, 0, len(req.Msg.Metrics))
	for _, m := range req.Msg.Metrics {
		fields := make(map[string]interface{})
		for k, v := range m.Fields {
			fields[k] = v
		}

		tags := make(map[string]string)
		tags["device_id"] = m.DeviceId

		for k, v := range m.Tags {
			tags[k] = v
		}

		p := influxdb2.NewPoint(
			m.Measurement,
			tags,
			fields,
			time.Now(),
		)
		points = append(points, p)
	}

	err := s.writeAPI.WritePoint(ctx, points...)
	if err != nil {
		return nil, fmt.Errorf("failed to write metrics: %w", err)
	}

	return connect.NewResponse(&metricspb.SendMetricsResponse{
		Success: true,
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

	defer telemetry.TrackInfluxOperation(ctx, "GetMetrics")(nil)

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
