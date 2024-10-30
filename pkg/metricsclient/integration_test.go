package metricsclient_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	metricspb "fleetd.sh/gen/metrics/v1"
	"fleetd.sh/internal/config"
	"fleetd.sh/pkg/metricsclient"
)

func TestMetricsClient_Integration(t *testing.T) {
	if testing.Short() || config.GetIntFromEnv("INTEGRATION", 0) != 1 {
		t.Skip("Skipping integration test in short mode")
	}

	client := metricsclient.NewClient("http://localhost:50053")
	ctx := context.Background()

	t.Run("SendAndRetrieveMetrics", func(t *testing.T) {
		deviceID := "test-device-001"
		metric := &metricspb.Metric{
			Measurement: "temperature",
			Fields:      map[string]float64{"value": 25.5},
			Timestamp:   timestamppb.Now(),
		}

		// Send metrics
		success, err := client.SendMetrics(ctx, deviceID, []*metricspb.Metric{metric})
		require.NoError(t, err)
		assert.True(t, success)

		// Wait for metrics to be processed
		time.Sleep(time.Second)

		// Retrieve metrics
		from := timestamppb.New(time.Now().Add(-1 * time.Hour))
		to := timestamppb.Now()

		metricCh, errCh := client.GetMetrics(ctx, deviceID, "temperature", from, to)

		var receivedMetrics []*metricspb.Metric
		for m := range metricCh {
			receivedMetrics = append(receivedMetrics, m)
		}

		require.NoError(t, <-errCh)
		require.NotEmpty(t, receivedMetrics)
		assert.Equal(t, metric.Measurement, receivedMetrics[0].Measurement)
	})
}
