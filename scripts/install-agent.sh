#!/bin/bash
#
# FleetD Agent Installation Script
# Supports: Ubuntu/Debian, CentOS/RHEL, Raspberry Pi OS
#

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
FLEETD_VERSION="${FLEETD_VERSION:-latest}"
FLEETD_SERVER="${FLEETD_SERVER:-}"
FLEETD_API_KEY="${FLEETD_API_KEY:-}"
FLEETD_USER="fleetd"
FLEETD_GROUP="fleetd"
FLEETD_HOME="/var/lib/fleetd"
FLEETD_CONFIG="/etc/fleetd"
FLEETD_LOGS="/var/log/fleetd"
FLEETD_BIN="/usr/local/bin/fleetd-agent"

# Detect OS
detect_os() {
    if [ -f /etc/os-release ]; then
        . /etc/os-release
        OS=$ID
        VERSION=$VERSION_ID
    elif [ -f /etc/debian_version ]; then
        OS="debian"
        VERSION=$(cat /etc/debian_version)
    elif [ -f /etc/redhat-release ]; then
        OS="centos"
        VERSION=$(rpm -qa \*-release | grep -Ei "oracle|redhat|centos" | cut -d"-" -f3)
    else
        echo -e "${RED}Unable to detect OS${NC}"
        exit 1
    fi

    # Detect architecture
    ARCH=$(uname -m)
    case $ARCH in
        x86_64)
            ARCH="amd64"
            ;;
        aarch64|arm64)
            ARCH="arm64"
            ;;
        armv7l|armhf)
            ARCH="arm"
            ;;
        *)
            echo -e "${RED}Unsupported architecture: $ARCH${NC}"
            exit 1
            ;;
    esac

    echo -e "${GREEN}Detected OS: $OS $VERSION ($ARCH)${NC}"
}

# Check if running as root
check_root() {
    if [ "$EUID" -ne 0 ]; then
        echo -e "${RED}Please run as root or with sudo${NC}"
        exit 1
    fi
}

# Install dependencies
install_dependencies() {
    echo -e "${YELLOW}Installing dependencies...${NC}"

    case $OS in
        ubuntu|debian|raspbian)
            apt-get update
            apt-get install -y curl wget systemd sqlite3
            ;;
        centos|rhel|fedora)
            yum install -y curl wget systemd sqlite
            ;;
        *)
            echo -e "${RED}Unsupported OS: $OS${NC}"
            exit 1
            ;;
    esac
}

# Create fleetd user and directories
create_user_and_dirs() {
    echo -e "${YELLOW}Creating fleetd user and directories...${NC}"

    # Create user if not exists
    if ! id "$FLEETD_USER" &>/dev/null; then
        useradd -r -s /bin/false -d "$FLEETD_HOME" -m "$FLEETD_USER"
    fi

    # Create directories
    mkdir -p "$FLEETD_HOME"
    mkdir -p "$FLEETD_CONFIG"
    mkdir -p "$FLEETD_LOGS"
    mkdir -p "$FLEETD_HOME/backup"
    mkdir -p "$FLEETD_HOME/cache"

    # Set permissions
    chown -R "$FLEETD_USER:$FLEETD_GROUP" "$FLEETD_HOME"
    chown -R "$FLEETD_USER:$FLEETD_GROUP" "$FLEETD_CONFIG"
    chown -R "$FLEETD_USER:$FLEETD_GROUP" "$FLEETD_LOGS"

    chmod 755 "$FLEETD_HOME"
    chmod 755 "$FLEETD_CONFIG"
    chmod 755 "$FLEETD_LOGS"
}

# Download and install binary
install_binary() {
    echo -e "${YELLOW}Downloading FleetD agent...${NC}"

    # Construct download URL
    if [ "$FLEETD_VERSION" = "latest" ]; then
        DOWNLOAD_URL="https://github.com/fleetd/fleetd/releases/latest/download/fleetd-agent-${OS}-${ARCH}"
    else
        DOWNLOAD_URL="https://github.com/fleetd/fleetd/releases/download/${FLEETD_VERSION}/fleetd-agent-${OS}-${ARCH}"
    fi

    # Download binary
    if ! wget -q -O "$FLEETD_BIN.tmp" "$DOWNLOAD_URL"; then
        echo -e "${RED}Failed to download FleetD agent${NC}"
        echo -e "${YELLOW}You can manually download from: $DOWNLOAD_URL${NC}"
        exit 1
    fi

    # Make executable and move
    chmod +x "$FLEETD_BIN.tmp"
    mv "$FLEETD_BIN.tmp" "$FLEETD_BIN"

    echo -e "${GREEN}FleetD agent installed to $FLEETD_BIN${NC}"
}

# Generate configuration
generate_config() {
    echo -e "${YELLOW}Generating configuration...${NC}"

    # Generate device ID if not provided
    if [ -z "$DEVICE_ID" ]; then
        DEVICE_ID=$(hostname)-$(date +%s)
    fi

    cat > "$FLEETD_CONFIG/agent.yaml" <<EOF
# FleetD Agent Configuration
server_url: ${FLEETD_SERVER:-http://localhost:8080}
api_key: ${FLEETD_API_KEY}
device_id: ${DEVICE_ID}

# Intervals
heartbeat_interval: 30s
update_check_interval: 5m
metrics_interval: 1m
health_check_interval: 30s

# Storage
data_dir: ${FLEETD_HOME}
log_dir: ${FLEETD_LOGS}
backup_dir: ${FLEETD_HOME}/backup

# Security
tls_verify: true

# Behavior
auto_update: true
auto_rollback: true
max_retries: 3
retry_backoff: 10s
offline_buffer_size: 1000

# Debug
log_level: info
debug: false

# Labels
labels:
  environment: production
  os: ${OS}
  arch: ${ARCH}

# Capabilities
capabilities:
  - update
  - metrics
  - logs
  - remote-exec
EOF

    chown "$FLEETD_USER:$FLEETD_GROUP" "$FLEETD_CONFIG/agent.yaml"
    chmod 600 "$FLEETD_CONFIG/agent.yaml"

    echo -e "${GREEN}Configuration generated at $FLEETD_CONFIG/agent.yaml${NC}"
}

# Install systemd service
install_service() {
    echo -e "${YELLOW}Installing systemd service...${NC}"

    cat > /etc/systemd/system/fleetd.service <<EOF
[Unit]
Description=FleetD IoT Device Management Agent
Documentation=https://github.com/fleetd/fleetd
After=network-online.target
Wants=network-online.target
StartLimitIntervalSec=0

[Service]
Type=notify
ExecStart=${FLEETD_BIN} -config ${FLEETD_CONFIG}/agent.yaml
ExecReload=/bin/kill -HUP \$MAINPID
Restart=always
RestartSec=10
TimeoutStopSec=30

# Watchdog
WatchdogSec=30
NotifyAccess=main

# Security
User=${FLEETD_USER}
Group=${FLEETD_GROUP}
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=${FLEETD_HOME} ${FLEETD_LOGS} /var/cache/fleetd
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
RestrictRealtime=true
RestrictNamespaces=true
RestrictSUIDSGID=true
LockPersonality=true
SystemCallFilter=@system-service
SystemCallErrorNumber=EPERM

# Resource limits
MemoryMax=256M
CPUQuota=50%
TasksMax=64

# Environment
Environment="FLEETD_CONFIG_PATH=${FLEETD_CONFIG}/agent.yaml"
Environment="FLEETD_DATA_DIR=${FLEETD_HOME}"
Environment="FLEETD_LOG_LEVEL=info"

[Install]
WantedBy=multi-user.target
EOF

    # Reload systemd
    systemctl daemon-reload

    echo -e "${GREEN}Systemd service installed${NC}"
}

# Configure for Raspberry Pi
configure_raspberry_pi() {
    if [ "$OS" = "raspbian" ] || [ -f /boot/config.txt ]; then
        echo -e "${YELLOW}Configuring for Raspberry Pi...${NC}"

        # Enable hardware monitoring
        if ! grep -q "dtparam=spi=on" /boot/config.txt; then
            echo "dtparam=spi=on" >> /boot/config.txt
        fi

        # Add temperature monitoring permission
        usermod -a -G video "$FLEETD_USER" 2>/dev/null || true

        # Create startup script for LED indication (optional)
        cat > /usr/local/bin/fleetd-led <<EOF
#!/bin/bash
# LED indicator for FleetD status
echo none > /sys/class/leds/led0/trigger
while true; do
    if systemctl is-active --quiet fleetd; then
        echo 1 > /sys/class/leds/led0/brightness
        sleep 2
        echo 0 > /sys/class/leds/led0/brightness
        sleep 2
    else
        echo 0 > /sys/class/leds/led0/brightness
        sleep 5
    fi
done
EOF
        chmod +x /usr/local/bin/fleetd-led

        echo -e "${GREEN}Raspberry Pi configuration applied${NC}"
    fi
}

# Start service
start_service() {
    echo -e "${YELLOW}Starting FleetD service...${NC}"

    systemctl enable fleetd
    systemctl start fleetd

    # Wait for service to start
    sleep 3

    if systemctl is-active --quiet fleetd; then
        echo -e "${GREEN}FleetD service started successfully${NC}"
        systemctl status fleetd --no-pager
    else
        echo -e "${RED}Failed to start FleetD service${NC}"
        journalctl -u fleetd --no-pager -n 20
        exit 1
    fi
}

# Print summary
print_summary() {
    echo -e "\n${GREEN}========================================${NC}"
    echo -e "${GREEN}FleetD Agent Installation Complete!${NC}"
    echo -e "${GREEN}========================================${NC}"
    echo -e "\nDevice ID: ${DEVICE_ID}"
    echo -e "Configuration: ${FLEETD_CONFIG}/agent.yaml"
    echo -e "Logs: ${FLEETD_LOGS}"
    echo -e "Data: ${FLEETD_HOME}"
    echo -e "\nUseful commands:"
    echo -e "  View status:  ${YELLOW}sudo systemctl status fleetd${NC}"
    echo -e "  View logs:    ${YELLOW}sudo journalctl -u fleetd -f${NC}"
    echo -e "  Restart:      ${YELLOW}sudo systemctl restart fleetd${NC}"
    echo -e "  Edit config:  ${YELLOW}sudo nano ${FLEETD_CONFIG}/agent.yaml${NC}"

    if [ -z "$FLEETD_SERVER" ]; then
        echo -e "\n${YELLOW}WARNING: No server URL configured!${NC}"
        echo -e "Please edit ${FLEETD_CONFIG}/agent.yaml and set the server_url"
    fi
}

# Uninstall function
uninstall() {
    echo -e "${YELLOW}Uninstalling FleetD agent...${NC}"

    # Stop and disable service
    systemctl stop fleetd 2>/dev/null || true
    systemctl disable fleetd 2>/dev/null || true

    # Remove files
    rm -f /etc/systemd/system/fleetd.service
    rm -f "$FLEETD_BIN"
    rm -f /usr/local/bin/fleetd-led

    # Optional: remove data (prompt user)
    read -p "Remove all FleetD data? (y/N) " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        rm -rf "$FLEETD_HOME"
        rm -rf "$FLEETD_CONFIG"
        rm -rf "$FLEETD_LOGS"
        userdel "$FLEETD_USER" 2>/dev/null || true
    fi

    systemctl daemon-reload

    echo -e "${GREEN}FleetD agent uninstalled${NC}"
}

# Main installation flow
main() {
    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN}FleetD Agent Installation Script${NC}"
    echo -e "${GREEN}========================================${NC}\n"

    # Check for uninstall flag
    if [ "$1" = "uninstall" ]; then
        check_root
        uninstall
        exit 0
    fi

    check_root
    detect_os
    install_dependencies
    create_user_and_dirs
    install_binary
    generate_config
    install_service
    configure_raspberry_pi
    start_service
    print_summary
}

# Run main function
main "$@"