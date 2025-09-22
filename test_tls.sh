#!/bin/bash

# Test script for TLS/mTLS functionality
set -e

echo "🔐 Testing fleetd TLS/mTLS Configuration"
echo "=========================================="

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Test directory for certificates
CERT_DIR="/tmp/fleetd-tls-test"
mkdir -p "$CERT_DIR"

# Function to print colored output
print_status() {
    if [ $1 -eq 0 ]; then
        echo -e "${GREEN}✓${NC} $2"
    else
        echo -e "${RED}✗${NC} $2"
    fi
}

# Function to test endpoint
test_endpoint() {
    local url=$1
    local expected_code=$2
    local desc=$3

    echo -n "Testing $desc... "

    # Use curl with insecure flag for self-signed certs
    response_code=$(curl -k -s -o /dev/null -w "%{http_code}" "$url" 2>/dev/null || echo "000")

    if [ "$response_code" = "$expected_code" ]; then
        print_status 0 "Expected $expected_code, got $response_code"
        return 0
    else
        print_status 1 "Expected $expected_code, got $response_code"
        return 1
    fi
}

# Test 1: Platform API with auto-generated TLS
echo ""
echo "Test 1: Platform API with Auto-Generated TLS"
echo "---------------------------------------------"

# Start platform-api with TLS
echo "Starting platform-api with TLS..."
FLEETD_JWT_SECRET=test-secret-key \
FLEETD_TLS_MODE=tls \
FLEETD_DB_PATH=:memory: \
./platform-api > "$CERT_DIR/platform-api.log" 2>&1 &
PLATFORM_PID=$!

# Wait for server to start
sleep 3

# Check if server started
if ps -p $PLATFORM_PID > /dev/null; then
    print_status 0 "Platform API started (PID: $PLATFORM_PID)"

    # Test HTTPS endpoint
    test_endpoint "https://localhost:8090/health" "200" "HTTPS health check"

    # Test HTTP redirect (should fail)
    test_endpoint "http://localhost:8090/health" "000" "HTTP should be disabled"
else
    print_status 1 "Platform API failed to start"
    cat "$CERT_DIR/platform-api.log"
fi

# Cleanup
kill $PLATFORM_PID 2>/dev/null || true
wait $PLATFORM_PID 2>/dev/null || true

# Test 2: Device API with auto-generated TLS
echo ""
echo "Test 2: Device API with Auto-Generated TLS"
echo "-------------------------------------------"

# Start device-api with TLS
echo "Starting device-api with TLS..."
FLEETD_SECRET_KEY=test-secret-key \
FLEETD_TLS_MODE=tls \
FLEETD_DB_PATH=:memory: \
./device-api > "$CERT_DIR/device-api.log" 2>&1 &
DEVICE_PID=$!

# Wait for server to start
sleep 3

# Check if server started
if ps -p $DEVICE_PID > /dev/null; then
    print_status 0 "Device API started (PID: $DEVICE_PID)"

    # Test HTTPS endpoint
    test_endpoint "https://localhost:8080/health" "200" "HTTPS health check"
else
    print_status 1 "Device API failed to start"
    cat "$CERT_DIR/device-api.log"
fi

# Cleanup
kill $DEVICE_PID 2>/dev/null || true
wait $DEVICE_PID 2>/dev/null || true

# Test 3: mTLS Mode
echo ""
echo "Test 3: mTLS Configuration"
echo "---------------------------"

# Start platform-api with mTLS
echo "Starting platform-api with mTLS..."
FLEETD_JWT_SECRET=test-secret-key \
FLEETD_TLS_MODE=mtls \
FLEETD_DB_PATH=:memory: \
./platform-api > "$CERT_DIR/platform-api-mtls.log" 2>&1 &
MTLS_PID=$!

# Wait for server to start
sleep 3

# Check if server started
if ps -p $MTLS_PID > /dev/null; then
    print_status 0 "Platform API with mTLS started (PID: $MTLS_PID)"

    # Test without client cert (should fail)
    response_code=$(curl -k -s -o /dev/null -w "%{http_code}" "https://localhost:8090/health" 2>/dev/null || echo "000")
    if [ "$response_code" = "000" ] || [ "$response_code" = "495" ]; then
        print_status 0 "mTLS properly rejects requests without client cert"
    else
        print_status 1 "mTLS should reject requests without client cert (got $response_code)"
    fi
else
    print_status 1 "Platform API with mTLS failed to start"
    cat "$CERT_DIR/platform-api-mtls.log"
fi

# Cleanup
kill $MTLS_PID 2>/dev/null || true
wait $MTLS_PID 2>/dev/null || true

# Test 4: Custom Certificates
echo ""
echo "Test 4: Custom Certificate Support"
echo "-----------------------------------"

# Generate test certificates
echo "Generating test certificates..."
openssl req -x509 -newkey rsa:2048 -keyout "$CERT_DIR/server.key" -out "$CERT_DIR/server.crt" \
    -days 365 -nodes -subj "/CN=localhost" 2>/dev/null

if [ -f "$CERT_DIR/server.crt" ] && [ -f "$CERT_DIR/server.key" ]; then
    print_status 0 "Test certificates generated"

    # Start with custom certificates
    echo "Starting platform-api with custom certificates..."
    FLEETD_JWT_SECRET=test-secret-key \
    FLEETD_TLS_MODE=tls \
    FLEETD_TLS_CERT="$CERT_DIR/server.crt" \
    FLEETD_TLS_KEY="$CERT_DIR/server.key" \
    FLEETD_DB_PATH=:memory: \
    ./platform-api > "$CERT_DIR/platform-api-custom.log" 2>&1 &
    CUSTOM_PID=$!

    sleep 3

    if ps -p $CUSTOM_PID > /dev/null; then
        print_status 0 "Platform API with custom certs started (PID: $CUSTOM_PID)"

        # Test HTTPS with custom cert
        test_endpoint "https://localhost:8090/health" "200" "HTTPS with custom cert"
    else
        print_status 1 "Platform API with custom certs failed to start"
        cat "$CERT_DIR/platform-api-custom.log"
    fi

    # Cleanup
    kill $CUSTOM_PID 2>/dev/null || true
    wait $CUSTOM_PID 2>/dev/null || true
else
    print_status 1 "Failed to generate test certificates"
fi

# Test 5: No TLS Mode
echo ""
echo "Test 5: TLS Disabled (HTTP only)"
echo "---------------------------------"

# Start without TLS
echo "Starting platform-api without TLS..."
FLEETD_JWT_SECRET=test-secret-key \
FLEETD_TLS_MODE=none \
FLEETD_DB_PATH=:memory: \
./platform-api > "$CERT_DIR/platform-api-notls.log" 2>&1 &
NOTLS_PID=$!

sleep 3

if ps -p $NOTLS_PID > /dev/null; then
    print_status 0 "Platform API without TLS started (PID: $NOTLS_PID)"

    # Test HTTP endpoint
    test_endpoint "http://localhost:8090/health" "200" "HTTP health check"

    # Test HTTPS (should fail)
    test_endpoint "https://localhost:8090/health" "000" "HTTPS should be disabled"
else
    print_status 1 "Platform API without TLS failed to start"
    cat "$CERT_DIR/platform-api-notls.log"
fi

# Cleanup
kill $NOTLS_PID 2>/dev/null || true
wait $NOTLS_PID 2>/dev/null || true

# Summary
echo ""
echo "=========================================="
echo "TLS Test Summary"
echo "=========================================="
echo ""
echo "Configuration Options:"
echo "  - FLEETD_TLS_MODE: none|tls|mtls (default: tls)"
echo "  - FLEETD_TLS_CERT: Path to certificate file"
echo "  - FLEETD_TLS_KEY: Path to private key file"
echo "  - FLEETD_TLS_CA: Path to CA certificate (for mTLS)"
echo ""
echo "Features:"
echo "  ✓ Auto-generates self-signed certificates if not provided"
echo "  ✓ Supports custom certificates"
echo "  ✓ mTLS for mutual authentication"
echo "  ✓ Flexible configuration via environment variables"
echo ""

# Cleanup
rm -rf "$CERT_DIR"

echo "Test complete!"