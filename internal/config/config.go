package config

import (
	"log/slog"
	"os"
	"strconv"
	"time"
)

// GetStringFromEnv retrieves a string value from the environment variables.
// If the key does not exist, it returns the default value.
func GetStringFromEnv(key string, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

// GetIntFromEnv retrieves an integer value from the environment variables.
// If the key does not exist or cannot be converted to an int, it returns the default value and an error.
func GetIntFromEnv(key string, defaultValue int) int {
	if value, exists := os.LookupEnv(key); exists {
		intValue, err := strconv.Atoi(value)
		if err != nil {
			slog.With("key", key).With("value", value).With("error", err).Error("error converting to int, using default value")
			return defaultValue
		}
		return intValue
	}
	return defaultValue
}

// GetDurationFromEnv retrieves a time duration from the environment variables.
// The value should be in a format accepted by time.ParseDuration, like "300ms", "1.5h", or "2h45m".
func GetDurationFromEnv(key string, defaultValue time.Duration) time.Duration {
	if value, exists := os.LookupEnv(key); exists {
		durationValue, err := time.ParseDuration(value)
		if err != nil {
			slog.With("key", key).With("value", value).With("error", err).Error("error parsing to duration, using default value")
			return defaultValue
		}
		return durationValue
	}
	return defaultValue
}

// GetFloatFromEnv retrieves a float value from the environment variables.
// If the key does not exist or cannot be converted to a float, it returns the default value and an error.
func GetFloatFromEnv(key string, defaultValue float64) float64 {
	if value, exists := os.LookupEnv(key); exists {
		floatValue, err := strconv.ParseFloat(value, 64)
		if err != nil {
			slog.With("key", key).With("value", value).With("error", err).Error("error parsing to float, using default value")
			return defaultValue
		}
		return floatValue
	}
	return defaultValue
}
