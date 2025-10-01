package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

// FleetClient is a simple Go client for fleetd platform API
type FleetClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewFleetClient creates a new fleetd API client
func NewFleetClient(baseURL, apiKey string) *FleetClient {
	return &FleetClient{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Fleet represents a fleet entity
type Fleet struct {
	ID           string                 `json:"id"`
	Name         string                 `json:"name"`
	Description  string                 `json:"description"`
	DeviceCount  int                    `json:"device_count"`
	OnlineCount  int                    `json:"online_count"`
	Tags         map[string]string      `json:"tags"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
}

// Device represents a device entity
type Device struct {
	ID         string                 `json:"id"`
	FleetID    string                 `json:"fleet_id"`
	Name       string                 `json:"name"`
	HardwareID string                 `json:"hardware_id"`
	Status     string                 `json:"status"`
	LastSeen   time.Time              `json:"last_seen"`
	Version    string                 `json:"version"`
	Metadata   map[string]interface{} `json:"metadata"`
}

// Deployment represents a deployment entity
type Deployment struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	FleetID  string    `json:"fleet_id"`
	Status   string    `json:"status"`
	Progress Progress  `json:"progress"`
}

// Progress represents deployment progress
type Progress struct {
	Total      int `json:"total"`
	Pending    int `json:"pending"`
	Running    int `json:"running"`
	Succeeded  int `json:"succeeded"`
	Failed     int `json:"failed"`
	Percentage float64 `json:"percentage"`
}

// doRequest performs an HTTP request
func (c *FleetClient) doRequest(method, path string, body interface{}) (*http.Response, error) {
	var reqBody []byte
	var err error

	if body != nil {
		reqBody, err = json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
	}

	req, err := http.NewRequest(method, c.baseURL+path, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", c.apiKey)

	return c.httpClient.Do(req)
}

// CreateFleet creates a new fleet
func (c *FleetClient) CreateFleet(ctx context.Context, name, description string, tags map[string]string) (*Fleet, error) {
	payload := map[string]interface{}{
		"name":        name,
		"description": description,
		"tags":        tags,
	}

	resp, err := c.doRequest("POST", "/api/v1/fleets", payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var fleet Fleet
	if err := json.NewDecoder(resp.Body).Decode(&fleet); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &fleet, nil
}

// ListFleets lists all fleets
func (c *FleetClient) ListFleets(ctx context.Context) ([]Fleet, error) {
	resp, err := c.doRequest("GET", "/api/v1/fleets", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var result struct {
		Fleets []Fleet `json:"fleets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result.Fleets, nil
}

// GetFleet gets a specific fleet
func (c *FleetClient) GetFleet(ctx context.Context, fleetID string) (*Fleet, error) {
	resp, err := c.doRequest("GET", fmt.Sprintf("/api/v1/fleets/%s", fleetID), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var fleet Fleet
	if err := json.NewDecoder(resp.Body).Decode(&fleet); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &fleet, nil
}

// ListDevices lists devices in a fleet
func (c *FleetClient) ListDevices(ctx context.Context, fleetID string) ([]Device, error) {
	resp, err := c.doRequest("GET", fmt.Sprintf("/api/v1/fleets/%s/devices", fleetID), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var result struct {
		Devices []Device `json:"devices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result.Devices, nil
}

// CreateDeployment creates a new deployment
func (c *FleetClient) CreateDeployment(ctx context.Context, name, fleetID string, manifest map[string]interface{}) (*Deployment, error) {
	payload := map[string]interface{}{
		"name":     name,
		"fleet_id": fleetID,
		"manifest": manifest,
		"strategy": map[string]interface{}{
			"type": "rolling",
			"rolling_update": map[string]interface{}{
				"max_unavailable": "25%",
				"max_surge":       "25%",
			},
		},
	}

	resp, err := c.doRequest("POST", "/api/v1/deployments", payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var deployment Deployment
	if err := json.NewDecoder(resp.Body).Decode(&deployment); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &deployment, nil
}

// GetDeploymentStatus gets deployment status
func (c *FleetClient) GetDeploymentStatus(ctx context.Context, deploymentID string) (*Deployment, error) {
	resp, err := c.doRequest("GET", fmt.Sprintf("/api/v1/deployments/%s", deploymentID), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var deployment Deployment
	if err := json.NewDecoder(resp.Body).Decode(&deployment); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &deployment, nil
}

// SendCommand sends a command to a device
func (c *FleetClient) SendCommand(ctx context.Context, deviceID, command string, payload map[string]interface{}) error {
	cmdPayload := map[string]interface{}{
		"command": command,
		"payload": payload,
		"timeout": 60,
	}

	resp, err := c.doRequest("POST", fmt.Sprintf("/api/v1/devices/%s/command", deviceID), cmdPayload)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}

func main() {
	// Initialize client
	client := NewFleetClient(
		os.Getenv("FLEET_API_URL"),
		os.Getenv("FLEET_API_KEY"),
	)

	ctx := context.Background()

	// Example: Create a fleet
	fleet, err := client.CreateFleet(ctx, "Production IoT Devices", "Main production fleet", map[string]string{
		"environment": "production",
		"region":      "us-east",
	})
	if err != nil {
		log.Fatalf("Failed to create fleet: %v", err)
	}
	fmt.Printf("Created fleet: %s (ID: %s)\n", fleet.Name, fleet.ID)

	// Example: List all fleets
	fleets, err := client.ListFleets(ctx)
	if err != nil {
		log.Fatalf("Failed to list fleets: %v", err)
	}
	fmt.Printf("Found %d fleets\n", len(fleets))
	for _, f := range fleets {
		fmt.Printf("  - %s: %d devices (%d online)\n", f.Name, f.DeviceCount, f.OnlineCount)
	}

	// Example: List devices in a fleet
	devices, err := client.ListDevices(ctx, fleet.ID)
	if err != nil {
		log.Fatalf("Failed to list devices: %v", err)
	}
	fmt.Printf("Fleet has %d devices\n", len(devices))
	for _, d := range devices {
		fmt.Printf("  - %s: %s (last seen: %v)\n", d.Name, d.Status, d.LastSeen)
	}

	// Example: Create a deployment
	deployment, err := client.CreateDeployment(ctx, "Firmware v2.0.0", fleet.ID, map[string]interface{}{
		"version": "2.0.0",
		"url":     "https://updates.example.com/firmware-v2.0.0.bin",
		"checksum": "sha256:abcdef1234567890",
	})
	if err != nil {
		log.Fatalf("Failed to create deployment: %v", err)
	}
	fmt.Printf("Created deployment: %s (ID: %s)\n", deployment.Name, deployment.ID)

	// Monitor deployment progress
	for {
		status, err := client.GetDeploymentStatus(ctx, deployment.ID)
		if err != nil {
			log.Fatalf("Failed to get deployment status: %v", err)
		}

		fmt.Printf("Deployment progress: %.1f%% (Success: %d, Failed: %d, Running: %d)\n",
			status.Progress.Percentage,
			status.Progress.Succeeded,
			status.Progress.Failed,
			status.Progress.Running,
		)

		if status.Status == "succeeded" || status.Status == "failed" {
			fmt.Printf("Deployment %s\n", status.Status)
			break
		}

		time.Sleep(10 * time.Second)
	}

	// Example: Send command to a device
	if len(devices) > 0 {
		deviceID := devices[0].ID
		err := client.SendCommand(ctx, deviceID, "reboot", map[string]interface{}{
			"delay": 60,
		})
		if err != nil {
			log.Fatalf("Failed to send command: %v", err)
		}
		fmt.Printf("Sent reboot command to device %s\n", deviceID)
	}
}