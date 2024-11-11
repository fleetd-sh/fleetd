package integration

import (
	"context"
	"fmt"
	"net"
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
	cfg.DisableMDNS = true

	a := agent.New(cfg)
	if err := a.Start(); err != nil {
		t.Fatalf("Failed to start agent: %v", err)
	}
	defer a.Stop()

	// Wait for RPC server to be ready with a shorter timeout
	readyCtx, readyCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer readyCancel()
	if err := a.WaitForReady(readyCtx); err != nil {
		t.Fatalf("Agent failed to become ready: %v", err)
	}

	// Get the actual RPC address after server starts
	rpcAddr := fmt.Sprintf("http://%s", a.RPCAddr())
	if rpcAddr == "" {
		t.Fatal("Failed to get RPC server address")
	}

	// Create Connect client with proper address
	client := agentrpc.NewDaemonServiceClient(
		http.DefaultClient,
		rpcAddr,
	)

	// Test ListBinaries with timeout
	listCtx, listCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer listCancel()

	resp, err := client.ListBinaries(listCtx, connect.NewRequest(&agentpb.ListBinariesRequest{}))
	if err != nil {
		t.Fatalf("Failed to list binaries: %v", err)
	}

	// Initially should be empty
	if len(resp.Msg.Binaries) != 0 {
		t.Errorf("Expected no binaries, got %d", len(resp.Msg.Binaries))
	}
}
