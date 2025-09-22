#!/usr/bin/env bash

# FleetD Release Script
# Builds and packages binaries for multiple platforms

set -euo pipefail

VERSION="${1:-}"
if [ -z "${VERSION}" ]; then
    echo "Usage: $0 <version>"
    echo "Example: $0 0.5.3"
    exit 1
fi

# Build configuration
BINARY_NAME="fleetctl"
OUTPUT_DIR="dist"
PLATFORMS=(
    "darwin/amd64"
    "darwin/arm64"
    "linux/amd64"
    "linux/arm64"
    "linux/arm"
    "linux/386"
    "windows/amd64"
    "windows/386"
)

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

# Clean previous builds
log_info "Cleaning previous builds..."
rm -rf "${OUTPUT_DIR}"
mkdir -p "${OUTPUT_DIR}"

# Update version in code
log_info "Updating version to ${VERSION}..."
cat > cmd/fleetctl/version.go <<EOF
package main

const (
    Version = "${VERSION}"
    GitCommit = "$(git rev-parse --short HEAD 2>/dev/null || echo 'unknown')"
    BuildDate = "$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
)
EOF

# Build for each platform
for platform in "${PLATFORMS[@]}"; do
    GOOS="${platform%/*}"
    GOARCH="${platform#*/}"
    
    output_name="${BINARY_NAME}_${VERSION}_${GOOS}_${GOARCH}"
    if [ "${GOOS}" = "windows" ]; then
        output_name="${output_name}.exe"
    fi
    
    log_info "Building for ${GOOS}/${GOARCH}..."
    
    env GOOS="${GOOS}" GOARCH="${GOARCH}" CGO_ENABLED=0 \
        go build -ldflags="-w -s \
            -X main.Version=${VERSION} \
            -X main.GitCommit=$(git rev-parse --short HEAD) \
            -X main.BuildDate=$(date -u +"%Y-%m-%dT%H:%M:%SZ")" \
        -o "${OUTPUT_DIR}/${output_name}" \
        ./cmd/fleetctl
    
    # Create tar.gz archive (except for Windows)
    if [ "${GOOS}" != "windows" ]; then
        log_info "Creating archive for ${GOOS}/${GOARCH}..."
        tar -czf "${OUTPUT_DIR}/${output_name}.tar.gz" \
            -C "${OUTPUT_DIR}" \
            "${output_name}"
        rm "${OUTPUT_DIR}/${output_name}"
    else
        # Create zip for Windows
        log_info "Creating zip for ${GOOS}/${GOARCH}..."
        (cd "${OUTPUT_DIR}" && zip -q "${output_name}.zip" "${output_name}")
        rm "${OUTPUT_DIR}/${output_name}"
    fi
done

# Generate checksums
log_info "Generating checksums..."
(cd "${OUTPUT_DIR}" && shasum -a 256 *.tar.gz *.zip > checksums.txt)

# Create release notes template
log_info "Creating release notes template..."
cat > "${OUTPUT_DIR}/release_notes.md" <<EOF
# FleetD v${VERSION}

## What's New
- Feature 1
- Feature 2
- Bug fix 1

## Installation

### macOS/Linux
\`\`\`bash
curl -fsSL https://get.fleetd.sh | bash
\`\`\`

### Homebrew
\`\`\`bash
brew tap fleetd/tap
brew install fleetctl
\`\`\`

### Docker
\`\`\`bash
docker pull fleetd/fleetd:${VERSION}
\`\`\`

## Checksums
\`\`\`
$(cat "${OUTPUT_DIR}/checksums.txt")
\`\`\`
EOF

log_success "Release artifacts created in ${OUTPUT_DIR}/"
log_info "Files:"
ls -lh "${OUTPUT_DIR}/"

echo
echo "Next steps:"
echo "1. Review and edit ${OUTPUT_DIR}/release_notes.md"
echo "2. Create GitHub release with: gh release create v${VERSION} ${OUTPUT_DIR}/*.tar.gz ${OUTPUT_DIR}/*.zip"
echo "3. Update Homebrew formula with new checksums"
echo "4. Build and push Docker image"
