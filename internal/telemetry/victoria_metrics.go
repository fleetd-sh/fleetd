package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"
)

// VictoriaMetricsClient handles metric ingestion to VictoriaMetrics
type VictoriaMetricsClient struct {
	url        string
	httpClient *http.Client

	// Metrics about the client itself
	metricsIngested   prometheus.Counter
	metricsFailed     prometheus.Counter
	ingestionDuration prometheus.Histogram
}

// NewVictoriaMetricsClient creates a new VictoriaMetrics client
func NewVictoriaMetricsClient(url string) *VictoriaMetricsClient {
	client := &VictoriaMetricsClient{
		url: url,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		metricsIngested: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "fleetd_metrics_ingested_total",
			Help: "Total number of metrics successfully ingested",
		}),
		metricsFailed: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "fleetd_metrics_failed_total",
			Help: "Total number of metrics that failed to ingest",
		}),
		ingestionDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "fleetd_metrics_ingestion_duration_seconds",
			Help:    "Duration of metrics ingestion in seconds",
			Buckets: prometheus.DefBuckets,
		}),
	}

	// Register metrics
	prometheus.MustRegister(client.metricsIngested)
	prometheus.MustRegister(client.metricsFailed)
	prometheus.MustRegister(client.ingestionDuration)

	return client
}

// MetricPoint represents a single metric data point
type MetricPoint struct {
	Timestamp time.Time
	Name      string
	Value     float64
	Labels    map[string]string
}

// DeviceMetrics contains metrics from a device
type DeviceMetrics struct {
	DeviceID string
	OrgID    string
	Points   []MetricPoint
}

// IngestMetrics sends metrics to VictoriaMetrics using Prometheus remote write protocol
func (c *VictoriaMetricsClient) IngestMetrics(ctx context.Context, metrics DeviceMetrics) error {
	timer := prometheus.NewTimer(c.ingestionDuration)
	defer timer.ObserveDuration()

	// Convert to Prometheus format
	var buf bytes.Buffer
	for _, point := range metrics.Points {
		// Create metric line in Prometheus exposition format
		fmt.Fprintf(&buf, "%s{device_id=\"%s\",org_id=\"%s\"",
			point.Name, metrics.DeviceID, metrics.OrgID)

		// Add custom labels
		for k, v := range point.Labels {
			fmt.Fprintf(&buf, ",%s=\"%s\"", k, v)
		}

		// Add value and timestamp
		fmt.Fprintf(&buf, "} %f %d\n", point.Value, point.Timestamp.UnixMilli())
	}

	// Send to VictoriaMetrics import endpoint
	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("%s/api/v1/import/prometheus", c.url),
		&buf)
	if err != nil {
		c.metricsFailed.Add(float64(len(metrics.Points)))
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "text/plain")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.metricsFailed.Add(float64(len(metrics.Points)))
		return fmt.Errorf("failed to send metrics: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		c.metricsFailed.Add(float64(len(metrics.Points)))
		return fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	c.metricsIngested.Add(float64(len(metrics.Points)))
	return nil
}

// BatchIngest handles batch ingestion of metrics from multiple devices
func (c *VictoriaMetricsClient) BatchIngest(ctx context.Context, batch []DeviceMetrics) error {
	timer := prometheus.NewTimer(c.ingestionDuration)
	defer timer.ObserveDuration()

	var buf bytes.Buffer
	totalPoints := 0

	for _, metrics := range batch {
		for _, point := range metrics.Points {
			// Create metric line in Prometheus exposition format
			fmt.Fprintf(&buf, "%s{device_id=\"%s\",org_id=\"%s\"",
				point.Name, metrics.DeviceID, metrics.OrgID)

			// Add custom labels
			for k, v := range point.Labels {
				fmt.Fprintf(&buf, ",%s=\"%s\"", k, v)
			}

			// Add value and timestamp
			fmt.Fprintf(&buf, "} %f %d\n", point.Value, point.Timestamp.UnixMilli())
			totalPoints++
		}
	}

	// Send to VictoriaMetrics import endpoint
	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("%s/api/v1/import/prometheus", c.url),
		&buf)
	if err != nil {
		c.metricsFailed.Add(float64(totalPoints))
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "text/plain")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.metricsFailed.Add(float64(totalPoints))
		return fmt.Errorf("failed to send metrics: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		c.metricsFailed.Add(float64(totalPoints))
		return fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	c.metricsIngested.Add(float64(totalPoints))
	return nil
}

// Query executes a MetricsQL query against VictoriaMetrics
func (c *VictoriaMetricsClient) Query(ctx context.Context, query string, timestamp time.Time) (*QueryResult, error) {
	req, err := http.NewRequestWithContext(ctx, "GET",
		fmt.Sprintf("%s/api/v1/query", c.url), nil)
	if err != nil {
		return nil, err
	}

	q := req.URL.Query()
	q.Add("query", query)
	if !timestamp.IsZero() {
		q.Add("time", fmt.Sprintf("%d", timestamp.Unix()))
	}
	req.URL.RawQuery = q.Encode()

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("query failed: %s - %s", resp.Status, body)
	}

	var result QueryResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// QueryRange executes a range query against VictoriaMetrics
func (c *VictoriaMetricsClient) QueryRange(ctx context.Context, query string, start, end time.Time, step time.Duration) (*QueryResult, error) {
	req, err := http.NewRequestWithContext(ctx, "GET",
		fmt.Sprintf("%s/api/v1/query_range", c.url), nil)
	if err != nil {
		return nil, err
	}

	q := req.URL.Query()
	q.Add("query", query)
	q.Add("start", fmt.Sprintf("%d", start.Unix()))
	q.Add("end", fmt.Sprintf("%d", end.Unix()))
	q.Add("step", fmt.Sprintf("%d", int(step.Seconds())))
	req.URL.RawQuery = q.Encode()

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("query range failed: %s - %s", resp.Status, body)
	}

	var result QueryResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// QueryResult represents the result of a metrics query
type QueryResult struct {
	Status string         `json:"status"`
	Data   map[string]any `json:"data"`
}

// PrometheusHandler returns an HTTP handler for Prometheus scraping
func (c *VictoriaMetricsClient) PrometheusHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Gather metrics from the default registry
		mfs, err := prometheus.DefaultGatherer.Gather()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Encode metrics in Prometheus format
		encoder := expfmt.NewEncoder(w, expfmt.FmtText)
		for _, mf := range mfs {
			if err := encoder.Encode(mf); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
	})
}

// Close closes the client connections
func (c *VictoriaMetricsClient) Close() error {
	c.httpClient.CloseIdleConnections()
	return nil
}
