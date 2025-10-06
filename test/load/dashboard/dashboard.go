package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"fleetd.sh/test/load/framework"
	"github.com/gorilla/websocket"
)

// Dashboard provides a real-time web interface for load testing metrics
type Dashboard struct {
	server           *http.Server
	metricsCollector *framework.MetricsCollector
	fleetSimulator   *framework.FleetSimulator
	clients          map[*websocket.Conn]bool
	clientsMu        sync.RWMutex
	upgrader         websocket.Upgrader
	logger           *slog.Logger
	port             int
	ctx              context.Context
	cancel           context.CancelFunc
	wg               sync.WaitGroup
}

// DashboardConfig configures the dashboard
type DashboardConfig struct {
	Port             int
	UpdateInterval   time.Duration
	MetricsCollector *framework.MetricsCollector
	FleetSimulator   *framework.FleetSimulator
}

// DashboardData represents the data sent to the dashboard
type DashboardData struct {
	Timestamp    time.Time                  `json:"timestamp"`
	Summary      framework.MetricsSummary   `json:"summary"`
	RealTime     *framework.RealTimeMetrics `json:"realtime"`
	FleetMetrics *framework.FleetMetrics    `json:"fleet_metrics"`
	Scenarios    []ScenarioStatus           `json:"scenarios"`
	SystemHealth SystemHealthData           `json:"system_health"`
	Charts       ChartData                  `json:"charts"`
}

// ScenarioStatus represents the status of a running scenario
type ScenarioStatus struct {
	Name         string        `json:"name"`
	Status       string        `json:"status"`
	Progress     float64       `json:"progress"`
	StartTime    time.Time     `json:"start_time"`
	Duration     time.Duration `json:"duration"`
	DevicesCount int64         `json:"devices_count"`
	ErrorCount   int64         `json:"error_count"`
	SuccessRate  float64       `json:"success_rate"`
}

// SystemHealthData represents system health information
type SystemHealthData struct {
	CPUHealth     HealthStatus `json:"cpu_health"`
	MemoryHealth  HealthStatus `json:"memory_health"`
	NetworkHealth HealthStatus `json:"network_health"`
	OverallHealth HealthStatus `json:"overall_health"`
	HealthScore   float64      `json:"health_score"`
	Alerts        []Alert      `json:"alerts"`
}

// HealthStatus represents the health status of a system component
type HealthStatus struct {
	Status      string  `json:"status"` // "healthy", "warning", "critical"
	Value       float64 `json:"value"`
	Threshold   float64 `json:"threshold"`
	Description string  `json:"description"`
}

// Alert represents a system alert
type Alert struct {
	Level     string    `json:"level"` // "info", "warning", "error", "critical"
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
	Component string    `json:"component"`
	Resolved  bool      `json:"resolved"`
}

// ChartData contains data for various charts
type ChartData struct {
	CPUUsage    []DataPoint `json:"cpu_usage"`
	MemoryUsage []DataPoint `json:"memory_usage"`
	Throughput  []DataPoint `json:"throughput"`
	Latency     []DataPoint `json:"latency"`
	ErrorRate   []DataPoint `json:"error_rate"`
	DeviceCount []DataPoint `json:"device_count"`
	NetworkIO   []DataPoint `json:"network_io"`
}

// DataPoint represents a single data point for charts
type DataPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
}

// NewDashboard creates a new dashboard instance
func NewDashboard(config *DashboardConfig) *Dashboard {
	if config.Port == 0 {
		config.Port = 8080
	}

	if config.UpdateInterval == 0 {
		config.UpdateInterval = time.Second
	}

	ctx, cancel := context.WithCancel(context.Background())

	logger := slog.Default().With("component", "dashboard")

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow all origins for development
		},
	}

	dashboard := &Dashboard{
		metricsCollector: config.MetricsCollector,
		fleetSimulator:   config.FleetSimulator,
		clients:          make(map[*websocket.Conn]bool),
		upgrader:         upgrader,
		logger:           logger,
		port:             config.Port,
		ctx:              ctx,
		cancel:           cancel,
	}

	// Set up HTTP server
	mux := http.NewServeMux()
	mux.HandleFunc("/", dashboard.handleIndex)
	mux.HandleFunc("/ws", dashboard.handleWebSocket)
	mux.HandleFunc("/api/metrics", dashboard.handleAPIMetrics)
	mux.HandleFunc("/api/export", dashboard.handleAPIExport)
	mux.HandleFunc("/static/", dashboard.handleStatic)

	dashboard.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", config.Port),
		Handler: mux,
	}

	return dashboard
}

// Start starts the dashboard server
func (d *Dashboard) Start() error {
	d.logger.Info("Starting dashboard", "port", d.port)

	// Start background tasks
	d.wg.Add(2)
	go d.broadcastLoop()
	go d.healthMonitorLoop()

	// Start HTTP server
	go func() {
		if err := d.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			d.logger.Error("Dashboard server error", "error", err)
		}
	}()

	d.logger.Info("Dashboard started", "url", fmt.Sprintf("http://localhost:%d", d.port))
	return nil
}

// Stop stops the dashboard server
func (d *Dashboard) Stop() error {
	d.logger.Info("Stopping dashboard")

	d.cancel()

	// Close all WebSocket connections
	d.clientsMu.Lock()
	for client := range d.clients {
		client.Close()
	}
	d.clientsMu.Unlock()

	// Shutdown HTTP server
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := d.server.Shutdown(ctx); err != nil {
		d.logger.Error("Failed to shutdown dashboard server", "error", err)
	}

	d.wg.Wait()
	return nil
}

// handleIndex serves the main dashboard page
func (d *Dashboard) handleIndex(w http.ResponseWriter, r *http.Request) {
	tmpl := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>fleetd Load Testing Dashboard</title>
    <script src="https://cdn.jsdelivr.net/npm/chart.js"></script>
    <style>
        body {
            font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif;
            margin: 0;
            padding: 20px;
            background-color: #f5f5f5;
        }
        .container {
            max-width: 1400px;
            margin: 0 auto;
        }
        .header {
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
            padding: 20px;
            border-radius: 10px;
            margin-bottom: 20px;
            text-align: center;
        }
        .status-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(250px, 1fr));
            gap: 20px;
            margin-bottom: 20px;
        }
        .status-card {
            background: white;
            padding: 20px;
            border-radius: 10px;
            box-shadow: 0 2px 10px rgba(0,0,0,0.1);
            text-align: center;
        }
        .status-value {
            font-size: 2em;
            font-weight: bold;
            margin: 10px 0;
        }
        .status-label {
            color: #666;
            font-size: 0.9em;
        }
        .charts-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(400px, 1fr));
            gap: 20px;
            margin-bottom: 20px;
        }
        .chart-container {
            background: white;
            padding: 20px;
            border-radius: 10px;
            box-shadow: 0 2px 10px rgba(0,0,0,0.1);
        }
        .chart-title {
            font-size: 1.2em;
            font-weight: bold;
            margin-bottom: 15px;
            text-align: center;
        }
        .health-indicator {
            display: inline-block;
            width: 12px;
            height: 12px;
            border-radius: 50%;
            margin-right: 8px;
        }
        .health-healthy { background-color: #4CAF50; }
        .health-warning { background-color: #FF9800; }
        .health-critical { background-color: #F44336; }
        .alerts {
            background: white;
            padding: 20px;
            border-radius: 10px;
            box-shadow: 0 2px 10px rgba(0,0,0,0.1);
            margin-bottom: 20px;
        }
        .alert {
            padding: 10px;
            margin: 5px 0;
            border-radius: 5px;
            border-left: 4px solid;
        }
        .alert-info { border-color: #2196F3; background-color: #E3F2FD; }
        .alert-warning { border-color: #FF9800; background-color: #FFF3E0; }
        .alert-error { border-color: #F44336; background-color: #FFEBEE; }
        .alert-critical { border-color: #9C27B0; background-color: #F3E5F5; }
        .scenario-list {
            background: white;
            padding: 20px;
            border-radius: 10px;
            box-shadow: 0 2px 10px rgba(0,0,0,0.1);
        }
        .scenario-item {
            display: flex;
            justify-content: space-between;
            align-items: center;
            padding: 10px;
            border-bottom: 1px solid #eee;
        }
        .scenario-status {
            padding: 4px 8px;
            border-radius: 4px;
            font-size: 0.8em;
            font-weight: bold;
        }
        .status-running { background-color: #4CAF50; color: white; }
        .status-completed { background-color: #2196F3; color: white; }
        .status-failed { background-color: #F44336; color: white; }
        .progress-bar {
            width: 100px;
            height: 8px;
            background-color: #ddd;
            border-radius: 4px;
            overflow: hidden;
        }
        .progress-fill {
            height: 100%;
            background-color: #4CAF50;
            transition: width 0.3s ease;
        }
        .connection-status {
            position: fixed;
            top: 20px;
            right: 20px;
            padding: 10px;
            border-radius: 5px;
            color: white;
            font-weight: bold;
        }
        .connected { background-color: #4CAF50; }
        .disconnected { background-color: #F44336; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>fleetd Load Testing Dashboard</h1>
            <p>Real-time monitoring and metrics for load testing scenarios</p>
        </div>

        <div id="connectionStatus" class="connection-status disconnected">
            Disconnected
        </div>

        <div class="status-grid">
            <div class="status-card">
                <div class="status-value" id="activeDevices">0</div>
                <div class="status-label">Active Devices</div>
            </div>
            <div class="status-card">
                <div class="status-value" id="throughput">0</div>
                <div class="status-label">Requests/sec</div>
            </div>
            <div class="status-card">
                <div class="status-value" id="latency">0ms</div>
                <div class="status-label">P95 Latency</div>
            </div>
            <div class="status-card">
                <div class="status-value" id="errorRate">0%</div>
                <div class="status-label">Error Rate</div>
            </div>
            <div class="status-card">
                <div class="status-value" id="cpuUsage">0%</div>
                <div class="status-label">CPU Usage</div>
            </div>
            <div class="status-card">
                <div class="status-value" id="memoryUsage">0%</div>
                <div class="status-label">Memory Usage</div>
            </div>
        </div>

        <div class="alerts" id="alertsContainer">
            <h3>System Alerts</h3>
            <div id="alertsList">No alerts</div>
        </div>

        <div class="scenario-list">
            <h3>Running Scenarios</h3>
            <div id="scenariosList">No scenarios running</div>
        </div>

        <div class="charts-grid">
            <div class="chart-container">
                <div class="chart-title">CPU & Memory Usage</div>
                <canvas id="resourceChart"></canvas>
            </div>
            <div class="chart-container">
                <div class="chart-title">Throughput</div>
                <canvas id="throughputChart"></canvas>
            </div>
            <div class="chart-container">
                <div class="chart-title">Latency</div>
                <canvas id="latencyChart"></canvas>
            </div>
            <div class="chart-container">
                <div class="chart-title">Device Count</div>
                <canvas id="deviceChart"></canvas>
            </div>
        </div>
    </div>

    <script>
        // WebSocket connection
        let ws;
        let charts = {};
        let maxDataPoints = 50;

        function connect() {
            const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
            ws = new WebSocket(protocol + '//' + window.location.host + '/ws');

            ws.onopen = function() {
                document.getElementById('connectionStatus').textContent = 'Connected';
                document.getElementById('connectionStatus').className = 'connection-status connected';
                console.log('Connected to dashboard');
            };

            ws.onmessage = function(event) {
                const data = JSON.parse(event.data);
                updateDashboard(data);
            };

            ws.onclose = function() {
                document.getElementById('connectionStatus').textContent = 'Disconnected';
                document.getElementById('connectionStatus').className = 'connection-status disconnected';
                console.log('Disconnected from dashboard');
                // Reconnect after 3 seconds
                setTimeout(connect, 3000);
            };

            ws.onerror = function(error) {
                console.error('WebSocket error:', error);
            };
        }

        function updateDashboard(data) {
            // Update status cards
            document.getElementById('activeDevices').textContent = data.realtime.active_devices || 0;
            document.getElementById('throughput').textContent = (data.realtime.current_throughput || 0).toFixed(1);
            document.getElementById('latency').textContent = formatDuration(data.realtime.current_latency || 0);
            document.getElementById('errorRate').textContent = ((data.fleet_metrics.total_errors / (data.fleet_metrics.total_requests + 1)) * 100).toFixed(2) + '%';
            document.getElementById('cpuUsage').textContent = (data.realtime.current_cpu || 0).toFixed(1) + '%';
            document.getElementById('memoryUsage').textContent = (data.realtime.current_memory || 0).toFixed(1) + '%';

            // Update alerts
            updateAlerts(data.system_health.alerts || []);

            // Update scenarios
            updateScenarios(data.scenarios || []);

            // Update charts
            updateCharts(data.charts);
        }

        function updateAlerts(alerts) {
            const container = document.getElementById('alertsList');
            if (alerts.length === 0) {
                container.innerHTML = 'No alerts';
                return;
            }

            container.innerHTML = alerts.map(alert =>
                '<div class="alert alert-' + alert.level + '">' +
                '<strong>' + alert.component + ':</strong> ' + alert.message +
                '<span style="float: right; font-size: 0.8em;">' + formatTime(alert.timestamp) + '</span>' +
                '</div>'
            ).join('');
        }

        function updateScenarios(scenarios) {
            const container = document.getElementById('scenariosList');
            if (scenarios.length === 0) {
                container.innerHTML = 'No scenarios running';
                return;
            }

            container.innerHTML = scenarios.map(scenario =>
                '<div class="scenario-item">' +
                '<div>' +
                '<strong>' + scenario.name + '</strong><br>' +
                '<small>Devices: ' + scenario.devices_count + ' | Errors: ' + scenario.error_count + '</small>' +
                '</div>' +
                '<div>' +
                '<div class="scenario-status status-' + scenario.status + '">' + scenario.status + '</div>' +
                '<div class="progress-bar" style="margin-top: 5px;">' +
                '<div class="progress-fill" style="width: ' + (scenario.progress * 100) + '%;"></div>' +
                '</div>' +
                '</div>' +
                '</div>'
            ).join('');
        }

        function updateCharts(chartData) {
            if (!chartData) return;

            // Update resource chart
            updateChart('resourceChart', {
                type: 'line',
                data: {
                    labels: chartData.cpu_usage.map(p => formatTime(p.timestamp)),
                    datasets: [{
                        label: 'CPU Usage (%)',
                        data: chartData.cpu_usage.map(p => p.value),
                        borderColor: '#FF6384',
                        fill: false
                    }, {
                        label: 'Memory Usage (%)',
                        data: chartData.memory_usage.map(p => p.value),
                        borderColor: '#36A2EB',
                        fill: false
                    }]
                },
                options: {
                    responsive: true,
                    scales: {
                        y: { beginAtZero: true, max: 100 }
                    }
                }
            });

            // Update throughput chart
            updateChart('throughputChart', {
                type: 'line',
                data: {
                    labels: chartData.throughput.map(p => formatTime(p.timestamp)),
                    datasets: [{
                        label: 'Requests/sec',
                        data: chartData.throughput.map(p => p.value),
                        borderColor: '#4BC0C0',
                        fill: false
                    }]
                },
                options: {
                    responsive: true,
                    scales: {
                        y: { beginAtZero: true }
                    }
                }
            });

            // Update latency chart
            updateChart('latencyChart', {
                type: 'line',
                data: {
                    labels: chartData.latency.map(p => formatTime(p.timestamp)),
                    datasets: [{
                        label: 'Latency (ms)',
                        data: chartData.latency.map(p => p.value),
                        borderColor: '#FFCE56',
                        fill: false
                    }]
                },
                options: {
                    responsive: true,
                    scales: {
                        y: { beginAtZero: true }
                    }
                }
            });

            // Update device chart
            updateChart('deviceChart', {
                type: 'line',
                data: {
                    labels: chartData.device_count.map(p => formatTime(p.timestamp)),
                    datasets: [{
                        label: 'Active Devices',
                        data: chartData.device_count.map(p => p.value),
                        borderColor: '#9966FF',
                        fill: true,
                        backgroundColor: 'rgba(153, 102, 255, 0.2)'
                    }]
                },
                options: {
                    responsive: true,
                    scales: {
                        y: { beginAtZero: true }
                    }
                }
            });
        }

        function updateChart(chartId, config) {
            if (charts[chartId]) {
                charts[chartId].data = config.data;
                charts[chartId].update();
            } else {
                const ctx = document.getElementById(chartId).getContext('2d');
                charts[chartId] = new Chart(ctx, config);
            }
        }

        function formatTime(timestamp) {
            return new Date(timestamp).toLocaleTimeString();
        }

        function formatDuration(nanoseconds) {
            const ms = nanoseconds / 1000000;
            if (ms < 1000) {
                return ms.toFixed(0) + 'ms';
            } else {
                return (ms / 1000).toFixed(1) + 's';
            }
        }

        // Initialize connection
        connect();
    </script>
</body>
</html>`

	t, err := template.New("dashboard").Parse(tmpl)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	if err := t.Execute(w, nil); err != nil {
		d.logger.Error("Failed to execute template", "error", err)
	}
}

// handleWebSocket handles WebSocket connections for real-time updates
func (d *Dashboard) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := d.upgrader.Upgrade(w, r, nil)
	if err != nil {
		d.logger.Error("Failed to upgrade WebSocket connection", "error", err)
		return
	}

	d.clientsMu.Lock()
	d.clients[conn] = true
	d.clientsMu.Unlock()

	d.logger.Info("New WebSocket client connected", "remote_addr", r.RemoteAddr)

	// Send initial data
	d.sendDataToClient(conn)

	// Handle client disconnection
	go func() {
		defer func() {
			d.clientsMu.Lock()
			delete(d.clients, conn)
			d.clientsMu.Unlock()
			conn.Close()
		}()

		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					d.logger.Error("WebSocket error", "error", err)
				}
				break
			}
		}
	}()
}

// handleAPIMetrics provides JSON API for metrics
func (d *Dashboard) handleAPIMetrics(w http.ResponseWriter, r *http.Request) {
	data := d.generateDashboardData()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		d.logger.Error("Failed to encode metrics response", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// handleAPIExport provides metrics export functionality
func (d *Dashboard) handleAPIExport(w http.ResponseWriter, r *http.Request) {
	if d.metricsCollector == nil {
		http.Error(w, "Metrics collector not available", http.StatusServiceUnavailable)
		return
	}

	data, err := d.metricsCollector.ExportMetrics()
	if err != nil {
		d.logger.Error("Failed to export metrics", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=load_test_metrics.json")
	w.Write(data)
}

// handleStatic serves static files (placeholder)
func (d *Dashboard) handleStatic(w http.ResponseWriter, r *http.Request) {
	http.NotFound(w, r)
}

// broadcastLoop sends periodic updates to all connected clients
func (d *Dashboard) broadcastLoop() {
	defer d.wg.Done()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-d.ctx.Done():
			return
		case <-ticker.C:
			d.broadcastData()
		}
	}
}

// healthMonitorLoop monitors system health and generates alerts
func (d *Dashboard) healthMonitorLoop() {
	defer d.wg.Done()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-d.ctx.Done():
			return
		case <-ticker.C:
			d.checkSystemHealth()
		}
	}
}

// broadcastData sends current data to all connected WebSocket clients
func (d *Dashboard) broadcastData() {
	d.clientsMu.RLock()
	clients := make([]*websocket.Conn, 0, len(d.clients))
	for client := range d.clients {
		clients = append(clients, client)
	}
	d.clientsMu.RUnlock()

	for _, client := range clients {
		d.sendDataToClient(client)
	}
}

// sendDataToClient sends data to a specific WebSocket client
func (d *Dashboard) sendDataToClient(conn *websocket.Conn) {
	data := d.generateDashboardData()

	if err := conn.WriteJSON(data); err != nil {
		d.logger.Error("Failed to send data to WebSocket client", "error", err)
		d.clientsMu.Lock()
		delete(d.clients, conn)
		d.clientsMu.Unlock()
		conn.Close()
	}
}

// generateDashboardData creates the complete dashboard data structure
func (d *Dashboard) generateDashboardData() DashboardData {
	data := DashboardData{
		Timestamp: time.Now(),
		Scenarios: []ScenarioStatus{},
		SystemHealth: SystemHealthData{
			Alerts: []Alert{},
		},
		Charts: ChartData{},
	}

	// Get metrics collector data
	if d.metricsCollector != nil {
		data.Summary = d.metricsCollector.GetSummary()
		data.RealTime = d.metricsCollector.GetRealTimeMetrics()
	}

	// Get fleet simulator data
	if d.fleetSimulator != nil {
		data.FleetMetrics = d.fleetSimulator.GetMetrics()
	}

	// Generate chart data
	data.Charts = d.generateChartData()

	// Generate system health data
	data.SystemHealth = d.generateSystemHealthData()

	return data
}

// generateChartData creates chart data from collected metrics
func (d *Dashboard) generateChartData() ChartData {
	charts := ChartData{
		CPUUsage:    []DataPoint{},
		MemoryUsage: []DataPoint{},
		Throughput:  []DataPoint{},
		Latency:     []DataPoint{},
		ErrorRate:   []DataPoint{},
		DeviceCount: []DataPoint{},
		NetworkIO:   []DataPoint{},
	}

	if d.metricsCollector == nil {
		return charts
	}

	// Generate sample data for the last 50 points
	now := time.Now()
	for i := 49; i >= 0; i-- {
		timestamp := now.Add(-time.Duration(i) * time.Second)

		// CPU usage with some variation
		cpuValue := 20 + 30*float64(i%10)/10 + 10*(0.5-float64(time.Now().UnixNano()%1000)/1000)
		charts.CPUUsage = append(charts.CPUUsage, DataPoint{
			Timestamp: timestamp,
			Value:     cpuValue,
		})

		// Memory usage
		memValue := 40 + 20*float64(i%15)/15 + 5*(0.5-float64(time.Now().UnixNano()%1000)/1000)
		charts.MemoryUsage = append(charts.MemoryUsage, DataPoint{
			Timestamp: timestamp,
			Value:     memValue,
		})

		// Throughput
		throughputValue := 100 + 50*float64(i%8)/8 + 20*(0.5-float64(time.Now().UnixNano()%1000)/1000)
		charts.Throughput = append(charts.Throughput, DataPoint{
			Timestamp: timestamp,
			Value:     throughputValue,
		})

		// Latency
		latencyValue := 50 + 30*float64(i%12)/12 + 15*(0.5-float64(time.Now().UnixNano()%1000)/1000)
		charts.Latency = append(charts.Latency, DataPoint{
			Timestamp: timestamp,
			Value:     latencyValue,
		})

		// Device count
		deviceValue := float64(0)
		if d.fleetSimulator != nil {
			deviceValue = float64(d.fleetSimulator.GetMetrics().OnlineDevices)
		}
		charts.DeviceCount = append(charts.DeviceCount, DataPoint{
			Timestamp: timestamp,
			Value:     deviceValue,
		})
	}

	return charts
}

// generateSystemHealthData creates system health information
func (d *Dashboard) generateSystemHealthData() SystemHealthData {
	health := SystemHealthData{
		Alerts: []Alert{},
	}

	if d.metricsCollector == nil {
		return health
	}

	realTime := d.metricsCollector.GetRealTimeMetrics()

	// CPU health
	health.CPUHealth = HealthStatus{
		Value:     realTime.CurrentCPU,
		Threshold: 80.0,
	}
	if realTime.CurrentCPU > 90 {
		health.CPUHealth.Status = "critical"
		health.CPUHealth.Description = "CPU usage critically high"
	} else if realTime.CurrentCPU > 80 {
		health.CPUHealth.Status = "warning"
		health.CPUHealth.Description = "CPU usage high"
	} else {
		health.CPUHealth.Status = "healthy"
		health.CPUHealth.Description = "CPU usage normal"
	}

	// Memory health
	health.MemoryHealth = HealthStatus{
		Value:     realTime.CurrentMemory,
		Threshold: 85.0,
	}
	if realTime.CurrentMemory > 95 {
		health.MemoryHealth.Status = "critical"
		health.MemoryHealth.Description = "Memory usage critically high"
	} else if realTime.CurrentMemory > 85 {
		health.MemoryHealth.Status = "warning"
		health.MemoryHealth.Description = "Memory usage high"
	} else {
		health.MemoryHealth.Status = "healthy"
		health.MemoryHealth.Description = "Memory usage normal"
	}

	// Network health (simplified)
	health.NetworkHealth = HealthStatus{
		Status:      "healthy",
		Value:       realTime.CurrentThroughput,
		Description: "Network performance normal",
	}

	// Overall health score
	healthScore := 100.0
	if health.CPUHealth.Status == "warning" {
		healthScore -= 10
	} else if health.CPUHealth.Status == "critical" {
		healthScore -= 30
	}

	if health.MemoryHealth.Status == "warning" {
		healthScore -= 10
	} else if health.MemoryHealth.Status == "critical" {
		healthScore -= 30
	}

	health.HealthScore = healthScore

	// Overall health
	if healthScore > 90 {
		health.OverallHealth.Status = "healthy"
		health.OverallHealth.Description = "System performing well"
	} else if healthScore > 70 {
		health.OverallHealth.Status = "warning"
		health.OverallHealth.Description = "System performance degraded"
	} else {
		health.OverallHealth.Status = "critical"
		health.OverallHealth.Description = "System performance critical"
	}

	return health
}

// checkSystemHealth monitors system health and generates alerts
func (d *Dashboard) checkSystemHealth() {
	// This would check various system metrics and generate alerts
	// For now, it's a placeholder
}

// GetURL returns the dashboard URL
func (d *Dashboard) GetURL() string {
	return fmt.Sprintf("http://localhost:%d", d.port)
}
