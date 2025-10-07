package fleet

import (
	"encoding/json"
	"fmt"
	"time"
)

// Deployment represents a fleet-wide deployment
type Deployment struct {
	ID        string             `json:"id" db:"id"`
	Name      string             `json:"name" db:"name"`
	Namespace string             `json:"namespace" db:"namespace"`
	Manifest  json.RawMessage    `json:"manifest" db:"manifest"`
	Status    DeploymentStatus   `json:"status" db:"status"`
	Strategy  DeploymentStrategy `json:"strategy" db:"strategy"`
	Selector  map[string]string  `json:"selector" db:"selector"`
	CreatedBy string             `json:"created_by" db:"created_by"`
	CreatedAt time.Time          `json:"created_at" db:"created_at"`
	UpdatedAt time.Time          `json:"updated_at" db:"updated_at"`

	// Computed fields
	Progress    *DeploymentProgress `json:"progress,omitempty" db:"-"`
	DeviceCount int                 `json:"device_count" db:"-"`
}

// DeploymentStatus represents the current state of a deployment
type DeploymentStatus string

const (
	DeploymentStatusPending     DeploymentStatus = "pending"
	DeploymentStatusRunning     DeploymentStatus = "running"
	DeploymentStatusPaused      DeploymentStatus = "paused"
	DeploymentStatusSucceeded   DeploymentStatus = "succeeded"
	DeploymentStatusFailed      DeploymentStatus = "failed"
	DeploymentStatusRollingBack DeploymentStatus = "rolling_back"
	DeploymentStatusCancelled   DeploymentStatus = "cancelled"
)

// DeploymentStrategy defines how updates are rolled out
type DeploymentStrategy struct {
	Type          string         `json:"type"`
	RollingUpdate *RollingUpdate `json:"rolling_update,omitempty"`
	Canary        *Canary        `json:"canary,omitempty"`
	BlueGreen     *BlueGreen     `json:"blue_green,omitempty"`
}

// RollingUpdate configuration
type RollingUpdate struct {
	MaxUnavailable string        `json:"max_unavailable"` // percentage or absolute number
	MaxSurge       string        `json:"max_surge"`       // percentage or absolute number
	WaitTime       time.Duration `json:"wait_time"`
	HealthTimeout  time.Duration `json:"health_timeout"`
}

// Canary deployment configuration
type Canary struct {
	Steps           []CanaryStep `json:"steps"`
	Analysis        *Analysis    `json:"analysis,omitempty"`
	RequireApproval bool         `json:"require_approval,omitempty"`
}

type CanaryStep struct {
	Weight   int           `json:"weight"` // percentage of devices
	Duration time.Duration `json:"duration"`
}

type Analysis struct {
	Metrics   []string `json:"metrics"`
	Threshold float64  `json:"threshold"`
}

// BlueGreen deployment configuration
type BlueGreen struct {
	AutoPromote    bool          `json:"auto_promote"`
	PromoteTimeout time.Duration `json:"promote_timeout"`
	ScaleDownDelay time.Duration `json:"scale_down_delay"`
}

// DeploymentProgress tracks rollout progress
type DeploymentProgress struct {
	Total      int     `json:"total"`
	Pending    int     `json:"pending"`
	Running    int     `json:"running"`
	Succeeded  int     `json:"succeeded"`
	Failed     int     `json:"failed"`
	Percentage float64 `json:"percentage"`
}

// DeviceDeployment tracks deployment status per device
type DeviceDeployment struct {
	DeviceID     string     `json:"device_id" db:"device_id"`
	DeploymentID string     `json:"deployment_id" db:"deployment_id"`
	Status       string     `json:"status" db:"status"`
	Progress     int        `json:"progress" db:"progress"`
	Message      string     `json:"message" db:"message"`
	StartedAt    *time.Time `json:"started_at" db:"started_at"`
	CompletedAt  *time.Time `json:"completed_at" db:"completed_at"`
}

// Validate checks if the deployment configuration is valid
func (d *Deployment) Validate() error {
	if d.Name == "" {
		return fmt.Errorf("deployment name is required")
	}
	if d.Namespace == "" {
		d.Namespace = "default"
	}
	if len(d.Selector) == 0 {
		return fmt.Errorf("device selector is required")
	}
	return nil
}

// IsTerminal returns true if the deployment is in a terminal state
func (s DeploymentStatus) IsTerminal() bool {
	return s == DeploymentStatusSucceeded ||
		s == DeploymentStatusFailed ||
		s == DeploymentStatusCancelled
}

// CanTransitionTo checks if a status transition is valid
func (s DeploymentStatus) CanTransitionTo(target DeploymentStatus) bool {
	if s == target {
		return true
	}

	switch s {
	case DeploymentStatusPending:
		return target == DeploymentStatusRunning || target == DeploymentStatusCancelled
	case DeploymentStatusRunning:
		return target == DeploymentStatusPaused ||
			target == DeploymentStatusSucceeded ||
			target == DeploymentStatusFailed ||
			target == DeploymentStatusRollingBack ||
			target == DeploymentStatusCancelled
	case DeploymentStatusPaused:
		return target == DeploymentStatusRunning || target == DeploymentStatusCancelled
	case DeploymentStatusRollingBack:
		return target == DeploymentStatusSucceeded || target == DeploymentStatusFailed
	case DeploymentStatusFailed:
		return target == DeploymentStatusPending // Allow retry
	case DeploymentStatusCancelled:
		return target == DeploymentStatusPending // Allow retry
	default:
		return false // Other terminal states can't transition
	}
}
