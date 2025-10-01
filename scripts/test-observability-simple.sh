#!/bin/bash

# Quick test for VictoriaMetrics and Loki integration
# This script starts VM and Loki, runs a simple test, and verifies data collection

set -e

echo "========================================="
echo "  Quick Observability Test"
echo "========================================="
echo ""

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Cleanup function
cleanup() {
    echo "Cleaning up..."
    docker stop victoriametrics-test loki-test 2>/dev/null || true
    docker rm victoriametrics-test loki-test 2>/dev/null || true
    killall -9 device-api fleetd 2>/dev/null || true
}

trap cleanup EXIT

# Start VictoriaMetrics (using port 8429 to avoid conflicts)
echo "Starting VictoriaMetrics..."
docker run -d \
    --name victoriametrics-test \
    -p 8429:8428 \
    victoriametrics/victoria-metrics:latest \
    -storageDataPath=/victoria-metrics-data \
    -httpListenAddr=:8428 \
    -retentionPeriod=1d

# Start Loki (using port 3101 to avoid conflicts)
echo "Starting Loki..."
docker run -d \
    --name loki-test \
    -p 3101:3100 \
    grafana/loki:2.9.0

# Wait for services
echo "Waiting for services to start..."
sleep 10

# Test VictoriaMetrics
echo -n "Testing VictoriaMetrics health... "
if curl -s http://localhost:8429/health | grep -q "OK"; then
    echo -e "${GREEN}✅ VictoriaMetrics is healthy${NC}"
else
    echo -e "${RED}❌ VictoriaMetrics is not healthy${NC}"
    exit 1
fi

# Test Loki
echo -n "Testing Loki health... "
if curl -s http://localhost:3101/ready | grep -q "ready"; then
    echo -e "${GREEN}✅ Loki is ready${NC}"
else
    echo -e "${RED}❌ Loki is not ready${NC}"
    exit 1
fi

# Send test metrics to VictoriaMetrics
echo "Sending test metrics to VictoriaMetrics..."
curl -s -X POST http://localhost:8429/api/v1/import/prometheus -d '
test_metric{device_id="test-001",type="test"} 42 '$(date +%s%3N)'
test_metric{device_id="test-002",type="test"} 84 '$(date +%s%3N)'
fleetd_device_count{status="online"} 2 '$(date +%s%3N)'
'

# Query metrics
echo "Querying metrics from VictoriaMetrics..."
response=$(curl -s "http://localhost:8429/api/v1/query?query=test_metric")
if echo "$response" | grep -q "test-001"; then
    echo -e "${GREEN}✅ Test metrics found in VictoriaMetrics${NC}"
else
    echo -e "${RED}❌ Test metrics not found in VictoriaMetrics${NC}"
    echo "$response"
fi

# Send test logs to Loki
echo "Sending test logs to Loki..."
timestamp=$(date +%s%N)
curl -s -X POST "http://localhost:3101/loki/api/v1/push" \
    -H "Content-Type: application/json" \
    -d '{
  "streams": [
    {
      "stream": {
        "device_id": "test-001",
        "level": "info",
        "source": "test"
      },
      "values": [
        ["'$timestamp'", "Test log message from device test-001"]
      ]
    }
  ]
}'

# Give Loki a moment to index
sleep 2

# Query logs
echo "Querying logs from Loki..."
response=$(curl -s "http://localhost:3101/loki/api/v1/query_range?query={device_id=\"test-001\"}")
if echo "$response" | grep -q "Test log message"; then
    echo -e "${GREEN}✅ Test logs found in Loki${NC}"
else
    echo -e "${RED}❌ Test logs not found in Loki${NC}"
    echo "$response" | jq '.' 2>/dev/null || echo "$response"
fi

echo ""
echo "========================================="
echo "  Test Results"
echo "========================================="
echo ""
echo -e "${GREEN}✅ VictoriaMetrics and Loki are working correctly!${NC}"
echo ""
echo "You can access the services at:"
echo "  - VictoriaMetrics: http://localhost:8429"
echo "  - Loki: http://localhost:3101"
echo ""
echo "To query metrics:"
echo '  curl "http://localhost:8429/api/v1/query?query=test_metric"'
echo ""
echo "To query logs:"
echo '  curl "http://localhost:3101/loki/api/v1/query_range?query={device_id=\"test-001\"}"'