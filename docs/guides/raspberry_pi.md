# Installing fleetd on Raspberry Pi

## Download and Install

1. Download the appropriate binary for your Raspberry Pi:

   **For Raspberry Pi 3 or newer (64-bit OS):**
   ```bash
   curl -LO https://github.com/fleetd-sh/fleetd/releases/download/v0.0.1/fleetd-linux-arm64
   chmod +x fleetd-linux-arm64
   sudo mv fleetd-linux-arm64 /usr/local/bin/fleetd
   ```

   **For older Raspberry Pi models (32-bit OS):**
   ```bash
   # Note: 32-bit ARM binary not yet available
   ```

2. Create the systemd service:
   ```bash
cp deployments/fleetd.service /etc/systemd/system/fleetd.service
   ```

3. Enable and start the service:
   ```bash
   systemctl enable fleetd
   systemctl start fleetd
   ```

4. Check the service status:
   ```bash
   systemctl status fleetd
   ```

## Troubleshooting

- View logs:
  ```bash
  journalctl -u fleetd -f
  ```

- Check if the binary is executable:
  ```bash
  ls -l /usr/local/bin/fleetd
  ```

- Verify architecture:
  ```bash
  uname -m
  ```
  Should show `aarch64` for 64-bit ARM
