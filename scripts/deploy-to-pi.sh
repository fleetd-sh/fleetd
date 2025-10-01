#!/bin/bash
# Deploy fleetd agent to Raspberry Pi

set -e

# Configuration
PI_HOST="${PI_HOST:-raspberrypi.local}"
PI_USER="${PI_USER:-pi}"
FLEET_SERVER="${FLEET_SERVER:-http://localhost:8080}"
AGENT_VERSION="${AGENT_VERSION:-latest}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}fleetd Raspberry Pi Deployment Script${NC}"
echo "======================================="
echo ""

# Function to print status
status() {
    echo -e "${GREEN}[+]${NC} $1"
}

error() {
    echo -e "${RED}[!]${NC} $1"
    exit 1
}

warning() {
    echo -e "${YELLOW}[*]${NC} $1"
}

# Check if binary exists
if [ ! -f "bin/fleetd-arm64" ]; then
    status "Building ARM64 binary..."
    GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o bin/fleetd-arm64 ./cmd/fleetd || error "Build failed"
fi

status "Deploying to $PI_USER@$PI_HOST..."

# Copy binary to Pi
status "Copying binary..."
scp bin/fleetd-arm64 $PI_USER@$PI_HOST:/tmp/fleetd || error "Failed to copy binary"

# Create installation script
cat > /tmp/install-fleetd.sh << 'EOF'
#!/bin/bash
set -e

# Move binary to system location
sudo mv /tmp/fleetd /usr/local/bin/fleetd
sudo chmod +x /usr/local/bin/fleetd

# Create fleetd user if it doesn't exist
if ! id -u fleetd >/dev/null 2>&1; then
    echo "Creating fleetd user..."
    sudo useradd -r -s /bin/false -m -d /var/lib/fleetd fleetd
fi

# Create directories
echo "Creating directories..."
sudo mkdir -p /etc/fleetd
sudo mkdir -p /var/lib/fleetd
sudo mkdir -p /var/log/fleetd

# Set permissions
sudo chown -R fleetd:fleetd /var/lib/fleetd
sudo chown -R fleetd:fleetd /var/log/fleetd
sudo chown -R fleetd:fleetd /etc/fleetd

echo "Installation complete!"
EOF

# Copy and run installation script
status "Installing on Pi..."
scp /tmp/install-fleetd.sh $PI_USER@$PI_HOST:/tmp/
ssh $PI_USER@$PI_HOST "chmod +x /tmp/install-fleetd.sh && /tmp/install-fleetd.sh"

# Create configuration file
status "Creating configuration..."
cat > /tmp/fleetd-config.yaml << EOF
# fleetd Agent Configuration
server_url: $FLEET_SERVER
storage_dir: /var/lib/fleetd
log_level: info
telemetry_interval: 60
heartbeat_interval: 30
device_name: $(ssh $PI_USER@$PI_HOST hostname)
EOF

# Copy configuration
scp /tmp/fleetd-config.yaml $PI_USER@$PI_HOST:/tmp/
ssh $PI_USER@$PI_HOST "sudo mv /tmp/fleetd-config.yaml /etc/fleetd/config.yaml"

# Create systemd service
status "Creating systemd service..."
cat > /tmp/fleetd.service << 'EOF'
[Unit]
Description=fleetd Device Agent
Documentation=https://github.com/fleetd-sh/fleetd
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=fleetd
Group=fleetd
ExecStart=/usr/local/bin/fleetd agent --server-url=FLEET_SERVER_URL
Restart=always
RestartSec=10
StandardOutput=append:/var/log/fleetd/agent.log
StandardError=append:/var/log/fleetd/error.log

# Security settings
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/fleetd /var/log/fleetd

# Resource limits
CPUQuota=50%
MemoryLimit=100M
TasksMax=50

[Install]
WantedBy=multi-user.target
EOF

# Replace server URL in service file
sed -i "s|FLEET_SERVER_URL|$FLEET_SERVER|g" /tmp/fleetd.service

# Install and start service
scp /tmp/fleetd.service $PI_USER@$PI_HOST:/tmp/
ssh $PI_USER@$PI_HOST << 'ENDSSH'
sudo mv /tmp/fleetd.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable fleetd
sudo systemctl start fleetd
ENDSSH

# Check status
status "Checking service status..."
ssh $PI_USER@$PI_HOST "sudo systemctl status fleetd --no-pager" || warning "Service may not be running correctly"

# Display device info
status "Getting device info..."
sleep 2
ssh $PI_USER@$PI_HOST "sudo journalctl -u fleetd -n 20 --no-pager"

echo ""
echo -e "${GREEN}Deployment complete!${NC}"
echo ""
echo "Useful commands:"
echo "  SSH to Pi:           ssh $PI_USER@$PI_HOST"
echo "  Check service:       ssh $PI_USER@$PI_HOST 'sudo systemctl status fleetd'"
echo "  View logs:           ssh $PI_USER@$PI_HOST 'sudo journalctl -u fleetd -f'"
echo "  Restart service:     ssh $PI_USER@$PI_HOST 'sudo systemctl restart fleetd'"
echo "  Check device:        curl http://$PI_HOST:8080/health"
echo ""

# Test connectivity
status "Testing agent connectivity..."
if curl -s -o /dev/null -w "%{http_code}" http://$PI_HOST:8080/health | grep -q "200"; then
    echo -e "${GREEN}✓ Agent is responding on port 8080${NC}"
else
    warning "Agent may not be accessible on port 8080 yet"
fi