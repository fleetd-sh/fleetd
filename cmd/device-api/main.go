package main

import (
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"

	"fleetd.sh/internal/server"
	fleetdTLS "fleetd.sh/internal/tls"
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

	// Configure TLS
	tlsConfig := &fleetdTLS.Config{
		Enabled:      tlsEnabled,
		CertFile:     tlsCertFile,
		KeyFile:      tlsKeyFile,
		AutoTLS:      tlsAutoTLS,
		Domain:       tlsDomain,
		SelfSigned:   tlsSelfSigned,
		Port:         443,
		HTTPPort:     serverPort,
		RedirectHTTP: tlsEnabled,
		CacheDir:     "./certs",
	}

	// Override ports if TLS is enabled
	if tlsEnabled {
		if serverPort == 8080 {
			tlsConfig.HTTPPort = 80
			tlsConfig.Port = 443
		} else {
			tlsConfig.HTTPPort = serverPort
			tlsConfig.Port = serverPort + 443 - 80 // e.g., 8080 -> 8443
		}
	}

	// Configure tracing from environment
	tracingConfig := tracing.LoadFromEnvironment("device-api")
	tracingConfig.ServiceVersion = version.Version

	config := &server.Config{
		Port:         serverPort,
		DatabasePath: serverDBPath,
		SecretKey:    secretKey,
		ServerURL:    serverURL,
		EnableMDNS:   enableMDNS,
		ValkeyAddr:   valkeyAddr,
		RateLimitReq: rateLimitReq,
		RateLimitWin: rateLimitWindow,
		TLS:          tlsConfig,
		Tracing:      tracingConfig,
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
	if tlsEnabled {
		protocol = "https"
		port = tlsConfig.Port
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
