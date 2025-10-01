#!/bin/bash

# Quick Load Test Script for fleetd
# This script provides easy access to common load testing scenarios

set -e

# Default configuration
DEFAULT_SERVER="http://localhost:8080"
DEFAULT_DEVICES=100
DEFAULT_DURATION="5m"
DEFAULT_OUTPUT_DIR="./load_test_results"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Function to print colored output
print_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

print_header() {
    echo ""
    echo "=================================================================="
    echo "                    fleetd Load Testing                          "
    echo "=================================================================="
    echo ""
}

print_usage() {
    cat << EOF
Usage: $0 [OPTIONS] <scenario>

Available scenarios:
  quick       - Quick smoke test (50 devices, 2 minutes)
  onboarding  - Device onboarding storm test
  steady      - Steady state performance test
  stress      - High load stress test (1000+ devices)
  resilience  - Network resilience and recovery test
  full        - Complete test suite (all scenarios)

Options:
  -s, --server URL      Server URL to test (default: $DEFAULT_SERVER)
  -d, --devices N       Number of devices to simulate (default: $DEFAULT_DEVICES)
  -t, --duration TIME   Test duration (default: $DEFAULT_DURATION)
  -o, --output DIR      Output directory (default: $DEFAULT_OUTPUT_DIR)
  -p, --port PORT       Dashboard port (default: 8081)
  --no-dashboard        Disable real-time dashboard
  --tls                 Enable TLS
  --auth-token TOKEN    Authentication token
  --verbose             Enable verbose output
  -h, --help            Show this help message

Examples:
  $0 quick                                    # Quick smoke test
  $0 stress -d 1000 -t 15m                   # Stress test with 1000 devices for 15 minutes
  $0 onboarding -s https://api.example.com   # Onboarding test against remote server
  $0 full --verbose                          # Full test suite with verbose output

Time format: 30s, 5m, 1h, etc.
EOF
}

# Parse command line arguments
SCENARIO=""
SERVER="$DEFAULT_SERVER"
DEVICES="$DEFAULT_DEVICES"
DURATION="$DEFAULT_DURATION"
OUTPUT_DIR="$DEFAULT_OUTPUT_DIR"
DASHBOARD_PORT="8081"
ENABLE_DASHBOARD="true"
TLS_ENABLED="false"
AUTH_TOKEN=""
VERBOSE="false"

while [[ $# -gt 0 ]]; do
    case $1 in
        -s|--server)
            SERVER="$2"
            shift 2
            ;;
        -d|--devices)
            DEVICES="$2"
            shift 2
            ;;
        -t|--duration)
            DURATION="$2"
            shift 2
            ;;
        -o|--output)
            OUTPUT_DIR="$2"
            shift 2
            ;;
        -p|--port)
            DASHBOARD_PORT="$2"
            shift 2
            ;;
        --no-dashboard)
            ENABLE_DASHBOARD="false"
            shift
            ;;
        --tls)
            TLS_ENABLED="true"
            shift
            ;;
        --auth-token)
            AUTH_TOKEN="$2"
            shift 2
            ;;
        --verbose)
            VERBOSE="true"
            shift
            ;;
        -h|--help)
            print_usage
            exit 0
            ;;
        -*)
            print_error "Unknown option: $1"
            print_usage
            exit 1
            ;;
        *)
            if [[ -z "$SCENARIO" ]]; then
                SCENARIO="$1"
            else
                print_error "Multiple scenarios specified. Use 'full' to run all scenarios."
                exit 1
            fi
            shift
            ;;
    esac
done

if [[ -z "$SCENARIO" ]]; then
    print_error "No scenario specified."
    print_usage
    exit 1
fi

# Validate scenario
case "$SCENARIO" in
    quick|onboarding|steady|stress|resilience|full)
        ;;
    *)
        print_error "Unknown scenario: $SCENARIO"
        print_usage
        exit 1
        ;;
esac

print_header

# Check if Go is installed
if ! command -v go &> /dev/null; then
    print_error "Go is not installed or not in PATH"
    exit 1
fi

# Ensure we're in the right directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LOAD_TEST_DIR="$(dirname "$SCRIPT_DIR")"

if [[ ! -f "$SCRIPT_DIR/run_load_test.go" ]]; then
    print_error "Load test script not found. Please run from the correct directory."
    exit 1
fi

# Create output directory
mkdir -p "$OUTPUT_DIR"

print_info "Load Test Configuration:"
echo "  Scenario: $SCENARIO"
echo "  Server: $SERVER"
echo "  Devices: $DEVICES"
echo "  Duration: $DURATION"
echo "  Output: $OUTPUT_DIR"
echo "  Dashboard: $ENABLE_DASHBOARD (port $DASHBOARD_PORT)"
echo ""

# Build common arguments
COMMON_ARGS=""
COMMON_ARGS="$COMMON_ARGS -server=\"$SERVER\""
COMMON_ARGS="$COMMON_ARGS -output=\"$OUTPUT_DIR\""
COMMON_ARGS="$COMMON_ARGS -dashboard-port=$DASHBOARD_PORT"

if [[ "$ENABLE_DASHBOARD" == "true" ]]; then
    COMMON_ARGS="$COMMON_ARGS -dashboard=true"
else
    COMMON_ARGS="$COMMON_ARGS -dashboard=false"
fi

if [[ "$TLS_ENABLED" == "true" ]]; then
    COMMON_ARGS="$COMMON_ARGS -tls=true"
fi

if [[ -n "$AUTH_TOKEN" ]]; then
    COMMON_ARGS="$COMMON_ARGS -auth-token=\"$AUTH_TOKEN\""
fi

if [[ "$VERBOSE" == "true" ]]; then
    COMMON_ARGS="$COMMON_ARGS -verbose=true"
fi

# Function to run a specific test configuration
run_test() {
    local test_name="$1"
    local test_args="$2"

    print_info "Starting $test_name..."

    if [[ "$ENABLE_DASHBOARD" == "true" ]]; then
        print_info "Dashboard will be available at: http://localhost:$DASHBOARD_PORT"
    fi

    cd "$SCRIPT_DIR"

    # Construct full command
    local cmd="go run run_load_test.go $COMMON_ARGS $test_args"

    if [[ "$VERBOSE" == "true" ]]; then
        print_info "Executing: $cmd"
    fi

    # Run the test
    if eval $cmd; then
        print_success "$test_name completed successfully!"
        return 0
    else
        print_error "$test_name failed!"
        return 1
    fi
}

# Function to wait for user input
wait_for_continue() {
    if [[ "$SCENARIO" == "full" ]]; then
        echo ""
        read -p "Press Enter to continue to the next scenario, or Ctrl+C to stop..." -r
        echo ""
    fi
}

# Execute the specified scenario
case "$SCENARIO" in
    quick)
        print_info "Running quick smoke test..."
        DEVICES=50
        DURATION="2m"
        run_test "Quick Smoke Test" "-devices=$DEVICES -duration=$DURATION -onboarding=true -steady-state=false -update-campaign=false -network-resilience=false"
        ;;

    onboarding)
        print_info "Running onboarding storm test..."
        run_test "Onboarding Storm Test" "-devices=$DEVICES -duration=$DURATION -onboarding=true -steady-state=false -update-campaign=false -network-resilience=false"
        ;;

    steady)
        print_info "Running steady state test..."
        run_test "Steady State Test" "-devices=$DEVICES -duration=$DURATION -onboarding=false -steady-state=true -update-campaign=false -network-resilience=false"
        ;;

    stress)
        print_info "Running stress test..."
        if [[ "$DEVICES" -lt 500 ]]; then
            DEVICES=1000
            print_warning "Increasing device count to 1000 for stress test"
        fi
        if [[ "$DURATION" == "$DEFAULT_DURATION" ]]; then
            DURATION="15m"
            print_warning "Extending duration to 15 minutes for stress test"
        fi
        run_test "Stress Test" "-devices=$DEVICES -duration=$DURATION -onboarding=true -steady-state=true -update-campaign=false -network-resilience=false"
        ;;

    resilience)
        print_info "Running network resilience test..."
        run_test "Network Resilience Test" "-devices=$DEVICES -duration=$DURATION -onboarding=false -steady-state=false -update-campaign=false -network-resilience=true"
        ;;

    full)
        print_info "Running complete test suite..."

        # Quick validation test
        print_info "Phase 1: Quick validation (2 minutes)"
        run_test "Quick Validation" "-devices=50 -duration=2m -onboarding=true -steady-state=false -update-campaign=false -network-resilience=false"
        wait_for_continue

        # Onboarding storm
        print_info "Phase 2: Onboarding storm test"
        run_test "Onboarding Storm" "-devices=$DEVICES -duration=$DURATION -onboarding=true -steady-state=false -update-campaign=false -network-resilience=false"
        wait_for_continue

        # Steady state
        print_info "Phase 3: Steady state test"
        run_test "Steady State" "-devices=$DEVICES -duration=$DURATION -onboarding=false -steady-state=true -update-campaign=false -network-resilience=false"
        wait_for_continue

        # Update campaign
        print_info "Phase 4: Update campaign test"
        run_test "Update Campaign" "-devices=$DEVICES -duration=$DURATION -onboarding=false -steady-state=false -update-campaign=true -network-resilience=false"
        wait_for_continue

        # Network resilience
        print_info "Phase 5: Network resilience test"
        run_test "Network Resilience" "-devices=$DEVICES -duration=$DURATION -onboarding=false -steady-state=false -update-campaign=false -network-resilience=true"

        print_success "Complete test suite finished!"
        ;;
esac

# Show results summary
echo ""
print_success "Load testing completed!"
print_info "Results saved to: $OUTPUT_DIR"

# List generated reports
if [[ -d "$OUTPUT_DIR" ]]; then
    echo ""
    print_info "Generated reports:"
    find "$OUTPUT_DIR" -name "*.html" -o -name "*.json" -o -name "*.csv" | head -10 | while read -r file; do
        echo "  - $(basename "$file")"
    done

    # Show HTML report URL if available
    HTML_REPORT=$(find "$OUTPUT_DIR" -name "*.html" | head -1)
    if [[ -n "$HTML_REPORT" ]]; then
        echo ""
        print_info "View HTML report: file://$(realpath "$HTML_REPORT")"
    fi
fi

echo ""
print_info "Test completed at $(date)"