package main

import (
	"context"
	"fmt"
	"log"
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
		log.Fatalln("Failed to initialize resolver:", err.Error())
	}

	entries := make(chan *zeroconf.ServiceEntry)
	go func(results <-chan *zeroconf.ServiceEntry) {
		for entry := range results {
			log.Println("Found device:", entry.HostName, "at", entry.AddrIPv4)
			configureDevice(entry)
		}
	}(entries)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
	defer cancel()
	err = resolver.Browse(ctx, "_fleet._tcp", "local.", entries)
	if err != nil {
		log.Fatalln("Failed to browse:", err.Error())
	}

	<-ctx.Done()
}

func configureDevice(entry *zeroconf.ServiceEntry) {
	url := fmt.Sprintf("http://%s:%d", entry.AddrIPv4[0], entry.Port)
	client := discoveryrpc.NewDiscoveryServiceClient(
		http.DefaultClient,
		url,
	)

	// TODO: Use actual config
	req := connect.NewRequest(&discoverypb.ConfigureDeviceRequest{
		DeviceId:         "device-001",
		FleetApiUrl:      "https://api.fleet.example.com",
		UpdateServerUrl:  "https://updates.fleet.example.com",
		MetricsServerUrl: "https://metrics.fleet.example.com",
	})

	resp, err := client.ConfigureDevice(context.Background(), req)
	if err != nil {
		log.Printf("Failed to configure device: %v", err)
		return
	}

	if resp.Msg.Success {
		log.Println("Device configured successfully:", resp.Msg.Message)
	} else {
		log.Println("Failed to configure device:", resp.Msg.Message)
	}
}
