# FleetD Developer Guide

This guide explains how to set up a development environment for FleetD and contribute to the project.

## Development Environment Setup

### Prerequisites

- Go 1.21 or later
- Protocol Buffers compiler (`protoc`)
- SQLite development libraries
- Git
- Make

#### Ubuntu/Debian
```bash
# Install Go
wget https://go.dev/dl/go1.21.0.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.21.0.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin

# Install other dependencies
sudo apt-get update
sudo apt-get install -y \
    protobuf-compiler \
    libsqlite3-dev \
    make \
    git
```

#### macOS
```bash
# Using Homebrew
brew install go protobuf sqlite make
```

#### Windows
```powershell
# Using Chocolatey
choco install golang protoc sqlite make git
```

### Getting the Code

1. Clone the repository:
```bash
git clone https://github.com/fleetd/fleetd.git
cd fleetd
```

2. Install Go dependencies:
```bash
go mod download
```

3. Install development tools:
```bash
make tools
```

## Building

### Building from Source

Build all binaries:
```bash
make build
```

Build specific components:
```bash
make build-server
make build-agent
```

Cross-compile for different platforms:
```bash
GOOS=linux GOARCH=arm64 make build
GOOS=windows GOARCH=amd64 make build
```

### Running Tests

Run all tests:
```bash
make test
```

Run specific test suites:
```bash
make test-unit
make test-integration
make test-e2e
```

Run tests with coverage:
```bash
make test-coverage
```

### Development Server

Run server in development mode:
```bash
make run-server
```

Run agent in development mode:
```bash
make run-agent
```

## Code Structure

```
.
├── cmd/                    # Command-line binaries
│   ├── fleetd/            # Server binary
│   └── fleetd-agent/      # Agent binary
├── internal/              # Internal packages
│   ├── agent/            # Agent implementation
│   ├── api/              # API implementations
│   ├── config/           # Configuration
│   ├── middleware/       # gRPC middleware
│   ├── storage/          # Storage backends
│   └── telemetry/        # Telemetry collection
├── pkg/                  # Public packages
│   ├── client/          # Client library
│   └── proto/           # Protocol definitions
├── proto/               # Protocol buffer definitions
├── sdk/                 # Language-specific SDKs
│   ├── go/             # Go SDK
│   └── python/         # Python SDK
└── test/               # Test suites
    ├── integration/    # Integration tests
    └── e2e/           # End-to-end tests
```

## Development Workflow

### Making Changes

1. Create a new branch:
```bash
git checkout -b feature/my-feature
```

2. Make changes and ensure tests pass:
```bash
make test
make lint
```

3. Commit changes with descriptive message:
```bash
git add .
git commit -m "feat: add new feature"
```

### Protocol Buffer Changes

1. Edit proto files in `proto/` directory

2. Generate code:
```bash
make proto
```

3. Update SDK examples if needed

### Database Changes

1. Create new migration in `internal/migrations/`:
```bash
make new-migration name=add_new_table
```

2. Edit the generated migration file

3. Run migrations in development:
```bash
make migrate-up
```

### Adding Dependencies

1. Add Go dependency:
```bash
go get github.com/example/package
```

2. Tidy and verify modules:
```bash
go mod tidy
go mod verify
```

## Testing

### Unit Tests

Write unit tests in `_test.go` files next to the code being tested:

```go
func TestMyFunction(t *testing.T) {
    // Setup
    svc := NewMyService()

    // Test
    result, err := svc.MyFunction()

    // Verify
    assert.NoError(t, err)
    assert.Equal(t, expected, result)
}
```

### Integration Tests

Write integration tests in `test/integration/`:

```go
func TestDeviceRegistration(t *testing.T) {
    // Setup test server
    server := setupTestServer(t)
    defer server.Close()

    // Create client
    client := newTestClient(server.URL)

    // Test registration
    device, err := client.RegisterDevice(...)
    assert.NoError(t, err)
    assert.NotEmpty(t, device.ID)
}
```

### End-to-End Tests

Write E2E tests in `test/e2e/`:

```go
func TestCompleteWorkflow(t *testing.T) {
    // Start server and agent
    env := setupTestEnvironment(t)
    defer env.Cleanup()

    // Test complete workflow
    t.Run("RegisterDevice", func(t *testing.T) {...})
    t.Run("UploadBinary", func(t *testing.T) {...})
    t.Run("CreateUpdate", func(t *testing.T) {...})
    t.Run("VerifyUpdate", func(t *testing.T) {...})
}
```

## Debugging

### Server Debugging

1. Enable debug logging:
```yaml
logging:
  level: debug
  format: text
```

2. Run with delve:
```bash
dlv debug ./cmd/fleetd/main.go
```

### Agent Debugging

1. Enable debug logging:
```yaml
logging:
  level: debug
  format: text
```

2. Run with environment variables:
```bash
FLEETD_DEBUG=1 ./fleetd-agent
```

### Common Issues

1. Proto generation issues:
```bash
make clean-proto
make proto
```

2. Database issues:
```bash
make clean-db
make migrate-up
```

3. Build issues:
```bash
make clean
go mod tidy
make build
```

## Contributing

### Code Style

- Follow Go style guide
- Use `gofmt` for formatting
- Add comments for exported functions
- Write descriptive commit messages

### Pull Requests

1. Create feature branch
2. Make changes
3. Run tests and linting
4. Push changes
5. Create pull request
6. Wait for review

### Documentation

- Update API docs for new endpoints
- Add godoc comments
- Update examples
- Update changelog

## Release Process

1. Update version:
```bash
make bump-version v=1.2.3
```

2. Update changelog:
```bash
make changelog
```

3. Create release:
```bash
make release
```

4. Push release:
```bash
make publish
```

## Getting Help

- Join Discord: https://discord.gg/fleetd
- GitHub Discussions: https://github.com/fleetd/fleetd/discussions
- Stack Overflow: Tag questions with `fleetd` 