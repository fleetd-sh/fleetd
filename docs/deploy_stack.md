# Deploying the fleetd stack

## Overview

The fleetd stack is a collection of services that work together to manage your fleet of devices.

The stack can be deployed using a single server program or by running each service in a separate containers.

> [!TIP]
> Unless you need to scale the services independently, it is recommended to run the stack on a single server.

## Deployment

### Docker

#### Compose

The easiest way to deploy the fleetd stack is to use the provided Docker Compose file:

1. Run the compose file. (TODO: Add compose file)

#### Standalone

This approach requires running dependent services (InfluxDB) separately.

```bash
docker run -p 50051:50051 \
-e DATABASE_URL=file:///data/db/fleet.db \
-e STORAGE_PATH=/data/storage \
-e INFLUXDB_URL=your_instance_url \
-e INFLUXDB_TOKEN=your_token \
-v /path/to/data:/data \
fleetd/fleet-server
```

### By hand

If you prefer to run the server manually, you can build the binary and run it directly. This requires running dependent services ([InfluxDB](https://www.influxdata.com)) separately.

1. Download the latest binary from the [releases page](https://github.com/fleetd-sh/fleetd/releases).
2. Configure the environment
3. Run the server

## Configuration

### Environment Variables
| Variable | Description | Default |
|----------|-------------|---------|
| DATABASE_URL | Database connection URL | file:fleet.db |
| STORAGE_PATH | Path for file storage | storage |
| INFLUXDB_URL | InfluxDB server URL | http://localhost:8086 |
| INFLUXDB_TOKEN | InfluxDB authentication token | |
| INFLUXDB_ORG | InfluxDB organization | fleet |
| INFLUXDB_BUCKET | InfluxDB bucket name | metrics |
| LISTEN_ADDR | Server listen address | localhost:8080 |

## Architecture

### Services
All services are exposed as Connect RPC endpoints on the same server:

- `/auth.v1.AuthService/*` - Authentication and authorization
- `/device.v1.DeviceService/*` - Device management and registration
- `/metrics.v1.MetricsService/*` - Metrics collection and querying
- `/update.v1.UpdateService/*` - Software update management
- `/storage.v1.StorageService/*` - File storage service