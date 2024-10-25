package main

import (
	"log/slog"
	"net/http"
	"os"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	storagerpc "fleetd.sh/gen/storage/v1/storagev1connect"
	"fleetd.sh/storage"
)

var (
	storagePath = os.Getenv("STORAGE_PATH")
	listenAddr  = os.Getenv("LISTEN_ADDR")
)

func main() {
	// Set default values if environment variables are not set
	if storagePath == "" {
		storagePath = "/tmp/fleet-storage"
	}
	if listenAddr == "" {
		listenAddr = "localhost:50054"
	}

	// Ensure the storage directory exists
	if err := os.MkdirAll(storagePath, 0755); err != nil {
		slog.With("error", err).Error("Failed to create storage directory")
		os.Exit(1)
	}

	// Initialize the storage service
	storageService := storage.NewStorageService(storagePath)

	// Set up the gRPC server
	mux := http.NewServeMux()
	path, handler := storagerpc.NewStorageServiceHandler(storageService)
	mux.Handle(path, handler)

	// Configure the HTTP server
	server := &http.Server{
		Addr:    listenAddr,
		Handler: h2c.NewHandler(mux, &http2.Server{}),
	}

	// Start the server
	slog.With("address", listenAddr, "path", storagePath).Info("Starting storage server")
	if err := server.ListenAndServe(); err != nil {
		slog.With("error", err).Error("Server error")
		os.Exit(1)
	}
}
