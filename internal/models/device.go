package models

import (
	"time"
)

// Device represents a managed device in the fleet
type Device struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Type      string         `json:"type"`
	Version   string         `json:"version"`
	APIKey    string         `json:"-"` // Never expose in JSON
	Metadata  map[string]any `json:"metadata,omitempty"`
	LastSeen  time.Time      `json:"last_seen"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`

	// Computed fields
	Status  DeviceStatus `json:"status"`
	Tags    []string     `json:"tags,omitempty"`
	GroupID string       `json:"group_id,omitempty"`
}

// DeviceStatus represents the current status of a device
type DeviceStatus string

const (
	DeviceStatusOnline      DeviceStatus = "online"
	DeviceStatusOffline     DeviceStatus = "offline"
	DeviceStatusUpdating    DeviceStatus = "updating"
	DeviceStatusError       DeviceStatus = "error"
	DeviceStatusMaintenance DeviceStatus = "maintenance"
)

// IsOnline returns true if the device is considered online
func (d *Device) IsOnline(threshold time.Duration) bool {
	return time.Since(d.LastSeen) < threshold
}

// GetStatus computes the device status based on last seen time
func (d *Device) GetStatus(onlineThreshold time.Duration) DeviceStatus {
	if d.IsOnline(onlineThreshold) {
		return DeviceStatusOnline
	}
	return DeviceStatusOffline
}

// Validate checks if the device data is valid
func (d *Device) Validate() error {
	if d.ID == "" {
		return ErrInvalidDevice("device ID is required")
	}
	if d.Name == "" {
		return ErrInvalidDevice("device name is required")
	}
	if d.Type == "" {
		return ErrInvalidDevice("device type is required")
	}
	if d.Version == "" {
		return ErrInvalidDevice("device version is required")
	}
	return nil
}

// ErrInvalidDevice represents a device validation error
type ErrInvalidDevice string

func (e ErrInvalidDevice) Error() string {
	return string(e)
}
