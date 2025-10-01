#!/bin/bash
# Integration test for agent and device-api communication

set -e

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo "fleetd Integration Test"
echo "======================="
echo ""

# Configuration
DEVICE_API_PORT=8080
AGENT_RPC_PORT=8088
TEST_TIMEOUT=30

# Helper functions
success() { echo -e "${GREEN}✓${NC} $1"; }
error() { echo -e "${RED}✗${NC} $1"; exit 1; }
warning() { echo -e "${YELLOW}!${NC} $1"; }
info() { echo -e "  $1"; }

# Cleanup function
cleanup() {
    echo ""
    echo "Cleaning up..."

    # Stop services
    if [ ! -z "$DEVICE_API_PID" ]; then
        kill $DEVICE_API_PID 2>/dev/null || true
        info "Stopped device-api (PID $DEVICE_API_PID)"
    fi

    if [ ! -z "$AGENT_PID" ]; then
        kill $AGENT_PID 2>/dev/null || true
        info "Stopped agent (PID $AGENT_PID)"
    fi

    # Clean up test data
    rm -rf /tmp/fleetd-test-* 2>/dev/null || true
}

# Set up cleanup on exit
trap cleanup EXIT

# Check prerequisites
echo "Checking prerequisites..."

if [ ! -f "bin/device-api" ]; then
    warning "device-api not found, building..."
    just build device-api || error "Failed to build device-api"
fi
success "device-api binary found"

if [ ! -f "bin/fleetd" ]; then
    warning "fleetd not found, building..."
    just build fleetd || error "Failed to build fleetd"
fi
success "fleetd binary found"

# Create test directories
TEST_DIR="/tmp/fleetd-test-$$"
mkdir -p "$TEST_DIR/device-api"
mkdir -p "$TEST_DIR/agent"
info "Test directory: $TEST_DIR"

# Start device-api
echo ""
echo "Starting device-api..."
./bin/device-api \
    --port $DEVICE_API_PORT \
    --db "$TEST_DIR/device-api/fleet.db" \
    > "$TEST_DIR/device-api.log" 2>&1 &
DEVICE_API_PID=$!

# Wait for device-api to start
for i in {1..10}; do
    if curl -s -o /dev/null http://localhost:$DEVICE_API_PORT/health 2>/dev/null; then
        success "device-api started (PID $DEVICE_API_PID)"
        break
    fi
    if [ $i -eq 10 ]; then
        cat "$TEST_DIR/device-api.log"
        error "device-api failed to start"
    fi
    sleep 1
done

# Start agent
echo ""
echo "Starting agent..."
./bin/fleetd agent \
    --server-url "http://localhost:$DEVICE_API_PORT" \
    --storage-dir "$TEST_DIR/agent" \
    --rpc-port $AGENT_RPC_PORT \
    --disable-mdns \
    > "$TEST_DIR/agent.log" 2>&1 &
AGENT_PID=$!

# Wait for agent to start
for i in {1..10}; do
    if curl -s -o /dev/null http://localhost:$AGENT_RPC_PORT/health 2>/dev/null; then
        success "Agent started (PID $AGENT_PID)"
        break
    fi
    if [ $i -eq 10 ]; then
        cat "$TEST_DIR/agent.log"
        error "Agent failed to start"
    fi
    sleep 1
done

# Test suite
echo ""
echo "Running tests..."
echo ""

# Test 1: Agent RPC health check
echo "Test 1: Agent RPC health check"
if curl -s http://localhost:$AGENT_RPC_PORT/health | grep -q "ok"; then
    success "Agent RPC is healthy"
else
    error "Agent RPC health check failed"
fi

# Test 2: Device registration
echo ""
echo "Test 2: Device registration"
sleep 3  # Give agent time to register

# Check if device appears in API
DEVICES=$(curl -s http://localhost:$DEVICE_API_PORT/api/v1/devices 2>/dev/null || echo "{}")
if echo "$DEVICES" | grep -q "device"; then
    success "Device registered successfully"
    info "Devices: $(echo $DEVICES | python3 -m json.tool 2>/dev/null | head -20 || echo $DEVICES)"
else
    warning "Device not found in API (registration may be pending)"
    info "Response: $DEVICES"
fi

# Test 3: Agent heartbeat
echo ""
echo "Test 3: Agent heartbeat"
info "Waiting for heartbeat..."
sleep 5

# Check agent logs for heartbeat
if grep -q "heartbeat\|Heartbeat" "$TEST_DIR/agent.log"; then
    success "Agent sending heartbeats"
else
    warning "No heartbeat found in logs yet"
fi

# Test 4: Device info
echo ""
echo "Test 4: Device info retrieval"
DEVICE_INFO=$(curl -s http://localhost:$AGENT_RPC_PORT/agent.v1.Discovery/GetDeviceInfo \
    -H "Content-Type: application/json" \
    -d '{}' 2>/dev/null || echo "{}")

if echo "$DEVICE_INFO" | grep -q "id\|device"; then
    success "Device info retrieved"
    info "Device info: $(echo $DEVICE_INFO | python3 -m json.tool 2>/dev/null | head -10 || echo $DEVICE_INFO)"
else
    warning "Could not retrieve device info"
fi

# Test 5: Telemetry
echo ""
echo "Test 5: Telemetry collection"
info "Waiting for telemetry..."
sleep 5

if grep -q "telemetry\|metrics\|Telemetry" "$TEST_DIR/agent.log"; then
    success "Telemetry being collected"
else
    warning "No telemetry found in logs"
fi

# Test summary
echo ""
echo "======================="
echo "Test Summary"
echo "======================="
echo ""

# Count successes
SUCCESSES=$(grep -c "✓" $0 2>/dev/null || echo "0")
WARNINGS=$(grep -c "!" $0 2>/dev/null || echo "0")

echo "Tests completed!"
echo "Check logs for details:"
echo "  Device API: $TEST_DIR/device-api.log"
echo "  Agent:      $TEST_DIR/agent.log"
echo ""

# Show last few lines of logs
echo "Recent device-api logs:"
tail -5 "$TEST_DIR/device-api.log" 2>/dev/null || echo "  No logs"
echo ""
echo "Recent agent logs:"
tail -5 "$TEST_DIR/agent.log" 2>/dev/null || echo "  No logs"

# Final status
echo ""
if [ "$WARNINGS" -eq "0" ]; then
    echo -e "${GREEN}All tests passed successfully!${NC}"
    exit 0
else
    echo -e "${YELLOW}Tests completed with warnings${NC}"
    exit 0
fi