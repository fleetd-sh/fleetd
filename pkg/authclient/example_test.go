package authclient_test

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"fleetd.sh/pkg/authclient"
)

func ExampleClient_Authenticate() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	client := authclient.NewClient("http://localhost:50051", authclient.WithLogger(logger))

	ctx := context.Background()
	authenticated, deviceID, err := client.Authenticate(ctx, "example-api-key")
	if err != nil {
		fmt.Printf("Error authenticating: %v\n", err)
		return
	}

	fmt.Printf("Authentication result: Authenticated=%v, DeviceID=%s\n", authenticated, deviceID)
}

func ExampleClient_GenerateAPIKey() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	client := authclient.NewClient("http://localhost:50051", authclient.WithLogger(logger))

	ctx := context.Background()
	apiKey, err := client.GenerateAPIKey(ctx, "example-device-id")
	if err != nil {
		fmt.Printf("Error generating API key: %v\n", err)
		return
	}

	fmt.Printf("Generated API Key: %s\n", apiKey)
}

func ExampleClient_RevokeAPIKey() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	client := authclient.NewClient("http://localhost:50051", authclient.WithLogger(logger))

	ctx := context.Background()
	success, err := client.RevokeAPIKey(ctx, "example-device-id")
	if err != nil {
		fmt.Printf("Error revoking API key: %v\n", err)
		return
	}

	fmt.Printf("API Key revocation result: Success=%v\n", success)
}
