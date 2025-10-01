# fleetd 72-Hour Stability Testing Framework

A comprehensive stability testing framework designed to validate the long-running reliability and performance of fleetd. This framework continuously monitors system resources, detects memory leaks, validates data integrity, and ensures connection stability over extended periods.

## Overview

The stability testing framework provides:

- **Memory Leak Detection**: Monitors memory usage patterns and detects gradual increases indicating leaks
- **Resource Monitoring**: Tracks CPU usage, goroutines, file descriptors, and database connections
- **Connection Stability**: Validates network connections and database connection pools
- **Data Integrity**: Verifies data consistency and database integrity over time
- **Performance Monitoring**: Detects performance degradation using trend analysis
- **TLS Management**: Monitors certificate validity and renewal processes
- **Deadlock Detection**: Identifies potential deadlocks through goroutine analysis

## Quick Start

### Basic Usage

```bash
# Run a full 72-hour stability test
./scripts/run-stability-test.sh

# Run a quick 1-hour development test
./scripts/run-stability-test.sh --quick

# Test specific components for 4 hours
./scripts/run-stability-test.sh --duration 4h --components memory,connections

# Generate configuration template
./scripts/run-stability-test.sh --generate-config
```

### CI/CD Integration

```bash
# Run abbreviated CI stability test
./scripts/ci-stability-test.sh

# Run parallel test scenarios
./scripts/ci-stability-test.sh --parallel --duration 30m
```

## Framework Components

### 1. System Monitor (`monitor.go`)

Continuously monitors system resources and detects abnormal patterns:

- **Memory Monitoring**: Tracks RSS, VMS, and allocation patterns
- **CPU Monitoring**: Monitors process and system CPU usage
- **Resource Counting**: Tracks goroutines, file descriptors, and connections
- **Leak Detection**: Uses linear regression to detect resource leaks
- **Threshold Validation**: Alerts when usage exceeds configured limits

### 2. Validators (`validators.go`)

Individual validators for specific stability concerns:

#### Memory Leak Validator
```go
// Detects memory leaks using trend analysis
validator := NewMemoryLeakValidator(config, logger)
```

#### Connection Stability Validator
```go
// Validates network connection stability
endpoints := []string{"http://localhost:8080/health", "localhost:8080"}
validator := NewConnectionStabilityValidator(config, logger, endpoints)
```

#### Database Integrity Validator
```go
// Tests database integrity and connection pool health
validator := NewDatabaseIntegrityValidator(config, logger)
```

#### Deadlock Detector
```go
// Detects potential deadlocks
detector := NewDeadlockDetector(config, logger)
```

### 3. Test Framework (`framework.go`)

Main orchestration component that:

- Manages validator lifecycle
- Collects and persists metrics
- Generates comprehensive reports
- Handles graceful shutdown and cleanup

### 4. Test Runner (`runner.go`)

Command-line interface and execution engine:

- Configures test environment
- Sets up signal handling
- Manages test lifecycle
- Generates final reports

## Configuration

### Default Configuration

The framework uses sensible defaults for 72-hour testing:

```json
{
  "duration": "72h",
  "monitor_interval": "30s",
  "validation_interval": "5m",
  "metrics_interval": "1m",
  "max_memory_mb": 2048,
  "max_cpu_percent": 80.0,
  "max_goroutines": 10000,
  "memory_leak_threshold": 10.0,
  "performance_threshold": 20.0,
  "enabled_components": [
    "memory", "cpu", "goroutines", "connections",
    "database", "tls", "network", "data_integrity"
  ]
}
```

### Custom Configuration

Generate a configuration template:

```bash
go run ./test/stability/main.go -generate-config stability-config.json
```

Validate configuration:

```bash
go run ./test/stability/main.go -validate-config stability-config.json
```

## Pass/Fail Criteria

### Memory Leak Detection

**PASS**: Memory usage increase < 10% over test duration
**FAIL**: Memory usage increase ≥ 10% with positive trend

```go
// Memory leak threshold (percentage increase)
MemoryLeakThreshold: 10.0
```

### Resource Usage Thresholds

**PASS**: Resource usage remains within configured limits
**FAIL**: Any resource exceeds threshold for sustained period

```go
MaxMemoryMB:    2048,  // Maximum memory usage
MaxCPUPercent:  80.0,  // Maximum CPU usage
MaxGoroutines:  10000, // Maximum goroutine count
MaxOpenFiles:   1000,  // Maximum file descriptors
MaxConnections: 500,   // Maximum network connections
```

### Performance Degradation

**PASS**: Performance remains within 20% of baseline
**FAIL**: Performance degrades by >20% compared to initial measurements

```go
// Performance degradation threshold (percentage)
PerformanceThreshold: 20.0
```

### Connection Stability

**PASS**: Connection error rate < 5%
**FAIL**: Connection error rate ≥ 5%

### Data Integrity

**PASS**: All data integrity checks pass
**FAIL**: Any data corruption or checksum mismatch detected

### Deadlock Detection

**PASS**: No deadlocks detected
**FAIL**: Goroutine count stable at high level for extended period

## Output and Reporting

### Generated Files

The framework generates comprehensive output:

```
stability-results/
├── stability.log              # Detailed test logs
├── metrics.jsonl              # Time-series metrics data
├── stability-report.json      # Final test report
├── test-config.json          # Test configuration used
├── ci-stability-report.txt   # Human-readable summary
├── goroutine-dumps/          # Stack traces (if issues detected)
├── memory-profiles/          # Memory profiles (if leaks detected)
└── fleetd-logs/             # fleetd application logs
```

### Report Structure

```json
{
  "config": {...},
  "start_time": "2024-01-01T00:00:00Z",
  "end_time": "2024-01-04T00:00:00Z",
  "duration": "72h0m0s",
  "success": true,
  "peak_memory_mb": 256,
  "average_cpu_percent": 15.5,
  "max_goroutines": 150,
  "memory_leak_detected": false,
  "performance_degradation": false,
  "connection_stability": true,
  "data_integrity_issues": false,
  "errors": [],
  "alerts": [],
  "metrics_summary": {...}
}
```

## CI/CD Integration

### GitHub Actions

The framework includes a comprehensive GitHub Actions workflow:

```yaml
# .github/workflows/stability-test.yml
name: Stability Tests
on:
  schedule:
    - cron: '0 2 * * *'  # Daily at 2 AM UTC
  release:
    types: [published]
  workflow_dispatch:
```

### Test Scenarios

1. **Quick Stability Check** (PRs): 10-minute test focusing on critical components
2. **Standard Stability Test** (main branch): 30-minute test with multiple scenarios
3. **Extended Stability Test** (releases/scheduled): Full 72-hour test

### Parallel Testing

The CI system supports parallel test execution:

```bash
./scripts/ci-stability-test.sh --parallel
```

This runs multiple focused scenarios simultaneously:
- Memory-focused: Tests memory and goroutine management
- Network-focused: Tests connection stability and TLS
- Database-focused: Tests data integrity and connection pools

## Best Practices

### Running Tests

1. **Development**: Use quick tests (1-4 hours) for rapid feedback
2. **Pre-release**: Run standard tests (30 minutes) to catch regressions
3. **Production Validation**: Run full 72-hour tests before major releases

### Monitoring During Tests

```bash
# Monitor test progress
tail -f stability-results/stability.log

# Check current metrics
cat stability-results/metrics.jsonl | tail -1 | jq

# Monitor system resources
htop
```

### Interpreting Results

#### Green Flags (Test Passing)
- Memory usage stable or decreasing over time
- CPU usage within normal operating ranges
- Connection error rate < 1%
- No data integrity issues
- Performance remains consistent

#### Yellow Flags (Investigate)
- Memory usage showing slight upward trend (5-10% increase)
- Occasional connection errors (1-5% error rate)
- Performance degradation 10-20%
- High but stable resource usage

#### Red Flags (Test Failing)
- Clear memory leak pattern (>10% increase)
- High connection error rate (>5%)
- Significant performance degradation (>20%)
- Data corruption detected
- Deadlock conditions

## Troubleshooting

### Common Issues

#### Test Fails to Start
```bash
# Check binary exists
ls -la ./bin/fleetd

# Check configuration
go run ./test/stability/main.go -validate-config config.json

# Check permissions
ls -la ./scripts/run-stability-test.sh
chmod +x ./scripts/run-stability-test.sh
```

#### High Memory Usage
```bash
# Generate memory profile
go tool pprof ./bin/fleetd stability-results/memory-profile.prof

# Analyze goroutine dump
go tool trace stability-results/trace.out
```

#### Connection Failures
```bash
# Check fleetd logs
tail -f stability-results/fleetd-logs/fleetd.log

# Test connectivity manually
curl -v http://localhost:8080/health
```

### Debug Mode

Enable verbose logging:

```bash
./scripts/run-stability-test.sh --log-level debug
```

### Custom Validators

Add custom validators by implementing the `Validator` interface:

```go
type CustomValidator struct {
    logger *logrus.Logger
    config *Config
}

func (v *CustomValidator) Name() string { return "custom_validator" }
func (v *CustomValidator) Validate(ctx context.Context) error {
    // Implementation
    return nil
}
func (v *CustomValidator) Configure(config map[string]interface{}) error { return nil }
func (v *CustomValidator) Reset() error { return nil }
```

## Performance Considerations

### Resource Requirements

- **Memory**: 512MB minimum, 2GB recommended for full test
- **CPU**: 2+ cores recommended for parallel scenarios
- **Disk**: 1GB minimum for logs and metrics (10GB for 72-hour test)
- **Network**: Stable connection for duration of test

### Scaling Recommendations

For large-scale deployments:

1. Adjust thresholds based on expected load
2. Use distributed testing for multiple instances
3. Implement custom metrics for application-specific concerns
4. Consider regional testing for global deployments

## Contributing

### Adding New Validators

1. Implement the `Validator` interface
2. Add configuration options to `Config` struct
3. Register validator in `setupValidators()`
4. Add tests in `validators_test.go`
5. Update documentation

### Extending Monitoring

1. Add new metrics to `MetricsSnapshot`
2. Update collection in `SystemMonitor`
3. Add analysis in report generation
4. Include in pass/fail criteria

## License

This stability testing framework is part of the fleetd project and follows the same license terms.