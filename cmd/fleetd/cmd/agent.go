package cmd

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"syscall"

	"fleetd.sh/internal/agent"
	"github.com/spf13/cobra"
)

var (
	serverURL   string
	storageDir  string
	rpcPort     int
	mdnsPort    int
	disableMDNS bool
	requireSudo bool
)

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Run the fleetd agent",
	Long:  `Start the fleetd agent which handles device management, monitoring, and updates.`,
	RunE:  runAgent,
}

func init() {
	agentCmd.Flags().StringVar(&serverURL, "server-url", "http://localhost:8080", "URL of the fleet management server")
	agentCmd.Flags().StringVar(&storageDir, "storage-dir", "", "Directory for storing agent data")
	agentCmd.Flags().IntVar(&rpcPort, "rpc-port", 0, "Port to use for the local RPC server (0 for auto-select)")
	agentCmd.Flags().IntVar(&mdnsPort, "mdns-port", 0, "Port to advertise via mDNS (defaults to RPC port)")
	agentCmd.Flags().BoolVar(&disableMDNS, "disable-mdns", false, "Disable mDNS discovery")
	agentCmd.Flags().BoolVar(&requireSudo, "require-sudo", true, "Require sudo for system directories")
}

func runAgent(cmd *cobra.Command, args []string) error {
	// Set default storage directory based on permissions
	if storageDir == "" {
		storageDir = getStorageDir()
	}

	// Auto-select port if not specified
	if rpcPort == 0 {
		port, err := getFreePort()
		if err != nil {
			return fmt.Errorf("failed to find free port: %w", err)
		}
		rpcPort = port
		log.Printf("Auto-selected RPC port: %d", rpcPort)
	}

	// Default mDNS advertised port to RPC port if not specified
	if mdnsPort == 0 {
		mdnsPort = rpcPort
	}

	// Create storage directory with appropriate permissions
	if err := ensureStorageDir(storageDir); err != nil {
		return err
	}

	// Start with defaults
	cfg := agent.DefaultConfig()

	// Override with command line flags
	cfg.ServerURL = serverURL
	cfg.StorageDir = storageDir
	cfg.RPCPort = rpcPort
	cfg.MDNSPort = mdnsPort
	cfg.DisableMDNS = disableMDNS

	a := agent.New(cfg)
	if err := a.Start(); err != nil {
		return fmt.Errorf("failed to start agent: %w", err)
	}

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	if err := a.Stop(); err != nil {
		log.Printf("Error stopping agent: %v", err)
	}

	return nil
}

func getStorageDir() string {
	// Check if running as root
	if os.Geteuid() == 0 {
		return "/var/lib/fleetd"
	}

	// Use user's home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Printf("Warning: could not determine home directory: %v", err)
		return ".fleetd"
	}

	return filepath.Join(homeDir, ".fleetd")
}

func ensureStorageDir(dir string) error {
	// Check if directory exists
	if _, err := os.Stat(dir); err == nil {
		return nil
	}

	// Try to create directory
	if err := os.MkdirAll(dir, 0755); err != nil {
		if !os.IsPermission(err) || !requireSudo {
			return err
		}

		// Need sudo for system directories
		if dir == "/var/lib/fleetd" {
			fmt.Println("Creating system directory requires sudo permission.")
			fmt.Println("Please run: sudo mkdir -p /var/lib/fleetd && sudo chown $USER /var/lib/fleetd")
			fmt.Println("Or run fleetd with --storage-dir to use a different location.")
			return fmt.Errorf("permission denied")
		}

		return err
	}

	return nil
}

func getFreePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()

	addr := listener.Addr().(*net.TCPAddr)
	return addr.Port, nil
}

func isRoot() bool {
	u, err := user.Current()
	if err != nil {
		return false
	}
	return u.Uid == "0"
}
