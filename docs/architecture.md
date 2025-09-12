# FleetD Architecture

## Overview

FleetD is a heterogeneous fleet management system with three main components:

### 1. fleetd (Agent)
The lightweight agent that runs on each device in the fleet.

**Usage:**
```bash
# Run agent with default settings (requires sudo for /var/lib/fleetd)
sudo fleetd agent

# Run agent with custom storage (no sudo needed)
fleetd agent --storage-dir ~/.fleetd

# Run agent with auto-selected port
fleetd agent --rpc-port 0

# Check version
fleetd version
```

**Features:**
- Minimal resource footprint
- Auto-registers with fleet server via mDNS
- Collects telemetry
- Handles updates
- Runs on Raspberry Pi, ESP32, and other edge devices

### 2. fleets (Server CLI)
The central management CLI for fleet administrators.

**Usage:**
```bash
# Discover devices on the network
fleets discover
fleets discover --all
fleets discover --service _ssh._tcp

# Manage devices (requires running server)
fleets devices list
fleets devices register [device-id]

# Run the fleet server
fleets server --port 8080 --db fleet.db

# Check version
fleets version
```

**Features:**
- Device discovery via mDNS
- Fleet management operations
- Update deployment
- Telemetry collection
- Web dashboard (when server is running)

### 3. fleetp (Provisioning Tool)
Tool for provisioning new devices with fleetd agent.

**Usage:**
```bash
# List available devices
fleetp list

# Provision a Raspberry Pi with DietPi
fleetp provision -device /dev/disk2 -wifi-ssid "Network" -wifi-pass "password"

# Provision with k3s server
fleetp provision -device /dev/disk2 -wifi-ssid "Network" -wifi-pass "password" \
  -os dietpi -plugin k3s -plugin-opt k3s.role=server

# Use custom OS image
fleetp provision -device /dev/disk2 -wifi-ssid "Network" -wifi-pass "password" \
  -os custom -image-url https://example.com/custom.img.xz

# Check version
fleetp version
```

**Features:**
- SD card provisioning for Raspberry Pi
- Automated OS setup (DietPi by default)
- Plugin system (k3s, docker, etc.)
- Zero-touch configuration
- WiFi and SSH key setup

## Network Architecture

```
┌─────────────┐     mDNS Discovery       ┌─────────────┐
│   fleets    │ ◄──────────────────────► │   fleetd    │
│  (Server)   │                          │  (Agent 1)  │
└──────┬──────┘                          └─────────────┘
       │
       │ HTTP/gRPC                       ┌─────────────┐
       ├────────────────────────────────►│   fleetd    │
       │                                 │  (Agent 2)  │
       │                                 └─────────────┘
       │
       │                                 ┌─────────────┐
       └────────────────────────────────►│   fleetd    │
                                         │  (Agent N)  │
                                         └─────────────┘
```

## Key Design Decisions

1. **Separation of Concerns**
   - `fleetd`: Lightweight agent for devices
   - `fleets`: Management CLI for administrators
   - `fleetp`: Provisioning tool for new devices

2. **Discovery**
   - Discovery functionality lives in `fleets` (server)
   - Agents announce themselves via mDNS
   - No agent-to-agent discovery needed

3. **Permissions**
   - `fleetd agent` uses user directory when not root
   - Auto-selects available ports
   - Interactive sudo only when needed

4. **Automation**
   - DietPi provisioning is fully automated
   - Auto-generated passwords: `fleetd-[device-id-prefix]`
   - Zero-touch setup after SD card write
```
