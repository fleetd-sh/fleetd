package metricsclient

import (
	"context"
	"log/slog"
	"net/http"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	metricspb "fleetd.sh/gen/metrics/v1"
	metricsrpc "fleetd.sh/gen/metrics/v1/metricsv1connect"
)

type Client struct {
	client metricsrpc.MetricsServiceClient
	logger *slog.Logger
}

type ClientOption func(*Client)

func WithLogger(logger *slog.Logger) ClientOption {
	return func(c *Client) {
		c.logger = logger
	}
}

func NewClient(baseURL string, opts ...ClientOption) *Client {
	c := &Client{
		client: metricsrpc.NewMetricsServiceClient(
			http.DefaultClient,
			baseURL,
		),
		logger: slog.Default(),
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

func (c *Client) SendMetrics(ctx context.Context, deviceID string, metrics []*metricspb.Metric) (bool, error) {
	c.logger.With("deviceID", deviceID, "metricCount", len(metrics)).Info("Sending metrics")
	req := connect.NewRequest(&metricspb.SendMetricsRequest{
		DeviceId: deviceID,
		Metrics:  metrics,
	})

	resp, err := c.client.SendMetrics(ctx, req)
	if err != nil {
		return false, err
	}

	return resp.Msg.Success, nil
}

func (c *Client) GetMetrics(ctx context.Context, deviceID, measurement string, startTime, endTime *timestamppb.Timestamp) (<-chan *metricspb.Metric, <-chan error) {
	c.logger.With(
		"deviceID", deviceID,
		"measurement", measurement,
		"startTime", startTime,
		"endTime", endTime,
	).Info("Getting metrics")

	req := connect.NewRequest(&metricspb.GetMetricsRequest{
		DeviceId:    deviceID,
		Measurement: measurement,
		StartTime:   startTime,
		EndTime:     endTime,
	})

	stream, err := c.client.GetMetrics(ctx, req)
	if err != nil {
		errCh := make(chan error, 1)
		errCh <- err
		return nil, errCh
	}

	metricCh := make(chan *metricspb.Metric)
	errCh := make(chan error, 1)

	go func() {
		defer close(metricCh)
		defer close(errCh)

		for stream.Receive() {
			metricCh <- stream.Msg().Metric
		}

		if err := stream.Err(); err != nil {
			errCh <- err
		}
	}()

	return metricCh, errCh
}
