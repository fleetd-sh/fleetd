# FleetD Authentication Testing

This document describes the authentication testing strategy. Authentication tests are integrated into the main test suite for simplicity and consistency.

## Test Structure

The authentication system has three levels of testing:

### 1. Unit Tests (`internal/auth/device_flow_test.go`)
- Tests individual functions in isolation
- Uses sqlmock for database mocking
- Fast execution, no external dependencies
- Coverage: device code generation, user code validation, token exchange logic

### 2. Integration Tests (`test/integration/auth_device_flow_test.go`)
- Tests HTTP endpoints with real database
- Validates request/response handling
- Tests database interactions
- Coverage: API endpoints, database operations, error handling

### 3. E2E Tests (`test/e2e/auth_flow_test.go`)
- Tests complete authentication flow
- Simulates real CLI behavior
- Tests with all services running
- Coverage: full device flow, token persistence, CLI integration

## Running Tests

### Using Just Commands

```bash
# Run all tests (includes auth tests)
just test-all

# Run unit tests (includes auth unit tests)
just test

# Run integration tests (includes auth integration tests)
just test-integration

# Run e2e tests (includes auth e2e tests)
just test-e2e

# Generate coverage report
just test-coverage
```

### Using Go Test Directly

```bash
# Run specific auth unit tests
go test -v -race ./internal/auth/...

# Run specific auth integration tests
INTEGRATION=1 JWT_SECRET=test-secret-key go test -v ./test/integration/auth_device_flow_test.go

# Run specific auth e2e tests
E2E=1 JWT_SECRET=test-secret-key go test -v ./test/e2e/auth_flow_test.go
```

## Test Environment

### Prerequisites

1. **Docker**: Required for PostgreSQL in integration tests
2. **Go 1.21+**: Required for running tests

### Test Database

Integration and E2E tests use PostgreSQL:
- Database: `fleetd_test`
- User: `fleetd`
- Password: `fleetd` or `fleetd_secret`
- Connection: Handled automatically by CI or local Docker

## Test Coverage

### What's Tested

#### Device Flow Authentication
- ✅ Device code generation (unique, secure)
- ✅ User code generation (user-friendly format)
- ✅ Code verification endpoint
- ✅ Code approval flow
- ✅ Token exchange (polling)
- ✅ Token persistence
- ✅ Token revocation

#### Error Cases
- ✅ Invalid client_id
- ✅ Expired codes
- ✅ Already used codes
- ✅ Invalid device codes
- ✅ Authorization pending
- ✅ Database errors

#### Security
- ✅ Token expiration
- ✅ Code expiration (15 minutes)
- ✅ Polling interval enforcement
- ✅ One-time code usage

## CI/CD Integration

Authentication tests run automatically as part of the main CI pipeline:
- **GitHub Actions workflow**: `.github/workflows/ci.yml`
- Triggered on pushes to `main` or `develop` branches
- Triggered on pull requests

### CI Pipeline Integration

1. **Unit Tests**: Run in `test-go` job (includes all unit tests)
2. **Integration Tests**: Run in `test-integration` job (includes auth integration)
3. **Security Scan**: Run in `security` job (scans all code)

## Debugging Test Failures

### Common Issues

#### Database Connection Failed
```bash
# Check if database is running
docker ps | grep postgres

# Start PostgreSQL for local testing
docker-compose up -d postgres
```

#### Port Already in Use
```bash
# Kill processes on common ports
lsof -ti:8090 | xargs kill -9  # Platform API
lsof -ti:3000 | xargs kill -9  # Web UI
lsof -ti:5432 | xargs kill -9  # PostgreSQL
```

#### Migration Errors
```bash
# Reset database
docker-compose down -v
docker-compose up -d postgres

# Run migrations manually if needed
for migration in internal/database/migrations/*.up.sql; do
  psql $DATABASE_URL -f $migration
done
```

### Viewing Test Logs

```bash
# View container logs
docker-compose logs platform-api
docker-compose logs postgres

# View test output with verbose mode
go test -v -race ./internal/auth/... 2>&1 | tee test.log

# View specific test failures
just test-run TestDeviceFlow
```

## Test Data

### Default Test Users
- Email: `test@example.com`
- Role: `admin`

### Test Client IDs
- `fleetctl`: CLI client (allowed)
- `invalid-client`: Test invalid client (rejected)

### Test Codes Format
- Device Code: 43 characters, base64 URL-safe
- User Code: 8 characters, format `XXXX-XXXX`
- Allowed characters: `ABCDEFGHJKLMNPQRSTUVWXYZ23456789` (no ambiguous characters)

## Performance Benchmarks

```bash
# Run benchmarks
go test -bench=. -benchmem ./internal/auth/...

# Run with CPU profile
go test -bench=. -cpuprofile=cpu.prof ./internal/auth/...
go tool pprof cpu.prof
```

Expected performance:
- Device code generation: < 1ms
- User code generation: < 0.5ms
- Token exchange: < 50ms
- Code verification: < 10ms

## Adding New Tests

When adding authentication features:

1. **Add unit tests** in `internal/auth/*_test.go`
2. **Add integration tests** in `test/integration/auth_*_test.go`
3. **Update E2E tests** if the flow changes
4. **Update this documentation**

### Test Template

```go
func TestNewAuthFeature(t *testing.T) {
    // Arrange
    suite := setupAuthTestSuite(t)
    defer suite.cleanup()

    // Act
    result, err := suite.YourNewFunction()

    // Assert
    assert.NoError(t, err)
    assert.NotNil(t, result)
}
```

## Security Testing

```bash
# Check for hardcoded secrets
grep -r "password\|secret\|key" --include="*.go" internal/auth/

# Run security scanner
gosec ./internal/auth/...

# Check dependencies for vulnerabilities
go list -json -deps ./... | nancy sleuth
```

## Troubleshooting

### Test Hanging
- Check polling timeout (default: 15 minutes)
- Verify database connections
- Check for deadlocks in concurrent tests

### Flaky Tests
- Use `testify/suite` for proper setup/teardown
- Clean test data between runs
- Use unique identifiers for parallel tests

### Coverage Issues
- Run with `-coverprofile=coverage.out`
- View HTML report: `go tool cover -html=coverage.out`
- Aim for >80% coverage on auth code

## Quick Reference

```bash
# Most common commands
just test           # Run unit tests
just test-all       # Run everything
just test-coverage  # Generate coverage report

# Debug specific test
go test -v -run TestDeviceFlow_CreateDeviceAuth ./internal/auth/...

# Check what's not covered
go test -coverprofile=c.out ./internal/auth/...
go tool cover -func=c.out | grep -v 100.0%
```