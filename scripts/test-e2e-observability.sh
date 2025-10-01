#!/bin/bash

# E2E Test with Observability Stack (VictoriaMetrics & Loki)
# This script tests the complete platform including metrics and logs collection

set -e

echo "========================================="
echo "  fleetd E2E Test with Observability"
echo "========================================="
echo ""

# Configuration
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
TEST_DIR="$PROJECT_ROOT/test/e2e"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Helper functions
log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_success() {
    echo -e "${GREEN}✅${NC} $1"
}

log_fail() {
    echo -e "${RED}❌${NC} $1"
}

# Check prerequisites
check_prerequisites() {
    log_info "Checking prerequisites..."

    if ! command -v docker &> /dev/null; then
        log_error "Docker is not installed"
        exit 1
    fi

    if ! command -v docker-compose &> /dev/null; then
        log_error "Docker Compose is not installed"
        exit 1
    fi

    log_success "All prerequisites met"
}

# Build binaries
build_binaries() {
    log_info "Building binaries..."
    cd "$PROJECT_ROOT"

    go build -o bin/device-api cmd/device-api/main.go
    go build -o bin/platform-api cmd/platform-api/main.go
    go build -o bin/fleetd cmd/fleetd/main.go

    log_success "Binaries built successfully"
}

# Start observability stack
start_observability_stack() {
    log_info "Starting observability stack..."
    cd "$TEST_DIR"

    # Stop any existing containers
    docker-compose -f docker-compose.test.yml -f docker-compose.observability.yml down -v 2>/dev/null || true

    # Start the stack
    docker-compose -f docker-compose.test.yml -f docker-compose.observability.yml up -d

    # Wait for services to be healthy
    log_info "Waiting for services to be healthy..."
    local max_attempts=30
    local attempt=0

    while [ $attempt -lt $max_attempts ]; do
        if docker-compose -f docker-compose.test.yml -f docker-compose.observability.yml ps | grep -q "unhealthy\|starting"; then
            echo -n "."
            sleep 2
            attempt=$((attempt + 1))
        else
            echo ""
            log_success "All services are healthy"
            break
        fi
    done

    if [ $attempt -eq $max_attempts ]; then
        log_error "Services failed to become healthy"
        docker-compose -f docker-compose.test.yml -f docker-compose.observability.yml ps
        exit 1
    fi
}

# Run local e2e test with metrics/logs integration
run_local_test() {
    log_info "Running local e2e test with observability..."

    # Create test directory
    TEST_RUN_DIR="/tmp/fleetd-obs-test-$$"
    mkdir -p "$TEST_RUN_DIR"

    # Start device-api with observability
    log_info "Starting device-api with metrics and logs export..."
    DEVICE_API_SECRET_KEY="test-secret" \
    VICTORIAMETRICS_URL="http://localhost:8428" \
    LOKI_URL="http://localhost:3100" \
    ENABLE_METRICS=true \
    ENABLE_LOGS_EXPORT=true \
    "$PROJECT_ROOT/bin/device-api" \
        --port=8080 \
        --db="$TEST_RUN_DIR/device-api.db" \
        --enable-mdns=false \
        > "$TEST_RUN_DIR/device-api.log" 2>&1 &
    DEVICE_API_PID=$!

    # Start platform-api
    log_info "Starting platform-api..."
    PLATFORM_API_SECRET_KEY="test-secret" \
    VICTORIAMETRICS_URL="http://localhost:8428" \
    LOKI_URL="http://localhost:3100" \
    "$PROJECT_ROOT/bin/platform-api" \
        --port=8090 \
        --db="$TEST_RUN_DIR/platform-api.db" \
        --device-api-url="http://localhost:8080" \
        > "$TEST_RUN_DIR/platform-api.log" 2>&1 &
    PLATFORM_API_PID=$!

    # Wait for services
    sleep 5

    # Start test agent
    log_info "Starting test agent with telemetry..."
    DEVICE_NAME="test-device-obs-001" \
    DEVICE_ID="obs-test-001" \
    ENABLE_TELEMETRY=true \
    TELEMETRY_INTERVAL=5 \
    SEND_LOGS=true \
    "$PROJECT_ROOT/bin/fleetd" agent \
        --server-url="http://localhost:8080" \
        --storage-dir="$TEST_RUN_DIR/agent" \
        --rpc-port=8088 \
        --disable-mdns \
        > "$TEST_RUN_DIR/agent.log" 2>&1 &
    AGENT_PID=$!

    # Wait for agent to register and send telemetry
    sleep 15

    # Cleanup local processes
    kill $DEVICE_API_PID $PLATFORM_API_PID $AGENT_PID 2>/dev/null || true

    log_success "Local test completed"
}

# Verify metrics collection
verify_metrics() {
    log_info "Verifying metrics collection..."

    # Check VictoriaMetrics for device metrics
    local vm_url="http://localhost:8428"

    # Query for device count
    echo -n "Checking device count metrics... "
    if curl -s "$vm_url/api/v1/query?query=fleetd_device_count" | grep -q '"result":\[\]'; then
        log_fail "No device count metrics found"
    else
        log_success "Device count metrics found"
    fi

    # Query for telemetry metrics
    echo -n "Checking telemetry metrics... "
    if curl -s "$vm_url/api/v1/query?query=fleetd_telemetry_received_total" | grep -q '"result":\[\]'; then
        log_warn "No telemetry metrics found (might not be implemented yet)"
    else
        log_success "Telemetry metrics found"
    fi

    # Query for API metrics
    echo -n "Checking API request metrics... "
    if curl -s "$vm_url/api/v1/query?query=http_requests_total" | grep -q '"result":\[\]'; then
        log_warn "No HTTP request metrics found"
    else
        log_success "HTTP request metrics found"
    fi

    # Show sample metrics
    log_info "Sample metrics from VictoriaMetrics:"
    curl -s "$vm_url/api/v1/label/__name__/values" | jq -r '.data[]' | head -10
}

# Verify logs collection
verify_logs() {
    log_info "Verifying logs collection..."

    # Check Loki for logs
    local loki_url="http://localhost:3100"

    # Query for device-api logs
    echo -n "Checking device-api logs... "
    if curl -s "$loki_url/loki/api/v1/query_range?query={source=\"device-api\"}&limit=1" | grep -q '"values":\[\]'; then
        log_warn "No device-api logs found in Loki"
    else
        log_success "Device-api logs found in Loki"
    fi

    # Query for device logs
    echo -n "Checking device logs... "
    if curl -s "$loki_url/loki/api/v1/query_range?query={device_id=~\".+\"}&limit=1" | grep -q '"values":\[\]'; then
        log_warn "No device logs found in Loki"
    else
        log_success "Device logs found in Loki"
    fi

    # Show log labels
    log_info "Available log labels in Loki:"
    curl -s "$loki_url/loki/api/v1/labels" | jq -r '.data[]' | head -10
}

# Run docker-compose based test
run_docker_test() {
    log_info "Running docker-compose based observability test..."
    cd "$TEST_DIR"

    # Run validation containers
    docker-compose -f docker-compose.test.yml -f docker-compose.observability.yml \
        run --rm metrics-validator

    docker-compose -f docker-compose.test.yml -f docker-compose.observability.yml \
        run --rm logs-validator
}

# Cleanup
cleanup() {
    log_info "Cleaning up..."

    # Kill local processes if running
    [ ! -z "$DEVICE_API_PID" ] && kill $DEVICE_API_PID 2>/dev/null || true
    [ ! -z "$PLATFORM_API_PID" ] && kill $PLATFORM_API_PID 2>/dev/null || true
    [ ! -z "$AGENT_PID" ] && kill $AGENT_PID 2>/dev/null || true

    # Stop docker containers if requested
    if [ "$KEEP_RUNNING" != "true" ]; then
        cd "$TEST_DIR"
        docker-compose -f docker-compose.test.yml -f docker-compose.observability.yml down -v
    fi

    # Remove test directory
    [ -d "$TEST_RUN_DIR" ] && rm -rf "$TEST_RUN_DIR"
}

# Main execution
main() {
    trap cleanup EXIT

    # Parse arguments
    MODE="${1:-local}"  # local or docker
    KEEP_RUNNING="${2:-false}"

    check_prerequisites
    build_binaries

    if [ "$MODE" = "docker" ]; then
        start_observability_stack
        run_docker_test

        # Verify from outside
        verify_metrics
        verify_logs
    else
        # Start just observability services
        log_info "Starting VictoriaMetrics and Loki..."
        cd "$TEST_DIR"
        docker-compose -f docker-compose.observability.yml up -d victoriametrics loki

        # Wait for them to be ready
        sleep 10

        run_local_test
        verify_metrics
        verify_logs
    fi

    echo ""
    echo "========================================="
    echo "  E2E Observability Test Results"
    echo "========================================="
    echo ""

    log_success "All observability tests completed!"

    if [ "$KEEP_RUNNING" = "true" ]; then
        echo ""
        log_info "Services are still running. Access them at:"
        echo "  - VictoriaMetrics: http://localhost:8428"
        echo "  - Loki: http://localhost:3100"
        echo "  - Grafana: http://localhost:3001 (if started)"
        echo ""
        echo "To stop services, run:"
        echo "  cd $TEST_DIR && docker-compose -f docker-compose.test.yml -f docker-compose.observability.yml down"
    fi
}

# Run main
main "$@"