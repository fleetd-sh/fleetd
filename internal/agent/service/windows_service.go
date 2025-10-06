//go:build windows

package service

import (
	"fleetd.sh/internal/config"
)

// Windows service status structure (placeholder - needs real Windows implementation)
type WindowsServiceStatus struct {
	State         string
	Accepts       uint32
	ProcessId     uint32
	Win32ExitCode uint32
}

// InstallService installs the Windows service (stub implementation)
func InstallService(configPath string) error {
	// TODO: Implement Windows service installation
	panic("Windows service installation not yet implemented")
}

// UninstallService uninstalls the Windows service (stub implementation)
func UninstallService() error {
	// TODO: Implement Windows service uninstallation
	panic("Windows service uninstallation not yet implemented")
}

// StartService starts the Windows service (stub implementation)
func StartService() error {
	// TODO: Implement Windows service start
	panic("Windows service start not yet implemented")
}

// StopService stops the Windows service (stub implementation)
func StopService() error {
	// TODO: Implement Windows service stop
	panic("Windows service stop not yet implemented")
}

// QueryService queries the Windows service status (stub implementation)
func QueryService() (map[string]interface{}, error) {
	// TODO: Implement Windows service query
	result := make(map[string]interface{})
	result["running"] = false
	result["status"] = "not implemented"
	return result, nil
}

// RunService runs the Windows service (stub implementation)
func RunService(cfg *config.AgentConfig) error {
	// TODO: Implement Windows service run
	panic("Windows service run not yet implemented")
}

// GetDefaultConfigPath returns the default config path for Windows
func GetDefaultConfigPath() string {
	return "C:\\ProgramData\\fleetd\\agent.yaml"
}

// GetDefaultDataDir returns the default data directory for Windows
func GetDefaultDataDir() string {
	return "C:\\ProgramData\\fleetd\\data"
}

// GetDefaultLogDir returns the default log directory for Windows
func GetDefaultLogDir() string {
	return "C:\\ProgramData\\fleetd\\logs"
}
