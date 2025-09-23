package cmd

import (
	"fmt"
	"net"
	"time"
)

// findAvailablePort finds an available port starting from the given port
func findAvailablePort(startPort int) int {
	for port := startPort; port < startPort+100; port++ {
		if isPortAvailable(port) {
			return port
		}
	}
	return 0
}

// isPortAvailable checks if a port is available
func isPortAvailable(port int) bool {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}
	listener.Close()

	// Double-check with a brief delay to avoid race conditions
	time.Sleep(10 * time.Millisecond)

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", port), 100*time.Millisecond)
	if err != nil {
		// Can't connect, port is available
		return true
	}
	conn.Close()
	// Could connect, port is in use
	return false
}

// getServicePorts returns the default and alternative ports for each service
func getServicePorts(service string) (int, []int) {
	switch service {
	case "traefik":
		return 8080, []int{8081, 8082, 8083, 8090, 8091}
	case "postgres":
		return 5432, []int{5433, 5434}
	case "valkey":
		return 6379, []int{6380, 6381}
	case "victoriametrics":
		return 8428, []int{8429, 8430}
	case "loki":
		return 3100, []int{3101, 3102}
	case "platform-api":
		return 8090, []int{8091, 8092}
	case "device-api":
		return 8081, []int{8082, 8083}
	case "studio":
		return 3000, []int{3001, 3002}
	default:
		return 0, nil
	}
}