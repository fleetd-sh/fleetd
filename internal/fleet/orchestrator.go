package fleet

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"fleetd.sh/internal/config"
)

// Orchestrator manages deployment rollouts
type Orchestrator struct {
	db           *sql.DB
	updateClient UpdateClient // Interface to UpdateService
	mu           sync.Mutex
	deployments  map[string]*rolloutState
	config       *config.OrchestratorConfig
}

// UpdateClient interface for interacting with UpdateService
type UpdateClient interface {
	CreateCampaign(ctx context.Context, deployment *Deployment, devices []string) (string, error)
	GetCampaignStatus(ctx context.Context, campaignID string) (*CampaignStatus, error)
	PauseCampaign(ctx context.Context, campaignID string) error
	ResumeCampaign(ctx context.Context, campaignID string) error
	CancelCampaign(ctx context.Context, campaignID string) error
}

// CampaignStatus represents the status of an update campaign
type CampaignStatus struct {
	ID        string
	Status    string
	Progress  DeploymentProgress
	UpdatedAt time.Time
}

// rolloutState tracks the state of an ongoing rollout
type rolloutState struct {
	mu                 sync.RWMutex
	deployment         *Deployment
	manifest           *Manifest
	campaignID         string
	startedAt          time.Time
	currentStep        int // For canary deployments
	cancelCh           chan struct{}
	waitingForApproval bool // For canary/blue-green with approval
}

// NewOrchestrator creates a new deployment orchestrator
func NewOrchestrator(db *sql.DB, updateClient UpdateClient) *Orchestrator {
	return NewOrchestratorWithConfig(db, updateClient, config.DefaultOrchestratorConfig())
}

// NewOrchestratorWithConfig creates a new deployment orchestrator with custom config
func NewOrchestratorWithConfig(db *sql.DB, updateClient UpdateClient, cfg *config.OrchestratorConfig) *Orchestrator {
	return &Orchestrator{
		db:           db,
		updateClient: updateClient,
		deployments:  make(map[string]*rolloutState),
		config:       cfg,
	}
}

// StartDeployment begins rolling out a deployment
func (o *Orchestrator) StartDeployment(ctx context.Context, deploymentID string) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	// Check if already running
	if _, exists := o.deployments[deploymentID]; exists {
		return fmt.Errorf("deployment %s is already running", deploymentID)
	}

	// Load deployment from database
	deployment, manifest, err := o.loadDeployment(ctx, deploymentID)
	if err != nil {
		return fmt.Errorf("failed to load deployment: %w", err)
	}

	// Validate deployment can start
	if deployment.Status != DeploymentStatusPending {
		return fmt.Errorf("deployment is not in pending state: %s", deployment.Status)
	}

	// Get target devices
	devices, err := o.getTargetDevices(ctx, deploymentID)
	if err != nil {
		return fmt.Errorf("failed to get target devices: %w", err)
	}

	if len(devices) == 0 {
		return fmt.Errorf("no devices to deploy to")
	}

	// Create rollout state
	state := &rolloutState{
		deployment: deployment,
		manifest:   manifest,
		startedAt:  time.Now(),
		cancelCh:   make(chan struct{}),
	}
	o.deployments[deploymentID] = state

	// Start rollout based on strategy
	// Use a background context to avoid cancellation when the HTTP request completes
	go o.runRollout(context.Background(), state, devices)

	return nil
}

// runRollout executes the deployment rollout based on strategy
func (o *Orchestrator) runRollout(ctx context.Context, state *rolloutState, devices []string) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("runRollout panic", "deployment", state.deployment.ID, "panic", r)
		}
		o.mu.Lock()
		delete(o.deployments, state.deployment.ID)
		o.mu.Unlock()
	}()

	// Update status to running
	if err := o.updateDeploymentStatus(ctx, state.deployment.ID, DeploymentStatusRunning); err != nil {
		slog.Error("Failed to update deployment status", "error", err)
		return
	}

	var err error
	switch state.deployment.Strategy.Type {
	case "", "RollingUpdate":
		err = o.runRollingUpdate(ctx, state, devices)
	case "Canary":
		err = o.runCanary(ctx, state, devices)
	case "BlueGreen":
		err = o.runBlueGreen(ctx, state, devices)
	default:
		err = fmt.Errorf("unknown strategy: %s", state.deployment.Strategy.Type)
	}

	// Update final status
	if state.waitingForApproval {
		// Don't update to succeeded if we're waiting for approval
		// The deployment stays in running state
		slog.Info("Deployment waiting for approval", "deployment", state.deployment.ID)
	} else {
		finalStatus := DeploymentStatusSucceeded
		if err != nil {
			finalStatus = DeploymentStatusFailed
			slog.Error("Deployment failed", "deployment", state.deployment.ID, "error", err)
			o.recordEvent(ctx, state.deployment.ID, "", "deployment_failed", err.Error())
		} else {
			o.recordEvent(ctx, state.deployment.ID, "", "deployment_succeeded", "Deployment completed successfully")
		}

		o.updateDeploymentStatus(ctx, state.deployment.ID, finalStatus)
	}
}

// runRollingUpdate performs a rolling update deployment
func (o *Orchestrator) runRollingUpdate(ctx context.Context, state *rolloutState, devices []string) error {
	config := state.deployment.Strategy.RollingUpdate
	if config == nil {
		// Default values if not specified
		config = &RollingUpdate{
			MaxUnavailable: "25%",
			MaxSurge:       "25%",
		}
	}

	totalDevices := len(devices)
	if totalDevices == 0 {
		return nil
	}

	// Calculate batch size based on maxUnavailable
	batchSize := calculateBatchSize(config.MaxUnavailable, totalDevices)
	if batchSize <= 0 {
		batchSize = 1
	}

	// Deploy in batches
	for i := 0; i < totalDevices; i += batchSize {
		select {
		case <-state.cancelCh:
			return fmt.Errorf("deployment cancelled")
		default:
		}

		// Calculate the end index for this batch
		end := i + batchSize
		if end > totalDevices {
			end = totalDevices
		}

		batch := devices[i:end]

		// Deploy this batch
		if err := o.deployToBatch(ctx, state, batch); err != nil {
			return fmt.Errorf("batch deployment failed: %w", err)
		}

		// Wait between batches if configured
		if config.WaitTime > 0 && end < totalDevices {
			select {
			case <-time.After(time.Duration(config.WaitTime) * time.Second):
				// Continue to next batch
			case <-state.cancelCh:
				return fmt.Errorf("deployment cancelled during wait")
			}
		}
	}

	return nil
}

// calculateBatchSize calculates the batch size from a percentage or absolute value
func calculateBatchSize(maxUnavailable string, totalDevices int) int {
	if maxUnavailable == "" {
		// Default to 25%
		return int(math.Ceil(float64(totalDevices) * 0.25))
	}

	// Check if it's a percentage
	if strings.HasSuffix(maxUnavailable, "%") {
		percentStr := strings.TrimSuffix(maxUnavailable, "%")
		percent, err := strconv.ParseFloat(percentStr, 64)
		if err != nil {
			return int(math.Ceil(float64(totalDevices) * 0.25)) // Default to 25%
		}
		return int(math.Ceil(float64(totalDevices) * percent / 100.0))
	}

	// Try to parse as absolute number
	batch, err := strconv.Atoi(maxUnavailable)
	if err != nil {
		return int(math.Ceil(float64(totalDevices) * 0.25)) // Default to 25%
	}
	return batch
}

// runCanary performs a canary deployment
func (o *Orchestrator) runCanary(ctx context.Context, state *rolloutState, devices []string) error {
	config := state.deployment.Strategy.Canary
	if config == nil || len(config.Steps) == 0 {
		return fmt.Errorf("canary configuration missing or invalid")
	}

	totalDevices := len(devices)
	deployedDevices := 0

	for stepIndex, step := range config.Steps {
		select {
		case <-state.cancelCh:
			return fmt.Errorf("deployment cancelled")
		default:
		}

		state.currentStep = stepIndex

		// Calculate devices for this step
		stepDevices := int(math.Ceil(float64(totalDevices) * float64(step.Weight) / 100.0))
		if stepDevices > totalDevices {
			stepDevices = totalDevices
		}

		// Get devices for this canary step
		newDevices := stepDevices - deployedDevices
		if newDevices <= 0 {
			continue
		}

		batch := devices[deployedDevices : deployedDevices+newDevices]
		deployedDevices += newDevices

		// Deploy to canary batch
		slog.Info("Starting canary step", "step", stepIndex+1, "weight", step.Weight, "devices", newDevices)
		o.recordEvent(ctx, state.deployment.ID, "", "canary_step_started",
			fmt.Sprintf("Step %d: deploying to %d%% (%d devices)", stepIndex+1, step.Weight, newDevices))

		// For canary with approval, we need special handling
		if config.RequireApproval && stepIndex < len(config.Steps)-1 {
			// Don't wait for full completion, just start the deployment
			campaignID, err := o.updateClient.CreateCampaign(ctx, state.deployment, batch)
			if err != nil {
				o.recordEvent(ctx, state.deployment.ID, "", "canary_failed",
					fmt.Sprintf("Step %d failed: %v", stepIndex+1, err))
				return o.rollbackDeployment(ctx, state)
			}
			state.mu.Lock()
			state.campaignID = campaignID
			state.mu.Unlock()

			// Update device statuses
			for _, deviceID := range batch {
				o.updateDeviceStatus(ctx, state.deployment.ID, deviceID, "running", 0, "Deployment started")
			}

			// Don't monitor to completion - just verify it started
			time.Sleep(50 * time.Millisecond) // Brief wait to let it start
		} else {
			// Normal deployment - wait for completion
			if err := o.deployToBatch(ctx, state, batch); err != nil {
				// Canary failed, initiate rollback
				o.recordEvent(ctx, state.deployment.ID, "", "canary_failed",
					fmt.Sprintf("Step %d failed: %v", stepIndex+1, err))
				return o.rollbackDeployment(ctx, state)
			}
		}

		// Wait for analysis period
		if step.Duration > 0 {
			slog.Info("Waiting for canary analysis", "duration", step.Duration)
			select {
			case <-time.After(step.Duration):
			case <-state.cancelCh:
				return fmt.Errorf("deployment cancelled")
			}
		}

		// Run analysis if configured
		if config.Analysis != nil {
			passed, err := o.runCanaryAnalysis(ctx, state, stepIndex)
			if err != nil {
				return fmt.Errorf("canary analysis failed: %w", err)
			}
			if !passed {
				o.recordEvent(ctx, state.deployment.ID, "", "canary_analysis_failed",
					fmt.Sprintf("Step %d failed analysis", stepIndex+1))
				return o.rollbackDeployment(ctx, state)
			}
		}

		o.recordEvent(ctx, state.deployment.ID, "", "canary_step_succeeded",
			fmt.Sprintf("Step %d completed successfully", stepIndex+1))

		// If approval is required and this isn't the last step, pause
		if config.RequireApproval && stepIndex < len(config.Steps)-1 {
			slog.Info("Canary deployment waiting for approval", "step", stepIndex+1)
			o.recordEvent(ctx, state.deployment.ID, "", "canary_waiting_approval",
				fmt.Sprintf("Step %d completed, waiting for approval", stepIndex+1))

			// Mark that we're waiting for approval
			state.waitingForApproval = true

			// Update deployment status to indicate waiting for approval
			// In a real system, this would wait for an external approval signal
			// For now, we'll just return to keep the deployment in running state
			return nil
		}
	}

	return nil
}

// runBlueGreen performs a blue-green deployment
func (o *Orchestrator) runBlueGreen(ctx context.Context, state *rolloutState, devices []string) error {
	config := state.deployment.Strategy.BlueGreen
	if config == nil {
		config = &BlueGreen{
			AutoPromote:    true,
			PromoteTimeout: 30 * time.Minute,
		}
	}

	// Deploy to all devices (green environment)
	slog.Info("Deploying to green environment", "devices", len(devices))
	o.recordEvent(ctx, state.deployment.ID, "", "blue_green_started",
		fmt.Sprintf("Deploying to %d devices", len(devices)))

	if err := o.deployToBatch(ctx, state, devices); err != nil {
		return fmt.Errorf("green deployment failed: %w", err)
	}

	// Wait for promotion decision
	if config.AutoPromote {
		slog.Info("Waiting for auto-promotion", "timeout", config.PromoteTimeout)
		select {
		case <-time.After(config.PromoteTimeout):
			// Auto-promote
			o.recordEvent(ctx, state.deployment.ID, "", "blue_green_promoted", "Auto-promoted after timeout")
		case <-state.cancelCh:
			return fmt.Errorf("deployment cancelled")
		}
	} else {
		// Wait for manual promotion
		o.recordEvent(ctx, state.deployment.ID, "", "blue_green_awaiting_promotion",
			"Awaiting manual promotion")
		// This would typically wait for an API call to promote
	}

	// Scale down blue (old version) after delay
	if config.ScaleDownDelay > 0 {
		time.Sleep(config.ScaleDownDelay)
	}

	return nil
}

// deployToBatch deploys to a batch of devices
func (o *Orchestrator) deployToBatch(ctx context.Context, state *rolloutState, devices []string) error {
	// Create update campaign for this batch
	campaignID, err := o.updateClient.CreateCampaign(ctx, state.deployment, devices)
	if err != nil {
		return fmt.Errorf("failed to create campaign: %w", err)
	}

	// Store campaign ID
	state.mu.Lock()
	state.campaignID = campaignID
	state.mu.Unlock()

	// Update device deployment status
	for _, deviceID := range devices {
		o.updateDeviceStatus(ctx, state.deployment.ID, deviceID, "running", 0, "Deployment started")
	}

	// Monitor campaign until completion
	return o.monitorCampaign(ctx, state, devices)
}

// monitorCampaign monitors an update campaign until completion
func (o *Orchestrator) monitorCampaign(ctx context.Context, state *rolloutState, devices []string) error {
	// Use configured monitor interval
	tickerDuration := o.config.MonitorInterval

	if o.config.EnableDebugLogging {
		slog.Debug("Starting campaign monitoring", "campaign", state.campaignID, "interval", tickerDuration)
	}

	ticker := time.NewTicker(tickerDuration)
	defer ticker.Stop()

	timeout := time.After(2 * time.Hour) // Maximum deployment time

	// Check status immediately before waiting
	status, err := o.updateClient.GetCampaignStatus(ctx, state.campaignID)
	if err == nil {
		o.updateProgressFromCampaign(ctx, state.deployment.ID, status)
		if status.Progress.Succeeded+status.Progress.Failed == status.Progress.Total && status.Progress.Total > 0 {
			if status.Progress.Failed > 0 {
				return fmt.Errorf("%d devices failed", status.Progress.Failed)
			}
			// Don't update deployment status here - let the calling function handle final status
			return nil
		}
	}

	tickCount := 0
	for {
		select {
		case <-ticker.C:
			tickCount++
			fmt.Printf("DEBUG: Ticker fired %d times for campaign %s\n", tickCount, state.campaignID)
			status, err := o.updateClient.GetCampaignStatus(ctx, state.campaignID)
			if err != nil {
				slog.Warn("Failed to get campaign status", "error", err)
				continue
			}

			// Update device statuses based on campaign progress
			o.updateProgressFromCampaign(ctx, state.deployment.ID, status)

			// Check if campaign is complete
			if status.Progress.Succeeded+status.Progress.Failed == status.Progress.Total {
				if status.Progress.Failed > 0 {
					return fmt.Errorf("%d devices failed", status.Progress.Failed)
				}
				// Don't update deployment status here - let the calling function handle final status
				return nil
			}

		case <-timeout:
			return fmt.Errorf("deployment timeout")

		case <-state.cancelCh:
			o.updateClient.CancelCampaign(ctx, state.campaignID)
			return fmt.Errorf("deployment cancelled")
		}
	}
}

// Helper functions

func (o *Orchestrator) loadDeployment(ctx context.Context, deploymentID string) (*Deployment, *Manifest, error) {
	query := `
		SELECT id, name, namespace, manifest, status, strategy, selector, created_by, created_at, updated_at
		FROM deployment
		WHERE id = $1
	`

	var deployment Deployment
	var manifestJSON, strategyJSON, selectorJSON []byte

	err := o.db.QueryRowContext(ctx, query, deploymentID).Scan(
		&deployment.ID,
		&deployment.Name,
		&deployment.Namespace,
		&manifestJSON,
		&deployment.Status,
		&strategyJSON,
		&selectorJSON,
		&deployment.CreatedBy,
		&deployment.CreatedAt,
		&deployment.UpdatedAt,
	)
	if err != nil {
		return nil, nil, err
	}

	deployment.Manifest = manifestJSON
	json.Unmarshal(strategyJSON, &deployment.Strategy)
	json.Unmarshal(selectorJSON, &deployment.Selector)

	var manifest Manifest
	if err := json.Unmarshal(manifestJSON, &manifest); err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal manifest: %w", err)
	}

	return &deployment, &manifest, nil
}

func (o *Orchestrator) getTargetDevices(ctx context.Context, deploymentID string) ([]string, error) {
	query := `
		SELECT device_id FROM device_deployment
		WHERE deployment_id = $1
		ORDER BY device_id
	`

	rows, err := o.db.QueryContext(ctx, query, deploymentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var devices []string
	for rows.Next() {
		var deviceID string
		if err := rows.Scan(&deviceID); err != nil {
			return nil, err
		}
		devices = append(devices, deviceID)
	}

	return devices, nil
}

func (o *Orchestrator) calculateBatchSize(value string, total int) (int, error) {
	if strings.HasSuffix(value, "%") {
		percentStr := strings.TrimSuffix(value, "%")
		percent, err := strconv.Atoi(percentStr)
		if err != nil {
			return 0, err
		}
		batchSize := int(math.Ceil(float64(total) * float64(percent) / 100.0))
		return batchSize, nil
	}

	return strconv.Atoi(value)
}

func (o *Orchestrator) updateDeploymentStatus(ctx context.Context, deploymentID string, status DeploymentStatus) error {
	query := `UPDATE deployment SET status = $1, updated_at = $2 WHERE id = $3`
	_, err := o.db.ExecContext(ctx, query, status, time.Now(), deploymentID)
	return err
}

func (o *Orchestrator) updateDeviceStatus(ctx context.Context, deploymentID, deviceID, status string, progress int, message string) error {
	query := `
		UPDATE device_deployment
		SET status = $1, progress = $2, message = $3, started_at = CASE WHEN started_at IS NULL THEN $4 ELSE started_at END
		WHERE deployment_id = $5 AND device_id = $6
	`
	_, err := o.db.ExecContext(ctx, query, status, progress, message, time.Now(), deploymentID, deviceID)
	return err
}

func (o *Orchestrator) recordEvent(ctx context.Context, deploymentID, deviceID, eventType, message string) {
	query := `
		INSERT INTO deployment_event (deployment_id, device_id, event_type, message, created_at)
		VALUES (?, ?, ?, ?, ?)
	`
	o.db.ExecContext(ctx, query, deploymentID, deviceID, eventType, message, time.Now())
}

func (o *Orchestrator) updateProgressFromCampaign(ctx context.Context, deploymentID string, status *CampaignStatus) {
	// Update overall deployment progress
	// The UpdateClientAdapter already updates device_deployment records
	// Note: Don't update the overall deployment status here - that should only
	// be done in runRollout after all batches complete, not after each campaign
}

func (o *Orchestrator) runCanaryAnalysis(ctx context.Context, state *rolloutState, stepIndex int) (bool, error) {
	// Implement canary analysis based on metrics
	// For now, always pass
	return true, nil
}

func (o *Orchestrator) rollbackDeployment(ctx context.Context, state *rolloutState) error {
	o.updateDeploymentStatus(ctx, state.deployment.ID, DeploymentStatusRollingBack)
	// Implement rollback logic
	return fmt.Errorf("rollback not yet implemented")
}

// PauseDeployment pauses an active deployment
func (o *Orchestrator) PauseDeployment(ctx context.Context, deploymentID string) error {
	o.mu.Lock()
	state, exists := o.deployments[deploymentID]
	o.mu.Unlock()

	if !exists {
		return fmt.Errorf("deployment %s is not running", deploymentID)
	}

	state.mu.RLock()
	campaignID := state.campaignID
	state.mu.RUnlock()

	if campaignID != "" {
		return o.updateClient.PauseCampaign(ctx, campaignID)
	}

	return o.updateDeploymentStatus(ctx, deploymentID, DeploymentStatusPaused)
}

// ResumeDeployment resumes a paused deployment
func (o *Orchestrator) ResumeDeployment(ctx context.Context, deploymentID string) error {
	o.mu.Lock()
	state, exists := o.deployments[deploymentID]
	o.mu.Unlock()

	if !exists {
		// Restart deployment
		return o.StartDeployment(ctx, deploymentID)
	}

	state.mu.RLock()
	campaignID := state.campaignID
	state.mu.RUnlock()

	if campaignID != "" {
		return o.updateClient.ResumeCampaign(ctx, campaignID)
	}

	return o.updateDeploymentStatus(ctx, deploymentID, DeploymentStatusRunning)
}

// CancelDeployment cancels an active deployment
func (o *Orchestrator) CancelDeployment(ctx context.Context, deploymentID string) error {
	o.mu.Lock()
	state, exists := o.deployments[deploymentID]
	o.mu.Unlock()

	if !exists {
		// Just update status if not running
		return o.updateDeploymentStatus(ctx, deploymentID, DeploymentStatusCancelled)
	}

	// Signal cancellation
	close(state.cancelCh)

	state.mu.RLock()
	campaignID := state.campaignID
	state.mu.RUnlock()

	if campaignID != "" {
		err := o.updateClient.CancelCampaign(ctx, campaignID)
		if err != nil {
			return err
		}
	}

	// Always update status to cancelled
	return o.updateDeploymentStatus(ctx, deploymentID, DeploymentStatusCancelled)
}
