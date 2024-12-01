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
)

func TestDaemonService(t *testing.T) {
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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := a.WaitForReady(ctx); err != nil {
		t.Fatalf("Agent failed to become ready: %v", err)
	}

	// Create Connect client
	client := agentrpc.NewDaemonServiceClient(
		http.DefaultClient,
		"http://localhost:8080",
	)

	// Test binary deployment
	testBinary := []byte("#!/bin/sh\necho 'test'")
	_, err := client.DeployBinary(context.Background(), connect.NewRequest(&agentpb.DeployBinaryRequest{
		Name: "test.sh",
		Data: testBinary,
	}))
	if err != nil {
		t.Fatalf("Failed to deploy binary: %v", err)
	}

	// Test binary listing
	listResp, err := client.ListBinaries(context.Background(), connect.NewRequest(&agentpb.ListBinariesRequest{}))
	if err != nil {
		t.Fatalf("Failed to list binaries: %v", err)
	}

	if len(listResp.Msg.Binaries) != 1 {
		t.Fatalf("Expected 1 binary, got %d", len(listResp.Msg.Binaries))
	}

	if listResp.Msg.Binaries[0].Name != "test.sh" {
		t.Errorf("Expected binary name 'test.sh', got %q", listResp.Msg.Binaries[0].Name)
	}

	// Test starting binary
	_, err = client.StartBinary(context.Background(), connect.NewRequest(&agentpb.StartBinaryRequest{
		Name: "test.sh",
	}))
	if err != nil {
		t.Fatalf("Failed to start binary: %v", err)
	}

	// Test stopping binary
	_, err = client.StopBinary(context.Background(), connect.NewRequest(&agentpb.StopBinaryRequest{
		Name: "test.sh",
	}))
	if err != nil {
		t.Fatalf("Failed to stop binary: %v", err)
	}

	// Verify binary is stopped
	listResp, err = client.ListBinaries(context.Background(), connect.NewRequest(&agentpb.ListBinariesRequest{}))
	if err != nil {
		t.Fatalf("Failed to list binaries: %v", err)
	}

	if listResp.Msg.Binaries[0].Status != "stopped" {
		t.Errorf("Expected binary status 'stopped', got %q", listResp.Msg.Binaries[0].Status)
	}
}
