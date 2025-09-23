package cmd

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"time"
)

// ServiceStatus represents the status of a service
type ServiceStatus struct {
	Name      string
	Running   bool
	Healthy   bool
	Container string
	Message   string
}

// checkDockerService checks if a Docker container is running
func checkDockerService(serviceName string) ServiceStatus {
	containerName := fmt.Sprintf("fleetd-%s", serviceName)

	// Check if container exists and is running
	cmd := exec.Command("docker", "ps", "--filter", fmt.Sprintf("name=%s", containerName), "--format", "{{.Status}}")
	output, err := cmd.Output()
	if err != nil {
		return ServiceStatus{
			Name:    serviceName,
			Running: false,
			Message: "Container not found",
		}
	}

	status := strings.TrimSpace(string(output))
	if status == "" {
		return ServiceStatus{
			Name:    serviceName,
			Running: false,
			Message: "Container not running",
		}
	}

	// Container is running
	healthy := strings.Contains(strings.ToLower(status), "healthy") || strings.Contains(status, "Up")

	return ServiceStatus{
		Name:      serviceName,
		Running:   true,
		Healthy:   healthy,
		Container: containerName,
		Message:   status,
	}
}

// checkPostgresConnection checks if PostgreSQL is accessible
func checkPostgresConnection() error {
	// Try to connect to PostgreSQL
	host := "localhost"
	port := "5432"

	// First check if port is open
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%s", host, port), 2*time.Second)
	if err != nil {
		return fmt.Errorf("PostgreSQL is not accessible on %s:%s", host, port)
	}
	conn.Close()

	// Try actual database connection
	connStr := fmt.Sprintf("host=%s port=%s user=fleetd dbname=fleetd sslmode=disable", host, port)
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return fmt.Errorf("failed to create database connection: %w", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		// Check if it's a connection refused error
		if strings.Contains(err.Error(), "connection refused") {
			return fmt.Errorf("PostgreSQL is not running. Did you run 'fleetctl start' first?")
		}
		// Check if it's an authentication error (which means the server is running)
		if strings.Contains(err.Error(), "password authentication failed") ||
		   strings.Contains(err.Error(), "FATAL") {
			// Server is running but auth failed - this is OK for our check
			return nil
		}
		return fmt.Errorf("cannot connect to PostgreSQL: %w", err)
	}

	return nil
}

// checkRequiredServices checks if required services are running
func checkRequiredServices(services []string) error {
	var missingServices []string
	var stoppedServices []string

	for _, service := range services {
		status := checkDockerService(service)
		if !status.Running {
			// Check if it's a required service
			if service == "postgres" {
				missingServices = append(missingServices, service)
			} else {
				stoppedServices = append(stoppedServices, service)
			}
		}
	}

	if len(missingServices) > 0 {
		return fmt.Errorf(`required services are not running: %s

Please start the Fleet platform first:
  fleetctl start

Or start specific services:
  fleetctl start --exclude <services-to-skip>

To check service status:
  fleetctl status`, strings.Join(missingServices, ", "))
	}

	if len(stoppedServices) > 0 {
		printWarning("Some services are not running: %s", strings.Join(stoppedServices, ", "))
	}

	return nil
}

// ensureServicesRunning checks that required services are available
func ensureServicesRunning(requirePostgres bool) error {
	if requirePostgres {
		// First check if Docker container is running
		status := checkDockerService("postgres")
		if !status.Running {
			return fmt.Errorf(`PostgreSQL is not running.

To start the Fleet platform:
  fleetctl start

To start only PostgreSQL:
  docker run -d --name fleetd-postgres \
    -p 5432:5432 \
    -e POSTGRES_PASSWORD=fleetd_secret \
    -e POSTGRES_USER=fleetd \
    -e POSTGRES_DB=fleetd \
    postgres:15-alpine

To check service status:
  fleetctl status`)
		}

		// Now check actual connectivity
		if err := checkPostgresConnection(); err != nil {
			return err
		}
	}

	return nil
}

// isFleetRunning checks if the Fleet platform is running
func isFleetRunning() bool {
	// Check for key services
	services := []string{"postgres", "platform-api", "device-api"}
	runningCount := 0

	for _, service := range services {
		status := checkDockerService(service)
		if status.Running {
			runningCount++
		}
	}

	// Consider platform running if at least postgres is up
	return runningCount > 0
}