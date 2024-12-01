package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"fleetd.sh/internal/agent"
)

func main() {
	// Parse flags first
	disableMDNS := flag.Bool("disable-mdns", false, "Disable mDNS discovery")
	flag.Parse()

	cfg := agent.DefaultConfig()
	cfg.EnableMDNS = !*disableMDNS

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
