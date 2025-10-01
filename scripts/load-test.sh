#!/bin/bash

# Load testing script for fleetd Platform
# Uses Apache Bench (ab) for basic load testing

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
PLATFORM_API_URL="${PLATFORM_API_URL:-http://localhost:8090}"
DEVICE_API_URL="${DEVICE_API_URL:-http://localhost:8082}"
STUDIO_URL="${STUDIO_URL:-http://localhost:3000}"

# Test parameters
CONCURRENT_USERS="${CONCURRENT_USERS:-10}"
TOTAL_REQUESTS="${TOTAL_REQUESTS:-1000}"
TIMEOUT="${TIMEOUT:-30}"

echo -e "${BLUE}=== fleetd Load Testing ===${NC}"
echo -e "Platform API: ${PLATFORM_API_URL}"
echo -e "Device API: ${DEVICE_API_URL}"
echo -e "Studio: ${STUDIO_URL}"
echo -e "Concurrent Users: ${CONCURRENT_USERS}"
echo -e "Total Requests: ${TOTAL_REQUESTS}"
echo ""

# Function to check if service is available
check_service() {
    local url=$1
    local name=$2
    if curl -s -o /dev/null -w "%{http_code}" "$url/health" | grep -q "200\|503"; then
        echo -e "${GREEN}✓${NC} $name is available at $url"
        return 0
    else
        echo -e "${RED}✗${NC} $name is not available at $url"
        return 1
    fi
}

# Function to run load test with ab
run_ab_test() {
    local url=$1
    local name=$2
    local endpoint=$3

    echo -e "\n${YELLOW}Testing $name - $endpoint${NC}"
    echo "URL: $url$endpoint"
    echo "---"

    # Run Apache Bench
    ab -n $TOTAL_REQUESTS -c $CONCURRENT_USERS -t $TIMEOUT \
       -g /tmp/fleetd-${name}-gnuplot.tsv \
       "$url$endpoint" 2>&1 | grep -E "Requests per second:|Time per request:|Transfer rate:|Failed requests:|Percentage|50%|66%|75%|80%|90%|95%|98%|99%|100%"
}

# Function to test with curl in a loop
run_curl_test() {
    local url=$1
    local name=$2
    local count=$3

    echo -e "\n${YELLOW}Running $count sequential requests to $name${NC}"

    local success=0
    local fail=0
    local total_time=0

    for i in $(seq 1 $count); do
        start_time=$(date +%s%N)

        if response=$(curl -s -w "\n%{http_code}" "$url" 2>/dev/null); then
            http_code=$(echo "$response" | tail -n1)
            if [[ $http_code == "200" ]] || [[ $http_code == "503" ]]; then
                ((success++))
            else
                ((fail++))
            fi
        else
            ((fail++))
        fi

        end_time=$(date +%s%N)
        elapsed=$((($end_time - $start_time) / 1000000))
        total_time=$(($total_time + $elapsed))

        if [ $((i % 10)) -eq 0 ]; then
            echo -n "."
        fi
    done

    echo ""
    echo -e "${GREEN}Success:${NC} $success/${count}"
    echo -e "${RED}Failed:${NC} $fail/${count}"
    avg_time=$(($total_time / $count))
    echo -e "Average response time: ${avg_time}ms"
}

# Check if ab is installed
if ! command -v ab &> /dev/null; then
    echo -e "${YELLOW}Apache Bench (ab) not found. Installing...${NC}"
    if [[ "$OSTYPE" == "darwin"* ]]; then
        # macOS - ab comes with Apache
        if ! command -v brew &> /dev/null; then
            echo -e "${RED}Homebrew not found. Please install Apache manually.${NC}"
            echo "Using curl-based testing instead..."
            USE_CURL=true
        else
            brew install apache2
        fi
    elif [[ "$OSTYPE" == "linux-gnu"* ]]; then
        # Linux
        if command -v apt-get &> /dev/null; then
            sudo apt-get update && sudo apt-get install -y apache2-utils
        elif command -v yum &> /dev/null; then
            sudo yum install -y httpd-tools
        else
            echo -e "${RED}Could not install ab. Using curl-based testing.${NC}"
            USE_CURL=true
        fi
    fi
fi

# Pre-test checks
echo -e "\n${BLUE}=== Pre-test Service Checks ===${NC}"
check_service "$PLATFORM_API_URL" "Platform API" || true
check_service "$DEVICE_API_URL" "Device API" || true
check_service "$STUDIO_URL" "Studio" || true

# Wait for user confirmation
echo -e "\n${YELLOW}Ready to start load testing. Press Enter to continue or Ctrl+C to cancel...${NC}"
read

# Run tests based on available tools
if [ "$USE_CURL" = true ]; then
    echo -e "\n${BLUE}=== Running Curl-based Load Tests ===${NC}"

    # Platform API tests
    run_curl_test "$PLATFORM_API_URL/health" "Platform API Health" 100

    # Device API tests
    run_curl_test "$DEVICE_API_URL/health" "Device API Health" 100

else
    echo -e "\n${BLUE}=== Running Apache Bench Load Tests ===${NC}"

    # Test Platform API
    echo -e "\n${GREEN}1. Platform API Tests${NC}"
    run_ab_test "$PLATFORM_API_URL" "platform-api" "/health"

    # Test Device API
    echo -e "\n${GREEN}2. Device API Tests${NC}"
    run_ab_test "$DEVICE_API_URL" "device-api" "/health"

    # Test Studio (static content)
    echo -e "\n${GREEN}3. Studio Web UI Test${NC}"
    run_ab_test "$STUDIO_URL" "studio" "/"
fi

# Concurrent connection test
echo -e "\n${BLUE}=== Concurrent Connection Test ===${NC}"
echo "Testing with $CONCURRENT_USERS concurrent connections..."

for i in $(seq 1 $CONCURRENT_USERS); do
    curl -s "$PLATFORM_API_URL/health" > /dev/null &
done
wait

echo -e "${GREEN}Concurrent connection test completed${NC}"

# Check if services are still healthy after load
echo -e "\n${BLUE}=== Post-test Service Health ===${NC}"
check_service "$PLATFORM_API_URL" "Platform API" || true
check_service "$DEVICE_API_URL" "Device API" || true

# Check Docker containers
echo -e "\n${BLUE}=== Container Status ===${NC}"
docker ps --format "table {{.Names}}\t{{.Status}}\t{{.State}}" | grep fleetd || true

# Generate summary
echo -e "\n${BLUE}=== Load Test Summary ===${NC}"
echo -e "Test completed at: $(date)"
echo -e "Platform tested: fleetd"
echo -e "Concurrent users: ${CONCURRENT_USERS}"
echo -e "Total requests: ${TOTAL_REQUESTS}"

# Show any errors in logs
echo -e "\n${YELLOW}Checking for errors in logs...${NC}"
docker logs fleetd-platform-api 2>&1 | tail -5 | grep -i error || echo "No recent errors in platform-api"
docker logs fleetd-device-api 2>&1 | tail -5 | grep -i error || echo "No recent errors in device-api"

echo -e "\n${GREEN}Load testing completed!${NC}"
