package update

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"fleetd.sh/internal/fleet"
	"log/slog"
)

// Client implements the fleet.UpdateClient interface for real device updates
type Client struct {
	db         *sql.DB
	campaigns  map[string]*Campaign
	mu         sync.RWMutex
	httpClient *DeviceHTTPClient // For device communication
}

// Campaign represents an active update campaign
type Campaign struct {
	ID          string
	Deployment  *fleet.Deployment
	Devices     []string
	DeviceState map[string]*DeviceUpdateState
	StartedAt   time.Time
	Status      string
	mu          sync.RWMutex
}

// DeviceUpdateState tracks update progress for a single device
type DeviceUpdateState struct {
	DeviceID    string
	Status      string // pending, downloading, installing, verifying, completed, failed
	Progress    int    // 0-100
	StartedAt   time.Time
	CompletedAt *time.Time
	Error       string
	RetryCount  int
	LastCheckIn time.Time
}

// DeviceStatus represents current device state
type DeviceStatus struct {
	DeviceID       string
	Online         bool
	CurrentVersion string
	LastSeen       time.Time
	Health         string
}

// HealthReport represents a device health check result
type HealthReport struct {
	DeviceID  string
	Timestamp time.Time
	Healthy   bool
	Metrics   map[string]interface{}
	Error     error
}

// NewClient creates a new update client
func NewClient(db *sql.DB, httpClient *DeviceHTTPClient) *Client {
	return &Client{
		db:         db,
		campaigns:  make(map[string]*Campaign),
		httpClient: httpClient,
	}
}

// CreateCampaign creates a new update campaign
func (c *Client) CreateCampaign(ctx context.Context, deployment *fleet.Deployment, devices []string) (string, error) {
	campaignID := fmt.Sprintf("campaign-%s-%d", deployment.ID, time.Now().Unix())

	campaign := &Campaign{
		ID:          campaignID,
		Deployment:  deployment,
		Devices:     devices,
		DeviceState: make(map[string]*DeviceUpdateState),
		StartedAt:   time.Now(),
		Status:      "running",
	}

	// Initialize device states
	for _, deviceID := range devices {
		campaign.DeviceState[deviceID] = &DeviceUpdateState{
			DeviceID:    deviceID,
			Status:      "pending",
			Progress:    0,
			StartedAt:   time.Now(),
			LastCheckIn: time.Now(),
		}
	}

	c.mu.Lock()
	c.campaigns[campaignID] = campaign
	c.mu.Unlock()

	// Start the campaign in a goroutine
	go c.runCampaign(context.Background(), campaign)

	// Store campaign in database
	_, err := c.db.ExecContext(ctx, `
		INSERT INTO campaign (id, deployment_id, status, started_at)
		VALUES (?, ?, ?, ?)`,
		campaignID, deployment.ID, "running", time.Now())
	if err != nil {
		return "", fmt.Errorf("failed to store campaign: %w", err)
	}

	return campaignID, nil
}

// runCampaign executes the update campaign
func (c *Client) runCampaign(ctx context.Context, campaign *Campaign) {
	slog.Info("Starting update campaign", "campaign", campaign.ID, "devices", len(campaign.Devices))

	// Create a worker pool for parallel device updates
	workerCount := 10 // Can be configurable
	if len(campaign.Devices) < workerCount {
		workerCount = len(campaign.Devices)
	}

	deviceChan := make(chan string, len(campaign.Devices))
	var wg sync.WaitGroup

	// Start workers
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for deviceID := range deviceChan {
				c.updateDevice(ctx, campaign, deviceID)
			}
		}(i)
	}

	// Queue devices
	for _, deviceID := range campaign.Devices {
		deviceChan <- deviceID
	}
	close(deviceChan)

	// Wait for all updates to complete
	wg.Wait()

	// Update campaign status
	campaign.mu.Lock()
	failed := 0
	succeeded := 0
	for _, state := range campaign.DeviceState {
		if state.Status == "completed" {
			succeeded++
		} else if state.Status == "failed" {
			failed++
		}
	}

	if failed > 0 {
		campaign.Status = "failed"
	} else {
		campaign.Status = "completed"
	}
	campaign.mu.Unlock()

	slog.Info("Campaign completed", "campaign", campaign.ID, "succeeded", succeeded, "failed", failed)

	// Update database
	c.db.ExecContext(ctx, `
		UPDATE campaign SET status = ?, completed_at = ?
		WHERE id = ?`,
		campaign.Status, time.Now(), campaign.ID)
}

// updateDevice handles the update process for a single device
func (c *Client) updateDevice(ctx context.Context, campaign *Campaign, deviceID string) {
	// Update status to downloading
	c.updateDeviceState(campaign, deviceID, "downloading", 10, "")

	// Send update command to device
	err := c.httpClient.SendUpdate(ctx, deviceID, campaign.Deployment.Manifest)
	if err != nil {
		c.updateDeviceState(campaign, deviceID, "failed", 0, err.Error())
		slog.Error("Failed to send update", "device", deviceID, "error", err)
		return
	}

	// Monitor device health during update
	c.updateDeviceState(campaign, deviceID, "installing", 50, "")

	// Poll for completion
	healthChan := c.httpClient.PollDeviceHealth(ctx, deviceID)
	timeout := time.After(30 * time.Minute) // Configurable timeout

	for {
		select {
		case health := <-healthChan:
			if health.Error != nil {
				c.updateDeviceState(campaign, deviceID, "failed", 0, health.Error.Error())
				return
			}
			if health.Healthy {
				c.updateDeviceState(campaign, deviceID, "completed", 100, "")
				slog.Info("Device update completed", "device", deviceID)
				return
			}
		case <-timeout:
			c.updateDeviceState(campaign, deviceID, "failed", 0, "timeout")
			slog.Error("Device update timeout", "device", deviceID)
			return
		case <-ctx.Done():
			c.updateDeviceState(campaign, deviceID, "failed", 0, "cancelled")
			return
		}
	}
}

// updateDeviceState updates the state of a device in the campaign
func (c *Client) updateDeviceState(campaign *Campaign, deviceID, status string, progress int, errorMsg string) {
	campaign.mu.Lock()
	defer campaign.mu.Unlock()

	state := campaign.DeviceState[deviceID]
	state.Status = status
	state.Progress = progress
	state.LastCheckIn = time.Now()

	if errorMsg != "" {
		state.Error = errorMsg
	}

	if status == "completed" || status == "failed" {
		now := time.Now()
		state.CompletedAt = &now
	}

	// Update database
	c.db.Exec(`
		INSERT OR REPLACE INTO device_campaign_state
		(campaign_id, device_id, status, progress, error, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		campaign.ID, deviceID, status, progress, errorMsg, time.Now())
}

// GetCampaignStatus returns the current status of a campaign
func (c *Client) GetCampaignStatus(ctx context.Context, campaignID string) (*fleet.CampaignStatus, error) {
	c.mu.RLock()
	campaign, exists := c.campaigns[campaignID]
	c.mu.RUnlock()

	if !exists {
		// Try to load from database
		var status string
		err := c.db.QueryRowContext(ctx,
			"SELECT status FROM campaign WHERE id = ?", campaignID).Scan(&status)
		if err != nil {
			return nil, fmt.Errorf("campaign not found: %s", campaignID)
		}

		// Return basic status from DB
		return &fleet.CampaignStatus{
			ID:     campaignID,
			Status: status,
		}, nil
	}

	campaign.mu.RLock()
	defer campaign.mu.RUnlock()

	// Calculate progress
	progress := fleet.DeploymentProgress{
		Total: len(campaign.Devices),
	}

	for _, state := range campaign.DeviceState {
		switch state.Status {
		case "pending":
			progress.Pending++
		case "downloading", "installing":
			progress.Running++
		case "completed":
			progress.Succeeded++
		case "failed":
			progress.Failed++
		}
	}

	return &fleet.CampaignStatus{
		ID:        campaignID,
		Status:    campaign.Status,
		Progress:  progress,
		UpdatedAt: time.Now(),
	}, nil
}

// PauseCampaign pauses an active campaign
func (c *Client) PauseCampaign(ctx context.Context, campaignID string) error {
	c.mu.RLock()
	campaign, exists := c.campaigns[campaignID]
	c.mu.RUnlock()

	if !exists {
		return fmt.Errorf("campaign not found: %s", campaignID)
	}

	campaign.mu.Lock()
	campaign.Status = "paused"
	campaign.mu.Unlock()

	// Update database
	_, err := c.db.ExecContext(ctx,
		"UPDATE campaign SET status = ? WHERE id = ?",
		"paused", campaignID)

	return err
}

// ResumeCampaign resumes a paused campaign
func (c *Client) ResumeCampaign(ctx context.Context, campaignID string) error {
	c.mu.RLock()
	campaign, exists := c.campaigns[campaignID]
	c.mu.RUnlock()

	if !exists {
		return fmt.Errorf("campaign not found: %s", campaignID)
	}

	campaign.mu.Lock()
	campaign.Status = "running"
	campaign.mu.Unlock()

	// Update database
	_, err := c.db.ExecContext(ctx,
		"UPDATE campaign SET status = ? WHERE id = ?",
		"running", campaignID)

	return err
}

// CancelCampaign cancels an active campaign
func (c *Client) CancelCampaign(ctx context.Context, campaignID string) error {
	c.mu.RLock()
	campaign, exists := c.campaigns[campaignID]
	c.mu.RUnlock()

	if !exists {
		return fmt.Errorf("campaign not found: %s", campaignID)
	}

	campaign.mu.Lock()
	campaign.Status = "cancelled"
	campaign.mu.Unlock()

	// Update database
	_, err := c.db.ExecContext(ctx,
		"UPDATE campaign SET status = ?, completed_at = ? WHERE id = ?",
		"cancelled", time.Now(), campaignID)

	return err
}
