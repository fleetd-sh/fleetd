package main

import (
	"log/slog"
	"net/http"
	"os"

	_ "github.com/tursodatabase/libsql-client-go/libsql"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"fleetd.sh/db"
	metricsrpc "fleetd.sh/gen/metrics/v1/metricsv1connect"
	"fleetd.sh/internal/migrations"
	"fleetd.sh/metrics"
)

var (
	dbURL      = os.Getenv("DATABASE_URL")
	listenAddr = os.Getenv("LISTEN_ADDR")
)

func main() {
	if dbURL == "" {
		// create temp file
		f, err := os.CreateTemp("tmp", "metrics.db")
		if err != nil {
			slog.With("error", err).Error("failed to create temp file")
			os.Exit(1)
		}
		dbURL = "file:" + f.Name()
		defer os.Remove(f.Name())
	}

	if listenAddr == "" {
		listenAddr = "localhost:50053"
	}

	d, err := db.New(dbURL)
	if err != nil {
		slog.Error("Failed to open database", "error", err)
		os.Exit(1)
	}
	defer d.Close()

	if err := migrations.MigrateUp(d); err != nil {
		slog.Error("Failed to migrate database", "error", err)
		os.Exit(1)
	}

	metricsService := metrics.NewMetricsService(d)
	mux := http.NewServeMux()
	path, handler := metricsrpc.NewMetricsServiceHandler(metricsService)
	mux.Handle(path, handler)

	slog.Info("Starting metrics server", "address", listenAddr)
	err = http.ListenAndServe(
		listenAddr,
		h2c.NewHandler(mux, &http2.Server{}),
	)
	if err != nil {
		slog.Error("Failed to start server", "error", err)
	}
}
