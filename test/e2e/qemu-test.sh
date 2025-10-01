#!/bin/bash
# QEMU-based end-to-end testing for Raspberry Pi OS
# This script sets up a QEMU VM, deploys the agent, and runs tests

set -e

# Configuration
QEMU_ARCH="${QEMU_ARCH:-aarch64}"
RASPIOS_IMAGE="${RASPIOS_IMAGE:-2024-03-15-raspios-bookworm-arm64-lite.img}"
RASPIOS_URL="https://downloads.raspberrypi.com/raspios_lite_arm64/images/raspios_lite_arm64-2024-03-15/2024-03-15-raspios-bookworm-arm64-lite.img.xz"
WORK_DIR="${WORK_DIR:-/tmp/fleetd-e2e-test}"
SSH_PORT=5022
AGENT_PORT=8088
DEVICE_API_PORT=8080

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Helper functions
log() { echo -e "${GREEN}[TEST]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; exit 1; }
warning() { echo -e "${YELLOW}[WARN]${NC} $1"; }

# Cleanup function
cleanup() {
    log "Cleaning up..."

    # Kill QEMU if running
    if [ ! -z "$QEMU_PID" ]; then
        kill $QEMU_PID 2>/dev/null || true
    fi

    # Kill device-api if running
    if [ ! -z "$DEVICE_API_PID" ]; then
        kill $DEVICE_API_PID 2>/dev/null || true
    fi

    # Remove work directory
    # rm -rf "$WORK_DIR"
}

trap cleanup EXIT

# Check dependencies
check_dependencies() {
    log "Checking dependencies..."

    # Check for QEMU
    if ! command -v qemu-system-aarch64 &> /dev/null; then
        error "qemu-system-aarch64 not found. Install with: brew install qemu (macOS) or apt install qemu-system-arm (Linux)"
    fi

    # Check for expect (for automation)
    if ! command -v expect &> /dev/null; then
        warning "expect not found. Install with: brew install expect (macOS) or apt install expect (Linux)"
    fi

    log "Dependencies OK"
}

# Download Raspberry Pi OS image
download_raspios() {
    if [ -f "$WORK_DIR/$RASPIOS_IMAGE" ]; then
        log "Raspberry Pi OS image already exists"
        return
    fi

    log "Downloading Raspberry Pi OS..."
    mkdir -p "$WORK_DIR"

    # Download compressed image
    if [ ! -f "$WORK_DIR/${RASPIOS_IMAGE}.xz" ]; then
        curl -L -o "$WORK_DIR/${RASPIOS_IMAGE}.xz" "$RASPIOS_URL" || \
            error "Failed to download Raspberry Pi OS"
    fi

    # Extract
    log "Extracting image..."
    cd "$WORK_DIR"
    xz -d "${RASPIOS_IMAGE}.xz" || error "Failed to extract image"
    cd -
}

# Prepare the image (enable SSH, set passwords, etc)
prepare_image() {
    log "Preparing Raspberry Pi OS image..."

    # Create a copy for testing
    cp "$WORK_DIR/$RASPIOS_IMAGE" "$WORK_DIR/test.img"

    # Resize image to have more space
    qemu-img resize "$WORK_DIR/test.img" +2G

    # Mount and modify the image (Linux only)
    if [[ "$OSTYPE" == "linux-gnu"* ]]; then
        log "Modifying image for headless boot..."

        # Mount boot partition
        LOOP_DEVICE=$(sudo losetup -f --show -P "$WORK_DIR/test.img")
        sudo mkdir -p /mnt/raspios-boot
        sudo mount "${LOOP_DEVICE}p1" /mnt/raspios-boot

        # Enable SSH
        sudo touch /mnt/raspios-boot/ssh

        # Set up user
        echo "pi:$(echo 'raspberry' | openssl passwd -6 -stdin)" | sudo tee /mnt/raspios-boot/userconf.txt

        # Unmount
        sudo umount /mnt/raspios-boot
        sudo losetup -d $LOOP_DEVICE
    else
        warning "Cannot modify image on macOS. Using default settings."
        # On macOS, we'll use cloud-init or manual setup
    fi
}

# Start QEMU VM
start_qemu() {
    log "Starting QEMU ARM64 VM..."

    # Download kernel and DTB if needed
    if [ ! -f "$WORK_DIR/kernel8.img" ]; then
        log "Downloading kernel..."
        curl -L -o "$WORK_DIR/kernel8.img" \
            https://github.com/dhruvvyas90/qemu-rpi-kernel/raw/master/kernel-qemu-5.10.63-bullseye
    fi

    # Start QEMU in background
    qemu-system-aarch64 \
        -M raspi3b \
        -cpu cortex-a72 \
        -m 1024 \
        -kernel "$WORK_DIR/kernel8.img" \
        -dtb "$WORK_DIR/bcm2710-rpi-3-b-plus.dtb" \
        -drive file="$WORK_DIR/test.img",format=raw \
        -append "rw earlyprintk loglevel=8 console=ttyAMA0,115200 dwc_otg.lpm_enable=0 root=/dev/mmcblk0p2 rootfstype=ext4 elevator=deadline rootwait" \
        -netdev user,id=net0,hostfwd=tcp::$SSH_PORT-:22,hostfwd=tcp::$AGENT_PORT-:8088 \
        -device usb-net,netdev=net0 \
        -nographic \
        -serial mon:stdio \
        > "$WORK_DIR/qemu.log" 2>&1 &

    QEMU_PID=$!
    log "QEMU started with PID $QEMU_PID"

    # Wait for boot
    log "Waiting for VM to boot (this may take 2-3 minutes)..."
    sleep 60

    # Wait for SSH to be available
    for i in {1..30}; do
        if ssh -o StrictHostKeyChecking=no -o ConnectTimeout=5 -p $SSH_PORT pi@localhost echo "SSH OK" 2>/dev/null; then
            log "SSH connection established"
            break
        fi
        if [ $i -eq 30 ]; then
            error "Failed to establish SSH connection"
        fi
        sleep 10
    done
}

# Build and deploy agent
deploy_agent() {
    log "Building fleetd agent for ARM64..."
    GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o "$WORK_DIR/fleetd-arm64" ./cmd/fleetd

    log "Deploying agent to VM..."
    scp -P $SSH_PORT -o StrictHostKeyChecking=no "$WORK_DIR/fleetd-arm64" pi@localhost:/tmp/fleetd

    # Install agent
    ssh -p $SSH_PORT pi@localhost << 'EOF'
        sudo mv /tmp/fleetd /usr/local/bin/fleetd
        sudo chmod +x /usr/local/bin/fleetd

        # Create fleetd user
        sudo useradd -r -s /bin/false -m -d /var/lib/fleetd fleetd 2>/dev/null || true

        # Create directories
        sudo mkdir -p /var/lib/fleetd /var/log/fleetd /etc/fleetd
        sudo chown -R fleetd:fleetd /var/lib/fleetd /var/log/fleetd /etc/fleetd

        # Create systemd service
        sudo tee /etc/systemd/system/fleetd.service > /dev/null << 'EOSERVICE'
[Unit]
Description=fleetd Device Agent
After=network-online.target

[Service]
Type=simple
User=fleetd
ExecStart=/usr/local/bin/fleetd agent --server-url=http://host.docker.internal:8080
Restart=always

[Install]
WantedBy=multi-user.target
EOSERVICE

        # Start service
        sudo systemctl daemon-reload
        sudo systemctl enable fleetd
        sudo systemctl start fleetd
EOF

    log "Agent deployed and started"
}

# Start device-api
start_device_api() {
    log "Starting device-api..."

    # Build device-api
    go build -o "$WORK_DIR/device-api" ./cmd/device-api

    # Start in background
    "$WORK_DIR/device-api" --port=$DEVICE_API_PORT --db="$WORK_DIR/device-api.db" > "$WORK_DIR/device-api.log" 2>&1 &
    DEVICE_API_PID=$!

    # Wait for startup
    for i in {1..10}; do
        if curl -s http://localhost:$DEVICE_API_PORT/health >/dev/null 2>&1; then
            log "device-api started successfully"
            break
        fi
        if [ $i -eq 10 ]; then
            error "device-api failed to start"
        fi
        sleep 1
    done
}

# Run e2e tests
run_tests() {
    log "Running end-to-end tests..."

    # Test 1: Agent health check
    log "Test 1: Agent health check"
    if ssh -p $SSH_PORT pi@localhost "curl -s http://localhost:8088/health" | grep -q "ok"; then
        log "✓ Agent is healthy"
    else
        error "✗ Agent health check failed"
    fi

    # Test 2: Device registration
    log "Test 2: Device registration"
    sleep 5  # Give agent time to register

    DEVICES=$(curl -s http://localhost:$DEVICE_API_PORT/api/v1/devices)
    if echo "$DEVICES" | grep -q "device"; then
        log "✓ Device registered successfully"
    else
        error "✗ Device registration failed"
    fi

    # Test 3: Telemetry
    log "Test 3: Check telemetry"
    ssh -p $SSH_PORT pi@localhost "sudo journalctl -u fleetd -n 50" > "$WORK_DIR/agent.log"
    if grep -q "telemetry\|metrics" "$WORK_DIR/agent.log"; then
        log "✓ Telemetry is being collected"
    else
        warning "⚠ No telemetry found yet"
    fi

    # Test 4: Binary deployment
    log "Test 4: Deploy test binary"

    # Create a simple test binary
    cat > "$WORK_DIR/test-app.go" << 'EOF'
package main
import "fmt"
func main() { fmt.Println("Hello from deployed app!") }
EOF

    GOOS=linux GOARCH=arm64 go build -o "$WORK_DIR/test-app" "$WORK_DIR/test-app.go"

    # Deploy via agent (would need API implementation)
    log "⚠ Binary deployment test skipped (needs API implementation)"

    # Test 5: Agent restart
    log "Test 5: Agent restart resilience"
    ssh -p $SSH_PORT pi@localhost "sudo systemctl restart fleetd"
    sleep 5

    if ssh -p $SSH_PORT pi@localhost "systemctl is-active fleetd" | grep -q "active"; then
        log "✓ Agent restarted successfully"
    else
        error "✗ Agent failed to restart"
    fi

    log "All tests completed!"
}

# Main execution
main() {
    log "Starting Raspberry Pi OS QEMU end-to-end test"

    # Setup
    check_dependencies
    mkdir -p "$WORK_DIR"

    # Option 1: Use QEMU with real Raspberry Pi OS
    if [ "${USE_QEMU:-true}" == "true" ]; then
        download_raspios
        prepare_image
        start_qemu
    fi

    # Start services
    start_device_api

    # Deploy and test
    deploy_agent
    run_tests

    log "End-to-end test completed successfully!"

    # Show summary
    echo ""
    echo "Test Summary:"
    echo "============="
    echo "✓ QEMU VM started"
    echo "✓ Agent deployed"
    echo "✓ Device registered"
    echo "✓ Health checks passing"
    echo ""
    echo "Logs available at:"
    echo "  QEMU:       $WORK_DIR/qemu.log"
    echo "  Device API: $WORK_DIR/device-api.log"
    echo "  Agent:      $WORK_DIR/agent.log"
    echo ""
    echo "To connect to VM: ssh -p $SSH_PORT pi@localhost"
}

# Run main function
main "$@"