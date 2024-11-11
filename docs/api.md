# FleetD API Documentation

FleetD provides a gRPC API for managing devices, binaries, updates, and analytics. This document describes the available services and their methods.

## Authentication

All API requests must include an API key in the `x-api-key` metadata header. API keys can be obtained through device registration or created in the admin interface.

## Services

### Device Service

The Device Service manages device registration, heartbeats, and status reporting.

#### Register

Registers a new device with the fleet.

```protobuf
rpc Register(RegisterRequest) returns (RegisterResponse);

message RegisterRequest {
  string name = 1;
  string type = 2;
  string version = 3;
  map<string, string> capabilities = 4;
}

message RegisterResponse {
  string device_id = 1;
  string api_key = 2;
}
```

Example using Go SDK:
```go
client := fleetd.NewClient(fleetd.ClientConfig{
    Address: "localhost:8080",
})

resp, err := client.Device().Register(ctx, fleetd.RegisterRequest{
    Name:    "my-device",
    Type:    "raspberry-pi",
    Version: "1.0.0",
    Capabilities: fleetd.Metadata{
        "feature1": "enabled",
    },
})
```

#### Heartbeat

Sends a periodic heartbeat and receives pending actions.

```protobuf
rpc Heartbeat(HeartbeatRequest) returns (HeartbeatResponse);

message HeartbeatRequest {
  string device_id = 1;
  map<string, string> metrics = 2;
}

message HeartbeatResponse {
  bool has_update = 1;
}
```

Example using Go SDK:
```go
resp, err := client.Device().Heartbeat(ctx, fleetd.HeartbeatRequest{
    DeviceID: "device-123",
    Metrics: fleetd.Metadata{
        "cpu": "50%",
        "memory": "2GB",
    },
})
```

#### Report Status

Updates device status and metrics.

```protobuf
rpc ReportStatus(ReportStatusRequest) returns (ReportStatusResponse);

message ReportStatusRequest {
  string device_id = 1;
  string status = 2;
  map<string, string> metrics = 3;
}

message ReportStatusResponse {
  bool success = 1;
}
```

Example using Go SDK:
```go
resp, err := client.Device().ReportStatus(ctx, fleetd.ReportStatusRequest{
    DeviceID: "device-123",
    Status:   "healthy",
    Metrics: fleetd.Metadata{
        "uptime": "24h",
    },
})
```

### Binary Service

The Binary Service manages binary uploads, downloads, and distribution.

#### Upload Binary

Uploads a new binary to the fleet.

```protobuf
rpc UploadBinary(stream UploadBinaryRequest) returns (UploadBinaryResponse);

message UploadBinaryRequest {
  oneof data {
    BinaryMetadata metadata = 1;
    bytes chunk = 2;
  }
}

message UploadBinaryResponse {
  string id = 1;
  string sha256 = 2;
}
```

Example using Go SDK:
```go
file, _ := os.Open("binary.tar.gz")
defer file.Close()

resp, err := client.Binary().Upload(ctx, fleetd.UploadRequest{
    Name:         "my-app",
    Version:      "1.0.0",
    Platform:     "linux",
    Architecture: "amd64",
    Reader:       file,
})
```

#### Download Binary

Downloads a binary from the fleet.

```protobuf
rpc DownloadBinary(DownloadBinaryRequest) returns (stream DownloadBinaryResponse);

message DownloadBinaryRequest {
  string id = 1;
}

message DownloadBinaryResponse {
  bytes chunk = 1;
}
```

Example using Go SDK:
```go
file, _ := os.Create("binary.tar.gz")
defer file.Close()

err := client.Binary().Download(ctx, "binary-123", file)
```

### Update Service

The Update Service manages fleet-wide updates and campaigns.

#### Create Update Campaign

Creates a new update campaign.

```protobuf
rpc CreateUpdateCampaign(CreateUpdateCampaignRequest) returns (CreateUpdateCampaignResponse);

message CreateUpdateCampaignRequest {
  string name = 1;
  string description = 2;
  string binary_id = 3;
  string target_version = 4;
  repeated string target_platforms = 5;
  repeated string target_architectures = 6;
  map<string, string> target_metadata = 7;
  UpdateStrategy strategy = 8;
}

message CreateUpdateCampaignResponse {
  string campaign_id = 1;
}
```

Example using Go SDK:
```go
resp, err := client.Update().CreateCampaign(ctx, fleetd.CreateCampaignRequest{
    Name:        "Update to 1.0.0",
    Description: "Rolling update to version 1.0.0",
    BinaryID:    "binary-123",
    Strategy:    fleetd.UpdateStrategyRolling,
    Targets: fleetd.UpdateTargets{
        Platforms:     []string{"linux"},
        Architectures: []string{"amd64", "arm64"},
    },
})
```

### Analytics Service

The Analytics Service provides metrics and insights about devices and updates.

#### Get Device Metrics

Gets device metrics over time.

```protobuf
rpc GetDeviceMetrics(GetDeviceMetricsRequest) returns (GetDeviceMetricsResponse);

message GetDeviceMetricsRequest {
  string device_id = 1;
  repeated string metric_names = 2;
  TimeRange time_range = 3;
  string aggregation = 4;
}

message GetDeviceMetricsResponse {
  repeated MetricSeries metrics = 1;
}
```

Example using Go SDK:
```go
resp, err := client.Analytics().GetDeviceMetrics(ctx, fleetd.GetDeviceMetricsRequest{
    DeviceID: "device-123",
    Metrics:  []string{"cpu", "memory"},
    TimeRange: fleetd.TimeRange{
        StartTime: time.Now().Add(-24 * time.Hour),
        EndTime:   time.Now(),
    },
    Aggregation: "avg",
})
```

### Webhook Service

The Webhook Service manages webhook subscriptions and deliveries.

#### Subscribe

Creates a new webhook subscription.

```protobuf
rpc Subscribe(SubscribeRequest) returns (SubscribeResponse);

message SubscribeRequest {
  string url = 1;
  string secret = 2;
  repeated string events = 3;
  map<string, string> headers = 4;
}

message SubscribeResponse {
  string webhook_id = 1;
}
```

Example using Go SDK:
```go
resp, err := client.Webhook().Subscribe(ctx, fleetd.SubscribeRequest{
    URL:    "https://example.com/webhook",
    Secret: "webhook-secret",
    Events: []string{"device.registered", "update.completed"},
})
```

## Error Handling

All API methods return standard gRPC status codes:

- `OK` (0): Success
- `INVALID_ARGUMENT` (3): Invalid request parameters
- `NOT_FOUND` (5): Resource not found
- `ALREADY_EXISTS` (6): Resource already exists
- `PERMISSION_DENIED` (7): Invalid or missing API key
- `RESOURCE_EXHAUSTED` (8): Rate limit exceeded
- `FAILED_PRECONDITION` (9): Operation prerequisites not met
- `INTERNAL` (13): Internal server error

Example error handling using Go SDK:
```go
resp, err := client.Device().GetDevice(ctx, "nonexistent")
if err != nil {
    if e, ok := err.(*fleetd.Error); ok {
        switch e.Code {
        case codes.NotFound:
            // Handle not found
        case codes.PermissionDenied:
            // Handle auth error
        default:
            // Handle other errors
        }
    }
}
```

## Rate Limiting

API requests are rate limited per API key. The default limits are:

- 100 requests per second per API key
- 1000 requests per minute per API key
- 10000 requests per hour per API key

Rate limit headers are included in responses:
- `X-RateLimit-Limit`: Request limit
- `X-RateLimit-Remaining`: Remaining requests
- `X-RateLimit-Reset`: Time until limit resets

## Pagination

List operations support pagination using `page_size` and `page_token` parameters:

```go
var devices []*fleetd.Device
pageToken := ""

for {
    resp, err := client.Device().ListDevices(ctx, fleetd.ListDevicesRequest{
        PageSize:  100,
        PageToken: pageToken,
    })
    if err != nil {
        break
    }
    
    devices = append(devices, resp.Devices...)
    
    if resp.NextPageToken == "" {
        break
    }
    pageToken = resp.NextPageToken
}
```

## Webhook Events

Webhook payloads are signed using HMAC-SHA256. Verify signatures using:

```go
verifier := fleetd.NewSignatureVerifier(webhookSecret)
err := verifier.Verify(body, signature)
```

Event payload format:
```json
{
    "id": "evt_123",
    "type": "device.registered",
    "timestamp": "2023-12-01T12:00:00Z",
    "data": {
        "device_id": "device-123",
        "name": "my-device",
        "type": "raspberry-pi"
    }
}
``` 