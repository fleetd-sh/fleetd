package main

import (
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"strings"

	"fleetd.sh/internal/control"
	"fleetd.sh/internal/version"
)

func main() {
	var (
		serverPort      int
		serverDBPath    string
		serverSecretKey string
		deviceAPIURL    string
		valkeyAddr      string
		rateLimitReq    int
		rateLimitWindow int
		showVersion     bool
		showHelp        bool
	)

	flag.IntVar(&serverPort, "port", 8090, "Port to listen on")
	flag.StringVar(&serverDBPath, "db", "./fleet.db", "Database file path")
	flag.StringVar(&serverSecretKey, "secret-key", "", "Secret key for API authentication")
	flag.StringVar(&deviceAPIURL, "device-api-url", "http://localhost:8080", "Device API URL")
	flag.StringVar(&valkeyAddr, "valkey", "", "Valkey address for caching (e.g., localhost:6379)")
	flag.IntVar(&rateLimitReq, "rate-limit-requests", 100, "Rate limit requests per window")
	flag.IntVar(&rateLimitWindow, "rate-limit-window", 60, "Rate limit window in seconds")
	flag.BoolVar(&showVersion, "version", false, "Show version information")
	flag.BoolVar(&showHelp, "help", false, "Show help message")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "platform-api - Platform Management API Server\n")
		fmt.Fprintf(os.Stderr, "Manages fleets, deployments, analytics, and provides control plane for CLI/Web UI\n\n")
		fmt.Fprintf(os.Stderr, "Usage: %s [options]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	if showHelp {
		flag.Usage()
		os.Exit(0)
	}

	if showVersion {
		fmt.Printf("platform-api\n")
		fmt.Printf("Version: %s\n", version.Version)
		fmt.Printf("Commit: %s\n", version.CommitSHA)
		fmt.Printf("Built: %s\n", version.BuildTime)
		os.Exit(0)
	}

	// Configure logging based on environment
	logLevel := os.Getenv("LOG_LEVEL")
	logFormat := os.Getenv("LOG_FORMAT")

	var logHandler slog.Handler
	opts := &slog.HandlerOptions{Level: slog.LevelInfo}

	// Set log level
	switch strings.ToLower(logLevel) {
	case "debug":
		opts.Level = slog.LevelDebug
	case "warn", "warning":
		opts.Level = slog.LevelWarn
	case "error":
		opts.Level = slog.LevelError
	}

	// Set log format
	if logFormat == "json" {
		logHandler = slog.NewJSONHandler(log.Writer(), opts)
	} else {
		logHandler = slog.NewTextHandler(log.Writer(), opts)
	}

	slog.SetDefault(slog.New(logHandler))

	// Use environment variable for secret key if not provided via flag
	secretKey := serverSecretKey
	if secretKey == "" {
		secretKey = os.Getenv("PLATFORM_API_SECRET_KEY")
		if secretKey == "" {
			secretKey = os.Getenv("JWT_SECRET")
		}
		if secretKey == "" {
			log.Fatal("Error: secret key is required: set via --secret-key flag, PLATFORM_API_SECRET_KEY, or JWT_SECRET environment variable")
		}
	}

	// Override with environment variables if set
	if envPort := os.Getenv("PLATFORM_API_PORT"); envPort != "" {
		fmt.Sscanf(envPort, "%d", &serverPort)
	}
	if envDB := os.Getenv("DB_PATH"); envDB != "" {
		serverDBPath = envDB
	}
	if envDeviceAPI := os.Getenv("DEVICE_API_URL"); envDeviceAPI != "" {
		deviceAPIURL = envDeviceAPI
	}

	// Database configuration from environment
	dbDriver := os.Getenv("DB_DRIVER")
	if dbDriver == "" {
		dbDriver = "sqlite3"
	}

	// PostgreSQL connection string
	if dbDriver == "postgres" {
		dbHost := os.Getenv("DB_HOST")
		dbPort := os.Getenv("DB_PORT")
		dbName := os.Getenv("DB_NAME")
		dbUser := os.Getenv("DB_USER")
		dbPassword := os.Getenv("DB_PASSWORD")
		dbSSLMode := os.Getenv("DB_SSLMODE")

		if dbHost != "" && dbName != "" && dbUser != "" {
			if dbPort == "" {
				dbPort = "5432"
			}
			if dbSSLMode == "" {
				dbSSLMode = "require"
			}
			serverDBPath = fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
				dbHost, dbPort, dbUser, dbPassword, dbName, dbSSLMode)
		}
	}

	// TLS Configuration
	tlsMode := os.Getenv("FLEETD_TLS_MODE")
	if tlsMode == "" {
		tlsMode = "tls" // Default to TLS enabled
	}
	tlsCertFile := os.Getenv("FLEETD_TLS_CERT")
	tlsKeyFile := os.Getenv("FLEETD_TLS_KEY")
	tlsCAFile := os.Getenv("FLEETD_TLS_CA")

	config := &control.Config{
		Port:         serverPort,
		DatabasePath: serverDBPath,
		SecretKey:    secretKey,
		DeviceAPIURL: deviceAPIURL,
		ValkeyAddr:   valkeyAddr,
		RateLimitReq: rateLimitReq,
		RateLimitWin: rateLimitWindow,
		TLSMode:      tlsMode,
		TLSCert:      tlsCertFile,
		TLSKey:       tlsKeyFile,
		TLSCA:        tlsCAFile,
	}

	s, err := control.NewServer(config)
	if err != nil {
		log.Fatalf("Failed to create control plane server: %v", err)
	}

	// Check auth mode for security status
	authMode := "PRODUCTION (Secure)"
	authModeColor := "\033[32m" // Green
	if os.Getenv("FLEETD_AUTH_MODE") == "development" ||
		os.Getenv("FLEETD_INSECURE") == "true" ||
		os.Getenv("NODE_ENV") == "development" {
		authMode = "DEVELOPMENT/INSECURE"
		authModeColor = "\033[31m" // Red

		// Print prominent security warning
		fmt.Println("\033[31m" + `
╔══════════════════════════════════════════════════════════════╗
║                      SECURITY WARNING                       ║
║                                                              ║
║  Authentication is DISABLED or running in INSECURE mode!    ║
║                                                              ║
║  This server will accept unauthenticated requests.          ║
║  DO NOT expose this to the internet or untrusted networks!  ║
║                                                              ║
║  To enable secure authentication:                           ║
║  - Set FLEETD_AUTH_MODE=production                          ║
║  - Remove FLEETD_INSECURE=true if set                       ║
╚══════════════════════════════════════════════════════════════╝
` + "\033[0m")
	}

	slog.Info("Starting Platform API Server",
		"port", serverPort,
		"database", serverDBPath,
		"deviceAPI", deviceAPIURL,
		"valkey", valkeyAddr != "",
		"rateLimit", fmt.Sprintf("%d req/%ds", rateLimitReq, rateLimitWindow))

	// Show auth status prominently
	fmt.Printf("%sAuthentication Mode: %s%s\033[0m\n", authModeColor, authModeColor, authMode)
	fmt.Println()

	slog.Info("Control plane endpoints available")
	log.Printf("- Fleet Management: http://localhost:%d/api/v1/fleet", serverPort)
	log.Printf("- Device Management: http://localhost:%d/api/v1/devices", serverPort)
	log.Printf("- Analytics: http://localhost:%d/api/v1/analytics", serverPort)
	log.Printf("- Deployments: http://localhost:%d/api/v1/deployments", serverPort)
	log.Printf("- Configuration: http://localhost:%d/api/v1/config", serverPort)

	if err := s.Run(); err != nil {
		log.Fatalf("Control plane server error: %v", err)
	}
}
