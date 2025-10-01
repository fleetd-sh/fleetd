package observability

import (
	"context"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type MetricsCollector struct {
	registry *prometheus.Registry

	// Device metrics
	devicesTotal         *prometheus.GaugeVec
	devicesOnline        *prometheus.GaugeVec
	deviceRegistrations  *prometheus.CounterVec
	deviceHeartbeats     *prometheus.CounterVec
	deviceErrors         *prometheus.CounterVec

	// Update metrics
	updatesInitiated     *prometheus.CounterVec
	updatesSuccessful    *prometheus.CounterVec
	updatesFailed        *prometheus.CounterVec
	updateDuration       *prometheus.HistogramVec
	rollbacksPerformed   *prometheus.CounterVec

	// API metrics
	httpRequestsTotal    *prometheus.CounterVec
	httpRequestDuration  *prometheus.HistogramVec
	httpResponseSize     *prometheus.HistogramVec
	activeConnections    prometheus.Gauge

	// System metrics
	cpuUsage            *prometheus.GaugeVec
	memoryUsage         *prometheus.GaugeVec
	diskUsage           *prometheus.GaugeVec
	networkBytesRx      *prometheus.CounterVec
	networkBytesTx      *prometheus.CounterVec

	// Database metrics
	dbConnectionsActive  prometheus.Gauge
	dbConnectionsIdle    prometheus.Gauge
	dbQueryDuration     *prometheus.HistogramVec
	dbQueryErrors       *prometheus.CounterVec
	dbTransactions      *prometheus.CounterVec

	// Business metrics
	metricsIngested     *prometheus.CounterVec
	metricsBuffered     prometheus.Gauge
	commandsExecuted    *prometheus.CounterVec
	commandsFailed      *prometheus.CounterVec

	// Agent metrics (for device-side)
	agentUptime         prometheus.Counter
	agentRestarts       prometheus.Counter
	agentStateChanges   *prometheus.CounterVec
	agentConfigReloads  prometheus.Counter

	// Rate limiting metrics
	rateLimitRequests     *prometheus.CounterVec
	rateLimitRejections   *prometheus.CounterVec
	rateLimitBans         *prometheus.CounterVec
	rateLimitActiveVisitors prometheus.Gauge
	circuitBreakerState   *prometheus.GaugeVec
	circuitBreakerTrips   *prometheus.CounterVec

	// Certificate metrics
	certificateExpiry     *prometheus.GaugeVec
	certificateRenewals   *prometheus.CounterVec
	certificateErrors     *prometheus.CounterVec
	certificateValid      *prometheus.GaugeVec
}

func NewMetricsCollector() *MetricsCollector {
	registry := prometheus.NewRegistry()

	mc := &MetricsCollector{
		registry: registry,

		// Device metrics
		devicesTotal: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "fleetd",
				Subsystem: "devices",
				Name:      "total",
				Help:      "Total number of registered devices",
			},
			[]string{"fleet_id", "device_type", "status"},
		),

		devicesOnline: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "fleetd",
				Subsystem: "devices",
				Name:      "online",
				Help:      "Number of online devices",
			},
			[]string{"fleet_id", "device_type"},
		),

		deviceRegistrations: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "fleetd",
				Subsystem: "devices",
				Name:      "registrations_total",
				Help:      "Total number of device registrations",
			},
			[]string{"fleet_id", "result"},
		),

		deviceHeartbeats: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "fleetd",
				Subsystem: "devices",
				Name:      "heartbeats_total",
				Help:      "Total number of device heartbeats received",
			},
			[]string{"fleet_id", "device_id"},
		),

		deviceErrors: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "fleetd",
				Subsystem: "devices",
				Name:      "errors_total",
				Help:      "Total number of device errors",
			},
			[]string{"fleet_id", "device_id", "error_type"},
		),

		// Update metrics
		updatesInitiated: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "fleetd",
				Subsystem: "updates",
				Name:      "initiated_total",
				Help:      "Total number of updates initiated",
			},
			[]string{"fleet_id", "update_type", "version"},
		),

		updatesSuccessful: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "fleetd",
				Subsystem: "updates",
				Name:      "successful_total",
				Help:      "Total number of successful updates",
			},
			[]string{"fleet_id", "update_type", "version"},
		),

		updatesFailed: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "fleetd",
				Subsystem: "updates",
				Name:      "failed_total",
				Help:      "Total number of failed updates",
			},
			[]string{"fleet_id", "update_type", "version", "reason"},
		),

		updateDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "fleetd",
				Subsystem: "updates",
				Name:      "duration_seconds",
				Help:      "Update duration in seconds",
				Buckets:   []float64{10, 30, 60, 120, 300, 600, 1800, 3600},
			},
			[]string{"fleet_id", "update_type"},
		),

		rollbacksPerformed: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "fleetd",
				Subsystem: "updates",
				Name:      "rollbacks_total",
				Help:      "Total number of rollbacks performed",
			},
			[]string{"fleet_id", "reason"},
		),

		// API metrics
		httpRequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "fleetd",
				Subsystem: "http",
				Name:      "requests_total",
				Help:      "Total number of HTTP requests",
			},
			[]string{"method", "endpoint", "status"},
		),

		httpRequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "fleetd",
				Subsystem: "http",
				Name:      "request_duration_seconds",
				Help:      "HTTP request duration in seconds",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"method", "endpoint"},
		),

		httpResponseSize: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "fleetd",
				Subsystem: "http",
				Name:      "response_size_bytes",
				Help:      "HTTP response size in bytes",
				Buckets:   prometheus.ExponentialBuckets(100, 10, 8),
			},
			[]string{"method", "endpoint"},
		),

		activeConnections: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "fleetd",
				Subsystem: "http",
				Name:      "active_connections",
				Help:      "Number of active HTTP connections",
			},
		),

		// System metrics
		cpuUsage: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "fleetd",
				Subsystem: "system",
				Name:      "cpu_usage_percent",
				Help:      "CPU usage percentage",
			},
			[]string{"core"},
		),

		memoryUsage: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "fleetd",
				Subsystem: "system",
				Name:      "memory_usage_bytes",
				Help:      "Memory usage in bytes",
			},
			[]string{"type"}, // rss, heap, stack
		),

		diskUsage: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "fleetd",
				Subsystem: "system",
				Name:      "disk_usage_bytes",
				Help:      "Disk usage in bytes",
			},
			[]string{"path", "type"}, // used, free, total
		),

		networkBytesRx: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "fleetd",
				Subsystem: "network",
				Name:      "bytes_received_total",
				Help:      "Total network bytes received",
			},
			[]string{"interface"},
		),

		networkBytesTx: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "fleetd",
				Subsystem: "network",
				Name:      "bytes_transmitted_total",
				Help:      "Total network bytes transmitted",
			},
			[]string{"interface"},
		),

		// Database metrics
		dbConnectionsActive: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "fleetd",
				Subsystem: "database",
				Name:      "connections_active",
				Help:      "Number of active database connections",
			},
		),

		dbConnectionsIdle: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "fleetd",
				Subsystem: "database",
				Name:      "connections_idle",
				Help:      "Number of idle database connections",
			},
		),

		dbQueryDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "fleetd",
				Subsystem: "database",
				Name:      "query_duration_seconds",
				Help:      "Database query duration in seconds",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"query_type", "table"},
		),

		dbQueryErrors: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "fleetd",
				Subsystem: "database",
				Name:      "query_errors_total",
				Help:      "Total number of database query errors",
			},
			[]string{"query_type", "error"},
		),

		dbTransactions: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "fleetd",
				Subsystem: "database",
				Name:      "transactions_total",
				Help:      "Total number of database transactions",
			},
			[]string{"status"}, // committed, rolled_back
		),

		// Business metrics
		metricsIngested: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "fleetd",
				Subsystem: "telemetry",
				Name:      "metrics_ingested_total",
				Help:      "Total number of metrics ingested",
			},
			[]string{"device_id", "metric_type"},
		),

		metricsBuffered: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "fleetd",
				Subsystem: "telemetry",
				Name:      "metrics_buffered",
				Help:      "Number of metrics currently buffered",
			},
		),

		commandsExecuted: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "fleetd",
				Subsystem: "commands",
				Name:      "executed_total",
				Help:      "Total number of commands executed",
			},
			[]string{"command_type", "device_id"},
		),

		commandsFailed: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "fleetd",
				Subsystem: "commands",
				Name:      "failed_total",
				Help:      "Total number of failed commands",
			},
			[]string{"command_type", "device_id", "reason"},
		),

		// Agent metrics
		agentUptime: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: "fleetd",
				Subsystem: "agent",
				Name:      "uptime_seconds_total",
				Help:      "Total agent uptime in seconds",
			},
		),

		agentRestarts: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: "fleetd",
				Subsystem: "agent",
				Name:      "restarts_total",
				Help:      "Total number of agent restarts",
			},
		),

		agentStateChanges: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "fleetd",
				Subsystem: "agent",
				Name:      "state_changes_total",
				Help:      "Total number of agent state changes",
			},
			[]string{"from_state", "to_state"},
		),

		agentConfigReloads: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: "fleetd",
				Subsystem: "agent",
				Name:      "config_reloads_total",
				Help:      "Total number of configuration reloads",
			},
		),

		// Rate limiting metrics
		rateLimitRequests: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "fleetd",
				Subsystem: "ratelimit",
				Name:      "requests_total",
				Help:      "Total number of rate limited requests",
			},
			[]string{"endpoint", "result"}, // result: "allowed", "rejected", "banned"
		),

		rateLimitRejections: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "fleetd",
				Subsystem: "ratelimit",
				Name:      "rejections_total",
				Help:      "Total number of rejected requests due to rate limiting",
			},
			[]string{"endpoint", "reason"}, // reason: "rate_exceeded", "circuit_open", "banned"
		),

		rateLimitBans: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "fleetd",
				Subsystem: "ratelimit",
				Name:      "bans_total",
				Help:      "Total number of IP bans",
			},
			[]string{"reason"}, // reason: "rate_abuse", "suspicious_activity", "ddos"
		),

		rateLimitActiveVisitors: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "fleetd",
				Subsystem: "ratelimit",
				Name:      "active_visitors",
				Help:      "Current number of active visitors being tracked",
			},
		),

		circuitBreakerState: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "fleetd",
				Subsystem: "circuit_breaker",
				Name:      "state",
				Help:      "Current state of circuit breaker (0=closed, 1=half-open, 2=open)",
			},
			[]string{"service"},
		),

		circuitBreakerTrips: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "fleetd",
				Subsystem: "circuit_breaker",
				Name:      "trips_total",
				Help:      "Total number of circuit breaker trips",
			},
			[]string{"service", "reason"},
		),

		// Certificate metrics
		certificateExpiry: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "fleetd",
				Subsystem: "certificates",
				Name:      "expiry_timestamp_seconds",
				Help:      "Unix timestamp when certificate expires",
			},
			[]string{"domain", "issuer"},
		),

		certificateRenewals: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "fleetd",
				Subsystem: "certificates",
				Name:      "renewals_total",
				Help:      "Total number of certificate renewals",
			},
			[]string{"domain", "result"}, // result: "success", "failure"
		),

		certificateErrors: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "fleetd",
				Subsystem: "certificates",
				Name:      "errors_total",
				Help:      "Total number of certificate errors",
			},
			[]string{"domain", "error_type"},
		),

		certificateValid: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "fleetd",
				Subsystem: "certificates",
				Name:      "valid",
				Help:      "Whether certificate is currently valid (1=valid, 0=invalid)",
			},
			[]string{"domain"},
		),
	}

	// Register all metrics
	registry.MustRegister(
		mc.devicesTotal,
		mc.devicesOnline,
		mc.deviceRegistrations,
		mc.deviceHeartbeats,
		mc.deviceErrors,
		mc.updatesInitiated,
		mc.updatesSuccessful,
		mc.updatesFailed,
		mc.updateDuration,
		mc.rollbacksPerformed,
		mc.httpRequestsTotal,
		mc.httpRequestDuration,
		mc.httpResponseSize,
		mc.activeConnections,
		mc.cpuUsage,
		mc.memoryUsage,
		mc.diskUsage,
		mc.networkBytesRx,
		mc.networkBytesTx,
		mc.dbConnectionsActive,
		mc.dbConnectionsIdle,
		mc.dbQueryDuration,
		mc.dbQueryErrors,
		mc.dbTransactions,
		mc.metricsIngested,
		mc.metricsBuffered,
		mc.commandsExecuted,
		mc.commandsFailed,
		mc.agentUptime,
		mc.agentRestarts,
		mc.agentStateChanges,
		mc.agentConfigReloads,
		mc.rateLimitRequests,
		mc.rateLimitRejections,
		mc.rateLimitBans,
		mc.rateLimitActiveVisitors,
		mc.circuitBreakerState,
		mc.circuitBreakerTrips,
		mc.certificateExpiry,
		mc.certificateRenewals,
		mc.certificateErrors,
		mc.certificateValid,
	)

	// Register standard Go metrics
	registry.MustRegister(collectors.NewGoCollector())
	registry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	return mc
}

// RecordHTTPRequest records HTTP request metrics
func (mc *MetricsCollector) RecordHTTPRequest(method, endpoint, status string, duration time.Duration, responseSize int) {
	mc.httpRequestsTotal.WithLabelValues(method, endpoint, status).Inc()
	mc.httpRequestDuration.WithLabelValues(method, endpoint).Observe(duration.Seconds())
	mc.httpResponseSize.WithLabelValues(method, endpoint).Observe(float64(responseSize))
}

// RecordDeviceRegistration records device registration
func (mc *MetricsCollector) RecordDeviceRegistration(fleetID string, success bool) {
	result := "success"
	if !success {
		result = "failure"
	}
	mc.deviceRegistrations.WithLabelValues(fleetID, result).Inc()
}

// RecordDeviceHeartbeat records device heartbeat
func (mc *MetricsCollector) RecordDeviceHeartbeat(fleetID, deviceID string) {
	mc.deviceHeartbeats.WithLabelValues(fleetID, deviceID).Inc()
}

// RecordUpdateMetrics records update-related metrics
func (mc *MetricsCollector) RecordUpdateStart(fleetID, updateType, version string) {
	mc.updatesInitiated.WithLabelValues(fleetID, updateType, version).Inc()
}

func (mc *MetricsCollector) RecordUpdateComplete(fleetID, updateType, version string, success bool, duration time.Duration, reason string) {
	if success {
		mc.updatesSuccessful.WithLabelValues(fleetID, updateType, version).Inc()
	} else {
		mc.updatesFailed.WithLabelValues(fleetID, updateType, version, reason).Inc()
	}
	mc.updateDuration.WithLabelValues(fleetID, updateType).Observe(duration.Seconds())
}

// RecordDatabaseQuery records database query metrics
func (mc *MetricsCollector) RecordDatabaseQuery(queryType, table string, duration time.Duration, err error) {
	mc.dbQueryDuration.WithLabelValues(queryType, table).Observe(duration.Seconds())
	if err != nil {
		mc.dbQueryErrors.WithLabelValues(queryType, err.Error()).Inc()
	}
}

// UpdateSystemMetrics updates system resource metrics
func (mc *MetricsCollector) UpdateSystemMetrics(cpu map[string]float64, memory map[string]float64, disk map[string]map[string]float64) {
	for core, usage := range cpu {
		mc.cpuUsage.WithLabelValues(core).Set(usage)
	}

	for memType, usage := range memory {
		mc.memoryUsage.WithLabelValues(memType).Set(usage)
	}

	for path, usage := range disk {
		for usageType, value := range usage {
			mc.diskUsage.WithLabelValues(path, usageType).Set(value)
		}
	}
}

// UpdateDeviceMetrics updates device count metrics
func (mc *MetricsCollector) UpdateDeviceMetrics(fleetID string, total, online map[string]int) {
	for deviceType, count := range total {
		mc.devicesTotal.WithLabelValues(fleetID, deviceType, "active").Set(float64(count))
	}

	for deviceType, count := range online {
		mc.devicesOnline.WithLabelValues(fleetID, deviceType).Set(float64(count))
	}
}

// IncrementAgentRestart increments agent restart counter
func (mc *MetricsCollector) IncrementAgentRestart() {
	mc.agentRestarts.Inc()
}

// RecordAgentStateChange records agent state transitions
func (mc *MetricsCollector) RecordAgentStateChange(fromState, toState string) {
	mc.agentStateChanges.WithLabelValues(fromState, toState).Inc()
}

// RecordRateLimitRequest records a rate limit request
func (mc *MetricsCollector) RecordRateLimitRequest(endpoint, result string) {
	mc.rateLimitRequests.WithLabelValues(endpoint, result).Inc()
}

// RecordRateLimitRejection records a rate limit rejection
func (mc *MetricsCollector) RecordRateLimitRejection(endpoint, reason string) {
	mc.rateLimitRejections.WithLabelValues(endpoint, reason).Inc()
	mc.rateLimitRequests.WithLabelValues(endpoint, "rejected").Inc()
}

// RecordRateLimitBan records an IP ban
func (mc *MetricsCollector) RecordRateLimitBan(reason string) {
	mc.rateLimitBans.WithLabelValues(reason).Inc()
}

// UpdateRateLimitActiveVisitors updates the count of active visitors
func (mc *MetricsCollector) UpdateRateLimitActiveVisitors(count float64) {
	mc.rateLimitActiveVisitors.Set(count)
}

// UpdateCircuitBreakerState updates circuit breaker state
func (mc *MetricsCollector) UpdateCircuitBreakerState(service, state string) {
	var value float64
	switch state {
	case "closed":
		value = 0
	case "half-open":
		value = 1
	case "open":
		value = 2
	}
	mc.circuitBreakerState.WithLabelValues(service).Set(value)
}

// RecordCircuitBreakerTrip records a circuit breaker trip
func (mc *MetricsCollector) RecordCircuitBreakerTrip(service, reason string) {
	mc.circuitBreakerTrips.WithLabelValues(service, reason).Inc()
}

// UpdateCertificateExpiry updates certificate expiry time
func (mc *MetricsCollector) UpdateCertificateExpiry(domain, issuer string, expiryTime time.Time) {
	mc.certificateExpiry.WithLabelValues(domain, issuer).Set(float64(expiryTime.Unix()))
}

// RecordCertificateRenewal records a certificate renewal attempt
func (mc *MetricsCollector) RecordCertificateRenewal(domain string, success bool) {
	result := "success"
	if !success {
		result = "failure"
	}
	mc.certificateRenewals.WithLabelValues(domain, result).Inc()
}

// RecordCertificateError records a certificate error
func (mc *MetricsCollector) RecordCertificateError(domain, errorType string) {
	mc.certificateErrors.WithLabelValues(domain, errorType).Inc()
}

// UpdateCertificateValidity updates whether a certificate is valid
func (mc *MetricsCollector) UpdateCertificateValidity(domain string, valid bool) {
	value := 0.0
	if valid {
		value = 1.0
	}
	mc.certificateValid.WithLabelValues(domain).Set(value)
}

// HTTPHandler returns the Prometheus HTTP handler
func (mc *MetricsCollector) HTTPHandler() http.Handler {
	return promhttp.HandlerFor(mc.registry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	})
}

// StartMetricsServer starts the Prometheus metrics server
func (mc *MetricsCollector) StartMetricsServer(ctx context.Context, addr string) error {
	mux := http.NewServeMux()
	mux.Handle("/metrics", mc.HTTPHandler())
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		server.Shutdown(context.Background())
	}()

	return server.ListenAndServe()
}

// MetricsMiddleware provides HTTP middleware for automatic metrics collection
func (mc *MetricsCollector) MetricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		wrapped := &responseWriter{ResponseWriter: w}

		mc.activeConnections.Inc()
		defer mc.activeConnections.Dec()

		next.ServeHTTP(wrapped, r)

		duration := time.Since(start)
		status := wrapped.Status()
		if status == 0 {
			status = 200
		}

		mc.RecordHTTPRequest(
			r.Method,
			r.URL.Path,
			fmt.Sprintf("%d", status),
			duration,
			wrapped.BytesWritten(),
		)
	})
}

type responseWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.bytes += n
	return n, err
}

func (rw *responseWriter) Status() int {
	return rw.status
}

func (rw *responseWriter) BytesWritten() int {
	return rw.bytes
}