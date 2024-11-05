package daemon

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type Config struct {
	DeviceID                 string `json:"device_id"`
	DeviceName               string `json:"device_name"`
	DeviceType               string `json:"device_type"`
	APIKey                   string `json:"api_key"`
	APIEndpoint              string `json:"api_endpoint"`
	ContainerImage           string `json:"container_image"`
	ConfigDir                string `json:"config_dir"`
	MetricCollectionInterval string `json:"metric_collection_interval"`
	UpdateCheckInterval      string `json:"update_check_interval"`
	DiscoveryPort            int    `json:"discovery_port"`

	mu sync.RWMutex
}

const (
	defaultConfigFile    = "fleet_config.json"
	defaultConfigDir     = "/etc/fleet"
	defaultDiscoveryPort = 50050
)

func LoadConfig() (*Config, error) {
	configDir := os.Getenv("FLEETD_CONFIG_DIR")
	if configDir == "" {
		configDir = defaultConfigDir
	}

	configPath := filepath.Join(configDir, defaultConfigFile)

	cfg := &Config{
		ConfigDir: configDir,
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return cfg, nil // Return default config if file doesn't exist
	}

	file, err := os.Open(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file: %w", err)
	}
	defer file.Close()

	if err := json.NewDecoder(file).Decode(cfg); err != nil {
		return nil, fmt.Errorf("failed to decode config file: %w", err)
	}

	return cfg, nil
}

func (c *Config) SaveConfig() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := os.MkdirAll(c.ConfigDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	configPath := filepath.Join(c.ConfigDir, defaultConfigFile)
	file, err := os.Create(configPath)
	if err != nil {
		return fmt.Errorf("failed to create config file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ") // Pretty-print the JSON
	if err := encoder.Encode(c); err != nil {
		return fmt.Errorf("failed to encode config: %w", err)
	}

	return nil
}

func (c *Config) IsConfigured() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.APIEndpoint != "" &&
		c.DiscoveryPort != 0
}

func (c *Config) GetAPIEndpoint() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.APIEndpoint
}

func (c *Config) GetDiscoveryPort() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.DiscoveryPort == 0 {
		return defaultDiscoveryPort
	}
	return c.DiscoveryPort
}

func (c *Config) SetAPIEndpoint(url string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.APIEndpoint = url
}

func (c *Config) SetDiscoveryPort(port int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.DiscoveryPort = port
}
