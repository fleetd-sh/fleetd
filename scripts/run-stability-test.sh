#!/bin/bash

# fleetd 72-Hour Stability Test Runner
# This script sets up and runs the stability testing framework

set -euo pipefail

# Default values
DURATION="72h"
OUTPUT_DIR="./stability-results"
CONFIG_FILE=""
COMPONENTS="memory,cpu,goroutines,connections,database,tls,network,data_integrity"
LOG_LEVEL="info"
FLEETD_BINARY="./bin/fleetd"
STABILITY_TEST_BINARY="./test/stability/stability"
BACKGROUND_MODE=false
QUICK_TEST=false

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

usage() {
    cat << EOF
fleetd 72-Hour Stability Test Runner

Usage: $0 [OPTIONS]

This script sets up and runs comprehensive stability tests for fleetd,
including monitoring for memory leaks, connection stability, and data integrity.

OPTIONS:
    -d, --duration DURATION     Test duration (default: 72h)
    -o, --output DIR           Output directory (default: ./stability-results)
    -c, --config FILE          Configuration file path
    -C, --components LIST      Comma-separated components to test (default: all)
    -l, --log-level LEVEL      Log level: debug, info, warn, error (default: info)
    -b, --binary PATH          Path to fleetd binary (default: ./bin/fleetd)
    -B, --background           Run test in background mode
    -q, --quick                Quick test mode (1 hour duration)
    -h, --help                 Show this help message

EXAMPLES:
    # Run full 72-hour test
    $0

    # Quick 1-hour development test
    $0 --quick

    # Test specific components for 4 hours
    $0 --duration 4h --components memory,connections

    # Run in background with custom config
    $0 --config my-config.json --background

    # Generate default configuration
    $0 --generate-config

ENVIRONMENT VARIABLES:
    FLEETD_CONFIG_PATH         Path to fleetd configuration file
    FLEETD_DATABASE_PATH       Path to fleetd database file
    FLEETD_TLS_CERT_PATH       Path to TLS certificate
    FLEETD_TLS_KEY_PATH        Path to TLS private key

EOF
}

log() {
    echo -e "${BLUE}[$(date +'%Y-%m-%d %H:%M:%S')]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[$(date +'%Y-%m-%d %H:%M:%S')] ✓${NC} $1"
}

log_error() {
    echo -e "${RED}[$(date +'%Y-%m-%d %H:%M:%S')] ✗${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[$(date +'%Y-%m-%d %H:%M:%S')] ⚠${NC} $1"
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -d|--duration)
            DURATION="$2"
            shift 2
            ;;
        -o|--output)
            OUTPUT_DIR="$2"
            shift 2
            ;;
        -c|--config)
            CONFIG_FILE="$2"
            shift 2
            ;;
        -C|--components)
            COMPONENTS="$2"
            shift 2
            ;;
        -l|--log-level)
            LOG_LEVEL="$2"
            shift 2
            ;;
        -b|--binary)
            FLEETD_BINARY="$2"
            shift 2
            ;;
        -B|--background)
            BACKGROUND_MODE=true
            shift
            ;;
        -q|--quick)
            QUICK_TEST=true
            DURATION="1h"
            shift
            ;;
        --generate-config)
            log "Generating default configuration..."
            mkdir -p "$(dirname "${OUTPUT_DIR}")"
            go run ./test/stability/main.go -generate-config "${OUTPUT_DIR}/stability-config.json"
            log_success "Configuration template created at ${OUTPUT_DIR}/stability-config.json"
            log "Edit the configuration and run: $0 -c ${OUTPUT_DIR}/stability-config.json"
            exit 0
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            log_error "Unknown option: $1"
            usage
            exit 1
            ;;
    esac
done

# Quick test adjustments
if [[ "$QUICK_TEST" == "true" ]]; then
    log_warning "Quick test mode enabled - duration set to 1 hour"
    COMPONENTS="memory,connections,database"
fi

# Validate inputs
if [[ ! "$DURATION" =~ ^[0-9]+[hms]$ ]]; then
    log_error "Invalid duration format: $DURATION (use format like 72h, 30m, etc.)"
    exit 1
fi

# Create output directory
mkdir -p "$OUTPUT_DIR"
OUTPUT_DIR=$(cd "$OUTPUT_DIR" && pwd)  # Get absolute path

log "Starting fleetd stability test framework"
log "Duration: $DURATION"
log "Output directory: $OUTPUT_DIR"
log "Components: $COMPONENTS"
log "Log level: $LOG_LEVEL"

# Build stability test if needed
if [[ ! -f "$STABILITY_TEST_BINARY" ]]; then
    log "Building stability test binary..."
    go build -o "$STABILITY_TEST_BINARY" ./test/stability/main.go
    log_success "Built stability test binary"
fi

# Check if fleetd binary exists
if [[ ! -f "$FLEETD_BINARY" ]]; then
    log_error "fleetd binary not found at: $FLEETD_BINARY"
    log "Build fleetd first with: make build"
    exit 1
fi

# Setup fleetd environment
setup_fleetd_environment() {
    local fleetd_config_dir="$OUTPUT_DIR/fleetd-config"
    local fleetd_data_dir="$OUTPUT_DIR/fleetd-data"

    mkdir -p "$fleetd_config_dir" "$fleetd_data_dir"

    # Create minimal fleetd configuration
    cat > "$fleetd_config_dir/config.toml" << EOF
# fleetd configuration for stability testing

[server]
listen_address = "localhost:8080"
health_check_port = 8081

[database]
path = "$fleetd_data_dir/fleetd.db"
max_connections = 100

[logging]
level = "$LOG_LEVEL"
format = "json"
output = "$fleetd_data_dir/fleetd.log"

[tls]
enabled = false
# cert_file = "$fleetd_data_dir/cert.pem"
# key_file = "$fleetd_data_dir/key.pem"

[observability]
metrics_enabled = true
traces_enabled = false
EOF

    echo "$fleetd_config_dir/config.toml"
}

# Start fleetd for testing
start_fleetd() {
    local config_file="$1"

    log "Starting fleetd for stability testing..."

    # Start fleetd in background
    FLEETD_CONFIG="$config_file" "$FLEETD_BINARY" server \
        > "$OUTPUT_DIR/fleetd-stdout.log" 2> "$OUTPUT_DIR/fleetd-stderr.log" &

    local fleetd_pid=$!
    echo $fleetd_pid > "$OUTPUT_DIR/fleetd.pid"

    # Wait for fleetd to start
    local retries=30
    while [[ $retries -gt 0 ]]; do
        if curl -s "http://localhost:8081/health" > /dev/null 2>&1; then
            log_success "fleetd is running (PID: $fleetd_pid)"
            return 0
        fi
        sleep 1
        retries=$((retries - 1))
    done

    log_error "fleetd failed to start within 30 seconds"
    return 1
}

# Stop fleetd
stop_fleetd() {
    if [[ -f "$OUTPUT_DIR/fleetd.pid" ]]; then
        local pid=$(cat "$OUTPUT_DIR/fleetd.pid")
        if kill -0 "$pid" 2>/dev/null; then
            log "Stopping fleetd (PID: $pid)..."
            kill -TERM "$pid"

            # Wait for graceful shutdown
            local retries=10
            while [[ $retries -gt 0 ]] && kill -0 "$pid" 2>/dev/null; do
                sleep 1
                retries=$((retries - 1))
            done

            # Force kill if still running
            if kill -0 "$pid" 2>/dev/null; then
                log_warning "Force killing fleetd..."
                kill -KILL "$pid"
            fi

            log_success "fleetd stopped"
        fi
        rm -f "$OUTPUT_DIR/fleetd.pid"
    fi
}

# Cleanup function
cleanup() {
    log "Cleaning up..."
    stop_fleetd

    # Generate summary if test was running
    if [[ -f "$OUTPUT_DIR/stability.log" ]]; then
        generate_summary
    fi
}

# Set up signal handlers
trap cleanup EXIT INT TERM

# Generate test summary
generate_summary() {
    local summary_file="$OUTPUT_DIR/test-summary.txt"

    log "Generating test summary..."

    cat > "$summary_file" << EOF
fleetd 72-Hour Stability Test Summary
====================================

Test Configuration:
- Duration: $DURATION
- Components: $COMPONENTS
- Start Time: $(date)
- Output Directory: $OUTPUT_DIR

Test Status: $(if [[ -f "$OUTPUT_DIR/stability-report.json" ]]; then echo "COMPLETED"; else echo "INTERRUPTED"; fi)

Files Generated:
EOF

    # List generated files
    find "$OUTPUT_DIR" -type f -name "*.log" -o -name "*.json" -o -name "*.jsonl" | while read -r file; do
        local size=$(du -h "$file" | cut -f1)
        echo "- $(basename "$file") ($size)" >> "$summary_file"
    done

    # Add resource usage summary if available
    if [[ -f "$OUTPUT_DIR/stability-report.json" ]]; then
        echo "" >> "$summary_file"
        echo "Resource Usage Summary:" >> "$summary_file"

        if command -v jq > /dev/null; then
            jq -r '
                "- Peak Memory: \(.peak_memory_mb) MB",
                "- Average CPU: \(.average_cpu_percent)%",
                "- Max Goroutines: \(.max_goroutines)",
                "- Total Errors: \(.errors | length)"
            ' "$OUTPUT_DIR/stability-report.json" >> "$summary_file"
        fi
    fi

    log_success "Test summary written to: $summary_file"
}

# Main execution
main() {
    # Setup fleetd environment
    local fleetd_config
    fleetd_config=$(setup_fleetd_environment)

    # Update database path for stability test config
    export FLEETD_DATABASE_PATH="$OUTPUT_DIR/fleetd-data/fleetd.db"

    # Start fleetd
    if ! start_fleetd "$fleetd_config"; then
        log_error "Failed to start fleetd"
        exit 1
    fi

    # Build and run stability test
    local stability_args=(
        "-duration" "$DURATION"
        "-output" "$OUTPUT_DIR"
        "-components" "$COMPONENTS"
    )

    if [[ -n "$CONFIG_FILE" ]]; then
        stability_args+=("-config" "$CONFIG_FILE")
    fi

    if [[ "$LOG_LEVEL" == "debug" ]]; then
        stability_args+=("-verbose")
    fi

    log "Starting stability test with arguments: ${stability_args[*]}"

    if [[ "$BACKGROUND_MODE" == "true" ]]; then
        # Run in background
        log "Running stability test in background mode..."
        nohup "$STABILITY_TEST_BINARY" "${stability_args[@]}" > "$OUTPUT_DIR/stability-runner.log" 2>&1 &
        local test_pid=$!
        echo $test_pid > "$OUTPUT_DIR/stability-test.pid"

        log_success "Stability test started in background (PID: $test_pid)"
        log "Monitor progress with: tail -f $OUTPUT_DIR/stability.log"
        log "Stop test with: kill $test_pid"

        # Don't cleanup on exit in background mode
        trap - EXIT
    else
        # Run in foreground
        "$STABILITY_TEST_BINARY" "${stability_args[@]}"

        if [[ $? -eq 0 ]]; then
            log_success "Stability test completed successfully!"
        else
            log_error "Stability test failed!"
            exit 1
        fi
    fi
}

# Run main function
main "$@"