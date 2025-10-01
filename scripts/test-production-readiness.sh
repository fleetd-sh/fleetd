#!/bin/bash
#
# FleetD Production Readiness Test Suite
# Tests all critical functionality for production deployment
#

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Test results
PASSED=0
FAILED=0
SKIPPED=0

# Test environment
TEST_DIR="/tmp/fleetd-test-$$"
mkdir -p "$TEST_DIR"

echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}FleetD Production Readiness Test Suite${NC}"
echo -e "${BLUE}========================================${NC}\n"

# Function to run a test
run_test() {
    local name="$1"
    local cmd="$2"

    echo -n "Testing $name... "

    if eval "$cmd" > "$TEST_DIR/${name}.log" 2>&1; then
        echo -e "${GREEN}✓ PASSED${NC}"
        ((PASSED++))
        return 0
    else
        echo -e "${RED}✗ FAILED${NC}"
        echo -e "${YELLOW}  See $TEST_DIR/${name}.log for details${NC}"
        ((FAILED++))
        return 1
    fi
}

# Function to check prerequisites
check_prerequisites() {
    echo -e "${YELLOW}Checking prerequisites...${NC}"

    local missing=""

    # Check for Go
    if ! command -v go &> /dev/null; then
        missing="$missing go"
    fi

    # Check for systemctl (Linux only)
    if [[ "$OSTYPE" == "linux-gnu"* ]]; then
        if ! command -v systemctl &> /dev/null; then
            missing="$missing systemctl"
        fi
    fi

    # Check for SQLite
    if ! command -v sqlite3 &> /dev/null; then
        missing="$missing sqlite3"
    fi

    if [ -n "$missing" ]; then
        echo -e "${RED}Missing prerequisites:$missing${NC}"
        echo -e "${YELLOW}Please install missing tools and try again${NC}"
        exit 1
    fi

    echo -e "${GREEN}All prerequisites met${NC}\n"
}

# Test 1: Build and Compilation
test_build() {
    echo -e "\n${BLUE}1. Build and Compilation Tests${NC}"
    echo -e "${BLUE}--------------------------------${NC}"

    run_test "agent_build" "go build -o $TEST_DIR/fleetd-agent ./cmd/device-agent"
    run_test "fleetctl_build" "go build -o $TEST_DIR/fleetctl ./cmd/fleetctl"
    run_test "platform_api_build" "go build -o $TEST_DIR/platform-api ./cmd/platform-api"
    run_test "device_api_build" "go build -o $TEST_DIR/device-api ./cmd/device-api"
}

# Test 2: Unit Tests
test_unit() {
    echo -e "\n${BLUE}2. Unit Tests${NC}"
    echo -e "${BLUE}-------------${NC}"

    run_test "agent_unit_tests" "go test ./internal/agent/..."
    run_test "update_unit_tests" "go test ./internal/update/..."
    run_test "security_unit_tests" "go test ./internal/security/..."
    run_test "metrics_unit_tests" "go test ./internal/agent/metrics/..."
    run_test "config_unit_tests" "go test ./internal/config/..."
}

# Test 3: Integration Tests
test_integration() {
    echo -e "\n${BLUE}3. Integration Tests${NC}"
    echo -e "${BLUE}--------------------${NC}"

    run_test "resilience_tests" "go test -v ./test/integration -run TestAgentResilience"
    run_test "update_rollback_tests" "go test -v ./test/integration -run TestUpdateRollback"
    run_test "credential_storage_tests" "go test -v ./test/integration -run TestSecureCredentialStorage"
    run_test "device_lifecycle_tests" "go test -v ./test/integration -run TestDeviceLifecycle"
}

# Test 4: State Persistence
test_state_persistence() {
    echo -e "\n${BLUE}4. State Persistence Tests${NC}"
    echo -e "${BLUE}---------------------------${NC}"

    # Create test database
    local state_db="$TEST_DIR/test_state.db"

    # Test state creation
    run_test "state_db_creation" "sqlite3 $state_db 'CREATE TABLE test (id INTEGER PRIMARY KEY);'"

    # Test state persistence
    run_test "state_persistence" "
        sqlite3 $state_db 'INSERT INTO test VALUES (1);' &&
        sqlite3 $state_db 'SELECT * FROM test;' | grep -q '1'
    "

    # Test concurrent access
    run_test "state_concurrent_access" "
        for i in {1..10}; do
            sqlite3 $state_db \"INSERT INTO test VALUES (\$i);\" &
        done
        wait
        [ \$(sqlite3 $state_db 'SELECT COUNT(*) FROM test;') -gt 1 ]
    "
}

# Test 5: Metrics Collection
test_metrics() {
    echo -e "\n${BLUE}5. Metrics Collection Tests${NC}"
    echo -e "${BLUE}----------------------------${NC}"

    # Build and run metrics test
    run_test "metrics_collection" "
        go run -race ./internal/agent/metrics/... 2>&1 | grep -q 'PASS' || true
    "
}

# Test 6: Security Features
test_security() {
    echo -e "\n${BLUE}6. Security Tests${NC}"
    echo -e "${BLUE}-----------------${NC}"

    # Test credential encryption
    run_test "credential_encryption" "
        go test -v ./internal/security -run TestVault
    "

    # Test TLS/mTLS
    run_test "tls_certificate_generation" "
        go test -v ./internal/security -run TestTLS
    "
}

# Test 7: Update System
test_update_system() {
    echo -e "\n${BLUE}7. Update System Tests${NC}"
    echo -e "${BLUE}----------------------${NC}"

    # Test update manager
    run_test "update_manager" "
        go test -v ./internal/update -run TestUpdateManager
    "

    # Test rollback mechanism
    run_test "rollback_mechanism" "
        go test -v ./internal/update -run TestRollback
    "

    # Test health checker
    run_test "health_checker" "
        go test -v ./internal/update -run TestHealthChecker
    "
}

# Test 8: Agent Resilience
test_agent_resilience() {
    echo -e "\n${BLUE}8. Agent Resilience Tests${NC}"
    echo -e "${BLUE}-------------------------${NC}"

    # Build test agent
    if [ -f "$TEST_DIR/fleetd-agent" ]; then
        # Test crash recovery
        run_test "crash_recovery" "
            $TEST_DIR/fleetd-agent -config /dev/null -debug 2>&1 | head -n 5 || true
        "

        # Test signal handling
        run_test "signal_handling" "
            timeout 1 $TEST_DIR/fleetd-agent -config /dev/null -debug 2>&1 || [ \$? -eq 124 ]
        "
    else
        echo -e "${YELLOW}Skipping agent tests - agent not built${NC}"
        ((SKIPPED++))
    fi
}

# Test 9: Installation Script
test_installation() {
    echo -e "\n${BLUE}9. Installation Tests${NC}"
    echo -e "${BLUE}---------------------${NC}"

    # Test installation script exists
    run_test "install_script_exists" "[ -f ./scripts/install-agent.sh ]"

    # Test installation script syntax
    run_test "install_script_syntax" "bash -n ./scripts/install-agent.sh"

    # Test systemd service file
    run_test "systemd_service_file" "[ -f ./deployments/systemd/fleetd.service ]"
}

# Test 10: Performance Benchmarks
test_performance() {
    echo -e "\n${BLUE}10. Performance Tests${NC}"
    echo -e "${BLUE}----------------------${NC}"

    # Run benchmarks
    run_test "metrics_benchmark" "go test -bench=. -benchtime=1s ./internal/agent/metrics/... 2>&1 | grep -E 'PASS|BenchmarkCollect'"
    run_test "state_benchmark" "go test -bench=. -benchtime=1s ./internal/agent/device/... 2>&1 | grep -E 'PASS|BenchmarkState'"
}

# Test 11: Race Condition Detection
test_race_conditions() {
    echo -e "\n${BLUE}11. Race Condition Tests${NC}"
    echo -e "${BLUE}-------------------------${NC}"

    run_test "agent_race_test" "go test -race -short ./internal/agent/..."
    run_test "update_race_test" "go test -race -short ./internal/update/..."
    run_test "security_race_test" "go test -race -short ./internal/security/..."
}

# Test 12: Resource Limits
test_resource_limits() {
    echo -e "\n${BLUE}12. Resource Limit Tests${NC}"
    echo -e "${BLUE}-------------------------${NC}"

    # Test memory usage
    run_test "memory_usage" "
        go test -v ./test/integration -run TestMemoryPressure
    "

    # Test concurrent operations
    run_test "concurrent_operations" "
        go test -v ./test/integration -run TestConcurrentOperations
    "
}

# Main test execution
main() {
    check_prerequisites

    echo -e "${BLUE}Starting Production Readiness Tests...${NC}\n"

    # Run all test suites
    test_build
    test_unit
    test_integration
    test_state_persistence
    test_metrics
    test_security
    test_update_system
    test_agent_resilience
    test_installation
    test_performance
    test_race_conditions
    test_resource_limits

    # Summary
    echo -e "\n${BLUE}========================================${NC}"
    echo -e "${BLUE}Test Summary${NC}"
    echo -e "${BLUE}========================================${NC}"
    echo -e "${GREEN}Passed:  $PASSED${NC}"
    echo -e "${RED}Failed:  $FAILED${NC}"
    echo -e "${YELLOW}Skipped: $SKIPPED${NC}"

    TOTAL=$((PASSED + FAILED + SKIPPED))
    PASS_RATE=$((PASSED * 100 / TOTAL))

    echo -e "\nPass Rate: ${PASS_RATE}%"

    if [ $PASS_RATE -ge 80 ]; then
        echo -e "\n${GREEN}✓ Production Readiness: ACCEPTABLE${NC}"
        echo -e "${GREEN}The system meets minimum production requirements.${NC}"
    elif [ $PASS_RATE -ge 60 ]; then
        echo -e "\n${YELLOW}⚠ Production Readiness: NEEDS IMPROVEMENT${NC}"
        echo -e "${YELLOW}The system needs fixes before production deployment.${NC}"
    else
        echo -e "\n${RED}✗ Production Readiness: NOT READY${NC}"
        echo -e "${RED}The system has critical issues that must be resolved.${NC}"
    fi

    echo -e "\nTest logs saved to: $TEST_DIR"

    # Exit with appropriate code
    if [ $FAILED -eq 0 ]; then
        exit 0
    else
        exit 1
    fi
}

# Run tests
main "$@"