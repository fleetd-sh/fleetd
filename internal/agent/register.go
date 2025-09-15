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
	"fleetd.sh/internal/ferrors"
)

// RegistrationClient handles device registration with enhanced error handling
type RegistrationClient struct {
	client         fleetpbconnect.DeviceServiceClient
	serverURL      string
	logger         *slog.Logger
	circuitBreaker *ferrors.CircuitBreaker
	retryPolicy    *ferrors.RetryPolicy
	errorHandler   *ferrors.ErrorHandler
}

// NewRegistrationClient creates a new registration client with production-ready error handling
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

	// Configure circuit breaker for registration service
	cbConfig := &ferrors.CircuitBreakerConfig{
		MaxFailures: 3,
		MaxRequests: 1,
		Interval:    60 * time.Second,
		Timeout:     30 * time.Second,
		OnStateChange: func(from, to ferrors.CircuitBreakerState) {
			slog.Warn("Circuit breaker state changed",
				"from", from.String(),
				"to", to.String(),
				"service", "registration",
			)
		},
		ShouldTrip: func(err error) bool {
			// Don't trip on client errors (4xx equivalent)
			code := ferrors.GetCode(err)
			switch code {
			case ferrors.ErrCodeInvalidInput,
				ferrors.ErrCodeNotFound,
				ferrors.ErrCodeAlreadyExists,
				ferrors.ErrCodePermissionDenied:
				return false
			default:
				return true
			}
		},
	}

	cb := ferrors.NewCircuitBreaker(cbConfig)

	// Configure retry policy
	retryConfig := &ferrors.RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 1 * time.Second,
		MaxDelay:     30 * time.Second,
		Multiplier:   2.0,
		Jitter:       0.1,
		RetryableFunc: func(err error) bool {
			// Check if error is retryable
			if ferrors.IsRetryable(err) {
				return true
			}

			// Check for specific Connect errors
			if connectErr := new(connect.Error); stderrors.As(err, &connectErr) {
				switch connectErr.Code() {
				case connect.CodeUnavailable,
					connect.CodeDeadlineExceeded,
					connect.CodeResourceExhausted:
					return true
				}
			}

			return false
		},
		OnRetry: func(attempt int, err error, delay time.Duration) {
			slog.Warn("Retrying registration",
				"attempt", attempt,
				"error", err,
				"delay", delay,
				"server", serverURL,
			)
		},
	}

	retryPolicy := ferrors.NewRetryPolicy(retryConfig, cb)

	// Configure error handler
	errorHandler := &ferrors.ErrorHandler{
		RequestID: requestID,
		OnError: func(err *ferrors.FleetError) {
			slog.Error("Registration error",
				"code", err.Code,
				"message", err.Message,
				"severity", err.Severity,
				"retryable", err.Retryable,
				"request_id", err.RequestID,
			)
		},
		OnPanic: func(recovered any, stack string) {
			slog.Error("Registration panic",
				"recovered", recovered,
				"stack", stack,
				"request_id", requestID,
			)
		},
	}

	return &RegistrationClient{
		client:         client,
		serverURL:      serverURL,
		logger:         slog.Default().With("component", "registration-client"),
		circuitBreaker: cb,
		retryPolicy:    retryPolicy,
		errorHandler:   errorHandler,
	}
}

// RegisterDevice registers a new device with enhanced error handling and resilience
func (r *RegistrationClient) RegisterDevice(ctx context.Context, deviceName, deviceType, version string) (*pb.RegisterResponse, error) {
	// Recover from panics
	defer r.errorHandler.HandlePanic()

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

	// Execute registration with retry and circuit breaker
	var resp *connect.Response[pb.RegisterResponse]
	err = r.retryPolicy.Execute(ctx, func() error {
		var execErr error
		resp, execErr = r.client.Register(ctx, connect.NewRequest(req))
		if execErr != nil {
			// Wrap Connect errors with our error types
			if connectErr := new(connect.Error); stderrors.As(execErr, &connectErr) {
				return r.wrapConnectError(connectErr, "registration failed")
			}
			return ferrors.Wrap(execErr, ferrors.ErrCodeInternal, "registration failed")
		}
		return nil
	})

	if err != nil {
		r.errorHandler.Handle(err)
		return nil, err
	}

	r.logger.Info("Device registered successfully",
		"device_id", resp.Msg.DeviceId,
		"has_api_key", resp.Msg.ApiKey != "",
		"circuit_breaker_state", r.circuitBreaker.GetState().String(),
	)

	// Record success metrics
	r.recordRegistrationMetrics(true, time.Since(time.Now()))

	return resp.Msg, nil
}

// UpdateSystemInfo sends updated system information with enhanced error handling
func (r *RegistrationClient) UpdateSystemInfo(ctx context.Context, deviceID string) error {
	// Recover from panics
	defer r.errorHandler.HandlePanic()

	// Validate input
	if deviceID == "" {
		err := ferrors.New(ferrors.ErrCodeInvalidInput, "device ID is required")
		r.errorHandler.Handle(err)
		return err
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

	// Execute with retry and circuit breaker
	err = r.retryPolicy.Execute(ctx, func() error {
		_, execErr := r.client.ReportStatus(ctx, connect.NewRequest(req))
		if execErr != nil {
			if connectErr := new(connect.Error); stderrors.As(execErr, &connectErr) {
				return r.wrapConnectError(connectErr, "status report failed")
			}
			return ferrors.Wrap(execErr, ferrors.ErrCodeInternal, "status report failed")
		}
		return nil
	})

	if err != nil {
		r.errorHandler.Handle(err)
		return err
	}

	r.logger.Debug("System info updated successfully",
		"device_id", deviceID,
	)

	return nil
}

// Heartbeat sends periodic heartbeat with error handling
func (r *RegistrationClient) Heartbeat(ctx context.Context, deviceID string) (*pb.HeartbeatResponse, error) {
	// Recover from panics
	defer r.errorHandler.HandlePanic()

	// Quick heartbeat with minimal retry
	retryConfig := &ferrors.RetryConfig{
		MaxAttempts:  2,
		InitialDelay: 500 * time.Millisecond,
		MaxDelay:     5 * time.Second,
		Multiplier:   2.0,
		RetryableFunc: func(err error) bool {
			// Only retry on network errors for heartbeat
			code := ferrors.GetCode(err)
			return code == ferrors.ErrCodeTimeout || code == ferrors.ErrCodeUnavailable
		},
	}

	var resp *connect.Response[pb.HeartbeatResponse]
	err := ferrors.Retry(ctx, retryConfig, func() error {
		req := &pb.HeartbeatRequest{
			DeviceId: deviceID,
			Metrics: map[string]string{
				"timestamp": time.Now().UTC().Format(time.RFC3339),
			},
		}

		var execErr error
		resp, execErr = r.client.Heartbeat(ctx, connect.NewRequest(req))
		if execErr != nil {
			if connectErr := new(connect.Error); stderrors.As(execErr, &connectErr) {
				// Don't trip circuit breaker on heartbeat failures
				return r.wrapConnectError(connectErr, "heartbeat failed")
			}
			return ferrors.Wrap(execErr, ferrors.ErrCodeInternal, "heartbeat failed")
		}
		return nil
	})

	if err != nil {
		// Log but don't fail - heartbeats are non-critical
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
	var sysInfo *SystemInfo
	err := ferrors.RetryWithCustom(ctx, 3, 100*time.Millisecond, func() error {
		var collectErr error
		sysInfo, collectErr = CollectSystemInfo()
		if collectErr != nil {
			return ferrors.Wrap(collectErr, ferrors.ErrCodeInternal, "system info collection failed")
		}
		return nil
	})
	return sysInfo, err
}

func (r *RegistrationClient) getSystemStatsWithRetry(ctx context.Context) (*SystemStats, error) {
	var stats *SystemStats
	err := ferrors.RetryWithCustom(ctx, 2, 100*time.Millisecond, func() error {
		var statsErr error
		stats, statsErr = GetSystemStats()
		if statsErr != nil {
			return ferrors.Wrap(statsErr, ferrors.ErrCodeInternal, "system stats collection failed")
		}
		return nil
	})
	return stats, err
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

func (r *RegistrationClient) wrapConnectError(err *connect.Error, message string) error {
	var code ferrors.ErrorCode
	var retryable bool

	switch err.Code() {
	case connect.CodeInvalidArgument:
		code = ferrors.ErrCodeInvalidInput
	case connect.CodeNotFound:
		code = ferrors.ErrCodeNotFound
	case connect.CodeAlreadyExists:
		code = ferrors.ErrCodeAlreadyExists
	case connect.CodePermissionDenied:
		code = ferrors.ErrCodePermissionDenied
	case connect.CodeResourceExhausted:
		code = ferrors.ErrCodeResourceExhausted
		retryable = true
	case connect.CodeFailedPrecondition:
		code = ferrors.ErrCodePreconditionFailed
	case connect.CodeAborted:
		code = ferrors.ErrCodeInternal
		retryable = true
	case connect.CodeOutOfRange:
		code = ferrors.ErrCodeInvalidInput
	case connect.CodeUnimplemented:
		code = ferrors.ErrCodeNotImplemented
	case connect.CodeInternal:
		code = ferrors.ErrCodeInternal
	case connect.CodeUnavailable:
		code = ferrors.ErrCodeUnavailable
		retryable = true
	case connect.CodeDataLoss:
		code = ferrors.ErrCodeDataLoss
	case connect.CodeUnauthenticated:
		code = ferrors.ErrCodePermissionDenied
	case connect.CodeDeadlineExceeded:
		code = ferrors.ErrCodeTimeout
		retryable = true
	default:
		code = ferrors.ErrCodeInternal
	}

	fleetErr := ferrors.Wrapf(err, code, "%s: %s", message, err.Message())
	fleetErr.Retryable = retryable

	// Add metadata from Connect error
	if len(err.Details()) > 0 {
		fleetErr.WithMetadata("connect_details", err.Details())
	}

	return fleetErr
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
		"circuit_breaker_metrics", r.circuitBreaker.GetMetrics(),
	)
}

// GetCircuitBreakerMetrics returns circuit breaker metrics for monitoring
func (r *RegistrationClient) GetCircuitBreakerMetrics() map[string]any {
	return r.circuitBreaker.GetMetrics()
}

// ResetCircuitBreaker manually resets the circuit breaker
func (r *RegistrationClient) ResetCircuitBreaker() {
	r.circuitBreaker.Reset()
	r.logger.Info("Circuit breaker reset manually")
}
