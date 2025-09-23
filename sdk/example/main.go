package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"connectrpc.com/connect"
	fleetpb "fleetd.sh/gen/fleetd/v1"
	"fleetd.sh/sdk"
)

func main() {
	// Create SDK client
	client, err := sdk.NewClient("http://localhost:8090", sdk.Options{
		APIKey:    "your-api-key-here", // In production, use environment variable
		Timeout:   30 * time.Second,
		UserAgent: "fleetd-sdk-example/1.0.0",
	})
	if err != nil {
		log.Fatal("Failed to create client:", err)
	}
	defer client.Close()

	ctx := context.Background()

	// Example: Create a new fleet
	fleet, err := client.CreateFleet(ctx, "Production Fleet", "Main production device fleet", map[string]string{
		"environment": "production",
		"region":      "us-west-2",
	})
	if err != nil {
		log.Fatal("Failed to create fleet:", err)
	}
	fmt.Printf("Created fleet: %s (ID: %s)\n", fleet.Name, fleet.Id)

	// Example: List all fleets
	fleets, err := client.ListFleets(ctx, 10, "")
	if err != nil {
		log.Fatal("Failed to list fleets:", err)
	}
	fmt.Printf("Found %d fleets\n", len(fleets.Fleets))
	for _, f := range fleets.Fleets {
		fmt.Printf("  - %s: %s\n", f.Name, f.Description)
	}

	// Example: List devices by status
	devices, err := client.ListDevices(ctx, "online", 100, "")
	if err != nil {
		log.Fatal("Failed to list devices:", err)
	}
	fmt.Printf("Found %d online devices\n", len(devices.Devices))
	for _, d := range devices.Devices {
		fmt.Printf("  - %s: %s (version: %s)\n", d.Name, d.Type, d.Version)
	}

	// Example: Get metrics for a specific device
	if len(devices.Devices) > 0 {
		deviceID := devices.Devices[0].Id
		metrics, err := client.GetMetrics(ctx, deviceID, []string{"cpu", "memory", "disk"})
		if err != nil {
			log.Fatal("Failed to get metrics:", err)
		}
		fmt.Printf("Device metrics: %+v\n", metrics)
	}

	// Example: Using the raw service clients for advanced operations
	// You can access the underlying Connect-RPC clients directly
	resp, err := client.Fleet.GetFleet(ctx, connect.NewRequest(&fleetpb.GetFleetRequest{
		Id: fleet.Id,
	}))
	if err != nil {
		log.Fatal("Failed to get fleet details:", err)
	}
	fmt.Printf("Fleet details: %+v\n", resp.Msg.Fleet)
}
