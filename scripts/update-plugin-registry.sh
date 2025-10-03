#!/bin/bash
set -e

# Update Plugin Registry
# This script generates/updates the plugin registry JSON file

VERSION=${1:-"dev"}
REGISTRY_FILE=${2:-"dist/registry.json"}

echo "Updating plugin registry for version: $VERSION"

# Start JSON
cat > "$REGISTRY_FILE" <<EOF
{
  "version": "1.0.0",
  "updated": "$(date -u +"%Y-%m-%dT%H:%M:%SZ")",
  "plugins": {
EOF

first=true
for plugin_dir in plugins/*/; do
    if [ ! -f "$plugin_dir/v1/plugin.yaml" ]; then
        continue
    fi

    plugin_name=$(basename "$plugin_dir")

    # Extract version from plugin.yaml
    plugin_version=$(grep "^version:" "$plugin_dir/v1/plugin.yaml" | awk '{print $2}' | tr -d '"' | tr -d "'")

    # Extract description
    description=$(grep "^description:" "$plugin_dir/v1/plugin.yaml" | cut -d: -f2- | sed 's/^ *//' | tr -d '"')

    if [ "$first" = false ]; then
        echo "," >> "$REGISTRY_FILE"
    fi
    first=false

    # Add plugin entry
    cat >> "$REGISTRY_FILE" <<EOF
    "$plugin_name": {
      "version": "$plugin_version",
      "description": "$description",
      "url": "https://github.com/fleetd-sh/fleetd/releases/download/${VERSION}/${plugin_name}-${VERSION}.tar.gz",
      "checksum_url": "https://github.com/fleetd-sh/fleetd/releases/download/${VERSION}/${plugin_name}-${VERSION}.tar.gz.sha256",
      "embedded": true
    }
EOF

    echo "  Added: $plugin_name v$plugin_version"
done

# Close JSON
cat >> "$REGISTRY_FILE" <<EOF

  }
}
EOF

echo "Plugin registry created: $REGISTRY_FILE"
cat "$REGISTRY_FILE"
