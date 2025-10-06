package update

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"log/slog"
)

// DeviceHTTPClient implements HTTPClient for real device communication
type DeviceHTTPClient struct {
	client    *http.Client
	baseURL   string // Base URL for device API
	apiKey    string // API key for authentication
	tlsVerify bool   // Whether to verify TLS certificates
}

// UpdateCommand represents an update command sent to devices
type UpdateCommand struct {
	Type      string          `json:"type"`
	Version   string          `json:"version"`
	Manifest  json.RawMessage `json:"manifest"`
	Timestamp time.Time       `json:"timestamp"`
	Signature string          `json:"signature,omitempty"`
}

// DeviceResponse represents a response from a device
type DeviceResponse struct {
	Success bool                   `json:"success"`
	Message string                 `json:"message,omitempty"`
	Data    map[string]interface{} `json:"data,omitempty"`
	Error   string                 `json:"error,omitempty"`
}

// NewDeviceHTTPClient creates a new HTTP client for device communication
func NewDeviceHTTPClient(baseURL, apiKey string, tlsVerify bool) *DeviceHTTPClient {
	return &DeviceHTTPClient{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		baseURL:   baseURL,
		apiKey:    apiKey,
		tlsVerify: tlsVerify,
	}
}

// SendUpdate sends an update command to a device
func (d *DeviceHTTPClient) SendUpdate(ctx context.Context, deviceID string, manifest []byte) error {
	// Parse manifest to extract version info
	var manifestData map[string]interface{}
	if err := json.Unmarshal(manifest, &manifestData); err != nil {
		return fmt.Errorf("invalid manifest: %w", err)
	}

	version, _ := manifestData["version"].(string)

	cmd := UpdateCommand{
		Type:      "firmware_update",
		Version:   version,
		Manifest:  manifest,
		Timestamp: time.Now(),
	}

	// TODO: Add signature for secure updates
	// cmd.Signature = d.signCommand(cmd)

	cmdBytes, err := json.Marshal(cmd)
	if err != nil {
		return fmt.Errorf("failed to marshal command: %w", err)
	}

	// Construct device endpoint URL
	url := fmt.Sprintf("%s/devices/%s/update", d.baseURL, deviceID)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(cmdBytes))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", d.apiKey))

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send update: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("update failed with status %d: %s", resp.StatusCode, string(body))
	}

	var deviceResp DeviceResponse
	if err := json.NewDecoder(resp.Body).Decode(&deviceResp); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if !deviceResp.Success {
		return fmt.Errorf("device rejected update: %s", deviceResp.Error)
	}

	slog.Info("Update command sent successfully", "device", deviceID, "version", version)
	return nil
}

// GetDeviceStatus gets the current status of a device
func (d *DeviceHTTPClient) GetDeviceStatus(ctx context.Context, deviceID string) (*DeviceStatus, error) {
	url := fmt.Sprintf("%s/devices/%s/status", d.baseURL, deviceID)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", d.apiKey))

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get status: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var status struct {
		DeviceID       string    `json:"device_id"`
		Online         bool      `json:"online"`
		CurrentVersion string    `json:"current_version"`
		LastSeen       time.Time `json:"last_seen"`
		Health         string    `json:"health"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &DeviceStatus{
		DeviceID:       status.DeviceID,
		Online:         status.Online,
		CurrentVersion: status.CurrentVersion,
		LastSeen:       status.LastSeen,
		Health:         status.Health,
	}, nil
}

// PollDeviceHealth continuously monitors device health
func (d *DeviceHTTPClient) PollDeviceHealth(ctx context.Context, deviceID string) <-chan HealthReport {
	healthChan := make(chan HealthReport)

	go func() {
		defer close(healthChan)

		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		retryCount := 0
		maxRetries := 10

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				health, err := d.checkDeviceHealth(ctx, deviceID)
				if err != nil {
					retryCount++
					if retryCount > maxRetries {
						healthChan <- HealthReport{
							DeviceID:  deviceID,
							Timestamp: time.Now(),
							Healthy:   false,
							Error:     fmt.Errorf("max retries exceeded: %w", err),
						}
						return
					}
					slog.Warn("Health check failed, retrying", "device", deviceID, "error", err, "retry", retryCount)
					continue
				}

				retryCount = 0 // Reset on success
				healthChan <- *health

				// If device reports update complete, we're done
				if health.Metrics != nil {
					if status, ok := health.Metrics["update_status"].(string); ok && status == "completed" {
						return
					}
				}
			}
		}
	}()

	return healthChan
}

// checkDeviceHealth performs a single health check
func (d *DeviceHTTPClient) checkDeviceHealth(ctx context.Context, deviceID string) (*HealthReport, error) {
	url := fmt.Sprintf("%s/devices/%s/health", d.baseURL, deviceID)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", d.apiKey))

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to check health: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("health check failed with status %d: %s", resp.StatusCode, string(body))
	}

	var healthData struct {
		Healthy bool                   `json:"healthy"`
		Metrics map[string]interface{} `json:"metrics"`
		Error   string                 `json:"error,omitempty"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&healthData); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	report := &HealthReport{
		DeviceID:  deviceID,
		Timestamp: time.Now(),
		Healthy:   healthData.Healthy,
		Metrics:   healthData.Metrics,
	}

	if healthData.Error != "" {
		report.Error = fmt.Errorf("%s", healthData.Error)
	}

	return report, nil
}

// SetTimeout sets the HTTP client timeout
func (d *DeviceHTTPClient) SetTimeout(timeout time.Duration) {
	d.client.Timeout = timeout
}

// SetTransport sets a custom transport (useful for testing)
func (d *DeviceHTTPClient) SetTransport(transport http.RoundTripper) {
	d.client.Transport = transport
}
