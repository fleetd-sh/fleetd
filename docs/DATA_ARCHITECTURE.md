# FleetD Data Architecture

## Overview

FleetD handles multiple data types from thousands of edge devices, each requiring specialized storage solutions for optimal performance and scalability.

## Data Types & Storage Solutions

### 1. Relational Data → PostgreSQL + TimescaleDB

**Data Types:**
- Device registry (ID, name, metadata, API keys)
- User accounts and authentication
- Organizations and tenancy
- Update campaigns and configurations
- Webhook configurations

**Why PostgreSQL:**
- ACID compliance for critical data
- Rich querying capabilities
- Foreign key constraints
- Row-level security for multi-tenancy

**TimescaleDB Extension:**
- Automatic partitioning for time-series tables
- Compression for historical data
- Continuous aggregates for real-time dashboards

### 2. Time-Series Metrics → VictoriaMetrics

**Data Types:**
- Device metrics (CPU, memory, disk, network)
- Performance counters
- Custom application metrics
- Real-time telemetry

**Why VictoriaMetrics:**
- Prometheus-compatible (drop-in replacement)
- 10x better compression than Prometheus
- Handles millions of active time series
- Built-in downsampling and retention policies
- MetricsQL for advanced queries

**Data Flow:**
```
Device → Agent → Fleet Server → VictoriaMetrics → Grafana
                                      ↓
                              Long-term storage
                                 (ClickHouse)
```

### 3. Logs → Grafana Loki

**Data Types:**
- Device system logs
- Application logs
- Audit trails
- Error logs and stack traces

**Why Loki:**
- Indexes metadata, not content (cost-effective)
- Native Grafana integration
- Kubernetes-native if needed
- LogQL query language
- Multi-tenancy support

### 4. Analytics → ClickHouse

**Data Types:**
- Aggregated metrics for reporting
- Historical trends
- Capacity planning data
- Business intelligence queries

**Why ClickHouse:**
- Columnar storage (100x compression)
- Sub-second OLAP queries
- SQL interface
- Materialized views for pre-aggregation
- Excellent for time-series analytics

## Data Pipeline Architecture

```mermaid
graph TB
    subgraph "Edge Devices"
        D1[Device 1]
        D2[Device 2]
        DN[Device N]
    end

    subgraph "Fleet Server"
        API[API Gateway]
        DP[Data Processor]
    end

    subgraph "Storage Layer"
        PG[(PostgreSQL<br/>+ TimescaleDB)]
        VM[(VictoriaMetrics)]
        LOKI[(Loki)]
        CH[(ClickHouse)]
        REDIS[(Valkey/Redis)]
    end

    subgraph "Query Layer"
        GF[Grafana]
        WEB[Web UI]
        ANALYTICS[Analytics API]
    end

    D1 --> API
    D2 --> API
    DN --> API

    API --> DP

    DP -->|Metadata| PG
    DP -->|Metrics| VM
    DP -->|Logs| LOKI
    DP -->|Sessions| REDIS

    VM -->|Downsample| CH
    PG -->|Archive| CH

    GF --> VM
    GF --> LOKI
    GF --> PG

    WEB --> API
    WEB --> PG

    ANALYTICS --> CH
    ANALYTICS --> PG
```

## Implementation Phases

### Phase 1: PostgreSQL Migration (Immediate)
- Migrate from SQLite to PostgreSQL
- Add TimescaleDB extension
- Set up connection pooling with PgBouncer
- Implement proper migrations

### Phase 2: Metrics Pipeline (Week 1)
- Deploy VictoriaMetrics
- Configure remote write from agents
- Set up retention policies
- Create Grafana dashboards

### Phase 3: Log Aggregation (Week 2)
- Deploy Loki
- Configure Promtail/Fluent Bit on devices
- Set up log forwarding
- Create log exploration dashboards

### Phase 4: Analytics Platform (Week 3-4)
- Deploy ClickHouse
- Set up data pipelines from VictoriaMetrics
- Create materialized views
- Build analytics API

## Storage Estimates

For 10,000 devices reporting every 30 seconds:

### Metrics (VictoriaMetrics)
- Raw: ~100 metrics/device × 10,000 devices = 1M data points/30s
- Storage: ~2 bytes/point after compression = 5.7 GB/day
- 1-year retention: ~2 TB

### Logs (Loki)
- Assuming 1 KB/log line, 100 lines/minute/device
- Storage: ~1.4 TB/day (before compression)
- With 10:1 compression: ~140 GB/day
- 30-day retention: ~4.2 TB

### PostgreSQL
- Device metadata: ~10 KB/device = 100 MB
- Configurations: ~1 GB
- Total: <10 GB for core data

### ClickHouse (Analytics)
- Downsampled metrics: ~200 GB/year
- Aggregated reports: ~50 GB/year

## Configuration Management

### Environment Variables

```env
# PostgreSQL
DATABASE_URL=postgresql://user:pass@localhost:5432/fleetd
DATABASE_POOL_SIZE=25

# TimescaleDB
TIMESCALE_CHUNK_INTERVAL=1d
TIMESCALE_RETENTION_DAYS=30

# VictoriaMetrics
VICTORIA_METRICS_URL=http://localhost:8428
METRICS_RETENTION_DAYS=365
METRICS_DOWNSAMPLING=5m:30d,1h:90d,1d:1y

# Loki
LOKI_URL=http://localhost:3100
LOKI_RETENTION_DAYS=30

# ClickHouse
CLICKHOUSE_URL=http://localhost:8123
CLICKHOUSE_DATABASE=fleetd_analytics

# Valkey/Redis
VALKEY_ADDR=localhost:6379
```

## Monitoring & Alerting

### Key Metrics to Monitor
1. **Ingestion Rate**: Points/second into VictoriaMetrics
2. **Query Latency**: P95 response times
3. **Storage Growth**: GB/day per system
4. **Device Check-in Rate**: Devices/minute
5. **Error Rates**: Failed writes, timeouts

### Alerting Rules
- Device offline > 5 minutes
- Ingestion rate drop > 20%
- Storage usage > 80%
- Query latency > 1s (P95)
- Failed writes > 1%

## Security Considerations

### Data Encryption
- TLS for all data in transit
- Encryption at rest for sensitive data
- API key rotation every 90 days

### Access Control
- Row-level security in PostgreSQL
- Tenant isolation in VictoriaMetrics
- Label-based access in Loki

### Compliance
- GDPR: 30-day log retention, right to deletion
- Data residency: Regional deployments
- Audit logging: All access logged

## Backup & Recovery

### Backup Strategy
- PostgreSQL: Daily full + hourly incremental
- VictoriaMetrics: Daily snapshots
- Loki: S3 backend with versioning
- ClickHouse: Weekly full backups

### Recovery Targets
- RPO (Recovery Point Objective): 1 hour
- RTO (Recovery Time Objective): 4 hours

## Cost Optimization

### Storage Tiering
1. **Hot** (0-7 days): NVMe SSD
2. **Warm** (7-30 days): Standard SSD
3. **Cold** (30+ days): Object storage (S3)

### Data Retention Policies
- Raw metrics: 30 days
- Downsampled metrics: 1 year
- Logs: 30 days (configurable)
- Analytics: 2 years

## Future Considerations

### Scaling Beyond 100K Devices
- Kafka for event streaming
- Multiple VictoriaMetrics clusters
- ClickHouse sharding
- Edge aggregation nodes

### ML/AI Integration
- Anomaly detection on metrics
- Predictive maintenance
- Automated root cause analysis

### Edge Computing
- Local metric aggregation
- Edge-based alerting
- Federated learning