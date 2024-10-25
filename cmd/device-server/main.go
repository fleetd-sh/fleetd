package main

import (
	"log/slog"
	"net/http"
	"os"

	_ "github.com/tursodatabase/libsql-client-go/libsql"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"fleetd.sh/db"
	"fleetd.sh/device"
	devicerpc "fleetd.sh/gen/device/v1/devicev1connect"
	"fleetd.sh/internal/migrations"
	"fleetd.sh/pkg/authclient"
)

var (
	dbURL          = os.Getenv("DATABASE_URL")
	authBaseURL    = os.Getenv("AUTH_BASE_URL")
	migrationsPath = os.Getenv("MIGRATIONS_PATH")
	listenAddr     = os.Getenv("LISTEN_ADDR")
)

func main() {
	// Load configuration (you may want to use environment variables or a config file)
	if dbURL == "" {
		if dbURL == "" {
			// create temp file
			f, err := os.CreateTemp("tmp", "device.db")
			if err != nil {
				slog.With("error", err).Error("failed to create temp file")
				os.Exit(1)
			}
			dbURL = "file:" + f.Name()
			defer os.Remove(f.Name())
		}
	}
	if authBaseURL == "" {
		authBaseURL = "http://localhost:8081"
	}
	if migrationsPath == "" {
		migrationsPath = "migrations"
	}
	if listenAddr == "" {
		listenAddr = "localhost:50051"
	}

	d, err := db.New(dbURL)
	if err != nil {
		slog.Error("Failed to open database", "error", err)
		os.Exit(1)
	}

	// Run migrations
	if err := migrations.MigrateUp(d); err != nil {
		slog.Error("Failed to run migrations", "error", err)
		os.Exit(1)
	}

	authClient := authclient.NewClient(authBaseURL)

	// Initialize DeviceService
	deviceService := device.NewDeviceService(d, authClient)
	if err != nil {
		slog.Error("Failed to create device service", "error", err)
		os.Exit(1)
	}

	// Set up the server
	mux := http.NewServeMux()
	path, handler := devicerpc.NewDeviceServiceHandler(deviceService)
	mux.Handle(path, handler)

	// Start the server
	slog.Info("Starting device server", "address", listenAddr)
	err = http.ListenAndServe(
		listenAddr,
		h2c.NewHandler(mux, &http2.Server{}),
	)
	if err != nil {
		slog.Error("Failed to start server", "error", err)
		os.Exit(1)
	}
}
