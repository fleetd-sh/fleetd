package main

import (
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"strings"

	"fleetd.sh/internal/server"
	"fleetd.sh/internal/tracing"
	"fleetd.sh/internal/version"
)

func main() {
	var (
		serverPort      int
		serverDBPath    string
		serverSecretKey string
		serverURL       string
		enableMDNS      bool
		valkeyAddr      string
		rateLimitReq    int
		rateLimitWindow int
		showVersion     bool
		showHelp        bool
		// TLS flags
		tlsEnabled    bool
		tlsCertFile   string
		tlsKeyFile    string
		tlsAutoTLS    bool
		tlsDomain     string
		tlsSelfSigned bool
	)

	flag.IntVar(&serverPort, "port", 8080, "Port to listen on")
	flag.StringVar(&serverDBPath, "db", "./fleet.db", "Database file path")
	flag.StringVar(&serverSecretKey, "secret-key", "", "Secret key for API authentication")
	flag.StringVar(&serverURL, "url", "", "Public URL of the server")
	flag.BoolVar(&enableMDNS, "enable-mdns", true, "Enable mDNS discovery")
	flag.StringVar(&valkeyAddr, "valkey", "", "Valkey address for rate limiting (e.g., localhost:6379)")
	flag.IntVar(&rateLimitReq, "rate-limit-requests", 100, "Rate limit requests per window")
	flag.IntVar(&rateLimitWindow, "rate-limit-window", 60, "Rate limit window in seconds")
	flag.BoolVar(&showVersion, "version", false, "Show version information")
	flag.BoolVar(&showHelp, "help", false, "Show help message")
	// TLS flags
	flag.BoolVar(&tlsEnabled, "tls", false, "Enable TLS/HTTPS")
	flag.StringVar(&tlsCertFile, "tls-cert", "", "Path to TLS certificate file")
	flag.StringVar(&tlsKeyFile, "tls-key", "", "Path to TLS key file")
	flag.BoolVar(&tlsAutoTLS, "tls-auto", false, "Enable automatic TLS with Let's Encrypt")
	flag.StringVar(&tlsDomain, "tls-domain", "", "Domain for TLS certificate")
	flag.BoolVar(&tlsSelfSigned, "tls-self-signed", false, "Use self-signed certificate (dev only)")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "device-api - High-Volume Device API Server\n")
		fmt.Fprintf(os.Stderr, "Handles device telemetry, metrics, logs, and registrations\n\n")
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
		fmt.Printf("device-api\n")
		fmt.Printf("Version: %s\n", version.Version)
		fmt.Printf("Commit: %s\n", version.CommitSHA)
		fmt.Printf("Built: %s\n", version.BuildTime)
		os.Exit(0)
	}

	// Configure logging
	slog.SetDefault(slog.New(slog.NewTextHandler(log.Writer(), nil)))

	// Use environment variable for secret key if not provided via flag
	secretKey := serverSecretKey
	if secretKey == "" {
		secretKey = os.Getenv("DEVICE_API_SECRET_KEY")
		if secretKey == "" {
			secretKey = os.Getenv("JWT_SECRET")
		}
		if secretKey == "" {
			log.Fatal("Error: secret key is required: set via --secret-key flag, DEVICE_API_SECRET_KEY, or JWT_SECRET environment variable")
		}
	}

	// Check TLS environment variables if not set via flags
	if !tlsEnabled && os.Getenv("TLS_ENABLED") == "true" {
		tlsEnabled = true
	}
	if tlsCertFile == "" {
		tlsCertFile = os.Getenv("TLS_CERT_FILE")
	}
	if tlsKeyFile == "" {
		tlsKeyFile = os.Getenv("TLS_KEY_FILE")
	}
	if !tlsAutoTLS && os.Getenv("TLS_AUTO") == "true" {
		tlsAutoTLS = true
	}
	if tlsDomain == "" {
		tlsDomain = os.Getenv("TLS_DOMAIN")
	}
	if !tlsSelfSigned && os.Getenv("TLS_SELF_SIGNED") == "true" {
		tlsSelfSigned = true
	}

	// Configure TLS using environment or flags
	tlsMode := os.Getenv("TLS_MODE")
	if tlsMode == "" && tlsEnabled {
		tlsMode = "tls"
	} else if tlsMode == "" {
		tlsMode = "none"
	}

	// Override with environment variables if not set via flags
	if tlsCertFile == "" {
		tlsCertFile = os.Getenv("FLEETD_TLS_CERT")
	}
	if tlsKeyFile == "" {
		tlsKeyFile = os.Getenv("FLEETD_TLS_KEY")
	}
	tlsCAFile := os.Getenv("FLEETD_TLS_CA")

	// For mTLS support
	if tlsCAFile != "" && tlsMode == "tls" {
		tlsMode = "mtls"
	}

	// Configure tracing from environment
	tracingConfig := tracing.LoadFromEnvironment("device-api")
	tracingConfig.ServiceVersion = version.Version

	// Database configuration from environment
	dbDriver := "sqlite3" // Default driver
	if databaseURL := os.Getenv("DATABASE_URL"); databaseURL != "" {
		// Parse DATABASE_URL to determine driver
		if strings.HasPrefix(databaseURL, "postgres://") || strings.HasPrefix(databaseURL, "postgresql://") {
			dbDriver = "postgres"
			serverDBPath = databaseURL
		} else if strings.HasPrefix(databaseURL, "sqlite://") {
			dbDriver = "sqlite3"
			serverDBPath = strings.TrimPrefix(databaseURL, "sqlite://")
		} else {
			// Assume it's a direct connection string
			if strings.Contains(databaseURL, "host=") {
				dbDriver = "postgres"
			}
			serverDBPath = databaseURL
		}
	} else {
		// Fall back to individual environment variables
		if envDriver := os.Getenv("DB_DRIVER"); envDriver != "" {
			dbDriver = envDriver
		}

		// PostgreSQL connection string from individual vars
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
	}

	config := &server.Config{
		Port:           serverPort,
		DatabaseDriver: dbDriver,
		DatabasePath:   serverDBPath,
		SecretKey:      secretKey,
		ServerURL:      serverURL,
		EnableMDNS:     enableMDNS,
		ValkeyAddr:     valkeyAddr,
		RateLimitReq:   rateLimitReq,
		TLSMode:        tlsMode,
		TLSCert:        tlsCertFile,
		TLSKey:         tlsKeyFile,
		TLSCA:          tlsCAFile,
		RateLimitWin:   rateLimitWindow,
		Tracing:        tracingConfig,
	}

	s, err := server.New(config)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	// Log server info
	slog.Info("Starting Device API Server",
		"port", serverPort,
		"database", serverDBPath,
		"mdns", enableMDNS,
		"valkey", valkeyAddr != "",
		"rateLimit", fmt.Sprintf("%d req/%ds", rateLimitReq, rateLimitWindow),
		"tls", tlsEnabled,
		"tls_auto", tlsAutoTLS,
		"tls_self_signed", tlsSelfSigned,
		"tracing", tracingConfig.Enabled,
		"tracing_endpoint", tracingConfig.Endpoint)

	// Log endpoints with correct protocol
	protocol := "http"
	port := serverPort
	if tlsMode != "none" && tlsMode != "" {
		protocol = "https"
		// Keep same port for now, in production would use 443 or 8443
	}

	slog.Info("Device API endpoints available")
	log.Printf("- Registration: %s://localhost:%d/api/v1/register", protocol, port)
	log.Printf("- Telemetry: %s://localhost:%d/api/v1/telemetry", protocol, port)
	log.Printf("- Metrics: %s://localhost:%d/api/v1/metrics", protocol, port)
	log.Printf("- Logs: %s://localhost:%d/api/v1/logs", protocol, port)
	log.Printf("- Heartbeat: %s://localhost:%d/api/v1/heartbeat", protocol, port)
	log.Printf("- Health: %s://localhost:%d/health", protocol, port)
	log.Printf("- Prometheus Metrics: %s://localhost:%d/metrics", protocol, port)

	// Run the server (it handles signals internally)
	if err := s.Run(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
