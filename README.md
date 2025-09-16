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
    J[CLI/fleetctl] -->|API| C
    K[Fleet Aggregator] -->|Sync| C
```

## 📦 Components

### Core Services
- **fleetd** - Device agent that runs on edge devices
- **fleets** - Central fleet management server
- **fleetctl** - CLI tool for fleet management and platform control
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
- Docker (for running platform services)
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
just build fleetctl

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

## 🗄️ Platform Services

The fleetd platform includes a comprehensive stack for metrics, logs, and analytics:

### Services Managed by fleetctl
- **PostgreSQL** - Primary database
- **VictoriaMetrics** - Time-series metrics
- **Loki** - Log aggregation
- **ClickHouse** - Analytics database
- **Valkey** - Cache and pub/sub
- **Fleet Server** - Central management server
- **Web Dashboard** - Management UI

### Platform Management

```bash
# Start the entire platform
fleetctl start

# Start with specific profile
fleetctl start --profile development
fleetctl start --profile production

# Stop all services
fleetctl stop

# Check status
fleetctl status

# View logs
fleetctl logs [service]

# Health check
fleetctl health
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
```

## 📝 CLI Usage

### Fleet Server (fleets)

```bash
# Start the fleet server
fleets --port 8080

# With custom configuration
fleets --port 8080 --db /path/to/fleet.db --secret-key <key>

# Enable mDNS discovery
fleets --enable-mdns

# With rate limiting via Valkey/Redis
fleets --valkey localhost:6379 --rate-limit-requests 100

# Show version
fleets --version

# Show help
fleets --help
```

### Fleet Control CLI (fleetctl)

```bash
# Platform management
fleetctl start              # Start all platform services
fleetctl stop               # Stop platform services
fleetctl status             # Check platform status
fleetctl logs [service]     # View service logs

# Device management
fleetctl devices list               # List all devices
fleetctl devices get <id>           # Get device details
fleetctl devices update <id>        # Update device configuration
fleetctl devices delete <id>        # Remove device from fleet
fleetctl devices logs <id>          # View device logs
fleetctl devices metrics <id>       # View device metrics

# Device discovery
fleetctl discover           # Discover devices on network

# Fleet configuration
fleetctl configure get              # Get current configuration
fleetctl configure set <key> <val>  # Set configuration value
fleetctl configure apply -f file    # Apply configuration from file
fleetctl configure validate -f file # Validate configuration file

# Database migrations
fleetctl migrate up         # Run pending migrations
fleetctl migrate down       # Rollback migrations
fleetctl migrate status     # Check migration status
fleetctl migrate create <name>  # Create new migration
fleetctl migrate reset      # Reset database

# Deployment management
fleetctl deploy <binary> --target <device-pattern>
fleetctl rollback <deployment-id>

# Other commands
fleetctl version            # Show version information
fleetctl init              # Initialize new fleet project
fleetctl provision <device> # Provision a new device
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

Comprehensive documentation is available in our [Wiki](https://github.com/fleetd-sh/fleetd/wiki):

- [Getting Started](https://github.com/fleetd-sh/fleetd/wiki/Getting-Started)
- [Architecture Overview](https://github.com/fleetd-sh/fleetd/wiki/Architecture)
- [CLI Reference](https://github.com/fleetd-sh/fleetd/wiki/CLI-Reference)
- [API Documentation](https://github.com/fleetd-sh/fleetd/wiki/API-Documentation)
- [Device Management](https://github.com/fleetd-sh/fleetd/wiki/Device-Management)
- [Deployment Guide](https://github.com/fleetd-sh/fleetd/wiki/Deployment-Guide)
- [Troubleshooting](https://github.com/fleetd-sh/fleetd/wiki/Troubleshooting)
- [Contributing](https://github.com/fleetd-sh/fleetd/wiki/Contributing)

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
│   ├── fleetctl/          # Management CLI
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
├── wiki/                 # Documentation wiki (submodule)
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
- [Documentation](https://github.com/fleetd-sh/fleetd/wiki)
- [Discord Community](https://discord.gg/fleetd)

## 🎯 Roadmap

- [ ] Multi-cloud support (AWS, Azure, GCP)
- [ ] Enhanced security features (mTLS, HSM support)
- [ ] Advanced deployment strategies (canary, blue-green)
- [ ] IoT device support expansion
- [ ] Kubernetes operator
- [ ] Mobile app for fleet management
- [ ] AI-powered anomaly detection