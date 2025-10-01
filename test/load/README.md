# fleetd Load Testing Framework

A comprehensive load testing infrastructure for fleetd that can simulate thousands of virtual devices and test various scenarios including device onboarding, steady-state operations, update campaigns, and network resilience.

## Features

- **Virtual Device Simulation**: Simulate thousands of devices with different profiles (full, constrained, minimal)
- **Multiple Test Scenarios**: Onboarding storms, steady-state operations, update campaigns, network resilience
- **Real-time Dashboard**: Web-based dashboard with live metrics and charts
- **Comprehensive Reporting**: Detailed performance reports in HTML, JSON, and CSV formats
- **CI/CD Integration**: GitHub Actions workflows and integration tools
- **Docker Support**: Containerized testing environment
- **Performance Monitoring**: System resource tracking and alerting

## Quick Start

### Prerequisites

- Go 1.21 or later
- Docker (optional, for containerized testing)
- fleetd server running

### Basic Usage

1. **Quick smoke test** (50 devices, 2 minutes):
   ```bash
   cd test/load/scripts
   ./quick_test.sh quick
   ```

2. **Onboarding storm test** (100 devices):
   ```bash
   ./quick_test.sh onboarding -d 100 -t 5m
   ```

3. **Stress test** (1000 devices, 15 minutes):
   ```bash
   ./quick_test.sh stress -d 1000 -t 15m
   ```

4. **Full test suite**:
   ```bash
   ./quick_test.sh full --verbose
   ```

### Advanced Usage

**Using Go directly**:
```bash
cd test/load/scripts
go run run_load_test.go \
  -server="http://localhost:8080" \
  -devices=500 \
  -duration=10m \
  -onboarding=true \
  -steady-state=true \
  -dashboard=true \
  -dashboard-port=8081
```

**Using Docker**:
```bash
cd test/load/scripts
./docker_test.sh setup
./docker_test.sh stress -c 3 -d 300  # 3 containers, 300 devices each
```

## Test Scenarios

### 1. Onboarding Storm
Tests rapid device registration and initial connection handling.

**Parameters**:
- Device registration rate
- Burst size and intervals
- Success rate thresholds
- Latency requirements

**Metrics**:
- Registration latency (P50, P95, P99)
- Success rate
- Server resource usage
- Connection handling

### 2. Steady State Operations
Tests sustained performance under normal operating conditions.

**Parameters**:
- Device count and profiles
- Metrics upload frequency
- Heartbeat intervals
- Test duration

**Metrics**:
- Sustained throughput
- Response time stability
- Resource efficiency
- Error rates

### 3. Update Campaign
Simulates fleet-wide software updates with canary deployments.

**Parameters**:
- Update batch size
- Rollout strategy
- Rollback thresholds
- Update duration

**Metrics**:
- Update success rate
- Rollout timing
- Rollback effectiveness
- Device availability

### 4. Network Resilience
Tests system behavior under network issues and device reconnections.

**Parameters**:
- Network event types and severity
- Recovery time targets
- Reconnection patterns
- Failure rates

**Metrics**:
- Recovery time (P95, P99)
- Reconnection success rate
- System stability
- Error handling

## Device Profiles

### Full Profile
- **Characteristics**: High-end devices with full capabilities
- **Resources**: 8 CPU cores, 16GB RAM, 1TB storage
- **Behavior**: Frequent metrics, low error rate, fast responses
- **Use Case**: Production servers, high-performance devices

### Constrained Profile
- **Characteristics**: Mid-range devices with limited resources
- **Resources**: 4 CPU cores, 4GB RAM, 256GB storage
- **Behavior**: Moderate metrics frequency, occasional errors
- **Use Case**: IoT gateways, edge devices

### Minimal Profile
- **Characteristics**: Resource-limited devices
- **Resources**: 2 CPU cores, 1GB RAM, 32GB storage
- **Behavior**: Infrequent metrics, higher error rate, slower responses
- **Use Case**: Sensors, embedded devices

## Dashboard

The real-time dashboard provides live monitoring of:

- **System Metrics**: CPU, memory, network usage
- **Performance Metrics**: Throughput, latency, error rates
- **Device Metrics**: Active devices, connection status
- **Test Progress**: Scenario status, completion rates
- **Alerts**: System health warnings and critical issues

Access the dashboard at `http://localhost:8081` (default port).

## Reports

Comprehensive reports are generated in multiple formats:

### HTML Report
- Executive summary with grades and scores
- Detailed performance analysis
- Resource usage charts
- Recommendations and issues
- Test configuration details

### JSON Report
- Machine-readable format
- Complete metrics data
- Suitable for automation and monitoring systems

### CSV Report
- Key metrics in spreadsheet format
- Easy import into analytics tools

## CI/CD Integration

### GitHub Actions

The framework includes GitHub Actions workflows for:

1. **Quick Tests**: Run on every PR and push
2. **Comprehensive Tests**: Manual or scheduled execution
3. **Performance Regression**: Compare against baselines
4. **Scheduled Monitoring**: Daily performance checks

#### Workflow Examples

**Basic PR Check**:
```yaml
- name: Run Load Test
  run: |
    cd test/load/scripts
    go run run_load_test.go -devices=50 -duration=2m
```

**Advanced Scenario**:
```yaml
- name: Stress Test
  run: |
    ./quick_test.sh stress -d 1000 -t 15m --no-dashboard
```

### CI Integration Tool

Process load test results for CI systems:

```bash
# Generate CI-friendly output
go run ci_integration.go \
  -report=./results/load_test_report.json \
  -format=junit \
  -exit-on-failure=true

# GitHub Actions format
go run ci_integration.go \
  -report=./results/load_test_report.json \
  -format=github
```

## Configuration

### Environment Variables

```bash
export FLEETD_SERVER_URL="https://api.example.com"
export FLEETD_AUTH_TOKEN="your-auth-token"
export LOAD_TEST_OUTPUT_DIR="./custom_results"
export DASHBOARD_PORT="8082"
```

### Command Line Options

```bash
# Server configuration
-server="http://localhost:8080"    # Server URL
-auth-token="token"                # Authentication token
-tls=true                         # Enable TLS

# Test configuration
-devices=100                      # Number of devices
-duration=10m                     # Test duration
-target-rps=1000                  # Target requests per second

# Device profiles
-full-devices=30                  # Full-featured devices
-constrained-devices=50           # Constrained devices
-minimal-devices=20               # Minimal devices

# Scenarios
-onboarding=true                  # Run onboarding test
-steady-state=true                # Run steady state test
-update-campaign=false            # Skip update campaign
-network-resilience=false         # Skip resilience test

# Output
-output="./results"               # Output directory
-report-formats="html,json,csv"   # Report formats
-dashboard=true                   # Enable dashboard
-dashboard-port=8081              # Dashboard port
-verbose=true                     # Verbose logging
```

## Performance Thresholds

Default thresholds for pass/fail determination:

| Metric | Threshold | Description |
|--------|-----------|-------------|
| Success Rate | ≥ 95% | Percentage of successful requests |
| Error Rate | ≤ 5% | Percentage of failed requests |
| P95 Latency | ≤ 100ms | 95th percentile response time |
| Throughput | ≥ 100 req/s | Requests processed per second |
| CPU Usage | ≤ 80% | Peak CPU utilization |
| Memory Usage | ≤ 85% | Peak memory utilization |

Customize thresholds:
```bash
go run run_load_test.go \
  -min-success-rate=0.98 \
  -max-latency=50ms \
  -target-rps=2000
```

## Docker Usage

### Setup
```bash
cd test/load/scripts
./docker_test.sh setup
```

### Run Tests
```bash
# Quick test
./docker_test.sh quick

# Stress test with multiple containers
./docker_test.sh stress -c 5 -d 200  # 5 containers, 200 devices each

# Custom duration
./docker_test.sh stress -t 30m
```

### Monitor
```bash
./docker_test.sh status  # Show container status
./docker_test.sh logs    # Show container logs
```

### Cleanup
```bash
./docker_test.sh clean
```

## Examples

### Example 1: Basic Load Test

```bash
# Test 100 devices for 5 minutes
cd test/load/scripts
go run run_load_test.go \
  -server="http://localhost:8080" \
  -devices=100 \
  -duration=5m \
  -onboarding=true \
  -steady-state=true
```

### Example 2: High-Scale Stress Test

```bash
# Test 1000 devices with specific profiles
go run run_load_test.go \
  -server="https://api.fleetd.example.com" \
  -devices=1000 \
  -full-devices=200 \
  -constrained-devices=500 \
  -minimal-devices=300 \
  -duration=30m \
  -tls=true \
  -auth-token="$AUTH_TOKEN"
```

### Example 3: CI/CD Pipeline

```bash
# Run test and check results
go run run_load_test.go -devices=50 -duration=2m -dashboard=false

# Process results for CI
go run ci_integration.go \
  -format=github \
  -success-rate-min=0.95 \
  -p95-latency-max=100ms \
  -exit-on-failure=true
```

### Example 4: Docker Multi-Container

```bash
# Setup environment
./docker_test.sh setup

# Run distributed load test
./docker_test.sh stress -c 10 -d 100 -t 20m

# Monitor and cleanup
./docker_test.sh status
./docker_test.sh clean
```

## Troubleshooting

### Common Issues

1. **"Connection refused" errors**
   - Ensure fleetd server is running
   - Check server URL and port
   - Verify network connectivity

2. **High memory usage**
   - Reduce device count
   - Decrease metrics frequency
   - Use minimal device profiles

3. **Dashboard not accessible**
   - Check dashboard port availability
   - Verify firewall settings
   - Try different port with `-dashboard-port`

4. **Test timeouts**
   - Increase test duration
   - Reduce device count
   - Check server capacity

### Debug Mode

Enable verbose logging:
```bash
go run run_load_test.go -verbose=true
```

Check system resources:
```bash
# Monitor during test
htop
iostat 1
netstat -an | grep :8080
```

### Performance Tuning

**For high device counts**:
```bash
# Increase file descriptor limits
ulimit -n 65536

# Tune TCP settings
echo 'net.core.somaxconn = 65536' >> /etc/sysctl.conf
echo 'net.ipv4.tcp_max_syn_backlog = 65536' >> /etc/sysctl.conf
sysctl -p
```

**For better performance**:
- Use SSD storage for databases
- Increase server memory allocation
- Use connection pooling
- Enable compression for metrics

## Contributing

1. Fork the repository
2. Create a feature branch
3. Add tests for new functionality
4. Ensure all tests pass
5. Submit a pull request

### Development Setup

```bash
# Clone and setup
git clone https://github.com/your-org/fleetd.git
cd fleetd/test/load

# Install dependencies
go mod download

# Run tests
go test ./...

# Build and test
go build ./scripts/run_load_test.go
./run_load_test -devices=10 -duration=30s
```

## License

This load testing framework is part of the fleetd project and follows the same license terms.

## Support

For questions, issues, or contributions:

- GitHub Issues: [fleetd/issues](https://github.com/your-org/fleetd/issues)
- Documentation: [fleetd docs](https://docs.fleetd.example.com)
- Community: [fleetd discussions](https://github.com/your-org/fleetd/discussions)