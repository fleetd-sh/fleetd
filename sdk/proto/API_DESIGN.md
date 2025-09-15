# FleetD API Architecture

## Overview

FleetD uses a dual-API architecture to separate device communication from management operations:

1. **Agent API** - High-volume device communication (registration, telemetry, updates)
2. **Control API** - Management operations (web UI, CLI, SDKs)

## Agent API (Device-facing)

**Purpose:** Handle all device-to-cloud communication
**Port:** 8080 (gRPC)
**Auth:** Device certificates or pre-shared keys

### Services

- **DeviceService** - Registration, heartbeat, status reporting
- **TelemetryService** - Metrics, logs, events ingestion
- **UpdateService** - OTA update polling and downloads
- **SyncService** - Bidirectional data synchronization
- **DiscoveryService** - Local network device discovery

### Characteristics

- Optimized for high-volume writes
- Minimal response payloads
- Strong backward compatibility requirements
- Simple authentication model
- Designed for unreliable network conditions

## Control API (Management-facing)

**Purpose:** Fleet management, analytics, and administration
**Port:** 8081 (gRPC) / 8082 (REST gateway)
**Auth:** User tokens (JWT) or API keys

### Services

- **FleetService** - Fleet-wide operations and queries
- **DeviceManagementService** - CRUD operations on devices
- **DeploymentService** - Update campaign management
- **AnalyticsService** - Metrics aggregation and reporting
- **OrganizationService** - Multi-tenancy and user management

### Characteristics

- Complex queries with filtering and pagination
- Rich response payloads with nested data
- Can evolve rapidly with versioning
- Multiple authentication methods
- Optimized for read-heavy workloads

## Protocol Structure

```
proto/
├── agent/           # Device-facing APIs
│   └── v1/
│       ├── device.proto
│       ├── telemetry.proto
│       ├── update.proto
│       ├── sync.proto
│       └── discovery.proto
├── control/         # Management APIs
│   └── v1/
│       ├── fleet.proto
│       ├── device_management.proto
│       ├── deployment.proto
│       ├── analytics.proto
│       └── organization.proto
└── common/          # Shared types
    └── v1/
        ├── device.proto
        ├── metrics.proto
        ├── timestamp.proto
        └── errors.proto
```

## Security Model

### Agent API Security
- Device provisioning with pre-shared keys
- Certificate-based authentication for production
- Rate limiting per device
- Tenant isolation via device groups

### Control API Security
- OAuth 2.0 / OpenID Connect for users
- API keys for service accounts
- Role-based access control (RBAC)
- Audit logging for all operations

## Deployment Architecture

```
                    ┌─────────────────┐
                    │   API Gateway   │
                    │  (Envoy/Nginx)  │
                    └────────┬────────┘
                             │
            ┌────────────────┴────────────────┐
            │                                  │
    ┌───────▼────────┐              ┌─────────▼────────┐
    │   Agent API    │              │   Control API    │
    │  (Port 8080)   │              │  (Port 8081)     │
    └───────┬────────┘              └─────────┬────────┘
            │                                  │
            └────────────┬─────────────────────┘
                         │
                ┌────────▼────────┐
                │   Data Layer    │
                │  (PostgreSQL)   │
                └─────────────────┘
```

## Migration Path

1. **Phase 1:** Create new proto structure (current)
2. **Phase 2:** Implement Agent API in existing server
3. **Phase 3:** Add Control API alongside Agent API
4. **Phase 4:** Deploy API gateway for routing
5. **Phase 5:** Optional: Split into separate services