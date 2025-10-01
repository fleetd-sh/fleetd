#!/bin/bash

# Docker-based Load Test Script for fleetd
# This script runs load tests using Docker containers

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

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
    echo "            fleetd Load Testing - Docker Environment              "
    echo "=================================================================="
    echo ""
}

print_usage() {
    cat << EOF
Usage: $0 [OPTIONS] <command>

Commands:
  setup       - Set up Docker environment
  quick       - Run quick load test in Docker
  stress      - Run stress test with multiple containers
  clean       - Clean up Docker resources
  logs        - Show container logs
  status      - Show container status

Options:
  -d, --devices N       Number of devices per container (default: 100)
  -c, --containers N    Number of load test containers (default: 1)
  -t, --duration TIME   Test duration (default: 5m)
  -p, --port PORT       Server port (default: 8080)
  --network NETWORK     Docker network name (default: fleetd-load-test)
  --build               Force rebuild of Docker images
  --verbose             Enable verbose output
  -h, --help            Show this help message

Examples:
  $0 setup                          # Set up Docker environment
  $0 quick                          # Quick test with default settings
  $0 stress -c 5 -d 200            # Stress test with 5 containers, 200 devices each
  $0 clean                          # Clean up all Docker resources

Time format: 30s, 5m, 1h, etc.
EOF
}

# Default configuration
DEVICES=100
CONTAINERS=1
DURATION="5m"
SERVER_PORT=8080
DOCKER_NETWORK="fleetd-load-test"
FORCE_BUILD="false"
VERBOSE="false"

# Parse command line arguments
COMMAND=""

while [[ $# -gt 0 ]]; do
    case $1 in
        -d|--devices)
            DEVICES="$2"
            shift 2
            ;;
        -c|--containers)
            CONTAINERS="$2"
            shift 2
            ;;
        -t|--duration)
            DURATION="$2"
            shift 2
            ;;
        -p|--port)
            SERVER_PORT="$2"
            shift 2
            ;;
        --network)
            DOCKER_NETWORK="$2"
            shift 2
            ;;
        --build)
            FORCE_BUILD="true"
            shift
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
            if [[ -z "$COMMAND" ]]; then
                COMMAND="$1"
            else
                print_error "Multiple commands specified."
                exit 1
            fi
            shift
            ;;
    esac
done

if [[ -z "$COMMAND" ]]; then
    print_error "No command specified."
    print_usage
    exit 1
fi

# Validate command
case "$COMMAND" in
    setup|quick|stress|clean|logs|status)
        ;;
    *)
        print_error "Unknown command: $COMMAND"
        print_usage
        exit 1
        ;;
esac

print_header

# Check if Docker is installed and running
check_docker() {
    if ! command -v docker &> /dev/null; then
        print_error "Docker is not installed or not in PATH"
        exit 1
    fi

    if ! docker info &> /dev/null; then
        print_error "Docker daemon is not running"
        exit 1
    fi

    print_info "Docker is available and running"
}

# Function to create Dockerfile for load testing
create_dockerfile() {
    local dockerfile_path="$1"

    cat > "$dockerfile_path" << 'EOF'
FROM golang:1.21-alpine AS builder

# Install dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the load test application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o load_test ./test/load/scripts/run_load_test.go

# Final stage
FROM alpine:latest

# Install ca-certificates for HTTPS requests
RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Copy the binary from builder stage
COPY --from=builder /app/load_test .

# Expose dashboard port
EXPOSE 8080

# Run the load test
ENTRYPOINT ["./load_test"]
EOF

    print_info "Created Dockerfile at $dockerfile_path"
}

# Function to create Docker Compose file
create_docker_compose() {
    local compose_path="$1"

    cat > "$compose_path" << EOF
version: '3.8'

services:
  fleetd-server:
    image: fleetd:latest
    container_name: fleetd-server
    ports:
      - "${SERVER_PORT}:8080"
    networks:
      - ${DOCKER_NETWORK}
    environment:
      - FLEETD_PORT=8080
      - FLEETD_LOG_LEVEL=info
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8080/health"]
      interval: 30s
      timeout: 10s
      retries: 3

  load-test-dashboard:
    build:
      context: .
      dockerfile: Dockerfile.loadtest
    container_name: fleetd-load-dashboard
    ports:
      - "8081:8081"
    networks:
      - ${DOCKER_NETWORK}
    environment:
      - DASHBOARD_PORT=8081
    depends_on:
      - fleetd-server

networks:
  ${DOCKER_NETWORK}:
    driver: bridge
    ipam:
      config:
        - subnet: 172.20.0.0/16
EOF

    print_info "Created Docker Compose file at $compose_path"
}

# Function to build Docker images
build_images() {
    local build_context="$1"

    print_info "Building Docker images..."

    # Create Dockerfile for load testing
    create_dockerfile "$build_context/Dockerfile.loadtest"

    # Build load test image
    docker build -f "$build_context/Dockerfile.loadtest" -t fleetd-loadtest:latest "$build_context"

    print_success "Docker images built successfully"
}

# Function to setup Docker environment
setup_environment() {
    print_info "Setting up Docker environment..."

    # Find the project root
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    PROJECT_ROOT="$(cd "$SCRIPT_DIR/../../../" && pwd)"

    print_info "Project root: $PROJECT_ROOT"

    # Create Docker network if it doesn't exist
    if ! docker network ls | grep -q "$DOCKER_NETWORK"; then
        print_info "Creating Docker network: $DOCKER_NETWORK"
        docker network create "$DOCKER_NETWORK"
    else
        print_info "Docker network '$DOCKER_NETWORK' already exists"
    fi

    # Create Docker Compose file
    create_docker_compose "$PROJECT_ROOT/docker-compose.loadtest.yml"

    # Build images if they don't exist or if forced
    if [[ "$FORCE_BUILD" == "true" ]] || ! docker images | grep -q "fleetd-loadtest"; then
        build_images "$PROJECT_ROOT"
    else
        print_info "Docker images already exist (use --build to force rebuild)"
    fi

    print_success "Docker environment setup complete"
}

# Function to run quick load test
run_quick_test() {
    print_info "Running quick load test in Docker..."

    local server_url="http://fleetd-server:8080"

    docker run --rm \
        --network "$DOCKER_NETWORK" \
        --name "fleetd-loadtest-quick" \
        fleetd-loadtest:latest \
        -server="$server_url" \
        -devices=50 \
        -duration=2m \
        -dashboard=false \
        -onboarding=true \
        -steady-state=false \
        -update-campaign=false \
        -network-resilience=false \
        -verbose="$VERBOSE"

    print_success "Quick load test completed"
}

# Function to run stress test with multiple containers
run_stress_test() {
    print_info "Running stress test with $CONTAINERS containers, $DEVICES devices each..."

    local server_url="http://fleetd-server:8080"
    local pids=()

    # Start multiple load test containers
    for i in $(seq 1 "$CONTAINERS"); do
        local container_name="fleetd-loadtest-stress-$i"

        print_info "Starting container $i/$CONTAINERS: $container_name"

        docker run --rm \
            --network "$DOCKER_NETWORK" \
            --name "$container_name" \
            fleetd-loadtest:latest \
            -server="$server_url" \
            -devices="$DEVICES" \
            -duration="$DURATION" \
            -dashboard=false \
            -onboarding=true \
            -steady-state=true \
            -update-campaign=false \
            -network-resilience=false \
            -verbose="$VERBOSE" &

        pids+=($!)
    done

    print_info "All $CONTAINERS containers started. Waiting for completion..."

    # Wait for all containers to complete
    local failed=0
    for pid in "${pids[@]}"; do
        if ! wait "$pid"; then
            failed=$((failed + 1))
        fi
    done

    if [[ $failed -eq 0 ]]; then
        print_success "Stress test completed successfully"
    else
        print_error "$failed out of $CONTAINERS containers failed"
        return 1
    fi
}

# Function to show container logs
show_logs() {
    print_info "Showing container logs..."

    # Show fleetd server logs
    echo ""
    print_info "=== fleetd Server Logs ==="
    if docker ps -a | grep -q "fleetd-server"; then
        docker logs fleetd-server --tail 50
    else
        print_warning "fleetd server container not found"
    fi

    # Show load test container logs
    echo ""
    print_info "=== Load Test Container Logs ==="
    local loadtest_containers=$(docker ps -a --filter "name=fleetd-loadtest" --format "{{.Names}}")

    if [[ -n "$loadtest_containers" ]]; then
        echo "$loadtest_containers" | while read -r container; do
            echo ""
            print_info "--- $container ---"
            docker logs "$container" --tail 20
        done
    else
        print_warning "No load test containers found"
    fi
}

# Function to show container status
show_status() {
    print_info "Container status:"

    echo ""
    print_info "=== Running Containers ==="
    docker ps --filter "name=fleetd" --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"

    echo ""
    print_info "=== Docker Network ==="
    if docker network ls | grep -q "$DOCKER_NETWORK"; then
        docker network inspect "$DOCKER_NETWORK" --format "{{.Name}}: {{len .Containers}} containers"
    else
        print_warning "Network '$DOCKER_NETWORK' not found"
    fi

    echo ""
    print_info "=== Resource Usage ==="
    docker stats --no-stream --format "table {{.Container}}\t{{.CPUPerc}}\t{{.MemUsage}}\t{{.NetIO}}" $(docker ps --filter "name=fleetd" -q) 2>/dev/null || print_warning "No containers running"
}

# Function to clean up Docker resources
cleanup() {
    print_info "Cleaning up Docker resources..."

    # Stop and remove containers
    local containers=$(docker ps -a --filter "name=fleetd" -q)
    if [[ -n "$containers" ]]; then
        print_info "Stopping and removing containers..."
        docker stop $containers 2>/dev/null || true
        docker rm $containers 2>/dev/null || true
    fi

    # Remove network
    if docker network ls | grep -q "$DOCKER_NETWORK"; then
        print_info "Removing Docker network: $DOCKER_NETWORK"
        docker network rm "$DOCKER_NETWORK" 2>/dev/null || true
    fi

    # Remove images if requested
    read -p "Remove Docker images? (y/N): " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        local images=$(docker images --filter "reference=fleetd*" -q)
        if [[ -n "$images" ]]; then
            print_info "Removing Docker images..."
            docker rmi $images 2>/dev/null || true
        fi
    fi

    print_success "Cleanup completed"
}

# Function to start the server
start_server() {
    print_info "Starting fleetd server..."

    # Check if server is already running
    if docker ps | grep -q "fleetd-server"; then
        print_warning "fleetd server is already running"
        return 0
    fi

    # Start server using Docker Compose
    local compose_file="docker-compose.loadtest.yml"
    if [[ -f "$compose_file" ]]; then
        docker-compose -f "$compose_file" up -d fleetd-server

        # Wait for server to be healthy
        print_info "Waiting for server to be ready..."
        for i in {1..30}; do
            if curl -sf "http://localhost:$SERVER_PORT/health" &>/dev/null; then
                print_success "Server is ready!"
                return 0
            fi
            sleep 2
        done

        print_error "Server failed to start or is not responding"
        return 1
    else
        print_error "Docker Compose file not found. Run 'setup' first."
        return 1
    fi
}

# Main execution
check_docker

case "$COMMAND" in
    setup)
        setup_environment
        ;;

    quick)
        setup_environment
        start_server
        run_quick_test
        ;;

    stress)
        setup_environment
        start_server
        run_stress_test
        ;;

    logs)
        show_logs
        ;;

    status)
        show_status
        ;;

    clean)
        cleanup
        ;;
esac

print_info "Command '$COMMAND' completed at $(date)"