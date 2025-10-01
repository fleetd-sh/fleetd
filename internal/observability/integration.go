package observability

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"fleetd.sh/internal/telemetry"
	"github.com/prometheus/client_golang/prometheus"
)

// IntegrationConfig holds the observability integration configuration
type IntegrationConfig struct {
	// VictoriaMetrics configuration
	VictoriaMetricsURL     string
	EnableMetrics          bool
	MetricsPushInterval    time.Duration
	MetricsBatchSize       int

	// Loki configuration
	LokiURL            string
	EnableLogsExport   bool
	LogsBatchSize      int
	LogsFlushInterval  time.Duration

	// Common settings
	DeviceID string
	OrgID    string
	Source   string
}

// Integration manages metrics and logs export to observability stack
type Integration struct {
	config    *IntegrationConfig
	vmClient  *telemetry.VictoriaMetricsClient
	lokiClient *telemetry.LokiClient

	// Metrics collectors
	deviceCount      *prometheus.GaugeVec
	telemetryReceived *prometheus.CounterVec
	apiRequests      *prometheus.CounterVec
	apiLatency       *prometheus.HistogramVec

	// Internal state
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup

	// Buffering for metrics
	metricsBuf []telemetry.MetricPoint
	metricsMux sync.Mutex
	metricsCh  chan telemetry.MetricPoint
}

// NewIntegration creates a new observability integration
func NewIntegration(config *IntegrationConfig) (*Integration, error) {
	if config == nil {
		config = &IntegrationConfig{}
	}

	// Load from environment if not provided
	if config.VictoriaMetricsURL == "" {
		config.VictoriaMetricsURL = os.Getenv("VICTORIAMETRICS_URL")
	}
	if config.LokiURL == "" {
		config.LokiURL = os.Getenv("LOKI_URL")
	}
	if config.Source == "" {
		config.Source = "device-api"
	}

	// Parse environment flags
	if os.Getenv("ENABLE_METRICS") == "true" {
		config.EnableMetrics = true
	}
	if os.Getenv("ENABLE_LOGS_EXPORT") == "true" {
		config.EnableLogsExport = true
	}

	// Set defaults
	if config.MetricsPushInterval == 0 {
		config.MetricsPushInterval = 30 * time.Second
	}
	if config.MetricsBatchSize == 0 {
		config.MetricsBatchSize = 1000
	}
	if config.LogsBatchSize == 0 {
		config.LogsBatchSize = 100
	}
	if config.LogsFlushInterval == 0 {
		config.LogsFlushInterval = 5 * time.Second
	}

	ctx, cancel := context.WithCancel(context.Background())

	i := &Integration{
		config:    config,
		ctx:       ctx,
		cancel:    cancel,
		metricsBuf: make([]telemetry.MetricPoint, 0, config.MetricsBatchSize),
		metricsCh:  make(chan telemetry.MetricPoint, 10000),
	}

	// Initialize Prometheus metrics
	i.initPrometheusMetrics()

	// Initialize VictoriaMetrics client
	if config.EnableMetrics && config.VictoriaMetricsURL != "" {
		i.vmClient = telemetry.NewVictoriaMetricsClient(config.VictoriaMetricsURL)
		slog.Info("VictoriaMetrics integration enabled",
			"url", config.VictoriaMetricsURL,
			"push_interval", config.MetricsPushInterval)
	}

	// Initialize Loki client
	if config.EnableLogsExport && config.LokiURL != "" {
		i.lokiClient = telemetry.NewLokiClient(config.LokiURL)
		slog.Info("Loki integration enabled",
			"url", config.LokiURL,
			"batch_size", config.LogsBatchSize,
			"flush_interval", config.LogsFlushInterval)

		// Hook into slog for automatic log export
		i.setupLogExport()
	}

	// Start background workers
	if i.vmClient != nil {
		i.wg.Add(1)
		go i.metricsWorker()
	}

	return i, nil
}

func (i *Integration) initPrometheusMetrics() {
	i.deviceCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "fleetd_device_count",
			Help: "Number of registered devices",
		},
		[]string{"status", "type"},
	)

	i.telemetryReceived = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "fleetd_telemetry_received_total",
			Help: "Total number of telemetry messages received",
		},
		[]string{"device_id", "type"},
	)

	i.apiRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "fleetd_api_requests_total",
			Help: "Total number of API requests",
		},
		[]string{"endpoint", "method", "status"},
	)

	i.apiLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "fleetd_api_latency_seconds",
			Help:    "API request latency in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"endpoint", "method"},
	)

	// Register metrics
	prometheus.MustRegister(i.deviceCount)
	prometheus.MustRegister(i.telemetryReceived)
	prometheus.MustRegister(i.apiRequests)
	prometheus.MustRegister(i.apiLatency)
}

// UpdateDeviceCount updates the device count metric
func (i *Integration) UpdateDeviceCount(status string, deviceType string, count float64) {
	if i.deviceCount != nil {
		i.deviceCount.WithLabelValues(status, deviceType).Set(count)
	}

	// Also send to VictoriaMetrics
	if i.vmClient != nil {
		i.metricsCh <- telemetry.MetricPoint{
			Timestamp: time.Now(),
			Name:      "fleetd_device_count",
			Value:     count,
			Labels: map[string]string{
				"status": status,
				"type":   deviceType,
			},
		}
	}
}

// RecordTelemetry records telemetry received from a device
func (i *Integration) RecordTelemetry(deviceID string, telemetryType string, metrics map[string]float64) {
	if i.telemetryReceived != nil {
		i.telemetryReceived.WithLabelValues(deviceID, telemetryType).Inc()
	}

	// Send device metrics to VictoriaMetrics
	if i.vmClient != nil {
		for name, value := range metrics {
			i.metricsCh <- telemetry.MetricPoint{
				Timestamp: time.Now(),
				Name:      fmt.Sprintf("device_%s", name),
				Value:     value,
				Labels: map[string]string{
					"device_id": deviceID,
					"type":      telemetryType,
				},
			}
		}
	}
}

// RecordAPIRequest records an API request
func (i *Integration) RecordAPIRequest(endpoint, method, status string, latency time.Duration) {
	if i.apiRequests != nil {
		i.apiRequests.WithLabelValues(endpoint, method, status).Inc()
	}
	if i.apiLatency != nil {
		i.apiLatency.WithLabelValues(endpoint, method).Observe(latency.Seconds())
	}
}

// LogDeviceEvent logs a device event to Loki
func (i *Integration) LogDeviceEvent(deviceID, level, message string, labels map[string]string) {
	if i.lokiClient == nil {
		return
	}

	entry := telemetry.LogEntry{
		Timestamp: time.Now(),
		Level:     level,
		DeviceID:  deviceID,
		OrgID:     i.config.OrgID,
		Source:    i.config.Source,
		Message:   message,
		Labels:    labels,
	}

	i.lokiClient.Log(entry)
}

// setupLogExport sets up automatic log export to Loki
func (i *Integration) setupLogExport() {
	// Create a custom slog handler that also sends to Loki
	originalHandler := slog.Default().Handler()

	lokiHandler := &lokiSlogHandler{
		inner:      originalHandler,
		lokiClient: i.lokiClient,
		source:     i.config.Source,
		deviceID:   i.config.DeviceID,
		orgID:      i.config.OrgID,
	}

	slog.SetDefault(slog.New(lokiHandler))
}

// lokiSlogHandler wraps slog handler to also send logs to Loki
type lokiSlogHandler struct {
	inner      slog.Handler
	lokiClient *telemetry.LokiClient
	source     string
	deviceID   string
	orgID      string
}

func (h *lokiSlogHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *lokiSlogHandler) Handle(ctx context.Context, record slog.Record) error {
	// Send to original handler first
	if err := h.inner.Handle(ctx, record); err != nil {
		return err
	}

	// Send to Loki
	if h.lokiClient != nil {
		entry := telemetry.LogEntry{
			Timestamp: record.Time,
			Level:     record.Level.String(),
			DeviceID:  h.deviceID,
			OrgID:     h.orgID,
			Source:    h.source,
			Message:   record.Message,
			Labels:    make(map[string]string),
		}

		// Add attributes as labels
		record.Attrs(func(attr slog.Attr) bool {
			entry.Labels[attr.Key] = fmt.Sprintf("%v", attr.Value.Any())
			return true
		})

		h.lokiClient.Log(entry)
	}

	return nil
}

func (h *lokiSlogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &lokiSlogHandler{
		inner:      h.inner.WithAttrs(attrs),
		lokiClient: h.lokiClient,
		source:     h.source,
		deviceID:   h.deviceID,
		orgID:      h.orgID,
	}
}

func (h *lokiSlogHandler) WithGroup(name string) slog.Handler {
	return &lokiSlogHandler{
		inner:      h.inner.WithGroup(name),
		lokiClient: h.lokiClient,
		source:     h.source,
		deviceID:   h.deviceID,
		orgID:      h.orgID,
	}
}

// metricsWorker sends metrics to VictoriaMetrics in batches
func (i *Integration) metricsWorker() {
	defer i.wg.Done()

	ticker := time.NewTicker(i.config.MetricsPushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-i.ctx.Done():
			i.flushMetrics()
			return

		case point := <-i.metricsCh:
			i.metricsMux.Lock()
			i.metricsBuf = append(i.metricsBuf, point)
			if len(i.metricsBuf) >= i.config.MetricsBatchSize {
				i.flushMetricsLocked()
			}
			i.metricsMux.Unlock()

		case <-ticker.C:
			i.flushMetrics()
		}
	}
}

func (i *Integration) flushMetrics() {
	i.metricsMux.Lock()
	defer i.metricsMux.Unlock()
	i.flushMetricsLocked()
}

func (i *Integration) flushMetricsLocked() {
	if len(i.metricsBuf) == 0 {
		return
	}

	metrics := telemetry.DeviceMetrics{
		DeviceID: i.config.DeviceID,
		OrgID:    i.config.OrgID,
		Points:   i.metricsBuf,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := i.vmClient.IngestMetrics(ctx, metrics); err != nil {
		slog.Error("Failed to send metrics to VictoriaMetrics",
			"error", err,
			"count", len(i.metricsBuf))
	} else {
		slog.Debug("Successfully sent metrics to VictoriaMetrics",
			"count", len(i.metricsBuf))
	}

	// Clear buffer
	i.metricsBuf = i.metricsBuf[:0]
}

// Shutdown gracefully shuts down the integration
func (i *Integration) Shutdown() error {
	i.cancel()

	// Flush remaining metrics
	if i.vmClient != nil {
		i.flushMetrics()
		i.vmClient.Close()
	}

	// Flush remaining logs
	if i.lokiClient != nil {
		i.lokiClient.Flush()
		i.lokiClient.Close()
	}

	// Wait for workers to finish
	i.wg.Wait()

	return nil
}