# Setting up a Device Fleet with fleetd

This guide will walk you through setting up devices using fleetd.

## Prerequisites

- Linux-based device

## Step 1: Set up fleetd on devices

1. Download the latest fleetd binary from the [releases page](https://github.com/fleetd-sh/fleetd/releases) or build it from source.

2. Copy the built binary to your device:
   ```
   scp bin/fleetd device_user@device_ip:/usr/local/bin/
   ```

3. Create a configuration file on the device:
   ```
   cat << EOF > /etc/fleetd.yaml
   device_id: "unique_device_id"
   fleet_api_url: "http://your_fleetd_stack_url"
   EOF
   ```

4. Set up fleetd as a system service:
   ```
   cat << EOF > /etc/systemd/system/fleetd.service
   [Unit]
   Description=Fleet Daemon
   After=network.target

   [Service]
   ExecStart=/usr/local/bin/fleetd
   Restart=always

   [Install]
   WantedBy=multi-user.target
   EOF

   systemctl enable fleetd
   systemctl start fleetd
   ```

## Step 2: Verify the setup

This step requires a running instance of the fleetd stack.

1. Check the device registration:
   ```
   grpcurl -plaintext -H "Authorization: Bearer YOUR_API_KEY" http://your_fleetd_stack_url/device.v1.DeviceService/ListDevices
   ```

2. Monitor metrics:
   ```
   grpcurl -plaintext -H "Authorization: Bearer YOUR_API_KEY" http://your_fleetd_stack_url/metrics.v1.MetricsService/GetMetrics -d '{"device_id": "unique_device_id"}'
   ```

3. Trigger an update:
   ```
   grpcurl -plaintext -H "Authorization: Bearer YOUR_API_KEY" http://your_fleetd_stack_url/update.v1.UpdateService/CreatePackage -d '{"version": "1.0.1", "description": "Test update"}'
   ```


> [!TIP]
> The full API schema can be inspected with grpcurl:
> ```bash
>   grpcurl -plaintext your_fleetd_stack_url describe
> ```

## Next Steps

- Set up monitoring and alerting for your fleet
- Implement custom metrics collection for your specific use case
- Set up a web interface for managing your fleet

