package cmd

import (
	"encoding/json"
	"os"
)

// getAPIPort returns the control API port
func getAPIPort() int {
	// TODO: Read from config or environment
	return 8090
}

// outputJSON outputs data as formatted JSON
func outputJSON(data interface{}) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}
