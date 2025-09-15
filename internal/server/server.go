package server

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	pb "fleetd.sh/gen/fleetd/v1/fleetpbconnect"
	"fleetd.sh/internal/api"
	"fleetd.sh/internal/middleware"

	_ "github.com/mattn/go-sqlite3"
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
	config      *Config
	db          *sql.DB
	httpServer  *http.Server
	mux         *http.ServeMux
	sseHub      *SSEHub
	rateLimiter *middleware.ValkeyRateLimiter
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

	// Initialize rate limiter if Valkey is configured
	var rateLimiter *middleware.ValkeyRateLimiter
	if config.ValkeyAddr != "" {
		rateLimiter, err = middleware.NewValkeyRateLimiter(
			config.ValkeyAddr,
			config.RateLimitReq,
			config.RateLimitWin,
		)
		if err != nil {
			slog.Warn("Failed to initialize Valkey rate limiter", "error", err)
			// Continue without rate limiting
		} else {
			slog.Info("Valkey rate limiter initialized", "addr", config.ValkeyAddr)
		}
	}

	return &Server{
		config:      config,
		db:          db,
		mux:         http.NewServeMux(),
		sseHub:      sseHub,
		rateLimiter: rateLimiter,
	}, nil
}

// Start starts the fleet server
func (s *Server) Start(ctx context.Context) error {
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

	// Configure CORS - same origin by default
	corsHandler := cors.New(cors.Options{
		AllowedOrigins:   []string{}, // Empty = same origin only
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"*"},
		AllowCredentials: true,
	}).Handler(s.mux)

	// Create HTTP server
	s.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.config.Port),
		Handler: corsHandler,
	}

	// Start SSE hub
	go s.sseHub.run()

	// Start server in goroutine
	errCh := make(chan error, 1)
	go func() {
		slog.Info("Starting fleet server", "port", s.config.Port)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	// Wait for context cancellation or error
	select {
	case <-ctx.Done():
		return s.Shutdown(context.Background())
	case err := <-errCh:
		return fmt.Errorf("server error: %w", err)
	}
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) error {
	slog.Info("Shutting down fleet server")

	if s.httpServer != nil {
		if err := s.httpServer.Shutdown(ctx); err != nil {
			return fmt.Errorf("failed to shutdown server: %w", err)
		}
	}

	if s.db != nil {
		if err := s.db.Close(); err != nil {
			return fmt.Errorf("failed to close database: %w", err)
		}
	}

	return nil
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
	// For now, just return the handler as-is
	// We can add middleware later
	return handler
}

// setupManagementAPI sets up the management REST API endpoints
func (s *Server) setupManagementAPI() {
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
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}

	// Run migrations
	if err := runMigrations(db); err != nil {
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
