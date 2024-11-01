package integration

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"fleetd.sh/auth"
	"fleetd.sh/device"
	authrpc "fleetd.sh/gen/auth/v1/authv1connect"
	devicerpc "fleetd.sh/gen/device/v1/devicev1connect"
	metricsrpc "fleetd.sh/gen/metrics/v1/metricsv1connect"
	storagerpc "fleetd.sh/gen/storage/v1/storagev1connect"
	updaterpc "fleetd.sh/gen/update/v1/updatev1connect"
	"fleetd.sh/metrics"
	"fleetd.sh/storage"
	"fleetd.sh/update"
)

func NewTestServer(t *testing.T,
	authService *auth.AuthService,
	deviceService *device.DeviceService,
	metricsService *metrics.MetricsService,
	updateService *update.UpdateService,
	storageService *storage.StorageService,
) *httptest.Server {
	mux := http.NewServeMux()

	// Register all service handlers
	authPath, authHandler := authrpc.NewAuthServiceHandler(authService)
	devicePath, deviceHandler := devicerpc.NewDeviceServiceHandler(deviceService)
	metricsPath, metricsHandler := metricsrpc.NewMetricsServiceHandler(metricsService)
	updatePath, updateHandler := updaterpc.NewUpdateServiceHandler(updateService)
	storagePath, storageHandler := storagerpc.NewStorageServiceHandler(storageService)

	mux.Handle(authPath, authHandler)
	mux.Handle(devicePath, deviceHandler)
	mux.Handle(metricsPath, metricsHandler)
	mux.Handle(updatePath, updateHandler)
	mux.Handle(storagePath, storageHandler)

	return httptest.NewServer(h2c.NewHandler(mux, &http2.Server{}))
}
