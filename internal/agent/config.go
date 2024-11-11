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

	// StorageDir is the path where the agent can store local data
	StorageDir string

	// RPCPort is the port to use for the local server
	RPCPort int

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

	// ServiceType is the mDNS service type to use
	ServiceType string
}

const (
	DefaultMDNSPort = 5353
	DefaultRPCPort  = 8080
)

// DefaultConfig returns a Config with default values
func DefaultConfig() *Config {
	return &Config{
		DeviceID:            uuid.New().String(),
		StorageDir:          "/var/lib/fleetd",
		ServerURL:           "http://localhost:8080",
		EnableMDNS:          true,
		MDNSPort:            DefaultMDNSPort,
		RPCPort:             DefaultRPCPort,
		TelemetryInterval:   60,
		UpdateCheckInterval: 24,
		DisableMDNS:         false,
	}
}

// ParseFlags parses command line flags into config
func ParseFlags() *Config {
	cfg := DefaultConfig()

	flag.StringVar(&cfg.StorageDir, "storage-dir", cfg.StorageDir, "Directory for storing agent data")
	flag.StringVar(&cfg.ServerURL, "server-url", cfg.ServerURL, "URL of the fleet management server")
	flag.BoolVar(&cfg.DisableMDNS, "disable-mdns", false, "Disable mDNS discovery")
	flag.IntVar(&cfg.RPCPort, "rpc-port", cfg.RPCPort, "Port to use for the local RPC server")
	flag.Parse()
	return cfg
}
