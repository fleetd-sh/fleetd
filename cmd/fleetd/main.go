package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"fleetd.sh/internal/agent"
)

func main() {
	// Parse flags first
	cfg := agent.ParseFlags()

	a := agent.New(cfg)
	if err := a.Start(); err != nil {
		log.Fatalf("Failed to start agent: %v", err)
	}

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	if err := a.Stop(); err != nil {
		log.Printf("Error stopping agent: %v", err)
	}
}
