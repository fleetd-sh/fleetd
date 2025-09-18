package control

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"fleetd.sh/gen/fleetd/v1/fleetpbconnect"
	"fleetd.sh/internal/database"
	"fleetd.sh/internal/metrics"
	"fleetd.sh/internal/middleware"
	"fleetd.sh/internal/version"
	_ "github.com/mattn/go-sqlite3"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/cors"
	"github.com/spf13/viper"
)

// Config holds the control plane server configuration
type Config struct {
	Port         int
	DatabasePath string
	SecretKey    string
	DeviceAPIURL string
	ValkeyAddr   string
	RateLimitReq int
	RateLimitWin int
}

// Server represents the control plane API server
type Server struct {
	config          *Config
	db              *sql.DB
	httpServer      *http.Server
	deviceAPI       *DeviceAPIClient
	valkeyLimiter   *middleware.ValkeyRateLimiter
	inMemoryLimiter *middleware.RateLimiter
}

// DeviceAPIClient wraps communication with the device API
type DeviceAPIClient struct {
	baseURL string
	client  *http.Client
}

// NewServer creates a new control plane server instance
func NewServer(config *Config) (*Server, error) {
	// Load configuration from config.toml if it exists
	viper.SetConfigName("config")
	viper.SetConfigType("toml")
	viper.AddConfigPath(".")

	if err := viper.ReadInConfig(); err == nil {
		slog.Info("Loaded configuration from config.toml")

		// Override with values from config.toml
		if viper.IsSet("control_api.port") && config.Port == 8090 {
			config.Port = viper.GetInt("control_api.port")
		}
		if viper.IsSet("control_api.host") {
			// Note: host is used for binding, not stored in config struct
		}
		if viper.IsSet("device_api.port") && config.DeviceAPIURL == "http://localhost:8080" {
			config.DeviceAPIURL = fmt.Sprintf("http://localhost:%d", viper.GetInt("device_api.port"))
		}
	}

	// Initialize database with migrations
	dbConfig := database.DefaultConfig("sqlite3")
	dbConfig.DSN = config.DatabasePath
	dbConfig.MigrationsPath = "migrations"

	dbInstance, err := database.New(dbConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	// Get the underlying sql.DB for compatibility
	db := dbInstance.DB

	// Create device API client
	deviceAPI := &DeviceAPIClient{
		baseURL: config.DeviceAPIURL,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}

	// Initialize rate limiting
	var valkeyLimiter *middleware.ValkeyRateLimiter
	var inMemoryLimiter *middleware.RateLimiter

	if config.ValkeyAddr != "" {
		// Use Valkey/Redis-based rate limiting for distributed systems
		var err error
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
		deviceAPI:       deviceAPI,
		valkeyLimiter:   valkeyLimiter,
		inMemoryLimiter: inMemoryLimiter,
	}, nil
}

// Run starts the control plane server
func (s *Server) Run() error {
	mux := http.NewServeMux()

	// Setup middleware
	authMiddleware := middleware.NewAuthMiddleware(s.config.SecretKey)
	loggingMiddleware := middleware.NewLoggingMiddleware()
	metricsMiddleware := middleware.NewMetricsMiddleware("platform-api")

	// Create service handlers
	fleetService := NewFleetService(s.db, s.deviceAPI)
	deviceService := NewDeviceService(s.db, s.deviceAPI)
	analyticsService := NewAnalyticsService(s.db)
	// TODO: Add deployment and configuration services when proto definitions are ready
	// deploymentService := NewDeploymentService(s.db, s.deviceAPI)
	// configService := NewConfigurationService(s.db)

	// Register Connect handlers
	fleetPath, fleetHandler := fleetpbconnect.NewFleetServiceHandler(fleetService)
	devicePath, deviceHandler := fleetpbconnect.NewDeviceServiceHandler(deviceService)
	analyticsPath, analyticsHandler := fleetpbconnect.NewAnalyticsServiceHandler(analyticsService)

	// Apply middleware and register routes
	mux.Handle(fleetPath, withMiddleware(fleetHandler, authMiddleware, loggingMiddleware))
	mux.Handle(devicePath, withMiddleware(deviceHandler, authMiddleware, loggingMiddleware))
	mux.Handle(analyticsPath, withMiddleware(analyticsHandler, authMiddleware, loggingMiddleware))

	// Health check endpoints
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/health/live", s.handleHealthLive)
	mux.HandleFunc("/health/ready", s.handleHealthReady)

	// Prometheus metrics endpoint
	mux.Handle("/metrics", promhttp.Handler())

	// Security status endpoint
	mux.HandleFunc("/security", func(w http.ResponseWriter, r *http.Request) {
		devMode := os.Getenv("FLEETD_AUTH_MODE") == "development" ||
			os.Getenv("FLEETD_INSECURE") == "true" ||
			os.Getenv("NODE_ENV") == "development"

		status := map[string]interface{}{
			"authentication_enabled": !devMode,
			"mode":                   "production",
			"warnings":               []string{},
		}

		if devMode {
			status["mode"] = "development"
			status["warnings"] = []string{
				"Authentication is disabled or running in insecure mode",
				"Unauthenticated requests are allowed",
				"DO NOT use this configuration in production",
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(status)
	})

	// Setup CORS
	c := cors.New(cors.Options{
		AllowedOrigins:   []string{"http://localhost:3000", "http://localhost:3001"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"*"},
		AllowCredentials: true,
	})

	// Apply middleware stack
	var handler http.Handler = mux

	// Apply metrics middleware
	handler = metricsMiddleware(handler)

	// Apply rate limiting middleware
	if s.inMemoryLimiter != nil {
		handler = middleware.RateLimitMiddleware(s.inMemoryLimiter)(handler)
	}

	// Apply CORS
	handler = c.Handler(handler)

	// Setup HTTP server
	s.httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.config.Port),
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Setup graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	// Start system metrics collector
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.collectSystemMetrics(ctx)

	go func() {
		slog.Info("Control plane server listening", "port", s.config.Port)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Server error", "error", err)
		}
	}()

	<-stop
	cancel()

	slog.Info("Shutting down control plane server...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
		slog.Error("Server shutdown error", "error", err)
	}

	return nil
}

// withMiddleware wraps a handler with middleware
func withMiddleware(handler http.Handler, middlewares ...func(http.Handler) http.Handler) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return handler
}

// handleHealth returns overall health status
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	health := s.checkHealth()

	w.Header().Set("Content-Type", "application/json")
	if health["status"] != "healthy" {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	json.NewEncoder(w).Encode(health)
}

// handleHealthLive returns liveness status (is the service running)
func (s *Server) handleHealthLive(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "alive",
		"service": "platform-api",
	})
}

// handleHealthReady returns readiness status (can the service handle requests)
func (s *Server) handleHealthReady(w http.ResponseWriter, r *http.Request) {
	// Check database connection
	if err := s.db.Ping(); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "not_ready",
			"checks": map[string]string{
				"database": fmt.Sprintf("unhealthy: %v", err),
			},
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ready",
		"checks": map[string]string{
			"database": "healthy",
		},
	})
}

// checkHealth performs comprehensive health checks
func (s *Server) checkHealth() map[string]interface{} {
	checks := make(map[string]string)
	status := "healthy"

	// Database check
	if err := s.db.Ping(); err != nil {
		checks["database"] = fmt.Sprintf("unhealthy: %v", err)
		status = "unhealthy"
	} else {
		checks["database"] = "healthy"
	}

	// Device API check
	if s.deviceAPI != nil {
		resp, err := http.Get(s.deviceAPI.baseURL + "/health")
		if err != nil {
			checks["device_api"] = fmt.Sprintf("unhealthy: %v", err)
			status = "degraded"
		} else {
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				checks["device_api"] = fmt.Sprintf("unhealthy: status %d", resp.StatusCode)
				status = "degraded"
			} else {
				checks["device_api"] = "healthy"
			}
		}
	}

	// Memory check
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	memoryUsageMB := m.Alloc / 1024 / 1024
	if memoryUsageMB > 500 { // Warning if over 500MB
		checks["memory"] = fmt.Sprintf("warning: %d MB", memoryUsageMB)
		if status == "healthy" {
			status = "degraded"
		}
	} else {
		checks["memory"] = fmt.Sprintf("healthy: %d MB", memoryUsageMB)
	}

	return map[string]interface{}{
		"status":    status,
		"checks":    checks,
		"timestamp": time.Now().Unix(),
		"version":   version.Version,
		"service":   "platform-api",
	}
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
			metrics.SystemUptime.WithLabelValues("platform-api").Set(time.Since(startTime).Seconds())

			// Update memory metrics
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			metrics.SystemMemoryUsage.WithLabelValues("platform-api", "alloc").Set(float64(m.Alloc))
			metrics.SystemMemoryUsage.WithLabelValues("platform-api", "heap").Set(float64(m.HeapAlloc))
			metrics.SystemMemoryUsage.WithLabelValues("platform-api", "sys").Set(float64(m.Sys))

			// Update goroutines count
			metrics.SystemGoroutines.WithLabelValues("platform-api").Set(float64(runtime.NumGoroutine()))

			// Update database connection metrics
			if s.db != nil {
				stats := s.db.Stats()
				metrics.DBConnectionsActive.WithLabelValues("platform-api").Set(float64(stats.OpenConnections))
			}

			// Update fleet metrics (query from database)
			if s.db != nil {
				var fleetsCount int
				s.db.QueryRow("SELECT COUNT(*) FROM fleets").Scan(&fleetsCount)
				metrics.FleetsTotal.Set(float64(fleetsCount))
			}
		}
	}
}

// Close closes the server resources
func (s *Server) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}
