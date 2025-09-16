# FleetD Sync Manager

## Overview

The Sync Manager is a critical component that handles data synchronization between edge devices and the FleetD cloud infrastructure. It provides resilient, efficient, and adaptive data transfer based on device capabilities.

## Architecture

```
┌─────────────────────────────────────┐
│            Device                    │
├─────────────────────────────────────┤
│  Metrics Generator                   │
│         ↓                            │
│  Storage Layer (SQLite/Memory)       │
│         ↓                            │
│  ┌─────────────────────────────┐    │
│  │      Sync Manager            │    │
│  ├─────────────────────────────┤    │
│  │ • Capability Detection       │    │
│  │ • Batch Processing           │    │
│  │ • Compression (zstd/gzip)    │    │
│  │ • Retry Logic                │    │
│  │ • Network Resilience         │    │
│  └─────────────────────────────┘    │
│         ↓                            │
│  Sync Client (Connect RPC)           │
└─────────────────────────────────────┘
                 ↓
         [Network - HTTPS]
                 ↓
┌─────────────────────────────────────┐
│         Fleet Server                 │
├─────────────────────────────────────┤
│  Sync Service Handler                │
│         ↓                            │
│  Data Router                         │
│    ├── PostgreSQL (metadata)         │
│    ├── VictoriaMetrics (metrics)     │
│    ├── Loki (logs)                   │
│    └── ClickHouse (analytics)        │
└─────────────────────────────────────┘
```

## Key Features

### 1. Adaptive Device Tiers

The system automatically detects device capabilities and adjusts behavior:

| Tier | Device Type | Storage | Sync Interval | Compression | Batch Size |
|------|------------|---------|---------------|-------------|------------|
| 1 | Full (RPi) | 100MB SQLite | 5 min | zstd | 1000 |
| 2 | Constrained (ESP32) | 5MB SQLite | 1 min | gzip | 100 |
| 3 | Minimal (Sensor) | Memory only | 10 sec | none | 10 |

### 2. Network Resilience

- **Exponential Backoff**: Automatic retry with increasing delays
- **Circuit Breaker**: Prevents cascading failures
- **Offline Queue**: SQLite-based queue for network outages
- **Compression**: 5-10x data reduction with zstd/gzip
- **Rate Limiting**: Prevents overwhelming constrained devices

### 3. Data Routing

Different data types are automatically routed to appropriate backends:

- **Metrics** → VictoriaMetrics (time-series)
- **Logs** → Loki (log aggregation)
- **Metadata** → PostgreSQL (relational)
- **Analytics** → ClickHouse (OLAP)

## Usage

### Basic Setup

```go
// Detect capabilities
cap := capability.DetectCapabilities()

// Create storage
storage, err := cap.CreateStorage("/var/lib/fleetd")

// Create sync client
client := sync.NewConnectSyncClient(
    "https://fleet.example.com",
    "api-key",
)

// Create sync manager
manager := sync.NewManager(
    storage,
    client,
    cap,
    &sync.SyncConfig{
        DeviceID: "device-001",
        OrgID:    "org-123",
    },
)

// Start syncing
manager.Start(ctx)
```

### Storing Metrics

```go
// Store a metric
storage.StoreMetric(storage.Metric{
    Name:      "cpu_usage",
    Value:     75.5,
    Timestamp: time.Now(),
    Labels: map[string]string{
        "core": "0",
    },
})

// Metrics are automatically synced based on device tier
```

### Manual Sync Trigger

```go
// Force immediate sync
manager.TriggerSync()

// Get sync status
metrics := manager.GetMetrics()
fmt.Printf("Synced: %d metrics, %d bytes\n",
    metrics.MetricsSynced,
    metrics.BytesSent)
```

## Configuration

### Environment Variables

```bash
# Server configuration
FLEET_SERVER_URL=https://fleet.example.com
FLEET_API_KEY=your-api-key
DEVICE_ID=device-001

# Override auto-detection (optional)
SYNC_INTERVAL=60s
BATCH_SIZE=500
COMPRESSION_TYPE=zstd
MAX_RETRIES=5
```

### Sync Configuration

```go
config := &sync.SyncConfig{
    DeviceID:           "device-001",
    OrgID:              "org-123",
    SyncInterval:       5 * time.Minute,
    BatchSize:          1000,
    CompressionEnabled: true,
    CompressionType:    "zstd",
    MaxRetries:         3,
    InitialBackoff:     1 * time.Second,
    MaxBackoff:         5 * time.Minute,
    BackoffMultiplier:  2.0,
}
```

## Monitoring

### Metrics Exposed

- `metrics_synced`: Total metrics successfully synced
- `logs_synced`: Total logs successfully synced
- `bytes_sent`: Total bytes sent (compressed)
- `bytes_compressed`: Original size before compression
- `compression_ratio`: Compression efficiency
- `successful_syncs`: Number of successful sync cycles
- `failed_syncs`: Number of failed sync cycles
- `consecutive_failures`: Current failure streak
- `last_sync_duration`: Time taken for last sync

### Health Checks

```go
// Check sync health
info := storage.GetStorageInfo()
if info.UnsyncedMetrics > 10000 {
    log.Warn("Large backlog detected")
}

// Check storage usage
if info.StorageBytes > cap.LocalStorageSize * 0.9 {
    log.Warn("Storage nearly full")
}
```

## Error Handling

### Retry Logic

```
Attempt 1: Wait 1s
Attempt 2: Wait 2s (with jitter)
Attempt 3: Wait 4s (with jitter)
...
Max wait: 5 minutes
```

### Network Failures

1. **Transient**: Automatic retry with backoff
2. **Extended**: Switch to offline mode, queue data
3. **Recovery**: Flush queue when connection restored

### Storage Full

1. **Tier 1-2**: Delete oldest synced data
2. **Tier 3**: Overwrite oldest data (ring buffer)

## Performance

### Compression Ratios

| Data Type | zstd | gzip | Reduction |
|-----------|------|------|-----------|
| Metrics | 8:1 | 5:1 | 80-87% |
| Logs | 10:1 | 6:1 | 83-90% |
| JSON | 12:1 | 7:1 | 86-92% |

### Bandwidth Usage

| Tier | Raw Data/Day | Compressed | Network Usage |
|------|--------------|------------|---------------|
| 1 | 100MB | 10MB | 2 Kbps avg |
| 2 | 10MB | 2MB | 0.2 Kbps avg |
| 3 | 1MB | 500KB | 0.05 Kbps avg |

### Sync Latency

- P50: <100ms
- P95: <500ms
- P99: <2s

## Security

### Encryption

- **Transport**: TLS 1.3 mandatory
- **At Rest**: Optional AES-256 for SQLite
- **API Keys**: Per-device, rotatable

### Authentication

```go
// API key in header
client := sync.NewConnectSyncClient(
    serverURL,
    apiKey, // Sent as X-API-Key header
)
```

## Troubleshooting

### Common Issues

#### High Memory Usage
```bash
# Check buffer sizes
fleetd debug storage

# Reduce batch size
export BATCH_SIZE=100
```

#### Sync Failures
```bash
# Check connectivity
fleetd debug ping

# View sync logs
fleetd logs sync

# Force sync
fleetd sync now
```

#### Storage Full
```bash
# Check storage
fleetd storage info

# Clean old data
fleetd storage clean --older-than 7d
```

## Future Enhancements

1. **Delta Compression**: Send only changed values
2. **Edge Analytics**: Process data locally before sync
3. **Predictive Sync**: ML-based optimal sync timing
4. **P2P Sync**: Device-to-device data sharing
5. **Selective Sync**: Priority-based data transmission

## Example Implementation

See `/examples/sync_agent.go` for a complete working example of a device agent using the sync manager.