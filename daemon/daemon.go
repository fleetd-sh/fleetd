package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"

	"fleetd.sh/internal/config"
	"github.com/grandcat/zeroconf"
)

type FleetDaemon struct {
	config *Config
	server *zeroconf.Server
	ctx    context.Context
	cancel context.CancelFunc
	port   int
}

func NewFleetDaemon(cfg *Config) (*FleetDaemon, error) {
	// Generate or load device ID
	deviceID := config.GetStringFromEnv("DEVICE_ID", cfg.DeviceID)
	if deviceID == "" {
		// Generate a temporary unique ID
		deviceID = fmt.Sprintf("temp-%s", generateDeviceID())
	}

	// Default port for fleetd discovery service
	port := config.GetIntFromEnv("DISCOVERY_PORT", cfg.DiscoveryPort)
	if port == 0 {
		port = 50055
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &FleetDaemon{
		config: cfg,
		ctx:    ctx,
		cancel: cancel,
		port:   port,
	}, nil
}

func (d *FleetDaemon) Start() error {
	// Get hostname
	hostname, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("failed to get hostname: %v", err)
	}

	// Start mDNS service
	server, err := zeroconf.Register(
		d.config.DeviceID, // Instance name
		"_fleet._tcp",     // Service type
		"local.",          // Domain
		d.port,            // Port
		[]string{ // TXT records
			fmt.Sprintf("device_id=%s", d.config.DeviceID),
			fmt.Sprintf("hostname=%s", hostname),
			fmt.Sprintf("port=%d", d.port),
		},
		nil, // Interfaces to advertise on (nil = all)
	)
	if err != nil {
		return fmt.Errorf("failed to register mDNS service: %v", err)
	}
	d.server = server

	slog.Info("Started mDNS advertisement",
		"device_id", d.config.DeviceID,
		"port", d.port,
		"hostname", hostname)

	// Keep running until context is cancelled
	<-d.ctx.Done()
	return nil
}

func (d *FleetDaemon) Stop() {
	if d.server != nil {
		d.server.Shutdown()
	}
	d.cancel()
}

func (d *FleetDaemon) IsConfigured() bool {
	return d.config != nil && d.config.DeviceID != "" && d.config.APIKey != "" && d.config.APIEndpoint != ""
}

func (d *FleetDaemon) GetDeviceID() string {
	return d.config.DeviceID
}

func (d *FleetDaemon) GetConfig() *Config {
	return d.config
}

// Helper function to generate a temporary,unique device ID
func generateDeviceID() string {
	// For now, use the MAC address of the first non-loopback interface
	interfaces, err := net.Interfaces()
	if err != nil {
		return fmt.Sprintf("temp-%d", os.Getpid())
	}

	for _, iface := range interfaces {
		if iface.Flags&net.FlagLoopback == 0 && iface.Flags&net.FlagUp != 0 {
			return iface.HardwareAddr.String()
		}
	}

	return fmt.Sprintf("unknown-%d", os.Getpid())
}
