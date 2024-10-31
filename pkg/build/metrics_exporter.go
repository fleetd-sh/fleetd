package build

import (
	"context"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// InfluxDB Exporter
type InfluxDBExporter struct {
	client   influxdb2.Client
	writeAPI api.WriteAPIBlocking
	org      string
	bucket   string
	tags     map[string]string
}

func NewInfluxDBExporter(url, token, org, bucket string, tags map[string]string) *InfluxDBExporter {
	client := influxdb2.NewClient(url, token)
	writeAPI := client.WriteAPIBlocking(org, bucket)

	return &InfluxDBExporter{
		client:   client,
		writeAPI: writeAPI,
		org:      org,
		bucket:   bucket,
		tags:     tags,
	}
}

func (e *InfluxDBExporter) Export(ctx context.Context, metrics *ServiceMetrics) error {
	p := influxdb2.NewPoint(
		"system_metrics",
		e.tags,
		map[string]interface{}{
			"cpu_usage":        metrics.CPU.Usage,
			"cpu_throttled":    metrics.CPU.Throttled,
			"cpu_throttled_ns": metrics.CPU.ThrottledNs,
			"memory_current":   metrics.Memory.Current,
			"memory_peak":      metrics.Memory.Peak,
			"memory_limit":     metrics.Memory.Limit,
			"memory_swapped":   metrics.Memory.Swapped,
			"io_read_bytes":    metrics.IO.ReadBytes,
			"io_write_bytes":   metrics.IO.WriteBytes,
			"io_read_ops":      metrics.IO.ReadOps,
			"io_write_ops":     metrics.IO.WriteOps,
			"net_rx_bytes":     metrics.Network.RxBytes,
			"net_tx_bytes":     metrics.Network.TxBytes,
			"net_rx_packets":   metrics.Network.RxPackets,
			"net_tx_packets":   metrics.Network.TxPackets,
			"restarts":         metrics.Restarts,
			"uptime_ns":        metrics.UptimeNs,
		},
		time.Now(),
	)

	return e.writeAPI.WritePoint(ctx, p)
}

func (e *InfluxDBExporter) Close() error {
	e.client.Close()
	return nil
}

// Prometheus Exporter
type PrometheusExporter struct {
	cpuUsage     *prometheus.GaugeVec
	memoryUsage  *prometheus.GaugeVec
	restarts     *prometheus.CounterVec
	ioReadBytes  *prometheus.CounterVec
	ioWriteBytes *prometheus.CounterVec
	netRxBytes   *prometheus.CounterVec
	netTxBytes   *prometheus.CounterVec
	labels       map[string]string
}

func NewPrometheusExporter(namespace string, labels map[string]string) *PrometheusExporter {
	labelNames := make([]string, 0, len(labels))
	for k := range labels {
		labelNames = append(labelNames, k)
	}

	return &PrometheusExporter{
		labels: labels,
		cpuUsage: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "cpu_usage_percent",
				Help:      "CPU usage in percentage",
			},
			labelNames,
		),
		memoryUsage: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "memory_usage_bytes",
				Help:      "Memory usage in bytes",
			},
			labelNames,
		),
		restarts: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "restarts_total",
				Help:      "Total number of restarts",
			},
			labelNames,
		),
		ioReadBytes: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "io_read_bytes_total",
				Help:      "Total bytes read from disk",
			},
			labelNames,
		),
		ioWriteBytes: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "io_write_bytes_total",
				Help:      "Total bytes written to disk",
			},
			labelNames,
		),
		netRxBytes: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "network_receive_bytes_total",
				Help:      "Total bytes received over network",
			},
			labelNames,
		),
		netTxBytes: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "network_transmit_bytes_total",
				Help:      "Total bytes transmitted over network",
			},
			labelNames,
		),
	}
}

func (e *PrometheusExporter) Export(ctx context.Context, metrics *ServiceMetrics) error {
	// Convert labels map to slice of values in the same order as labelNames
	labelValues := make([]string, 0, len(e.labels))
	for _, v := range e.labels {
		labelValues = append(labelValues, v)
	}

	e.cpuUsage.WithLabelValues(labelValues...).Set(metrics.CPU.Usage)
	e.memoryUsage.WithLabelValues(labelValues...).Set(float64(metrics.Memory.Current))
	e.restarts.WithLabelValues(labelValues...).Add(float64(metrics.Restarts))
	e.ioReadBytes.WithLabelValues(labelValues...).Add(float64(metrics.IO.ReadBytes))
	e.ioWriteBytes.WithLabelValues(labelValues...).Add(float64(metrics.IO.WriteBytes))
	e.netRxBytes.WithLabelValues(labelValues...).Add(float64(metrics.Network.RxBytes))
	e.netTxBytes.WithLabelValues(labelValues...).Add(float64(metrics.Network.TxBytes))

	return nil
}

func (e *PrometheusExporter) Close() error {
	// Prometheus metrics are automatically deregistered when the process exits
	return nil
}
