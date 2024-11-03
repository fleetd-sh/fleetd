package metricsclient_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	metricsrpc "fleetd.sh/gen/metrics/v1/metricsv1connect"
	"fleetd.sh/internal/config"
	"fleetd.sh/internal/testutil/containers"
	"fleetd.sh/metrics"
	"fleetd.sh/pkg/metricsclient"
)

func TestMetricsClient_Integration(t *testing.T) {
	if testing.Short() || config.GetIntFromEnv("INTEGRATION", 0) != 1 {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Start InfluxDB container
	influxContainer, err := containers.NewInfluxDBContainer(ctx)
	require.NoError(t, err)
	t.Cleanup(func() {
		if err := influxContainer.Close(); err != nil {
			t.Logf("failed to close InfluxDB container: %v", err)
		}
	})

	// Set up metrics service and server
	metricsService := metrics.NewMetricsService(
		influxContainer.Client,
		influxContainer.Org,
		influxContainer.Bucket,
	)
	metricsPath, metricsHandler := metricsrpc.NewMetricsServiceHandler(metricsService)

	mux := http.NewServeMux()
	mux.Handle(metricsPath, metricsHandler)
	server := httptest.NewServer(mux)
	defer server.Close()

	client := metricsclient.NewClient(server.URL)

	t.Run("SendAndRetrieveMetrics", func(t *testing.T) {
		ctx := context.Background()

		// Send metric
		err := client.SendMetrics(ctx, []*metricsclient.Metric{
			{
				DeviceID:    "test-device-001",
				Measurement: "temperature",
				Fields:      map[string]float64{"value": 25.5},
				Tags:        map[string]string{"device_id": "test-device-001", "type": "temperature"},
				Timestamp:   time.Now(),
			},
		}, "s")
		require.NoError(t, err)

		// Wait for metrics to be processed
		time.Sleep(2 * time.Second)

		// Query with retry
		var metrics []*metricsclient.Metric
		require.Eventually(t, func() bool {
			metricsCh, errCh := client.GetMetrics(
				ctx,
				&metricsclient.MetricQuery{
					DeviceID:    "test-device-001",
					Measurement: "temperature",
					StartTime:   time.Now().Add(-1 * time.Hour),
					EndTime:     time.Now(),
				},
			)

			for m := range metricsCh {
				metrics = append(metrics, m)
			}
			err := <-errCh
			return err == nil && len(metrics) > 0
		}, 5*time.Second, 500*time.Millisecond, "Metrics not available after waiting")

		assert.NotEmpty(t, metrics)
		assert.Equal(t, 25.5, metrics[0].Fields["value"])
	})
}
