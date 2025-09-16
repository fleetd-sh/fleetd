package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all application configuration
type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	Device   DeviceConfig
	API      APIConfig
	SSE      SSEConfig
	Auth     AuthConfig
	Features FeatureConfig
}

// ServerConfig contains server-specific settings
type ServerConfig struct {
	Port         int           `env:"PORT" default:"8080"`
	Host         string        `env:"HOST" default:"0.0.0.0"`
	ReadTimeout  time.Duration `env:"READ_TIMEOUT" default:"30s"`
	WriteTimeout time.Duration `env:"WRITE_TIMEOUT" default:"30s"`
	IdleTimeout  time.Duration `env:"IDLE_TIMEOUT" default:"120s"`
	EnableMDNS   bool          `env:"ENABLE_MDNS" default:"true"`
	MDNSPort     int           `env:"MDNS_PORT" default:"5353"`
}

// DatabaseConfig contains database settings
type DatabaseConfig struct {
	URL             string        `env:"DATABASE_URL" default:"fleet.db"`
	MaxConnections  int           `env:"DB_MAX_CONNECTIONS" default:"25"`
	MaxIdleConns    int           `env:"DB_MAX_IDLE_CONNS" default:"5"`
	ConnMaxLifetime time.Duration `env:"DB_CONN_MAX_LIFETIME" default:"30m"`
}

// DeviceConfig contains device-related settings
type DeviceConfig struct {
	OnlineThreshold   time.Duration `env:"DEVICE_ONLINE_THRESHOLD" default:"5m"`
	HeartbeatInterval time.Duration `env:"DEVICE_HEARTBEAT" default:"30s"`
	HeartbeatTimeout  time.Duration `env:"DEVICE_HEARTBEAT_TIMEOUT" default:"90s"`
	MaxMetadataSize   int           `env:"DEVICE_MAX_METADATA_SIZE" default:"10240"` // 10KB
	CleanupInterval   time.Duration `env:"DEVICE_CLEANUP_INTERVAL" default:"1h"`
	RetentionDays     int           `env:"DEVICE_RETENTION_DAYS" default:"30"`
}

// APIConfig contains API-specific settings
type APIConfig struct {
	MaxPageSize        int32         `env:"API_MAX_PAGE_SIZE" default:"100"`
	DefaultPageSize    int32         `env:"API_DEFAULT_PAGE_SIZE" default:"20"`
	RateLimitRequests  int           `env:"API_RATE_LIMIT_REQUESTS" default:"100"`
	RateLimitWindow    time.Duration `env:"API_RATE_LIMIT_WINDOW" default:"1m"`
	RequestTimeout     time.Duration `env:"API_REQUEST_TIMEOUT" default:"30s"`
	MaxRequestSize     int64         `env:"API_MAX_REQUEST_SIZE" default:"10485760"` // 10MB
	CORSAllowedOrigins []string      `env:"API_CORS_ORIGINS" default:""`
	ValkeyAddr         string        `env:"VALKEY_ADDR" default:""`
}

// SSEConfig contains Server-Sent Events settings
type SSEConfig struct {
	HeartbeatInterval time.Duration `env:"SSE_HEARTBEAT" default:"30s"`
	ClientTimeout     time.Duration `env:"SSE_CLIENT_TIMEOUT" default:"5m"`
	MaxClients        int           `env:"SSE_MAX_CLIENTS" default:"1000"`
	BufferSize        int           `env:"SSE_BUFFER_SIZE" default:"10"`
}

// AuthConfig contains authentication settings
type AuthConfig struct {
	JWTSecret             string        `env:"JWT_SECRET"`
	JWTAccessTTL          time.Duration `env:"JWT_ACCESS_TTL" default:"15m"`
	JWTRefreshTTL         time.Duration `env:"JWT_REFRESH_TTL" default:"168h"` // 7 days
	BCryptCost            int           `env:"BCRYPT_COST" default:"12"`
	SessionTimeout        time.Duration `env:"SESSION_TIMEOUT" default:"30m"`
	MaxConcurrentSessions int           `env:"MAX_CONCURRENT_SESSIONS" default:"5"`
	PasswordMinLength     int           `env:"PASSWORD_MIN_LENGTH" default:"12"`
	RequireStrongPassword bool          `env:"REQUIRE_STRONG_PASSWORD" default:"true"`
}

// FeatureConfig contains feature flags
type FeatureConfig struct {
	EnableSSO          bool `env:"ENABLE_SSO" default:"false"`
	EnableMultiTenancy bool `env:"ENABLE_MULTI_TENANCY" default:"false"`
	EnableBilling      bool `env:"ENABLE_BILLING" default:"false"`
	EnableAnalytics    bool `env:"ENABLE_ANALYTICS" default:"false"`
	EnableAutoUpdate   bool `env:"ENABLE_AUTO_UPDATE" default:"false"`
	DebugMode          bool `env:"DEBUG_MODE" default:"false"`
}

// Load loads configuration from environment variables
func Load() (*Config, error) {
	cfg := &Config{}

	cfg.Server.Port = getEnvInt("PORT", 8080)
	cfg.Server.Host = getEnvString("HOST", "0.0.0.0")
	cfg.Server.ReadTimeout = getEnvDuration("READ_TIMEOUT", 30*time.Second)
	cfg.Server.WriteTimeout = getEnvDuration("WRITE_TIMEOUT", 30*time.Second)
	cfg.Server.IdleTimeout = getEnvDuration("IDLE_TIMEOUT", 120*time.Second)
	cfg.Server.EnableMDNS = getEnvBool("ENABLE_MDNS", true)
	cfg.Server.MDNSPort = getEnvInt("MDNS_PORT", 5353)

	cfg.Database.URL = getEnvString("DATABASE_URL", "fleet.db")
	cfg.Database.MaxConnections = getEnvInt("DB_MAX_CONNECTIONS", 25)
	cfg.Database.MaxIdleConns = getEnvInt("DB_MAX_IDLE_CONNS", 5)
	cfg.Database.ConnMaxLifetime = getEnvDuration("DB_CONN_MAX_LIFETIME", 30*time.Minute)

	cfg.Device.OnlineThreshold = getEnvDuration("DEVICE_ONLINE_THRESHOLD", 5*time.Minute)
	cfg.Device.HeartbeatInterval = getEnvDuration("DEVICE_HEARTBEAT", 30*time.Second)
	cfg.Device.HeartbeatTimeout = getEnvDuration("DEVICE_HEARTBEAT_TIMEOUT", 90*time.Second)
	cfg.Device.MaxMetadataSize = getEnvInt("DEVICE_MAX_METADATA_SIZE", 10240)
	cfg.Device.CleanupInterval = getEnvDuration("DEVICE_CLEANUP_INTERVAL", time.Hour)
	cfg.Device.RetentionDays = getEnvInt("DEVICE_RETENTION_DAYS", 30)

	cfg.API.MaxPageSize = int32(getEnvInt("API_MAX_PAGE_SIZE", 100))
	cfg.API.DefaultPageSize = int32(getEnvInt("API_DEFAULT_PAGE_SIZE", 20))
	cfg.API.RateLimitRequests = getEnvInt("API_RATE_LIMIT_REQUESTS", 100)
	cfg.API.RateLimitWindow = getEnvDuration("API_RATE_LIMIT_WINDOW", time.Minute)
	cfg.API.RequestTimeout = getEnvDuration("API_REQUEST_TIMEOUT", 30*time.Second)
	cfg.API.MaxRequestSize = int64(getEnvInt("API_MAX_REQUEST_SIZE", 10485760))
	cfg.API.ValkeyAddr = getEnvString("VALKEY_ADDR", "")
	corsOrigins := getEnvString("API_CORS_ORIGINS", "")
	if corsOrigins != "" {
		cfg.API.CORSAllowedOrigins = []string{corsOrigins}
	} else {
		cfg.API.CORSAllowedOrigins = []string{}
	}

	cfg.SSE.HeartbeatInterval = getEnvDuration("SSE_HEARTBEAT", 30*time.Second)
	cfg.SSE.ClientTimeout = getEnvDuration("SSE_CLIENT_TIMEOUT", 5*time.Minute)
	cfg.SSE.MaxClients = getEnvInt("SSE_MAX_CLIENTS", 1000)
	cfg.SSE.BufferSize = getEnvInt("SSE_BUFFER_SIZE", 10)

	cfg.Auth.JWTSecret = getEnvString("JWT_SECRET", "")
	if cfg.Auth.JWTSecret == "" && !cfg.Features.DebugMode {
		return nil, fmt.Errorf("JWT_SECRET is required in production")
	}
	cfg.Auth.JWTAccessTTL = getEnvDuration("JWT_ACCESS_TTL", 15*time.Minute)
	cfg.Auth.JWTRefreshTTL = getEnvDuration("JWT_REFRESH_TTL", 168*time.Hour)
	cfg.Auth.BCryptCost = getEnvInt("BCRYPT_COST", 12)
	cfg.Auth.SessionTimeout = getEnvDuration("SESSION_TIMEOUT", 30*time.Minute)
	cfg.Auth.MaxConcurrentSessions = getEnvInt("MAX_CONCURRENT_SESSIONS", 5)
	cfg.Auth.PasswordMinLength = getEnvInt("PASSWORD_MIN_LENGTH", 12)
	cfg.Auth.RequireStrongPassword = getEnvBool("REQUIRE_STRONG_PASSWORD", true)

	cfg.Features.EnableSSO = getEnvBool("ENABLE_SSO", false)
	cfg.Features.EnableMultiTenancy = getEnvBool("ENABLE_MULTI_TENANCY", false)
	cfg.Features.EnableBilling = getEnvBool("ENABLE_BILLING", false)
	cfg.Features.EnableAnalytics = getEnvBool("ENABLE_ANALYTICS", false)
	cfg.Features.EnableAutoUpdate = getEnvBool("ENABLE_AUTO_UPDATE", false)
	cfg.Features.DebugMode = getEnvBool("DEBUG_MODE", false)

	return cfg, cfg.Validate()
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid port: %d", c.Server.Port)
	}

	if c.Database.MaxConnections < 1 {
		return fmt.Errorf("invalid max connections: %d", c.Database.MaxConnections)
	}

	if c.API.MaxPageSize < 1 {
		return fmt.Errorf("invalid max page size: %d", c.API.MaxPageSize)
	}

	if c.Auth.BCryptCost < 10 || c.Auth.BCryptCost > 31 {
		return fmt.Errorf("invalid bcrypt cost: %d (must be 10-31)", c.Auth.BCryptCost)
	}

	return nil
}

func getEnvString(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolVal, err := strconv.ParseBool(value); err == nil {
			return boolVal
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}
