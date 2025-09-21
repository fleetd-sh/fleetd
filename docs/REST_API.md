# FleetD REST API Documentation

FleetD supports both **Connect-RPC** (default) and **REST API** (via Vanguard transcoder) protocols. This document describes the REST API endpoints.

## Enabling REST API Support

To enable REST API support, set the environment variable when starting the platform API:

```bash
export FLEETD_ENABLE_REST=true
just platform-api-rest

# Or directly:
JWT_SECRET=dev-secret FLEETD_ENABLE_REST=true go run cmd/platform-api/main.go --port 8090
```

## Authentication

All API endpoints (except health checks) require authentication via JWT token or API key.

### JWT Authentication
```bash
curl -X POST http://localhost:8090/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username": "admin", "password": "password"}'
```

Include the token in subsequent requests:
```bash
-H "Authorization: Bearer <jwt-token>"
```

### API Key Authentication
```bash
-H "X-API-Key: fld_1234567890"
```

## REST API Endpoints

The REST API endpoints are automatically generated from the protobuf definitions with HTTP annotations.

### Device Management

#### List Devices
```bash
GET /api/v1/devices
```

Query Parameters:
- `type` - Filter by device type
- `status` - Filter by status
- `page_size` - Number of results per page
- `page_token` - Pagination token

Example:
```bash
curl -X GET "http://localhost:8090/api/v1/devices?type=edge&status=online" \
  -H "Authorization: Bearer $JWT_TOKEN"
```

#### Register Device
```bash
POST /api/v1/devices/register
```

Request Body:
```json
{
  "device_id": "device-001",
  "name": "Edge Device 1",
  "type": "edge",
  "version": "1.0.0",
  "metadata": {
    "location": "datacenter-1"
  }
}
```

#### Get Device
```bash
GET /api/v1/devices/{device_id}
```

#### Update Device Status
```bash
POST /api/v1/devices/{device_id}/status
```

Request Body:
```json
{
  "status": "online",
  "metrics": {
    "cpu_usage": "45.2",
    "memory_usage": "62.1"
  }
}
```

#### Delete Device
```bash
DELETE /api/v1/devices/{device_id}
```

### Fleet Management

#### List Fleets
```bash
GET /api/v1/fleets
```

Query Parameters:
- `tags` - Filter by tags (key=value format)
- `page_size` - Number of results per page
- `page_token` - Pagination token

#### Create Fleet
```bash
POST /api/v1/fleets
```

Request Body:
```json
{
  "name": "production-fleet",
  "description": "Production edge devices",
  "tags": {
    "environment": "production",
    "region": "us-west"
  },
  "config": {
    "update_strategy": "rolling",
    "max_concurrent_updates": 10,
    "auto_rollback": true
  }
}
```

#### Get Fleet
```bash
GET /api/v1/fleets/{id}
```

#### Update Fleet
```bash
PATCH /api/v1/fleets/{id}
```

Request Body (partial update):
```json
{
  "description": "Updated description",
  "config": {
    "max_concurrent_updates": 20
  }
}
```

#### Delete Fleet
```bash
DELETE /api/v1/fleets/{id}
```

#### Get Device Logs
```bash
GET /api/v1/devices/{device_id}/logs
```

Query Parameters:
- `limit` - Maximum number of log entries
- `since` - Start timestamp (RFC3339)
- `until` - End timestamp (RFC3339)
- `levels` - Log levels (comma-separated: INFO,WARN,ERROR)

### Analytics

#### Get Device Metrics
```bash
GET /api/v1/analytics/devices/{device_id}/metrics
```

Query Parameters:
- `metric_names` - Comma-separated list of metrics
- `start_time` - Start of time range (RFC3339)
- `end_time` - End of time range (RFC3339)

#### Get Update Analytics
```bash
GET /api/v1/analytics/updates/{campaign_id}
```

#### Get Device Health
```bash
GET /api/v1/analytics/devices/{device_id}/health
```

#### Get Performance Metrics
```bash
GET /api/v1/analytics/performance
```

### Health Checks

These endpoints don't require authentication:

#### Health Check
```bash
GET /health
```

#### Liveness Check
```bash
GET /health/live
```

#### Readiness Check
```bash
GET /health/ready
```

## Protocol Comparison

FleetD supports multiple protocols through the same server:

### Connect-RPC (Default)
```bash
curl -X POST http://localhost:8090/fleetd.v1.DeviceService/ListDevices \
  -H "Content-Type: application/json" \
  -H "Connect-Protocol-Version: 1" \
  -H "Authorization: Bearer $JWT_TOKEN" \
  -d '{}'
```

### REST (via Vanguard)
```bash
curl -X GET http://localhost:8090/api/v1/devices \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $JWT_TOKEN"
```

### gRPC
```bash
grpcurl -H "Authorization: Bearer $JWT_TOKEN" \
  -d '{}' \
  localhost:8090 \
  fleetd.v1.DeviceService/ListDevices
```

## Error Responses

All endpoints return standard HTTP status codes:

- `200 OK` - Success
- `201 Created` - Resource created
- `400 Bad Request` - Invalid request
- `401 Unauthorized` - Missing or invalid authentication
- `403 Forbidden` - Insufficient permissions
- `404 Not Found` - Resource not found
- `429 Too Many Requests` - Rate limit exceeded
- `500 Internal Server Error` - Server error

Error Response Format:
```json
{
  "error": {
    "code": "INVALID_ARGUMENT",
    "message": "Device ID is required",
    "details": {
      "field": "device_id"
    }
  }
}
```

## Rate Limiting

API endpoints are rate-limited per IP address:
- Anonymous: 60 requests/hour
- Authenticated: 1000 requests/hour

Rate limit headers:
- `X-RateLimit-Limit` - Request limit
- `X-RateLimit-Remaining` - Remaining requests
- `X-RateLimit-Reset` - Reset timestamp
- `Retry-After` - Seconds to wait (when rate limited)

## Testing

Use the provided test script:

```bash
# Set JWT token (get from login endpoint)
export JWT_TOKEN="your-jwt-token"

# Run tests
./scripts/test-rest-api.sh
```

## SDK Support

The FleetD SDK supports all protocols:

```go
// Connect-RPC (default)
client, _ := sdk.NewClient("http://localhost:8090", sdk.Options{
    APIKey: "fld_...",
})

// REST endpoints are automatically handled by Vanguard
// The same SDK client works with both protocols
devices, _ := client.ListDevices(ctx, nil)
```

## Migration from Connect-RPC to REST

If migrating from Connect-RPC to REST:

1. Enable REST support with `FLEETD_ENABLE_REST=true`
2. Update client code to use REST endpoints
3. Both protocols work simultaneously during migration
4. Vanguard handles protocol translation automatically

## OpenAPI/Swagger Documentation

When REST support is enabled, OpenAPI documentation is available:

```bash
# Generate OpenAPI spec
just docs-generate

# Serve Swagger UI
just docs-serve

# Open in browser
open http://localhost:8082
```