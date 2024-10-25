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
	slog.Info("Starting Fleet Daemon", "version", version.GetVersion())

	daemon, err := daemon.NewFleetDaemon()
	if err != nil {
		slog.With("error", err).Error("Failed to initialize daemon")
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := daemon.Start(); err != nil {
			slog.With("error", err).Error("Failed to start daemon")
			cancel()
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

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
