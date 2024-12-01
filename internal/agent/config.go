package agent

import (
	"flag"

	"github.com/google/uuid"
)

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

	// DisableMDNS disables mDNS discovery
	DisableMDNS bool
}

const (
	DefaultMDNSPort = 5353
)

// DefaultConfig returns a Config with default values
func DefaultConfig() *Config {
	return &Config{
		DeviceID:            uuid.New().String(),
		LocalStoragePath:    "/var/lib/fleetd",
		ServerURL:           ":8080",
		EnableMDNS:          true,
		MDNSPort:            5353,
		TelemetryInterval:   60,
		UpdateCheckInterval: 24,
		DisableMDNS:         false,
	}
}

// ParseFlags parses command line flags into config
func ParseFlags() *Config {
	cfg := DefaultConfig()

	flag.BoolVar(&cfg.DisableMDNS, "disable-mdns", false, "Disable mDNS discovery")

	flag.Parse()
	return cfg
}
