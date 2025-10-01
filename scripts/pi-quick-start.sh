#!/bin/bash
# Quick start script for testing fleetd on Raspberry Pi
# Run this directly on the Pi

set -e

echo "fleetd Quick Start for Raspberry Pi"
echo "===================================="
echo ""

# Check if running on Pi
if ! grep -q "Raspberry Pi" /proc/device-tree/model 2>/dev/null; then
    echo "Warning: This doesn't appear to be a Raspberry Pi"
    read -p "Continue anyway? (y/n) " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        exit 1
    fi
fi

# Configuration
FLEET_SERVER="${FLEET_SERVER:-http://localhost:8080}"
INSTALL_DIR="${HOME}/.local/fleetd"

# Download or use existing binary
if [ ! -f "fleetd" ]; then
    echo "Downloading fleetd binary..."
    # Replace with actual download URL when available
    DOWNLOAD_URL="${FLEETD_DOWNLOAD_URL:-https://github.com/fleetd-sh/fleetd/releases/latest/download/fleetd-linux-arm64}"

    if command -v wget >/dev/null 2>&1; then
        wget -O fleetd "$DOWNLOAD_URL"
    elif command -v curl >/dev/null 2>&1; then
        curl -L -o fleetd "$DOWNLOAD_URL"
    else
        echo "Error: Neither wget nor curl found. Please install one."
        exit 1
    fi
fi

# Make executable
chmod +x fleetd

# Create directories
echo "Creating directories..."
mkdir -p "$INSTALL_DIR/bin"
mkdir -p "$INSTALL_DIR/data"
mkdir -p "$INSTALL_DIR/logs"
mkdir -p "$INSTALL_DIR/config"

# Move binary
cp fleetd "$INSTALL_DIR/bin/"

# Create basic config
cat > "$INSTALL_DIR/config/agent.yaml" << EOF
# fleetd Agent Configuration
server_url: $FLEET_SERVER
storage_dir: $INSTALL_DIR/data
log_level: info
device_name: $(hostname)-$(date +%s)
telemetry_interval: 60
heartbeat_interval: 30
rpc_port: 8088
disable_mdns: false
EOF

# Create start script
cat > "$INSTALL_DIR/start.sh" << EOF
#!/bin/bash
cd $INSTALL_DIR
./bin/fleetd agent \\
    --server-url="$FLEET_SERVER" \\
    --storage-dir="$INSTALL_DIR/data" \\
    --rpc-port=8088 \\
    2>&1 | tee -a logs/agent.log
EOF
chmod +x "$INSTALL_DIR/start.sh"

# Create stop script
cat > "$INSTALL_DIR/stop.sh" << 'EOF'
#!/bin/bash
pkill -f "fleetd agent" || echo "Agent not running"
EOF
chmod +x "$INSTALL_DIR/stop.sh"

# Display system info
echo ""
echo "System Information:"
echo "==================="
echo "Hostname:    $(hostname)"
echo "OS:          $(uname -s) $(uname -r)"
echo "Arch:        $(uname -m)"
echo "Pi Model:    $(cat /proc/device-tree/model 2>/dev/null || echo 'Unknown')"
echo "Memory:      $(free -h | grep Mem | awk '{print $2}')"
echo "Disk:        $(df -h / | tail -1 | awk '{print $4}' ) available"
echo "CPU Temp:    $(vcgencmd measure_temp 2>/dev/null || echo 'N/A')"
echo ""

# Test run
echo "Testing agent..."
timeout 5 "$INSTALL_DIR/bin/fleetd" agent --help >/dev/null 2>&1 && echo "✓ Binary works!" || echo "✗ Binary test failed"

echo ""
echo "Installation complete!"
echo ""
echo "Quick Start Commands:"
echo "====================="
echo ""
echo "# Start agent in foreground:"
echo "  $INSTALL_DIR/start.sh"
echo ""
echo "# Start agent in background:"
echo "  nohup $INSTALL_DIR/start.sh > /dev/null 2>&1 &"
echo ""
echo "# Stop agent:"
echo "  $INSTALL_DIR/stop.sh"
echo ""
echo "# Check if running:"
echo "  ps aux | grep fleetd"
echo ""
echo "# View logs:"
echo "  tail -f $INSTALL_DIR/logs/agent.log"
echo ""
echo "# Test local RPC:"
echo "  curl http://localhost:8088/health"
echo ""

# Optional: Start agent now
read -p "Start agent now? (y/n) " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo "Starting fleetd agent..."
    "$INSTALL_DIR/start.sh" &
    AGENT_PID=$!
    sleep 3

    if kill -0 $AGENT_PID 2>/dev/null; then
        echo "✓ Agent started with PID $AGENT_PID"
        echo ""
        echo "Testing connectivity..."
        sleep 2
        if curl -s -o /dev/null -w "%{http_code}" http://localhost:8088/health | grep -q "200"; then
            echo "✓ Agent is responding!"
        else
            echo "✗ Agent not responding yet, check logs"
        fi
    else
        echo "✗ Agent failed to start, check logs at $INSTALL_DIR/logs/agent.log"
    fi
fi