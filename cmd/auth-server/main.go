package main

import (
	"log/slog"
	"net/http"
	"os"

	_ "github.com/tursodatabase/libsql-client-go/libsql"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"fleetd.sh/auth"
	"fleetd.sh/db"
	authrpc "fleetd.sh/gen/auth/v1/authv1connect"
	"fleetd.sh/internal/migrations"
)

var (
	dbURL      = os.Getenv("DATABASE_URL")
	listenAddr = os.Getenv("LISTEN_ADDR")
)

func main() {
	// Load configuration
	if dbURL == "" {
		// create temp file
		f, err := os.CreateTemp("tmp", "auth.db")
		if err != nil {
			slog.With("error", err).Error("failed to create temp file")
			os.Exit(1)
		}
		dbURL = "file:" + f.Name()
		defer os.Remove(f.Name())
	}
	if listenAddr == "" {
		listenAddr = "localhost:50052"
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

	// Initialize AuthService
	authService, err := auth.NewAuthService(d)
	if err != nil {
		slog.Error("Failed to create auth service", "error", err)
		os.Exit(1)
	}

	// Set up the server
	mux := http.NewServeMux()
	path, handler := authrpc.NewAuthServiceHandler(authService)
	mux.Handle(path, handler)

	// Start the server
	slog.Info("Starting auth server", "address", listenAddr)
	err = http.ListenAndServe(
		listenAddr,
		h2c.NewHandler(mux, &http2.Server{}),
	)
	if err != nil {
		slog.Error("Failed to start server", "error", err)
		os.Exit(1)
	}
}
