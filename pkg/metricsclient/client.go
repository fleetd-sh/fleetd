package metricsclient

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	metricspb "fleetd.sh/gen/metrics/v1"
	metricsrpc "fleetd.sh/gen/metrics/v1/metricsv1connect"
)

type Metric struct {
	DeviceID    string
	Measurement string
	Timestamp   time.Time
	Tags        map[string]string
	Fields      map[string]float64
}

type MetricQuery struct {
	DeviceID    string
	Measurement string
	StartTime   time.Time
	EndTime     time.Time
	Aggregation string
	GroupBy     []string
}

type Client struct {
	client metricsrpc.MetricsServiceClient
	logger *slog.Logger
}

func NewClient(baseURL string) *Client {
	return &Client{
		client: metricsrpc.NewMetricsServiceClient(
			http.DefaultClient,
			baseURL,
		),
		logger: slog.Default(),
	}
}

func (c *Client) SendMetrics(ctx context.Context, metrics []*Metric, precision string) error {
	pbMetrics := make([]*metricspb.Metric, len(metrics))
	for i, m := range metrics {
		pbMetrics[i] = &metricspb.Metric{
			DeviceId:    m.DeviceID,
			Measurement: m.Measurement,
			Timestamp:   timestamppb.New(m.Timestamp),
			Tags:        m.Tags,
			Fields:      m.Fields,
		}
	}

	req := connect.NewRequest(&metricspb.SendMetricsRequest{
		Metrics:   pbMetrics,
		Precision: precision,
	})

	_, err := c.client.SendMetrics(ctx, req)
	return err
}

func (c *Client) GetMetrics(ctx context.Context, query *MetricQuery) (<-chan *Metric, <-chan error) {
	req := connect.NewRequest(&metricspb.GetMetricsRequest{
		DeviceId:    query.DeviceID,
		Measurement: query.Measurement,
		StartTime:   timestamppb.New(query.StartTime),
		EndTime:     timestamppb.New(query.EndTime),
	})

	stream, err := c.client.GetMetrics(ctx, req)
	if err != nil {
		errCh := make(chan error, 1)
		errCh <- err
		return nil, errCh
	}

	metricCh := make(chan *Metric)
	errCh := make(chan error, 1)

	go func() {
		defer close(metricCh)
		defer close(errCh)

		for stream.Receive() {
			deviceID := stream.Msg().Metric.Tags["device_id"]
			metricCh <- &Metric{
				DeviceID:    deviceID,
				Measurement: stream.Msg().Metric.Measurement,
				Timestamp:   stream.Msg().Metric.Timestamp.AsTime(),
				Tags:        stream.Msg().Metric.Tags,
				Fields:      stream.Msg().Metric.Fields,
			}
		}

		if err := stream.Err(); err != nil {
			errCh <- err
		}
	}()

	return metricCh, errCh
}
