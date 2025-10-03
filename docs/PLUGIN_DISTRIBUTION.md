## Plugin Distribution Strategy

# Plugin Distribution Strategy for fleetd

## Overview

fleetd uses a **YAML-based plugin system** that eliminates the need for Go plugin compilation and central registries. Plugins can be embedded, loaded from local files, or fetched from URLs.

## Architecture

### Key Components

1. **Plugin Format**: YAML manifests + shell scripts (no CGO needed)
2. **Embedded Plugins**: Core plugins shipped in fleetctl binary via go:embed
3. **File-based Loading**: Local plugin.yaml files for development
4. **URL-based Loading**: Fetch plugins from any HTTPS URL
5. **CLI Commands**: `fleetctl plugin` subcommands for management

### Plugin Structure

```
plugins/
  k3s/
    v1/
      plugin.yaml       # Manifest with schema and metadata
      install.sh        # Installation script for device
      k3s.env.tmpl      # Configuration template
```

## Distribution Methods

### 1. Embedded Plugins (First-party)

**For**: Core functionality (k3s, docker, etc.)

**How it works:**
- Plugins stored in `plugins/` directory
- Embedded using `//go:embed` directive
- Always available, no download needed
- Updated with fleetctl releases

**Usage:**
```bash
fleetctl provision --device /dev/disk2 --plugin k3s
```

**Pros:**
- Instant availability
- No network dependency
- Version-locked with binary
- Zero CGO issues

### 2. Local File Plugins (Development)

**For**: Plugin development and testing

**How it works:**
- Create plugin.yaml and scripts locally
- Reference by file path
- Same YAML + script format
- No installation needed

**Usage:**
```bash
fleetctl provision --device /dev/disk2 --plugin ./my-plugin/plugin.yaml
fleetctl provision --device /dev/disk2 --plugin /path/to/plugin/plugin.yaml
```

**Pros:**
- Fast iteration during development
- No need to rebuild fleetctl
- Easy testing and debugging

### 3. URL Plugins (Third-party)

**For**: Community and third-party plugins

**How it works:**
- Third parties host their plugin.yaml files
- Users reference by HTTPS URL
- Downloaded on-demand
- No central registry needed

**Usage:**
```bash
fleetctl provision --device /dev/disk2 \
  --plugin https://example.com/plugins/docker/plugin.yaml
```

**Pros:**
- Decentralized distribution
- No registry maintenance
- Third parties control their plugins
- Users choose sources they trust

## CLI Usage

```bash
# List available embedded plugins
fleetctl plugin list

# Show plugin details
fleetctl plugin info k3s

# Use an embedded plugin during provisioning
fleetctl provision --device /dev/disk2 \
  --plugin k3s \
  --plugin-opt k3s.role=server

# Use a local plugin file
fleetctl provision --device /dev/disk2 \
  --plugin ./custom-plugin/plugin.yaml \
  --plugin-opt custom-plugin.option=value

# Use a third-party plugin from URL
fleetctl provision --device /dev/disk2 \
  --plugin https://example.com/plugins/docker/plugin.yaml \
  --plugin-opt docker.version=24.0

# Combine multiple plugins
fleetctl provision --device /dev/disk2 \
  --plugin k3s \
  --plugin https://example.com/plugins/monitoring/plugin.yaml \
  --plugin-opt k3s.role=server
```

## CI/CD Integration

### Build Process

Embedded plugins are built directly into the fleetctl binary:

```yaml
# .goreleaser.yml
builds:
  - id: fleetctl
    main: ./cmd/fleetctl
    binary: fleetctl
    # go:embed automatically includes plugins/* directory
```

No separate plugin build step needed - `go:embed` handles it automatically.

## Plugin Development

### Creating a Plugin

1. **Create structure:**
```bash
mkdir -p plugins/myplugin/v1
```

2. **Write plugin.yaml:**
```yaml
name: myplugin
version: 1.0.0
description: My custom plugin
platforms: [linux]
architectures: [amd64, arm64, arm]

options:
  setting:
    type: string
    required: true

files:
  - source: install.sh
    destination: /boot/firmware/plugins/myplugin-install.sh
    permissions: "0755"
```

3. **Write install.sh:**
```bash
#!/bin/bash
set -e

# Source configuration
source /boot/firmware/plugins/myplugin.env

# Installation logic
echo "Installing myplugin with setting: $SETTING"
```

4. **Test locally:**
```bash
# Test with local file path
fleetctl provision --device /dev/disk2 \
  --plugin ./plugins/myplugin/v1/plugin.yaml \
  --plugin-opt myplugin.setting=value

# Or with absolute path
fleetctl provision --device /dev/disk2 \
  --plugin /path/to/plugins/myplugin/v1/plugin.yaml \
  --plugin-opt myplugin.setting=value
```

5. **Distribute (optional):**
```bash
# Host plugin.yaml on your server/GitHub
# Users can then reference it:
fleetctl provision --device /dev/disk2 \
  --plugin https://yoursite.com/plugins/myplugin/plugin.yaml
```

## Security Considerations

1. **HTTPS Only**: URL-based plugins must use HTTPS
2. **Code Review**: All embedded plugins reviewed before inclusion
3. **Sandboxing**: Scripts run in restricted provisioning context
4. **User Trust Model**: Users choose which plugin sources to trust
5. **Local Verification**: Users can inspect plugin.yaml before use

**For third-party plugins**: Users should review the plugin.yaml and referenced scripts before use, especially when loading from URLs.

## Migration Path

### From Go Plugins (.so)

1. Extract logic from Go plugin
2. Convert to shell script
3. Create plugin.yaml manifest
4. Test thoroughly
5. Submit PR for inclusion

### Backward Compatibility

- Old Go plugin system marked deprecated
- Will be removed in v2.0
- Clear migration guide provided

## Distribution Infrastructure

### Required Infrastructure

**For first-party plugins:**
- Embedded in fleetctl binary via `go:embed`
- No separate hosting needed

**For third-party plugins:**
- Plugin authors host their own plugin.yaml files
- Can use GitHub Pages, Cloudflare Pages, CDN, or any static hosting
- No central infrastructure required

**Documentation:**
- Plugin development guide
- Best practices for third-party distribution
- Example plugins in fleetd repository

## Benefits

✅ **No CGO dependency** - Pure data files, works everywhere
✅ **Cross-platform** - Same on macOS, Linux, Windows
✅ **Version independent** - No Go version matching
✅ **Easy distribution** - Simple file hosting, no central registry
✅ **User-friendly** - No build tools required
✅ **Extensible** - Anyone can create and distribute plugins
✅ **Fast** - Core plugins instant via embedding
✅ **Lightweight** - Plugin YAML files are tiny (~1-5KB)
✅ **Decentralized** - No single point of failure
✅ **Flexible** - Local development, URL-based distribution

## Implementation Status

- [x] YAML plugin schema designed
- [x] Plugin loader implemented (embedded, file, URL)
- [x] go:embed support added
- [x] k3s plugin converted to YAML format
- [x] CLI commands created (`fleetctl plugin`)
- [x] Documentation written
- [x] File and URL loading support added
- [x] Registry server removed (not needed)

## Next Steps

1. Integrate plugin system fully into provision command
2. Test end-to-end plugin provisioning
3. Create additional embedded plugins (docker, monitoring, etc.)
4. Write migration guide for existing plugins
5. Create example third-party plugin repository

## Design Decisions

### No Central Registry

**Why?**
- Reduces infrastructure burden
- Decentralizes control
- Allows third parties to self-host
- Users trust sources they choose
- Simpler architecture

**Trade-offs:**
- No automatic plugin discovery
- Users must know plugin URLs
- No version tracking across ecosystem

**Verdict:** Benefits outweigh costs. Users can maintain their own plugin lists.

### Plugin Versioning

Plugins use semantic versioning independent of fleetd versions. Plugin compatibility is documented in plugin.yaml metadata.
