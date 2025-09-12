package integration

import (
	"context"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	agentpb "fleetd.sh/gen/agent/v1"
	agentrpc "fleetd.sh/gen/agent/v1/agentpbconnect"
	"fleetd.sh/internal/agent"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/emptypb"
)

func TestDiscoveryService(t *testing.T) {
	if testing.Short() || os.Getenv("INTEGRATION") == "" {
		t.Skip("Skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tmpDir := t.TempDir()

	// Start server on random port
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer listener.Close()

	cfg := agent.DefaultConfig()
	cfg.StorageDir = tmpDir
	cfg.ServerURL = listener.Addr().String()
	cfg.RPCPort = 0
	cfg.EnableMDNS = true
	cfg.DisableMDNS = false
	cfg.MDNSPort = 5353

	a := agent.New(cfg)
	if err := a.Start(); err != nil {
		t.Fatalf("Failed to start agent: %v", err)
	}
	defer func() {
		if err := a.Stop(); err != nil {
			t.Errorf("Failed to stop agent: %v", err)
		}
	}()

	// Wait for agent to be ready with timeout
	readyCtx, readyCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer readyCancel()
	if err := a.WaitForReady(readyCtx); err != nil {
		t.Fatalf("Agent failed to become ready: %v", err)
	}

	// Create Connect client
	client := agentrpc.NewDiscoveryServiceClient(
		http.DefaultClient,
		"http://"+a.RPCAddr(),
	)

	// Use context for all RPC calls
	infoResp, err := client.GetDeviceInfo(
		ctx,
		connect.NewRequest(&emptypb.Empty{}),
	)
	if err != nil {
		t.Fatalf("Failed to get device info: %v", err)
	}

	if infoResp.Msg.DeviceInfo.Id == "" {
		t.Error("Expected non-empty device ID")
	}

	// Test ConfigureDevice
	configResp, err := client.ConfigureDevice(
		ctx,
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
		ctx,
		connect.NewRequest(&emptypb.Empty{}),
	)
	if err != nil {
		t.Fatalf("Failed to get device info: %v", err)
	}

	if !infoResp.Msg.DeviceInfo.Configured {
		t.Error("Expected device to be configured")
	}
}
