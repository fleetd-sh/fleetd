package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// getAPIURL returns the API URL from environment or default
func getAPIURL() string {
	if url := os.Getenv("FLEETD_API_URL"); url != "" {
		return url
	}
	return "http://localhost:8090"
}

// getAPIPort returns the control API port
func getAPIPort() int {
	// Check if we have a full URL in environment
	if url := os.Getenv("FLEETD_API_URL"); url != "" {
		// Extract port from URL if possible
		parts := strings.Split(url, ":")
		if len(parts) >= 3 {
			portStr := strings.TrimPrefix(parts[2], "//")
			if port, err := strconv.Atoi(portStr); err == nil {
				return port
			}
		}
	}
	// TODO: Read from config
	return 8090
}

// outputJSON outputs data as formatted JSON
func outputJSON(data interface{}) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}

// getServerURL returns the server URL from environment or default
func getServerURL() string {
	if url := os.Getenv("FLEETD_SERVER_URL"); url != "" {
		return url
	}
	// Fall back to API URL
	if url := os.Getenv("FLEETD_API_URL"); url != "" {
		return url
	}
	return "http://localhost:8080"
}

// getConfigDir returns the configuration directory
func getConfigDir() (string, error) {
	// Check environment variable
	if dir := os.Getenv("FLEETD_CONFIG_DIR"); dir != "" {
		return dir, nil
	}

	// Use home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	configDir := filepath.Join(homeDir, ".fleetd")

	// Create directory if it doesn't exist
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return "", err
	}

	return configDir, nil
}
