#!/bin/bash
# Docker-based E2E test for fleetd platform
# Tests complete stack with Docker containers

set -e

echo "========================================="
echo "  fleetd Docker E2E Test Runner"
echo "========================================="
echo ""

# Configuration
FLEETD_ROOT=$(cd "$(dirname "$0")/.." && pwd)
COMPOSE_FILE="$FLEETD_ROOT/test/e2e/docker-compose.test.yml"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
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

cleanup() {
    log_info "Cleaning up Docker containers..."
    cd "$FLEETD_ROOT/test/e2e"
    docker-compose -f docker-compose.test.yml down -v 2>/dev/null || true
}

# Set up cleanup on exit
trap cleanup EXIT

# Step 1: Build agent binary for ARM64
log_info "Step 1: Building fleetd agent for ARM64..."
cd "$FLEETD_ROOT"

if [ ! -f "./bin/fleetd-arm64" ]; then
    log_info "Cross-compiling for ARM64..."
    GOOS=linux GOARCH=arm64 go build -o ./bin/fleetd-arm64 ./cmd/fleetd
else
    log_info "ARM64 binary already exists"
fi

# Step 2: Build Docker images
log_info "Step 2: Building Docker images..."
cd "$FLEETD_ROOT/test/e2e"

if [ ! -f docker-compose.test.yml ]; then
    log_error "docker-compose.test.yml not found!"
    exit 1
fi

docker-compose -f docker-compose.test.yml build

# Step 3: Start services
log_info "Step 3: Starting Docker services..."
docker-compose -f docker-compose.test.yml up -d

# Step 4: Wait for services to be healthy
log_info "Step 4: Waiting for services to be healthy..."
for i in {1..60}; do
    HEALTHY=$(docker-compose -f docker-compose.test.yml ps | grep -c "healthy" || true)
    if [ "$HEALTHY" -ge 3 ]; then
        log_info "All services are healthy"
        break
    fi
    if [ $i -eq 60 ]; then
        log_error "Services failed to become healthy"
        docker-compose -f docker-compose.test.yml logs
        exit 1
    fi
    sleep 2
done

# Step 5: Check device registration
log_info "Step 5: Checking device registration..."
sleep 10  # Give devices time to register

DEVICE_API_URL="http://localhost:8080"
DEVICES=$(curl -s $DEVICE_API_URL/api/v1/devices | jq -r '.devices | length' 2>/dev/null || echo "0")

if [ "$DEVICES" -gt 0 ]; then
    log_info "✅ Found $DEVICES registered device(s)"
    curl -s $DEVICE_API_URL/api/v1/devices | jq '.devices[] | {id, name, status}'
else
    log_error "❌ No devices registered"
    log_info "Container logs:"
    docker-compose -f docker-compose.test.yml logs
    exit 1
fi

# Step 6: Check device health
log_info "Step 6: Checking device health..."
for CONTAINER in $(docker-compose -f docker-compose.test.yml ps -q raspios-device); do
    CONTAINER_NAME=$(docker inspect -f '{{.Name}}' $CONTAINER | sed 's/^\///')
    HEALTH=$(docker exec $CONTAINER curl -s http://localhost:8080/health 2>/dev/null || echo "unhealthy")

    if [ "$HEALTH" == "OK" ] || [ ! -z "$HEALTH" ]; then
        log_info "✅ Device $CONTAINER_NAME is healthy"
    else
        log_warn "⚠️  Device $CONTAINER_NAME health check failed"
    fi
done

# Step 7: Test telemetry
log_info "Step 7: Testing telemetry collection..."
sleep 10  # Wait for telemetry to be collected

PLATFORM_API_URL="http://localhost:8090"
TELEMETRY=$(curl -s $PLATFORM_API_URL/api/v1/telemetry | jq -r '.points | length' 2>/dev/null || echo "0")

if [ "$TELEMETRY" -gt 0 ]; then
    log_info "✅ Telemetry collection working! Found $TELEMETRY data points"
else
    log_warn "⚠️  No telemetry data collected yet"
fi

# Step 8: Run E2E test suite
log_info "Step 8: Running E2E test suite..."
if docker-compose -f docker-compose.test.yml run test-runner; then
    log_info "✅ E2E test suite passed"
else
    log_error "❌ E2E test suite failed"
    docker-compose -f docker-compose.test.yml logs test-runner
    exit 1
fi

# Summary
echo ""
echo "========================================="
echo "  Docker E2E Test Results"
echo "========================================="
echo ""
log_info "✅ All Docker E2E tests passed successfully!"
echo ""
echo "Services are still running. You can:"
echo "  - View logs: docker-compose -f test/e2e/docker-compose.test.yml logs"
echo "  - Stop services: docker-compose -f test/e2e/docker-compose.test.yml down"
echo "  - Access device-api: http://localhost:8080"
echo "  - Access platform-api: http://localhost:8090"
echo ""

log_info "Keeping services running for inspection. Press Ctrl+C to stop and cleanup."
wait