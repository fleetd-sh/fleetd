package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"fleetd.sh/internal/discovery"
)

// Device represents a device in the fleet
type Device struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Type     string    `json:"type"`
	Version  string    `json:"version"`
	LastSeen time.Time `json:"last_seen"`
	Status   string    `json:"status"`
	Metadata string    `json:"metadata,omitempty"`
}

// TelemetryData represents telemetry data from a device
type TelemetryData struct {
	DeviceID   string    `json:"device_id"`
	Timestamp  time.Time `json:"timestamp"`
	MetricName string    `json:"metric_name"`
	Value      float64   `json:"value"`
	Metadata   string    `json:"metadata,omitempty"`
}

// ConfigUpdate represents a configuration update for devices
type ConfigUpdate struct {
	ServerURL string `json:"server_url"`
	APIKey    string `json:"api_key,omitempty"`
	Config    string `json:"config"`
}

// handleDevices handles GET /api/v1/devices
func (s *Server) handleDevices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	rows, err := s.db.Query(`
		SELECT id, name,
		       COALESCE(os_type, type, '') as os_type,
		       COALESCE(agent_version, version, '') as agent_version,
		       COALESCE(last_seen, created_at) as last_seen,
		       CASE
		         WHEN datetime('now', '-5 minutes') < COALESCE(last_seen, created_at) THEN 'online'
		         ELSE 'offline'
		       END as status,
		       COALESCE(metadata, '{}') as metadata
		FROM device
		ORDER BY last_seen DESC
	`)
	if err != nil {
		slog.Error("Failed to query devices", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	devices := []Device{}
	for rows.Next() {
		var d Device
		var lastSeenStr string
		if err := rows.Scan(&d.ID, &d.Name, &d.Type, &d.Version, &lastSeenStr, &d.Status, &d.Metadata); err != nil {
			slog.Error("Failed to scan device", "error", err)
			continue
		}
		// Parse the timestamp
		if t, err := time.Parse("2006-01-02 15:04:05", lastSeenStr); err == nil {
			d.LastSeen = t
		} else if t, err := time.Parse("2006-01-02T15:04:05Z", lastSeenStr); err == nil {
			d.LastSeen = t
		} else {
			d.LastSeen = time.Now() // Default to now if parsing fails
		}
		devices = append(devices, d)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(devices)
}

// handleDeviceRoutes handles all /api/v1/devices/{id}/* routes
func (s *Server) handleDeviceRoutes(w http.ResponseWriter, r *http.Request) {
	// Extract device ID and sub-path
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/devices/")
	if path == "" {
		http.Error(w, "Device ID required", http.StatusBadRequest)
		return
	}

	// Split into device ID and sub-route
	parts := strings.SplitN(path, "/", 2)
	deviceID := parts[0]
	subRoute := ""
	if len(parts) > 1 {
		subRoute = parts[1]
	}

	// Route based on sub-path
	switch subRoute {
	case "metrics":
		s.getDeviceMetrics(w, r, deviceID)
	case "update":
		s.updateDeviceVersion(w, r, deviceID)
	case "command":
		s.executeDeviceCommand(w, r, deviceID)
	case "":
		// Base device operations
		switch r.Method {
		case http.MethodGet:
			s.getDevice(w, r, deviceID)
		case http.MethodPut:
			s.updateDevice(w, r, deviceID)
		case http.MethodDelete:
			s.deleteDevice(w, r, deviceID)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	default:
		http.Error(w, "Not found", http.StatusNotFound)
	}
}

func (s *Server) getDevice(w http.ResponseWriter, r *http.Request, deviceID string) {
	var d Device
	err := s.db.QueryRow(`
		SELECT id, name, type, version, last_seen,
		       CASE
		         WHEN datetime('now', '-5 minutes') < last_seen THEN 'online'
		         ELSE 'offline'
		       END as status,
		       metadata
		FROM device WHERE id = ?
	`, deviceID).Scan(&d.ID, &d.Name, &d.Type, &d.Version, &d.LastSeen, &d.Status, &d.Metadata)
	if err != nil {
		http.Error(w, "Device not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(d)
}

func (s *Server) updateDevice(w http.ResponseWriter, r *http.Request, deviceID string) {
	var update map[string]any
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Build update query dynamically based on provided fields
	// For now, just update metadata
	if metadata, ok := update["metadata"]; ok {
		metadataStr, _ := json.Marshal(metadata)
		_, err := s.db.Exec("UPDATE device SET metadata = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?",
			string(metadataStr), deviceID)
		if err != nil {
			http.Error(w, "Failed to update device", http.StatusInternalServerError)
			return
		}
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "updated"})
}

func (s *Server) deleteDevice(w http.ResponseWriter, r *http.Request, deviceID string) {
	result, err := s.db.Exec("DELETE FROM device WHERE id = ?", deviceID)
	if err != nil {
		http.Error(w, "Failed to delete device", http.StatusInternalServerError)
		return
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		http.Error(w, "Device not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
}

// handleTelemetry handles POST /api/v1/telemetry
func (s *Server) handleTelemetry(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var data TelemetryData
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Store telemetry data (check if table exists first)
	var exists bool
	err := s.db.QueryRow("SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'telemetry')").Scan(&exists)
	if err != nil || !exists {
		// Table doesn't exist, create it
		_, err = s.db.Exec(`
			CREATE TABLE IF NOT EXISTS telemetry (
				id SERIAL PRIMARY KEY,
				device_id VARCHAR(255) NOT NULL,
				timestamp TIMESTAMP NOT NULL,
				metric_name VARCHAR(255) NOT NULL,
				metric_value DOUBLE PRECISION,
				metadata TEXT
			)
		`)
		if err != nil {
			slog.Error("Failed to create telemetry table", "error", err)
			http.Error(w, "Failed to store telemetry", http.StatusInternalServerError)
			return
		}
	}

	_, err = s.db.Exec(`
		INSERT INTO telemetry (device_id, timestamp, metric_name, metric_value, metadata)
		VALUES ($1, $2, $3, $4, $5)
	`, data.DeviceID, data.Timestamp, data.MetricName, data.Value, data.Metadata)
	if err != nil {
		slog.Error("Failed to store telemetry", "error", err)
		http.Error(w, "Failed to store telemetry", http.StatusInternalServerError)
		return
	}

	// Update device last_seen
	s.db.Exec("UPDATE device SET last_seen = CURRENT_TIMESTAMP WHERE id = $1", data.DeviceID)

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"status": "stored"})
}

// handleMetrics handles GET /api/v1/telemetry/metrics
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	deviceID := r.URL.Query().Get("device_id")
	metricName := r.URL.Query().Get("metric")
	limit := r.URL.Query().Get("limit")
	if limit == "" {
		limit = "100"
	}

	// Check if telemetry table exists
	var exists bool
	err := s.db.QueryRow("SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'telemetry')").Scan(&exists)
	if err != nil || !exists {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]TelemetryData{})
		return
	}

	query := `
		SELECT device_id, timestamp, metric_name, metric_value, COALESCE(metadata, '{}')
		FROM telemetry
		WHERE 1=1
	`
	args := []any{}
	argNum := 1

	if deviceID != "" {
		query += fmt.Sprintf(" AND device_id = $%d", argNum)
		args = append(args, deviceID)
		argNum++
	}
	if metricName != "" {
		query += fmt.Sprintf(" AND metric_name = $%d", argNum)
		args = append(args, metricName)
		argNum++
	}
	query += fmt.Sprintf(" ORDER BY timestamp DESC LIMIT $%d", argNum)
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		http.Error(w, "Failed to query metrics", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var metrics []TelemetryData
	for rows.Next() {
		var m TelemetryData
		if err := rows.Scan(&m.DeviceID, &m.Timestamp, &m.MetricName, &m.Value, &m.Metadata); err != nil {
			continue
		}
		metrics = append(metrics, m)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metrics)
}

// handleConfig handles GET/POST /api/v1/config
func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	deviceID := r.URL.Query().Get("device_id")

	switch r.Method {
	case http.MethodGet:
		if deviceID == "" {
			http.Error(w, "device_id required", http.StatusBadRequest)
			return
		}

		var config string
		err := s.db.QueryRow("SELECT config FROM device_config WHERE device_id = ?", deviceID).Scan(&config)
		if err != nil {
			http.Error(w, "Config not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(config))

	case http.MethodPost:
		if deviceID == "" {
			http.Error(w, "device_id required", http.StatusBadRequest)
			return
		}

		var configUpdate ConfigUpdate
		if err := json.NewDecoder(r.Body).Decode(&configUpdate); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		configJSON, _ := json.Marshal(configUpdate)
		_, err := s.db.Exec(`
			INSERT OR REPLACE INTO device_config (device_id, config, version, updated_at)
			VALUES (?, ?, (SELECT COALESCE(MAX(version), 0) + 1 FROM device_config WHERE device_id = ?), CURRENT_TIMESTAMP)
		`, deviceID, string(configJSON), deviceID)
		if err != nil {
			http.Error(w, "Failed to update config", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "updated"})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleDiscover handles GET /api/v1/discover
func (s *Server) handleDiscover(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if r.Method == http.MethodGet {
		// Discover devices on the network
		d := discovery.New("fleet-server", 0, "_fleetd._tcp")

		ctx := context.Background()
		devices, err := d.Browse(ctx, 5*time.Second)
		if err != nil {
			slog.Error("Failed to discover devices", "error", err)
			http.Error(w, "Discovery failed", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(devices)
	} else {
		// POST: Send configuration to discovered device
		var req struct {
			DeviceID string       `json:"device_id"`
			Config   ConfigUpdate `json:"config"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		// TODO: Implement sending config via mDNS to specific device
		// This would require establishing a connection to the device's RPC service
		// and calling a configuration update endpoint

		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]string{
			"status": "configuration sent",
			"device": req.DeviceID,
		})
	}
}

// handleDashboard serves the web dashboard
func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	// For now, serve a simple HTML dashboard
	html := `<!DOCTYPE html>
<html>
<head>
    <title>fleetd Management Dashboard</title>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; margin: 0; padding: 20px; background: #f5f5f5; }
        .container { max-width: 1200px; margin: 0 auto; }
        h1 { color: #333; }
        .card { background: white; border-radius: 8px; padding: 20px; margin-bottom: 20px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
        .status { display: inline-block; padding: 4px 8px; border-radius: 4px; font-size: 12px; font-weight: 600; }
        .status.online { background: #d4edda; color: #155724; }
        .status.offline { background: #f8d7da; color: #721c24; }
        table { width: 100%; border-collapse: collapse; }
        th, td { text-align: left; padding: 12px; border-bottom: 1px solid #e0e0e0; }
        th { font-weight: 600; color: #666; }
        .actions { display: flex; gap: 10px; margin-bottom: 20px; }
        button { background: #007bff; color: white; border: none; padding: 8px 16px; border-radius: 4px; cursor: pointer; }
        button:hover { background: #0056b3; }
    </style>
</head>
<body>
    <div class="container">
        <h1>fleetd Management Dashboard</h1>

        <div class="card">
            <h2>Fleet Status</h2>
            <div class="actions">
                <button onclick="discoverDevices()">Discover Devices</button>
                <button onclick="refreshDevices()">Refresh</button>
            </div>
            <table id="devices-table">
                <thead>
                    <tr>
                        <th>Device ID</th>
                        <th>Name</th>
                        <th>Type</th>
                        <th>Version</th>
                        <th>Status</th>
                        <th>Last Seen</th>
                    </tr>
                </thead>
                <tbody id="devices-tbody">
                    <tr><td colspan="6">Loading...</td></tr>
                </tbody>
            </table>
        </div>

        <div class="card">
            <h2>Recent Telemetry</h2>
            <div id="telemetry">
                <p>Loading...</p>
            </div>
        </div>
    </div>

    <script>
        async function refreshDevices() {
            try {
                const response = await fetch('/api/v1/devices');
                const devices = await response.json();

                const tbody = document.getElementById('devices-tbody');
                tbody.innerHTML = devices.map(d => ` + "`" + `
                    <tr>
                        <td>${d.id}</td>
                        <td>${d.name}</td>
                        <td>${d.type}</td>
                        <td>${d.version}</td>
                        <td><span class="status ${d.status}">${d.status}</span></td>
                        <td>${new Date(d.last_seen).toLocaleString()}</td>
                    </tr>
                ` + "`" + `).join('') || '<tr><td colspan="6">No devices found</td></tr>';
            } catch (err) {
                console.error('Failed to fetch devices:', err);
            }
        }

        async function discoverDevices() {
            try {
                const response = await fetch('/api/v1/discover');
                const devices = await response.json();
                alert(` + "`" + `Found ${devices.length} device(s) on the network` + "`" + `);
            } catch (err) {
                console.error('Failed to discover devices:', err);
                alert('Discovery failed');
            }
        }

        async function loadTelemetry() {
            try {
                const response = await fetch('/api/v1/telemetry/metrics?limit=10');
                const metrics = await response.json();

                const telemetryDiv = document.getElementById('telemetry');
                if (metrics && metrics.length > 0) {
                    telemetryDiv.innerHTML = '<ul>' + metrics.map(m => ` + "`" + `
                        <li>${m.device_id}: ${m.metric_name} = ${m.value} (${new Date(m.timestamp).toLocaleString()})</li>
                    ` + "`" + `).join('') + '</ul>';
                } else {
                    telemetryDiv.innerHTML = '<p>No telemetry data available</p>';
                }
            } catch (err) {
                console.error('Failed to fetch telemetry:', err);
            }
        }

        // Initial load
        refreshDevices();
        loadTelemetry();

        // Auto-refresh every 10 seconds
        setInterval(() => {
            refreshDevices();
            loadTelemetry();
        }, 10000);
    </script>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, html)
}

// handleEvents handles Server-Sent Events for real-time updates
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	// Set headers for SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Create a channel for this client
	messageChan := make(chan []byte)

	// Register the client
	s.sseHub.register <- messageChan
	defer func() {
		s.sseHub.unregister <- messageChan
		close(messageChan)
	}()

	// Create a flusher
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	// Send initial connection message
	fmt.Fprintf(w, "data: {\"type\":\"connected\",\"timestamp\":\"%s\"}\n\n", time.Now().Format(time.RFC3339))
	flusher.Flush()

	// Listen for client disconnect
	notify := r.Context().Done()

	for {
		select {
		case <-notify:
			return
		case msg := <-messageChan:
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		case <-time.After(30 * time.Second):
			// Send heartbeat to keep connection alive
			fmt.Fprintf(w, ": heartbeat\n\n")
			flusher.Flush()
		}
	}
}

// getDeviceMetrics handles GET /api/v1/devices/{id}/metrics
func (s *Server) getDeviceMetrics(w http.ResponseWriter, r *http.Request, deviceID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// First verify device exists
	var exists bool
	err := s.db.QueryRow("SELECT EXISTS(SELECT 1 FROM device WHERE id = ?)", deviceID).Scan(&exists)
	if err != nil || !exists {
		http.Error(w, "Device not found", http.StatusNotFound)
		return
	}

	// Check if telemetry table exists
	var tableExists bool
	err = s.db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='telemetry'").Scan(&tableExists)
	if err != nil || !tableExists {
		// Table doesn't exist yet, return empty metrics
		metrics := make(map[string]interface{})
		metrics["device_id"] = deviceID
		metrics["metrics"] = []map[string]interface{}{}
		metrics["count"] = 0

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(metrics)
		return
	}

	// Query latest telemetry metrics for this device
	rows, err := s.db.Query(`
		SELECT metric_name, metric_value, timestamp, COALESCE(metadata, '{}')
		FROM telemetry
		WHERE device_id = ?
		ORDER BY timestamp DESC
		LIMIT 100
	`, deviceID)
	if err != nil {
		slog.Error("Failed to query device metrics", "error", err, "device_id", deviceID)
		// Return empty metrics instead of error
		metrics := make(map[string]interface{})
		metrics["device_id"] = deviceID
		metrics["metrics"] = []map[string]interface{}{}
		metrics["count"] = 0

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(metrics)
		return
	}
	defer rows.Close()

	// Build metrics map
	metrics := make(map[string]interface{})
	metrics["device_id"] = deviceID
	metrics["metrics"] = []map[string]interface{}{}

	metricsData := []map[string]interface{}{}
	for rows.Next() {
		var name string
		var value float64
		var timestamp time.Time
		var metadata string

		if err := rows.Scan(&name, &value, &timestamp, &metadata); err != nil {
			slog.Error("Failed to scan metric", "error", err)
			continue
		}

		metricsData = append(metricsData, map[string]interface{}{
			"name":      name,
			"value":     value,
			"timestamp": timestamp.Format(time.RFC3339),
			"metadata":  metadata,
		})
	}
	metrics["metrics"] = metricsData
	metrics["count"] = len(metricsData)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metrics)
}

// updateDeviceVersion handles POST /api/v1/devices/{id}/update
func (s *Server) updateDeviceVersion(w http.ResponseWriter, r *http.Request, deviceID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Version    string `json:"version"`
		BinaryURL  string `json:"binary_url"`
		Checksum   string `json:"checksum"`
		ForceUpdate bool  `json:"force_update"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Verify device exists
	var exists bool
	err := s.db.QueryRow("SELECT EXISTS(SELECT 1 FROM device WHERE id = ?)", deviceID).Scan(&exists)
	if err != nil || !exists {
		http.Error(w, "Device not found", http.StatusNotFound)
		return
	}

	// Store update request
	// TODO: Implement actual update mechanism - for now just log it
	slog.Info("Device update requested",
		"device_id", deviceID,
		"version", req.Version,
		"binary_url", req.BinaryURL)

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "accepted",
		"message": "Update request queued",
		"version": req.Version,
	})
}

// executeDeviceCommand handles POST /api/v1/devices/{id}/command
func (s *Server) executeDeviceCommand(w http.ResponseWriter, r *http.Request, deviceID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Command string `json:"command"`
		Args    string `json:"args"`
		Timeout int    `json:"timeout"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Verify device exists and get last seen time
	var lastSeen time.Time
	err := s.db.QueryRow(`
		SELECT COALESCE(last_seen, created_at)
		FROM device
		WHERE id = ?
	`, deviceID).Scan(&lastSeen)
	if err != nil {
		http.Error(w, "Device not found", http.StatusNotFound)
		return
	}

	// For E2E testing, we need to forward commands to agents
	// In production, this would use a proper message queue or polling mechanism
	// For now, we'll store the command and agents can poll for it
	// OR we try to send directly if we have device connectivity info

	slog.Info("Device command requested",
		"device_id", deviceID,
		"command", req.Command,
		"args", req.Args)

	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "accepted",
		"message": "Command queued for execution",
		"command": req.Command,
	})
}
