package daemon

import (
	"fmt"
	"log/slog"
)

type FleetDaemon struct {
	config           *Config
	containerMgr     *ContainerManager
	metricCollector  *MetricCollector
	updateManager    *UpdateManager
	discoveryService *DiscoveryService
}

func NewFleetDaemon() (*FleetDaemon, error) {
	cfg, err := Load()
	if err != nil {
		return nil, err
	}

	cm, err := NewContainerManager(cfg)
	if err != nil {
		return nil, err
	}

	mc, err := NewMetricCollector(cfg)
	if err != nil {
		return nil, err
	}

	um, err := NewUpdateManager(cfg)
	if err != nil {
		return nil, err
	}

	ds := NewDiscoveryService(cfg)

	return &FleetDaemon{
		config:           cfg,
		containerMgr:     cm,
		metricCollector:  mc,
		updateManager:    um,
		discoveryService: ds,
	}, nil
}

func (fd *FleetDaemon) Start() error {
	if !fd.config.IsConfigured() {
		slog.Info("Device not configured. Starting discovery service...")
		if err := fd.discoveryService.StartBroadcasting(); err != nil {
			return fmt.Errorf("failed to start discovery service: %v", err)
		}
		ip, err := fd.discoveryService.GetIPAddress()
		port := fd.discoveryService.GetPort()
		if err == nil {
			slog.Info("Device can be configured", "url", fmt.Sprintf("http://%s:%d/configure", ip, port))
		}
	} else {
		slog.Info("Device configured. Starting normal operations...")
		go fd.containerMgr.Start()
		go fd.metricCollector.Start()
		go fd.updateManager.Start()
	}

	return nil
}

func (fd *FleetDaemon) Stop() {
	fd.containerMgr.Stop()
	fd.metricCollector.Stop()
	fd.updateManager.Stop()
	fd.discoveryService.StopBroadcasting()
}
