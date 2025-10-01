#!/bin/sh
# fleetd CLI Installation Script
# This script would be hosted at https://get.fleetd.sh
#
# Usage:
#   curl -sSL https://get.fleetd.sh | sh
#   wget -qO- https://get.fleetd.sh | sh
#
# Options:
#   INSTALL_DIR=/custom/path - Install to custom directory
#   VERSION=v1.0.0 - Install specific version
#   INSTALL_DOCKER_COMPOSE=1 - Also install docker-compose files
#

set -e

# Configuration
BINARY_NAME="fleetctl"
GITHUB_REPO="fleetd-sh/fleetd"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
CONFIG_DIR="${CONFIG_DIR:-$HOME/.fleetd}"
VERSION="${VERSION:-latest}"

# Determine base URL
if [ "$VERSION" = "latest" ]; then
    BASE_URL="https://github.com/${GITHUB_REPO}/releases/latest/download"
else
    BASE_URL="https://github.com/${GITHUB_REPO}/releases/download/${VERSION}"
fi

# Detect OS and Architecture
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$ARCH" in
    x86_64)
        ARCH="amd64"
        ;;
    aarch64|arm64)
        ARCH="arm64"
        ;;
    armv7l|armv6l)
        ARCH="arm"
        ;;
    *)
        echo "Unsupported architecture: $ARCH"
        exit 1
        ;;
esac

case "$OS" in
    darwin)
        OS="darwin"
        ;;
    linux)
        OS="linux"
        ;;
    *)
        echo "Unsupported OS: $OS"
        exit 1
        ;;
esac

# Construct download URL
BINARY_FILE="${BINARY_NAME}_${OS}_${ARCH}"
if [ "$OS" = "darwin" ]; then
    # macOS binaries often don't have .exe but might have different naming
    DOWNLOAD_URL="${BASE_URL}/${BINARY_FILE}"
else
    DOWNLOAD_URL="${BASE_URL}/${BINARY_FILE}"
fi

# Create temp directory
TMP_DIR=$(mktemp -d -t fleetctl-install.XXXXXXXXXX)
trap "rm -rf $TMP_DIR" EXIT

# Print installation info
echo "═══════════════════════════════════════════════════════════════"
echo "                    fleetd Platform Installer                   "
echo "═══════════════════════════════════════════════════════════════"
echo ""
echo "Installing fleetd CLI..."
echo "  Version: $VERSION"
echo "  OS: $OS"
echo "  Architecture: $ARCH"
echo "  Install directory: $INSTALL_DIR"
echo "  Config directory: $CONFIG_DIR"
echo ""
echo "Downloading from: $DOWNLOAD_URL"
echo ""

# Download binary
if command -v curl >/dev/null 2>&1; then
    curl -sSL "$DOWNLOAD_URL" -o "$TMP_DIR/$BINARY_NAME"
elif command -v wget >/dev/null 2>&1; then
    wget -q "$DOWNLOAD_URL" -O "$TMP_DIR/$BINARY_NAME"
else
    echo "Error: Neither curl nor wget found. Please install one of them."
    exit 1
fi

# Make binary executable
chmod +x "$TMP_DIR/$BINARY_NAME"

# Check if we need sudo for installation
if [ -w "$INSTALL_DIR" ]; then
    mv "$TMP_DIR/$BINARY_NAME" "$INSTALL_DIR/"
    echo "fleetd CLI installed to $INSTALL_DIR/$BINARY_NAME"
else
    echo "Installing to $INSTALL_DIR requires sudo privileges..."
    sudo mv "$TMP_DIR/$BINARY_NAME" "$INSTALL_DIR/"
    echo "fleetd CLI installed to $INSTALL_DIR/$BINARY_NAME (with sudo)"
fi

# Create config directory if it doesn't exist
if [ ! -d "$CONFIG_DIR" ]; then
    mkdir -p "$CONFIG_DIR"
    echo "Created config directory at $CONFIG_DIR"
fi

# Install Docker Compose files if requested
if [ "$INSTALL_DOCKER_COMPOSE" = "1" ]; then
    echo ""
    echo "Installing Docker Compose configuration..."

    # Create docker directory
    DOCKER_DIR="$CONFIG_DIR/docker"
    mkdir -p "$DOCKER_DIR"

    # Download docker-compose files
    echo "Downloading docker-compose.yml..."
    if command -v curl >/dev/null 2>&1; then
        curl -sSL "${BASE_URL}/docker-compose.yml" -o "$DOCKER_DIR/docker-compose.yml"
        curl -sSL "${BASE_URL}/.env.example" -o "$DOCKER_DIR/.env"
    elif command -v wget >/dev/null 2>&1; then
        wget -q "${BASE_URL}/docker-compose.yml" -O "$DOCKER_DIR/docker-compose.yml"
        wget -q "${BASE_URL}/.env.example" -O "$DOCKER_DIR/.env"
    fi

    echo "Docker Compose files installed to $DOCKER_DIR"
fi

# Verify installation
if command -v $BINARY_NAME >/dev/null 2>&1; then
    echo ""
    echo "═══════════════════════════════════════════════════════════════"
    echo "           Installation Completed Successfully! 🎉              "
    echo "═══════════════════════════════════════════════════════════════"
    echo ""
    echo "fleetd CLI version:"
    $BINARY_NAME version 2>/dev/null || echo "  $VERSION"
    echo ""
    echo "Quick Start:"
    echo "  $BINARY_NAME start              # Start fleetd platform locally"
    echo "  $BINARY_NAME status             # Check platform status"
    echo "  $BINARY_NAME logs               # View platform logs"
    echo ""
    echo "Production Deployment:"
    echo "  $BINARY_NAME certs init         # Setup TLS certificates"
    echo "  $BINARY_NAME migrate            # Run database migrations"
    echo "  $BINARY_NAME start --production # Start in production mode"
    echo ""
    echo "Documentation: https://github.com/fleetd-sh/fleetd/wiki"
    echo "Get Help: $BINARY_NAME --help"
else
    echo ""
    echo "⚠️  Installation complete, but $BINARY_NAME is not in your PATH."
    echo ""
    echo "To add to PATH, run one of:"
    echo "  export PATH=\"\$PATH:$INSTALL_DIR\""
    echo "  echo 'export PATH=\"\$PATH:$INSTALL_DIR\"' >> ~/.bashrc"
    echo "  echo 'export PATH=\"\$PATH:$INSTALL_DIR\"' >> ~/.zshrc"
    echo ""
    echo "Or run directly: $INSTALL_DIR/$BINARY_NAME --help"
fi

# Check for Docker if not already installed
if ! command -v docker >/dev/null 2>&1; then
    echo ""
    echo "⚠️  Docker is not installed."
    echo "Fleet requires Docker for container management."
    echo "Install Docker: https://docs.docker.com/get-docker/"
fi