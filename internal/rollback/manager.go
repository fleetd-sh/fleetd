package rollback

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"fleetd.sh/internal/fleet"
	"fleetd.sh/internal/health"
	"log/slog"
)

// Manager handles deployment rollbacks
type Manager struct {
	db            *sql.DB
	healthChecker *health.Checker
	orchestrator  fleet.UpdateClient
	policies      map[string]*RollbackPolicy
	mu            sync.RWMutex
	snapshots     map[string]*Snapshot
}

// RollbackPolicy defines when and how to rollback
type RollbackPolicy struct {
	DeploymentID       string        `json:"deployment_id"`
	AutoRollback       bool          `json:"auto_rollback"`
	HealthCheckTimeout time.Duration `json:"health_check_timeout"`
	MaxErrorRate       float64       `json:"max_error_rate"`
	MaxFailedDevices   int           `json:"max_failed_devices"`
	MinSuccessRate     float64       `json:"min_success_rate"`
	CooldownPeriod     time.Duration `json:"cooldown_period"`
	MaxRetries         int           `json:"max_retries"`
}

// Snapshot represents a pre-deployment state snapshot
type Snapshot struct {
	DeploymentID string                 `json:"deployment_id"`
	Timestamp    time.Time              `json:"timestamp"`
	Devices      []DeviceSnapshot       `json:"devices"`
	Metadata     map[string]interface{} `json:"metadata"`
}

// DeviceSnapshot captures device state before update
type DeviceSnapshot struct {
	DeviceID      string                 `json:"device_id"`
	Version       string                 `json:"version"`
	Configuration map[string]interface{} `json:"configuration"`
	Status        string                 `json:"status"`
	LastHealthy   time.Time              `json:"last_healthy"`
}

// RollbackResult represents the outcome of a rollback
type RollbackResult struct {
	Success    bool      `json:"success"`
	RolledBack int       `json:"rolled_back"`
	Failed     int       `json:"failed"`
	Skipped    int       `json:"skipped"`
	StartTime  time.Time `json:"start_time"`
	EndTime    time.Time `json:"end_time"`
	Errors     []string  `json:"errors"`
}

// TriggerReason represents why a rollback was triggered
type TriggerReason struct {
	Type        string                 `json:"type"` // health_check, error_rate, manual, etc.
	Description string                 `json:"description"`
	Metrics     map[string]interface{} `json:"metrics"`
	Timestamp   time.Time              `json:"timestamp"`
}

// NewManager creates a new rollback manager
func NewManager(db *sql.DB, healthChecker *health.Checker, orchestrator fleet.UpdateClient) *Manager {
	return &Manager{
		db:            db,
		healthChecker: healthChecker,
		orchestrator:  orchestrator,
		policies:      make(map[string]*RollbackPolicy),
		snapshots:     make(map[string]*Snapshot),
	}
}

// SetPolicy sets the rollback policy for a deployment
func (m *Manager) SetPolicy(deploymentID string, policy *RollbackPolicy) {
	m.mu.Lock()
	defer m.mu.Unlock()

	policy.DeploymentID = deploymentID
	m.policies[deploymentID] = policy

	// Store in database
	policyJSON, _ := json.Marshal(policy)
	m.db.Exec(`
		INSERT INTO rollback_policies (deployment_id, policy, created_at)
		VALUES (?, ?, ?)
		ON CONFLICT(deployment_id) DO UPDATE SET policy = ?, updated_at = ?`,
		deploymentID, policyJSON, time.Now(), policyJSON, time.Now())
}

// CreateSnapshot creates a pre-deployment snapshot
func (m *Manager) CreateSnapshot(ctx context.Context, deploymentID string) error {
	slog.Info("Creating deployment snapshot", "deployment_id", deploymentID)

	// Get target devices
	rows, err := m.db.QueryContext(ctx, `
		SELECT dd.device_id, d.current_version, d.configuration, d.status
		FROM device_deployment dd
		JOIN device d ON dd.device_id = d.id
		WHERE dd.deployment_id = ?`,
		deploymentID)
	if err != nil {
		return fmt.Errorf("failed to query devices: %w", err)
	}
	defer rows.Close()

	snapshot := &Snapshot{
		DeploymentID: deploymentID,
		Timestamp:    time.Now(),
		Devices:      []DeviceSnapshot{},
		Metadata: map[string]interface{}{
			"created_by": "rollback_manager",
		},
	}

	for rows.Next() {
		var ds DeviceSnapshot
		var configJSON sql.NullString

		err := rows.Scan(&ds.DeviceID, &ds.Version, &configJSON, &ds.Status)
		if err != nil {
			continue
		}

		if configJSON.Valid {
			json.Unmarshal([]byte(configJSON.String), &ds.Configuration)
		}
		ds.LastHealthy = time.Now()

		snapshot.Devices = append(snapshot.Devices, ds)
	}

	// Store snapshot
	m.mu.Lock()
	m.snapshots[deploymentID] = snapshot
	m.mu.Unlock()

	// Persist to database
	snapshotJSON, _ := json.Marshal(snapshot)
	_, err = m.db.ExecContext(ctx, `
		INSERT INTO deployment_snapshots (deployment_id, snapshot, created_at)
		VALUES (?, ?, ?)`,
		deploymentID, snapshotJSON, time.Now())

	if err != nil {
		return fmt.Errorf("failed to store snapshot: %w", err)
	}

	slog.Info("Snapshot created", "deployment_id", deploymentID, "devices", len(snapshot.Devices))
	return nil
}

// MonitorDeployment monitors a deployment for rollback conditions
func (m *Manager) MonitorDeployment(ctx context.Context, deploymentID string) {
	m.mu.RLock()
	policy, exists := m.policies[deploymentID]
	m.mu.RUnlock()

	if !exists || !policy.AutoRollback {
		return
	}

	slog.Info("Starting rollback monitoring", "deployment_id", deploymentID)

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	timeout := time.After(policy.HealthCheckTimeout)

	for {
		select {
		case <-ctx.Done():
			return
		case <-timeout:
			slog.Info("Health check timeout reached", "deployment_id", deploymentID)
			m.evaluateRollback(ctx, deploymentID, &TriggerReason{
				Type:        "health_timeout",
				Description: "Health check timeout exceeded",
				Timestamp:   time.Now(),
			})
			return
		case <-ticker.C:
			if m.shouldRollback(ctx, deploymentID, policy) {
				m.evaluateRollback(ctx, deploymentID, &TriggerReason{
					Type:        "policy_violation",
					Description: "Deployment policy thresholds exceeded",
					Timestamp:   time.Now(),
				})
				return
			}
		}
	}
}

// shouldRollback checks if rollback conditions are met
func (m *Manager) shouldRollback(ctx context.Context, deploymentID string, policy *RollbackPolicy) bool {
	// Check error rate
	var errorCount, totalCount int
	err := m.db.QueryRowContext(ctx, `
		SELECT
			COUNT(CASE WHEN status = 'failed' THEN 1 END),
			COUNT(*)
		FROM device_deployment
		WHERE deployment_id = ?`,
		deploymentID).Scan(&errorCount, &totalCount)

	if err == nil && totalCount > 0 {
		errorRate := float64(errorCount) * 100.0 / float64(totalCount)
		if errorRate > policy.MaxErrorRate {
			slog.Warn("Error rate exceeded",
				"deployment_id", deploymentID,
				"error_rate", errorRate,
				"threshold", policy.MaxErrorRate)
			return true
		}

		successRate := float64(totalCount-errorCount) * 100.0 / float64(totalCount)
		if successRate < policy.MinSuccessRate {
			slog.Warn("Success rate below minimum",
				"deployment_id", deploymentID,
				"success_rate", successRate,
				"threshold", policy.MinSuccessRate)
			return true
		}
	}

	// Check failed device count
	if errorCount > policy.MaxFailedDevices {
		slog.Warn("Failed device count exceeded",
			"deployment_id", deploymentID,
			"failed_devices", errorCount,
			"threshold", policy.MaxFailedDevices)
		return true
	}

	// Check health status
	healthReport := m.healthChecker.GetLastReport()
	if healthReport != nil && healthReport.Status == health.StatusUnhealthy {
		slog.Warn("System health check failed", "deployment_id", deploymentID)
		return true
	}

	return false
}

// evaluateRollback decides whether to proceed with rollback
func (m *Manager) evaluateRollback(ctx context.Context, deploymentID string, reason *TriggerReason) {
	slog.Info("Evaluating rollback", "deployment_id", deploymentID, "reason", reason.Type)

	// Check cooldown period
	var lastRollback sql.NullTime
	m.db.QueryRowContext(ctx, `
		SELECT MAX(created_at)
		FROM rollback_history
		WHERE deployment_id = ?`,
		deploymentID).Scan(&lastRollback)

	m.mu.RLock()
	policy := m.policies[deploymentID]
	m.mu.RUnlock()

	if lastRollback.Valid && time.Since(lastRollback.Time) < policy.CooldownPeriod {
		slog.Info("Rollback in cooldown period", "deployment_id", deploymentID)
		return
	}

	// Execute rollback
	result := m.ExecuteRollback(ctx, deploymentID, reason)

	// Record rollback
	m.recordRollback(ctx, deploymentID, reason, result)
}

// ExecuteRollback performs the actual rollback
func (m *Manager) ExecuteRollback(ctx context.Context, deploymentID string, reason *TriggerReason) *RollbackResult {
	slog.Info("Executing rollback", "deployment_id", deploymentID)

	result := &RollbackResult{
		StartTime: time.Now(),
		Success:   true,
	}

	// Get snapshot
	m.mu.RLock()
	snapshot, exists := m.snapshots[deploymentID]
	m.mu.RUnlock()

	if !exists {
		// Try to load from database
		var snapshotJSON string
		err := m.db.QueryRowContext(ctx, `
			SELECT snapshot FROM deployment_snapshots
			WHERE deployment_id = ?
			ORDER BY created_at DESC
			LIMIT 1`,
			deploymentID).Scan(&snapshotJSON)

		if err != nil {
			result.Success = false
			result.Errors = append(result.Errors, "No snapshot available")
			slog.Error("Rollback failed: no snapshot", "deployment_id", deploymentID)
			return result
		}

		snapshot = &Snapshot{}
		json.Unmarshal([]byte(snapshotJSON), snapshot)
	}

	// Pause deployment
	if err := m.orchestrator.PauseCampaign(ctx, deploymentID); err != nil {
		slog.Error("Failed to pause campaign", "error", err)
	}

	// Rollback each device
	var wg sync.WaitGroup
	rollbackChan := make(chan bool, len(snapshot.Devices))

	for _, device := range snapshot.Devices {
		wg.Add(1)
		go func(ds DeviceSnapshot) {
			defer wg.Done()

			if err := m.rollbackDevice(ctx, ds); err != nil {
				slog.Error("Device rollback failed",
					"device_id", ds.DeviceID,
					"error", err)
				rollbackChan <- false
			} else {
				rollbackChan <- true
			}
		}(device)
	}

	// Wait for rollbacks to complete
	go func() {
		wg.Wait()
		close(rollbackChan)
	}()

	// Count results
	for success := range rollbackChan {
		if success {
			result.RolledBack++
		} else {
			result.Failed++
		}
	}

	result.EndTime = time.Now()
	result.Success = result.Failed == 0

	// Update deployment status
	status := "rolled_back"
	if !result.Success {
		status = "rollback_failed"
	}

	m.db.ExecContext(ctx, `
		UPDATE deployment
		SET status = ?, updated_at = ?
		WHERE id = ?`,
		status, time.Now(), deploymentID)

	slog.Info("Rollback completed",
		"deployment_id", deploymentID,
		"rolled_back", result.RolledBack,
		"failed", result.Failed,
		"duration", result.EndTime.Sub(result.StartTime))

	return result
}

// rollbackDevice rolls back a single device
func (m *Manager) rollbackDevice(ctx context.Context, snapshot DeviceSnapshot) error {
	// Update device record
	configJSON, _ := json.Marshal(snapshot.Configuration)
	_, err := m.db.ExecContext(ctx, `
		UPDATE device
		SET current_version = ?,
		    configuration = ?,
		    status = ?,
		    updated_at = ?
		WHERE id = ?`,
		snapshot.Version,
		configJSON,
		snapshot.Status,
		time.Now(),
		snapshot.DeviceID)

	if err != nil {
		return fmt.Errorf("failed to update device record: %w", err)
	}

	// TODO: Send rollback command to actual device
	// This would involve calling the device API to trigger the rollback

	return nil
}

// recordRollback records rollback history
func (m *Manager) recordRollback(ctx context.Context, deploymentID string, reason *TriggerReason, result *RollbackResult) {
	reasonJSON, _ := json.Marshal(reason)
	resultJSON, _ := json.Marshal(result)

	_, err := m.db.ExecContext(ctx, `
		INSERT INTO rollback_history (
			deployment_id, reason, result, created_at
		) VALUES (?, ?, ?, ?)`,
		deploymentID, reasonJSON, resultJSON, time.Now())

	if err != nil {
		slog.Error("Failed to record rollback history", "error", err)
	}
}

// ManualRollback triggers a manual rollback
func (m *Manager) ManualRollback(ctx context.Context, deploymentID string, reason string) (*RollbackResult, error) {
	trigger := &TriggerReason{
		Type:        "manual",
		Description: reason,
		Timestamp:   time.Now(),
		Metrics: map[string]interface{}{
			"triggered_by": "user",
		},
	}

	return m.ExecuteRollback(ctx, deploymentID, trigger), nil
}
