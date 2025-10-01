#!/bin/bash

# fleetd Linux Installation Script
# Installs fleetd agent as a systemd service

set -euo pipefail

# Configuration
SERVICE_NAME="fleetd"
INSTALL_DIR="/usr/local/bin"
DATA_DIR="/var/lib/fleetd"
LOG_DIR="/var/log/fleetd"
CONFIG_DIR="/etc/fleetd"
CONFIG_FILE="${CONFIG_DIR}/agent.yaml"
EXECUTABLE_NAME="fleetd"
EXECUTABLE_PATH="${INSTALL_DIR}/${EXECUTABLE_NAME}"
SYSTEMD_SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}.service"

# GitHub repository info
GITHUB_REPO="fleetd-sh/fleetd"
BASE_URL="https://github.com/${GITHUB_REPO}/releases"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Default values
VERSION="latest"
UNINSTALL=false
SERVER_URL=""
API_KEY=""
DEVICE_ID=""

# Functions
log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1" >&2
}

log_warning() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

usage() {
    cat << EOF
Usage: $0 [OPTIONS]

Install fleetd agent on Linux

OPTIONS:
    -s, --server-url URL     Fleet server URL (required for installation)
    -k, --api-key KEY        API key for authentication (optional)
    -d, --device-id ID       Device identifier (optional, auto-generated if not provided)
    -v, --version VERSION    Version to install (default: latest)
    -u, --uninstall          Uninstall the agent
    -h, --help               Show this help message

EXAMPLES:
    $0 --server-url "https://fleet.example.com" --api-key "your-api-key"
    $0 --uninstall

EOF
}

check_root() {
    if [[ $EUID -ne 0 ]]; then
        log_error "This script must be run as root (use sudo)"
        exit 1
    fi
}

check_linux() {
    if [[ "$(uname)" != "Linux" ]]; then
        log_error "This script is for Linux only"
        exit 1
    fi
}

check_systemd() {
    if ! command -v systemctl >/dev/null 2>&1; then
        log_error "systemd is required but not found"
        exit 1
    fi
}

get_architecture() {
    local arch=$(uname -m)
    case $arch in
        x86_64)
            echo "amd64"
            ;;
        aarch64|arm64)
            echo "arm64"
            ;;
        armv7l|armv6l)
            echo "arm"
            ;;
        *)
            log_error "Unsupported architecture: $arch"
            exit 1
            ;;
    esac
}

get_latest_version() {
    log_info "Fetching latest version..."
    if command -v curl >/dev/null 2>&1; then
        curl -s "${BASE_URL}/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/' | sed 's/^v//'
    elif command -v wget >/dev/null 2>&1; then
        wget -qO- "${BASE_URL}/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/' | sed 's/^v//'
    else
        log_error "Neither curl nor wget is available"
        exit 1
    fi
}

stop_service() {
    if systemctl is-active --quiet "$SERVICE_NAME"; then
        log_info "Stopping service..."
        systemctl stop "$SERVICE_NAME"
    fi
}

uninstall_agent() {
    log_info "Uninstalling fleetd agent..."

    # Stop and disable service
    if systemctl is-enabled --quiet "$SERVICE_NAME" 2>/dev/null; then
        log_info "Disabling service..."
        systemctl disable "$SERVICE_NAME"
    fi

    stop_service

    # Remove systemd service file
    if [[ -f "$SYSTEMD_SERVICE_FILE" ]]; then
        log_info "Removing systemd service..."
        rm -f "$SYSTEMD_SERVICE_FILE"
        systemctl daemon-reload
    fi

    # Remove binary
    if [[ -f "$EXECUTABLE_PATH" ]]; then
        log_info "Removing binary..."
        rm -f "$EXECUTABLE_PATH"
    fi

    log_info "Uninstallation completed successfully!"
    log_info "Configuration and data in $CONFIG_DIR and $DATA_DIR have been preserved."
    log_info "Remove them manually if desired:"
    log_info "  sudo rm -rf $CONFIG_DIR $DATA_DIR $LOG_DIR"
}

download_agent() {
    local version="$1"
    local arch=$(get_architecture)

    if [[ "$version" == "latest" ]]; then
        version=$(get_latest_version)
        if [[ -z "$version" ]]; then
            log_error "Could not determine latest version"
            exit 1
        fi
    fi

    local download_url="${BASE_URL}/download/v${version}/fleetd-linux-${arch}"
    local temp_file="/tmp/fleetd-${version}"

    log_info "Downloading fleetd v${version} for ${arch}..."
    log_info "URL: $download_url"

    if command -v curl >/dev/null 2>&1; then
        curl -L -o "$temp_file" "$download_url"
    elif command -v wget >/dev/null 2>&1; then
        wget -O "$temp_file" "$download_url"
    else
        log_error "Neither curl nor wget is available"
        exit 1
    fi

    if [[ ! -f "$temp_file" ]]; then
        log_error "Download failed"
        exit 1
    fi

    echo "$temp_file"
}

create_directories() {
    log_info "Creating directories..."
    mkdir -p "$DATA_DIR" "$LOG_DIR" "$CONFIG_DIR"

    # Set proper ownership and permissions
    chown root:root "$DATA_DIR" "$LOG_DIR" "$CONFIG_DIR"
    chmod 755 "$DATA_DIR" "$LOG_DIR" "$CONFIG_DIR"
}

install_binary() {
    local temp_file="$1"

    log_info "Installing binary to $EXECUTABLE_PATH..."
    cp "$temp_file" "$EXECUTABLE_PATH"
    chmod 755 "$EXECUTABLE_PATH"
    chown root:root "$EXECUTABLE_PATH"

    # Remove temporary file
    rm -f "$temp_file"
}

create_config() {
    log_info "Creating configuration file..."

    if [[ -z "$DEVICE_ID" ]]; then
        DEVICE_ID="$(hostname)-$(date +%Y%m%d-%H%M%S)"
    fi

    cat > "$CONFIG_FILE" << EOF
server_url: "$SERVER_URL"
device_id: "$DEVICE_ID"
data_dir: "$DATA_DIR"
log_dir: "$LOG_DIR"
heartbeat_interval: 30s
update_check_interval: 5m
metrics_interval: 1m
auto_update: true
auto_rollback: true
debug: false
log_level: info
EOF

    if [[ -n "$API_KEY" ]]; then
        echo "api_key: \"$API_KEY\"" >> "$CONFIG_FILE"
    fi

    # Set proper permissions (readable by root only)
    chmod 600 "$CONFIG_FILE"
    chown root:root "$CONFIG_FILE"
}

install_service() {
    log_info "Installing systemd service..."

    "$EXECUTABLE_PATH" service install --config "$CONFIG_FILE"

    if [[ -f "$SYSTEMD_SERVICE_FILE" ]]; then
        log_info "Reloading systemd configuration..."
        systemctl daemon-reload

        log_info "Enabling service..."
        systemctl enable "$SERVICE_NAME"

        log_info "Starting service..."
        systemctl start "$SERVICE_NAME"

        # Give the service a moment to start
        sleep 3

        log_info "Installation completed successfully!"
        echo
        log_info "Service Status:"
        systemctl status "$SERVICE_NAME" --no-pager --lines=0

        echo
        log_info "Configuration file: $CONFIG_FILE"
        log_info "Data directory: $DATA_DIR"
        log_info "Log directory: $LOG_DIR"
        echo
        log_info "To check logs: journalctl -u $SERVICE_NAME -f"
        log_info "To check status: systemctl status $SERVICE_NAME"
        log_info "To restart: systemctl restart $SERVICE_NAME"
        log_info "To uninstall: $0 --uninstall"
    else
        log_error "Failed to create systemd service"
        exit 1
    fi
}

detect_init_system() {
    if [[ -d /run/systemd/system ]]; then
        echo "systemd"
    elif command -v service >/dev/null 2>&1; then
        echo "sysv"
    else
        echo "unknown"
    fi
}

install_agent() {
    if [[ -z "$SERVER_URL" ]]; then
        log_error "Server URL is required for installation"
        echo
        usage
        exit 1
    fi

    # Check init system
    local init_system=$(detect_init_system)
    if [[ "$init_system" != "systemd" ]]; then
        log_warning "systemd not detected. This script is designed for systemd-based systems."
        log_warning "Detected init system: $init_system"
        read -p "Continue anyway? (y/N): " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            log_info "Installation cancelled."
            exit 0
        fi
    fi

    log_info "Installing fleetd agent..."

    # Stop existing service if running
    stop_service

    # Create directories
    create_directories

    # Download and install binary
    local temp_file=$(download_agent "$VERSION")
    install_binary "$temp_file"

    # Create configuration
    create_config

    # Install and start service
    install_service
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -s|--server-url)
            SERVER_URL="$2"
            shift 2
            ;;
        -k|--api-key)
            API_KEY="$2"
            shift 2
            ;;
        -d|--device-id)
            DEVICE_ID="$2"
            shift 2
            ;;
        -v|--version)
            VERSION="$2"
            shift 2
            ;;
        -u|--uninstall)
            UNINSTALL=true
            shift
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            log_error "Unknown option: $1"
            usage
            exit 1
            ;;
    esac
done

# Main execution
check_linux
check_root
check_systemd

if [[ "$UNINSTALL" == "true" ]]; then
    uninstall_agent
else
    install_agent
fi