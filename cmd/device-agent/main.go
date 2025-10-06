package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"

	"fleetd.sh/internal/agent/device"
	"fleetd.sh/internal/agent/service"
	"fleetd.sh/internal/config"
	"github.com/coreos/go-systemd/v22/daemon"
)

var (
	agentInstance *device.Agent
	agentMutex    sync.RWMutex
	shutdownOnce  sync.Once
)

func main() {
	var (
		configFile = flag.String("config", config.GetDefaultConfigPath(), "Configuration file path")
		serverURL  = flag.String("server", "", "Fleet server URL (overrides config)")
		deviceID   = flag.String("device-id", "", "Device ID (overrides config)")
		apiKey     = flag.String("api-key", "", "API key for authentication")
		debug      = flag.Bool("debug", false, "Enable debug logging")
		dataDir    = flag.String("data-dir", "", "Data directory for persistent storage (uses platform default if empty)")
		serviceCmd = flag.String("service", "", "Service command: install, uninstall, start, stop, restart, status")
	)
	flag.Parse()

	// Handle service commands first
	if *serviceCmd != "" {
		handleServiceCommand(*serviceCmd, *configFile)
		return
	}

	// Load configuration
	cfg, err := config.LoadAgentConfig(*configFile)
	if err != nil {
		log.Printf("Warning: Could not load config file: %v", err)
		cfg = config.DefaultAgentConfig()
	}

	// Override with command-line flags
	if *serverURL != "" {
		cfg.ServerURL = *serverURL
	}
	if *deviceID != "" {
		cfg.DeviceID = *deviceID
	}
	if *apiKey != "" {
		cfg.APIKey = *apiKey
	}
	if *debug {
		cfg.Debug = true
	}

	// Validate configuration
	if cfg.ServerURL == "" {
		log.Fatal("Server URL is required")
	}
	if cfg.DeviceID == "" {
		// Generate device ID if not provided
		cfg.DeviceID = device.GenerateDeviceID()
		log.Printf("Generated device ID: %s", cfg.DeviceID)
	}

	// Set data directory in config (use provided or default)
	if *dataDir != "" {
		cfg.DataDir = *dataDir
	}

	// Ensure data directory exists
	if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
		log.Fatalf("Failed to create data directory: %v", err)
	}

	// Create state file path
	stateFile := filepath.Join(cfg.DataDir, "agent.state")

	// Try to restore previous state
	if err := restoreAgentState(stateFile, cfg); err != nil {
		log.Printf("Could not restore previous state: %v", err)
	}

	// Create agent
	agent, err := device.NewAgent(cfg)
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	agentMutex.Lock()
	agentInstance = agent
	agentMutex.Unlock()

	// Setup signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	// Handle signals
	go func() {
		for sig := range sigChan {
			switch sig {
			case syscall.SIGINT, syscall.SIGTERM:
				log.Println("Received shutdown signal")
				gracefulShutdown(stateFile)
				cancel()
			case syscall.SIGHUP:
				log.Println("Received reload signal")
				if err := reloadConfiguration(*configFile); err != nil {
					log.Printf("Failed to reload configuration: %v", err)
				}
			}
		}
	}()

	// Start systemd watchdog if available (Linux only)
	if runtime.GOOS == "linux" {
		if interval, err := daemon.SdWatchdogEnabled(false); err == nil && interval > 0 {
			go systemdWatchdog(ctx)
		}
	}

	// Notify systemd we're ready (Linux only)
	if runtime.GOOS == "linux" {
		if _, err := daemon.SdNotify(false, daemon.SdNotifyReady); err != nil {
			log.Printf("Failed to notify systemd: %v", err)
		}
	}

	// Start agent with crash recovery
	log.Printf("Starting FleetD device agent (ID: %s)", cfg.DeviceID)
	if err := runAgentWithRecovery(ctx, agent); err != nil {
		log.Fatalf("Agent failed: %v", err)
	}

	log.Println("Agent stopped")
}

// runAgentWithRecovery runs the agent with panic recovery
func runAgentWithRecovery(ctx context.Context, agent *device.Agent) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Run agent with panic recovery
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("Agent panic recovered: %v", r)
					// Notify systemd of the error (Linux only)
					if runtime.GOOS == "linux" {
						daemon.SdNotify(false, fmt.Sprintf("STATUS=Recovered from panic: %v", r))
					}
					// Wait before restart
					time.Sleep(5 * time.Second)
				}
			}()

			if err := agent.Start(ctx); err != nil {
				if ctx.Err() != nil {
					return // Context cancelled, exit normally
				}
				log.Printf("Agent error: %v, restarting in 10 seconds", err)
				if runtime.GOOS == "linux" {
					daemon.SdNotify(false, fmt.Sprintf("STATUS=Restarting after error: %v", err))
				}
				time.Sleep(10 * time.Second)
			}
		}()
	}
}

// systemdWatchdog sends keepalive signals to systemd
func systemdWatchdog(ctx context.Context) {
	interval, err := daemon.SdWatchdogEnabled(false)
	if err != nil || interval == 0 {
		return
	}

	// Send watchdog keepalive at half the interval
	ticker := time.NewTicker(interval / 2)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			agentMutex.RLock()
			agent := agentInstance
			agentMutex.RUnlock()

			if agent != nil && agent.IsHealthy() {
				daemon.SdNotify(false, daemon.SdNotifyWatchdog)
			} else {
				log.Println("Agent unhealthy, not sending watchdog keepalive")
			}
		}
	}
}

// gracefulShutdown performs cleanup before exit
func gracefulShutdown(stateFile string) {
	shutdownOnce.Do(func() {
		log.Println("Starting graceful shutdown...")
		if runtime.GOOS == "linux" {
			daemon.SdNotify(false, "STOPPING=1")
		}

		agentMutex.RLock()
		agent := agentInstance
		agentMutex.RUnlock()

		if agent != nil {
			// Save current state
			if err := saveAgentState(stateFile, agent); err != nil {
				log.Printf("Failed to save agent state: %v", err)
			}

			// Perform cleanup
			if err := agent.Cleanup(); err != nil {
				log.Printf("Cleanup error: %v", err)
			}
		}

		log.Println("Graceful shutdown completed")
	})
}

// reloadConfiguration reloads the configuration file
func reloadConfiguration(configFile string) error {
	log.Printf("Reloading configuration from %s", configFile)
	if runtime.GOOS == "linux" {
		daemon.SdNotify(false, "RELOADING=1")
	}

	cfg, err := config.LoadAgentConfig(configFile)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	agentMutex.Lock()
	defer agentMutex.Unlock()

	if agentInstance != nil {
		if err := agentInstance.UpdateConfig(cfg); err != nil {
			return fmt.Errorf("failed to update config: %w", err)
		}
	}

	if runtime.GOOS == "linux" {
		daemon.SdNotify(false, "READY=1")
	}
	log.Println("Configuration reloaded successfully")
	return nil
}

// saveAgentState saves agent state to disk
func saveAgentState(stateFile string, agent *device.Agent) error {
	state := agent.GetState()
	if state == nil {
		return nil
	}

	data, err := state.Marshal()
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	// Write atomically
	tmpFile := stateFile + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0600); err != nil {
		return fmt.Errorf("failed to write state: %w", err)
	}

	return os.Rename(tmpFile, stateFile)
}

// restoreAgentState restores agent state from disk
func restoreAgentState(stateFile string, cfg *config.AgentConfig) error {
	data, err := os.ReadFile(stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No previous state
		}
		return fmt.Errorf("failed to read state: %w", err)
	}

	// Parse and apply state to config
	if err := cfg.RestoreState(data); err != nil {
		return fmt.Errorf("failed to restore state: %w", err)
	}

	log.Printf("Restored previous agent state from %s", stateFile)
	return nil
}

// handleServiceCommand handles service management commands
func handleServiceCommand(cmd, configFile string) {
	serviceManager := service.NewServiceManager()

	switch cmd {
	case "install":
		if err := serviceManager.Install(configFile); err != nil {
			log.Fatalf("Failed to install service: %v", err)
		}
		log.Println("Service installed successfully")

	case "uninstall":
		if err := serviceManager.Uninstall(); err != nil {
			log.Fatalf("Failed to uninstall service: %v", err)
		}
		log.Println("Service uninstalled successfully")

	case "start":
		if err := serviceManager.Start(); err != nil {
			log.Fatalf("Failed to start service: %v", err)
		}
		log.Println("Service started successfully")

	case "stop":
		if err := serviceManager.Stop(); err != nil {
			log.Fatalf("Failed to stop service: %v", err)
		}
		log.Println("Service stopped successfully")

	case "restart":
		if err := serviceManager.Restart(); err != nil {
			log.Fatalf("Failed to restart service: %v", err)
		}
		log.Println("Service restarted successfully")

	case "status":
		status, err := serviceManager.Query()
		if err != nil {
			log.Fatalf("Failed to query service status: %v", err)
		}
		fmt.Printf("Service Status:\n")
		for key, value := range status {
			fmt.Printf("  %s: %v\n", key, value)
		}

	case "enable":
		if err := serviceManager.Enable(); err != nil {
			log.Fatalf("Failed to enable service: %v", err)
		}
		log.Println("Service enabled successfully")

	case "disable":
		if err := serviceManager.Disable(); err != nil {
			log.Fatalf("Failed to disable service: %v", err)
		}
		log.Println("Service disabled successfully")

	default:
		log.Fatalf("Unknown service command: %s\nValid commands: install, uninstall, start, stop, restart, status, enable, disable", cmd)
	}
}
