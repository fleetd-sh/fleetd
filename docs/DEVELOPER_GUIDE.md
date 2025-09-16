# FleetD Developer Guide

## Table of Contents
- [Getting Started](#getting-started)
- [Architecture Overview](#architecture-overview)
- [Development Setup](#development-setup)
- [Code Structure](#code-structure)
- [Testing](#testing)
- [Contributing](#contributing)
- [Best Practices](#best-practices)

## Getting Started

### Prerequisites

- Go 1.23+
- Bun (for web development)
- Docker & Docker Compose
- PostgreSQL 15+ (or SQLite for development)
- Valkey/Redis 7+ (for caching)
- Protocol Buffers compiler (buf)
- Just (command runner)

### Quick Start

```bash
# Clone repository
git clone https://github.com/fleetd/fleetd.git
cd fleetd

# Install all dependencies
just install

# Check required tools
just check-tools

# Generate protobuf code
just proto

# Run tests
just test-all

# Start development environment
just dev

# Build binaries
just build-all
```

## Architecture Overview

### Core Components

```
fleetd/
├── cmd/                    # Command-line applications
│   ├── fleetd/            # Device agent
│   ├── fleets/            # Fleet server
│   ├── fleet/             # Management CLI
│   └── discover/          # Discovery service
├── internal/              # Private application code
│   ├── api/              # API handlers
│   ├── agent/            # Agent implementation
│   ├── database/         # Database layer
│   ├── ferrors/          # Error handling
│   ├── middleware/       # HTTP/RPC middleware
│   ├── observability/    # Metrics & tracing
│   ├── provision/        # Device provisioning
│   ├── sync/             # Synchronization
│   └── telemetry/        # Telemetry collection
├── gen/                   # Generated code
│   ├── agent/v1/         # Agent protobuf
│   ├── fleetd/v1/        # Fleet protobuf
│   └── public/v1/        # Public API protobuf
├── proto/                 # Protocol buffer definitions
│   ├── agent/v1/         # Agent API
│   ├── fleetd/v1/        # Fleet API
│   └── public/v1/        # Public API
├── web/                   # Next.js dashboard
│   ├── app/              # App router
│   ├── components/       # React components
│   └── lib/              # Utilities
└── test/                 # Integration tests
```

### Technology Stack

- **Backend**: Go 1.23+
- **Frontend**: Next.js 15 with TypeScript
- **UI Components**: shadcn/ui + Radix UI
- **Styling**: Tailwind CSS
- **API**: Connect-RPC (gRPC-Web compatible)
- **Database**: PostgreSQL/SQLite
- **Caching**: Valkey (Redis fork)
- **Metrics**: VictoriaMetrics
- **Logs**: Loki
- **Analytics**: ClickHouse
- **Gateway**: Traefik

### Design Patterns

#### 1. Repository Pattern
```go
// Repository interface
type DeviceRepository interface {
    List(ctx context.Context, opts ListOptions) ([]*Device, error)
    Get(ctx context.Context, id string) (*Device, error)
    Create(ctx context.Context, device *Device) error
    Update(ctx context.Context, device *Device) error
    Delete(ctx context.Context, id string) error
}

// Implementation
type deviceRepository struct {
    db     *database.DB
    logger *slog.Logger
}

func (r *deviceRepository) Get(ctx context.Context, id string) (*Device, error) {
    // Implementation with error handling, metrics, tracing
}
```

#### 2. Error Handling with ferrors
```go
// Custom error handling
import "fleetd.sh/internal/ferrors"

// Creating errors
err := ferrors.New(ferrors.CodeNotFound, "device not found")
    .WithMetadata("device_id", deviceID)
    .WithCause(originalErr)

// Wrapping errors
if err != nil {
    return ferrors.Wrap(err, ferrors.CodeInternal, "failed to process device")
}

// Checking error codes
if ferrors.Code(err) == ferrors.CodeNotFound {
    // Handle not found
}
```

#### 3. Middleware Chain
```go
// HTTP middleware
router.Use(
    middleware.Recovery(),
    middleware.RequestID(),
    middleware.Logger(),
    middleware.CORS(),
    middleware.RateLimiter(),
    middleware.Authentication(),
)

// RPC interceptors
interceptors := []connect.Interceptor{
    interceptor.Logger(),
    interceptor.Auth(),
    interceptor.Metrics(),
    interceptor.Tracing(),
}
```

## Development Setup

### Local Development

```bash
# Start data stack (PostgreSQL, VictoriaMetrics, Loki, etc.)
just stack-up

# Start development servers (backend + frontend)
just dev

# Or run separately:
just server-dev  # Backend only
just web-dev     # Frontend only

# Watch mode for backend
just server-watch
```

### Environment Configuration

Create `.envrc` file:

```bash
# Server
export FLEETS_PORT=8080
export FLEETS_HOST=0.0.0.0

# Database
export DATABASE_URL=postgresql://fleetd:password@localhost/fleetd
export DATABASE_MAX_CONNS=25

# Metrics
export VICTORIA_METRICS_URL=http://localhost:8428
export METRICS_ENABLED=true

# Logs
export LOKI_URL=http://localhost:3100
export LOG_LEVEL=debug

# Development
export DEBUG=true
export ENVIRONMENT=development
```

### Database Setup

```bash
# Run migrations
just db-migrate

# Create new migration
just db-migration add_new_feature

# Reset database
just db-reset

# Seed development data
just db-seed
```

## Code Structure

### Command Structure (Cobra)

```go
// cmd/fleets/cmd/root.go
var rootCmd = &cobra.Command{
    Use:   "fleets",
    Short: "FleetD server management CLI",
}

// Add subcommands
func init() {
    rootCmd.AddCommand(serverCmd)
    rootCmd.AddCommand(discoverCmd)
    rootCmd.AddCommand(devicesCmd)
    rootCmd.AddCommand(versionCmd)
}
```

### API Handlers (Connect-RPC)

```go
// internal/api/device_handler.go
type DeviceHandler struct {
    repo DeviceRepository
    auth AuthService
}

func (h *DeviceHandler) ListDevices(
    ctx context.Context,
    req *connect.Request[fleetv1.ListDevicesRequest],
) (*connect.Response[fleetv1.ListDevicesResponse], error) {
    // Authentication
    if err := h.auth.Authorize(ctx, "device:read"); err != nil {
        return nil, connect.NewError(connect.CodePermissionDenied, err)
    }

    // Business logic
    devices, err := h.repo.List(ctx, toListOptions(req.Msg))
    if err != nil {
        return nil, ferrors.ToConnectError(err)
    }

    // Response
    return connect.NewResponse(&fleetv1.ListDevicesResponse{
        Devices: toProtoDevices(devices),
    }), nil
}
```

### Frontend API Client

```typescript
// web/lib/api/client.ts
import { createPromiseClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { DeviceService } from "./gen/fleetd/v1/device_connect";

const transport = createConnectTransport({
  baseUrl: process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080",
  credentials: "include",
});

export const deviceClient = createPromiseClient(DeviceService, transport);

// Usage in React component
const { data, error } = useQuery({
  queryKey: ["devices"],
  queryFn: () => deviceClient.listDevices({}),
});
```

## Testing

### Running Tests

```bash
# All tests
just test-all

# Go tests only
just test-go

# Specific test
just test-go-run TestDeviceRepository

# With coverage
just test-go-coverage

# Integration tests
just test-go-integration

# Web tests
just test-web

# Type checking
just test-web-types
```

### Writing Tests

#### Unit Tests
```go
// internal/repository/device_test.go
func TestDeviceRepository_Get(t *testing.T) {
    // Setup
    db := database.NewTestDB(t)
    repo := NewDeviceRepository(db)

    // Test data
    device := &Device{
        ID:   "test-123",
        Name: "Test Device",
    }

    // Create
    err := repo.Create(context.Background(), device)
    require.NoError(t, err)

    // Get
    got, err := repo.Get(context.Background(), device.ID)
    require.NoError(t, err)
    assert.Equal(t, device.Name, got.Name)
}
```

#### Integration Tests
```go
// test/integration/device_test.go
func TestDeviceLifecycle(t *testing.T) {
    if !testing.Short() {
        t.Skip("Integration test")
    }

    // Start test server
    srv := testserver.New(t)
    defer srv.Close()

    // Client
    client := srv.Client()

    // Full lifecycle test
    // ...
}
```

#### Frontend Tests
```typescript
// web/components/device-list.test.tsx
import { render, screen } from "@testing-library/react";
import { DeviceList } from "./device-list";

describe("DeviceList", () => {
  it("renders devices", async () => {
    render(<DeviceList devices={mockDevices} />);

    expect(screen.getByText("Device 1")).toBeInTheDocument();
    expect(screen.getAllByRole("row")).toHaveLength(3);
  });
});
```

## Protocol Buffers

### Working with Protos

```bash
# Format proto files
just proto-format

# Lint proto files
just proto-lint

# Generate code
just proto

# Check for breaking changes
just proto-breaking
```

### Proto Structure
```protobuf
// proto/fleetd/v1/device.proto
syntax = "proto3";

package fleetd.v1;

service DeviceService {
  rpc ListDevices(ListDevicesRequest) returns (ListDevicesResponse);
  rpc GetDevice(GetDeviceRequest) returns (Device);
  rpc UpdateDevice(UpdateDeviceRequest) returns (Device);
}

message Device {
  string id = 1;
  string name = 2;
  DeviceStatus status = 3;
  google.protobuf.Timestamp created_at = 4;
}
```

## Building & Deployment

### Building Binaries

```bash
# Build all
just build-all

# Build specific binary
just build fleetd
just build fleets
just build fleet

# Cross-compilation
just build fleetd arm64
just build fleetd amd64
```

### Docker Images

```bash
# Build images
just docker-build
just docker-build-web

# Run with docker-compose
just docker-up

# View logs
just docker-logs fleets
```

### Release Process

```bash
# Create release
just release 1.0.0

# Deploy to environment
just deploy production
```

## Best Practices

### Code Style

- Use `gofmt` and `goimports` for Go code
- Use Biome for TypeScript/JavaScript
- Follow [Effective Go](https://golang.org/doc/effective_go.html)
- Use meaningful variable names
- Keep functions small and focused

### Error Handling

- Always use `ferrors` package for error creation
- Include context in error messages
- Use appropriate error codes
- Log errors at the appropriate level

```go
// Good
if err != nil {
    return ferrors.Wrap(err, ferrors.CodeInternal, "failed to update device")
        .WithMetadata("device_id", deviceID)
}

// Bad
if err != nil {
    return fmt.Errorf("error: %v", err)
}
```

### Security

- Never log sensitive data (passwords, tokens, keys)
- Always validate input
- Use prepared statements for SQL
- Implement rate limiting
- Use TLS for all network communication

### Performance

- Use connection pooling
- Implement caching where appropriate
- Profile before optimizing
- Use batch operations for bulk data
- Implement pagination for large datasets

### Documentation

- Document all exported functions
- Include examples in documentation
- Keep README files up to date
- Document breaking changes
- Use meaningful commit messages

## Contributing

### Pull Request Process

1. Fork the repository
2. Create feature branch: `git checkout -b feature/amazing-feature`
3. Make changes and test: `just test-all`
4. Format code: `just format-all`
5. Lint code: `just lint-all`
6. Commit: `git commit -m 'feat: add amazing feature'`
7. Push: `git push origin feature/amazing-feature`
8. Open Pull Request

### Commit Message Format

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
feat: add device export functionality
fix: resolve connection timeout issue
docs: update API documentation
test: add integration tests for sync
refactor: simplify error handling
chore: update dependencies
```

### Code Review Checklist

- [ ] Tests pass (`just test-all`)
- [ ] Code is formatted (`just format-all`)
- [ ] No linting errors (`just lint-all`)
- [ ] Documentation updated
- [ ] Breaking changes documented
- [ ] Security implications considered
- [ ] Performance impact assessed

## Debugging

### Local Debugging

```bash
# Run with debug logging
DEBUG=true LOG_LEVEL=debug just server-dev

# Use delve for Go debugging
dlv debug cmd/fleets/main.go -- server --port 8080

# Frontend debugging
just web-dev
# Open browser DevTools
```

### Common Issues

#### Database Connection
```bash
# Check PostgreSQL is running
docker ps | grep postgres

# Test connection
psql $DATABASE_URL -c "SELECT 1"
```

#### Port Already in Use
```bash
# Find process using port
lsof -i :8080

# Kill process
kill -9 <PID>
```

#### Proto Generation Failed
```bash
# Install/update buf
go install github.com/bufbuild/buf/cmd/buf@latest

# Clean and regenerate
rm -rf gen/
just proto
```

## Resources

- [Go Documentation](https://go.dev/doc/)
- [Connect-RPC Documentation](https://connect.build/)
- [Next.js Documentation](https://nextjs.org/docs)
- [Protocol Buffers](https://protobuf.dev/)
- [Project Issues](https://github.com/fleetd/fleetd/issues)
- [Discord Community](https://discord.gg/fleetd)