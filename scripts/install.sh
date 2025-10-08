#!/usr/bin/env bash

# fleetd CLI Installer Script
# Usage: curl -sSL https://get.fleetd.sh | sh
# or:    wget -qO- https://get.fleetd.sh | sh

set -euo pipefail

# Configuration
REPO_OWNER="fleetd-sh"
REPO_NAME="fleetd"
BINARY_NAME="fleetctl"
GITHUB_API="https://api.github.com"
GITHUB_REPO="${GITHUB_API}/repos/${REPO_OWNER}/${REPO_NAME}"

# Helper functions
log_info() {
    printf "[INFO] %s\n" "$1"
}

log_success() {
    printf "[SUCCESS] %s\n" "$1"
}

log_warning() {
    printf "[WARNING] %s\n" "$1"
}

log_error() {
    printf "[ERROR] %s\n" "$1"
    exit 1
}

# Detect OS and architecture
detect_platform() {
    OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
    ARCH="$(uname -m)"
    
    case "${OS}" in
        linux*)
            PLATFORM="linux"
            ;;
        darwin*)
            PLATFORM="darwin"
            ;;
        msys*|mingw*|cygwin*)
            PLATFORM="windows"
            ;;
        *)
            log_error "Unsupported operating system: ${OS}"
            ;;
    esac
    
    case "${ARCH}" in
        x86_64|amd64)
            ARCH="amd64"
            ;;
        arm64|aarch64)
            ARCH="arm64"
            ;;
        armv7l|armhf)
            ARCH="arm"
            ;;
        i386|i686)
            ARCH="386"
            ;;
        *)
            log_error "Unsupported architecture: ${ARCH}"
            ;;
    esac
    
    log_info "Detected platform: ${PLATFORM}/${ARCH}"
}

# Check for required dependencies
check_dependencies() {
    local deps_missing=false
    
    if ! command -v curl &> /dev/null && ! command -v wget &> /dev/null; then
        log_error "Neither curl nor wget found. Please install one of them."
    fi
    
    if ! command -v tar &> /dev/null; then
        log_error "tar is required but not installed."
    fi
    
    # Check for Docker (optional but recommended)
    if ! command -v docker &> /dev/null; then
        log_warning "Docker is not installed. Some features may not work."
        log_warning "Install Docker from: https://docs.docker.com/get-docker/"
    fi
}

# Get the latest version from GitHub
get_latest_version() {
    local version
    
    if command -v curl &> /dev/null; then
        version=$(curl -s "${GITHUB_REPO}/releases/latest" | grep '"tag_name":' | sed -E 's/.*"v?([^"]+)".*/\1/')
    else
        version=$(wget -qO- "${GITHUB_REPO}/releases/latest" | grep '"tag_name":' | sed -E 's/.*"v?([^"]+)".*/\1/')
    fi
    
    if [ -z "${version}" ]; then
        log_error "Could not determine latest version"
    fi
    
    echo "${version}"
}

# Download binary
download_binary() {
    local version="$1"
    local platform="$2"
    local arch="$3"
    
    local filename="${BINARY_NAME}_${version}_${platform}_${arch}.tar.gz"
    local url="https://github.com/${REPO_OWNER}/${REPO_NAME}/releases/download/v${version}/${filename}"
    local tmp_dir="$(mktemp -d)"
    local tmp_file="${tmp_dir}/${filename}"
    
    log_info "Downloading fleetd CLI v${version}..."
    log_info "URL: ${url}"
    
    if command -v curl &> /dev/null; then
        curl -fsSL "${url}" -o "${tmp_file}" || log_error "Download failed"
    else
        wget -q "${url}" -O "${tmp_file}" || log_error "Download failed"
    fi
    
    log_info "Extracting binary..."
    tar -xzf "${tmp_file}" -C "${tmp_dir}" || log_error "Extraction failed"
    
    echo "${tmp_dir}"
}

# Install binary
install_binary() {
    local src_dir="$1"
    local install_dir="${INSTALL_DIR:-/usr/local/bin}"
    
    # Check if we need sudo
    local use_sudo=""
    if [ ! -w "${install_dir}" ]; then
        if command -v sudo &> /dev/null; then
            use_sudo="sudo"
            log_info "Installing to ${install_dir} requires sudo privileges"
        else
            log_error "Cannot write to ${install_dir} and sudo is not available"
        fi
    fi
    
    log_info "Installing ${BINARY_NAME} to ${install_dir}..."
    
    ${use_sudo} mkdir -p "${install_dir}"
    ${use_sudo} cp "${src_dir}/${BINARY_NAME}" "${install_dir}/"
    ${use_sudo} chmod +x "${install_dir}/${BINARY_NAME}"
    
    # Verify installation
    if command -v "${BINARY_NAME}" &> /dev/null; then
        local installed_version="$(${BINARY_NAME} version --short 2>/dev/null || echo 'unknown')"
        log_success "fleetd CLI installed successfully!"
        log_info "Version: ${installed_version}"
    else
        log_warning "${BINARY_NAME} installed but not in PATH"
        log_info "Add ${install_dir} to your PATH:"
        log_info "  export PATH=\"${install_dir}:\$PATH\""
    fi
}

# Install shell completions
install_completions() {
    local shell="${SHELL##*/}"
    
    case "${shell}" in
        bash)
            if [ -d "/etc/bash_completion.d" ]; then
                log_info "Installing bash completions..."
                ${BINARY_NAME} completion bash | sudo tee /etc/bash_completion.d/fleetctl > /dev/null
            fi
            ;;
        zsh)
            if [ -d "${HOME}/.oh-my-zsh/custom/plugins" ]; then
                log_info "Installing zsh completions..."
                mkdir -p "${HOME}/.oh-my-zsh/custom/plugins/fleetctl"
                ${BINARY_NAME} completion zsh > "${HOME}/.oh-my-zsh/custom/plugins/fleetctl/_fleetctl"
            fi
            ;;
        fish)
            if [ -d "${HOME}/.config/fish/completions" ]; then
                log_info "Installing fish completions..."
                ${BINARY_NAME} completion fish > "${HOME}/.config/fish/completions/fleetctl.fish"
            fi
            ;;
    esac
}

# Main installation flow
main() {
    printf "╔════════════════════════════════════╗\n"
    printf "║   fleetd CLI Installer             ║\n"
    printf "╚════════════════════════════════════╝\n"
    echo
    
    # Parse command line arguments
    while [[ $# -gt 0 ]]; do
        case $1 in
            --version)
                VERSION="$2"
                shift 2
                ;;
            --install-dir)
                INSTALL_DIR="$2"
                shift 2
                ;;
            --no-completions)
                NO_COMPLETIONS=true
                shift
                ;;
            *)
                log_error "Unknown option: $1"
                ;;
        esac
    done
    
    detect_platform
    check_dependencies
    
    # Get version to install
    if [ -z "${VERSION:-}" ]; then
        VERSION="$(get_latest_version)"
        log_info "Installing latest version: v${VERSION}"
    else
        log_info "Installing specified version: v${VERSION}"
    fi
    
    # Download and install
    local tmp_dir
    tmp_dir="$(download_binary "${VERSION}" "${PLATFORM}" "${ARCH}")"
    
    install_binary "${tmp_dir}"
    
    # Install completions unless disabled
    if [ "${NO_COMPLETIONS:-false}" != "true" ]; then
        install_completions
    fi
    
    # Cleanup
    rm -rf "${tmp_dir}"
    
    echo
    log_success "Installation complete!"
    echo
    echo "To get started, run:"
    echo "  ${BINARY_NAME} init        # Initialize a new fleetd project"
    echo "  ${BINARY_NAME} start       # Start local fleetd stack"
    echo "  ${BINARY_NAME} help        # Show available commands"
    echo
    echo "Documentation: https://docs.fleetd.sh"
    echo "GitHub: https://github.com/${REPO_OWNER}/${REPO_NAME}"
}

main "$@"
