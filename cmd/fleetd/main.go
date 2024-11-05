package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"fleetd.sh/daemon"
	"fleetd.sh/internal/version"
)

func main() {
	slog.With("version", version.GetVersion()).Info("Starting Fleet Daemon")

	cfg, err := daemon.LoadConfig()
	if err != nil {
		slog.With("error", err).Error("Failed to load config")
		os.Exit(1)
	}

	daemon, err := daemon.NewFleetDaemon(cfg)
	if err != nil {
		slog.With("error", err).Error("Failed to initialize daemon")
		os.Exit(1)
	}

	// Set up signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the daemon in a goroutine
	errChan := make(chan error, 1)
	go func() {
		if err := daemon.Start(); err != nil {
			errChan <- err
		}
	}()

	// Wait for either an error or a signal
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errChan:
		slog.With("error", err).Error("Daemon failed")
		os.Exit(1)
	case s := <-sig:
		slog.Info("Received signal", "signal", s)
	}

	// Graceful shutdown
	slog.Info("Shutting down...")
	shutdownCtx, shutdownCancel := context.WithTimeout(ctx, 10*time.Second)
	defer shutdownCancel()

	daemon.Stop()

	select {
	case <-shutdownCtx.Done():
		slog.Error("Shutdown timed out")
	default:
		slog.Info("Shutdown complete")
	}
}
