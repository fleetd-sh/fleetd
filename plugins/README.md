# fleetd Plugins

Plugins extend fleetd's provisioning capabilities beyond the core agent setup.

## Plugin System

fleetd uses a YAML-based plugin system that:
- Works without CGO (no compilation needed)
- Is platform-independent
- Supports embedded, local file, and URL-based plugins
- Uses simple YAML configuration files and shell scripts
- No central registry required - third parties self-host their plugins

## Plugin Sources

fleetctl supports loading plugins from multiple sources:

**Embedded plugins** (built into fleetctl):
```bash
fleetctl provision --device /dev/disk2 --plugin k3s
```

**Local file paths** (for development):
```bash
fleetctl provision --device /dev/disk2 --plugin ./my-plugin/plugin.yaml
fleetctl provision --device /dev/disk2 --plugin /path/to/custom-plugin/plugin.yaml
```

**URLs** (third-party plugins):
```bash
fleetctl provision --device /dev/disk2 \
  --plugin https://example.com/plugins/docker/plugin.yaml
```

## Available Plugins

### k3s (embedded)
Lightweight Kubernetes cluster support for edge devices.

**Usage:**
```bash
# Server node
fleetctl provision --device /dev/disk2 \
  --plugin k3s \
  --plugin-opt k3s.role=server

# Agent node
fleetctl provision --device /dev/disk2 \
  --plugin k3s \
  --plugin-opt k3s.role=agent \
  --plugin-opt k3s.server=https://192.168.1.10:6443 \
  --plugin-opt k3s.token=<token-from-server>
```

## Plugin Commands

```bash
# List available embedded plugins
fleetctl plugin list

# Show plugin details
fleetctl plugin info k3s
```

## Plugin Structure

Each plugin consists of:

```
plugins/
  k3s/
    v1/
      plugin.yaml       # Plugin manifest
      install.sh        # Installation script
      *.env.tmpl        # Configuration templates
```

### plugin.yaml

Defines plugin metadata, configuration options, and files:

```yaml
name: k3s
version: 1.0.0
description: Lightweight Kubernetes cluster support
platforms:
  - linux          # Supported platforms: linux, rtos, windows, darwin
architectures:
  - amd64          # Supported architectures: amd64, arm64, arm, xtensa
  - arm64
  - arm

options:
  role:
    type: enum
    values:
      - server
      - agent
    required: true
  server:
    type: string
    description: K3s server URL (required for agent nodes)
    required_if: role=agent
    example: https://192.168.1.10:6443

files:
  - source: install.sh
    destination: /boot/firmware/plugins/k3s-install.sh
    permissions: "0755"
    template: false
  - source: k3s.env.tmpl
    destination: /boot/firmware/plugins/k3s.env
    permissions: "0644"
    template: true

hooks:
  firstrun:
    enabled: true
    script: /boot/firmware/plugins/k3s-install.sh
    order: 100

resources:
  min_memory_mb: 512
  min_disk_mb: 2048
  requires_network: true
```

### install.sh

Shell script executed during first boot:

```bash
#!/bin/bash
set -e

# Installation logic
echo "Installing plugin..."
```

## Platform Compatibility

Plugins declare compatibility requirements via `platforms` and `architectures` fields. The plugin system automatically validates compatibility before loading plugins.

**Supported platforms:**
- `linux` - Linux-based systems (Raspberry Pi OS, Ubuntu, Debian, etc.)
- `rtos` - Real-time operating systems (FreeRTOS on ESP32)
- `windows` - Windows systems
- `darwin` - macOS systems

**Supported architectures:**
- `amd64` - x86-64 (Intel/AMD 64-bit)
- `arm64` - ARM 64-bit (Raspberry Pi 3/4/5, modern ARM servers)
- `arm` - ARM 32-bit (older Raspberry Pi models)
- `xtensa` - Xtensa architecture (ESP32)

**Example:** The k3s plugin only supports Linux:
```yaml
platforms:
  - linux
architectures:
  - amd64
  - arm64
  - arm
```

If you try to provision an ESP32 (platform=rtos, arch=xtensa) with the k3s plugin, fleetctl will reject it:
```bash
$ fleetctl provision --device /dev/ttyUSB0 --plugin k3s
Error: plugin k3s does not support platform 'rtos' (supported: [linux])
```

**Leave empty for no restrictions:**
```yaml
platforms: []       # Works on any platform
architectures: []   # Works on any architecture
```

**Custom images without platform info:**

If you use a custom image without specifying `--image-platform` and `--image-arch`, fleetctl will warn you but allow any plugins:

```bash
$ fleetctl provision --device /dev/disk2 \
  --image-url https://example.com/custom.img.xz \
  --plugin k3s

⚠ Using custom image without platform/architecture information
Plugin compatibility cannot be verified. Selected plugins require:
  - k3s: platforms: [linux], architectures: [amd64 arm64 arm]

To specify platform/architecture, use:
  --image-platform <linux|rtos|windows|darwin>
  --image-arch <amd64|arm64|arm|xtensa>
```

**Multiple plugin compatibility:**

When using multiple plugins, fleetctl ensures they have overlapping platform/architecture support:

```bash
$ fleetctl provision --device /dev/disk2 \
  --plugin linux-only-plugin \
  --plugin rtos-only-plugin
Error: plugins have no compatible platforms: rtos-only-plugin requires [rtos], but other plugins require [linux]
```

## Creating Custom Plugins

1. Create plugin directory structure:
```bash
mkdir -p ~/.fleetd/plugins/myplugin/v1
```

2. Create `plugin.yaml`:
```yaml
name: myplugin
version: 1.0.0
description: My custom plugin
platforms:
  - linux
architectures:
  - amd64
  - arm64

options:
  setting:
    type: string
    required: true

files:
  - source: install.sh
    destination: /boot/firmware/plugins/myplugin-install.sh
    permissions: "0755"
```

3. Create installation script:
```bash
cat > ~/.fleetd/plugins/myplugin/v1/install.sh <<'EOF'
#!/bin/bash
set -e
echo "Installing myplugin..."
# Your installation logic
EOF
```

4. Use the plugin:
```bash
fleetctl provision --device /dev/disk2 \
  --plugin myplugin \
  --plugin-opt myplugin.setting=value
```

## Distribution

### Embedded Plugins
Core plugins (like k3s) are embedded in fleetctl binary and always available.

### Plugin Registry
External plugins can be distributed via the plugin registry at `plugins.fleetd.sh/registry.json`.

### Custom Plugins
Place custom plugins in `~/.fleetd/plugins/` for local use.

## Migration from Go Plugins

Old Go plugin (.so files) are deprecated. To migrate:

1. Convert plugin code to YAML + shell scripts
2. Test locally in `~/.fleetd/plugins/`
3. Submit PR to add to core plugins

## Security

- All plugin scripts run with elevated privileges during provisioning
- Only install trusted plugins
- Review plugin scripts before use
- Plugin registry uses checksum verification
- HTTPS-only downloads

## Contributing

To contribute a plugin:

1. Create plugin in `plugins/<name>/v1/`
2. Follow the structure above
3. Test thoroughly on target platforms
4. Submit PR with documentation
5. Plugin will be reviewed and potentially added to core

See [CONTRIBUTING.md](../CONTRIBUTING.md) for details.
