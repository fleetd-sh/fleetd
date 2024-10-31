package deviceclient_test

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"fleetd.sh/pkg/deviceclient"
)

func ExampleClient_RegisterDevice() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	client := deviceclient.NewClient("http://localhost:50051", deviceclient.WithLogger(logger))

	ctx := context.Background()
	deviceID, apiKey, err := client.RegisterDevice(ctx, &deviceclient.NewDevice{
		Name:    "Example Device",
		Type:    "SENSOR",
		Version: "v1.0.0",
	})
	if err != nil {
		fmt.Printf("Error registering device: %v\n", err)
		return
	}

	fmt.Printf("Device registered successfully. ID: %s, API Key: %s\n", deviceID, apiKey)
}

func ExampleClient_ListDevices() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	client := deviceclient.NewClient("http://localhost:50051", deviceclient.WithLogger(logger))

	ctx := context.Background()
	deviceCh, errCh := client.ListDevices(ctx)

	for {
		select {
		case device, ok := <-deviceCh:
			if !ok {
				return
			}
			fmt.Printf("Device: ID=%s, Name=%s, Type=%s, Status=%s\n", device.ID, device.Name, device.Type, device.Status)
		case err := <-errCh:
			if err != nil {
				fmt.Printf("Error listing devices: %v\n", err)
				return
			}
		}
	}
}
