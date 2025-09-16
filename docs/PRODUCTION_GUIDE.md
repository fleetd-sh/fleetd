# FleetD Production Deployment Guide

## Table of Contents
- [Overview](#overview)
- [System Requirements](#system-requirements)
- [Architecture](#architecture)
- [Deployment](#deployment)
- [Configuration](#configuration)
- [Security](#security)
- [Monitoring](#monitoring)
- [Troubleshooting](#troubleshooting)

## Overview

FleetD is a production-ready IoT fleet management system built with enterprise-grade features including:
- **Error Handling**: Circuit breakers, retry logic, and panic recovery
- **Observability**: OpenTelemetry tracing, Prometheus metrics, and health checks
- **Security**: mTLS, JWT authentication, and RBAC authorization
- **Scalability**: Connect RPC, connection pooling, and resource isolation

## System Requirements

### Server Requirements
- **OS**: Linux (Ubuntu 20.04+, RHEL 8+, Debian 11+)
- **CPU**: Minimum 4 cores, recommended 8+ cores
- **RAM**: Minimum 8GB, recommended 16GB+
- **Storage**: 100GB+ SSD (depends on fleet size)
- **Network**: 1Gbps+ network connection

### Device Requirements
- **Raspberry Pi**: Model 3B+ or newer
- **ESP32**: 4MB+ flash, 320KB+ RAM
- **Other IoT**: Linux-based with 512MB+ RAM

### Database Requirements
- **PostgreSQL**: Version 13+ (production)
- **SQLite**: For development/testing only

## Architecture

### Component Overview

```
┌─────────────────────────────────────────────────────────┐
│                     Load Balancer                        │
│                    (HAProxy/Nginx)                       │
└─────────────────┬───────────────────────────────────────┘
                  │
┌─────────────────┴───────────────────────────────────────┐
│                   API Gateway Layer                      │
│          (Connect RPC + HTTP/2 + WebSocket)             │
├──────────────────────────────────────────────────────────┤
│                  Fleet Server Cluster                    │
│  ┌────────────┐  ┌────────────┐  ┌────────────┐       │
│  │  Server 1  │  │  Server 2  │  │  Server N  │       │
│  └────────────┘  └────────────┘  └────────────┘       │
├──────────────────────────────────────────────────────────┤
│                    Data Layer                            │
│  ┌────────────┐  ┌────────────┐  ┌────────────┐       │
│  │ PostgreSQL │  │   Redis    │  │  S3/Minio  │       │
│  └────────────┘  └────────────┘  └────────────┘       │
├──────────────────────────────────────────────────────────┤
│                 Observability Stack                      │
│  ┌────────────┐  ┌────────────┐  ┌────────────┐       │
│  │ Prometheus │  │   Jaeger   │  │    Loki    │       │
│  └────────────┘  └────────────┘  └────────────┘       │
└──────────────────────────────────────────────────────────┘
```

### High Availability Setup

#### Multi-Server Deployment
```yaml
# docker-compose.prod.yml
version: '3.8'

services:
  fleetd-1:
    image: fleetd:latest
    environment:
      - NODE_ID=1
      - CLUSTER_PEERS=fleetd-2,fleetd-3
      - DB_HOST=postgres
      - REDIS_HOST=redis
    deploy:
      resources:
        limits:
          cpus: '2'
          memory: 4G
        reservations:
          cpus: '1'
          memory: 2G

  fleetd-2:
    image: fleetd:latest
    environment:
      - NODE_ID=2
      - CLUSTER_PEERS=fleetd-1,fleetd-3
      - DB_HOST=postgres
      - REDIS_HOST=redis

  fleetd-3:
    image: fleetd:latest
    environment:
      - NODE_ID=3
      - CLUSTER_PEERS=fleetd-1,fleetd-2
      - DB_HOST=postgres
      - REDIS_HOST=redis

  postgres:
    image: postgres:15
    environment:
      - POSTGRES_REPLICATION_MODE=master
      - POSTGRES_REPLICATION_USER=replicator
      - POSTGRES_REPLICATION_PASSWORD=${DB_REPL_PASSWORD}
    volumes:
      - postgres_data:/var/lib/postgresql/data

  postgres-replica:
    image: postgres:15
    environment:
      - POSTGRES_REPLICATION_MODE=slave
      - POSTGRES_MASTER_HOST=postgres
      - POSTGRES_REPLICATION_USER=replicator
      - POSTGRES_REPLICATION_PASSWORD=${DB_REPL_PASSWORD}

  redis:
    image: redis:7-alpine
    command: redis-server --appendonly yes
    volumes:
      - redis_data:/data
```

## Deployment

### 1. Prerequisites Installation

```bash
# Install Docker and Docker Compose
curl -fsSL https://get.docker.com | bash
sudo usermod -aG docker $USER

# Install PostgreSQL client tools
sudo apt-get update
sudo apt-get install -y postgresql-client

# Install monitoring tools
sudo apt-get install -y prometheus-node-exporter
```

### 2. Environment Configuration

Create `.env.production` file:
```env
# Database
DATABASE_URL=postgresql://fleetd:password@localhost:5432/fleetd
DATABASE_POOL_SIZE=50
DATABASE_MAX_IDLE=10

# Redis
REDIS_URL=redis://localhost:6379
REDIS_POOL_SIZE=100

# Security
JWT_SECRET=$(openssl rand -base64 32)
TLS_CERT_PATH=/etc/fleetd/certs/server.crt
TLS_KEY_PATH=/etc/fleetd/certs/server.key
TLS_CA_PATH=/etc/fleetd/certs/ca.crt
ENABLE_MTLS=true
ENABLE_RBAC=true

# Observability
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317
METRICS_PORT=9090
ENABLE_TRACING=true
ENABLE_METRICS=true
TRACE_SAMPLE_RATE=0.1

# Fleet Configuration
MAX_DEVICES_PER_NODE=10000
HEARTBEAT_INTERVAL=60s
HEARTBEAT_TIMEOUT=180s
UPDATE_CHECK_INTERVAL=300s

# Circuit Breaker
CIRCUIT_BREAKER_MAX_FAILURES=5
CIRCUIT_BREAKER_TIMEOUT=30s
CIRCUIT_BREAKER_INTERVAL=60s

# Rate Limiting
RATE_LIMIT_RPS=100
RATE_LIMIT_BURST=200
```

### 3. Certificate Generation

```bash
# Generate CA certificate
openssl genrsa -out ca.key 4096
openssl req -new -x509 -days 3650 -key ca.key -out ca.crt \
  -subj "/C=US/ST=CA/L=SF/O=FleetD/CN=FleetD CA"

# Generate server certificate
openssl genrsa -out server.key 4096
openssl req -new -key server.key -out server.csr \
  -subj "/C=US/ST=CA/L=SF/O=FleetD/CN=fleetd.example.com"
openssl x509 -req -days 365 -in server.csr -CA ca.crt -CAkey ca.key \
  -CAcreateserial -out server.crt

# Generate client certificate for devices
openssl genrsa -out device.key 4096
openssl req -new -key device.key -out device.csr \
  -subj "/C=US/ST=CA/L=SF/O=FleetD/CN=device-001"
openssl x509 -req -days 365 -in device.csr -CA ca.crt -CAkey ca.key \
  -CAcreateserial -out device.crt

# Install certificates
sudo mkdir -p /etc/fleetd/certs
sudo cp *.crt *.key /etc/fleetd/certs/
sudo chmod 600 /etc/fleetd/certs/*.key
```

### 4. Database Setup

```sql
-- Create database and user
CREATE DATABASE fleetd;
CREATE USER fleetd WITH ENCRYPTED PASSWORD 'secure_password';
GRANT ALL PRIVILEGES ON DATABASE fleetd TO fleetd;

-- Enable required extensions
\c fleetd
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pg_stat_statements";

-- Create read-only user for monitoring
CREATE USER fleetd_reader WITH ENCRYPTED PASSWORD 'reader_password';
GRANT CONNECT ON DATABASE fleetd TO fleetd_reader;
GRANT USAGE ON SCHEMA public TO fleetd_reader;
GRANT SELECT ON ALL TABLES IN SCHEMA public TO fleetd_reader;
```

### 5. Service Deployment

```bash
# Using systemd
sudo cat > /etc/systemd/system/fleetd.service <<EOF
[Unit]
Description=FleetD Server
After=network.target postgresql.service redis.service

[Service]
Type=simple
User=fleetd
Group=fleetd
WorkingDirectory=/opt/fleetd
ExecStart=/opt/fleetd/fleetd server
Restart=always
RestartSec=10
LimitNOFILE=65536
Environment="ENV_FILE=/opt/fleetd/.env.production"

# Security
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/log/fleetd /var/lib/fleetd

[Install]
WantedBy=multi-user.target
EOF

# Start service
sudo systemctl daemon-reload
sudo systemctl enable fleetd
sudo systemctl start fleetd
```

## Configuration

### Server Configuration

```yaml
# config/production.yaml
server:
  host: 0.0.0.0
  port: 8080
  grpc_port: 9090
  max_connections: 10000
  read_timeout: 30s
  write_timeout: 30s
  idle_timeout: 120s

database:
  driver: postgres
  dsn: ${DATABASE_URL}
  max_open_conns: 50
  max_idle_conns: 10
  conn_max_lifetime: 5m
  migrations_path: /opt/fleetd/migrations

security:
  enable_tls: true
  enable_mtls: true
  tls_cert_path: ${TLS_CERT_PATH}
  tls_key_path: ${TLS_KEY_PATH}
  tls_ca_path: ${TLS_CA_PATH}
  min_tls_version: "1.3"
  jwt_secret: ${JWT_SECRET}
  token_ttl: 15m
  refresh_token_ttl: 7d

observability:
  enable_metrics: true
  metrics_port: 9090
  enable_tracing: true
  trace_endpoint: ${OTEL_EXPORTER_OTLP_ENDPOINT}
  sample_rate: 0.1
  enable_profiling: false

fleet:
  max_devices: 100000
  heartbeat_interval: 60s
  heartbeat_timeout: 180s
  update_check_interval: 5m
  deployment_timeout: 30m
  max_concurrent_deployments: 100

rate_limiting:
  enable: true
  requests_per_second: 100
  burst: 200
  cleanup_interval: 1m

circuit_breaker:
  max_failures: 5
  max_requests: 1
  interval: 60s
  timeout: 30s

retry:
  max_attempts: 3
  initial_delay: 100ms
  max_delay: 10s
  multiplier: 2.0
```

### Device Configuration

```yaml
# config/device.yaml
device:
  id: ${DEVICE_ID}
  name: ${DEVICE_NAME}
  type: raspberry-pi

server:
  url: https://fleetd.example.com:8080
  ca_cert: /etc/fleetd/ca.crt
  client_cert: /etc/fleetd/device.crt
  client_key: /etc/fleetd/device.key

agent:
  heartbeat_interval: 60s
  metrics_interval: 30s
  log_level: info
  runtime_dir: /var/lib/fleetd
  log_dir: /var/log/fleetd

updates:
  check_interval: 5m
  download_dir: /var/cache/fleetd
  install_timeout: 30m
  rollback_on_failure: true
```

## Security

### 1. Network Security

```bash
# Firewall rules (iptables)
sudo iptables -A INPUT -p tcp --dport 8080 -j ACCEPT  # API
sudo iptables -A INPUT -p tcp --dport 9090 -j ACCEPT  # gRPC
sudo iptables -A INPUT -p tcp --dport 9091 -j ACCEPT  # Metrics
sudo iptables -A INPUT -p tcp --dport 5432 -s 10.0.0.0/8 -j ACCEPT  # PostgreSQL (internal only)
sudo iptables -A INPUT -p tcp --dport 6379 -s 10.0.0.0/8 -j ACCEPT  # Redis (internal only)

# Save rules
sudo iptables-save > /etc/iptables/rules.v4
```

### 2. TLS Configuration

```nginx
# nginx.conf for TLS termination
server {
    listen 443 ssl http2;
    server_name fleetd.example.com;

    ssl_certificate /etc/nginx/certs/server.crt;
    ssl_certificate_key /etc/nginx/certs/server.key;
    ssl_client_certificate /etc/nginx/certs/ca.crt;
    ssl_verify_client optional;

    ssl_protocols TLSv1.3;
    ssl_ciphers ECDHE-RSA-AES256-GCM-SHA512:DHE-RSA-AES256-GCM-SHA512;
    ssl_prefer_server_ciphers off;

    ssl_session_cache shared:SSL:10m;
    ssl_session_timeout 10m;
    ssl_stapling on;
    ssl_stapling_verify on;

    location / {
        grpc_pass grpc://backend;
        grpc_set_header X-Client-Cert $ssl_client_escaped_cert;
    }
}

upstream backend {
    least_conn;
    server fleetd-1:8080 max_fails=3 fail_timeout=30s;
    server fleetd-2:8080 max_fails=3 fail_timeout=30s;
    server fleetd-3:8080 max_fails=3 fail_timeout=30s;
}
```

### 3. RBAC Configuration

```yaml
# rbac.yaml
roles:
  - name: admin
    permissions:
      - device:*
      - update:*
      - analytics:*
      - system:*
      - user:*

  - name: operator
    permissions:
      - device:list
      - device:view
      - device:update
      - update:*
      - analytics:view

  - name: viewer
    permissions:
      - device:list
      - device:view
      - update:list
      - update:view
      - analytics:view

  - name: device
    permissions:
      - device:register
      - device:heartbeat
      - device:view  # self only
      - update:view

policies:
  - name: require-mfa-for-admin
    resource: "*"
    actions: ["*"]
    effect: deny
    conditions:
      - type: role
        operator: equals
        value: admin
      - type: mfa
        operator: equals
        value: false

  - name: restrict-delete-production
    resource: "device"
    actions: ["delete"]
    effect: deny
    conditions:
      - type: environment
        operator: equals
        value: production
      - type: role
        operator: not_equals
        value: admin
```

### 4. Secrets Management

```bash
# Using HashiCorp Vault
vault server -config=/etc/vault/config.hcl

# Store secrets
vault kv put secret/fleetd/production \
  db_password="secure_password" \
  jwt_secret="$(openssl rand -base64 32)" \
  api_key="$(uuidgen)"

# Configure Kubernetes secrets
kubectl create secret generic fleetd-secrets \
  --from-literal=db-password='secure_password' \
  --from-literal=jwt-secret='$(openssl rand -base64 32)'
```

## Monitoring

### 1. Prometheus Configuration

```yaml
# prometheus.yml
global:
  scrape_interval: 15s
  evaluation_interval: 15s

scrape_configs:
  - job_name: 'fleetd'
    static_configs:
      - targets: ['fleetd-1:9090', 'fleetd-2:9090', 'fleetd-3:9090']

  - job_name: 'postgres'
    static_configs:
      - targets: ['postgres-exporter:9187']

  - job_name: 'redis'
    static_configs:
      - targets: ['redis-exporter:9121']

  - job_name: 'node'
    static_configs:
      - targets: ['node-exporter:9100']

rule_files:
  - '/etc/prometheus/alerts.yml'

alerting:
  alertmanagers:
    - static_configs:
        - targets: ['alertmanager:9093']
```

### 2. Alert Rules

```yaml
# alerts.yml
groups:
  - name: fleetd
    interval: 30s
    rules:
      - alert: HighErrorRate
        expr: rate(http_requests_total{status=~"5.."}[5m]) > 0.05
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: High error rate detected
          description: "Error rate is {{ $value }} errors per second"

      - alert: DeviceOffline
        expr: time() - device_last_seen > 300
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: Device offline
          description: "Device {{ $labels.device_id }} has been offline for >5 minutes"

      - alert: DatabaseConnectionPoolExhausted
        expr: db_connections_open / db_connections_max > 0.9
        for: 2m
        labels:
          severity: critical
        annotations:
          summary: Database connection pool nearly exhausted
          description: "{{ $value }}% of database connections in use"

      - alert: CircuitBreakerOpen
        expr: circuit_breaker_state == 1
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: Circuit breaker open
          description: "Circuit breaker {{ $labels.name }} is open"
```

### 3. Grafana Dashboards

Key metrics to monitor:
- **System Metrics**: CPU, Memory, Disk, Network
- **Application Metrics**: Request rate, latency, error rate
- **Business Metrics**: Active devices, deployments, success rate
- **Database Metrics**: Query performance, connection pool, slow queries
- **Security Metrics**: Auth failures, rate limit violations

### 4. Logging

```yaml
# fluentd.conf
<source>
  @type tail
  path /var/log/fleetd/*.log
  pos_file /var/log/fleetd/fluentd.pos
  tag fleetd.*
  <parse>
    @type json
  </parse>
</source>

<filter fleetd.**>
  @type record_transformer
  <record>
    hostname ${hostname}
    environment production
  </record>
</filter>

<match fleetd.**>
  @type elasticsearch
  host elasticsearch
  port 9200
  logstash_format true
  logstash_prefix fleetd
  <buffer>
    @type file
    path /var/log/fluentd/buffer
    flush_interval 10s
  </buffer>
</match>
```

## Troubleshooting

### Common Issues

#### 1. High Memory Usage
```bash
# Check memory usage
free -h
ps aux | sort -nrk 4 | head

# Analyze heap profile
go tool pprof http://localhost:6060/debug/pprof/heap

# Adjust GOGC and GOMEMLIMIT
export GOGC=50
export GOMEMLIMIT=4GiB
```

#### 2. Database Connection Issues
```sql
-- Check connections
SELECT count(*) FROM pg_stat_activity;

-- Kill idle connections
SELECT pg_terminate_backend(pid)
FROM pg_stat_activity
WHERE state = 'idle'
  AND state_change < current_timestamp - interval '10 minutes';

-- Check slow queries
SELECT query, mean_exec_time, calls
FROM pg_stat_statements
ORDER BY mean_exec_time DESC
LIMIT 10;
```

#### 3. Circuit Breaker Trips
```bash
# Check circuit breaker status
curl http://localhost:9090/metrics | grep circuit_breaker_state

# Reset circuit breaker (via admin API)
curl -X POST http://localhost:8080/admin/circuit-breaker/reset

# Adjust circuit breaker settings
export CIRCUIT_BREAKER_MAX_FAILURES=10
export CIRCUIT_BREAKER_TIMEOUT=60s
```

#### 4. Rate Limiting Issues
```bash
# Check rate limit metrics
curl http://localhost:9090/metrics | grep rate_limit

# Adjust rate limits
export RATE_LIMIT_RPS=200
export RATE_LIMIT_BURST=400

# Whitelist specific IPs
iptables -I INPUT -s 192.168.1.100 -j ACCEPT
```

### Performance Tuning

#### 1. Database Optimization
```sql
-- Update statistics
ANALYZE;

-- Reindex tables
REINDEX TABLE device;

-- Vacuum to reclaim space
VACUUM FULL ANALYZE;

-- Configure shared_buffers (25% of RAM)
ALTER SYSTEM SET shared_buffers = '4GB';

-- Configure work_mem
ALTER SYSTEM SET work_mem = '16MB';

-- Configure effective_cache_size (50-75% of RAM)
ALTER SYSTEM SET effective_cache_size = '12GB';
```

#### 2. Application Tuning
```bash
# Increase file descriptors
ulimit -n 65536

# TCP tuning
sysctl -w net.core.somaxconn=65535
sysctl -w net.ipv4.tcp_max_syn_backlog=65535
sysctl -w net.ipv4.ip_local_port_range="1024 65535"

# Enable TCP keepalive
sysctl -w net.ipv4.tcp_keepalive_time=600
sysctl -w net.ipv4.tcp_keepalive_intvl=60
sysctl -w net.ipv4.tcp_keepalive_probes=3
```

### Backup and Recovery

#### 1. Database Backup
```bash
#!/bin/bash
# backup.sh
DATE=$(date +%Y%m%d_%H%M%S)
BACKUP_DIR="/backup/postgres"

# Full backup
pg_dump -h localhost -U fleetd -d fleetd | gzip > $BACKUP_DIR/fleetd_$DATE.sql.gz

# Incremental backup using WAL archiving
archive_command = 'test ! -f /backup/wal/%f && cp %p /backup/wal/%f'

# Retention policy (keep 7 days)
find $BACKUP_DIR -name "*.sql.gz" -mtime +7 -delete
```

#### 2. Disaster Recovery
```bash
# Restore from backup
gunzip < /backup/postgres/fleetd_20240101_120000.sql.gz | psql -h localhost -U fleetd -d fleetd

# Point-in-time recovery
recovery_target_time = '2024-01-01 12:00:00'
restore_command = 'cp /backup/wal/%f %p'
```

## Health Checks

### Endpoints
- `/health` - Liveness check
- `/ready` - Readiness check
- `/metrics` - Prometheus metrics
- `/health/detailed` - Detailed health status

### Example Health Response
```json
{
  "status": "healthy",
  "checks": {
    "database": {
      "status": "healthy",
      "message": "Database connection healthy",
      "duration_ms": 5
    },
    "redis": {
      "status": "healthy",
      "message": "Redis connection healthy",
      "duration_ms": 2
    },
    "disk_space": {
      "status": "healthy",
      "message": "Sufficient disk space",
      "metadata": {
        "used_percent": 45,
        "free_gb": 55
      }
    }
  },
  "time": "2024-01-01T12:00:00Z"
}
```

## Support

For production support:
- Documentation: https://docs.fleetd.io
- Issues: https://github.com/fleetd/fleetd/issues
- Security: security@fleetd.io
- Enterprise Support: support@fleetd.io