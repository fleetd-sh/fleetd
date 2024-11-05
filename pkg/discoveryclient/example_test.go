package discoveryclient_test

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"fleetd.sh/pkg/discoveryclient"
)

func ExampleClient_ConfigureDevice() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	client := discoveryclient.NewClient("http://localhost:50051", discoveryclient.WithLogger(logger))

	ctx := context.Background()
	device := discoveryclient.Device{
		Name: "temp-device-001",
		ID:   "temp-device-001",
	}
	config := discoveryclient.Configuration{
		APIEndpoint: "http://fleet-api.example.com",
	}
	success, err := client.ConfigureDevice(ctx, device, config)
	if err != nil {
		fmt.Printf("Error configuring device: %v\n", err)
		return
	}

	fmt.Printf("Device configuration result: %v\n", success)
}
