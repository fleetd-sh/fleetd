package agent

// Config holds the agent configuration
type Config struct {
	// DeviceID is the unique identifier for this device
	DeviceID string

	// ServerURL is the URL of the fleet management server
	ServerURL string

	// LocalStoragePath is the path where the agent can store local data
	LocalStoragePath string

	// EnableMDNS enables mDNS discovery
	EnableMDNS bool

	// Port to use for mDNS discovery
	MDNSPort int

	// TelemetryInterval is the interval between telemetry collections
	TelemetryInterval int

	// UpdateCheckInterval is how often to check for updates (in hours)
	UpdateCheckInterval int
}

const (
	DefaultMDNSPort = 5353
)

// DefaultConfig returns a Config with default values
func DefaultConfig() *Config {
	return &Config{
		EnableMDNS:          true,
		TelemetryInterval:   60, // seconds
		UpdateCheckInterval: 24, // Check daily
		MDNSPort:            DefaultMDNSPort,
	}
}
