# FleetD Protocol Buffers

## Structure

The proto definitions are organized into three main packages:

### 1. Agent API (`agent/v1/`)
**Purpose:** Device-to-cloud communication
**Used by:** On-device agents
**Authentication:** Device certificates or pre-shared keys

Services:
- **DeviceService** - Device registration, heartbeat, status reporting
- **TelemetryService** - Metrics, logs, and event ingestion
- **UpdateService** - OTA update checking and downloading
- **DiscoveryService** - Local network device discovery

### 2. Control API (`control/v1/`)
**Purpose:** Fleet management and administration
**Used by:** Web UI, CLI, SDKs
**Authentication:** User tokens (JWT) or API keys

Services:
- **FleetService** - Fleet-wide device management
- **DeploymentService** - Software deployment campaigns
- **AnalyticsService** - Metrics, logs, and reporting
- **OrganizationService** - Multi-tenancy and user management

### 3. Common Types (`common/v1/`)
**Purpose:** Shared data structures
**Used by:** Both Agent and Control APIs

Types:
- **Device** - Core device representation
- **Metrics** - Metric points and system metrics
- **LogEntry** - Log entries and events

## Compilation

### Install buf
```bash
brew install bufbuild/buf/buf
```

### Generate code
```bash
cd proto
buf generate
```

### Lint protos
```bash
buf lint
```

### Breaking change detection
```bash
buf breaking --against '.git#branch=main'
```

## Usage Examples

### Agent API (Device-side)
```go
// Device registration
client := agentpb.NewDeviceServiceClient(...)
resp, err := client.Register(ctx, &agentpb.RegisterRequest{
    Name: "sensor-001",
    Type: "temperature-sensor",
    ProvisioningKey: "xxx",
})

// Send telemetry
telemetryClient := agentpb.NewTelemetryServiceClient(...)
_, err = telemetryClient.SendMetrics(ctx, &agentpb.SendMetricsRequest{
    DeviceId: deviceID,
    SystemMetrics: &commonpb.SystemMetrics{
        CpuUsagePercent: 45.2,
        MemoryUsagePercent: 62.1,
    },
})
```

### Control API (Management-side)
```go
// List devices
fleetClient := controlpb.NewFleetServiceClient(...)
resp, err := fleetClient.ListDevices(ctx, &controlpb.ListDevicesRequest{
    Statuses: []commonpb.DeviceStatus{
        commonpb.DeviceStatus_DEVICE_STATUS_ONLINE,
    },
    PageSize: 100,
})

// Create deployment
deployClient := controlpb.NewDeploymentServiceClient(...)
resp, err := deployClient.CreateDeployment(ctx, &controlpb.CreateDeploymentRequest{
    Name: "firmware-v2.0",
    Strategy: &controlpb.DeploymentStrategy{
        Type: controlpb.DeploymentType_DEPLOYMENT_TYPE_ROLLING,
    },
})
```

## Security Considerations

### Agent API
- Uses device-specific authentication
- Rate limited per device
- Minimal response payloads
- No cross-device data access

### Control API
- Requires user authentication
- Role-based access control (RBAC)
- Audit logging for all operations
- Organization-level data isolation

## Migration Guide

If migrating from the old proto structure:

1. Update imports:
   - `fleetd/v1/device.proto` → `agent/v1/device.proto` or `control/v1/fleet.proto`
   - `fleetd/v1/analytics.proto` → `control/v1/analytics.proto`

2. Update service clients:
   - Device operations: Use `agent.DeviceService` for device-side, `control.FleetService` for management
   - Telemetry: Use `agent.TelemetryService` for ingestion, `control.AnalyticsService` for queries

3. Update authentication:
   - Agent API: Use device tokens
   - Control API: Use user tokens or API keys