# Data Stack Implementation Strategy

## Overview

Our implementation follows a **hybrid edge-cloud architecture** where devices store data locally using SQLite when possible, with intelligent sync to our cloud data stack. This provides resilience, reduces bandwidth, and enables offline operation.

## Device Storage Tiers

### Tier 1: Full-Featured Devices (Raspberry Pi, Industrial PCs)
- **Storage**: 8GB+ disk, 1GB+ RAM
- **Local DB**: SQLite with 7-30 days retention
- **Capabilities**: Full metrics, logs, and event storage
- **Sync**: Batch upload every 5 minutes

### Tier 2: Constrained Devices (ESP32, Arduino-class)
- **Storage**: 4-16MB flash, 320KB-520KB RAM
- **Local DB**: Ring buffer in memory, minimal SQLite
- **Capabilities**: Latest 1000 metrics only
- **Sync**: Stream directly, buffer on network loss

### Tier 3: Minimal Devices (Sensors, LoRa nodes)
- **Storage**: <1MB flash, <64KB RAM
- **Local DB**: None, memory buffer only
- **Capabilities**: Current state + last 10 readings
- **Sync**: Immediate forward, no local persistence

## On-Device Architecture

```
┌─────────────────────────────────────┐
│         Device (Tier 1)             │
├─────────────────────────────────────┤
│  Application                        │
│      ↓                              │
│  FleetD Agent                       │
│      ↓                              │
│  ┌─────────────────────────┐       │
│  │   SQLite Database       │       │
│  ├─────────────────────────┤       │
│  │ • metrics (ring table)  │       │
│  │ • events (7 days)       │       │
│  │ • logs (compressed)     │       │
│  │ • sync_queue           │       │
│  └─────────────────────────┘       │
│      ↓                              │
│  Sync Manager                       │
│      ↓                              │
│  Network Layer → Fleet Server       │
└─────────────────────────────────────┘
```

## Implementation Phases

### Phase 0: Foundation (Week 1) ✅ Current
- [x] Design data architecture
- [x] Set up Docker Compose stack
- [x] Create PostgreSQL schema with TimescaleDB
- [ ] Migrate existing SQLite code to PostgreSQL

### Phase 1: PostgreSQL Migration (Week 2)
**Goal**: Move server from SQLite to PostgreSQL

```bash
# 1. Update connection code
internal/storage/postgres.go

# 2. Migrate existing data
just migrate-to-postgres

# 3. Update configuration
DATABASE_URL=postgresql://fleetd:pass@localhost:5432/fleetd

# 4. Test with existing devices
just test-integration
```

**Tasks**:
- [ ] Create PostgreSQL storage adapter
- [ ] Implement connection pooling with PgBouncer
- [ ] Add migration tool for existing SQLite data
- [ ] Update all queries to use PostgreSQL syntax
- [ ] Add multi-tenancy with Row Level Security

### Phase 2: On-Device SQLite (Week 3)
**Goal**: Implement local storage on capable devices

```sql
-- On-device schema (SQLite)
CREATE TABLE metrics_buffer (
    id INTEGER PRIMARY KEY,
    timestamp INTEGER NOT NULL,
    metric_name TEXT NOT NULL,
    value REAL NOT NULL,
    labels TEXT, -- JSON
    synced INTEGER DEFAULT 0
);

-- Ring buffer implementation (auto-delete old data)
CREATE TRIGGER limit_metrics_buffer
AFTER INSERT ON metrics_buffer
BEGIN
    DELETE FROM metrics_buffer
    WHERE id <= (
        SELECT id FROM metrics_buffer
        ORDER BY id DESC
        LIMIT 1 OFFSET 10000  -- Keep last 10k metrics
    );
END;

CREATE TABLE sync_queue (
    id INTEGER PRIMARY KEY,
    data_type TEXT NOT NULL, -- metrics, logs, events
    payload BLOB NOT NULL,    -- Compressed batch
    created_at INTEGER NOT NULL,
    retry_count INTEGER DEFAULT 0,
    next_retry INTEGER
);
```

**Implementation**:
```go
// internal/agent/storage/sqlite.go
type DeviceStorage interface {
    StoreMetric(metric Metric) error
    GetUnsynced() ([]Metric, error)
    MarkSynced(ids []int64) error
    GetStorageInfo() StorageInfo
}

type SQLiteStorage struct {
    db *sql.DB
    maxSize int64
    retention time.Duration
}

// Auto-detect storage capability
func NewDeviceStorage() DeviceStorage {
    info := getSystemInfo()

    if info.DiskSpace > 1_000_000_000 { // 1GB+
        return NewSQLiteStorage(
            WithRetention(7 * 24 * time.Hour),
            WithMaxSize(100_000_000), // 100MB
        )
    } else if info.DiskSpace > 10_000_000 { // 10MB+
        return NewSQLiteStorage(
            WithRetention(24 * time.Hour),
            WithMaxSize(5_000_000), // 5MB
        )
    } else {
        return NewMemoryStorage(1000) // Last 1000 points
    }
}
```

### Phase 3: VictoriaMetrics Integration (Week 4)
**Goal**: Start ingesting metrics at scale

**Server-side implementation**:
```go
// internal/api/metrics_ingestion.go
func (s *Server) IngestMetrics(ctx context.Context, req *pb.MetricsBatch) error {
    // 1. Validate device authentication
    device, err := s.validateDevice(ctx)

    // 2. Transform to VictoriaMetrics format
    vmMetrics := transformToVM(req, device)

    // 3. Send to VictoriaMetrics
    err = s.vmClient.BatchIngest(ctx, vmMetrics)

    // 4. Also store aggregates in PostgreSQL
    go s.storeAggregates(ctx, req)

    return err
}
```

**Device-side sync**:
```go
// internal/agent/sync.go
func (a *Agent) syncMetrics() error {
    // Get unsynced metrics from SQLite
    metrics, err := a.storage.GetUnsynced()

    // Batch compress
    batch := compressMetrics(metrics)

    // Send to server
    err = a.client.SendMetrics(batch)

    // Mark as synced
    if err == nil {
        a.storage.MarkSynced(getIDs(metrics))
    }

    return err
}
```

### Phase 4: Loki Log Integration (Week 5)
**Goal**: Centralized log aggregation

**Device-side log shipping**:
```yaml
# fluent-bit config on device
[INPUT]
    Name              tail
    Path              /var/log/fleetd/*.log
    Tag               fleetd.*
    DB                /var/lib/fleetd/fluent-bit.db
    Buffer_Max_Size   5MB

[FILTER]
    Name              record_modifier
    Match             *
    Record device_id  ${DEVICE_ID}
    Record org_id     ${ORG_ID}

[OUTPUT]
    Name              loki
    Match             *
    Host              fleet.example.com
    Port              443
    TLS               On
    Labels            device_id=${DEVICE_ID}, org_id=${ORG_ID}

[OUTPUT]
    Name              file
    Match             *
    Path              /var/lib/fleetd/logs-buffer
    Format            json
    # Fallback for offline operation
```

### Phase 5: ClickHouse Analytics (Week 6)
**Goal**: Long-term analytics and reporting

**Data pipeline**:
```sql
-- ClickHouse schema
CREATE TABLE device_metrics_aggregated (
    date Date,
    hour DateTime,
    device_id String,
    org_id String,
    metric_name String,
    avg_value Float64,
    min_value Float64,
    max_value Float64,
    sample_count UInt32
) ENGINE = MergeTree()
PARTITION BY toYYYYMM(date)
ORDER BY (org_id, device_id, metric_name, hour);

-- Materialized view from VictoriaMetrics
CREATE MATERIALIZED VIEW metrics_hourly
ENGINE = SummingMergeTree()
AS SELECT
    toDate(timestamp) as date,
    toStartOfHour(timestamp) as hour,
    device_id,
    org_id,
    metric_name,
    avg(value) as avg_value,
    min(value) as min_value,
    max(value) as max_value,
    count() as sample_count
FROM victoria_metrics.metrics
GROUP BY date, hour, device_id, org_id, metric_name;
```

### Phase 6: Production Hardening (Week 7-8)
**Goal**: Production readiness

- [ ] Implement device capability detection
- [ ] Add automatic failover for network issues
- [ ] Create data retention policies per tier
- [ ] Implement end-to-end encryption
- [ ] Add data compression (zstd)
- [ ] Performance tuning and load testing

## Device Capability Detection

```go
// internal/agent/capability/detector.go
type DeviceCapability struct {
    Tier          int
    TotalRAM      int64
    AvailableRAM  int64
    TotalDisk     int64
    AvailableDisk int64
    CPUCores      int
    HasSQLite     bool
    HasNetwork    bool

    // Calculated
    LocalStorageSize   int64
    SyncInterval       time.Duration
    CompressionEnabled bool
    BatchSize          int
}

func DetectCapabilities() *DeviceCapability {
    cap := &DeviceCapability{}

    // Detect hardware
    cap.TotalRAM = getMemoryTotal()
    cap.TotalDisk = getDiskTotal()
    cap.CPUCores = runtime.NumCPU()

    // Determine tier
    if cap.TotalDisk > 1_000_000_000 && cap.TotalRAM > 512_000_000 {
        cap.Tier = 1 // Full featured
        cap.LocalStorageSize = 100_000_000 // 100MB for telemetry
        cap.SyncInterval = 5 * time.Minute
        cap.CompressionEnabled = true
        cap.BatchSize = 1000
        cap.HasSQLite = true
    } else if cap.TotalDisk > 10_000_000 && cap.TotalRAM > 100_000_000 {
        cap.Tier = 2 // Constrained
        cap.LocalStorageSize = 5_000_000 // 5MB
        cap.SyncInterval = 1 * time.Minute
        cap.CompressionEnabled = false
        cap.BatchSize = 100
        cap.HasSQLite = true
    } else {
        cap.Tier = 3 // Minimal
        cap.LocalStorageSize = 0 // Memory only
        cap.SyncInterval = 10 * time.Second
        cap.CompressionEnabled = false
        cap.BatchSize = 10
        cap.HasSQLite = false
    }

    return cap
}
```

## Data Sync Protocol

```protobuf
// proto/fleetd/v1/sync.proto
message SyncRequest {
    string device_id = 1;
    DeviceCapability capability = 2;

    oneof data {
        MetricsBatch metrics = 3;
        LogsBatch logs = 4;
        EventsBatch events = 5;
    }

    SyncMetadata metadata = 6;
}

message SyncMetadata {
    int64 sequence_number = 1;
    int64 timestamp = 2;
    bool compressed = 3;
    string compression_algo = 4; // zstd, gzip, none
    int32 retry_count = 5;
    bytes checksum = 6;
}

message SyncResponse {
    bool success = 1;
    int64 last_sequence_ack = 2;
    SyncConfig config_update = 3; // Server can adjust sync params
}

message SyncConfig {
    int32 batch_size = 1;
    int32 sync_interval_seconds = 2;
    int32 retention_hours = 3;
    bool compression_enabled = 4;
}
```

## Network Resilience

```go
// internal/agent/sync/resilient.go
type ResilientSyncer struct {
    client     FleetClient
    storage    DeviceStorage
    queue      *SyncQueue
    capability *DeviceCapability
}

func (s *ResilientSyncer) Start() {
    ticker := time.NewTicker(s.capability.SyncInterval)

    for range ticker.C {
        if err := s.syncBatch(); err != nil {
            s.handleSyncError(err)
        }
    }
}

func (s *ResilientSyncer) handleSyncError(err error) {
    if isNetworkError(err) {
        // Exponential backoff
        s.queue.ScheduleRetry(
            time.Now().Add(s.calculateBackoff()),
        )

        // Switch to local storage if space available
        if s.capability.HasSQLite {
            s.storage.EnableOfflineMode()
        }
    }
}
```

## Migration Tools

```bash
# Create migration tool
just create-migration "migrate_to_postgres"

# Test migration with sample data
just test-migration --sample-size=1000

# Run migration with progress
just migrate-to-postgres --batch-size=10000 --progress

# Verify migration
just verify-migration --check-counts --check-integrity
```

## Rollout Strategy

### 1. Canary Deployment (5% of devices)
- Deploy to test devices first
- Monitor metrics ingestion rates
- Check SQLite sync performance
- Validate data integrity

### 2. Gradual Rollout (25%, 50%, 100%)
- Roll out by device tier
- Start with Tier 1 (full-featured)
- Monitor resource usage
- Adjust sync parameters

### 3. Performance Metrics to Monitor
- Ingestion rate (points/sec)
- Sync success rate
- Local storage usage
- Network bandwidth
- Query latency (P50, P95, P99)

## Success Criteria

- [ ] 99.9% data delivery from device to storage
- [ ] <1% storage overhead on constrained devices
- [ ] <5MB/day bandwidth per device
- [ ] Automatic recovery from 24-hour network outage
- [ ] 5-second query response time for 30-day data
- [ ] Zero data loss during migration

## Troubleshooting Guide

### Device won't sync
1. Check capability detection: `fleetd debug capabilities`
2. Verify SQLite health: `fleetd debug storage`
3. Check sync queue: `fleetd debug queue`
4. Test connectivity: `fleetd debug ping`

### High bandwidth usage
1. Enable compression: `fleetd config set compression=zstd`
2. Increase batch interval: `fleetd config set sync_interval=10m`
3. Reduce metric frequency: `fleetd config set metric_interval=60s`

### Storage full on device
1. Reduce retention: `fleetd config set retention=24h`
2. Disable log storage: `fleetd config set store_logs=false`
3. Clear old data: `fleetd maintenance clean`