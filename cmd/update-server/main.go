package main

import (
	"log/slog"
	"net/http"
	"os"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"fleetd.sh/db"
	updaterpc "fleetd.sh/gen/update/v1/updatev1connect"
	"fleetd.sh/internal/migrations"
	"fleetd.sh/update"
)

var (
	dbURL      = os.Getenv("DB_URL")
	listenAddr = os.Getenv("LISTEN_ADDR")
)

func main() {
	// Set default values if environment variables are not set
	if dbURL == "" {
		dbURL = "file:update.db?cache=shared&mode=rwc"
	}
	if listenAddr == "" {
		listenAddr = "localhost:50055"
	}

	// Open the database connection
	d, err := db.New(dbURL)
	if err != nil {
		slog.With("error", err).Error("Failed to open database")
		os.Exit(1)
	}
	defer d.Close()

	// Run migrations
	if _, _, err := migrations.MigrateUp(d); err != nil {
		slog.With("error", err).Error("Failed to run migrations")
		os.Exit(1)
	}

	// Initialize the update service
	updateService := update.NewUpdateService(d)

	// Set up the gRPC server
	mux := http.NewServeMux()
	path, handler := updaterpc.NewUpdateServiceHandler(updateService)
	mux.Handle(path, handler)

	// Configure the HTTP server
	server := &http.Server{
		Addr:    listenAddr,
		Handler: h2c.NewHandler(mux, &http2.Server{}),
	}

	// Start the server
	slog.With("address", listenAddr).Info("Starting update server")
	if err := server.ListenAndServe(); err != nil {
		slog.With("error", err).Error("Server error")
		os.Exit(1)
	}
}
