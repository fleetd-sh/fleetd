# fleetd Go SDK

Official Go SDK for fleetd - A modern fleet management platform for IoT and edge devices.

## Installation

```bash
go get fleetd.sh/sdk
```

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "log"

    "fleetd.sh/sdk"
)

func main() {
    // Initialize the client
    client, err := sdk.NewClient("https://api.fleetd.sh", sdk.Options{
        APIKey: "your-api-key",
    })
    if err != nil {
        log.Fatal(err)
    }
    defer client.Close()

    // List all devices
    ctx := context.Background()
    devices, err := client.Device.List(ctx, sdk.ListOptions{
        Limit: 10,
    })
    if err != nil {
        log.Fatal(err)
    }

    for _, device := range devices {
        fmt.Printf("Device: %s (Status: %s)\n", device.Name, device.Status)
    }
}
```

## Features

- 🚀 **Simple API** - Easy-to-use, idiomatic Go interface
- 🔐 **Secure** - Built-in authentication with API keys
- 🌐 **Connect RPC** - Modern RPC framework with HTTP/2 and streaming
- 📦 **Complete** - Full coverage of fleetd API
- 🔄 **Real-time** - Support for streaming logs and updates
- ⚡ **Performant** - Connection pooling and efficient serialization

## Usage Examples

### Device Management

```go
// Register a new device
device, err := client.Device.Register(ctx, sdk.RegisterDeviceOptions{
    Name:    "edge-device-001",
    Type:    "raspberry-pi",
    Version: "1.0.0",
    Labels: map[string]string{
        "location": "warehouse-a",
        "env":      "production",
    },
})

// Get device details
device, err := client.Device.Get(ctx, "device-id")

// Update device metadata
device, err := client.Device.Update(ctx, "device-id", map[string]interface{}{
    "status": "maintenance",
})

// Stream device logs
logStream, err := client.Device.StreamLogs(ctx, "device-id", true)
if err != nil {
    log.Fatal(err)
}

for line := range logStream {
    fmt.Println(line)
}

// Delete a device
err = client.Device.Delete(ctx, "device-id")
```

### Software Updates

```go
// Create a new update
update, err := client.Update.Create(ctx, sdk.CreateUpdateOptions{
    Version:     "2.0.0",
    Description: "Major feature release",
    BinaryID:    "binary-123",
    Rollout: &sdk.Rollout{
        Strategy:   sdk.RolloutStrategyPhased,
        Percentage: 25, // Start with 25% of devices
    },
})

// Trigger update for specific devices
err = client.Update.Trigger(ctx, update.ID, sdk.TriggerUpdateOptions{
    DeviceLabels: map[string]string{
        "env": "staging",
    },
})

// Monitor update progress
progressStream, err := client.Update.StreamProgress(ctx, "device-id")
if err != nil {
    log.Fatal(err)
}

for status := range progressStream {
    fmt.Printf("Update progress: %d%% - %s\n", status.Progress, status.Message)
}

// Rollback if needed
err = client.Update.Rollback(ctx, update.ID, []string{"device-id"})
```

### Binary Management

```go
// Upload a binary
file, err := os.Open("app-v2.0.0")
if err != nil {
    log.Fatal(err)
}
defer file.Close()

binary, err := client.Binary.Upload(ctx, file, map[string]string{
    "version":  "2.0.0",
    "platform": "linux",
    "arch":     "arm64",
})

// Download a binary
outFile, err := os.Create("downloaded-binary")
if err != nil {
    log.Fatal(err)
}
defer outFile.Close()

err = client.Binary.Download(ctx, binary.ID, outFile)
```

### Error Handling

The SDK uses Connect RPC error codes for consistent error handling:

```go
import "connectrpc.com/connect"

devices, err := client.Device.List(ctx, sdk.ListOptions{})
if err != nil {
    if connect.CodeOf(err) == connect.CodePermissionDenied {
        // Handle permission error
        log.Fatal("Invalid API key or insufficient permissions")
    }
    if connect.CodeOf(err) == connect.CodeNotFound {
        // Handle not found
        log.Fatal("Resource not found")
    }
    // Handle other errors
    log.Fatal(err)
}
```

### Custom HTTP Client

You can provide a custom HTTP client for advanced use cases:

```go
import "net/http"

httpClient := &http.Client{
    Timeout: 60 * time.Second,
    Transport: &http.Transport{
        MaxIdleConns:    100,
        IdleConnTimeout: 90 * time.Second,
    },
}

client, err := sdk.NewClient("https://api.fleetd.sh", sdk.Options{
    APIKey:     "your-api-key",
    HTTPClient: httpClient,
})
```

## API Reference

### Client

- `NewClient(baseURL string, opts Options) (*Client, error)` - Create a new client
- `Ping(ctx context.Context) error` - Check server connectivity
- `Close() error` - Close the client

### Device Service

- `Register(ctx, opts RegisterDeviceOptions) (*Device, error)` - Register a device
- `Get(ctx, deviceID string) (*Device, error)` - Get device details
- `List(ctx, opts ListOptions) ([]*Device, error)` - List devices
- `Update(ctx, deviceID string, updates map[string]interface{}) (*Device, error)` - Update device
- `Delete(ctx, deviceID string) error` - Delete device
- `Heartbeat(ctx, deviceID string) error` - Send heartbeat
- `StreamLogs(ctx, deviceID string, follow bool) (<-chan string, error)` - Stream logs

### Update Service

- `Create(ctx, opts CreateUpdateOptions) (*Update, error)` - Create update
- `Get(ctx, updateID string) (*Update, error)` - Get update details
- `List(ctx) ([]*Update, error)` - List updates
- `Trigger(ctx, updateID string, opts TriggerUpdateOptions) error` - Trigger update
- `GetStatus(ctx, deviceID string) (*UpdateStatus, error)` - Get update status
- `StreamProgress(ctx, deviceID string) (<-chan *UpdateStatus, error)` - Stream progress
- `Rollback(ctx, updateID string, deviceIDs []string) error` - Rollback update
- `Download(ctx, updateID string, writer io.Writer) error` - Download update

### Binary Service

- `Upload(ctx, reader io.Reader, metadata map[string]string) (*Binary, error)` - Upload binary
- `Download(ctx, binaryID string, writer io.Writer) error` - Download binary
- `Delete(ctx, binaryID string) error` - Delete binary

## Configuration

### Environment Variables

You can configure the SDK using environment variables:

```bash
export FLEETD_API_URL="https://api.fleetd.sh"
export FLEETD_API_KEY="your-api-key"
```

Then initialize without explicit configuration:

```go
client, err := sdk.NewClientFromEnv()
```

## Contributing

We welcome contributions! Please see our [Contributing Guide](https://github.com/fleetdsh/fleetd/blob/main/CONTRIBUTING.md) for details.

## Support

- [Documentation](https://github.com/fleetd-sh/fleetd/wiki)
- [Discord Community](https://discord.gg/fleetd)
- [GitHub Issues](https://github.com/fleetd-sh/fleetd/issues)
- [GitHub Discussions](https://github.com/fleetd-sh/fleetd/discussions)

## License

This SDK is distributed under the Apache 2.0 License. See [LICENSE](LICENSE) for more information.
