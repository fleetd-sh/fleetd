# FleetD Management System Documentation

## Table of Contents
- [Overview](#overview)
- [Architecture](#architecture)
- [Installation](#installation)
- [Quick Start Guide](#quick-start-guide)
- [Components](#components)
- [API Reference](#api-reference)
- [Troubleshooting](#troubleshooting)

## Overview

FleetD is a comprehensive fleet management system for IoT and edge devices. It consists of:
- **Fleet Server**: Central management server with REST APIs and web dashboard
- **FleetD Agent**: Lightweight agent running on managed devices
- **Discovery Service**: mDNS-based automatic device discovery
- **Configuration Tools**: CLI tools for device configuration and management

### Key Features
- 🔍 Automatic device discovery via mDNS
- 📊 Real-time telemetry and metrics collection
- 🎯 Remote device configuration
- 📈 Web dashboard for monitoring
- 🔄 Over-the-air updates
- 📡 Bidirectional RPC communication

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                       Fleet Server                          │
│  ┌─────────────┐  ┌──────────────┐  ┌─────────────────┐   │
│  │   REST API  │  │  Web Dashboard│  │  mDNS Discovery │   │
│  └──────┬──────┘  └───────┬──────┘  └────────┬────────┘   │
│         │                  │                   │            │
│  ┌──────┴──────────────────┴───────────────────┴────────┐  │
│  │              SQLite Database                          │  │
│  └───────────────────────────────────────────────────────┘  │
└─────────────────────────┬───────────────────────────────────┘
                          │
                    Network (HTTP/RPC)
                          │
    ┌─────────────────────┼─────────────────────────┐
    │                     │                         │
┌───▼────────┐    ┌───────▼────────┐    ┌──────────▼────────┐
│FleetD Agent│    │ FleetD Agent   │    │  FleetD Agent    │
│  Device 1  │    │   Device 2     │    │    Device N      │
└────────────┘    └────────────────┘    └──────────────────┘
```

### Communication Flow

1. **Device Registration**: Agents announce themselves via mDNS
2. **Configuration**: Server discovers and configures agents
3. **Telemetry**: Agents report metrics to server
4. **Management**: Server sends commands and updates to agents

## Installation

### Prerequisites
- Go 1.21 or later
- SQLite3
- Network connectivity between server and devices

### Building from Source

```bash
# Clone the repository
git clone https://github.com/your-org/fleetd.git
cd fleetd

# Build the fleet server
go build -o fleets ./cmd/fleets

# Build the fleetd agent
go build -o fleetd ./cmd/fleetd
```

### Docker Installation

```bash
# Fleet Server
docker run -d \
  -p 8080:8080 \
  -v /var/lib/fleetd:/data \
  --name fleet-server \
  fleetd/server:latest

# FleetD Agent
docker run -d \
  --network host \
  --name fleetd-agent \
  fleetd/agent:latest
```

## Quick Start Guide

### 1. Start the Fleet Server

```bash
# Start with default settings
./fleets server

# Or with custom configuration
./fleets server \
  --port 8080 \
  --database /var/lib/fleetd/fleet.db \
  --secret-key your-secret-key
```

The server will start on `http://localhost:8080` with:
- Web Dashboard: `http://localhost:8080/`
- REST API: `http://localhost:8080/api/v1/`

### 2. Deploy FleetD Agents

On each device you want to manage:

```bash
# Start the agent
./fleetd agent

# The agent will:
# - Generate a unique device ID
# - Start mDNS announcement
# - Wait for configuration
```

### 3. Discover and Configure Devices

Use the `fleets configure` command to discover and configure devices:

```bash
# Interactive mode - discover and select devices
./fleets configure --server http://your-server:8080

# Example output:
# Discovering fleetd devices on the network...
# 
# Found 3 device(s):
# 
# 1. Device: fleetd-rpi4-001._fleetd._tcp.local.
#    Host: raspberrypi.local
#    IPv4: 192.168.1.100
#    Port: 6789
#    Device ID: rpi4-001
# 
# 2. Device: fleetd-jetson-002._fleetd._tcp.local.
#    Host: jetson.local
#    IPv4: 192.168.1.101
#    Port: 6789
#    Device ID: jetson-002
# 
# Which devices would you like to configure? (comma-separated numbers, or 'all'): all
# 
# Configuring 2 device(s) with server URL: http://your-server:8080
# 
# ✅ Successfully configured fleetd-rpi4-001._fleetd._tcp.local.
# ✅ Successfully configured fleetd-jetson-002._fleetd._tcp.local.
```

#### Automatic Configuration

For automated deployments:

```bash
# Configure all discovered devices automatically
./fleets configure \
  --server http://your-server:8080 \
  --auto \
  --api-key your-api-key
```

### 4. Monitor via Dashboard

Open `http://localhost:8080` in your browser to access the dashboard:

- **Fleet Status**: View all connected devices
- **Device Details**: CPU, memory, disk usage
- **Telemetry**: Real-time metrics and logs
- **Configuration**: Update device settings

## Components

### Fleet Server (`fleets`)

The central management server providing:

#### REST API Endpoints
- Device management
- Telemetry collection
- Configuration updates
- OTA updates

#### Web Dashboard
- Real-time device monitoring
- Interactive management interface
- Metric visualization

#### Database
- SQLite for persistent storage
- Device registry
- Telemetry history
- Configuration versions

### FleetD Agent (`fleetd`)

Lightweight agent running on each device:

#### Features
- Automatic registration via mDNS
- System metrics collection
- Remote configuration
- Binary management
- Container orchestration

#### Configuration File
```yaml
# /etc/fleetd/config.yaml
device_name: "rpi4-001"
api_endpoint: "http://fleet-server:8080"
api_key: "auto-generated-key"
telemetry:
  interval: 60s
  metrics:
    - cpu_usage
    - memory_usage
    - disk_usage
```

### Discovery Service

mDNS-based service for automatic device discovery:

- **Service Type**: `_fleetd._tcp`
- **Port**: 6789 (default)
- **TXT Records**: Device ID, version, capabilities

### CLI Tools

#### `fleets` - Server Management
```bash
# Start server
fleets server [flags]

# Discover devices
fleets discover

# Configure devices
fleets configure [flags]
```

#### `fleetd` - Agent
```bash
# Run agent
fleetd agent [flags]

# Show device info
fleetd info

# Check status
fleetd status
```

## API Reference

### Device Management

#### List Devices
```http
GET /api/v1/devices
```

Response:
```json
[
  {
    "id": "rpi4-001",
    "name": "Raspberry Pi 4",
    "type": "raspberrypi",
    "version": "1.0.0",
    "status": "online",
    "last_seen": "2024-01-15T10:30:00Z"
  }
]
```

#### Get Device Details
```http
GET /api/v1/devices/{id}
```

#### Update Device
```http
PUT /api/v1/devices/{id}
Content-Type: application/json

{
  "metadata": {
    "location": "Lab A",
    "owner": "Engineering"
  }
}
```

#### Delete Device
```http
DELETE /api/v1/devices/{id}
```

### Telemetry

#### Submit Telemetry
```http
POST /api/v1/telemetry
Content-Type: application/json

{
  "device_id": "rpi4-001",
  "timestamp": "2024-01-15T10:30:00Z",
  "metric_name": "cpu_usage",
  "value": 45.2,
  "metadata": "{\"cores\": 4}"
}
```

#### Query Metrics
```http
GET /api/v1/telemetry/metrics?device_id=rpi4-001&metric=cpu_usage&limit=100
```

### Configuration

#### Get Device Configuration
```http
GET /api/v1/config?device_id=rpi4-001
```

#### Update Configuration
```http
POST /api/v1/config?device_id=rpi4-001
Content-Type: application/json

{
  "server_url": "http://new-server:8080",
  "api_key": "new-key",
  "config": "{\"telemetry_interval\": 30}"
}
```

### Discovery

#### Discover Devices
```http
GET /api/v1/discover
```

#### Configure Discovered Device
```http
POST /api/v1/discover
Content-Type: application/json

{
  "device_id": "rpi4-001",
  "config": {
    "server_url": "http://fleet-server:8080",
    "api_key": "device-key"
  }
}
```

## Configuration Examples

### Multi-Network Setup

When devices are on different networks:

```bash
# 1. Use a public server endpoint
./fleets server --port 8080 --public-url https://fleet.example.com

# 2. Configure devices with public URL
./fleets configure --server https://fleet.example.com
```

### High Availability Setup

For production environments:

```bash
# Use external database
./fleets server \
  --database postgres://user:pass@db-server/fleetd \
  --port 8080

# Run multiple server instances behind load balancer
./fleets server --port 8081 &
./fleets server --port 8082 &
./fleets server --port 8083 &
```

### Secure Communication

Enable TLS for secure communication:

```bash
# Generate certificates
openssl req -x509 -newkey rsa:4096 \
  -keyout server.key -out server.crt \
  -days 365 -nodes

# Start server with TLS
./fleets server \
  --tls-cert server.crt \
  --tls-key server.key \
  --port 8443
```

## Monitoring and Observability

### Metrics Collection

The system collects various metrics:

- **System Metrics**: CPU, memory, disk usage
- **Network Metrics**: Bandwidth, latency, packet loss
- **Application Metrics**: Custom metrics from applications

### Alerting

Configure alerts for critical conditions:

```yaml
# alerts.yaml
alerts:
  - name: high_cpu_usage
    condition: cpu_usage > 90
    duration: 5m
    action: email
    
  - name: device_offline
    condition: last_seen > 10m
    action: webhook
    url: https://alerts.example.com/webhook
```

### Logging

Access logs for debugging:

```bash
# Server logs
journalctl -u fleet-server -f

# Agent logs
journalctl -u fleetd-agent -f

# Or direct log files
tail -f /var/log/fleetd/server.log
tail -f /var/log/fleetd/agent.log
```

## Troubleshooting

### Common Issues

#### Devices Not Discovered

1. Check network connectivity:
```bash
# On server
ping device-hostname

# Check mDNS
dns-sd -B _fleetd._tcp
```

2. Verify firewall rules:
```bash
# Allow mDNS port
sudo ufw allow 5353/udp

# Allow agent RPC port
sudo ufw allow 6789/tcp
```

3. Check agent is announcing:
```bash
# On device
./fleetd agent --debug
```

#### Configuration Not Applied

1. Check agent logs:
```bash
tail -f /var/log/fleetd/agent.log
```

2. Verify RPC connectivity:
```bash
curl http://device-ip:6789/health
```

3. Test configuration manually:
```bash
./fleets configure --server http://server:8080 --debug
```

#### High Memory Usage

1. Check telemetry retention:
```sql
-- Connect to database
sqlite3 /var/lib/fleetd/fleet.db

-- Check telemetry size
SELECT COUNT(*) FROM telemetry;

-- Clean old data
DELETE FROM telemetry WHERE timestamp < datetime('now', '-30 days');
```

2. Optimize collection interval:
```yaml
# Reduce frequency in agent config
telemetry:
  interval: 5m  # Instead of 1m
```

### Debug Mode

Enable debug logging for troubleshooting:

```bash
# Server debug mode
FLEETD_LOG_LEVEL=debug ./fleets server

# Agent debug mode
FLEETD_LOG_LEVEL=debug ./fleetd agent

# Or via environment file
echo "FLEETD_LOG_LEVEL=debug" >> /etc/fleetd/env
systemctl restart fleetd
```

## Advanced Topics

### Custom Telemetry Plugins

Create custom telemetry collectors:

```go
// custom_collector.go
type CustomCollector struct{}

func (c *CustomCollector) Collect() (string, float64, error) {
    // Your collection logic
    value := measureCustomMetric()
    return "custom_metric", value, nil
}

// Register in agent
agent.RegisterCollector("custom", &CustomCollector{})
```

### Webhook Integration

Configure webhooks for events:

```json
{
  "webhooks": [
    {
      "event": "device_online",
      "url": "https://slack.example.com/webhook",
      "method": "POST"
    },
    {
      "event": "alert_triggered",
      "url": "https://pagerduty.example.com/webhook",
      "headers": {
        "Authorization": "Bearer token"
      }
    }
  ]
}
```

### Batch Operations

Perform operations on multiple devices:

```bash
# Update all Raspberry Pi devices
./fleets batch-update \
  --filter "type=raspberrypi" \
  --config update.json

# Restart all offline devices
./fleets batch-command \
  --filter "status=offline" \
  --command "restart"
```

## Contributing

We welcome contributions! See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

FleetD is licensed under the MIT License. See [LICENSE](LICENSE) for details.

## Support

- Documentation: https://docs.fleetd.io
- Issues: https://github.com/your-org/fleetd/issues
- Community: https://discord.gg/fleetd
- Commercial Support: support@fleetd.io