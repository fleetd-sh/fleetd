package main

import (
	"log/slog"
	"net/http"
	"os"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	metricsrpc "fleetd.sh/gen/metrics/v1/metricsv1connect"
	"fleetd.sh/internal/config"
	"fleetd.sh/metrics"
)

var (
	InfluxURL    = config.GetStringFromEnv("INFLUX_URL", "http://localhost:8086")
	InfluxToken  = config.GetStringFromEnv("INFLUX_TOKEN", "")
	InfluxOrg    = config.GetStringFromEnv("INFLUX_ORG", "")
	InfluxBucket = config.GetStringFromEnv("INFLUX_BUCKET", "")
	ListenAddr   = config.GetStringFromEnv("LISTEN_ADDR", "localhost:50053")
)

func main() {
	if InfluxURL == "" {
		slog.With("error", "INFLUX_URL environment variable is not set").Error("Failed to load config")
		os.Exit(1)
	}
	if InfluxToken == "" {
		slog.With("error", "INFLUX_TOKEN environment variable is not set").Error("Failed to load config")
		os.Exit(1)
	}
	if InfluxOrg == "" {
		slog.With("error", "INFLUX_ORG environment variable is not set").Error("Failed to load config")
		os.Exit(1)
	}
	if InfluxBucket == "" {
		slog.With("error", "INFLUX_BUCKET environment variable is not set").Error("Failed to load config")
		os.Exit(1)
	}
	if ListenAddr == "" {
		slog.With("error", "LISTEN_ADDR environment variable is not set").Error("Failed to load config")
		os.Exit(1)
	}

	client := influxdb2.NewClientWithOptions(InfluxURL, InfluxToken,
		influxdb2.DefaultOptions().SetTLSConfig(nil).SetHTTPRequestTimeout(uint(30*time.Second)))
	defer client.Close()

	metricsService := metrics.NewMetricsService(client, InfluxOrg, InfluxBucket)
	mux := http.NewServeMux()
	path, handler := metricsrpc.NewMetricsServiceHandler(metricsService)
	mux.Handle(path, handler)

	server := &http.Server{
		Addr:         ListenAddr,
		Handler:      h2c.NewHandler(mux, &http2.Server{}),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	slog.With("address", ListenAddr).Info("Starting metrics server")
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.With("error", err).Error("Failed to start server")
		os.Exit(1)
	}
}
