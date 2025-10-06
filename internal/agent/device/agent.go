package device

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"fleetd.sh/internal/agent/metrics"
	"fleetd.sh/internal/config"
	"fleetd.sh/internal/security"
)

// Agent represents a device agent that communicates with the fleet server
type Agent struct {
	config           *config.AgentConfig
	client           *http.Client
	deviceInfo       *DeviceInfo
	currentState     *State
	updateChan       chan *UpdateCommand
	metricsChan      chan *Metrics
	mu               sync.RWMutex
	shutdownChan     chan struct{}
	lastHealthCheck  time.Time
	healthStatus     bool
	stateStore       *StateStore
	metricsCollector *metrics.Collector
}

// DeviceInfo contains device metadata
type DeviceInfo struct {
	DeviceID       string            `json:"device_id"`
	Hostname       string            `json:"hostname"`
	OS             string            `json:"os"`
	Arch           string            `json:"arch"`
	CurrentVersion string            `json:"current_version"`
	IPAddress      string            `json:"ip_address"`
	Labels         map[string]string `json:"labels"`
	Capabilities   []string          `json:"capabilities"`
}

// State represents the current agent state
type State struct {
	Status         string    `json:"status"` // idle, updating, error
	LastHeartbeat  time.Time `json:"last_heartbeat"`
	LastUpdate     time.Time `json:"last_update"`
	UpdateProgress int       `json:"update_progress"`
	Error          string    `json:"error,omitempty"`
	MetricsBuffer  []Metrics `json:"metrics_buffer,omitempty"`
	PendingUpdates []string  `json:"pending_updates,omitempty"`
}

// UpdateCommand represents an update command from the server
type UpdateCommand struct {
	ID        string                 `json:"id"`
	Type      string                 `json:"type"`
	Version   string                 `json:"version"`
	URL       string                 `json:"url"`
	Checksum  string                 `json:"checksum"`
	Signature string                 `json:"signature"`
	Manifest  map[string]interface{} `json:"manifest"`
	Timestamp time.Time              `json:"timestamp"`
}

// Metrics represents device metrics
type Metrics struct {
	Timestamp   time.Time              `json:"timestamp"`
	CPUUsage    float64                `json:"cpu_usage"`
	MemoryUsage float64                `json:"memory_usage"`
	DiskUsage   float64                `json:"disk_usage"`
	Temperature float64                `json:"temperature,omitempty"`
	Custom      map[string]interface{} `json:"custom,omitempty"`
}

// NewAgent creates a new device agent
func NewAgent(cfg *config.AgentConfig) (*Agent, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	// Gather device information
	deviceInfo := &DeviceInfo{
		DeviceID:       cfg.DeviceID,
		Hostname:       getHostname(),
		OS:             runtime.GOOS,
		Arch:           runtime.GOARCH,
		CurrentVersion: cfg.CurrentVersion,
		IPAddress:      getIPAddress(),
		Labels:         cfg.Labels,
		Capabilities:   cfg.Capabilities,
	}

	// Create data directory if it doesn't exist
	if cfg.DataDir != "" {
		if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create data directory: %w", err)
		}
	}

	// Initialize state store
	stateStore, err := NewStateStore(filepath.Join(cfg.DataDir, "state.db"))
	if err != nil {
		return nil, fmt.Errorf("failed to initialize state store: %w", err)
	}

	agent := &Agent{
		config:           cfg,
		client:           &http.Client{Timeout: 30 * time.Second},
		deviceInfo:       deviceInfo,
		currentState:     &State{Status: "idle"},
		updateChan:       make(chan *UpdateCommand, 10),
		metricsChan:      make(chan *Metrics, 100),
		shutdownChan:     make(chan struct{}),
		healthStatus:     true,
		stateStore:       stateStore,
		metricsCollector: metrics.NewCollector(),
	}

	// Restore previous state if available
	if savedState, err := stateStore.LoadState(); err == nil && savedState != nil {
		agent.currentState = savedState
		log.Printf("Restored previous agent state: %+v", savedState)
	}

	return agent, nil
}

// Start begins the agent's main loop
func (a *Agent) Start(ctx context.Context) error {
	// Initialize heartbeat timestamp
	a.mu.Lock()
	a.currentState.LastHeartbeat = time.Now()
	a.mu.Unlock()

	// Register with server
	if err := a.register(ctx); err != nil {
		return fmt.Errorf("registration failed: %w", err)
	}

	// Start background workers
	var wg sync.WaitGroup

	// Heartbeat worker
	wg.Add(1)
	go func() {
		defer wg.Done()
		a.heartbeatLoop(ctx)
	}()

	// Update check worker
	wg.Add(1)
	go func() {
		defer wg.Done()
		a.updateCheckLoop(ctx)
	}()

	// Metrics collection worker
	wg.Add(1)
	go func() {
		defer wg.Done()
		a.metricsLoop(ctx)
	}()

	// Update processor
	wg.Add(1)
	go func() {
		defer wg.Done()
		a.updateProcessor(ctx)
	}()

	// Wait for shutdown
	<-ctx.Done()
	close(a.shutdownChan)
	wg.Wait()

	return nil
}

// register registers the device with the fleet server
func (a *Agent) register(ctx context.Context) error {
	url := fmt.Sprintf("%s/api/v1/devices/register", a.config.ServerURL)

	payload, err := json.Marshal(a.deviceInfo)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payload))
	if err != nil {
		return err
	}

	a.setAuthHeaders(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("registration failed with status %d: %s", resp.StatusCode, string(body))
	}

	log.Printf("Device registered successfully")
	return nil
}

// heartbeatLoop sends periodic heartbeats to the server
func (a *Agent) heartbeatLoop(ctx context.Context) {
	if a.config.HeartbeatInterval <= 0 {
		<-ctx.Done()
		return
	}

	ticker := time.NewTicker(a.config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.sendHeartbeat(ctx)
		}
	}
}

// sendHeartbeat sends a heartbeat to the server
func (a *Agent) sendHeartbeat(ctx context.Context) error {
	url := fmt.Sprintf("%s/api/v1/devices/%s/heartbeat", a.config.ServerURL, a.config.DeviceID)

	a.mu.RLock()
	state := *a.currentState
	a.mu.RUnlock()

	state.LastHeartbeat = time.Now()

	payload, err := json.Marshal(state)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payload))
	if err != nil {
		return err
	}

	a.setAuthHeaders(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		log.Printf("Heartbeat failed: %v", err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("Heartbeat failed with status %d: %s", resp.StatusCode, string(body))
		return fmt.Errorf("heartbeat failed")
	}

	return nil
}

// updateCheckLoop periodically checks for updates
func (a *Agent) updateCheckLoop(ctx context.Context) {
	if a.config.UpdateCheckInterval <= 0 {
		<-ctx.Done()
		return
	}

	ticker := time.NewTicker(a.config.UpdateCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.checkForUpdates(ctx)
		}
	}
}

// checkForUpdates checks the server for available updates
func (a *Agent) checkForUpdates(ctx context.Context) {
	url := fmt.Sprintf("%s/api/v1/devices/%s/updates", a.config.ServerURL, a.config.DeviceID)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return
	}

	a.setAuthHeaders(req)

	resp, err := a.client.Do(req)
	if err != nil {
		log.Printf("Update check failed: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		// No updates available
		return
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("Update check failed with status %d: %s", resp.StatusCode, string(body))
		return
	}

	var update UpdateCommand
	if err := json.NewDecoder(resp.Body).Decode(&update); err != nil {
		log.Printf("Failed to decode update: %v", err)
		return
	}

	// Queue update for processing
	select {
	case a.updateChan <- &update:
		log.Printf("Update queued: version %s", update.Version)
	default:
		log.Printf("Update queue full, dropping update")
	}
}

// updateProcessor processes queued updates
func (a *Agent) updateProcessor(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case update := <-a.updateChan:
			a.processUpdate(ctx, update)
		}
	}
}

// processUpdate downloads and applies an update
func (a *Agent) processUpdate(ctx context.Context, update *UpdateCommand) {
	log.Printf("Processing update: %s (version %s)", update.ID, update.Version)

	// Update state
	a.updateState("updating", 0, "")

	// Download update
	updatePath, err := a.downloadUpdate(ctx, update)
	if err != nil {
		log.Printf("Download failed: %v", err)
		a.updateState("error", 0, fmt.Sprintf("download failed: %v", err))
		a.reportUpdateStatus(ctx, update.ID, "failed", err.Error())
		return
	}
	a.updateState("updating", 25, "")

	// Verify update
	if err := a.verifyUpdate(updatePath, update); err != nil {
		log.Printf("Verification failed: %v", err)
		a.updateState("error", 25, fmt.Sprintf("verification failed: %v", err))
		a.reportUpdateStatus(ctx, update.ID, "failed", err.Error())
		os.Remove(updatePath)
		return
	}
	a.updateState("updating", 50, "")

	// Apply update
	if err := a.applyUpdate(ctx, updatePath, update); err != nil {
		log.Printf("Application failed: %v", err)
		a.updateState("error", 50, fmt.Sprintf("application failed: %v", err))
		a.reportUpdateStatus(ctx, update.ID, "failed", err.Error())
		os.Remove(updatePath)
		return
	}
	a.updateState("updating", 90, "")

	// Verify health after update
	if err := a.verifyHealth(ctx); err != nil {
		log.Printf("Health check failed, rolling back: %v", err)
		a.rollback(ctx, update)
		a.updateState("error", 90, fmt.Sprintf("health check failed: %v", err))
		a.reportUpdateStatus(ctx, update.ID, "failed", err.Error())
		return
	}

	// Update successful
	a.deviceInfo.CurrentVersion = update.Version
	a.updateState("idle", 100, "")
	a.reportUpdateStatus(ctx, update.ID, "completed", "")
	log.Printf("Update completed successfully")

	// Clean up
	os.Remove(updatePath)
}

// downloadUpdate downloads an update package
func (a *Agent) downloadUpdate(ctx context.Context, update *UpdateCommand) (string, error) {
	// Create temp directory for downloads
	tempDir := filepath.Join(a.config.DataDir, "downloads")
	os.MkdirAll(tempDir, 0755)

	// Download file
	fileName := fmt.Sprintf("update-%s.tar.gz", update.Version)
	filePath := filepath.Join(tempDir, fileName)

	req, err := http.NewRequestWithContext(ctx, "GET", update.URL, nil)
	if err != nil {
		return "", err
	}

	a.setAuthHeaders(req)

	resp, err := a.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	// Create file
	file, err := os.Create(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	// Copy content
	_, err = io.Copy(file, resp.Body)
	if err != nil {
		os.Remove(filePath)
		return "", err
	}

	return filePath, nil
}

// verifyUpdate verifies the integrity and signature of an update
func (a *Agent) verifyUpdate(filePath string, update *UpdateCommand) error {
	// Verify checksum
	checksum, err := security.CalculateChecksum(filePath)
	if err != nil {
		return err
	}

	if checksum != update.Checksum {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", update.Checksum, checksum)
	}

	// TODO: Verify signature
	// if err := security.VerifySignature(filePath, update.Signature, a.config.PublicKey); err != nil {
	//     return fmt.Errorf("signature verification failed: %w", err)
	// }

	return nil
}

// applyUpdate applies an update to the system
func (a *Agent) applyUpdate(ctx context.Context, filePath string, update *UpdateCommand) error {
	// Extract update package
	extractDir := filepath.Join(a.config.DataDir, "staging")
	os.RemoveAll(extractDir)
	os.MkdirAll(extractDir, 0755)

	cmd := exec.CommandContext(ctx, "tar", "-xzf", filePath, "-C", extractDir)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("extraction failed: %w", err)
	}

	// Run update script if present
	updateScript := filepath.Join(extractDir, "update.sh")
	if _, err := os.Stat(updateScript); err == nil {
		cmd := exec.CommandContext(ctx, "/bin/sh", updateScript)
		cmd.Dir = extractDir
		cmd.Env = append(os.Environ(),
			fmt.Sprintf("DEVICE_ID=%s", a.config.DeviceID),
			fmt.Sprintf("VERSION=%s", update.Version),
		)

		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("update script failed: %w\nOutput: %s", err, string(output))
		}

		log.Printf("Update script output: %s", string(output))
	}

	// Clean up staging directory
	os.RemoveAll(extractDir)

	return nil
}

// verifyHealth performs health checks after an update
func (a *Agent) verifyHealth(ctx context.Context) error {
	// Basic health check - ensure we can still communicate with server
	if err := a.sendHeartbeat(ctx); err != nil {
		return fmt.Errorf("heartbeat failed: %w", err)
	}

	// TODO: Add more comprehensive health checks
	// - Check critical services
	// - Verify system resources
	// - Test functionality

	return nil
}

// rollback rolls back a failed update
func (a *Agent) rollback(ctx context.Context, update *UpdateCommand) error {
	log.Printf("Rolling back update %s", update.ID)

	// TODO: Implement rollback mechanism
	// - Restore previous version
	// - Restart services
	// - Verify rollback success

	return fmt.Errorf("rollback not implemented")
}

// reportUpdateStatus reports the status of an update to the server
func (a *Agent) reportUpdateStatus(ctx context.Context, updateID, status, errorMsg string) error {
	url := fmt.Sprintf("%s/api/v1/devices/%s/updates/%s/status",
		a.config.ServerURL, a.config.DeviceID, updateID)

	payload := map[string]interface{}{
		"status":    status,
		"timestamp": time.Now(),
	}
	if errorMsg != "" {
		payload["error"] = errorMsg
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		return err
	}

	a.setAuthHeaders(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

// metricsLoop collects and sends metrics periodically
func (a *Agent) metricsLoop(ctx context.Context) {
	if a.config.MetricsInterval <= 0 {
		<-ctx.Done()
		return
	}

	ticker := time.NewTicker(a.config.MetricsInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			metrics := a.collectMetrics()
			a.sendMetrics(ctx, metrics)
		}
	}
}

// collectMetrics gathers system metrics
func (a *Agent) collectMetrics() *Metrics {
	sysMetrics, err := a.metricsCollector.Collect()
	if err != nil {
		log.Printf("Failed to collect metrics: %v", err)
		return &Metrics{
			Timestamp: time.Now(),
		}
	}

	// Convert to agent metrics format
	metrics := &Metrics{
		Timestamp:   sysMetrics.Timestamp,
		CPUUsage:    sysMetrics.CPU.UsagePercent,
		MemoryUsage: sysMetrics.Memory.UsedPercent,
		DiskUsage:   sysMetrics.Disk.UsedPercent,
		Custom: map[string]interface{}{
			"load_avg_1":       sysMetrics.CPU.LoadAvg1,
			"load_avg_5":       sysMetrics.CPU.LoadAvg5,
			"load_avg_15":      sysMetrics.CPU.LoadAvg15,
			"memory_total":     sysMetrics.Memory.Total,
			"memory_available": sysMetrics.Memory.Available,
			"disk_total":       sysMetrics.Disk.Total,
			"disk_free":        sysMetrics.Disk.Free,
			"network_sent":     sysMetrics.Network.TotalSent,
			"network_recv":     sysMetrics.Network.TotalRecv,
			"uptime":           sysMetrics.System.Uptime,
			"agent_cpu":        sysMetrics.Process.AgentCPU,
			"agent_memory":     sysMetrics.Process.AgentMem,
		},
	}

	// Add temperature if available
	if sysMetrics.Temperature != nil && sysMetrics.Temperature.CPU > 0 {
		metrics.Temperature = sysMetrics.Temperature.CPU
	}

	return metrics
}

// sendMetrics sends metrics to the server with retry logic
func (a *Agent) sendMetrics(ctx context.Context, metrics *Metrics) error {
	url := fmt.Sprintf("%s/api/v1/devices/%s/metrics", a.config.ServerURL, a.config.DeviceID)

	payload, err := json.Marshal(metrics)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payload))
	if err != nil {
		return err
	}

	a.setAuthHeaders(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		// Store metrics for later retry
		if a.stateStore != nil {
			a.stateStore.BufferMetrics(metrics)
		}
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		// Store metrics for later retry
		if a.stateStore != nil {
			a.stateStore.BufferMetrics(metrics)
		}
		return fmt.Errorf("metrics submission failed with status %d", resp.StatusCode)
	}

	// Try to send any buffered metrics
	if a.stateStore != nil {
		go a.sendBufferedMetrics(context.Background())
	}

	return nil
}

// sendBufferedMetrics sends previously buffered metrics
func (a *Agent) sendBufferedMetrics(ctx context.Context) {
	if a.stateStore == nil {
		return
	}

	buffered, err := a.stateStore.GetUnsentMetrics(100)
	if err != nil || len(buffered) == 0 {
		return
	}

	log.Printf("Sending %d buffered metrics", len(buffered))

	for _, m := range buffered {
		// Create a copy to avoid reference issues
		metricsCopy := m
		if err := a.sendMetrics(ctx, &metricsCopy); err != nil {
			log.Printf("Failed to send buffered metrics: %v", err)
			break
		}
	}
}

// updateState updates the agent's current state
func (a *Agent) updateState(status string, progress int, errorMsg string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.currentState.Status = status
	a.currentState.UpdateProgress = progress
	a.currentState.Error = errorMsg
	if status == "idle" && progress == 100 {
		a.currentState.LastUpdate = time.Now()
	}
}

// setAuthHeaders sets authentication headers on a request
func (a *Agent) setAuthHeaders(req *http.Request) {
	if a.config.APIKey != "" {
		req.Header.Set("X-API-Key", a.config.APIKey)
	} else if a.config.AuthToken != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", a.config.AuthToken))
	}
}

// Helper functions

// GenerateDeviceID generates a unique device ID
func GenerateDeviceID() string {
	// Use MAC address or machine ID
	// For now, use hostname + timestamp
	hostname, _ := os.Hostname()
	return fmt.Sprintf("%s-%d", hostname, time.Now().Unix())
}

func getHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return hostname
}

func getIPAddress() string {
	// TODO: Implement proper IP address detection
	return "127.0.0.1"
}

// Deprecated helper functions - kept for compatibility
func getCPUUsage() float64 {
	return 0.0
}

func getMemoryUsage() float64 {
	return 0.0
}

func getDiskUsage() float64 {
	return 0.0
}

// IsHealthy returns the current health status of the agent
func (a *Agent) IsHealthy() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()

	// Check if last heartbeat was recent
	if time.Since(a.currentState.LastHeartbeat) > a.config.HeartbeatInterval*3 {
		return false
	}

	// Check if there's a critical error
	if a.currentState.Status == "error" && a.currentState.Error != "" {
		return false
	}

	return a.healthStatus
}

// GetState returns a copy of the current state
func (a *Agent) GetState() *State {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.currentState == nil {
		return nil
	}

	stateCopy := *a.currentState
	return &stateCopy
}

// Cleanup performs cleanup operations before shutdown
func (a *Agent) Cleanup() error {
	log.Println("Performing agent cleanup...")

	// Save current state
	a.mu.RLock()
	state := *a.currentState
	a.mu.RUnlock()

	if a.stateStore != nil {
		if err := a.stateStore.SaveState(&state); err != nil {
			log.Printf("Failed to save state: %v", err)
		}
		a.stateStore.Close()
	}

	// Flush any pending metrics
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	select {
	case metrics := <-a.metricsChan:
		a.sendMetrics(ctx, metrics)
	default:
	}

	return nil
}

// UpdateConfig updates the agent's configuration
func (a *Agent) UpdateConfig(cfg *config.AgentConfig) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Validate critical fields haven't changed
	if cfg.DeviceID != a.config.DeviceID {
		return fmt.Errorf("cannot change device ID")
	}

	a.config = cfg
	log.Printf("Configuration updated")
	return nil
}

// Marshal serializes the state for persistence
func (s *State) Marshal() ([]byte, error) {
	return json.Marshal(s)
}

// Unmarshal deserializes the state from persistence
func (s *State) Unmarshal(data []byte) error {
	return json.Unmarshal(data, s)
}
