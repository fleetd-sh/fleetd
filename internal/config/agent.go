package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"gopkg.in/yaml.v3"
)

// AgentConfig holds configuration for the device agent
type AgentConfig struct {
	// Server configuration
	ServerURL string `json:"server_url" yaml:"server_url"`
	APIKey    string `json:"api_key" yaml:"api_key"`
	AuthToken string `json:"auth_token" yaml:"auth_token"`

	// Device identity
	DeviceID       string            `json:"device_id" yaml:"device_id"`
	CurrentVersion string            `json:"current_version" yaml:"current_version"`
	Labels         map[string]string `json:"labels" yaml:"labels"`
	Capabilities   []string          `json:"capabilities" yaml:"capabilities"`

	// Intervals
	HeartbeatInterval   time.Duration `json:"heartbeat_interval" yaml:"heartbeat_interval"`
	UpdateCheckInterval time.Duration `json:"update_check_interval" yaml:"update_check_interval"`
	MetricsInterval     time.Duration `json:"metrics_interval" yaml:"metrics_interval"`

	// Storage
	DataDir   string `json:"data_dir" yaml:"data_dir"`
	LogDir    string `json:"log_dir" yaml:"log_dir"`
	BackupDir string `json:"backup_dir" yaml:"backup_dir"`

	// Security
	TLSVerify bool   `json:"tls_verify" yaml:"tls_verify"`
	TLSCert   string `json:"tls_cert" yaml:"tls_cert"`
	TLSKey    string `json:"tls_key" yaml:"tls_key"`
	TLSCACert string `json:"tls_ca_cert" yaml:"tls_ca_cert"`
	PublicKey string `json:"public_key" yaml:"public_key"`

	// Behavior
	AutoUpdate          bool          `json:"auto_update" yaml:"auto_update"`
	AutoRollback        bool          `json:"auto_rollback" yaml:"auto_rollback"`
	MaxRetries          int           `json:"max_retries" yaml:"max_retries"`
	RetryBackoff        time.Duration `json:"retry_backoff" yaml:"retry_backoff"`
	OfflineBufferSize   int           `json:"offline_buffer_size" yaml:"offline_buffer_size"`
	HealthCheckInterval time.Duration `json:"health_check_interval" yaml:"health_check_interval"`

	// Debug
	Debug    bool   `json:"debug" yaml:"debug"`
	LogLevel string `json:"log_level" yaml:"log_level"`
}

// DefaultAgentConfig returns default agent configuration with platform-specific paths
func DefaultAgentConfig() *AgentConfig {
	dataDir, logDir, backupDir := getPlatformDefaultPaths()

	return &AgentConfig{
		HeartbeatInterval:   30 * time.Second,
		UpdateCheckInterval: 5 * time.Minute,
		MetricsInterval:     1 * time.Minute,
		HealthCheckInterval: 30 * time.Second,
		DataDir:             dataDir,
		LogDir:              logDir,
		BackupDir:           backupDir,
		TLSVerify:           true,
		AutoUpdate:          true,
		AutoRollback:        true,
		MaxRetries:          3,
		RetryBackoff:        10 * time.Second,
		OfflineBufferSize:   1000,
		LogLevel:            "info",
		Labels:              make(map[string]string),
		Capabilities:        []string{"update", "metrics", "logs"},
	}
}

// LoadAgentConfig loads agent configuration from a file
func LoadAgentConfig(path string) (*AgentConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := DefaultAgentConfig()

	// Try YAML first
	if err := yaml.Unmarshal(data, cfg); err != nil {
		// Try JSON
		if err := json.Unmarshal(data, cfg); err != nil {
			return nil, err
		}
	}

	// Apply environment variable overrides
	cfg.applyEnvOverrides()

	return cfg, nil
}

// applyEnvOverrides applies environment variable overrides
func (c *AgentConfig) applyEnvOverrides() {
	if val := os.Getenv("FLEETD_SERVER_URL"); val != "" {
		c.ServerURL = val
	}
	if val := os.Getenv("FLEETD_API_KEY"); val != "" {
		c.APIKey = val
	}
	if val := os.Getenv("FLEETD_DEVICE_ID"); val != "" {
		c.DeviceID = val
	}
	if val := os.Getenv("FLEETD_DATA_DIR"); val != "" {
		c.DataDir = val
	}
	if val := os.Getenv("FLEETD_DEBUG"); val == "true" {
		c.Debug = true
		c.LogLevel = "debug"
	}
}

// SaveAgentConfig saves agent configuration to a file
func SaveAgentConfig(cfg *AgentConfig, path string) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}

// RestoreState restores saved state from persisted data
func (c *AgentConfig) RestoreState(data []byte) error {
	var state struct {
		DeviceID string `json:"device_id"`
		Version  string `json:"version"`
	}

	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}

	if state.DeviceID != "" {
		c.DeviceID = state.DeviceID
	}
	if state.Version != "" {
		c.CurrentVersion = state.Version
	}

	return nil
}

// getPlatformDefaultPaths returns platform-specific default paths
func getPlatformDefaultPaths() (dataDir, logDir, backupDir string) {
	switch runtime.GOOS {
	case "windows":
		programData := os.Getenv("PROGRAMDATA")
		if programData == "" {
			programData = `C:\ProgramData`
		}
		dataDir = filepath.Join(programData, "fleetd", "data")
		logDir = filepath.Join(programData, "fleetd", "logs")
		backupDir = filepath.Join(programData, "fleetd", "backup")

	case "darwin":
		dataDir = "/var/lib/fleetd"
		logDir = "/var/log/fleetd"
		backupDir = "/var/lib/fleetd/backup"

	default: // Linux and other Unix-like systems
		dataDir = "/var/lib/fleetd"
		logDir = "/var/log/fleetd"
		backupDir = "/var/lib/fleetd/backup"
	}

	return dataDir, logDir, backupDir
}

// GetDefaultConfigPath returns the default configuration file path for the current platform
func GetDefaultConfigPath() string {
	switch runtime.GOOS {
	case "windows":
		programData := os.Getenv("PROGRAMDATA")
		if programData == "" {
			programData = `C:\ProgramData`
		}
		return filepath.Join(programData, "fleetd", "agent.yaml")

	case "darwin":
		return "/etc/fleetd/agent.yaml"

	default: // Linux and other Unix-like systems
		return "/etc/fleetd/agent.yaml"
	}
}
