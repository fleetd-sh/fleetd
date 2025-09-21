package control

import (
	"net/http"

	"connectrpc.com/vanguard"
	"fleetd.sh/gen/fleetd/v1/fleetpbconnect"
)

// SetupVanguard creates a Vanguard transcoder for REST API support
func (s *Server) SetupVanguard() (http.Handler, error) {
	// Create service implementations
	fleetService := NewFleetService(s.db, s.deviceAPI)
	deviceService := NewDeviceService(s.db, s.deviceAPI)
	analyticsService := NewAnalyticsService(s.db)

	// Create a mux for the services
	mux := http.NewServeMux()

	// Register Connect handlers
	fleetPath, fleetHandler := fleetpbconnect.NewFleetServiceHandler(fleetService)
	devicePath, deviceHandler := fleetpbconnect.NewDeviceServiceHandler(deviceService)
	analyticsPath, analyticsHandler := fleetpbconnect.NewAnalyticsServiceHandler(analyticsService)

	mux.Handle(fleetPath, fleetHandler)
	mux.Handle(devicePath, deviceHandler)
	mux.Handle(analyticsPath, analyticsHandler)

	// Create Vanguard transcoder with the mux
	// This will handle both Connect-RPC and REST requests
	services := []*vanguard.Service{
		vanguard.NewService(
			fleetPath,
			fleetHandler,
			vanguard.WithTargetProtocols(vanguard.ProtocolConnect, vanguard.ProtocolGRPC, vanguard.ProtocolGRPCWeb),
		),
		vanguard.NewService(
			devicePath,
			deviceHandler,
			vanguard.WithTargetProtocols(vanguard.ProtocolConnect, vanguard.ProtocolGRPC, vanguard.ProtocolGRPCWeb),
		),
		vanguard.NewService(
			analyticsPath,
			analyticsHandler,
			vanguard.WithTargetProtocols(vanguard.ProtocolConnect, vanguard.ProtocolGRPC, vanguard.ProtocolGRPCWeb),
		),
	}

	// Create transcoder with all services
	transcoder, err := vanguard.NewTranscoder(services)
	if err != nil {
		return nil, err
	}

	return transcoder, nil
}

// SetupVanguardWithMiddleware wraps the Vanguard transcoder with middleware
func (s *Server) SetupVanguardWithMiddleware(transcoder *vanguard.Transcoder) http.Handler {
	// Create middleware chain
	handler := http.Handler(transcoder)

	// Apply middlewares in reverse order (innermost first)
	// Add your existing middleware here
	// handler = authMiddleware(handler)
	// handler = loggingMiddleware(handler)
	// handler = metricsMiddleware(handler)

	return handler
}

// createHealthService creates the health service implementation if needed
func (s *Server) createHealthService() interface{} {
	// TODO: Return your health service implementation if you have one
	// This should implement the health.v1.HealthService interface
	return nil
}