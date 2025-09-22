#!/usr/bin/env bash

# FleetD Service Installer
# Installs FleetD as a system service (systemd or launchd)

set -euo pipefail

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
log_warning() { echo -e "${YELLOW}[WARNING]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; exit 1; }

# Detect OS
detect_os() {
    if [[ "$OSTYPE" == "linux-gnu"* ]]; then
        if command -v systemctl &> /dev/null; then
            echo "systemd"
        else
            log_error "Linux system without systemd is not supported"
        fi
    elif [[ "$OSTYPE" == "darwin"* ]]; then
        echo "launchd"
    else
        log_error "Unsupported OS: $OSTYPE"
    fi
}

# Install systemd service (Linux)
install_systemd_service() {
    local service_type="$1"
    local config_path="${2:-/etc/fleetd/config.yaml}"
    local binary_path="$(which fleetctl || echo /usr/local/bin/fleetctl)"
    
    log_info "Installing FleetD systemd service..."
    
    # Create service file
    local service_file="/etc/systemd/system/fleetd-${service_type}.service"
    
    case "$service_type" in
        agent)
            cat <<EOF | sudo tee "$service_file" > /dev/null
[Unit]
Description=FleetD Agent Service
Documentation=https://docs.fleetd.sh
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=fleetd
Group=fleetd
WorkingDirectory=/var/lib/fleetd
Environment="FLEETD_CONFIG=${config_path}"
ExecStart=${binary_path} agent run --config ${config_path}
ExecReload=/bin/kill -HUP \$MAINPID
Restart=always
RestartSec=10
StandardOutput=journal
StandardError=journal
SyslogIdentifier=fleetd-agent

# Security hardening
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/fleetd /var/log/fleetd

[Install]
WantedBy=multi-user.target
EOF
            ;;
        
        platform-api)
            cat <<EOF | sudo tee "$service_file" > /dev/null
[Unit]
Description=FleetD Platform API Service
Documentation=https://docs.fleetd.sh
After=network-online.target postgresql.service
Wants=network-online.target
Requires=postgresql.service

[Service]
Type=simple
User=fleetd
Group=fleetd
WorkingDirectory=/var/lib/fleetd
EnvironmentFile=-/etc/fleetd/platform-api.env
ExecStartPre=/usr/local/bin/platform-api migrate
ExecStart=/usr/local/bin/platform-api serve --config ${config_path}
ExecReload=/bin/kill -HUP \$MAINPID
Restart=always
RestartSec=10
StandardOutput=journal
StandardError=journal
SyslogIdentifier=fleetd-platform-api

# Security hardening
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/fleetd /var/log/fleetd

[Install]
WantedBy=multi-user.target
EOF
            ;;
        
        device-api)
            cat <<EOF | sudo tee "$service_file" > /dev/null
[Unit]
Description=FleetD Device API Service
Documentation=https://docs.fleetd.sh
After=network-online.target fleetd-platform-api.service
Wants=network-online.target
Requires=fleetd-platform-api.service

[Service]
Type=simple
User=fleetd
Group=fleetd
WorkingDirectory=/var/lib/fleetd
EnvironmentFile=-/etc/fleetd/device-api.env
ExecStart=/usr/local/bin/device-api serve --config ${config_path}
ExecReload=/bin/kill -HUP \$MAINPID
Restart=always
RestartSec=10
StandardOutput=journal
StandardError=journal
SyslogIdentifier=fleetd-device-api

# Security hardening
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/fleetd /var/log/fleetd

[Install]
WantedBy=multi-user.target
EOF
            ;;
        
        *)
            log_error "Unknown service type: $service_type"
            ;;
    esac
    
    # Create user and directories
    if ! id -u fleetd &> /dev/null; then
        log_info "Creating fleetd user..."
        sudo useradd -r -s /bin/false -m -d /var/lib/fleetd fleetd
    fi
    
    # Create directories
    sudo mkdir -p /etc/fleetd /var/lib/fleetd /var/log/fleetd
    sudo chown -R fleetd:fleetd /var/lib/fleetd /var/log/fleetd
    sudo chmod 755 /etc/fleetd
    
    # Create default config if not exists
    if [ ! -f "$config_path" ]; then
        log_info "Creating default configuration..."
        create_default_config "$config_path" "$service_type"
    fi
    
    # Reload systemd and enable service
    sudo systemctl daemon-reload
    sudo systemctl enable "fleetd-${service_type}.service"
    
    log_success "FleetD $service_type service installed"
    echo
    echo "To start the service:"
    echo "  sudo systemctl start fleetd-${service_type}"
    echo
    echo "To view logs:"
    echo "  sudo journalctl -u fleetd-${service_type} -f"
}

# Install launchd service (macOS)
install_launchd_service() {
    local service_type="$1"
    local config_path="${2:-$HOME/.fleetd/config.yaml}"
    local binary_path="$(which fleetctl || echo /usr/local/bin/fleetctl)"
    
    log_info "Installing FleetD launchd service..."
    
    local plist_file="$HOME/Library/LaunchAgents/sh.fleetd.${service_type}.plist"
    
    case "$service_type" in
        agent)
            cat <<EOF > "$plist_file"
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>sh.fleetd.agent</string>
    
    <key>ProgramArguments</key>
    <array>
        <string>${binary_path}</string>
        <string>agent</string>
        <string>run</string>
        <string>--config</string>
        <string>${config_path}</string>
    </array>
    
    <key>EnvironmentVariables</key>
    <dict>
        <key>FLEETD_CONFIG</key>
        <string>${config_path}</string>
    </dict>
    
    <key>WorkingDirectory</key>
    <string>$HOME/.fleetd</string>
    
    <key>RunAtLoad</key>
    <true/>
    
    <key>KeepAlive</key>
    <dict>
        <key>SuccessfulExit</key>
        <false/>
    </dict>
    
    <key>StandardOutPath</key>
    <string>$HOME/.fleetd/logs/agent.log</string>
    
    <key>StandardErrorPath</key>
    <string>$HOME/.fleetd/logs/agent.error.log</string>
    
    <key>ThrottleInterval</key>
    <integer>10</integer>
</dict>
</plist>
EOF
            ;;
        
        platform-api)
            cat <<EOF > "$plist_file"
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>sh.fleetd.platform-api</string>
    
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/platform-api</string>
        <string>serve</string>
        <string>--config</string>
        <string>${config_path}</string>
    </array>
    
    <key>EnvironmentVariables</key>
    <dict>
        <key>FLEETD_CONFIG</key>
        <string>${config_path}</string>
        <key>PATH</key>
        <string>/usr/local/bin:/usr/bin:/bin</string>
    </dict>
    
    <key>WorkingDirectory</key>
    <string>$HOME/.fleetd</string>
    
    <key>RunAtLoad</key>
    <true/>
    
    <key>KeepAlive</key>
    <dict>
        <key>SuccessfulExit</key>
        <false/>
    </dict>
    
    <key>StandardOutPath</key>
    <string>$HOME/.fleetd/logs/platform-api.log</string>
    
    <key>StandardErrorPath</key>
    <string>$HOME/.fleetd/logs/platform-api.error.log</string>
    
    <key>ThrottleInterval</key>
    <integer>10</integer>
</dict>
</plist>
EOF
            ;;
        
        *)
            log_error "Unknown service type: $service_type"
            ;;
    esac
    
    # Create directories
    mkdir -p "$HOME/.fleetd/logs"
    
    # Create default config if not exists
    if [ ! -f "$config_path" ]; then
        log_info "Creating default configuration..."
        create_default_config "$config_path" "$service_type"
    fi
    
    # Load the service
    launchctl load "$plist_file"
    
    log_success "FleetD $service_type service installed"
    echo
    echo "To start the service:"
    echo "  launchctl start sh.fleetd.${service_type}"
    echo
    echo "To stop the service:"
    echo "  launchctl stop sh.fleetd.${service_type}"
    echo
    echo "To view logs:"
    echo "  tail -f $HOME/.fleetd/logs/${service_type}.log"
}

# Create default configuration
create_default_config() {
    local config_path="$1"
    local service_type="$2"
    
    case "$service_type" in
        agent)
            cat <<EOF | sudo tee "$config_path" > /dev/null
# FleetD Agent Configuration

server:
  url: "https://api.fleetd.sh"
  token: "YOUR_API_TOKEN_HERE"

agent:
  id: "$(hostname -s)"
  name: "$(hostname)"
  tags:
    - "production"
    - "linux"

telemetry:
  enabled: true
  interval: 60s
  
logging:
  level: info
  format: json

tls:
  mode: tls
  auto_generate: true
EOF
            ;;
        
        platform-api)
            cat <<EOF | sudo tee "$config_path" > /dev/null
# FleetD Platform API Configuration

server:
  host: "0.0.0.0"
  port: 8090

database:
  url: "postgres://fleetd:password@localhost/fleetd?sslmode=disable"
  max_connections: 25

auth:
  jwt_secret: "$(openssl rand -hex 32)"
  jwt_access_ttl: "15m"
  jwt_refresh_ttl: "168h"

tls:
  mode: tls
  auto_generate: true
  organization: "FleetD"
  common_name: "platform-api.local"
  
logging:
  level: info
  format: json
EOF
            ;;
    esac
    
    sudo chmod 600 "$config_path"
    if [ "$service_type" != "agent" ]; then
        sudo chown fleetd:fleetd "$config_path" 2>/dev/null || true
    fi
}

# Uninstall service
uninstall_service() {
    local service_type="$1"
    local os_type="$(detect_os)"
    
    if [ "$os_type" == "systemd" ]; then
        log_info "Uninstalling systemd service..."
        sudo systemctl stop "fleetd-${service_type}" 2>/dev/null || true
        sudo systemctl disable "fleetd-${service_type}" 2>/dev/null || true
        sudo rm -f "/etc/systemd/system/fleetd-${service_type}.service"
        sudo systemctl daemon-reload
        log_success "Service uninstalled"
    elif [ "$os_type" == "launchd" ]; then
        log_info "Uninstalling launchd service..."
        launchctl stop "sh.fleetd.${service_type}" 2>/dev/null || true
        launchctl unload "$HOME/Library/LaunchAgents/sh.fleetd.${service_type}.plist" 2>/dev/null || true
        rm -f "$HOME/Library/LaunchAgents/sh.fleetd.${service_type}.plist"
        log_success "Service uninstalled"
    fi
}

# Main
main() {
    local action="${1:-install}"
    local service_type="${2:-agent}"
    local config_path="${3:-}"
    
    case "$action" in
        install)
            os_type="$(detect_os)"
            if [ "$os_type" == "systemd" ]; then
                install_systemd_service "$service_type" "$config_path"
            elif [ "$os_type" == "launchd" ]; then
                install_launchd_service "$service_type" "$config_path"
            fi
            ;;
        
        uninstall)
            uninstall_service "$service_type"
            ;;
        
        *)
            echo "FleetD Service Installer"
            echo
            echo "Usage: $0 [install|uninstall] [agent|platform-api|device-api] [config-path]"
            echo
            echo "Examples:"
            echo "  $0 install agent                    # Install agent service"
            echo "  $0 install platform-api             # Install platform API service"
            echo "  $0 uninstall agent                  # Uninstall agent service"
            echo "  $0 install agent /path/to/config    # Install with custom config"
            exit 1
            ;;
    esac
}

main "$@"
