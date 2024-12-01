package integration

import (
	"context"
	"net/http"
	"testing"
	"time"

	agentpb "fleetd.sh/gen/agent/v1"
	agentrpc "fleetd.sh/gen/agent/v1/agentpbconnect"
	"fleetd.sh/internal/agent"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/emptypb"
)

func TestDiscoveryService(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Start agent with API enabled
	cfg := agent.DefaultConfig()
	cfg.ServerURL = ":8080"

	a := agent.New(cfg)
	if err := a.Start(); err != nil {
		t.Fatalf("Failed to start agent: %v", err)
	}
	defer a.Stop()

	// Give API server time to start
	time.Sleep(100 * time.Millisecond)

	// Create Connect client
	client := agentrpc.NewDiscoveryServiceClient(
		http.DefaultClient,
		"http://localhost:8080",
	)

	// Test GetDeviceInfo
	infoResp, err := client.GetDeviceInfo(
		context.Background(),
		connect.NewRequest(&emptypb.Empty{}),
	)
	if err != nil {
		t.Fatalf("Failed to get device info: %v", err)
	}

	if infoResp.Msg.DeviceId == "" {
		t.Error("Expected non-empty device ID")
	}

	// Test ConfigureDevice
	configResp, err := client.ConfigureDevice(
		context.Background(),
		connect.NewRequest(&agentpb.ConfigureDeviceRequest{
			DeviceName:  "test-device",
			ApiEndpoint: "https://api.example.com",
		}),
	)
	if err != nil {
		t.Fatalf("Failed to configure device: %v", err)
	}

	if !configResp.Msg.Success {
		t.Error("Expected successful configuration")
	}

	if configResp.Msg.ApiKey == "" {
		t.Error("Expected non-empty API key")
	}

	// Verify device is configured
	infoResp, err = client.GetDeviceInfo(
		context.Background(),
		connect.NewRequest(&emptypb.Empty{}),
	)
	if err != nil {
		t.Fatalf("Failed to get device info: %v", err)
	}

	if !infoResp.Msg.Configured {
		t.Error("Expected device to be configured")
	}
}
