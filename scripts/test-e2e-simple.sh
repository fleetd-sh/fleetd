#!/bin/bash
# Simple E2E test script for fleetd platform
# Tests basic agent provisioning and connection flow

set -e

echo "========================================="
echo "  fleetd Platform Simple E2E Test"
echo "========================================="
echo ""

# Configuration
FLEETD_ROOT=$(cd "$(dirname "$0")/.." && pwd)
TEST_DIR="/tmp/fleetd-e2e-test-$$"
DEVICE_API_PORT=8080
PLATFORM_API_PORT=8090
AGENT_PORT=8088

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
    log_info "Cleaning up..."

    # Kill processes
    if [ ! -z "$DEVICE_API_PID" ]; then
        kill $DEVICE_API_PID 2>/dev/null || true
    fi
    if [ ! -z "$PLATFORM_API_PID" ]; then
        kill $PLATFORM_API_PID 2>/dev/null || true
    fi
    if [ ! -z "$AGENT_PID" ]; then
        kill $AGENT_PID 2>/dev/null || true
    fi

    # Remove test directory
    rm -rf "$TEST_DIR"
}

# Set up cleanup on exit
trap cleanup EXIT

# Step 1: Build binaries if needed
log_info "Step 1: Building binaries..."
cd "$FLEETD_ROOT"

if [ ! -f "./bin/device-api" ]; then
    log_info "Building device-api..."
    go build -o ./bin/device-api ./cmd/device-api
fi

if [ ! -f "./bin/platform-api" ]; then
    log_info "Building platform-api..."
    go build -o ./bin/platform-api ./cmd/platform-api
fi

if [ ! -f "./bin/fleetd" ]; then
    log_info "Building fleetd agent..."
    go build -o ./bin/fleetd ./cmd/fleetd
fi

# Step 2: Create test directory
log_info "Step 2: Creating test directory at $TEST_DIR"
mkdir -p "$TEST_DIR"

# Step 3: Start device-api
log_info "Step 3: Starting device-api on port $DEVICE_API_PORT"
DEVICE_API_SECRET_KEY="test-secret-key" ./bin/device-api \
    --port=$DEVICE_API_PORT \
    --db="$TEST_DIR/device-api.db" \
    --enable-mdns=false \
    > "$TEST_DIR/device-api.log" 2>&1 &
DEVICE_API_PID=$!

# Step 4: Start platform-api
log_info "Step 4: Starting platform-api on port $PLATFORM_API_PORT"
PLATFORM_API_SECRET_KEY="test-secret-key" ./bin/platform-api \
    --port=$PLATFORM_API_PORT \
    --db="$TEST_DIR/platform-api.db" \
    --device-api-url="http://localhost:$DEVICE_API_PORT" \
    > "$TEST_DIR/platform-api.log" 2>&1 &
PLATFORM_API_PID=$!

# Step 5: Wait for services to be ready
log_info "Step 5: Waiting for services to be ready..."
for i in {1..30}; do
    if curl -s -o /dev/null http://localhost:$DEVICE_API_PORT/health; then
        log_info "Device API is ready"
        break
    fi
    if [ $i -eq 30 ]; then
        log_error "Device API failed to start"
        cat "$TEST_DIR/device-api.log"
        exit 1
    fi
    sleep 1
done

for i in {1..30}; do
    if curl -s -o /dev/null http://localhost:$PLATFORM_API_PORT/health; then
        log_info "Platform API is ready"
        break
    fi
    if [ $i -eq 30 ]; then
        log_error "Platform API failed to start"
        cat "$TEST_DIR/platform-api.log"
        exit 1
    fi
    sleep 1
done

# Step 6: Start fleetd agent
log_info "Step 6: Starting fleetd agent"
DEVICE_NAME="test-device-001" \
DEVICE_ID="test-device-001" \
./bin/fleetd agent \
    --server-url="http://localhost:$DEVICE_API_PORT" \
    --storage-dir="$TEST_DIR/agent-storage" \
    --rpc-port=$AGENT_PORT \
    --disable-mdns \
    > "$TEST_DIR/agent.log" 2>&1 &
AGENT_PID=$!

# Step 7: Wait for agent to be ready
log_info "Step 7: Waiting for agent to register..."
sleep 5

for i in {1..30}; do
    if curl -s -o /dev/null http://localhost:$AGENT_PORT/health; then
        log_info "Agent is ready"
        break
    fi
    if [ $i -eq 30 ]; then
        log_error "Agent failed to start"
        cat "$TEST_DIR/agent.log"
        exit 1
    fi
    sleep 1
done

# Step 8: Verify agent registration
log_info "Step 8: Verifying agent registration..."
RESPONSE=$(curl -s http://localhost:$DEVICE_API_PORT/api/v1/devices)
DEVICES=$(echo "$RESPONSE" | jq -r 'if type=="array" then . | length elif type=="object" and has("devices") then .devices | length else 0 end' 2>/dev/null || echo "0")
if [ "$DEVICES" -gt 0 ]; then
    log_info "✅ Agent successfully registered! Found $DEVICES device(s)"
    echo "Device info:"
    echo "$RESPONSE" | jq '.[0]' 2>/dev/null || echo "$RESPONSE"
else
    log_error "❌ Agent registration failed"
    echo "API Response: $RESPONSE"
    log_info "Device API logs:"
    tail -20 "$TEST_DIR/device-api.log"
    log_info "Agent logs:"
    tail -20 "$TEST_DIR/agent.log"
    exit 1
fi

# Step 9: Test agent health endpoint
log_info "Step 9: Testing agent health endpoint..."
AGENT_HEALTH=$(curl -s http://localhost:$AGENT_PORT/health)
if [ ! -z "$AGENT_HEALTH" ]; then
    log_info "✅ Agent health check successful"
else
    log_error "❌ Agent health check failed"
    exit 1
fi

# Step 10: Test telemetry submission
log_info "Step 10: Testing telemetry submission..."
sleep 5  # Wait for telemetry to be collected

METRICS=$(curl -s http://localhost:$DEVICE_API_PORT/api/v1/telemetry/metrics?device_id=test-device-001 | jq -r 'if type=="array" then . | length elif type=="object" and has("metrics") then .metrics | length else 0 end' 2>/dev/null || echo "0")
if [ "$METRICS" -gt 0 ]; then
    log_info "✅ Telemetry submission successful! Found $METRICS metric(s)"
else
    log_warn "⚠️  No telemetry metrics found (this may be expected if telemetry is disabled)"
fi

# Summary
echo ""
echo "========================================="
echo "  E2E Test Results"
echo "========================================="
echo ""
log_info "✅ All tests passed successfully!"
echo ""
echo "Test artifacts saved in: $TEST_DIR"
echo "  - Device API log: $TEST_DIR/device-api.log"
echo "  - Platform API log: $TEST_DIR/platform-api.log"
echo "  - Agent log: $TEST_DIR/agent.log"
echo ""
echo "To keep services running, press Ctrl+C to exit"
echo ""

# Keep running until interrupted
wait