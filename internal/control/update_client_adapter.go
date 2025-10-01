package control

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"fleetd.sh/internal/fleet"
)

// UpdateClientAdapter adapts the orchestrator's UpdateClient interface
// to the actual UpdateService implementation
type UpdateClientAdapter struct {
	deviceAPI *DeviceAPIClient
	campaigns map[string]*campaignState // Track active campaigns
	mu        sync.RWMutex              // Protect campaigns map
	db        *sql.DB                   // Database connection for updating device statuses
}

// campaignState tracks the state of an update campaign
type campaignState struct {
	ID         string
	Deployment *fleet.Deployment
	Devices    []string
	StartedAt  time.Time
	Status     string
}

// NewUpdateClientAdapter creates a new update client adapter
func NewUpdateClientAdapter(deviceAPI *DeviceAPIClient, db *sql.DB) *UpdateClientAdapter {
	return &UpdateClientAdapter{
		deviceAPI: deviceAPI,
		campaigns: make(map[string]*campaignState),
		db:        db,
	}
}

// CreateCampaign creates a new update campaign for a deployment
func (a *UpdateClientAdapter) CreateCampaign(ctx context.Context, deployment *fleet.Deployment, devices []string) (string, error) {
	// Parse the manifest to get artifact details
	var manifest fleet.Manifest
	if err := json.Unmarshal(deployment.Manifest, &manifest); err != nil {
		return "", fmt.Errorf("failed to parse manifest: %w", err)
	}

	// For now, we'll create a simple campaign ID
	campaignID := fmt.Sprintf("campaign-%s-%d", deployment.ID, time.Now().Unix())

	// Store campaign state
	a.mu.Lock()
	a.campaigns[campaignID] = &campaignState{
		ID:         campaignID,
		Deployment: deployment,
		Devices:    devices,
		StartedAt:  time.Now(),
		Status:     "running",
	}
	a.mu.Unlock()

	// In a real implementation, this would:
	// 1. Call the UpdateService to create update campaigns for each device
	// 2. Upload artifacts to storage (S3, GCS, etc.)
	// 3. Trigger device updates via the device API

	// For each artifact in the manifest
	for _, artifact := range manifest.Spec.Template.Spec.Artifacts {
		// Create update payload
		updatePayload := map[string]interface{}{
			"artifact":   artifact.Name,
			"version":    artifact.Version,
			"url":        artifact.URL,
			"checksum":   artifact.Checksum,
			"target":     artifact.Target,
			"deployment": deployment.ID,
		}

		// Send update to each device
		for _, deviceID := range devices {
			// This would call the actual UpdateService
			// For now, we'll simulate the call
			fmt.Printf("Creating update for device %s: %v\n", deviceID, updatePayload)
		}
	}

	return campaignID, nil
}

// GetCampaignStatus returns the status of a campaign
func (a *UpdateClientAdapter) GetCampaignStatus(ctx context.Context, campaignID string) (*fleet.CampaignStatus, error) {
	a.mu.RLock()
	campaign, exists := a.campaigns[campaignID]
	a.mu.RUnlock()
	if !exists {
		return nil, fmt.Errorf("campaign not found: %s", campaignID)
	}


	// In a real implementation, this would query the UpdateService
	// to get actual device update statuses

	// For now, simulate progress
	elapsed := time.Since(campaign.StartedAt)
	totalDevices := len(campaign.Devices)

	// Make sure we have devices
	if totalDevices == 0 {
		return nil, fmt.Errorf("campaign has no devices")
	}

	// Simulate gradual progress
	progress := fleet.DeploymentProgress{
		Total:      totalDevices,
		Pending:    0,
		Running:    0,
		Succeeded:  0,
		Failed:     0,
		Percentage: 0,
	}

	// Simplified simulation: complete immediately after 50ms
	if elapsed > 50*time.Millisecond {
		// All complete
		progress.Succeeded = totalDevices
		progress.Percentage = 100
		campaign.Status = "succeeded"
		// Mark all as succeeded
		for _, deviceID := range campaign.Devices {
			a.db.ExecContext(ctx, `UPDATE device_deployment SET status = 'succeeded' WHERE deployment_id = ? AND device_id = ?`,
				campaign.Deployment.ID, deviceID)
		}
	} else {
		// Still running
		progress.Running = totalDevices
		campaign.Status = "running"
	}


	return &fleet.CampaignStatus{
		ID:        campaignID,
		Status:    campaign.Status,
		Progress:  progress,
		UpdatedAt: time.Now(),
	}, nil
}

// PauseCampaign pauses an active campaign
func (a *UpdateClientAdapter) PauseCampaign(ctx context.Context, campaignID string) error {
	campaign, exists := a.campaigns[campaignID]
	if !exists {
		return fmt.Errorf("campaign not found: %s", campaignID)
	}

	campaign.Status = "paused"

	// In a real implementation, this would:
	// 1. Call UpdateService to pause device updates
	// 2. Stop any in-progress downloads or installations

	fmt.Printf("Pausing campaign %s\n", campaignID)
	return nil
}

// ResumeCampaign resumes a paused campaign
func (a *UpdateClientAdapter) ResumeCampaign(ctx context.Context, campaignID string) error {
	campaign, exists := a.campaigns[campaignID]
	if !exists {
		return fmt.Errorf("campaign not found: %s", campaignID)
	}

	if campaign.Status != "paused" {
		return fmt.Errorf("campaign is not paused: %s", campaign.Status)
	}

	campaign.Status = "running"

	// In a real implementation, this would:
	// 1. Call UpdateService to resume device updates
	// 2. Restart downloads and installations

	fmt.Printf("Resuming campaign %s\n", campaignID)
	return nil
}

// CancelCampaign cancels an active campaign
func (a *UpdateClientAdapter) CancelCampaign(ctx context.Context, campaignID string) error {
	campaign, exists := a.campaigns[campaignID]
	if !exists {
		return fmt.Errorf("campaign not found: %s", campaignID)
	}

	campaign.Status = "cancelled"

	// In a real implementation, this would:
	// 1. Call UpdateService to cancel all pending updates
	// 2. Rollback any in-progress installations if possible
	// 3. Clean up temporary files and resources

	fmt.Printf("Cancelling campaign %s\n", campaignID)
	return nil
}

// GetDeviceStatuses returns the update status for specific devices
func (a *UpdateClientAdapter) GetDeviceStatuses(ctx context.Context, campaignID string, deviceIDs []string) (map[string]string, error) {
	// In a real implementation, this would query the UpdateService
	// to get actual per-device statuses

	statuses := make(map[string]string)
	for _, deviceID := range deviceIDs {
		// Simulate device statuses
		statuses[deviceID] = "running"
	}

	return statuses, nil
}