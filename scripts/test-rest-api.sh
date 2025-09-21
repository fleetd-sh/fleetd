#!/bin/bash
# Test REST API endpoints via Vanguard transcoder

BASE_URL="http://localhost:8090"

echo "Testing FleetD REST API endpoints..."
echo "====================================="
echo ""

# Test health endpoint (should work without auth)
echo "1. Testing health endpoint:"
curl -X GET "$BASE_URL/health" -H "Content-Type: application/json" | jq .
echo ""

# Test listing devices (REST endpoint via Vanguard)
echo "2. Testing device list (REST):"
curl -X GET "$BASE_URL/api/v1/devices" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $JWT_TOKEN" | jq .
echo ""

# Test registering a device
echo "3. Testing device registration (REST):"
curl -X POST "$BASE_URL/api/v1/devices/register" \
  -H "Content-Type: application/json" \
  -d '{
    "device_id": "test-device-001",
    "name": "Test Device",
    "type": "edge",
    "version": "1.0.0",
    "metadata": {
      "location": "datacenter-1",
      "environment": "test"
    }
  }' | jq .
echo ""

# Test listing fleets
echo "4. Testing fleet list (REST):"
curl -X GET "$BASE_URL/api/v1/fleets" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $JWT_TOKEN" | jq .
echo ""

# Test creating a fleet
echo "5. Testing fleet creation (REST):"
curl -X POST "$BASE_URL/api/v1/fleets" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $JWT_TOKEN" \
  -d '{
    "name": "test-fleet",
    "description": "Test Fleet via REST API",
    "tags": {
      "environment": "test",
      "region": "us-west"
    }
  }' | jq .
echo ""

# Test Connect-RPC endpoint for comparison
echo "6. Testing Connect-RPC endpoint directly:"
curl -X POST "$BASE_URL/fleetd.v1.DeviceService/ListDevices" \
  -H "Content-Type: application/json" \
  -H "Connect-Protocol-Version: 1" \
  -H "Authorization: Bearer $JWT_TOKEN" \
  -d '{}' | jq .
echo ""

echo "====================================="
echo "REST API tests completed!"
echo ""
echo "Note: Set JWT_TOKEN environment variable for authenticated endpoints"
echo "Example: export JWT_TOKEN='your-jwt-token-here'"