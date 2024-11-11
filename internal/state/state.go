package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// State represents the agent's persistent state
type State struct {
	// Version of the state schema
	Version int `json:"version"`

	// LastStartTime is when the agent last started
	LastStartTime time.Time `json:"lastStartTime"`

	// DeviceInfo contains static device information
	DeviceInfo DeviceInfo `json:"deviceInfo"`

	// RuntimeState contains dynamic state information
	RuntimeState RuntimeState `json:"runtime_state"`

	// UpdateHistory tracks past updates
	UpdateHistory []UpdateRecord `json:"updateHistory"`
}

type DeviceInfo struct {
	ID            string            `json:"id"`
	Hardware      string            `json:"hardware"`
	Architecture  string            `json:"architecture"`
	OSInfo        string            `json:"osInfo"`
	Capabilities  []string          `json:"capabilities"`
	Tags          map[string]string `json:"tags"`
	FirstSeenTime time.Time         `json:"firstSeenTime"`
}

type RuntimeState struct {
	DeployedBinaries map[string]BinaryInfo `json:"deployed_binaries"`
	LastHealthCheck  time.Time             `json:"lastHealthCheck"`
	Status           string                `json:"status"`
}

type BinaryInfo struct {
	Name        string    `json:"name"`
	Version     string    `json:"version"`
	DeployedAt  time.Time `json:"deployedAt"`
	LastStarted time.Time `json:"lastStarted"`
	Status      string    `json:"status"`
}

type UpdateRecord struct {
	Version     string    `json:"version"`
	UpdatedAt   time.Time `json:"updatedAt"`
	Success     bool      `json:"success"`
	ErrorDetail string    `json:"errorDetail,omitempty"`
}

// Manager handles persistent state storage
type Manager struct {
	path       string
	backupPath string
	state      *State
	mu         sync.RWMutex
}

// New creates a new state manager
func New(path string) (*Manager, error) {
	// Ensure the directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create state directory: %w", err)
	}

	m := &Manager{
		path:       path,
		backupPath: path + ".bak",
		state: &State{
			RuntimeState: RuntimeState{
				DeployedBinaries: make(map[string]BinaryInfo),
			},
		},
	}

	// Try to load existing state
	if err := m.load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load state: %w", err)
	}

	return m, nil
}

// Get returns a copy of the current state
func (m *Manager) Get() State {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return a deep copy to prevent external modifications
	stateCopy := *m.state
	return stateCopy
}

// Update atomically updates the state
func (m *Manager) Update(fn func(*State) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := fn(m.state); err != nil {
		return err
	}

	return m.save()
}

func (m *Manager) load() error {
	data, err := os.ReadFile(m.path)
	if err != nil {
		return err
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("failed to parse state: %w", err)
	}

	m.state = &state
	return nil
}

// Save persists the current state to disk
func (m *Manager) Save() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.save()
}

// save is the internal implementation that assumes the lock is held
func (m *Manager) save() error {
	// No need to lock here as the caller must hold the lock

	// Ensure directory exists
	dir := filepath.Dir(m.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	// Create backup of existing state file if it exists
	if err := m.backup(); err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}

	data, err := json.MarshalIndent(m.state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	// Write to temporary file first
	tmpPath := m.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write state: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, m.path); err != nil {
		os.Remove(tmpPath) // Clean up temp file
		return fmt.Errorf("failed to save state: %w", err)
	}

	return nil
}

func (m *Manager) backup() error {
	if _, err := os.Stat(m.path); err == nil {
		if err := os.Rename(m.path, m.backupPath); err != nil {
			return fmt.Errorf("failed to create backup: %w", err)
		}
	}
	return nil
}

func (m *Manager) recoverFromBackup() error {
	if _, err := os.Stat(m.backupPath); err != nil {
		return fmt.Errorf("no backup available: %w", err)
	}

	if err := os.Rename(m.backupPath, m.path); err != nil {
		return fmt.Errorf("failed to restore backup: %w", err)
	}

	return m.load()
}
