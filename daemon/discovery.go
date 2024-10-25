package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"sync"

	"connectrpc.com/connect"
	"github.com/grandcat/zeroconf"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	discoverypb "fleetd.sh/gen/discovery/v1"
	discoveryrpc "fleetd.sh/gen/discovery/v1/discoveryv1connect"
)

const (
	serviceName = "fleet-device"
	serviceType = "_fleetd._tcp"
	domain      = "local."
)

type DiscoveryService struct {
	config       *Config
	server       *zeroconf.Server
	httpServer   *http.Server
	mu           sync.Mutex
	isConfigured bool
}

func NewDiscoveryService(cfg *Config) *DiscoveryService {
	return &DiscoveryService{
		config:       cfg,
		isConfigured: cfg.IsConfigured(),
	}
}

func (ds *DiscoveryService) StartBroadcasting() error {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	if ds.isConfigured {
		return nil // Already configured, no need to broadcast
	}

	host, _ := os.Hostname()
	port := ds.config.GetDiscoveryPort()
	info := []string{"Fleet Device"}
	var err error
	ds.server, err = zeroconf.Register(host, serviceName, domain, port, info, nil)
	if err != nil {
		return fmt.Errorf("failed to register mDNS service: %w", err)
	}

	// Start HTTP server for configuration
	mux := http.NewServeMux()
	path, handler := discoveryrpc.NewDiscoveryServiceHandler(ds)
	mux.Handle(path, handler)

	ds.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", ds.GetPort()),
		Handler: h2c.NewHandler(mux, &http2.Server{}),
	}

	go func() {
		if err := ds.httpServer.ListenAndServe(); err != http.ErrServerClosed {
			slog.With("error", err).Error("HTTP server error")
		}
	}()

	slog.With("address", fmt.Sprintf(":%d", port)).Info("Discovery service started")
	return nil
}

func (ds *DiscoveryService) StopBroadcasting() {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	if ds.server != nil {
		ds.server.Shutdown()
		ds.server = nil
	}

	if ds.httpServer != nil {
		if err := ds.httpServer.Close(); err != nil {
			slog.With("error", err).Error("Error closing HTTP server")
		}
		ds.httpServer = nil
	}
}

func (ds *DiscoveryService) ConfigureDevice(
	ctx context.Context,
	req *connect.Request[discoverypb.ConfigureDeviceRequest],
) (*connect.Response[discoverypb.ConfigureDeviceResponse], error) {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	// Update the configuration
	ds.config.DeviceID = req.Msg.DeviceId
	ds.config.FleetAPIURL = req.Msg.FleetApiUrl
	ds.config.UpdateServerURL = req.Msg.UpdateServerUrl
	ds.config.MetricsServerURL = req.Msg.MetricsServerUrl

	// Save the new configuration
	if err := ds.config.Save(); err != nil {
		return connect.NewResponse(&discoverypb.ConfigureDeviceResponse{
			Success: false,
			Message: fmt.Sprintf("Failed to save configuration: %v", err),
		}), nil
	}

	ds.isConfigured = true

	// Stop broadcasting as we're now configured
	ds.StopBroadcasting()

	return connect.NewResponse(&discoverypb.ConfigureDeviceResponse{
		Success: true,
		Message: "Device configured successfully",
	}), nil
}

func (ds *DiscoveryService) GetIPAddress() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", err
	}
	for _, address := range addrs {
		// Check the address type and if it is not a loopback then display it
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String(), nil
			}
		}
	}
	return "", fmt.Errorf("no suitable IP address found")
}

func (ds *DiscoveryService) GetPort() int {
	return ds.config.GetDiscoveryPort()
}
