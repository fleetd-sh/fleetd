#!/bin/bash

# CI/CD Stability Test Integration Script
# Runs abbreviated stability tests suitable for CI/CD pipelines

set -euo pipefail

# CI-specific defaults
DURATION="30m"  # Shorter duration for CI
OUTPUT_DIR="${CI_WORKSPACE:-./ci-stability-results}"
FAIL_FAST=true
COMPONENTS="memory,connections,database"
LOG_LEVEL="info"
PARALLEL_TESTS=false

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log() {
    echo -e "${BLUE}[$(date +'%Y-%m-%d %H:%M:%S')]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[$(date +'%Y-%m-%d %H:%M:%S')] ✓${NC} $1"
}

log_error() {
    echo -e "${RED}[$(date +'%Y-%m-%d %H:%M:%S')] ✗${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[$(date +'%Y-%m-%d %H:%M:%S')] ⚠${NC} $1"
}

usage() {
    cat << EOF
CI/CD Stability Test Integration

Usage: $0 [OPTIONS]

This script runs abbreviated stability tests suitable for CI/CD pipelines.
It focuses on detecting critical issues within a reasonable time frame.

OPTIONS:
    -d, --duration DURATION     Test duration (default: 30m)
    -o, --output DIR           Output directory (default: ./ci-stability-results)
    -C, --components LIST      Components to test (default: memory,connections,database)
    -l, --log-level LEVEL      Log level (default: info)
    -p, --parallel             Run multiple test scenarios in parallel
    -F, --no-fail-fast         Continue testing even after failures
    -h, --help                 Show this help

ENVIRONMENT VARIABLES:
    CI                         Set to 'true' to enable CI mode
    CI_WORKSPACE              Workspace directory for CI
    STABILITY_TEST_TIMEOUT    Maximum test timeout (default: 45m)
    FLEETD_BINARY_PATH        Path to fleetd binary

EXIT CODES:
    0: All tests passed
    1: Tests failed
    2: Configuration error
    3: Timeout exceeded

EOF
}

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -d|--duration)
            DURATION="$2"
            shift 2
            ;;
        -o|--output)
            OUTPUT_DIR="$2"
            shift 2
            ;;
        -C|--components)
            COMPONENTS="$2"
            shift 2
            ;;
        -l|--log-level)
            LOG_LEVEL="$2"
            shift 2
            ;;
        -p|--parallel)
            PARALLEL_TESTS=true
            shift
            ;;
        -F|--no-fail-fast)
            FAIL_FAST=false
            shift
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            log_error "Unknown option: $1"
            usage
            exit 2
            ;;
    esac
done

# Detect CI environment
detect_ci_environment() {
    if [[ "${CI:-}" == "true" ]] || [[ -n "${GITHUB_ACTIONS:-}" ]] || [[ -n "${GITLAB_CI:-}" ]] || [[ -n "${JENKINS_URL:-}" ]]; then
        log "CI environment detected"
        # Set CI-optimized settings
        LOG_LEVEL="info"
        FAIL_FAST=true

        # Set timeout based on CI system
        if [[ -n "${GITHUB_ACTIONS:-}" ]]; then
            export STABILITY_TEST_TIMEOUT="${STABILITY_TEST_TIMEOUT:-40m}"
        elif [[ -n "${GITLAB_CI:-}" ]]; then
            export STABILITY_TEST_TIMEOUT="${STABILITY_TEST_TIMEOUT:-35m}"
        else
            export STABILITY_TEST_TIMEOUT="${STABILITY_TEST_TIMEOUT:-30m}"
        fi
    else
        log "Local environment detected"
        export STABILITY_TEST_TIMEOUT="${STABILITY_TEST_TIMEOUT:-45m}"
    fi
}

# Create CI-optimized configuration
create_ci_config() {
    local config_file="$OUTPUT_DIR/ci-stability-config.json"

    cat > "$config_file" << EOF
{
  "duration": "$DURATION",
  "monitor_interval": "10s",
  "validation_interval": "30s",
  "metrics_interval": "15s",
  "max_memory_mb": 1024,
  "max_cpu_percent": 90.0,
  "max_goroutines": 5000,
  "max_open_files": 500,
  "max_connections": 200,
  "memory_leak_threshold": 15.0,
  "memory_leak_window": "10m",
  "performance_threshold": 30.0,
  "response_time_limit": "5s",
  "database_path": "$OUTPUT_DIR/test.db",
  "max_db_connections": 50,
  "network_timeout": "10s",
  "retry_attempts": 2,
  "output_dir": "$OUTPUT_DIR",
  "report_format": "json",
  "log_level": "$LOG_LEVEL",
  "enabled_components": [$(echo "$COMPONENTS" | sed 's/,/","/g' | sed 's/^/"/; s/$/"/')]
  "fail_on_memory_leak": $FAIL_FAST,
  "fail_on_crash": true,
  "fail_on_deadlock": true,
  "fail_on_data_corruption": true
}
EOF

    echo "$config_file"
}

# Run single stability test
run_stability_test() {
    local test_name="$1"
    local config_file="$2"
    local test_output_dir="$OUTPUT_DIR/$test_name"

    mkdir -p "$test_output_dir"

    log "Running $test_name stability test..."

    local start_time=$(date +%s)

    # Run with timeout
    timeout "${STABILITY_TEST_TIMEOUT}" \
        ./scripts/run-stability-test.sh \
        --config "$config_file" \
        --output "$test_output_dir" \
        --duration "$DURATION" \
        --log-level "$LOG_LEVEL" || local exit_code=$?

    local end_time=$(date +%s)
    local duration=$((end_time - start_time))

    if [[ ${exit_code:-0} -eq 0 ]]; then
        log_success "$test_name completed successfully in ${duration}s"
        return 0
    elif [[ ${exit_code:-0} -eq 124 ]]; then
        log_error "$test_name timed out after ${STABILITY_TEST_TIMEOUT}"
        return 3
    else
        log_error "$test_name failed with exit code ${exit_code:-1}"
        return 1
    fi
}

# Run parallel tests
run_parallel_tests() {
    local config_file="$1"

    log "Running parallel stability test scenarios..."

    # Define test scenarios
    local scenarios=(
        "memory-focused:memory,goroutines"
        "network-focused:connections,tls,network"
        "database-focused:database,data_integrity"
    )

    local pids=()
    local results=()

    # Start all tests
    for scenario in "${scenarios[@]}"; do
        local name="${scenario%%:*}"
        local components="${scenario##*:}"

        # Create scenario-specific config
        local scenario_config="$OUTPUT_DIR/${name}-config.json"
        sed "s/\"enabled_components\": \[.*\]/\"enabled_components\": [$(echo "$components" | sed 's/,/","/g' | sed 's/^/"/; s/$/"/')]/" \
            "$config_file" > "$scenario_config"

        # Run in background
        (
            run_stability_test "$name" "$scenario_config"
            echo $? > "$OUTPUT_DIR/${name}.exitcode"
        ) &

        pids+=($!)
        results+=("$name")
    done

    # Wait for all tests to complete
    local overall_result=0
    for i in "${!pids[@]}"; do
        wait "${pids[$i]}"
        local pid_result=$?
        local test_name="${results[$i]}"

        if [[ -f "$OUTPUT_DIR/${test_name}.exitcode" ]]; then
            local test_result=$(cat "$OUTPUT_DIR/${test_name}.exitcode")
            if [[ $test_result -ne 0 ]]; then
                log_error "Parallel test $test_name failed"
                overall_result=1
            fi
        else
            log_error "Parallel test $test_name did not complete properly"
            overall_result=1
        fi
    done

    return $overall_result
}

# Generate CI report
generate_ci_report() {
    local report_file="$OUTPUT_DIR/ci-stability-report.txt"

    log "Generating CI stability report..."

    cat > "$report_file" << EOF
fleetd CI Stability Test Report
================================

Test Configuration:
- Duration: $DURATION
- Components: $COMPONENTS
- Parallel Tests: $PARALLEL_TESTS
- Fail Fast: $FAIL_FAST
- Environment: $(if [[ "${CI:-}" == "true" ]]; then echo "CI"; else echo "Local"; fi)

Test Results:
EOF

    # Collect results from all test directories
    local total_tests=0
    local passed_tests=0
    local failed_tests=0

    find "$OUTPUT_DIR" -name "stability-report.json" | while read -r report; do
        local test_dir=$(dirname "$report")
        local test_name=$(basename "$test_dir")

        if command -v jq > /dev/null && [[ -f "$report" ]]; then
            local success=$(jq -r '.success' "$report")
            local errors=$(jq -r '.errors | length' "$report")
            local duration=$(jq -r '.duration' "$report")

            echo "- $test_name: $(if [[ "$success" == "true" ]]; then echo "PASSED"; else echo "FAILED"; fi) (${errors} errors, ${duration})" >> "$report_file"

            total_tests=$((total_tests + 1))
            if [[ "$success" == "true" ]]; then
                passed_tests=$((passed_tests + 1))
            else
                failed_tests=$((failed_tests + 1))
            fi
        fi
    done

    cat >> "$report_file" << EOF

Summary:
- Total Tests: $total_tests
- Passed: $passed_tests
- Failed: $failed_tests
- Success Rate: $(if [[ $total_tests -gt 0 ]]; then echo "$((passed_tests * 100 / total_tests))%"; else echo "N/A"; fi)

Generated: $(date)
EOF

    log_success "CI report generated: $report_file"

    # Display summary
    if [[ $total_tests -gt 0 ]]; then
        if [[ $failed_tests -eq 0 ]]; then
            log_success "All $total_tests tests passed!"
            return 0
        else
            log_error "$failed_tests out of $total_tests tests failed"
            return 1
        fi
    else
        log_warning "No test results found"
        return 1
    fi
}

# Main execution
main() {
    log "Starting CI/CD Stability Test"

    # Detect CI environment and adjust settings
    detect_ci_environment

    # Create output directory
    mkdir -p "$OUTPUT_DIR"
    OUTPUT_DIR=$(cd "$OUTPUT_DIR" && pwd)

    log "Output directory: $OUTPUT_DIR"
    log "Test duration: $DURATION"
    log "Components: $COMPONENTS"
    log "Parallel mode: $PARALLEL_TESTS"

    # Create CI-optimized configuration
    local config_file
    config_file=$(create_ci_config)
    log "Generated CI config: $config_file"

    # Validate configuration
    if ! go run ./test/stability/main.go -validate-config "$config_file"; then
        log_error "Configuration validation failed"
        exit 2
    fi

    # Build stability test
    log "Building stability test..."
    if ! go build -o ./test/stability/stability ./test/stability/main.go; then
        log_error "Failed to build stability test"
        exit 2
    fi

    # Run tests
    local test_result=0
    if [[ "$PARALLEL_TESTS" == "true" ]]; then
        run_parallel_tests "$config_file" || test_result=$?
    else
        run_stability_test "main" "$config_file" || test_result=$?
    fi

    # Generate CI report
    generate_ci_report || test_result=$?

    # Cleanup
    log "Cleaning up test resources..."
    pkill -f "fleetd" || true

    if [[ $test_result -eq 0 ]]; then
        log_success "CI stability test completed successfully"
    else
        log_error "CI stability test failed"
    fi

    exit $test_result
}

# Run main function
main "$@"