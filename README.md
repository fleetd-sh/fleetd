# fleetd

> [!WARNING]
> This is _very_ early work in progress.

Manage your fleet of edge devices; provision, update, monitor, and secure.

The fleet daemon, _fleetd_, is a long-running service that monitors and manages the lifecycle of devices and deployed software in the fleet.

## 🏗️ Architecture

```mermaid
graph TD
    A[Device Agent/fleetd] -->|mDNS Discovery| B[Discovery Service]
    A -->|gRPC/Connect| C[Fleet Server/fleets]
    C --> D[PostgreSQL/SQLite]
    C --> E[Binary Storage]
    C --> F[VictoriaMetrics]
    C --> G[Loki]
    C --> H[ClickHouse]
    I[Web Dashboard] -->|API| C
    J[CLI/fleet] -->|API| C
    K[Fleet Aggregator] -->|Sync| C
```

## 📦 Components

### Core Services
- **fleetd** - Device agent that runs on edge devices
- **fleets** - Central fleet management server
- **fleet** - CLI tool for fleet management
- **discover** - mDNS discovery service

### Web Dashboard
- Next.js application with shadcn/ui components
- Real-time device monitoring
- Fleet management interface
- Located in `/web` directory

## 🚀 Quick Start

### Prerequisites

- Go 1.23+
- Bun (for web development)
- Docker & Docker Compose (optional, for data stack)
- Just (command runner)

### Installation

```bash
# Install dependencies
just install

# Check all tools are installed
just check-tools

# Run development environment
just dev
```

### Development Commands

```bash
# Start development servers (backend + frontend)
just dev

# Build everything
just build-all

# Run all tests
just test-all

# Format code
just format-all

# Lint code
just lint-all
```

## 🛠️ Development

### Backend Development

```bash
# Build specific binary
just build fleetd
just build fleets
just build fleet

# Build all binaries
just build-go

# Run tests
just test-go

# Run with coverage
just test-go-coverage

# Run specific test
just test-go-run TestName

# Start backend development server
just server-dev

# Watch mode (auto-reload)
just server-watch
```

### Frontend Development

```bash
# Install web dependencies
just web-install

# Start development server
just web-dev

# Build for production
just build-web

# Run tests
just test-web

# Type checking
just test-web-types

# Format with Biome
just format-web

# Lint with Biome
just lint-web
```

### Protocol Buffers

```bash
# Generate Go and TypeScript code
just proto

# Format proto files
just proto-format

# Lint proto files
just proto-lint

# Check for breaking changes
just proto-breaking
```

## 🗄️ Data Stack

The project includes a comprehensive data stack for metrics, logs, and analytics:

### Services
- **PostgreSQL** - Primary database
- **VictoriaMetrics** - Time-series metrics
- **Loki** - Log aggregation
- **ClickHouse** - Analytics database
- **Valkey** - Cache and pub/sub

### Data Stack Commands

```bash
# Start the full data stack
just stack-up

# Stop the data stack
just stack-down

# View logs
just stack-logs [service]

# Check health
just stack-health

# Reset data stack (WARNING: deletes all data)
just stack-reset
```

## 🌐 Gateway & Load Balancing

```bash
# Start gateway with Traefik
just gateway-up

# Stop gateway
just gateway-down

# View gateway logs
just gateway-logs

# Show routes
just gateway-routes

# Show services
just gateway-services

# Test health endpoints
just gateway-test
```

## 🗃️ Database Management

```bash
# Run migrations
just db-migrate

# Rollback migration
just db-rollback

# Create new migration
just db-migration <name>

# Reset database
just db-reset
```

## 🐳 Docker

```bash
# Build backend Docker image
just docker-build [tag]

# Build web Docker image
just docker-build-web [tag]

# Run with docker-compose
just docker-up

# Stop docker-compose
just docker-down

# View logs
just docker-logs [service]
```

## 📝 CLI Usage

### Fleet Server (fleets)

```bash
# Start server
fleets server --port 8080

# Discover devices
fleets discover

# Manage devices
fleets devices list
fleets devices get <device-id>

# Configuration
fleets configure

# Version info
fleets version
```

### Fleet CLI (fleet)

```bash
# Device management
fleet device list
fleet device get <id>
fleet device update <id>
fleet device delete <id>

# Deployment management
fleet deploy <binary> --target <device-pattern>
fleet rollback <deployment-id>

# Monitoring
fleet status
fleet metrics
fleet logs
```

### Device Agent (fleetd)

```bash
# Run agent
fleetd --server <server-url>

# With custom configuration
fleetd --config /etc/fleetd/config.yaml

# Debug mode
fleetd --debug --verbose
```

## 🧪 Testing

```bash
# Run all tests
just test-all

# Go tests only
just test-go

# Web tests only
just test-web

# Integration tests
just test-go-integration

# Watch mode
just watch-test
```

## 📚 Documentation

Comprehensive documentation is available in the `/docs` directory:

- [Architecture Overview](docs/architecture.md)
- [API Reference](docs/API_REFERENCE.md)
- [Developer Guide](docs/DEVELOPER_GUIDE.md)
- [Production Guide](docs/PRODUCTION_GUIDE.md)
- [Authentication](docs/AUTHENTICATION.md)
- [Data Architecture](docs/DATA_ARCHITECTURE.md)
- [Fleet Management](docs/FLEET_MANAGEMENT.md)
- [Software Lifecycle](docs/SOFTWARE_LIFECYCLE.md)

### Generate Documentation

```bash
# Generate API documentation
just docs-api

# Serve documentation locally
just docs-serve
```

## 🔧 Utility Commands

```bash
# Update all dependencies
just update-deps

# Run security audit
just audit

# Show project statistics
just stats

# Find TODO comments
just todos

# Pre-commit checks
just pre-commit
```

## 🚢 Deployment

```bash
# Deploy to environment
just deploy [environment]

# Create release
just release <version>
```

## 📁 Project Structure

```
fleetd/
├── cmd/                    # Command-line applications
│   ├── fleetd/            # Device agent
│   ├── fleets/            # Fleet server
│   ├── fleet/             # Management CLI
│   └── discover/          # Discovery service
├── internal/              # Internal packages
│   ├── agent/            # Agent implementation
│   ├── api/              # API handlers
│   ├── database/         # Database layer
│   ├── ferrors/          # Error handling
│   ├── middleware/       # HTTP middleware
│   ├── provision/        # Device provisioning
│   ├── sync/             # Synchronization
│   └── telemetry/        # Metrics & logging
├── gen/                   # Generated code (proto)
├── proto/                 # Protocol buffer definitions
├── web/                   # Next.js web dashboard
│   ├── app/              # App router pages
│   ├── components/       # React components
│   └── lib/              # Utilities and API
├── docker/               # Docker configurations
├── deployments/          # Deployment manifests
├── docs/                 # Documentation
└── test/                 # Integration tests
```

## 🔑 Environment Variables

Create a `.envrc` file in the project root:

```bash
# Server Configuration
export FLEETS_PORT=8080
export FLEETS_HOST=0.0.0.0

# Database
export DATABASE_URL=postgresql://user:pass@localhost/fleetd

# Metrics
export METRICS_ENABLED=true
export VICTORIA_METRICS_URL=http://localhost:8428

# Logs
export LOKI_URL=http://localhost:3100

# Development
export DEBUG=true
export LOG_LEVEL=debug
```

## 🤝 Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Run tests (`just test-all`)
4. Commit your changes (`git commit -m 'feat: add amazing feature'`)
5. Push to the branch (`git push origin feature/amazing-feature`)
6. Open a Pull Request

## 📄 License

This project is licensed under the MIT License - see the LICENSE file for details.

## 🆘 Support

- [GitHub Issues](https://github.com/fleetd/fleetd/issues)
- [Documentation](https://fleetd.sh/docs)
- [Discord Community](https://discord.gg/fleetd)

## 🎯 Roadmap

- [ ] Multi-cloud support (AWS, Azure, GCP)
- [ ] Enhanced security features (mTLS, HSM support)
- [ ] Advanced deployment strategies (canary, blue-green)
- [ ] IoT device support expansion
- [ ] Kubernetes operator
- [ ] Mobile app for fleet management
- [ ] AI-powered anomaly detection