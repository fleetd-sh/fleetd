package config

import (
	"fmt"
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
func GetIntFromEnv(key string, defaultValue int) (int, error) {
	if value, exists := os.LookupEnv(key); exists {
		intValue, err := strconv.Atoi(value)
		if err != nil {
			return defaultValue, fmt.Errorf("error converting %s to int: %v", key, err)
		}
		return intValue, nil
	}
	return defaultValue, nil
}

// GetDurationFromEnv retrieves a time duration from the environment variables.
// The value should be in a format accepted by time.ParseDuration, like "300ms", "1.5h", or "2h45m".
func GetDurationFromEnv(key string, defaultValue time.Duration) (time.Duration, error) {
	if value, exists := os.LookupEnv(key); exists {
		durationValue, err := time.ParseDuration(value)
		if err != nil {
			return defaultValue, fmt.Errorf("error parsing %s as duration: %v", key, err)
		}
		return durationValue, nil
	}
	return defaultValue, nil
}

// GetFloatFromEnv retrieves a float value from the environment variables.
// If the key does not exist or cannot be converted to a float, it returns the default value and an error.
func GetFloatFromEnv(key string, defaultValue float64) (float64, error) {
	if value, exists := os.LookupEnv(key); exists {
		floatValue, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return defaultValue, fmt.Errorf("error parsing %s as float: %v", key, err)
		}
		return floatValue, nil
	}
	return defaultValue, nil
}
