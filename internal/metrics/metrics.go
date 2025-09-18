package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// HTTP metrics
	HTTPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "fleetd_http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"service", "method", "endpoint", "status"},
	)

	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "fleetd_http_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"service", "method", "endpoint"},
	)

	HTTPRequestSize = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "fleetd_http_request_size_bytes",
			Help:    "HTTP request size in bytes",
			Buckets: []float64{100, 1000, 10000, 100000, 1000000},
		},
		[]string{"service", "method", "endpoint"},
	)

	HTTPResponseSize = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "fleetd_http_response_size_bytes",
			Help:    "HTTP response size in bytes",
			Buckets: []float64{100, 1000, 10000, 100000, 1000000},
		},
		[]string{"service", "method", "endpoint"},
	)

	// Database metrics
	DBQueriesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "fleetd_db_queries_total",
			Help: "Total number of database queries",
		},
		[]string{"service", "operation", "table"},
	)

	DBQueryDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "fleetd_db_query_duration_seconds",
			Help:    "Database query duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"service", "operation", "table"},
	)

	DBConnectionsActive = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "fleetd_db_connections_active",
			Help: "Number of active database connections",
		},
		[]string{"service"},
	)

	// Device metrics
	DevicesTotal = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "fleetd_devices_total",
			Help: "Total number of devices",
		},
		[]string{"status", "fleet"},
	)

	DevicesConnected = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "fleetd_devices_connected",
			Help: "Number of currently connected devices",
		},
	)

	DeviceHeartbeatsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "fleetd_device_heartbeats_total",
			Help: "Total number of device heartbeats",
		},
		[]string{"device_id", "fleet"},
	)

	// Fleet metrics
	FleetsTotal = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "fleetd_fleets_total",
			Help: "Total number of fleets",
		},
	)

	DeploymentsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "fleetd_deployments_total",
			Help: "Total number of deployments",
		},
		[]string{"fleet", "status"},
	)

	DeploymentDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "fleetd_deployment_duration_seconds",
			Help:    "Deployment duration in seconds",
			Buckets: []float64{10, 30, 60, 120, 300, 600, 1800, 3600},
		},
		[]string{"fleet"},
	)

	// Update metrics
	UpdatesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "fleetd_updates_total",
			Help: "Total number of updates",
		},
		[]string{"type", "status"},
	)

	UpdateBytesTransferred = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "fleetd_update_bytes_transferred_total",
			Help: "Total bytes transferred for updates",
		},
		[]string{"type"},
	)

	// Error metrics
	ErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "fleetd_errors_total",
			Help: "Total number of errors",
		},
		[]string{"service", "type", "operation"},
	)

	// System metrics
	SystemUptime = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "fleetd_system_uptime_seconds",
			Help: "System uptime in seconds",
		},
		[]string{"service"},
	)

	SystemMemoryUsage = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "fleetd_system_memory_usage_bytes",
			Help: "System memory usage in bytes",
		},
		[]string{"service", "type"},
	)

	SystemGoroutines = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "fleetd_system_goroutines",
			Help: "Number of goroutines",
		},
		[]string{"service"},
	)
)

// RecordHTTPRequest records HTTP request metrics
func RecordHTTPRequest(service, method, endpoint, status string, duration float64, reqSize, respSize float64) {
	HTTPRequestsTotal.WithLabelValues(service, method, endpoint, status).Inc()
	HTTPRequestDuration.WithLabelValues(service, method, endpoint).Observe(duration)
	if reqSize > 0 {
		HTTPRequestSize.WithLabelValues(service, method, endpoint).Observe(reqSize)
	}
	if respSize > 0 {
		HTTPResponseSize.WithLabelValues(service, method, endpoint).Observe(respSize)
	}
}

// RecordDBQuery records database query metrics
func RecordDBQuery(service, operation, table string, duration float64) {
	DBQueriesTotal.WithLabelValues(service, operation, table).Inc()
	DBQueryDuration.WithLabelValues(service, operation, table).Observe(duration)
}

// RecordError records error metrics
func RecordError(service, errType, operation string) {
	ErrorsTotal.WithLabelValues(service, errType, operation).Inc()
}
