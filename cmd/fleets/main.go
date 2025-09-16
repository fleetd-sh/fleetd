package main

import (
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"

	"fleetd.sh/internal/server"
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

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "fleets - Central management system for fleetd agents\n\n")
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
		fmt.Printf("fleets\n")
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
		secretKey = os.Getenv("FLEETD_SECRET_KEY")
		if secretKey == "" {
			secretKey = os.Getenv("JWT_SECRET")
		}
		if secretKey == "" {
			log.Fatal("Error: secret key is required: set via --secret-key flag, FLEETD_SECRET_KEY, or JWT_SECRET environment variable")
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
		log.Fatalf("Failed to create server: %v", err)
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

	if err := s.Run(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
