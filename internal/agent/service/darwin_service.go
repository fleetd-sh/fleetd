//go:build darwin

package service

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"text/template"

	"fleetd.sh/internal/agent/device"
	"fleetd.sh/internal/config"
)

const (
	darwinServiceName = "sh.fleetd.agent"
	plistFile         = "sh.fleetd.agent.plist"
	launchdDir        = "/Library/LaunchDaemons"
)

// DarwinService implements launchd service management
type DarwinService struct {
	agent  *device.Agent
	ctx    context.Context
	cancel context.CancelFunc
}

// launchd plist template
const plistTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>{{.ServiceName}}</string>
	<key>ProgramArguments</key>
	<array>
		<string>{{.ExecutablePath}}</string>
		<string>--config</string>
		<string>{{.ConfigPath}}</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
	<key>StandardOutPath</key>
	<string>{{.LogDir}}/fleetd.log</string>
	<key>StandardErrorPath</key>
	<string>{{.LogDir}}/fleetd.err</string>
	<key>WorkingDirectory</key>
	<string>{{.DataDir}}</string>
	<key>EnvironmentVariables</key>
	<dict>
		<key>FLEETD_DATA_DIR</key>
		<string>{{.DataDir}}</string>
	</dict>
</dict>
</plist>`

type plistData struct {
	ServiceName    string
	ExecutablePath string
	ConfigPath     string
	DataDir        string
	LogDir         string
}

// NewDarwinService creates a new Darwin service
func NewDarwinService(cfg *config.AgentConfig) (*DarwinService, error) {
	agent, err := device.NewAgent(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create agent: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &DarwinService{
		agent:  agent,
		ctx:    ctx,
		cancel: cancel,
	}, nil
}

// Run starts the service and handles signals
func (s *DarwinService) Run() error {
	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	// Start agent in goroutine
	agentErrChan := make(chan error, 1)
	go func() {
		agentErrChan <- s.agent.Start(s.ctx)
	}()

	// Wait for termination signal or agent error
	select {
	case sig := <-sigChan:
		log.Printf("Received signal: %v, shutting down gracefully", sig)
		s.cancel()

		// Give agent time to clean up
		if err := s.agent.Cleanup(); err != nil {
			log.Printf("Agent cleanup error: %v", err)
		}

		return nil

	case err := <-agentErrChan:
		if err != nil && s.ctx.Err() == nil {
			return fmt.Errorf("agent failed: %w", err)
		}
		return nil
	}
}

// InstallService installs the launchd service
func InstallService(configPath string) error {
	// Get executable path
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	// Resolve symlinks
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return fmt.Errorf("failed to resolve executable path: %w", err)
	}

	// Get config and data directories
	dataDir := GetDefaultDataDir()
	logDir := GetDefaultLogDir()

	// Ensure directories exist
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	// Create launchd plist file
	plistPath := filepath.Join(launchdDir, plistFile)

	// Check if service already exists
	if _, err := os.Stat(plistPath); err == nil {
		return fmt.Errorf("service %s already exists at %s", darwinServiceName, plistPath)
	}

	// Parse template
	tmpl, err := template.New("plist").Parse(plistTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse plist template: %w", err)
	}

	// Create plist file
	file, err := os.Create(plistPath)
	if err != nil {
		return fmt.Errorf("failed to create plist file: %w", err)
	}
	defer file.Close()

	// Execute template
	data := plistData{
		ServiceName:    darwinServiceName,
		ExecutablePath: exePath,
		ConfigPath:     configPath,
		DataDir:        dataDir,
		LogDir:         logDir,
	}

	err = tmpl.Execute(file, data)
	if err != nil {
		os.Remove(plistPath)
		return fmt.Errorf("failed to write plist file: %w", err)
	}

	// Set proper permissions
	err = os.Chmod(plistPath, 0644)
	if err != nil {
		log.Printf("Warning: failed to set plist file permissions: %v", err)
	}

	log.Printf("Service %s installed successfully at %s", darwinServiceName, plistPath)
	log.Printf("Use 'sudo launchctl load %s' to start the service", plistPath)

	return nil
}

// UninstallService uninstalls the launchd service
func UninstallService() error {
	plistPath := filepath.Join(launchdDir, plistFile)

	// Stop service first if running
	if err := StopService(); err != nil {
		log.Printf("Warning: failed to stop service: %v", err)
	}

	// Unload service
	cmd := exec.Command("launchctl", "unload", plistPath)
	if err := cmd.Run(); err != nil {
		log.Printf("Warning: failed to unload service: %v", err)
	}

	// Remove plist file
	if err := os.Remove(plistPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("service %s is not installed", darwinServiceName)
		}
		return fmt.Errorf("failed to remove plist file: %w", err)
	}

	log.Printf("Service %s uninstalled successfully", darwinServiceName)
	return nil
}

// StartService starts the launchd service
func StartService() error {
	plistPath := filepath.Join(launchdDir, plistFile)

	cmd := exec.Command("launchctl", "load", plistPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to start service: %w (output: %s)", err, string(output))
	}

	log.Printf("Service %s started successfully", darwinServiceName)
	return nil
}

// StopService stops the launchd service
func StopService() error {
	plistPath := filepath.Join(launchdDir, plistFile)

	cmd := exec.Command("launchctl", "unload", plistPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to stop service: %w (output: %s)", err, string(output))
	}

	log.Printf("Service %s stopped successfully", darwinServiceName)
	return nil
}

// RestartService restarts the launchd service
func RestartService() error {
	if err := StopService(); err != nil {
		return err
	}
	return StartService()
}

// EnableService enables the launchd service to start at boot
func EnableService() error {
	// On macOS, loading the service with RunAtLoad=true enables it
	return StartService()
}

// DisableService disables the launchd service from starting at boot
func DisableService() error {
	// On macOS, unloading the service disables it
	return StopService()
}

// QueryService queries the service status
func QueryService() (map[string]interface{}, error) {
	cmd := exec.Command("launchctl", "list", darwinServiceName)
	output, err := cmd.CombinedOutput()

	result := make(map[string]interface{})

	if err != nil {
		result["running"] = false
		result["status"] = "not running"
		return result, nil
	}

	result["running"] = true
	result["status"] = "running"
	result["output"] = string(output)

	return result, nil
}

// RunService runs the service
func RunService(cfg *config.AgentConfig) error {
	svc, err := NewDarwinService(cfg)
	if err != nil {
		return err
	}
	return svc.Run()
}

// GetDefaultConfigPath returns the default config path for Darwin
func GetDefaultConfigPath() string {
	return "/etc/fleetd/agent.yaml"
}

// GetDefaultDataDir returns the default data directory for Darwin
func GetDefaultDataDir() string {
	return "/var/lib/fleetd"
}

// GetDefaultLogDir returns the default log directory for Darwin
func GetDefaultLogDir() string {
	return "/var/log/fleetd"
}
