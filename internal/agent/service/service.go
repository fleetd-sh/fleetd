package service

import (
	"context"
	"fmt"
	"runtime"

	"fleetd.sh/internal/agent/device"
	"fleetd.sh/internal/config"
)

// ServiceManager provides cross-platform service management
type ServiceManager interface {
	Install(configPath string) error
	Uninstall() error
	Start() error
	Stop() error
	Restart() error
	Enable() error
	Disable() error
	Query() (map[string]interface{}, error)
	Run(cfg *config.AgentConfig) error
}

// NewServiceManager creates a platform-specific service manager
func NewServiceManager() ServiceManager {
	switch runtime.GOOS {
	case "windows":
		return &WindowsServiceManager{}
	case "darwin":
		return &DarwinServiceManager{}
	case "linux":
		return &LinuxServiceManager{}
	default:
		return &GenericServiceManager{}
	}
}

// WindowsServiceManager implements Windows service management
type WindowsServiceManager struct{}

func (w *WindowsServiceManager) Install(configPath string) error {
	return InstallService(configPath)
}

func (w *WindowsServiceManager) Uninstall() error {
	return UninstallService()
}

func (w *WindowsServiceManager) Start() error {
	return StartService()
}

func (w *WindowsServiceManager) Stop() error {
	return StopService()
}

func (w *WindowsServiceManager) Restart() error {
	if err := w.Stop(); err != nil {
		return err
	}
	return w.Start()
}

func (w *WindowsServiceManager) Enable() error {
	// Windows services are enabled by default when installed
	return nil
}

func (w *WindowsServiceManager) Disable() error {
	// Windows services need to be uninstalled to be disabled
	return fmt.Errorf("use uninstall to disable Windows service")
}

func (w *WindowsServiceManager) Query() (map[string]interface{}, error) {
	return QueryService()
}

func (w *WindowsServiceManager) Run(cfg *config.AgentConfig) error {
	return RunService(cfg)
}

// DarwinServiceManager implements macOS launchd service management
type DarwinServiceManager struct{}

func (d *DarwinServiceManager) Install(configPath string) error {
	return InstallService(configPath)
}

func (d *DarwinServiceManager) Uninstall() error {
	return UninstallService()
}

func (d *DarwinServiceManager) Start() error {
	return StartService()
}

func (d *DarwinServiceManager) Stop() error {
	return StopService()
}

func (d *DarwinServiceManager) Restart() error {
	return RestartService()
}

func (d *DarwinServiceManager) Enable() error {
	return EnableService()
}

func (d *DarwinServiceManager) Disable() error {
	return DisableService()
}

func (d *DarwinServiceManager) Query() (map[string]interface{}, error) {
	return QueryService()
}

func (d *DarwinServiceManager) Run(cfg *config.AgentConfig) error {
	return RunService(cfg)
}

// LinuxServiceManager implements Linux systemd service management
type LinuxServiceManager struct{}

func (l *LinuxServiceManager) Install(configPath string) error {
	return InstallService(configPath)
}

func (l *LinuxServiceManager) Uninstall() error {
	return UninstallService()
}

func (l *LinuxServiceManager) Start() error {
	return StartService()
}

func (l *LinuxServiceManager) Stop() error {
	return StopService()
}

func (l *LinuxServiceManager) Restart() error {
	return RestartService()
}

func (l *LinuxServiceManager) Enable() error {
	return EnableService()
}

func (l *LinuxServiceManager) Disable() error {
	return DisableService()
}

func (l *LinuxServiceManager) Query() (map[string]interface{}, error) {
	return QueryService()
}

func (l *LinuxServiceManager) Run(cfg *config.AgentConfig) error {
	return RunService(cfg)
}

// GenericServiceManager provides basic service management for unsupported platforms
type GenericServiceManager struct{}

func (g *GenericServiceManager) Install(configPath string) error {
	return fmt.Errorf("service installation not supported on %s", runtime.GOOS)
}

func (g *GenericServiceManager) Uninstall() error {
	return fmt.Errorf("service management not supported on %s", runtime.GOOS)
}

func (g *GenericServiceManager) Start() error {
	return fmt.Errorf("service management not supported on %s", runtime.GOOS)
}

func (g *GenericServiceManager) Stop() error {
	return fmt.Errorf("service management not supported on %s", runtime.GOOS)
}

func (g *GenericServiceManager) Restart() error {
	return fmt.Errorf("service management not supported on %s", runtime.GOOS)
}

func (g *GenericServiceManager) Enable() error {
	return fmt.Errorf("service management not supported on %s", runtime.GOOS)
}

func (g *GenericServiceManager) Disable() error {
	return fmt.Errorf("service management not supported on %s", runtime.GOOS)
}

func (g *GenericServiceManager) Query() (map[string]interface{}, error) {
	return nil, fmt.Errorf("service management not supported on %s", runtime.GOOS)
}

func (g *GenericServiceManager) Run(cfg *config.AgentConfig) error {
	// For unsupported platforms, run as a regular process
	agent, err := device.NewAgent(cfg)
	if err != nil {
		return fmt.Errorf("failed to create agent: %w", err)
	}

	ctx := context.Background()
	return agent.Start(ctx)
}

// GetDefaultPaths returns platform-specific default paths
func GetDefaultPaths() (configPath, dataDir, logDir string) {
	switch runtime.GOOS {
	case "windows":
		return GetDefaultConfigPath(), GetDefaultDataDir(), GetDefaultLogDir()
	case "darwin":
		return GetDefaultConfigPath(), GetDefaultDataDir(), GetDefaultLogDir()
	case "linux":
		return GetDefaultConfigPath(), GetDefaultDataDir(), GetDefaultLogDir()
	default:
		return "./agent.yaml", "./data", "./logs"
	}
}
