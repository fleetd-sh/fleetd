package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"github.com/grandcat/zeroconf"

	discoverypb "fleetd.sh/gen/discovery/v1"
	discoveryrpc "fleetd.sh/gen/discovery/v1/discoveryv1connect"
)

func main() {
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		slog.Error("Failed to initialize resolver", "error", err)
	}

	entries := make(chan *zeroconf.ServiceEntry)
	go func(results <-chan *zeroconf.ServiceEntry) {
		for entry := range results {
			slog.With("host_name", entry.HostName, "ip", entry.AddrIPv4).Info("Found device")
			configureDevice(entry)
		}
	}(entries)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
	defer cancel()
	err = resolver.Browse(ctx, "_fleet._tcp", "local.", entries)
	if err != nil {
		slog.Error("Failed to browse", "error", err)
	}

	<-ctx.Done()
}

func configureDevice(entry *zeroconf.ServiceEntry) {
	url := fmt.Sprintf("http://%s:%d", entry.AddrIPv4[0], entry.Port)
	client := discoveryrpc.NewDiscoveryServiceClient(
		http.DefaultClient,
		url,
	)

	// TODO: apply actual config
	req := connect.NewRequest(&discoverypb.ConfigureDeviceRequest{
		DeviceName:  "device-001",
		ApiEndpoint: "http://192.168.1.146:50051",
	})

	resp, err := client.ConfigureDevice(context.Background(), req)
	if err != nil {
		slog.With("error", err).Error("Failed to configure device")
		return
	}

	if resp.Msg.Success {
		slog.Info("Device configured successfully", "message", resp.Msg.Message)
	} else {
		slog.With("message", resp.Msg.Message).Error("Failed to configure device")
	}
}
