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

- Go 1.21+
- Node.js 20+ (for web UI)
- Docker & Docker Compose
- PostgreSQL 15+ (or SQLite for development)
- Redis 7+ (optional, for caching)
- Protocol Buffers compiler

### Quick Start

```bash
# Clone repository
git clone https://github.com/fleetd/fleetd.git
cd fleetd

# Install dependencies
go mod download
cd web && npm install && cd ..

# Generate protobuf code
just generate

# Run tests
just test

# Start development environment
just dev

# Build binaries
just build
```

## Architecture Overview

### Core Components

```
fleetd/
├── cmd/                    # Command-line applications
│   ├── fleetd/            # Main server binary
│   ├── fleets/            # Fleet CLI tool
│   └── agent/             # Device agent
├── internal/              # Private application code
│   ├── api/              # API handlers
│   ├── agent/            # Agent implementation
│   ├── database/         # Database layer
│   ├── ferrors/          # Error handling
│   ├── middleware/       # HTTP/RPC middleware
│   ├── models/           # Data models
│   ├── observability/    # Metrics & tracing
│   ├── repository/       # Data access layer
│   ├── security/         # Auth & security
│   └── server/           # Server implementation
├── gen/                   # Generated code
│   └── fleetd/v1/        # Protobuf generated
├── proto/                 # Protocol buffer definitions
│   └── fleetd/v1/        # API definitions
├── web/                   # Web UI (Next.js)
├── plugins/              # Extension plugins
└── test/                 # Integration tests
```

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

#### 2. Service Layer
```go
type DeviceService struct {
    repo     DeviceRepository
    metrics  *observability.Metrics
    logger   *slog.Logger
}

func (s *DeviceService) RegisterDevice(ctx context.Context, req *RegisterRequest) (*RegisterResponse, error) {
    // Business logic
    // Validation
    // Repository calls
    // Event emission
    // Metrics recording
}
```

#### 3. Error Handling
```go
// Custom error types with context
err := ferrors.New(ferrors.ErrCodeNotFound, "device not found").
    WithMetadata("device_id", deviceID).
    WithRequestID(requestID)

// Error wrapping
if err := repo.Get(ctx, id); err != nil {
    return ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to get device")
}

// Circuit breaker integration
err := cb.Execute(ctx, func() error {
    return externalService.Call()
})
```

## Development Setup

### Local Environment

```bash
# Start dependencies
docker-compose -f docker-compose.dev.yml up -d

# Set environment variables
export DATABASE_URL="postgresql://fleetd:password@localhost:5432/fleetd_dev"
export REDIS_URL="redis://localhost:6379"
export JWT_SECRET="dev-secret"
export OTEL_EXPORTER_OTLP_ENDPOINT="http://localhost:4317"

# Run migrations
just migrate

# Start server with hot reload
just watch

# Start web UI dev server
cd web && npm run dev
```

### Docker Development

```dockerfile
# Dockerfile.dev
FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go install github.com/cosmtrek/air@latest

CMD ["air", "-c", ".air.toml"]
```

### VS Code Configuration

```json
// .vscode/launch.json
{
  "version": "0.2.0",
  "configurations": [
    {
      "name": "Debug Server",
      "type": "go",
      "request": "launch",
      "mode": "debug",
      "program": "${workspaceFolder}/cmd/fleetd",
      "args": ["server"],
      "env": {
        "DATABASE_URL": "postgresql://localhost:5432/fleetd_dev",
        "LOG_LEVEL": "debug"
      }
    },
    {
      "name": "Debug Agent",
      "type": "go",
      "request": "launch",
      "mode": "debug",
      "program": "${workspaceFolder}/cmd/agent",
      "args": ["--config", "config/agent.dev.yaml"]
    }
  ]
}
```

## Code Structure

### API Implementation

```go
// internal/api/device.go
package api

import (
    "context"
    "connectrpc.com/connect"
    pb "fleetd.sh/gen/fleetd/v1"
    "fleetd.sh/internal/service"
)

type DeviceHandler struct {
    service *service.DeviceService
}

func (h *DeviceHandler) RegisterDevice(
    ctx context.Context,
    req *connect.Request[pb.RegisterDeviceRequest],
) (*connect.Response[pb.RegisterDeviceResponse], error) {
    // Extract trace context
    ctx, span := observability.StartSpan(ctx, "RegisterDevice")
    defer span.End()

    // Validate request
    if err := h.validateRegisterRequest(req.Msg); err != nil {
        return nil, connect.NewError(connect.CodeInvalidArgument, err)
    }

    // Call service layer
    device, err := h.service.Register(ctx, req.Msg)
    if err != nil {
        return nil, h.handleError(err)
    }

    // Build response
    return connect.NewResponse(&pb.RegisterDeviceResponse{
        DeviceId:     device.ID,
        ApiKey:       device.APIKey,
        RegisteredAt: timestamppb.New(device.CreatedAt),
    }), nil
}
```

### Database Migrations

```go
// internal/migrations/001_create_devices.go
package migrations

import (
    "database/sql"
    "github.com/pressly/goose/v3"
)

func init() {
    goose.AddMigration(upCreateDevices, downCreateDevices)
}

func upCreateDevices(tx *sql.Tx) error {
    _, err := tx.Exec(`
        CREATE TABLE device (
            id VARCHAR(255) PRIMARY KEY,
            name VARCHAR(255) NOT NULL,
            type VARCHAR(100) NOT NULL,
            version VARCHAR(50) NOT NULL,
            api_key VARCHAR(255) UNIQUE,
            last_seen TIMESTAMP,
            metadata JSONB,
            created_at TIMESTAMP NOT NULL DEFAULT NOW(),
            updated_at TIMESTAMP NOT NULL DEFAULT NOW()
        );

        CREATE INDEX idx_device_type ON device(type);
        CREATE INDEX idx_device_last_seen ON device(last_seen);
        CREATE INDEX idx_device_created_at ON device(created_at);
    `)
    return err
}

func downCreateDevices(tx *sql.Tx) error {
    _, err := tx.Exec("DROP TABLE IF EXISTS device")
    return err
}
```

### Agent Implementation

```go
// internal/agent/agent.go
package agent

type Agent struct {
    config         *Config
    registryClient *RegistrationClient
    updateManager  *UpdateManager
    processManager *ProcessManager
    metricsCollector *MetricsCollector
}

func (a *Agent) Run(ctx context.Context) error {
    // Register with server
    if err := a.register(ctx); err != nil {
        return err
    }

    // Start components
    g, ctx := errgroup.WithContext(ctx)

    g.Go(func() error {
        return a.heartbeatLoop(ctx)
    })

    g.Go(func() error {
        return a.metricsLoop(ctx)
    })

    g.Go(func() error {
        return a.updateCheckLoop(ctx)
    })

    return g.Wait()
}
```

## Testing

### Unit Tests

```go
// internal/repository/device_test.go
package repository

import (
    "context"
    "testing"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestDeviceRepository_Create(t *testing.T) {
    // Setup
    db := setupTestDB(t)
    repo := NewDeviceRepository(db)

    // Test case
    device := &models.Device{
        ID:   "test-001",
        Name: "Test Device",
        Type: "test",
    }

    // Execute
    err := repo.Create(context.Background(), device)

    // Assert
    require.NoError(t, err)

    // Verify
    retrieved, err := repo.Get(context.Background(), device.ID)
    require.NoError(t, err)
    assert.Equal(t, device.ID, retrieved.ID)
    assert.Equal(t, device.Name, retrieved.Name)
}

func TestDeviceRepository_Create_Duplicate(t *testing.T) {
    db := setupTestDB(t)
    repo := NewDeviceRepository(db)

    device := &models.Device{ID: "test-001"}

    // First create should succeed
    err := repo.Create(context.Background(), device)
    require.NoError(t, err)

    // Second create should fail
    err = repo.Create(context.Background(), device)
    assert.Error(t, err)
    assert.Equal(t, ferrors.ErrCodeAlreadyExists, ferrors.GetCode(err))
}
```

### Integration Tests

```go
// test/integration/device_test.go
package integration

import (
    "context"
    "testing"
    "fleetd.sh/test/fixtures"
)

func TestDeviceLifecycle(t *testing.T) {
    // Start test server
    server := fixtures.StartTestServer(t)
    defer server.Stop()

    client := fixtures.NewTestClient(server.URL)

    // Register device
    resp, err := client.RegisterDevice(context.Background(), &pb.RegisterDeviceRequest{
        DeviceId:   "test-001",
        DeviceName: "Test Device",
        DeviceType: "test",
    })
    require.NoError(t, err)
    assert.NotEmpty(t, resp.ApiKey)

    // Send heartbeat
    hbResp, err := client.Heartbeat(context.Background(), &pb.HeartbeatRequest{
        DeviceId: "test-001",
        Status: &pb.DeviceStatus{
            State: pb.DeviceState_ONLINE,
        },
    })
    require.NoError(t, err)
    assert.True(t, hbResp.Acknowledged)

    // List devices
    listResp, err := client.ListDevices(context.Background(), &pb.ListDevicesRequest{})
    require.NoError(t, err)
    assert.Len(t, listResp.Devices, 1)
    assert.Equal(t, "test-001", listResp.Devices[0].Id)
}
```

### Load Testing

```go
// test/load/device_load_test.go
package load

import (
    "context"
    "testing"
    "time"
    "golang.org/x/sync/errgroup"
)

func TestDeviceRegistrationLoad(t *testing.T) {
    const (
        numDevices = 1000
        numWorkers = 10
    )

    client := NewTestClient()
    devices := make(chan int, numDevices)

    // Populate work queue
    for i := 0; i < numDevices; i++ {
        devices <- i
    }
    close(devices)

    // Start workers
    start := time.Now()
    g, ctx := errgroup.WithContext(context.Background())

    for w := 0; w < numWorkers; w++ {
        g.Go(func() error {
            for id := range devices {
                _, err := client.RegisterDevice(ctx, &pb.RegisterDeviceRequest{
                    DeviceId:   fmt.Sprintf("load-test-%d", id),
                    DeviceName: fmt.Sprintf("Load Test Device %d", id),
                })
                if err != nil {
                    return err
                }
            }
            return nil
        })
    }

    require.NoError(t, g.Wait())

    duration := time.Since(start)
    rps := float64(numDevices) / duration.Seconds()

    t.Logf("Registered %d devices in %v (%.2f req/s)", numDevices, duration, rps)
    assert.Greater(t, rps, 100.0, "Registration rate should exceed 100 req/s")
}
```

### Benchmarks

```go
// internal/ferrors/errors_bench_test.go
package ferrors

import "testing"

func BenchmarkErrorCreation(b *testing.B) {
    for i := 0; i < b.N; i++ {
        _ = New(ErrCodeInternal, "test error").
            WithMetadata("key", "value").
            WithRequestID("req-123")
    }
}

func BenchmarkErrorWrapping(b *testing.B) {
    baseErr := New(ErrCodeNotFound, "not found")

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _ = Wrap(baseErr, ErrCodeInternal, "wrapped error")
    }
}
```

## Contributing

### Development Workflow

1. **Fork and Clone**
   ```bash
   git clone https://github.com/yourusername/fleetd.git
   cd fleetd
   git remote add upstream https://github.com/fleetd/fleetd.git
   ```

2. **Create Feature Branch**
   ```bash
   git checkout -b feature/my-feature
   ```

3. **Make Changes**
   - Write code following style guide
   - Add tests for new functionality
   - Update documentation

4. **Run Tests**
   ```bash
   just test         # Unit tests
   just test-integration  # Integration tests
   just lint         # Linting
   just fmt          # Format code
   ```

5. **Commit Changes**
   ```bash
   git add .
   git commit -m "feat: add new feature

   - Detailed description
   - Closes #123"
   ```

6. **Push and Create PR**
   ```bash
   git push origin feature/my-feature
   ```

### Code Style

#### Go Guidelines
```go
// Good: Clear naming, proper error handling
func (s *DeviceService) GetDevice(ctx context.Context, id string) (*Device, error) {
    if id == "" {
        return nil, ferrors.New(ferrors.ErrCodeInvalidInput, "device ID is required")
    }

    device, err := s.repo.Get(ctx, id)
    if err != nil {
        return nil, ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to get device")
    }

    return device, nil
}

// Bad: Poor naming, no error context
func (s *DeviceService) Get(id string) (*Device, error) {
    d, e := s.repo.Get(context.Background(), id)
    if e != nil {
        return nil, e
    }
    return d, nil
}
```

#### TypeScript Guidelines
```typescript
// Good: Type safety, clear interfaces
interface Device {
  id: string;
  name: string;
  type: DeviceType;
  status: DeviceStatus;
  metadata?: Record<string, unknown>;
}

async function getDevice(id: string): Promise<Device> {
  if (!id) {
    throw new Error('Device ID is required');
  }

  const response = await client.device.get({ deviceId: id });
  return response.device;
}

// Bad: Any types, no validation
async function getDevice(id: any): Promise<any> {
  const response = await client.device.get({ deviceId: id });
  return response.device;
}
```

### Commit Messages

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
feat: add device metrics endpoint
fix: resolve memory leak in agent
docs: update API documentation
test: add integration tests for updates
refactor: simplify error handling
perf: optimize database queries
chore: update dependencies
```

## Best Practices

### 1. Error Handling
```go
// Always wrap errors with context
if err := db.Query(ctx, query); err != nil {
    return ferrors.Wrap(err, ferrors.ErrCodeInternal, "database query failed").
        WithMetadata("query", query).
        WithRequestID(GetRequestID(ctx))
}

// Use typed errors for known conditions
var ErrDeviceNotFound = ferrors.New(ferrors.ErrCodeNotFound, "device not found")

// Handle panics gracefully
defer func() {
    if r := recover(); r != nil {
        err := ferrors.Newf(ferrors.ErrCodeInternal, "panic: %v", r).
            WithStackTrace()
        logger.Error("Panic recovered", "error", err)
    }
}()
```

### 2. Context Usage
```go
// Pass context through entire call chain
func (s *Service) ProcessRequest(ctx context.Context, req Request) error {
    ctx, span := trace.StartSpan(ctx, "ProcessRequest")
    defer span.End()

    // Add request ID to context
    ctx = context.WithValue(ctx, "request_id", generateRequestID())

    // Use context for cancellation
    ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
    defer cancel()

    return s.doWork(ctx, req)
}
```

### 3. Concurrency
```go
// Use errgroup for concurrent operations
g, ctx := errgroup.WithContext(ctx)

g.Go(func() error {
    return s.processDevices(ctx)
})

g.Go(func() error {
    return s.collectMetrics(ctx)
})

if err := g.Wait(); err != nil {
    return err
}

// Protect shared state
type SafeCounter struct {
    mu    sync.RWMutex
    value int64
}

func (c *SafeCounter) Increment() {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.value++
}

func (c *SafeCounter) Get() int64 {
    c.mu.RLock()
    defer c.mu.RUnlock()
    return c.value
}
```

### 4. Resource Management
```go
// Always close resources
resp, err := http.Get(url)
if err != nil {
    return err
}
defer resp.Body.Close()

// Use connection pooling
db.SetMaxOpenConns(25)
db.SetMaxIdleConns(5)
db.SetConnMaxLifetime(5 * time.Minute)

// Implement graceful shutdown
sigCh := make(chan os.Signal, 1)
signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

<-sigCh
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

if err := server.Shutdown(ctx); err != nil {
    logger.Error("Shutdown error", "error", err)
}
```

### 5. Testing
```go
// Use table-driven tests
func TestValidateDevice(t *testing.T) {
    tests := []struct {
        name    string
        device  *Device
        wantErr bool
        errCode ferrors.ErrorCode
    }{
        {
            name:    "valid device",
            device:  &Device{ID: "test-001", Name: "Test"},
            wantErr: false,
        },
        {
            name:    "missing ID",
            device:  &Device{Name: "Test"},
            wantErr: true,
            errCode: ferrors.ErrCodeInvalidInput,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := ValidateDevice(tt.device)
            if tt.wantErr {
                assert.Error(t, err)
                assert.Equal(t, tt.errCode, ferrors.GetCode(err))
            } else {
                assert.NoError(t, err)
            }
        })
    }
}

// Use test fixtures
func setupTestDB(t *testing.T) *sql.DB {
    db, err := sql.Open("sqlite3", ":memory:")
    require.NoError(t, err)

    t.Cleanup(func() {
        db.Close()
    })

    return db
}
```

### 6. Performance
```go
// Profile code regularly
import _ "net/http/pprof"

go func() {
    log.Println(http.ListenAndServe("localhost:6060", nil))
}()

// Use sync.Pool for frequently allocated objects
var bufferPool = sync.Pool{
    New: func() any {
        return new(bytes.Buffer)
    },
}

func processData(data []byte) {
    buf := bufferPool.Get().(*bytes.Buffer)
    defer func() {
        buf.Reset()
        bufferPool.Put(buf)
    }()

    buf.Write(data)
    // Process buffer
}

// Avoid unnecessary allocations
// Good: Pre-allocate slice
devices := make([]*Device, 0, expectedCount)

// Bad: Growing slice
var devices []*Device
for _, d := range results {
    devices = append(devices, d)
}
```

## Development Tools

### Makefile/Justfile Commands

```bash
just generate     # Generate protobuf code
just build        # Build all binaries
just test         # Run unit tests
just test-integration  # Run integration tests
just test-e2e     # Run end-to-end tests
just lint         # Run linters
just fmt          # Format code
just migrate      # Run database migrations
just dev          # Start development environment
just docker-build # Build Docker images
just clean        # Clean build artifacts
```

### Useful Scripts

```bash
# scripts/setup-dev.sh
#!/bin/bash
set -e

echo "Setting up development environment..."

# Install tools
go install github.com/bufbuild/buf/cmd/buf@latest
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
go install github.com/cosmtrek/air@latest

# Setup database
createdb fleetd_dev
psql fleetd_dev < schema.sql

# Generate code
buf generate

echo "Development environment ready!"
```

## Debugging

### Remote Debugging

```go
// Enable Delve debugger
import _ "github.com/go-delve/delve/cmd/dlv"

// Start with debug mode
// dlv debug --headless --listen=:2345 --api-version=2 ./cmd/fleetd
```

### Tracing

```go
// Add custom spans
ctx, span := trace.StartSpan(ctx, "operation_name",
    trace.WithAttributes(
        attribute.String("key", "value"),
        attribute.Int("count", 42),
    ),
)
defer span.End()

// Record events
span.AddEvent("processing started")

// Record errors
if err != nil {
    span.RecordError(err)
    span.SetStatus(codes.Error, err.Error())
}
```

### Logging

```go
// Structured logging
logger.Info("Device registered",
    "device_id", device.ID,
    "type", device.Type,
    "duration_ms", time.Since(start).Milliseconds(),
)

// Debug logging
if logger.Enabled(ctx, slog.LevelDebug) {
    logger.Debug("Request details",
        "headers", req.Header,
        "body", req.Body,
    )
}
```

## Resources

- [Go Style Guide](https://google.github.io/styleguide/go/)
- [Effective Go](https://golang.org/doc/effective_go)
- [Connect RPC Documentation](https://connect.build/docs)
- [OpenTelemetry Go](https://opentelemetry.io/docs/instrumentation/go/)
- [Protocol Buffers](https://developers.google.com/protocol-buffers)
