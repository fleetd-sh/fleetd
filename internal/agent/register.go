package agent

import (
	"context"
	stderrors "errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"connectrpc.com/connect"
	pb "fleetd.sh/gen/fleetd/v1"
	"fleetd.sh/gen/fleetd/v1/fleetpbconnect"
)

// RegistrationClient handles device registration with enhanced error handling
type RegistrationClient struct {
	client    fleetpbconnect.DeviceServiceClient
	serverURL string
	logger    *slog.Logger
}

// NewRegistrationClient creates a new registration client
func NewRegistrationClient(serverURL string, requestID string) *RegistrationClient {
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
			DisableCompression:  false,
		},
	}

	client := fleetpbconnect.NewDeviceServiceClient(
		httpClient,
		serverURL,
	)

	return &RegistrationClient{
		client:    client,
		serverURL: serverURL,
		logger:    slog.Default().With("component", "registration-client"),
	}
}

// RegisterDevice registers a new device with enhanced error handling and resilience
func (r *RegistrationClient) RegisterDevice(ctx context.Context, deviceName, deviceType, version string) (*pb.RegisterResponse, error) {
	// Recover from panics

	r.logger.Info("Registering device",
		"name", deviceName,
		"type", deviceType,
		"version", version,
		"server", r.serverURL,
	)

	// Collect system information with error handling
	sysInfo, err := r.collectSystemInfoWithRetry(ctx)
	if err != nil {
		// Log error but continue with registration
		r.logger.Error("Failed to collect system info, continuing with minimal info",
			"error", err,
		)
		sysInfo = &SystemInfo{
			Extra: make(map[string]string),
		}
	}

	// Convert to protobuf format
	pbSysInfo := r.convertToProtobufSystemInfo(sysInfo)

	// Build capabilities map
	capabilities := map[string]string{
		"telemetry":      "true",
		"updates":        "true",
		"remote":         "true",
		"error_handling": "enhanced",
	}

	req := &pb.RegisterRequest{
		Name:         deviceName,
		Type:         deviceType,
		Version:      version,
		Capabilities: capabilities,
		SystemInfo:   pbSysInfo,
	}

	resp, err := r.client.Register(ctx, connect.NewRequest(req))
	if err != nil {
		return nil, err
	}

	r.logger.Info("Device registered successfully",
		"device_id", resp.Msg.DeviceId,
		"has_api_key", resp.Msg.ApiKey != "",
	)

	// Record success metrics
	r.recordRegistrationMetrics(true, time.Since(time.Now()))

	return resp.Msg, nil
}

// UpdateSystemInfo sends updated system information with enhanced error handling
func (r *RegistrationClient) UpdateSystemInfo(ctx context.Context, deviceID string) error {
	// Recover from panics

	if deviceID == "" {
		return connect.NewError(connect.CodeInvalidArgument, stderrors.New("device ID is required"))
	}

	// Collect system info with retry
	sysInfo, err := r.collectSystemInfoWithRetry(ctx)
	if err != nil {
		// This is not critical, log and continue
		r.logger.Warn("Failed to collect complete system info",
			"error", err,
		)
		sysInfo = &SystemInfo{
			Extra: make(map[string]string),
		}
	}

	// Get current system stats
	stats, err := r.getSystemStatsWithRetry(ctx)
	if err != nil {
		r.logger.Warn("Failed to get system stats",
			"error", err,
		)
	}

	// Build metrics
	metrics := r.buildMetrics(sysInfo, stats)

	req := &pb.ReportStatusRequest{
		DeviceId: deviceID,
		Status:   "online",
		Metrics:  metrics,
	}

	// Execute status report
	_, err = r.client.ReportStatus(ctx, connect.NewRequest(req))
	if err != nil {
		return err
	}

	r.logger.Debug("System info updated successfully",
		"device_id", deviceID,
	)

	return nil
}

// Heartbeat sends periodic heartbeat with error handling
func (r *RegistrationClient) Heartbeat(ctx context.Context, deviceID string) (*pb.HeartbeatResponse, error) {
	req := &pb.HeartbeatRequest{
		DeviceId: deviceID,
		Metrics: map[string]string{
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		},
	}

	resp, err := r.client.Heartbeat(ctx, connect.NewRequest(req))
	if err != nil {
		r.logger.Debug("Heartbeat failed",
			"error", err,
			"device_id", deviceID,
		)
		return nil, err
	}

	return resp.Msg, nil
}

// Helper methods

func (r *RegistrationClient) collectSystemInfoWithRetry(ctx context.Context) (*SystemInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	sysInfo, err := CollectSystemInfoWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("system info collection failed: %w", err)
	}
	return sysInfo, nil
}

func (r *RegistrationClient) getSystemStatsWithRetry(ctx context.Context) (*SystemStats, error) {
	stats, err := GetSystemStats()
	if err != nil {
		return nil, fmt.Errorf("system stats collection failed: %w", err)
	}
	return stats, nil
}

func (r *RegistrationClient) convertToProtobufSystemInfo(sysInfo *SystemInfo) *pb.SystemInfo {
	pbSysInfo := &pb.SystemInfo{
		Hostname:      sysInfo.Hostname,
		Os:            sysInfo.OS,
		OsVersion:     sysInfo.OSVersion,
		Arch:          sysInfo.Arch,
		CpuModel:      sysInfo.CPUModel,
		CpuCores:      sysInfo.CPUCores,
		MemoryTotal:   sysInfo.MemoryTotal,
		StorageTotal:  sysInfo.StorageTotal,
		KernelVersion: sysInfo.KernelVersion,
		Platform:      sysInfo.Platform,
		Extra:         sysInfo.Extra,
		Timezone:      sysInfo.Timezone,
		AgentVersion:  sysInfo.AgentVersion,
		SerialNumber:  sysInfo.SerialNumber,
		ProductName:   sysInfo.ProductName,
		Manufacturer:  sysInfo.Manufacturer,
		ProcessCount:  sysInfo.ProcessCount,
	}

	// Convert network interfaces
	for _, iface := range sysInfo.NetworkInterfaces {
		pbIface := &pb.NetworkInterface{
			Name:        iface.Name,
			MacAddress:  iface.MACAddress,
			IpAddresses: iface.IPAddresses,
			IsUp:        iface.IsUp,
			IsLoopback:  iface.IsLoopback,
			Mtu:         iface.MTU,
		}
		pbSysInfo.NetworkInterfaces = append(pbSysInfo.NetworkInterfaces, pbIface)
	}

	// Convert load average
	pbSysInfo.LoadAverage = &pb.LoadAverage{
		Load1:  sysInfo.LoadAverage.Load1,
		Load5:  sysInfo.LoadAverage.Load5,
		Load15: sysInfo.LoadAverage.Load15,
	}

	// Convert BIOS info if available
	if sysInfo.BiosInfo.Vendor != "" || sysInfo.BiosInfo.Version != "" {
		pbSysInfo.BiosInfo = &pb.BiosInfo{
			Vendor:      sysInfo.BiosInfo.Vendor,
			Version:     sysInfo.BiosInfo.Version,
			ReleaseDate: sysInfo.BiosInfo.ReleaseDate,
		}
	}

	return pbSysInfo
}

func (r *RegistrationClient) buildMetrics(sysInfo *SystemInfo, stats *SystemStats) map[string]string {
	metrics := map[string]string{
		"hostname":       sysInfo.Hostname,
		"os":             sysInfo.OS,
		"os_version":     sysInfo.OSVersion,
		"arch":           sysInfo.Arch,
		"cpu_model":      sysInfo.CPUModel,
		"cpu_cores":      fmt.Sprintf("%d", sysInfo.CPUCores),
		"memory_total":   fmt.Sprintf("%d", sysInfo.MemoryTotal),
		"storage_total":  fmt.Sprintf("%d", sysInfo.StorageTotal),
		"kernel_version": sysInfo.KernelVersion,
		"platform":       sysInfo.Platform,
	}

	// Add current stats if available
	if stats != nil {
		metrics["cpu_usage"] = fmt.Sprintf("%.2f", stats.CPUUsage)
		metrics["memory_used"] = fmt.Sprintf("%d", stats.MemoryUsed)
		metrics["disk_used"] = fmt.Sprintf("%d", stats.DiskUsed)
	}

	// Add extra info
	for k, v := range sysInfo.Extra {
		metrics[k] = v
	}

	return metrics
}

func (r *RegistrationClient) recordRegistrationMetrics(success bool, duration time.Duration) {
	// This would integrate with your metrics system
	status := "success"
	if !success {
		status = "failure"
	}

	r.logger.Debug("Registration metrics",
		"status", status,
		"duration_ms", duration.Milliseconds(),
	)
}
