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
	FleetAPIURL              string `json:"fleet_api_url"`
	ClientID                 string `json:"client_id"`
	ClientSecret             string `json:"client_secret"`
	CertificatePath          string `json:"certificate_path"`
	PrivateKeyPath           string `json:"private_key_path"`
	UpdateServerURL          string `json:"update_server_url"`
	MetricsServerURL         string `json:"metrics_server_url"`
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

func Load() (*Config, error) {
	configDir := os.Getenv("FLEET_CONFIG_DIR")
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

func (c *Config) Save() error {
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

	return c.DeviceID != "" &&
		c.FleetAPIURL != "" &&
		c.ClientID != "" &&
		c.ClientSecret != "" &&
		c.DiscoveryPort != 0
}

// Getter methods
func (c *Config) GetDeviceID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.DeviceID
}

func (c *Config) GetFleetAPIURL() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.FleetAPIURL
}

func (c *Config) GetClientID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ClientID
}

func (c *Config) GetClientSecret() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ClientSecret
}

func (c *Config) GetDiscoveryPort() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.DiscoveryPort == 0 {
		return defaultDiscoveryPort
	}
	return c.DiscoveryPort
}

// Setter methods
func (c *Config) SetDeviceID(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.DeviceID = id
}

func (c *Config) SetFleetAPIURL(url string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.FleetAPIURL = url
}

func (c *Config) SetClientCredentials(clientID, clientSecret string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ClientID = clientID
	c.ClientSecret = clientSecret
}

func (c *Config) SetDiscoveryPort(port int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.DiscoveryPort = port
}

// Methods for certificate and key management
func (c *Config) SavePrivateKey(keyPEM []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.PrivateKeyPath == "" {
		c.PrivateKeyPath = filepath.Join(c.ConfigDir, "device.key")
	}

	return os.WriteFile(c.PrivateKeyPath, keyPEM, 0600)
}

func (c *Config) SaveCertificate(certPEM []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.CertificatePath == "" {
		c.CertificatePath = filepath.Join(c.ConfigDir, "device.crt")
	}

	return os.WriteFile(c.CertificatePath, certPEM, 0644)
}

func (c *Config) LoadPrivateKey() ([]byte, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return os.ReadFile(c.PrivateKeyPath)
}

func (c *Config) LoadCertificate() ([]byte, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return os.ReadFile(c.CertificatePath)
}
