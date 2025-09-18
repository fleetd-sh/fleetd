package server

import (
	"context"
	"crypto/tls"
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	pb "fleetd.sh/gen/fleetd/v1/fleetpbconnect"
	"fleetd.sh/internal/api"
	"fleetd.sh/internal/database"
	"fleetd.sh/internal/metrics"
	"fleetd.sh/internal/middleware"
	fleetdTLS "fleetd.sh/internal/tls"
	"fleetd.sh/internal/tracing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/cors"
)

//go:embed static/*
var staticFS embed.FS

// Config holds the server configuration
type Config struct {
	Port         int
	DatabasePath string
	EnableMDNS   bool
	MDNSPort     int
	ServerURL    string
	SecretKey    string
	ValkeyAddr   string
	RateLimitReq int
	RateLimitWin int
	TLS          *fleetdTLS.Config
	Tracing      *tracing.Config
}

// SSEHub manages SSE client connections
type SSEHub struct {
	clients    map[chan []byte]bool
	broadcast  chan []byte
	register   chan chan []byte
	unregister chan chan []byte
}

// Server represents the fleet management server
type Server struct {
	config          *Config
	db              *sql.DB
	httpServer      *http.Server
	httpsServer     *http.Server
	mux             *http.ServeMux
	sseHub          *SSEHub
	valkeyLimiter   *middleware.ValkeyRateLimiter
	inMemoryLimiter *middleware.RateLimiter
	tlsConfig       *tls.Config
}

// New creates a new fleet server instance
func New(config *Config) (*Server, error) {
	// Initialize database
	db, err := initDatabase(config.DatabasePath)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	// Create SSE hub
	sseHub := &SSEHub{
		clients:    make(map[chan []byte]bool),
		broadcast:  make(chan []byte),
		register:   make(chan chan []byte),
		unregister: make(chan chan []byte),
	}

	// Initialize rate limiter
	var valkeyLimiter *middleware.ValkeyRateLimiter
	var inMemoryLimiter *middleware.RateLimiter

	if config.ValkeyAddr != "" {
		// Use Valkey/Redis-based rate limiting for distributed systems
		valkeyLimiter, err = middleware.NewValkeyRateLimiter(
			config.ValkeyAddr,
			config.RateLimitReq,
			config.RateLimitWin,
		)
		if err != nil {
			slog.Warn("Failed to initialize Valkey rate limiter", "error", err)
			// Fall back to in-memory rate limiting
			rate := float64(config.RateLimitReq) / float64(config.RateLimitWin)
			inMemoryLimiter = middleware.NewRateLimiter(middleware.RateLimiterConfig{
				Rate:       rate,
				Burst:      config.RateLimitReq,
				Expiration: 1 * time.Hour,
			})
			slog.Info("Falling back to in-memory rate limiting",
				"rate_per_second", rate,
				"burst", config.RateLimitReq)
		} else {
			slog.Info("Valkey rate limiter initialized", "addr", config.ValkeyAddr)
		}
	} else {
		// Use in-memory rate limiting when Valkey is not configured
		rate := float64(config.RateLimitReq) / float64(config.RateLimitWin)
		inMemoryLimiter = middleware.NewRateLimiter(middleware.RateLimiterConfig{
			Rate:       rate,
			Burst:      config.RateLimitReq,
			Expiration: 1 * time.Hour,
		})
		slog.Info("Using in-memory rate limiting",
			"rate_per_second", rate,
			"burst", config.RateLimitReq)
	}

	return &Server{
		config:          config,
		db:              db,
		mux:             http.NewServeMux(),
		sseHub:          sseHub,
		valkeyLimiter:   valkeyLimiter,
		inMemoryLimiter: inMemoryLimiter,
	}, nil
}

// Start starts the fleet server
func (s *Server) Start(ctx context.Context) error {
	// Initialize tracing if configured
	var tracingShutdown func()
	if s.config.Tracing != nil {
		_, shutdown, err := tracing.Initialize(s.config.Tracing)
		if err != nil {
			slog.Warn("Failed to initialize tracing", "error", err)
		} else {
			tracingShutdown = shutdown
			defer tracingShutdown()
		}
	}

	// Initialize services
	deviceService := api.NewDeviceService(s.db)
	updateService := api.NewUpdateService(s.db)
	analyticsService := api.NewAnalyticsService(s.db)

	// Register Connect services
	path, handler := pb.NewDeviceServiceHandler(deviceService)
	s.mux.Handle(path, handler)

	path, handler = pb.NewUpdateServiceHandler(updateService)
	s.mux.Handle(path, handler)

	path, handler = pb.NewAnalyticsServiceHandler(analyticsService)
	s.mux.Handle(path, handler)

	// API endpoints for management
	s.setupManagementAPI()

	// Static files and dashboard
	s.setupDashboard()

	// Apply middleware stack
	var httpHandler http.Handler = s.mux

	// Apply tracing middleware
	if s.config.Tracing != nil && s.config.Tracing.Enabled {
		tracingMiddleware := middleware.NewTracingMiddleware("device-api")
		httpHandler = tracingMiddleware(httpHandler)
	}

	// Apply metrics middleware
	metricsMiddleware := middleware.NewMetricsMiddleware("device-api")
	httpHandler = metricsMiddleware(httpHandler)

	// Apply rate limiting middleware
	if s.inMemoryLimiter != nil {
		httpHandler = middleware.RateLimitMiddleware(s.inMemoryLimiter)(httpHandler)
	}

	// Configure CORS - same origin by default
	corsHandler := cors.New(cors.Options{
		AllowedOrigins:   []string{}, // Empty = same origin only
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"*"},
		AllowCredentials: true,
	}).Handler(httpHandler)

	// Start system metrics collector
	go s.collectSystemMetrics(ctx)

	// Start SSE hub
	go s.sseHub.run()

	// Setup servers based on TLS configuration
	errCh := make(chan error, 2)

	if s.config.TLS != nil && s.config.TLS.Enabled {
		// Get TLS configuration
		tlsConfig, err := s.config.TLS.GetTLSConfig()
		if err != nil {
			return fmt.Errorf("failed to get TLS config: %w", err)
		}

		// Create HTTPS server with security headers
		httpsHandler := s.withSecurityHeaders(corsHandler)
		s.httpsServer = &http.Server{
			Addr:              fmt.Sprintf(":%d", s.config.TLS.Port),
			Handler:           httpsHandler,
			TLSConfig:         tlsConfig,
			ReadTimeout:       15 * time.Second,
			ReadHeaderTimeout: 10 * time.Second,
			WriteTimeout:      15 * time.Second,
			IdleTimeout:       60 * time.Second,
		}

		// Start HTTPS server
		go func() {
			slog.Info("Starting HTTPS server",
				"port", s.config.TLS.Port,
				"auto_tls", s.config.TLS.AutoTLS,
				"self_signed", s.config.TLS.SelfSigned)

			if s.config.TLS.CertFile != "" && s.config.TLS.KeyFile != "" {
				err = s.httpsServer.ListenAndServeTLS(s.config.TLS.CertFile, s.config.TLS.KeyFile)
			} else {
				err = s.httpsServer.ListenAndServeTLS("", "")
			}
			if err != nil && err != http.ErrServerClosed {
				errCh <- fmt.Errorf("HTTPS server error: %w", err)
			}
		}()

		// Start HTTP redirect server if configured
		if s.config.TLS.RedirectHTTP {
			s.httpServer = &http.Server{
				Addr:              fmt.Sprintf(":%d", s.config.TLS.HTTPPort),
				Handler:           s.redirectToHTTPS(),
				ReadTimeout:       5 * time.Second,
				ReadHeaderTimeout: 3 * time.Second,
				WriteTimeout:      5 * time.Second,
				IdleTimeout:       15 * time.Second,
			}

			go func() {
				slog.Info("Starting HTTP redirect server", "port", s.config.TLS.HTTPPort)
				if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					errCh <- fmt.Errorf("HTTP redirect server error: %w", err)
				}
			}()
		}
	} else {
		// HTTP only server
		s.httpServer = &http.Server{
			Addr:              fmt.Sprintf(":%d", s.config.Port),
			Handler:           corsHandler,
			ReadTimeout:       15 * time.Second,
			ReadHeaderTimeout: 10 * time.Second,
			WriteTimeout:      15 * time.Second,
			IdleTimeout:       60 * time.Second,
		}

		go func() {
			slog.Info("Starting HTTP server (no TLS)", "port", s.config.Port)
			if s.config.TLS == nil || !s.config.TLS.Enabled {
				slog.Warn("[SECURITY] Running without TLS - DO NOT USE IN PRODUCTION")
			}
			if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				errCh <- fmt.Errorf("HTTP server error: %w", err)
			}
		}()
	}

	// Wait for context cancellation or error
	select {
	case <-ctx.Done():
		return s.Shutdown(context.Background())
	case err := <-errCh:
		return err
	}
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) error {
	slog.Info("Shutting down fleet server")

	var httpErr, httpsErr error

	if s.httpServer != nil {
		httpErr = s.httpServer.Shutdown(ctx)
		if httpErr != nil {
			slog.Error("Failed to shutdown HTTP server", "error", httpErr)
		}
	}

	if s.httpsServer != nil {
		httpsErr = s.httpsServer.Shutdown(ctx)
		if httpsErr != nil {
			slog.Error("Failed to shutdown HTTPS server", "error", httpsErr)
		}
	}

	if s.db != nil {
		if err := s.db.Close(); err != nil {
			slog.Error("Failed to close database", "error", err)
		}
	}

	if httpErr != nil {
		return httpErr
	}
	return httpsErr
}

// Run starts the server and handles shutdown signals
func (s *Server) Run() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigCh
		slog.Info("Received shutdown signal")
		cancel()
	}()

	return s.Start(ctx)
}

// withMiddleware wraps handlers with common middleware
func (s *Server) withMiddleware(handler http.Handler) http.Handler {
	// Apply rate limiting
	if s.inMemoryLimiter != nil {
		// Use in-memory rate limiting
		return middleware.RateLimitMiddleware(s.inMemoryLimiter)(handler)
	}

	// Note: ValkeyRateLimiter would be applied differently at the Connect-RPC level
	// For HTTP endpoints, we use the in-memory limiter
	return handler
}

// setupManagementAPI sets up the management REST API endpoints
func (s *Server) setupManagementAPI() {
	// Health check endpoints
	s.mux.HandleFunc("/health", s.handleHealth)
	s.mux.HandleFunc("/health/live", s.handleHealthLive)
	s.mux.HandleFunc("/health/ready", s.handleHealthReady)

	// Prometheus metrics endpoint
	s.mux.Handle("/metrics", promhttp.Handler())

	// Device management endpoints
	s.mux.HandleFunc("/api/v1/devices", s.handleDevices)
	s.mux.HandleFunc("/api/v1/devices/", s.handleDevice)

	// Telemetry endpoints
	s.mux.HandleFunc("/api/v1/telemetry", s.handleTelemetry)
	s.mux.HandleFunc("/api/v1/telemetry/metrics", s.handleMetrics)

	// Configuration endpoints
	s.mux.HandleFunc("/api/v1/config", s.handleConfig)

	// mDNS discovery endpoint
	s.mux.HandleFunc("/api/v1/discover", s.handleDiscover)

	// SSE events endpoint
	s.mux.HandleFunc("/api/v1/events", s.handleEvents)
}

// setupDashboard sets up the web dashboard
func (s *Server) setupDashboard() {
	// Serve static files
	fs := http.FileServer(http.FS(staticFS))
	s.mux.Handle("/static/", fs)

	// Dashboard route
	s.mux.HandleFunc("/", s.handleDashboard)
}

// initDatabase initializes the SQLite database
func initDatabase(path string) (*sql.DB, error) {
	ctx := context.Background()
	config := database.DefaultRetryConfig()

	// Use retry logic for database connection
	db, err := database.OpenWithRetry(ctx, "sqlite3", path, config)
	if err != nil {
		return nil, fmt.Errorf("failed to open database with retry: %w", err)
	}

	// Run migrations with retry
	err = database.ExecuteWithRetry(ctx, db, func() error {
		return runMigrations(db)
	}, config)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return db, nil
}

// runMigrations runs database migrations
func runMigrations(db *sql.DB) error {
	// Create tables if they don't exist
	queries := []string{
		`CREATE TABLE IF NOT EXISTS device (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			type TEXT NOT NULL,
			version TEXT NOT NULL,
			api_key TEXT UNIQUE NOT NULL,
			metadata TEXT,
			last_seen TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS telemetry (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			device_id TEXT NOT NULL,
			timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			metric_name TEXT NOT NULL,
			metric_value REAL NOT NULL,
			metadata TEXT,
			FOREIGN KEY (device_id) REFERENCES device(id)
		)`,
		`CREATE TABLE IF NOT EXISTS device_config (
			device_id TEXT PRIMARY KEY,
			config TEXT NOT NULL,
			version INTEGER DEFAULT 1,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (device_id) REFERENCES device(id)
		)`,
		`CREATE TABLE IF NOT EXISTS updates (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			version TEXT NOT NULL,
			description TEXT,
			binary_url TEXT NOT NULL,
			checksum TEXT NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS device_updates (
			device_id TEXT NOT NULL,
			update_id INTEGER NOT NULL,
			status TEXT DEFAULT 'pending',
			applied_at TIMESTAMP,
			PRIMARY KEY (device_id, update_id),
			FOREIGN KEY (device_id) REFERENCES device(id),
			FOREIGN KEY (update_id) REFERENCES updates(id)
		)`,
	}

	for _, query := range queries {
		if _, err := db.Exec(query); err != nil {
			return fmt.Errorf("failed to execute migration: %w", err)
		}
	}

	return nil
}

// run manages the SSE hub
func (h *SSEHub) run() {
	for {
		select {
		case client := <-h.register:
			h.clients[client] = true
			slog.Info("SSE client connected", "total", len(h.clients))

		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				slog.Info("SSE client disconnected", "total", len(h.clients))
			}

		case message := <-h.broadcast:
			for client := range h.clients {
				select {
				case client <- message:
				default:
					// Client's send channel is full, close it
					delete(h.clients, client)
					close(client)
				}
			}
		}
	}
}

// BroadcastEvent sends an event to all connected SSE clients
func (s *Server) BroadcastEvent(eventType string, data map[string]any) {
	data["type"] = eventType
	data["timestamp"] = time.Now().Format(time.RFC3339)

	jsonData, err := json.Marshal(data)
	if err != nil {
		slog.Error("Failed to marshal SSE event", "error", err)
		return
	}

	select {
	case s.sseHub.broadcast <- jsonData:
	default:
		// Broadcast channel is full, skip
	}
}

// handleHealth returns the overall health status
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	health := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().Unix(),
		"service":   "device-api",
		"version":   "1.0.0",
		"checks": map[string]interface{}{
			"database": s.checkDatabase(),
			"memory":   s.checkMemory(),
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(health)
}

// handleHealthLive returns liveness status (is the service running?)
func (s *Server) handleHealthLive(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "alive",
		"timestamp": time.Now().Unix(),
	})
}

// handleHealthReady returns readiness status (is the service ready to handle requests?)
func (s *Server) handleHealthReady(w http.ResponseWriter, r *http.Request) {
	// Check if database is accessible
	if err := s.db.Ping(); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":    "not_ready",
			"error":     err.Error(),
			"timestamp": time.Now().Unix(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "ready",
		"timestamp": time.Now().Unix(),
	})
}

// collectSystemMetrics periodically collects system metrics
func (s *Server) collectSystemMetrics(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	startTime := time.Now()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Update uptime
			metrics.SystemUptime.WithLabelValues("device-api").Set(time.Since(startTime).Seconds())

			// Update memory metrics
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			metrics.SystemMemoryUsage.WithLabelValues("device-api", "alloc").Set(float64(m.Alloc))
			metrics.SystemMemoryUsage.WithLabelValues("device-api", "heap").Set(float64(m.HeapAlloc))
			metrics.SystemMemoryUsage.WithLabelValues("device-api", "sys").Set(float64(m.Sys))

			// Update goroutines count
			metrics.SystemGoroutines.WithLabelValues("device-api").Set(float64(runtime.NumGoroutine()))

			// Update database connection metrics
			if s.db != nil {
				stats := s.db.Stats()
				metrics.DBConnectionsActive.WithLabelValues("device-api").Set(float64(stats.OpenConnections))
			}

			// Update device count metrics (query from database)
			if s.db != nil {
				var totalDevices, connectedDevices int
				s.db.QueryRow("SELECT COUNT(*) FROM devices").Scan(&totalDevices)
				s.db.QueryRow("SELECT COUNT(*) FROM devices WHERE status = 'online'").Scan(&connectedDevices)

				metrics.DevicesTotal.WithLabelValues("all", "all").Set(float64(totalDevices))
				metrics.DevicesConnected.Set(float64(connectedDevices))
			}
		}
	}
}

// checkDatabase checks database connectivity
func (s *Server) checkDatabase() string {
	if err := s.db.Ping(); err != nil {
		return "unhealthy"
	}
	return "healthy"
}

// checkMemory checks memory usage
func (s *Server) checkMemory() string {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// Consider unhealthy if using more than 1GB
	if m.Alloc > 1024*1024*1024 {
		return "degraded"
	}
	return "healthy"
}

// redirectToHTTPS returns a handler that redirects HTTP to HTTPS
func (s *Server) redirectToHTTPS() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		// Remove port from host if present
		if idx := strings.LastIndex(host, ":"); idx != -1 {
			host = host[:idx]
		}

		// Build HTTPS URL
		url := fmt.Sprintf("https://%s", host)
		if s.config.TLS.Port != 443 {
			url = fmt.Sprintf("https://%s:%d", host, s.config.TLS.Port)
		}
		url += r.URL.String()

		http.Redirect(w, r, url, http.StatusMovedPermanently)
	})
}

// withSecurityHeaders adds security headers to HTTPS responses
func (s *Server) withSecurityHeaders(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// HSTS - Strict Transport Security
		w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")

		// Prevent clickjacking
		w.Header().Set("X-Frame-Options", "DENY")

		// Prevent MIME type sniffing
		w.Header().Set("X-Content-Type-Options", "nosniff")

		// XSS Protection (for older browsers)
		w.Header().Set("X-XSS-Protection", "1; mode=block")

		// Content Security Policy
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'")

		// Referrer Policy
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

		// Permissions Policy
		w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")

		handler.ServeHTTP(w, r)
	})
}
