#!/bin/bash
# Debug version of E2E test to see what's actually happening

set -e

echo "========================================="
echo "  fleetd E2E Debug Test"
echo "========================================="
echo ""

# Configuration
FLEETD_ROOT=$(cd "$(dirname "$0")/.." && pwd)
TEST_DIR="/tmp/fleetd-e2e-debug-$$"
DEVICE_API_PORT=8180
AGENT_PORT=8188

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

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
    if [ ! -z "$DEVICE_API_PID" ]; then
        kill $DEVICE_API_PID 2>/dev/null || true
    fi
    if [ ! -z "$AGENT_PID" ]; then
        kill $AGENT_PID 2>/dev/null || true
    fi
    rm -rf "$TEST_DIR"
}

trap cleanup EXIT

# Build binaries
log_info "Building binaries..."
cd "$FLEETD_ROOT"

if [ ! -f "./bin/device-api" ]; then
    go build -o ./bin/device-api ./cmd/device-api
fi

if [ ! -f "./bin/fleetd" ]; then
    go build -o ./bin/fleetd ./cmd/fleetd
fi

# Create test directory
log_info "Creating test directory at $TEST_DIR"
mkdir -p "$TEST_DIR"

# Start device-api
log_info "Starting device-api on port $DEVICE_API_PORT"
DEVICE_API_SECRET_KEY="test-secret-key" ./bin/device-api \
    --port=$DEVICE_API_PORT \
    --db="$TEST_DIR/device-api.db" \
    --enable-mdns=false \
    > "$TEST_DIR/device-api.log" 2>&1 &
DEVICE_API_PID=$!

# Wait for device-api
log_info "Waiting for device-api..."
sleep 3

# Check if device-api is running
if ! ps -p $DEVICE_API_PID > /dev/null; then
    log_error "Device API failed to start. Logs:"
    cat "$TEST_DIR/device-api.log"
    exit 1
fi

# Test device-api health
log_info "Testing device-api health endpoint..."
HEALTH_RESPONSE=$(curl -s -w "\n%{http_code}" http://localhost:$DEVICE_API_PORT/health)
HTTP_CODE=$(echo "$HEALTH_RESPONSE" | tail -n1)
BODY=$(echo "$HEALTH_RESPONSE" | head -n-1)
log_info "Health endpoint returned: HTTP $HTTP_CODE, Body: $BODY"

# Test devices endpoint
log_info "Testing devices endpoint..."
DEVICES_RESPONSE=$(curl -s -w "\n%{http_code}" http://localhost:$DEVICE_API_PORT/api/v1/devices)
HTTP_CODE=$(echo "$DEVICES_RESPONSE" | tail -n1)
BODY=$(echo "$DEVICES_RESPONSE" | head -n-1)
log_info "Devices endpoint returned: HTTP $HTTP_CODE"
log_info "Response body: $BODY"

# Start agent
log_info "Starting fleetd agent..."
DEVICE_NAME="test-device-001" \
DEVICE_ID="test-device-001" \
./bin/fleetd agent \
    --server-url="http://localhost:$DEVICE_API_PORT" \
    --storage-dir="$TEST_DIR/agent-storage" \
    --rpc-port=$AGENT_PORT \
    --disable-mdns \
    > "$TEST_DIR/agent.log" 2>&1 &
AGENT_PID=$!

# Wait for agent
log_info "Waiting for agent to start..."
sleep 5

# Check if agent is running
if ! ps -p $AGENT_PID > /dev/null; then
    log_error "Agent failed to start. Logs:"
    cat "$TEST_DIR/agent.log"
    exit 1
fi

# Test agent health
log_info "Testing agent health endpoint..."
AGENT_HEALTH=$(curl -s -w "\n%{http_code}" http://localhost:$AGENT_PORT/health)
HTTP_CODE=$(echo "$AGENT_HEALTH" | tail -n1)
BODY=$(echo "$AGENT_HEALTH" | head -n-1)
log_info "Agent health returned: HTTP $HTTP_CODE, Body: $BODY"

# Check devices again
log_info "Checking devices after agent registration..."
DEVICES_RESPONSE=$(curl -s http://localhost:$DEVICE_API_PORT/api/v1/devices)
log_info "Devices response: $DEVICES_RESPONSE"

# Check if it's JSON
if echo "$DEVICES_RESPONSE" | jq . > /dev/null 2>&1; then
    log_info "Response is valid JSON"

    # Count devices
    if echo "$DEVICES_RESPONSE" | jq -e '.devices' > /dev/null 2>&1; then
        COUNT=$(echo "$DEVICES_RESPONSE" | jq '.devices | length')
        log_info "Found $COUNT devices"
    elif echo "$DEVICES_RESPONSE" | jq -e 'type == "array"' > /dev/null 2>&1; then
        COUNT=$(echo "$DEVICES_RESPONSE" | jq '. | length')
        log_info "Found $COUNT devices (array response)"
    else
        log_warn "Unexpected JSON structure"
    fi
else
    log_error "Response is not valid JSON"
fi

# Show logs
echo ""
log_info "Device API logs (last 20 lines):"
tail -20 "$TEST_DIR/device-api.log"

echo ""
log_info "Agent logs (last 20 lines):"
tail -20 "$TEST_DIR/agent.log"

log_info "Debug test complete. Services still running for inspection."
log_info "Test directory: $TEST_DIR"
log_info "Press Ctrl+C to clean up"

wait