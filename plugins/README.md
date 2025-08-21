# Plugins

This directory contains plugins that extend fleetp's provisioning capabilities beyond the core fleetd agent setup.

## What are plugins?

Plugins are optional components that add functionality during device provisioning. The core fleetp tool focuses on:
- Installing the fleetd agent
- Configuring network connectivity
- Setting up mDNS discovery
- Basic SSH access

Everything else (k3s, Docker, monitoring tools, etc.) is handled through plugins.

## Available plugins

### k3s
Adds Kubernetes edge computing capabilities to Raspberry Pi devices.

```bash
# Use with fleetp
fleetp -type rpi -device /dev/disk2 \
  -plugin k3s \
  -plugin-opt k3s.role=server
```

### docker (example)
Installs and configures Docker on capable devices.

```bash
fleetp -type rpi -device /dev/disk2 \
  -plugin docker \
  -plugin-opt docker.version=latest
```

## Creating a Plugin

Plugins implement the `provision.Hook` interface:

```go
package main

import (
    "context"
    "fleetd.sh/internal/provision"
)

type MyPlugin struct {
    *provision.BaseHook
}

// Hook is the required export
var Hook provision.Hook = &MyPlugin{
    BaseHook: provision.NewBaseHook("myplugin", 100),
}

// Add your implementation...
func (p *MyPlugin) ModifyConfig(config *provision.Config) error {
    // Modify provisioning configuration
    return nil
}

func (p *MyPlugin) AddFiles() (map[string][]byte, error) {
    // Add files to be written to device
    return nil, nil
}
```

## Building plugins

```bash
# Build as a Go plugin
go build -buildmode=plugin -o myplugin.so myplugin.go

# Place in plugins directory
cp myplugin.so ~/.fleetd/plugins/
```

## Plugin configuration

Plugins can be configured via:

1. **Command line options**:
   ```bash
   fleetp -plugin myplugin -plugin-opt myplugin.key=value
   ```

2. **Configuration file**:
   ```json
   {
     "plugins": {
       "myplugin": {
         "key": "value"
       }
     }
   }
   ```

3. **Environment variables**:
   ```bash
   FLEETD_PLUGIN_MYPLUGIN_KEY=value fleetp ...
   ```

## Plugin hooks

Plugins can hook into these provisioning stages:

- **PreProvision**: Before any provisioning starts
- **ModifyConfig**: Modify the provisioning configuration
- **AddFiles**: Add files to be written to the device
- **AddTemplates**: Add templates to be processed
- **PostProvision**: After provisioning completes

## Security

Plugins run with the same privileges as fleetp. Only install trusted plugins.

## Examples

See the `k3s` directory for a complete plugin example.
