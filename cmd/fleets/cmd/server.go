package cmd

import (
	"fmt"
	"log"
	"log/slog"
	"os"

	"fleetd.sh/internal/server"
	"github.com/spf13/cobra"
)

var (
	serverPort      int
	serverDBPath    string
	serverSecretKey string
	serverURL       string
	enableMDNS      bool
	valkeyAddr      string
	rateLimitReq    int
	rateLimitWindow int
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Run the fleet management server",
	Long: `Start the fleet management server that coordinates all fleetd agents.
	
The server provides:
- Device registration and authentication
- Update distribution
- Telemetry collection
- Web dashboard`,
	RunE: runServer,
}

func init() {
	serverCmd.Flags().IntVar(&serverPort, "port", 8080, "Port to listen on")
	serverCmd.Flags().StringVar(&serverDBPath, "db", "./fleet.db", "Database file path")
	serverCmd.Flags().StringVar(&serverSecretKey, "secret-key", "", "Secret key for API authentication")
	serverCmd.Flags().StringVar(&serverURL, "url", "", "Public URL of the server")
	serverCmd.Flags().BoolVar(&enableMDNS, "enable-mdns", true, "Enable mDNS discovery")
	serverCmd.Flags().StringVar(&valkeyAddr, "valkey", "", "Valkey address for rate limiting (e.g., localhost:6379)")
	serverCmd.Flags().IntVar(&rateLimitReq, "rate-limit-requests", 100, "Rate limit requests per window")
	serverCmd.Flags().IntVar(&rateLimitWindow, "rate-limit-window", 60, "Rate limit window in seconds")
}

func runServer(cmd *cobra.Command, args []string) error {
	// Configure logging
	slog.SetDefault(slog.New(slog.NewTextHandler(log.Writer(), nil)))

	// Use environment variable for secret key if not provided via flag
	secretKey := serverSecretKey
	if secretKey == "" {
		secretKey = os.Getenv("FLEETD_SECRET_KEY")
		if secretKey == "" {
			secretKey = os.Getenv("JWT_SECRET")
		}
		if secretKey == "" {
			return fmt.Errorf("secret key is required: set via --secret-key flag, FLEETD_SECRET_KEY, or JWT_SECRET environment variable")
		}
	}

	config := &server.Config{
		Port:         serverPort,
		DatabasePath: serverDBPath,
		SecretKey:    secretKey,
		ServerURL:    serverURL,
		EnableMDNS:   enableMDNS,
		ValkeyAddr:   valkeyAddr,
		RateLimitReq: rateLimitReq,
		RateLimitWin: rateLimitWindow,
	}

	s, err := server.New(config)
	if err != nil {
		return err
	}

	slog.Info("Starting fleet server",
		"port", serverPort,
		"database", serverDBPath,
		"mdns", enableMDNS,
		"valkey", valkeyAddr != "",
		"rateLimit", fmt.Sprintf("%d req/%ds", rateLimitReq, rateLimitWindow))

	slog.Info("Server endpoints available")
	log.Printf("- Dashboard: http://localhost:%d/", serverPort)
	log.Printf("- Devices API: http://localhost:%d/api/v1/devices", serverPort)
	log.Printf("- Telemetry API: http://localhost:%d/api/v1/telemetry", serverPort)
	log.Printf("- Discovery API: http://localhost:%d/api/v1/discover", serverPort)

	return s.Run()
}
