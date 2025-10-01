//go:build linux

package service

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"text/template"

	"fleetd.sh/internal/agent/device"
	"fleetd.sh/internal/config"
)

const (
	serviceName = "fleetd"
	unitFile    = "fleetd.service"
	systemdDir  = "/etc/systemd/system"
)

// LinuxService implements systemd service management
type LinuxService struct {
	agent  *device.Agent
	ctx    context.Context
	cancel context.CancelFunc
}

// systemd unit template
const unitTemplate = `[Unit]
Description=Fleet Device Agent
Documentation=https://github.com/fleetd-sh/fleetd
After=network.target
Wants=network.target

[Service]
Type=notify
ExecStart={{.ExecutablePath}} --config {{.ConfigPath}}
ExecReload=/bin/kill -HUP $MAINPID
Restart=on-failure
RestartSec=5
TimeoutStopSec=30
User=root
Group=root
Environment=FLEETD_DATA_DIR={{.DataDir}}
WorkingDirectory={{.DataDir}}
StandardOutput=journal
StandardError=journal
SyslogIdentifier=fleetd
KillMode=mixed
KillSignal=SIGTERM

# Security settings
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths={{.DataDir}} {{.LogDir}}
PrivateTmp=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true

[Install]
WantedBy=multi-user.target`

type unitData struct {
	ExecutablePath string
	ConfigPath     string
	DataDir        string
	LogDir         string
}

// NewLinuxService creates a new Linux service
func NewLinuxService(cfg *config.AgentConfig) (*LinuxService, error) {
	agent, err := device.NewAgent(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create agent: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &LinuxService{
		agent:  agent,
		ctx:    ctx,
		cancel: cancel,
	}, nil
}

// Run starts the service and handles signals
func (s *LinuxService) Run() error {
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

// InstallService installs the systemd service
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

	// Create systemd unit file
	unitPath := filepath.Join(systemdDir, unitFile)

	// Check if service already exists
	if _, err := os.Stat(unitPath); err == nil {
		return fmt.Errorf("service %s already exists at %s", serviceName, unitPath)
	}

	// Parse template
	tmpl, err := template.New("unit").Parse(unitTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse unit template: %w", err)
	}

	// Create unit file
	file, err := os.Create(unitPath)
	if err != nil {
		return fmt.Errorf("failed to create unit file: %w", err)
	}
	defer file.Close()

	// Execute template
	data := unitData{
		ExecutablePath: exePath,
		ConfigPath:     configPath,
		DataDir:        dataDir,
		LogDir:         logDir,
	}

	err = tmpl.Execute(file, data)
	if err != nil {
		os.Remove(unitPath)
		return fmt.Errorf("failed to write unit file: %w", err)
	}

	// Set proper permissions
	err = os.Chmod(unitPath, 0644)
	if err != nil {
		log.Printf("Warning: failed to set unit file permissions: %v", err)
	}

	// Reload systemd configuration
	err = systemdReload()
	if err != nil {
		log.Printf("Warning: failed to reload systemd: %v", err)
	}

	log.Printf("Service %s installed successfully at %s", serviceName, unitPath)
	return nil
}

// UninstallService removes the systemd service
func UninstallService() error {
	unitPath := filepath.Join(systemdDir, unitFile)

	// Check if service exists
	if _, err := os.Stat(unitPath); os.IsNotExist(err) {
		return fmt.Errorf("service %s not found", serviceName)
	}

	// Stop and disable the service first
	err := StopService()
	if err != nil {
		log.Printf("Warning: failed to stop service: %v", err)
	}

	err = DisableService()
	if err != nil {
		log.Printf("Warning: failed to disable service: %v", err)
	}

	// Remove unit file
	err = os.Remove(unitPath)
	if err != nil {
		return fmt.Errorf("failed to remove unit file: %w", err)
	}

	// Reload systemd configuration
	err = systemdReload()
	if err != nil {
		log.Printf("Warning: failed to reload systemd: %v", err)
	}

	log.Printf("Service %s uninstalled successfully", serviceName)
	return nil
}

// StartService starts the systemd service
func StartService() error {
	cmd := exec.Command("systemctl", "start", serviceName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to start service: %w\nOutput: %s", err, string(output))
	}

	log.Printf("Service %s started successfully", serviceName)
	return nil
}

// StopService stops the systemd service
func StopService() error {
	cmd := exec.Command("systemctl", "stop", serviceName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to stop service: %w\nOutput: %s", err, string(output))
	}

	log.Printf("Service %s stopped successfully", serviceName)
	return nil
}

// RestartService restarts the systemd service
func RestartService() error {
	cmd := exec.Command("systemctl", "restart", serviceName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to restart service: %w\nOutput: %s", err, string(output))
	}

	log.Printf("Service %s restarted successfully", serviceName)
	return nil
}

// EnableService enables the service to start at boot
func EnableService() error {
	cmd := exec.Command("systemctl", "enable", serviceName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to enable service: %w\nOutput: %s", err, string(output))
	}

	log.Printf("Service %s enabled for auto-start", serviceName)
	return nil
}

// DisableService disables the service from starting at boot
func DisableService() error {
	cmd := exec.Command("systemctl", "disable", serviceName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to disable service: %w\nOutput: %s", err, string(output))
	}

	log.Printf("Service %s disabled from auto-start", serviceName)
	return nil
}

// QueryService returns the service status
func QueryService() (map[string]interface{}, error) {
	cmd := exec.Command("systemctl", "status", serviceName, "--no-pager")
	output, err := cmd.CombinedOutput()

	result := make(map[string]interface{})
	result["output"] = string(output)

	if err != nil {
		// Service might be inactive or failed
		if exitError, ok := err.(*exec.ExitError); ok {
			result["exit_code"] = exitError.ExitCode()
		}
		result["error"] = err.Error()
	}

	// Parse systemctl output for status
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Active:") {
			result["status"] = strings.TrimSpace(strings.TrimPrefix(line, "Active:"))
			break
		}
	}

	if result["status"] == nil {
		result["status"] = "unknown"
	}

	return result, nil
}

// systemdReload reloads systemd configuration
func systemdReload() error {
	cmd := exec.Command("systemctl", "daemon-reload")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to reload systemd: %w\nOutput: %s", err, string(output))
	}
	return nil
}

// RunService runs the service (called by main)
func RunService(cfg *config.AgentConfig) error {
	service, err := NewLinuxService(cfg)
	if err != nil {
		return fmt.Errorf("failed to create service: %w", err)
	}

	return service.Run()
}

// GetDefaultConfigPath returns the default configuration path for Linux
func GetDefaultConfigPath() string {
	return "/etc/fleetd/agent.yaml"
}

// GetDefaultDataDir returns the default data directory for Linux
func GetDefaultDataDir() string {
	return "/var/lib/fleetd"
}

// GetDefaultLogDir returns the default log directory for Linux
func GetDefaultLogDir() string {
	return "/var/log/fleetd"
}