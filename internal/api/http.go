package api

import (
	"fmt"
	"net/http"

	agentrpc "fleetd.sh/gen/agent/v1/agentpbconnect"
	"fleetd.sh/internal/agent"

	"connectrpc.com/grpchealth"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

func NewHTTPServer(agent *agent.Agent, port int) *http.Server {
	mux := http.NewServeMux()

	// Connect handlers
	daemonPath, daemonHandler := agentrpc.NewDaemonServiceHandler(
		NewDaemonService(agent),
	)
	discoveryPath, discoveryHandler := agentrpc.NewDiscoveryServiceHandler(
		NewDiscoveryService(agent),
	)
	mux.Handle(daemonPath, daemonHandler)
	mux.Handle(discoveryPath, discoveryHandler)

	// Add standard health check
	checker := grpchealth.NewStaticChecker(
		"agent.v1.DaemonService",
	)
	mux.Handle(grpchealth.NewHandler(checker))

	return &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: h2c.NewHandler(mux, &http2.Server{}),
	}
}
