#!/bin/bash
set -e

# K3s Installation Script
# This script is executed on first boot of the Raspberry Pi

BOOT_PARTITION="/boot"
if [ -d "/boot/firmware" ]; then
    BOOT_PARTITION="/boot/firmware"
fi

log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] k3s: $*"
}

log "Starting k3s installation"

# Source environment variables from provisioning
if [ -f "${BOOT_PARTITION}/plugins/k3s.env" ]; then
    source "${BOOT_PARTITION}/plugins/k3s.env"
else
    log "ERROR: k3s.env not found, cannot configure k3s"
    exit 1
fi

# Validate required variables
if [ -z "$K3S_ROLE" ]; then
    log "ERROR: K3S_ROLE not set"
    exit 1
fi

# Set k3s version
export INSTALL_K3S_VERSION="${K3S_VERSION:-v1.28.3+k3s1}"

log "Installing k3s version: $INSTALL_K3S_VERSION"
log "Role: $K3S_ROLE"

# Install k3s based on role
if [ "$K3S_ROLE" = "server" ]; then
    log "Installing as k3s server"

    K3S_ARGS="server"

    if [ "$K3S_CLUSTER_INIT" = "true" ]; then
        K3S_ARGS="$K3S_ARGS --cluster-init"
    fi

    curl -sfL https://get.k3s.io | sh -s - $K3S_ARGS \
        --write-kubeconfig-mode 644 \
        --disable traefik \
        --disable servicelb

    # Wait for k3s to be ready
    sleep 10

    # Save the node token for adding agents
    if [ -f /var/lib/rancher/k3s/server/node-token ]; then
        cp /var/lib/rancher/k3s/server/node-token "${BOOT_PARTITION}/k3s-node-token.txt"
        chmod 600 "${BOOT_PARTITION}/k3s-node-token.txt"
        log "Node token saved to ${BOOT_PARTITION}/k3s-node-token.txt"
    fi

    # Save kubeconfig
    if [ -f /etc/rancher/k3s/k3s.yaml ]; then
        cp /etc/rancher/k3s/k3s.yaml "${BOOT_PARTITION}/k3s-kubeconfig.yaml"
        chmod 600 "${BOOT_PARTITION}/k3s-kubeconfig.yaml"
        log "Kubeconfig saved to ${BOOT_PARTITION}/k3s-kubeconfig.yaml"
    fi

elif [ "$K3S_ROLE" = "agent" ]; then
    log "Installing as k3s agent"

    if [ -z "$K3S_URL" ] || [ -z "$K3S_TOKEN" ]; then
        log "ERROR: K3S_URL and K3S_TOKEN required for agent role"
        exit 1
    fi

    export K3S_URL
    export K3S_TOKEN

    curl -sfL https://get.k3s.io | sh -s - agent

else
    log "ERROR: Invalid K3S_ROLE: $K3S_ROLE (must be 'server' or 'agent')"
    exit 1
fi

# Verify installation
if systemctl is-active --quiet k3s || systemctl is-active --quiet k3s-agent; then
    log "k3s installation completed successfully"

    # Mark installation as complete
    touch "${BOOT_PARTITION}/plugins/.k3s-installed"
else
    log "ERROR: k3s service is not running"
    systemctl status k3s || systemctl status k3s-agent || true
    exit 1
fi

log "k3s is now running"
