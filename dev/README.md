# FleetD Mock Data Generator

A powerful development tool for generating realistic mock data for FleetD testing and demos.

## Features

- **Realistic Device Simulation**: 5 different device profiles (Raspberry Pi, Edge Servers, IoT Sensors, GPU Compute, Industrial Controllers)
- **Geographic Distribution**: Devices across 10 global locations with timezone-aware patterns
- **Real-time Data Generation**: Continuous telemetry generation every 5 seconds
- **Historical Data Seeding**: Generate days or weeks of historical data
- **Event Simulation**: Random device online/offline events, deployments, and errors
- **Database Support**: Works with both SQLite (default) and PostgreSQL

## Quick Start

```bash
# Make the runner script executable
chmod +x ./dev/run-mock-data.sh

# Run with presets
./dev/run-mock-data.sh small    # 10 devices, 3 days (quick testing)
./dev/run-mock-data.sh medium   # 50 devices, 7 days (default)
./dev/run-mock-data.sh large    # 200 devices, 14 days
./dev/run-mock-data.sh demo     # 25 devices, optimized for demos
./dev/run-mock-data.sh stress   # 1000 devices, 30 days (stress testing)

# Custom configuration
./dev/run-mock-data.sh -n 100 -d 14  # 100 devices, 14 days of history
```

## Device Profiles

### Raspberry Pi (`rpi`)
- Low-moderate CPU usage (25% base)
- Moderate memory usage (45% base)
- Typical edge computing patterns

### Edge Server (`edge`)
- Moderate CPU usage (40% base)
- Higher memory usage (60% base)
- Business hours patterns

### IoT Sensor (`sensor`)
- Minimal resource usage (5% CPU)
- Very low memory (20% base)
- Periodic burst patterns

### GPU Compute (`gpu`)
- High CPU usage (70% base)
- High memory usage (80% base)
- Batch processing patterns

### Industrial Controller (`plc`)
- Low CPU usage (15% base)
- Stable operation patterns
- Shift-based activity

## Data Patterns

The generator creates realistic patterns including:

- **Business Hours**: Higher load during 9-5
- **Time Zone Awareness**: Devices follow local time patterns
- **Network Issues**: Simulated connection drops
- **Gradual Disk Growth**: Realistic disk usage increase
- **Temperature Correlation**: CPU load affects temperature
- **Random Events**: Deployments, errors, maintenance

## Direct Usage

```bash
# Build the generator
go build -o ./dev/mock-generator ./dev/mock-data-generator.go

# Run with environment variables
DB_TYPE=sqlite \
DB_PATH=./dev/test.db \
NUM_DEVICES=25 \
SEED_DAYS=3 \
./dev/mock-generator
```

## PostgreSQL Setup

```bash
# Set PostgreSQL connection details
export DB_TYPE=postgres
export DB_HOST=localhost
export DB_PORT=5432
export DB_NAME=fleetd
export DB_USER=fleetd
export DB_PASSWORD=your_password

# Run the generator
./dev/run-mock-data.sh large
```

## Generated Data

The mock generator creates:

### Devices
- Unique IDs (`dev-rpi-0001`, `dev-edge-0002`, etc.)
- Geographic metadata (city, country, coordinates)
- Version information
- Last seen timestamps

### Telemetry
- CPU usage with daily patterns
- Memory usage correlated with CPU
- Disk usage with gradual growth
- Network traffic with occasional spikes
- Temperature readings

### Logs
- Multiple log levels (DEBUG, INFO, WARN, ERROR)
- Realistic system messages
- Event notifications
- Error scenarios

### Deployments
- Deployment records every 2 minutes
- Success/failure scenarios
- Rollback simulations

## Viewing the Data

### SQLite
```bash
# Open the database
sqlite3 ./dev/mock-data.db

# Example queries
SELECT COUNT(*) FROM device;
SELECT * FROM device LIMIT 5;
SELECT AVG(cpu_usage) FROM telemetry WHERE timestamp > datetime('now', '-1 hour');
```

### Web UI
```bash
# Start the platform with mock data
./bin/fleetctl start

# Open the studio
cd studio && bun run dev

# Navigate to http://localhost:3000
```

### CLI
```bash
# View devices
./bin/fleetctl devices list

# View telemetry
./bin/fleetctl telemetry get
./bin/fleetctl metrics sparkline

# View logs
./bin/fleetctl telemetry logs --follow
```

## Scenarios

Check `scenarios.yaml` for predefined scenarios:

- **Normal Operations**: Typical day-to-day patterns
- **High Load**: Testing performance limits
- **Unstable Network**: Simulating connectivity issues
- **Global Fleet**: Time zone distributed devices
- **Deployment Chaos**: Frequent updates and failures
- **Memory Leak**: Resource leak simulation
- **Production Simulation**: Realistic mixed environment

## Cleanup

```bash
# Remove SQLite database
rm -f ./dev/mock-data.db

# Clean and restart
./dev/run-mock-data.sh --clean demo
```

## Tips for Testing

1. **Performance Testing**: Use `stress` preset with 1000+ devices
2. **UI Development**: Use `demo` preset for visually appealing data
3. **Error Handling**: Modify error rates in the generator
4. **Network Issues**: Adjust `online_probability` for offline scenarios
5. **Time-based Testing**: Use longer `SEED_DAYS` for trend analysis

## Troubleshooting

- **Database locked**: Stop other processes using the database
- **Permission denied**: Make scripts executable with `chmod +x`
- **Out of memory**: Reduce `NUM_DEVICES` or `SEED_DAYS`
- **Slow generation**: Use SQLite for faster local testing

## Development

The generator is intentionally kept as a single file for easy modification. Feel free to:

- Add new device profiles
- Create custom patterns
- Modify telemetry ranges
- Add new event types
- Customize log messages

All generated files are git-ignored and safe for experimentation.