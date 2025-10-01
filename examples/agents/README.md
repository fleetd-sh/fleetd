# fleetd Device Agent Examples

This directory contains example implementations of fleetd device agents for various platforms.

## Available Agents

### Python Agent for Raspberry Pi
- **Location**: `python-raspberry-pi/`
- **Language**: Python 3
- **Platform**: Raspberry Pi (Linux ARM)
- **Features**:
  - Automatic device enrollment
  - System telemetry reporting (CPU, memory, disk, network)
  - Temperature monitoring (Raspberry Pi specific)
  - Remote command execution
  - OTA update support
  - Log collection and upload
  - mTLS and token-based authentication

## Quick Start

### Raspberry Pi Agent

1. **Install the agent**:
```bash
cd python-raspberry-pi
chmod +x install.sh
./install.sh
```

2. **Configure the agent**:
```bash
sudo nano /etc/fleetd/agent.conf
```

Set your fleetd server URL and enrollment token:
```bash
FLEET_SERVER_URL=https://devices.fleet.yourdomain.com
FLEET_ENROLLMENT_TOKEN=your-enrollment-token
```

3. **Start the agent**:
```bash
sudo systemctl start fleetd-agent
sudo systemctl status fleetd-agent
```

4. **View logs**:
```bash
sudo journalctl -u fleetd-agent -f
```

## Agent Features

### Core Capabilities
- **Device Enrollment**: Automatic registration with fleetd platform
- **Heartbeat**: Regular connectivity checks
- **Telemetry**: System metrics reporting
- **Remote Commands**: Execute commands from fleetd platform
- **OTA Updates**: Download and install updates
- **Log Upload**: Send device logs for debugging

### Security Features
- **mTLS Support**: Certificate-based authentication
- **Token Authentication**: Secure API tokens
- **Checksum Verification**: Verify update integrity
- **Secure Credential Storage**: Protected local storage

## Creating Custom Agents

To create a custom agent for your platform:

1. **Implement the Device API**:
   - See the OpenAPI specification at `/docs/api/openapi-device.yaml`
   - Required endpoints:
     - `/api/v1/discovery` - Service discovery
     - `/api/v1/enroll` - Device enrollment
     - `/api/v1/device/heartbeat` - Connectivity check
     - `/api/v1/device/telemetry` - Metrics reporting

2. **Core Components**:
   - **Hardware ID**: Unique device identifier (MAC address, serial number)
   - **Enrollment**: One-time registration process
   - **Authentication**: Token or certificate-based
   - **Heartbeat Loop**: Regular connectivity checks
   - **Telemetry Loop**: Periodic metrics reporting
   - **Command Polling**: Check for remote commands

3. **Example Flow**:
```
1. Discover services (/api/v1/discovery)
2. Enroll device (/api/v1/enroll)
3. Save credentials locally
4. Start background threads:
   - Heartbeat thread (every 60s)
   - Telemetry thread (every 30s)
   - Command polling thread (every 10s)
5. Handle commands and updates
```

## SDK Support

fleetd provides SDKs for common languages:

- **Go**: `github.com/fleetd-sh/fleetd/sdk/go`
- **Python**: `pip install fleetd-sdk`
- **Node.js**: `npm install @fleetd/device-sdk`
- **Rust**: `cargo add fleetd-sdk`

## Platform-Specific Notes

### Raspberry Pi
- Supports temperature monitoring via `vcgencmd`
- Automatic detection of Pi model and serial number
- Optimized for ARM architecture

### Arduino/ESP32
- Lightweight C++ implementation
- Minimal memory footprint
- WiFi/Ethernet support
- See `arduino-esp32/` (coming soon)

### Industrial Controllers
- Modbus integration
- PLC communication
- Real-time data collection
- See `industrial/` (coming soon)

## Testing

To test an agent locally:

1. **Run fleetd platform locally**:
```bash
fleetctl start
```

2. **Get enrollment token**:
```bash
fleetctl fleet create-token --name test-device
```

3. **Configure agent with local URL**:
```bash
FLEET_SERVER_URL=http://localhost:8081
FLEET_ENROLLMENT_TOKEN=<token-from-step-2>
```

4. **Run agent in debug mode**:
```bash
FLEET_LOG_LEVEL=DEBUG python3 fleetd_agent.py
```

## Troubleshooting

### Common Issues

1. **Enrollment fails**:
   - Check enrollment token is valid
   - Verify server URL is correct
   - Ensure network connectivity

2. **No telemetry data**:
   - Check agent has correct permissions
   - Verify psutil is installed
   - Check system resources are accessible

3. **Commands not received**:
   - Verify heartbeat is working
   - Check command polling interval
   - Ensure device is online in fleetd dashboard

### Debug Mode

Enable debug logging:
```bash
FLEET_LOG_LEVEL=DEBUG
```

Test connectivity:
```bash
curl https://devices.fleet.yourdomain.com/api/v1/discovery
```

## Contributing

To contribute a new agent implementation:

1. Create a directory: `platform-name/`
2. Include:
   - Agent source code
   - `requirements.txt` or dependency file
   - `install.sh` installation script
   - `README.md` with platform-specific instructions
3. Follow the Device API specification
4. Add tests if possible
5. Submit a pull request

## License

All agent examples are provided under the MIT license.