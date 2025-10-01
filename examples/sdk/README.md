# fleetd Platform SDK Examples

Simple SDK examples for integrating with fleetd platform APIs.

## Available SDKs

### Go SDK
- **Location**: `go/`
- **Usage**: Direct API client implementation
- **Features**: Full API coverage, deployment monitoring, telemetry streaming

### Node.js SDK
- **Location**: `nodejs/`
- **Usage**: JavaScript/TypeScript applications
- **Features**: Promise-based API, WebSocket streaming, deployment monitoring

## Quick Start

### Go

```bash
cd go
go mod init fleet-example
go get

# Run with environment variables
FLEET_API_URL=https://api.fleet.yourdomain.com \
FLEET_API_KEY=your-api-key \
go run main.go
```

### Node.js

```bash
cd nodejs
npm install

# Run with environment variables
FLEET_API_URL=https://api.fleet.yourdomain.com \
FLEET_API_KEY=your-api-key \
node fleet-client.js
```

## API Coverage

Both SDKs demonstrate:

- **Device Fleet Management**: Create, list, update, delete device fleets
- **Device Operations**: List devices, send commands
- **Deployments**: Create deployments, monitor progress
- **Telemetry**: Query metrics, stream real-time data

## Authentication

Use API keys for service-to-service communication:

```bash
# Generate API key using fleetctl
fleetctl api-key create --name "sdk-example" --role admin
```

## Using the SDKs

### Go Example

```go
client := NewFleetClient(apiURL, apiKey)

// Create a fleet
fleet, err := client.CreateFleet(ctx, "Production", "Main fleet", map[string]string{
    "environment": "prod",
})

// Deploy update
deployment, err := client.CreateDeployment(ctx, "v2.0", fleet.ID, manifest)

// Monitor progress
status, err := client.GetDeploymentStatus(ctx, deployment.ID)
fmt.Printf("Progress: %.1f%%\n", status.Progress.Percentage)
```

### Node.js Example

```javascript
const client = new FleetClient(apiURL, apiKey);

// Create a fleet
const fleet = await client.createFleet('Production', 'Main fleet', {
    environment: 'prod'
});

// Deploy update
const deployment = await client.createDeployment('v2.0', fleet.id, manifest);

// Monitor progress
const result = await client.waitForDeployment(deployment.id);
console.log('Deployment', result.success ? 'succeeded' : 'failed');
```

## Building Production SDKs

These examples are starting points. For production use:

1. **Error Handling**: Implement retry logic and exponential backoff
2. **Connection Pooling**: Reuse HTTP connections
3. **Rate Limiting**: Respect API rate limits
4. **Caching**: Cache frequently accessed data
5. **Logging**: Add structured logging
6. **Testing**: Write unit and integration tests
7. **Documentation**: Generate API docs from code

## OpenAPI Client Generation

Generate SDKs from OpenAPI specifications:

```bash
# Install OpenAPI Generator
npm install -g @openapitools/openapi-generator-cli

# Generate Go client
openapi-generator-cli generate \
  -i ../../docs/api/openapi-platform.yaml \
  -g go \
  -o generated/go

# Generate TypeScript client
openapi-generator-cli generate \
  -i ../../docs/api/openapi-platform.yaml \
  -g typescript-axios \
  -o generated/typescript
```

## Contributing

To add SDK examples for other languages:

1. Create a directory for the language
2. Implement basic API operations
3. Include dependency management files
4. Add usage examples
5. Document authentication and configuration

## Resources

- [Platform API Specification](../../docs/api/openapi-platform.yaml)
- [Device API Specification](../../docs/api/openapi-device.yaml)
- [Fleet Documentation](https://github.com/fleetd-sh/fleetd/wiki)