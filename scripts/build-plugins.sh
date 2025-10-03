#!/bin/bash
set -e

# Build Plugin Bundles for Release
# This script packages plugins for distribution

VERSION=${1:-"dev"}
OUTPUT_DIR=${2:-"dist/plugins"}

echo "Building plugin bundles for version: $VERSION"

# Create output directory
mkdir -p "$OUTPUT_DIR"

# Package each plugin
for plugin_dir in plugins/*/; do
    if [ ! -d "$plugin_dir/v1" ]; then
        echo "Skipping $plugin_dir (no v1 directory)"
        continue
    fi

    plugin_name=$(basename "$plugin_dir")
    echo "Packaging plugin: $plugin_name"

    # Create tarball
    tar czf "$OUTPUT_DIR/${plugin_name}-${VERSION}.tar.gz" \
        -C "$plugin_dir/v1" \
        .

    # Generate checksum
    cd "$OUTPUT_DIR"
    sha256sum "${plugin_name}-${VERSION}.tar.gz" > "${plugin_name}-${VERSION}.tar.gz.sha256"
    cd - > /dev/null

    echo "  Created: ${plugin_name}-${VERSION}.tar.gz"
done

echo "Plugin bundles created in: $OUTPUT_DIR"
ls -lh "$OUTPUT_DIR"
