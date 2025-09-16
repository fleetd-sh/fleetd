# FleetD API Reference

## Overview

FleetD provides a Connect RPC API built on HTTP/2 with support for streaming, bi-directional communication, and automatic code generation. All API endpoints require authentication unless explicitly marked as public.

## Authentication

FleetD supports multiple authentication methods:

### JWT Authentication
```bash
curl -H "Authorization: Bearer $TOKEN" \
  https://api.fleetd.io/fleetd.v1.DeviceService/ListDevices
```

### mTLS Authentication
```bash
curl --cert device.crt --key device.key --cacert ca.crt \
  https://api.fleetd.io/fleetd.v1.DeviceService/RegisterDevice
```

### API Key Authentication
```bash
curl -H "X-API-Key: $API_KEY" \
  https://api.fleetd.io/fleetd.v1.DeviceService/GetDevice
```

## Error Handling

All errors follow a consistent format:

```json
{
  "code": "PERMISSION_DENIED",
  "message": "User does not have permission device:delete",
  "details": {
    "request_id": "abc123",
    "user_id": "user-001",
    "permission": "device:delete"
  }
}
```

### Error Codes
- `INVALID_ARGUMENT` (400) - Invalid request parameters
- `NOT_FOUND` (404) - Resource not found
- `ALREADY_EXISTS` (409) - Resource already exists
- `PERMISSION_DENIED` (403) - Insufficient permissions
- `UNAUTHENTICATED` (401) - Authentication required
- `RESOURCE_EXHAUSTED` (429) - Rate limit exceeded
- `FAILED_PRECONDITION` (412) - Precondition failed
- `UNAVAILABLE` (503) - Service temporarily unavailable
- `INTERNAL` (500) - Internal server error

## Rate Limiting

Default rate limits:
- **Anonymous**: 10 requests/second
- **Authenticated**: 100 requests/second
- **Device**: 10 requests/minute
- **Admin**: Unlimited

Rate limit headers:
```
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 95
X-RateLimit-Reset: 1640995200
```

## API Services

### Device Service

#### RegisterDevice
Register a new device with the fleet.

**Request:**
```protobuf
message RegisterDeviceRequest {
  string device_id = 1;
  string device_name = 2;
  string device_type = 3;
  string version = 4;
  SystemInfo system_info = 5;
  map<string, string> metadata = 6;
}
```

**Response:**
```protobuf
message RegisterDeviceResponse {
  string device_id = 1;
  string api_key = 2;
  string certificate = 3;
  google.protobuf.Timestamp registered_at = 4;
}
```

**Example:**
```bash
grpcurl -d '{
  "device_id": "rpi-001",
  "device_name": "Raspberry Pi 001",
  "device_type": "raspberry-pi",
  "version": "1.0.0",
  "system_info": {
    "os": "Linux",
    "arch": "arm64",
    "cpu_cores": 4,
    "memory_mb": 4096
  }
}' api.fleetd.io:443 fleetd.v1.DeviceService/RegisterDevice
```

**Permissions Required:** `device:register`

---

#### ListDevices
List all devices in the fleet.

**Request:**
```protobuf
message ListDevicesRequest {
  int32 page_size = 1;
  string page_token = 2;
  string filter = 3;
  string order_by = 4;
}
```

**Response:**
```protobuf
message ListDevicesResponse {
  repeated Device devices = 1;
  string next_page_token = 2;
  int32 total_count = 3;
}
```

**Example:**
```bash
grpcurl -d '{
  "page_size": 20,
  "filter": "status:online",
  "order_by": "last_seen DESC"
}' api.fleetd.io:443 fleetd.v1.DeviceService/ListDevices
```

**Permissions Required:** `device:list`

---

#### GetDevice
Get detailed information about a specific device.

**Request:**
```protobuf
message GetDeviceRequest {
  string device_id = 1;
  bool include_metrics = 2;
  bool include_health = 3;
}
```

**Response:**
```protobuf
message GetDeviceResponse {
  Device device = 1;
  DeviceMetrics metrics = 2;
  DeviceHealth health = 3;
}
```

**Example:**
```bash
grpcurl -d '{
  "device_id": "rpi-001",
  "include_metrics": true,
  "include_health": true
}' api.fleetd.io:443 fleetd.v1.DeviceService/GetDevice
```

**Permissions Required:** `device:view`

---

#### UpdateDevice
Update device information.

**Request:**
```protobuf
message UpdateDeviceRequest {
  string device_id = 1;
  google.protobuf.FieldMask update_mask = 2;
  Device device = 3;
}
```

**Response:**
```protobuf
message UpdateDeviceResponse {
  Device device = 1;
  google.protobuf.Timestamp updated_at = 2;
}
```

**Example:**
```bash
grpcurl -d '{
  "device_id": "rpi-001",
  "update_mask": {
    "paths": ["name", "metadata"]
  },
  "device": {
    "name": "Raspberry Pi Production",
    "metadata": {
      "location": "datacenter-1"
    }
  }
}' api.fleetd.io:443 fleetd.v1.DeviceService/UpdateDevice
```

**Permissions Required:** `device:update`

---

#### DeleteDevice
Remove a device from the fleet.

**Request:**
```protobuf
message DeleteDeviceRequest {
  string device_id = 1;
  bool force = 2;
}
```

**Response:**
```protobuf
message DeleteDeviceResponse {
  bool success = 1;
  google.protobuf.Timestamp deleted_at = 2;
}
```

**Example:**
```bash
grpcurl -d '{
  "device_id": "rpi-001",
  "force": false
}' api.fleetd.io:443 fleetd.v1.DeviceService/DeleteDevice
```

**Permissions Required:** `device:delete`

---

#### Heartbeat
Send device heartbeat (called by devices).

**Request:**
```protobuf
message HeartbeatRequest {
  string device_id = 1;
  DeviceStatus status = 2;
  SystemStats stats = 3;
  repeated ProcessInfo processes = 4;
}
```

**Response:**
```protobuf
message HeartbeatResponse {
  bool acknowledged = 1;
  repeated Command commands = 2;
  UpdateInfo pending_update = 3;
}
```

**Example:**
```bash
grpcurl -d '{
  "device_id": "rpi-001",
  "status": {
    "state": "ONLINE",
    "uptime_seconds": 3600,
    "last_boot": "2024-01-01T12:00:00Z"
  },
  "stats": {
    "cpu_percent": 25.5,
    "memory_used_mb": 1024,
    "disk_used_gb": 10.5
  }
}' api.fleetd.io:443 fleetd.v1.DeviceService/Heartbeat
```

**Permissions Required:** `device:heartbeat`

---

### Update Service

#### CreateUpdate
Create a new software update.

**Request:**
```protobuf
message CreateUpdateRequest {
  string name = 1;
  string version = 2;
  string description = 3;
  repeated Artifact artifacts = 4;
  UpdateStrategy strategy = 5;
  map<string, string> metadata = 6;
}
```

**Response:**
```protobuf
message CreateUpdateResponse {
  Update update = 1;
  string update_id = 2;
  google.protobuf.Timestamp created_at = 3;
}
```

**Example:**
```bash
grpcurl -d '{
  "name": "Firmware Update v2.0",
  "version": "2.0.0",
  "description": "Security patches and performance improvements",
  "artifacts": [{
    "platform": "raspberry-pi",
    "url": "https://updates.fleetd.io/firmware-2.0.0-rpi.tar.gz",
    "checksum": "sha256:abc123...",
    "size_bytes": 104857600
  }],
  "strategy": {
    "type": "ROLLING",
    "max_parallel": 10,
    "max_failure_percentage": 5
  }
}' api.fleetd.io:443 fleetd.v1.UpdateService/CreateUpdate
```

**Permissions Required:** `update:create`

---

#### DeployUpdate
Deploy an update to devices.

**Request:**
```protobuf
message DeployUpdateRequest {
  string update_id = 1;
  repeated string device_ids = 2;
  string device_filter = 3;
  DeploymentConfig config = 4;
}
```

**Response:**
```protobuf
message DeployUpdateResponse {
  string deployment_id = 1;
  int32 target_device_count = 2;
  google.protobuf.Timestamp started_at = 3;
}
```

**Example:**
```bash
grpcurl -d '{
  "update_id": "update-001",
  "device_filter": "type:raspberry-pi AND status:online",
  "config": {
    "strategy": "CANARY",
    "canary_percentage": 10,
    "validation_duration": "3600s",
    "auto_promote": true
  }
}' api.fleetd.io:443 fleetd.v1.UpdateService/DeployUpdate
```

**Permissions Required:** `update:deploy`

---

#### GetDeploymentStatus
Get the status of a deployment.

**Request:**
```protobuf
message GetDeploymentStatusRequest {
  string deployment_id = 1;
  bool include_device_details = 2;
}
```

**Response:**
```protobuf
message GetDeploymentStatusResponse {
  DeploymentStatus status = 1;
  repeated DeviceDeploymentStatus device_statuses = 2;
  DeploymentMetrics metrics = 3;
}
```

**Example:**
```bash
grpcurl -d '{
  "deployment_id": "deploy-001",
  "include_device_details": true
}' api.fleetd.io:443 fleetd.v1.UpdateService/GetDeploymentStatus
```

**Permissions Required:** `update:view`

---

#### RollbackDeployment
Rollback a deployment.

**Request:**
```protobuf
message RollbackDeploymentRequest {
  string deployment_id = 1;
  string reason = 2;
  bool force = 3;
}
```

**Response:**
```protobuf
message RollbackDeploymentResponse {
  bool success = 1;
  int32 devices_rolled_back = 2;
  google.protobuf.Timestamp rolled_back_at = 3;
}
```

**Example:**
```bash
grpcurl -d '{
  "deployment_id": "deploy-001",
  "reason": "High error rate detected",
  "force": false
}' api.fleetd.io:443 fleetd.v1.UpdateService/RollbackDeployment
```

**Permissions Required:** `update:rollback`

---

### Analytics Service

#### GetMetrics
Get fleet metrics and statistics.

**Request:**
```protobuf
message GetMetricsRequest {
  google.protobuf.Timestamp start_time = 1;
  google.protobuf.Timestamp end_time = 2;
  repeated string metric_names = 3;
  string aggregation = 4;
  google.protobuf.Duration interval = 5;
  map<string, string> labels = 6;
}
```

**Response:**
```protobuf
message GetMetricsResponse {
  repeated MetricSeries series = 1;
  MetricSummary summary = 2;
}
```

**Example:**
```bash
grpcurl -d '{
  "start_time": "2024-01-01T00:00:00Z",
  "end_time": "2024-01-02T00:00:00Z",
  "metric_names": ["cpu_usage", "memory_usage", "device_online_count"],
  "aggregation": "AVG",
  "interval": "3600s"
}' api.fleetd.io:443 fleetd.v1.AnalyticsService/GetMetrics
```

**Permissions Required:** `analytics:view`

---

#### GetDeviceHealth
Get health information for devices.

**Request:**
```protobuf
message GetDeviceHealthRequest {
  repeated string device_ids = 1;
  string device_filter = 2;
  HealthCheckType check_type = 3;
}
```

**Response:**
```protobuf
message GetDeviceHealthResponse {
  map<string, DeviceHealth> device_health = 1;
  HealthSummary summary = 2;
}
```

**Example:**
```bash
grpcurl -d '{
  "device_filter": "type:raspberry-pi",
  "check_type": "COMPREHENSIVE"
}' api.fleetd.io:443 fleetd.v1.AnalyticsService/GetDeviceHealth
```

**Permissions Required:** `analytics:view`

---

#### ExportData
Export fleet data for analysis.

**Request:**
```protobuf
message ExportDataRequest {
  ExportFormat format = 1;
  DataType data_type = 2;
  google.protobuf.Timestamp start_time = 3;
  google.protobuf.Timestamp end_time = 4;
  repeated string fields = 5;
}
```

**Response:**
```protobuf
message ExportDataResponse {
  string export_url = 1;
  int64 size_bytes = 2;
  google.protobuf.Timestamp expires_at = 3;
}
```

**Example:**
```bash
grpcurl -d '{
  "format": "CSV",
  "data_type": "DEVICE_METRICS",
  "start_time": "2024-01-01T00:00:00Z",
  "end_time": "2024-01-31T23:59:59Z",
  "fields": ["device_id", "timestamp", "cpu_usage", "memory_usage"]
}' api.fleetd.io:443 fleetd.v1.AnalyticsService/ExportData
```

**Permissions Required:** `analytics:export`

---

### Binary Service (Streaming)

#### UploadBinary
Upload a binary artifact (streaming).

**Request Stream:**
```protobuf
message UploadBinaryRequest {
  oneof data {
    BinaryMetadata metadata = 1;
    bytes chunk = 2;
  }
}
```

**Response:**
```protobuf
message UploadBinaryResponse {
  string binary_id = 1;
  string checksum = 2;
  int64 size_bytes = 3;
  google.protobuf.Timestamp uploaded_at = 4;
}
```

**Example:**
```python
import grpc
from fleetd.v1 import binary_pb2, binary_pb2_grpc

def upload_binary(stub, file_path):
    def generate_chunks():
        # Send metadata first
        yield binary_pb2.UploadBinaryRequest(
            metadata=binary_pb2.BinaryMetadata(
                name="firmware.bin",
                version="2.0.0",
                platform="raspberry-pi"
            )
        )

        # Send file chunks
        with open(file_path, 'rb') as f:
            while True:
                chunk = f.read(1024 * 1024)  # 1MB chunks
                if not chunk:
                    break
                yield binary_pb2.UploadBinaryRequest(chunk=chunk)

    response = stub.UploadBinary(generate_chunks())
    return response
```

**Permissions Required:** `update:create`

---

#### DownloadBinary
Download a binary artifact (streaming).

**Request:**
```protobuf
message DownloadBinaryRequest {
  string binary_id = 1;
  int64 offset = 2;
  int64 chunk_size = 3;
}
```

**Response Stream:**
```protobuf
message DownloadBinaryResponse {
  oneof data {
    BinaryMetadata metadata = 1;
    bytes chunk = 2;
  }
}
```

**Example:**
```python
def download_binary(stub, binary_id, output_path):
    request = binary_pb2.DownloadBinaryRequest(
        binary_id=binary_id,
        chunk_size=1024 * 1024  # 1MB chunks
    )

    with open(output_path, 'wb') as f:
        for response in stub.DownloadBinary(request):
            if response.HasField('chunk'):
                f.write(response.chunk)
```

**Permissions Required:** `update:view`

---

## WebSocket API

For real-time updates, FleetD provides WebSocket endpoints:

### Device Events
```javascript
const ws = new WebSocket('wss://api.fleetd.io/ws/devices');

ws.onopen = () => {
  ws.send(JSON.stringify({
    type: 'subscribe',
    filter: 'type:raspberry-pi',
    events: ['status_change', 'heartbeat', 'deployment']
  }));
};

ws.onmessage = (event) => {
  const data = JSON.parse(event.data);
  console.log('Device event:', data);
};
```

### Metrics Stream
```javascript
const ws = new WebSocket('wss://api.fleetd.io/ws/metrics');

ws.onopen = () => {
  ws.send(JSON.stringify({
    type: 'subscribe',
    metrics: ['cpu_usage', 'memory_usage'],
    interval: 5000  // 5 seconds
  }));
};
```

## SDK Examples

### Go Client
```go
package main

import (
    "context"
    "log"

    "fleetd.sh/sdk/go/fleetd"
    "fleetd.sh/gen/fleetd/v1/fleetpbconnect"
)

func main() {
    client := fleetd.NewClient(
        fleetd.WithEndpoint("https://api.fleetd.io"),
        fleetd.WithAPIKey("your-api-key"),
    )

    resp, err := client.Device.List(context.Background(), &fleetpb.ListDevicesRequest{
        PageSize: 10,
        Filter: "status:online",
    })
    if err != nil {
        log.Fatal(err)
    }

    for _, device := range resp.Devices {
        log.Printf("Device: %s (%s)", device.Id, device.Name)
    }
}
```

### Python Client
```python
import fleetd
from fleetd.v1 import device_pb2

client = fleetd.Client(
    endpoint="https://api.fleetd.io",
    api_key="your-api-key"
)

# List devices
devices = client.device.list(
    page_size=10,
    filter="status:online"
)

for device in devices.devices:
    print(f"Device: {device.id} ({device.name})")

# Register device
response = client.device.register(
    device_id="rpi-001",
    device_name="Raspberry Pi 001",
    device_type="raspberry-pi",
    version="1.0.0"
)
print(f"Registered with API key: {response.api_key}")
```

### JavaScript/TypeScript Client
```typescript
import { FleetDClient } from '@fleetd/client';

const client = new FleetDClient({
  endpoint: 'https://api.fleetd.io',
  apiKey: 'your-api-key',
});

// List devices
const devices = await client.device.list({
  pageSize: 10,
  filter: 'status:online',
});

devices.devices.forEach(device => {
  console.log(`Device: ${device.id} (${device.name})`);
});

// Subscribe to device events
const stream = client.device.streamEvents({
  deviceFilter: 'type:raspberry-pi',
  events: ['status_change', 'heartbeat'],
});

for await (const event of stream) {
  console.log('Device event:', event);
}
```

## Pagination

All list endpoints support pagination:

```protobuf
message PageRequest {
  int32 page_size = 1;   // Number of items per page (max: 100)
  string page_token = 2;  // Token from previous response
}

message PageResponse {
  string next_page_token = 1;  // Token for next page
  int32 total_count = 2;       // Total number of items
}
```

Example:
```bash
# First page
grpcurl -d '{"page_size": 20}' api.fleetd.io:443 fleetd.v1.DeviceService/ListDevices

# Next page
grpcurl -d '{"page_size": 20, "page_token": "next-token-from-response"}' \
  api.fleetd.io:443 fleetd.v1.DeviceService/ListDevices
```

## Filtering

Support for filtering using a simple query language:

```
field:value              # Exact match
field:value1,value2      # Multiple values (OR)
field:>value             # Greater than
field:<value             # Less than
field:value*             # Prefix match
field:*value             # Suffix match
field:*value*            # Contains
NOT field:value          # Negation
field1:value1 AND field2:value2  # Conjunction
field1:value1 OR field2:value2   # Disjunction
```

Examples:
- `status:online` - Online devices
- `type:raspberry-pi,esp32` - Raspberry Pi or ESP32
- `last_seen:>2024-01-01` - Seen after Jan 1, 2024
- `cpu_usage:>80 AND memory_usage:>90` - High resource usage
- `NOT status:offline` - Not offline

## Sorting

Use the `order_by` field with format: `field [ASC|DESC]`

Examples:
- `last_seen DESC` - Most recently seen first
- `name ASC` - Alphabetical by name
- `cpu_usage DESC, memory_usage DESC` - By CPU then memory

## Field Masks

For partial updates, use field masks to specify which fields to update:

```protobuf
message UpdateRequest {
  google.protobuf.FieldMask update_mask = 1;
  Resource resource = 2;
}
```

Example:
```json
{
  "update_mask": {
    "paths": ["name", "metadata.location", "status"]
  },
  "device": {
    "name": "New Name",
    "metadata": {
      "location": "Building A"
    },
    "status": "MAINTENANCE"
  }
}
```

## Webhooks

Configure webhooks for real-time notifications:

```json
{
  "url": "https://your-server.com/webhook",
  "events": ["device.offline", "deployment.failed", "update.available"],
  "headers": {
    "X-Webhook-Secret": "your-secret"
  },
  "retry": {
    "max_attempts": 3,
    "backoff": "exponential"
  }
}
```

Webhook payload:
```json
{
  "event": "device.offline",
  "timestamp": "2024-01-01T12:00:00Z",
  "data": {
    "device_id": "rpi-001",
    "last_seen": "2024-01-01T11:55:00Z"
  }
}
```

## API Versioning

FleetD follows semantic versioning for API changes:
- `v1` - Current stable version
- `v2-beta` - Next version in beta
- `v1-deprecated` - Deprecated, will be removed

Version is specified in the service path:
```
/fleetd.v1.DeviceService/ListDevices    # v1
/fleetd.v2.DeviceService/ListDevices    # v2
```

## Rate Limit Bypassing

For high-volume operations, request rate limit bypass:

```bash
curl -H "X-API-Key: $ADMIN_KEY" \
     -H "X-Bypass-Rate-Limit: true" \
     https://api.fleetd.io/...
```

## Debugging

Enable debug mode for detailed error information:

```bash
grpcurl -H "X-Debug: true" \
  api.fleetd.io:443 fleetd.v1.DeviceService/GetDevice
```

Debug response includes:
- Stack traces
- Database queries
- Performance metrics
- Request processing timeline