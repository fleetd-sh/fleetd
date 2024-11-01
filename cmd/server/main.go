package main

import (
	"log/slog"
	"net/http"
	"os"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	_ "github.com/tursodatabase/libsql-client-go/libsql"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"fleetd.sh/auth"
	"fleetd.sh/db"
	"fleetd.sh/device"
	authrpc "fleetd.sh/gen/auth/v1/authv1connect"
	devicerpc "fleetd.sh/gen/device/v1/devicev1connect"
	metricsrpc "fleetd.sh/gen/metrics/v1/metricsv1connect"
	storagerpc "fleetd.sh/gen/storage/v1/storagev1connect"
	updaterpc "fleetd.sh/gen/update/v1/updatev1connect"
	"fleetd.sh/internal/migrations"
	"fleetd.sh/metrics"
	"fleetd.sh/storage"
	"fleetd.sh/update"
)

func main() {
	// Load configuration from environment
	dbURL := getEnv("DATABASE_URL", "file:fleet.db")
	listenAddr := getEnv("LISTEN_ADDR", "localhost:50051")
	storagePath := getEnv("STORAGE_PATH", "storage")
	influxDBURL := getEnv("INFLUXDB_URL", "http://localhost:8086")
	influxDBToken := getEnv("INFLUXDB_TOKEN", "")
	influxDBOrg := getEnv("INFLUXDB_ORG", "fleet")
	influxDBBucket := getEnv("INFLUXDB_BUCKET", "metrics")

	// Set up database
	db, err := db.New(dbURL)
	if err != nil {
		slog.Error("Failed to open database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// Run migrations
	version, dirty, err := migrations.MigrateUp(db)
	if err != nil {
		slog.Error("Failed to run migrations", "error", err)
		os.Exit(1)
	}
	slog.Info("Migrations complete", "version", version, "dirty", dirty)

	influxClient := influxdb2.NewClient(influxDBURL, influxDBToken)

	// Initialize all services
	authService := auth.NewAuthService(db)
	deviceService := device.NewDeviceService(db)
	metricsService := metrics.NewMetricsService(influxClient, influxDBOrg, influxDBBucket)
	updateService := update.NewUpdateService(db)
	storageService := storage.NewStorageService(storagePath)

	// Create mux and register all service handlers
	mux := http.NewServeMux()

	authPath, authHandler := authrpc.NewAuthServiceHandler(authService)
	devicePath, deviceHandler := devicerpc.NewDeviceServiceHandler(deviceService)
	metricsPath, metricsHandler := metricsrpc.NewMetricsServiceHandler(metricsService)
	updatePath, updateHandler := updaterpc.NewUpdateServiceHandler(updateService)
	storagePath, storageHandler := storagerpc.NewStorageServiceHandler(storageService)

	mux.Handle(authPath, authHandler)
	mux.Handle(devicePath, deviceHandler)
	mux.Handle(metricsPath, metricsHandler)
	mux.Handle(updatePath, updateHandler)
	mux.Handle(storagePath, storageHandler)

	// Start server
	slog.Info("Starting server", "address", listenAddr)
	if err := http.ListenAndServe(
		listenAddr,
		h2c.NewHandler(mux, &http2.Server{}),
	); err != nil {
		slog.Error("Server failed", "error", err)
		os.Exit(1)
	}
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
