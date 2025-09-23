package control

import (
	"net/http"

	"connectrpc.com/vanguard"
	"fleetd.sh/gen/fleetd/v1/fleetpbconnect"
	"fleetd.sh/internal/database"
	"fleetd.sh/internal/services"
)

// SetupVanguard creates a Vanguard transcoder for REST API support
func (s *Server) SetupVanguard() (http.Handler, error) {
	// Create JWT manager for auth service (when needed)
	// jwtManager, err := security.NewJWTManager(&security.JWTConfig{
	//	SigningKey:      []byte(s.config.SecretKey),
	//	Issuer:          "fleetd",
	//	AccessTokenTTL:  1 * time.Hour,
	//	RefreshTokenTTL: 24 * time.Hour * 7,
	// })
	// if err != nil {
	//	return nil, err
	// }

	// Create database instance for services
	dbWrapper := &database.DB{DB: s.db}

	// Create service implementations
	fleetService := NewFleetService(s.db, s.deviceAPI)
	deviceService := NewDeviceService(s.db, s.deviceAPI)
	analyticsService := NewAnalyticsService(s.db)
	// authService := NewAuthService(s.db, jwtManager) // TODO: Add auth service to Vanguard
	telemetryService := services.NewTelemetryService(dbWrapper)
	settingsService := services.NewSettingsService(dbWrapper)

	// Create a mux for the services
	mux := http.NewServeMux()

	// Register Connect handlers
	fleetPath, fleetHandler := fleetpbconnect.NewFleetServiceHandler(fleetService)
	devicePath, deviceHandler := fleetpbconnect.NewDeviceServiceHandler(deviceService)
	analyticsPath, analyticsHandler := fleetpbconnect.NewAnalyticsServiceHandler(analyticsService)
	telemetryPath, telemetryHandler := fleetpbconnect.NewTelemetryServiceHandler(telemetryService)
	settingsPath, settingsHandler := fleetpbconnect.NewSettingsServiceHandler(settingsService)

	mux.Handle(fleetPath, fleetHandler)
	mux.Handle(devicePath, deviceHandler)
	mux.Handle(analyticsPath, analyticsHandler)
	mux.Handle(telemetryPath, telemetryHandler)
	mux.Handle(settingsPath, settingsHandler)

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
		vanguard.NewService(
			telemetryPath,
			telemetryHandler,
			vanguard.WithTargetProtocols(vanguard.ProtocolConnect, vanguard.ProtocolGRPC, vanguard.ProtocolGRPCWeb),
		),
		vanguard.NewService(
			settingsPath,
			settingsHandler,
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
