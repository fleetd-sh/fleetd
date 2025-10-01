package config

import (
	"time"

	"fleetd.sh/internal/middleware"
)

// RateLimitingConfig defines rate limiting configuration
type RateLimitingConfig struct {
	// Enable rate limiting
	Enabled bool `yaml:"enabled" json:"enabled" env:"RATELIMIT_ENABLED" default:"true"`

	// Global rate limits
	Global GlobalRateLimits `yaml:"global" json:"global"`

	// Device-specific rate limits
	Device DeviceRateLimits `yaml:"device" json:"device"`

	// Device type specific limits (e.g., constrained devices, full devices)
	DeviceTypes map[string]DeviceTypeLimit `yaml:"device_types" json:"device_types"`

	// Endpoint-specific configurations
	Endpoints []EndpointConfig `yaml:"endpoints" json:"endpoints"`

	// DDoS protection settings
	DDoS DDoSConfig `yaml:"ddos" json:"ddos"`

	// Circuit breaker settings
	CircuitBreaker CircuitBreakerConfig `yaml:"circuit_breaker" json:"circuit_breaker"`

	// Maintenance settings
	Maintenance MaintenanceConfig `yaml:"maintenance" json:"maintenance"`
}

// GlobalRateLimits defines global rate limiting settings
type GlobalRateLimits struct {
	RequestsPerSecond int `yaml:"requests_per_second" json:"requests_per_second" env:"RATELIMIT_GLOBAL_RPS" default:"100"`
	BurstSize         int `yaml:"burst_size" json:"burst_size" env:"RATELIMIT_GLOBAL_BURST" default:"200"`
}

// DeviceRateLimits defines device-specific rate limiting
type DeviceRateLimits struct {
	RequestsPerMinute int `yaml:"requests_per_minute" json:"requests_per_minute" env:"RATELIMIT_DEVICE_RPM" default:"60"`
	BurstSize         int `yaml:"burst_size" json:"burst_size" env:"RATELIMIT_DEVICE_BURST" default:"10"`

	// Metrics endpoint has higher limits
	MetricsRPM   int `yaml:"metrics_rpm" json:"metrics_rpm" env:"RATELIMIT_DEVICE_METRICS_RPM" default:"120"`
	MetricsBurst int `yaml:"metrics_burst" json:"metrics_burst" env:"RATELIMIT_DEVICE_METRICS_BURST" default:"20"`
}

// DeviceTypeLimit defines rate limits for specific device types
type DeviceTypeLimit struct {
	Type              string `yaml:"type" json:"type"` // "full", "constrained", "minimal"
	RequestsPerMinute int    `yaml:"requests_per_minute" json:"requests_per_minute"`
	BurstSize         int    `yaml:"burst_size" json:"burst_size"`
	Description       string `yaml:"description" json:"description"`
}

// EndpointConfig defines rate limits for specific endpoints
type EndpointConfig struct {
	Path              string   `yaml:"path" json:"path"`
	Methods           []string `yaml:"methods" json:"methods"`
	RequestsPerSecond int      `yaml:"requests_per_second" json:"requests_per_second"`
	BurstSize         int      `yaml:"burst_size" json:"burst_size"`
	Description       string   `yaml:"description" json:"description"`
}

// DDoSConfig defines DDoS protection settings
type DDoSConfig struct {
	Enabled              bool          `yaml:"enabled" json:"enabled" env:"DDOS_PROTECTION_ENABLED" default:"true"`
	MaxConnectionsPerIP  int           `yaml:"max_connections_per_ip" json:"max_connections_per_ip" env:"DDOS_MAX_CONN_PER_IP" default:"100"`
	MaxRequestsPerIPPerMin int         `yaml:"max_requests_per_ip_per_min" json:"max_requests_per_ip_per_min" env:"DDOS_MAX_REQ_PER_IP" default:"1000"`
	BanDuration          time.Duration `yaml:"ban_duration" json:"ban_duration" env:"DDOS_BAN_DURATION" default:"15m"`

	// Whitelist IPs that bypass DDoS protection
	WhitelistedIPs []string `yaml:"whitelisted_ips" json:"whitelisted_ips"`

	// Blacklist IPs that are always blocked
	BlacklistedIPs []string `yaml:"blacklisted_ips" json:"blacklisted_ips"`
}

// CircuitBreakerConfig defines circuit breaker settings
type CircuitBreakerConfig struct {
	Enabled         bool          `yaml:"enabled" json:"enabled" env:"CIRCUIT_BREAKER_ENABLED" default:"true"`
	ErrorThreshold  int           `yaml:"error_threshold" json:"error_threshold" env:"CIRCUIT_BREAKER_ERROR_THRESHOLD" default:"50"`
	ErrorWindow     time.Duration `yaml:"error_window" json:"error_window" env:"CIRCUIT_BREAKER_ERROR_WINDOW" default:"1m"`
	RecoveryTimeout time.Duration `yaml:"recovery_timeout" json:"recovery_timeout" env:"CIRCUIT_BREAKER_RECOVERY_TIMEOUT" default:"30s"`
}

// MaintenanceConfig defines rate limiter maintenance settings
type MaintenanceConfig struct {
	CleanupInterval time.Duration `yaml:"cleanup_interval" json:"cleanup_interval" env:"RATELIMIT_CLEANUP_INTERVAL" default:"1m"`
	VisitorTimeout  time.Duration `yaml:"visitor_timeout" json:"visitor_timeout" env:"RATELIMIT_VISITOR_TIMEOUT" default:"3m"`
}

// DefaultRateLimitConfig returns default rate limiting configuration
func DefaultRateLimitConfig() *RateLimitingConfig {
	return &RateLimitingConfig{
		Enabled: true,
		Global: GlobalRateLimits{
			RequestsPerSecond: 100,
			BurstSize:         200,
		},
		Device: DeviceRateLimits{
			RequestsPerMinute: 60,
			BurstSize:         10,
			MetricsRPM:        120,
			MetricsBurst:      20,
		},
		DeviceTypes: map[string]DeviceTypeLimit{
			"full": {
				Type:              "full",
				RequestsPerMinute: 120,
				BurstSize:         20,
				Description:       "Full-featured devices (servers, powerful edge devices)",
			},
			"constrained": {
				Type:              "constrained",
				RequestsPerMinute: 60,
				BurstSize:         10,
				Description:       "Resource-constrained devices (Raspberry Pi, small IoT)",
			},
			"minimal": {
				Type:              "minimal",
				RequestsPerMinute: 30,
				BurstSize:         5,
				Description:       "Minimal devices (sensors, microcontrollers)",
			},
		},
		Endpoints: []EndpointConfig{
			{
				Path:              "/api/v1/auth/login",
				Methods:           []string{"POST"},
				RequestsPerSecond: 5,
				BurstSize:         10,
				Description:       "Login endpoint with stricter limits",
			},
			{
				Path:              "/api/v1/devices/register",
				Methods:           []string{"POST"},
				RequestsPerSecond: 10,
				BurstSize:         20,
				Description:       "Device registration endpoint",
			},
			{
				Path:              "/api/v1/metrics",
				Methods:           []string{"POST"},
				RequestsPerSecond: 1000,
				BurstSize:         2000,
				Description:       "High-volume metrics ingestion",
			},
			{
				Path:              "/api/v1/updates/download",
				Methods:           []string{"GET"},
				RequestsPerSecond: 20,
				BurstSize:         40,
				Description:       "Update download endpoint",
			},
			{
				Path:              "/api/v1/logs",
				Methods:           []string{"POST"},
				RequestsPerSecond: 500,
				BurstSize:         1000,
				Description:       "Log ingestion endpoint",
			},
		},
		DDoS: DDoSConfig{
			Enabled:                true,
			MaxConnectionsPerIP:    100,
			MaxRequestsPerIPPerMin: 1000,
			BanDuration:            15 * time.Minute,
			WhitelistedIPs:         []string{},
			BlacklistedIPs:         []string{},
		},
		CircuitBreaker: CircuitBreakerConfig{
			Enabled:         true,
			ErrorThreshold:  50,
			ErrorWindow:     1 * time.Minute,
			RecoveryTimeout: 30 * time.Second,
		},
		Maintenance: MaintenanceConfig{
			CleanupInterval: 1 * time.Minute,
			VisitorTimeout:  3 * time.Minute,
		},
	}
}

// ToMiddlewareConfig converts to middleware configuration
func (c *RateLimitingConfig) ToMiddlewareConfig() middleware.RateLimitConfig {
	config := middleware.RateLimitConfig{
		RequestsPerSecond:      c.Global.RequestsPerSecond,
		BurstSize:              c.Global.BurstSize,
		DeviceRequestsPerMinute: c.Device.RequestsPerMinute,
		DeviceBurstSize:        c.Device.BurstSize,
		MaxConnectionsPerIP:    c.DDoS.MaxConnectionsPerIP,
		MaxRequestsPerIPPerMin: c.DDoS.MaxRequestsPerIPPerMin,
		BanDuration:            c.DDoS.BanDuration,
		ErrorThreshold:         c.CircuitBreaker.ErrorThreshold,
		ErrorWindow:            c.CircuitBreaker.ErrorWindow,
		RecoveryTimeout:        c.CircuitBreaker.RecoveryTimeout,
		CleanupInterval:        c.Maintenance.CleanupInterval,
		VisitorTimeout:         c.Maintenance.VisitorTimeout,
		EndpointLimits:         make(map[string]middleware.EndpointLimit),
		APIKeyLimits:           make(map[string]middleware.APIKeyLimit),
	}

	// Convert endpoint limits
	for _, endpoint := range c.Endpoints {
		config.EndpointLimits[endpoint.Path] = middleware.EndpointLimit{
			Path:              endpoint.Path,
			RequestsPerSecond: endpoint.RequestsPerSecond,
			BurstSize:         endpoint.BurstSize,
			Methods:           endpoint.Methods,
		}
	}

	// Note: API key limits would be populated based on actual authentication
	// For an open-source project, this might be based on:
	// - Authenticated vs unauthenticated requests
	// - Admin vs regular user roles
	// - Internal services vs external clients

	return config
}

// ProductionRateLimitConfig returns production-optimized configuration
func ProductionRateLimitConfig() *RateLimitingConfig {
	config := DefaultRateLimitConfig()

	// Conservative limits for production
	config.Global.RequestsPerSecond = 500
	config.Global.BurstSize = 1000

	// Stricter device limits to prevent misbehaving devices
	config.Device.RequestsPerMinute = 30
	config.Device.BurstSize = 5

	// Enhanced DDoS protection
	config.DDoS.MaxConnectionsPerIP = 50
	config.DDoS.MaxRequestsPerIPPerMin = 500
	config.DDoS.BanDuration = 30 * time.Minute

	// More sensitive circuit breaker
	config.CircuitBreaker.ErrorThreshold = 20
	config.CircuitBreaker.RecoveryTimeout = 1 * time.Minute

	return config
}

// DevelopmentRateLimitConfig returns development-friendly configuration
func DevelopmentRateLimitConfig() *RateLimitingConfig {
	config := DefaultRateLimitConfig()

	// Relaxed limits for development
	config.Global.RequestsPerSecond = 10000
	config.Global.BurstSize = 20000

	// Very relaxed device limits
	config.Device.RequestsPerMinute = 1000
	config.Device.BurstSize = 100

	// Minimal DDoS protection
	config.DDoS.Enabled = false

	// Disabled circuit breaker
	config.CircuitBreaker.Enabled = false

	return config
}