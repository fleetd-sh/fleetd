# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

### Build and Run
```bash
# Build all binaries (fleetd and fleetp) for current architecture
just build-all

# Build specific target for current architecture
just build fleetd
just build fleetp

# Build for specific architecture
just build fleetd arm64   # ARM64 (Raspberry Pi)
just build fleetd amd64   # AMD64/x86_64
just build fleetp arm64   # ARM64 fleetp
just build fleetp amd64   # AMD64 fleetp

# Run in development mode with auto-reload
just watch fleetd

# Format code
just format

# Lint code
just lint
```

### Device Provisioning with fleetp
```bash
# List available devices
fleetp -list

# Basic provisioning (fleetd agent only)
fleetp -device /dev/disk2 -wifi-ssid MyNetwork -wifi-pass secret

# With SSH access
fleetp -device /dev/disk2 -wifi-ssid MyNetwork -wifi-pass secret -ssh-key ~/.ssh/id_rsa.pub

# With specific fleet server
fleetp -device /dev/disk2 -wifi-ssid MyNetwork -wifi-pass secret -fleet-server https://fleet.local:8080

# With plugins (k3s, docker, etc.)
fleetp -device /dev/disk2 -wifi-ssid MyNetwork -wifi-pass secret \
  -plugin k3s -plugin-opt k3s.role=server

# Multiple plugins
fleetp -device /dev/disk2 -wifi-ssid MyNetwork -wifi-pass secret \
  -plugin k3s -plugin-opt k3s.role=agent -plugin-opt k3s.server=https://192.168.1.100:6443 \
  -plugin docker
```

### Testing
```bash
# Run all tests
just test-all

# Run specific test
just test TestFunctionName

# Run tests for specific package
just test ./internal/agent

# Run tests from development docs (alternative)
make test
make test-unit
make test-integration
make test-e2e
make test-coverage
```

### Protocol Buffers
```bash
# Generate protobuf code
buf generate

# Update proto dependencies
buf mod update
```

### Database Migrations
```bash
# Create new migration (from development docs)
make new-migration name=migration_name

# Run migrations up
make migrate-up

# Run migrations down
make migrate-down
```

## Architecture Overview

FleetD is a heterogeneous fleet management system supporting both constrained edge devices (ESP32) and capable Linux devices (Raspberry Pi), with a focus on bare metal deployment, offline operation, and resource efficiency.

### Core Components

1. **Device Agent** (`cmd/fleetd/main.go`, `internal/agent/`)
   - Runs on edge devices (ESP32, Raspberry Pi)
   - Manages binary lifecycle and updates
   - Collects telemetry
   - Supports mDNS discovery with device capabilities
   - Minimal dependencies with offline-first operation
   - K3s integration for Raspberry Pi clusters

2. **Server Components** (`internal/api/`)
   - Device registration and authentication
   - Binary/package distribution
   - Telemetry ingestion
   - Fleet-wide update management
   - Uses Connect RPC (gRPC-compatible) with HTTP/REST proxies

3. **Discovery Service** (`internal/discovery/`)
   - mDNS-based local discovery with extended device info
   - Allows devices to find and register with servers
   - Enables vendor apps to discover devices
   - RPi-specific discovery for unconfigured devices and k3s nodes

4. **Device Provisioning** (`internal/provision/`, `cmd/fleetp/`)
   - Core: Provisions devices with fleetd agent for fleet management
   - Plugin system for extending functionality (k3s, docker, etc.)
   - Zero-touch configuration via mDNS
   - Focus on simplicity and speed

### Key Directories

- `proto/`: Protocol buffer definitions for all RPC services
  - `agent/v1/`: Agent-specific protocols
  - `fleetd/v1/`: Fleet management protocols (analytics, binary, device, update, cluster)
  - `health/v1/`: Health check protocols
  - New `cluster.proto`: K3s cluster management for Raspberry Pi

- `gen/`: Generated protobuf code (do not edit manually)

- `internal/`: Core implementation
  - `agent/`: Device agent logic, daemon, and discovery
  - `api/`: Server API implementations
  - `container/`: Docker container support
  - `discovery/`: Enhanced mDNS discovery with RPi support
  - `middleware/`: gRPC middleware (rate limiting)
  - `migrations/`: SQLite database migrations
  - `provision/rpi/`: Raspberry Pi SD card provisioning
  - `runtime/`: Runtime management (health, logs, resources)
  - `state/`: State management
  - `storage/`: Metrics storage backends
  - `telemetry/`: Telemetry collection
  - `update/`: Update mechanism
  - `webhook/`: Webhook support with signature verification

- `sdk/go/fleetd/`: Go SDK for client applications

- `test/`: Test suites
  - `integration/`: Integration tests for all major components
  - `e2e/`: End-to-end tests with Docker containers

### Data Storage

- **Primary**: SQLite for core data (device registry, configuration)
- **Metrics**: SQLite-based metrics storage with aggregation support
- **Optional**: Redis support for distributed scenarios

### Communication

- **RPC Framework**: Connect RPC (gRPC-compatible)
- **Discovery**: mDNS for local network discovery
- **Transport**: HTTP/2 with TLS support

### Development Philosophy

- OS-agnostic, bare metal first approach
- Local-first operation with offline capabilities
- Minimal external dependencies
- Progressive enhancement based on available resources
- Single binary deployment for simplicity

## Testing Strategy

- Unit tests: Located next to code in `*_test.go` files
- Integration tests: Test component interactions in `test/integration/`
- E2E tests: Full workflow testing in `test/e2e/`
- Use testcontainers for Docker-based testing
- Mock external dependencies appropriately

## Important Context

The system is designed for three main use cases:
1. **Hobbyist deployment**: Personal device management (e.g., Raspberry Pi k3s clusters)
2. **Consumer hardware**: Smart home devices with app-based management
3. **Industrial IoT**: Large-scale deployments with strict reliability requirements

### Device Tiers
- **Tier 1 (Constrained)**: ESP32 devices with limited resources
- **Tier 2 (Capable)**: Raspberry Pi with full Linux, Docker, and k3s support

### Raspberry Pi Features
- **Automated Provisioning**: SD card setup with DietPi, fleetd agent, and optional k3s
- **Zero-Touch Config**: Devices self-register via mDNS after boot
- **K3s Integration**: Automated cluster creation and node joining
- **Template System**: Configurable provisioning for different deployment scenarios

Current implementation includes:
- Basic device agent with runtime management
- Raspberry Pi SD card provisioning tool
- Enhanced mDNS discovery for heterogeneous fleets
- K3s cluster management protocols
- Binary deployment and update mechanisms
