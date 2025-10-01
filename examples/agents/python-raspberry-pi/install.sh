#!/bin/bash
# fleetd Device Agent Installation Script for Raspberry Pi

set -e

INSTALL_DIR="/opt/fleetd"
SERVICE_FILE="/etc/systemd/system/fleetd-agent.service"
CONFIG_FILE="/etc/fleetd/agent.conf"

echo "Installing fleetd Device Agent for Raspberry Pi..."

# Check if running on Raspberry Pi
if ! grep -q "Raspberry Pi" /proc/device-tree/model 2>/dev/null; then
    echo "Warning: This doesn't appear to be a Raspberry Pi"
    read -p "Continue anyway? (y/n): " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        exit 1
    fi
fi

# Install dependencies
echo "Installing dependencies..."
sudo apt-get update
sudo apt-get install -y python3 python3-pip python3-venv

# Create installation directory
echo "Creating installation directory..."
sudo mkdir -p $INSTALL_DIR
sudo mkdir -p /etc/fleetd
sudo mkdir -p /var/log/fleetd

# Copy agent files
echo "Copying agent files..."
sudo cp fleetd_agent.py $INSTALL_DIR/
sudo cp requirements.txt $INSTALL_DIR/

# Create virtual environment and install dependencies
echo "Setting up Python environment..."
cd $INSTALL_DIR
sudo python3 -m venv venv
sudo ./venv/bin/pip install --upgrade pip
sudo ./venv/bin/pip install -r requirements.txt

# Create configuration file
echo "Creating configuration..."
if [ ! -f $CONFIG_FILE ]; then
    sudo tee $CONFIG_FILE > /dev/null <<EOF
# fleetd Device Agent Configuration
FLEET_SERVER_URL=https://devices.fleet.yourdomain.com
FLEET_ENROLLMENT_TOKEN=
FLEET_HEARTBEAT_INTERVAL=60
FLEET_TELEMETRY_INTERVAL=30
FLEET_LOG_LEVEL=INFO
EOF
    echo "Configuration file created at $CONFIG_FILE"
    echo "Please edit this file to set your fleetd server URL and enrollment token"
fi

# Create systemd service
echo "Creating systemd service..."
sudo tee $SERVICE_FILE > /dev/null <<EOF
[Unit]
Description=fleetd Device Agent
After=network.target
Wants=network-online.target

[Service]
Type=simple
User=pi
Group=pi
WorkingDirectory=$INSTALL_DIR
Environment="PATH=$INSTALL_DIR/venv/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
EnvironmentFile=$CONFIG_FILE
ExecStart=$INSTALL_DIR/venv/bin/python $INSTALL_DIR/fleetd_agent.py
Restart=always
RestartSec=10
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
EOF

# Reload systemd and enable service
echo "Enabling service..."
sudo systemctl daemon-reload
sudo systemctl enable fleetd-agent.service

echo ""
echo "Installation complete!"
echo ""
echo "Next steps:"
echo "1. Edit the configuration file: sudo nano $CONFIG_FILE"
echo "2. Set your FLEET_SERVER_URL and FLEET_ENROLLMENT_TOKEN"
echo "3. Start the agent: sudo systemctl start fleetd-agent"
echo "4. Check status: sudo systemctl status fleetd-agent"
echo "5. View logs: sudo journalctl -u fleetd-agent -f"
echo ""