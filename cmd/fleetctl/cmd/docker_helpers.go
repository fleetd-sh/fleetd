package cmd

import (
	"fmt"
	"os/exec"
	"strings"
)

// runDockerCommand runs a docker command and returns combined output on error
func runDockerCommand(args ...string) error {
	cmd := exec.Command("docker", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if len(output) > 0 {
			// Parse Docker error messages
			errorMsg := string(output)

			// Check for common Docker errors
			if strings.Contains(errorMsg, "Error response from daemon") {
				// Extract the actual error message
				lines := strings.Split(errorMsg, "\n")
				for _, line := range lines {
					if strings.Contains(line, "Error") {
						return fmt.Errorf("%s", strings.TrimSpace(line))
					}
				}
			}

			// Check for port already in use
			if strings.Contains(errorMsg, "bind: address already in use") {
				return fmt.Errorf("port already in use. Check if another service is running on the same port")
			}

			// Check for image not found
			if strings.Contains(errorMsg, "Unable to find image") {
				return fmt.Errorf("Docker image not found. The image will be downloaded automatically")
			}

			return fmt.Errorf("%s", strings.TrimSpace(errorMsg))
		}
		return err
	}
	return nil
}

// getDockerError provides user-friendly error messages for Docker failures
func getDockerError(err error, service string) string {
	if err == nil {
		return ""
	}

	errStr := err.Error()

	// Common Docker errors and their solutions
	if strings.Contains(errStr, "Cannot connect to the Docker daemon") {
		return fmt.Sprintf(`Docker is not running. Please start Docker first.

On macOS: Open Docker Desktop
On Linux: sudo systemctl start docker
On Windows: Start Docker Desktop`)
	}

	if strings.Contains(errStr, "permission denied") {
		return fmt.Sprintf(`Permission denied. You may need to run with elevated privileges or add your user to the docker group:

sudo usermod -aG docker $USER
Then log out and back in.`)
	}

	if strings.Contains(errStr, "port already in use") || strings.Contains(errStr, "address already in use") {
		// Try to identify which port
		if strings.Contains(errStr, ":80") || strings.Contains(errStr, ":80:") {
			return "Port 80 is already in use. This might be Apache, nginx, or another web server."
		}
		if strings.Contains(errStr, ":443") {
			return "Port 443 is already in use. This might be another HTTPS service."
		}
		if strings.Contains(errStr, ":8080") {
			return "Port 8080 is already in use. Check for other development servers."
		}
		if strings.Contains(errStr, ":5432") {
			return "Port 5432 is already in use. Another PostgreSQL instance might be running."
		}
		if strings.Contains(errStr, ":6379") {
			return "Port 6379 is already in use. Another Redis/Valkey instance might be running."
		}
		if strings.Contains(errStr, ":3000") {
			return "Port 3000 is already in use. Another web application might be running."
		}

		return fmt.Sprintf(`A required port is already in use.

To find what's using the port:
  lsof -i :<port>  (macOS/Linux)
  netstat -ano | findstr :<port>  (Windows)

To stop all Fleet containers:
  fleetctl stop`)
	}

	if strings.Contains(errStr, "network fleetd-network not found") {
		return "Docker network not found. It will be created automatically."
	}

	// Generic error
	return fmt.Sprintf("Failed to start %s: %v", service, err)
}
