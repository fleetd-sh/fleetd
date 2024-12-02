# FleetD Deployment Guide

This guide explains how to deploy FleetD in production environments.

## System Requirements

### Server
- Linux (recommended) or Windows Server
- 2+ CPU cores
- 4GB+ RAM
- 20GB+ disk space
- SQLite or compatible database

### Device Agent
- Linux, Windows, or macOS
- 100MB+ RAM
- 100MB+ disk space
- Network connectivity to server

## Installation

### Server Installation

1. Download the latest server binary:
```bash
curl -L -o fleetd-server https://github.com/fleetd/fleetd/releases/latest/download/fleetd-server-$(uname -s)-$(uname -m)
chmod +x fleetd-server
```

2. Create configuration file (`config.yaml`):
```yaml
server:
  host: 0.0.0.0
  port: 8080
  metrics_port: 9090

storage:
  type: sqlite
  path: /var/lib/fleetd/data.db

binary_storage:
  type: filesystem
  path: /var/lib/fleetd/binaries

security:
  api_key_salt: "<random-string>"
  webhook_signing_secret: "<random-string>"

rate_limiting:
  requests_per_second: 100
  burst_size: 200

logging:
  level: info
  format: json
```

3. Create systemd service (`/etc/systemd/system/fleetd.service`):
```ini
[Unit]
Description=FleetD Server
After=network.target

[Service]
Type=simple
User=fleetd
Group=fleetd
ExecStart=/usr/local/bin/fleetd-server -config /etc/fleetd/config.yaml
Restart=always
RestartSec=5
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
```

4. Create user and directories:
```bash
useradd -r -s /bin/false fleetd
mkdir -p /etc/fleetd /var/lib/fleetd/binaries
chown -R fleetd:fleetd /etc/fleetd /var/lib/fleetd
```

5. Start service:
```bash
systemctl daemon-reload
systemctl enable fleetd
systemctl start fleetd
```

### Device Agent Installation

1. Download agent binary:
```bash
curl -L -o fleetd-agent https://github.com/fleetd/fleetd/releases/latest/download/fleetd-agent-$(uname -s)-$(uname -m)
chmod +x fleetd-agent
```

2. Create configuration file (`/etc/fleetd/agent.yaml`):
```yaml
server:
  address: fleetd.example.com:8080
  tls:
    enabled: true
    ca_cert: /etc/fleetd/ca.crt

device:
  name: "device-1"
  type: "raspberry-pi"
  version: "1.0.0"

storage:
  path: /var/lib/fleetd/agent

telemetry:
  interval: 60s
  metrics:
    - name: cpu
      collector: system
    - name: memory
      collector: system
    - name: disk
      collector: system

logging:
  level: info
  path: /var/log/fleetd/agent.log
```

3. Create systemd service (`/etc/systemd/system/fleetd-agent.service`):
```ini
[Unit]
Description=FleetD Agent
After=network.target

[Service]
Type=simple
User=fleetd
Group=fleetd
ExecStart=/usr/local/bin/fleetd-agent -config /etc/fleetd/agent.yaml
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

4. Start service:
```bash
systemctl daemon-reload
systemctl enable fleetd-agent
systemctl start fleetd-agent
```

## Security

### TLS Configuration

1. Generate certificates:
```bash
# Generate CA key and certificate
openssl genrsa -out ca.key 4096
openssl req -new -x509 -key ca.key -out ca.crt -days 365

# Generate server key and CSR
openssl genrsa -out server.key 2048
openssl req -new -key server.key -out server.csr

# Sign server certificate
openssl x509 -req -in server.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out server.crt -days 365
```

2. Update server config:
```yaml
security:
  tls:
    enabled: true
    cert_file: /etc/fleetd/server.crt
    key_file: /etc/fleetd/server.key
```

3. Distribute CA certificate to agents.

### Firewall Configuration

Allow the following ports:
- 8080/tcp: gRPC API
- 9090/tcp: Metrics (optional)
- 5353/udp: mDNS discovery (optional)

Example using `ufw`:
```bash
ufw allow 8080/tcp
ufw allow 9090/tcp
ufw allow 5353/udp
```

## Monitoring

### Prometheus Integration

1. Add Prometheus scrape config:
```yaml
scrape_configs:
  - job_name: fleetd
    static_configs:
      - targets: ['localhost:9090']
```

2. Available metrics:
- `fleetd_devices_total`: Total number of registered devices
- `fleetd_device_status`: Device status by type
- `fleetd_updates_total`: Total number of updates
- `fleetd_update_success_rate`: Update success rate
- `fleetd_api_requests_total`: Total API requests
- `fleetd_api_errors_total`: Total API errors

### Logging

Logs are written in JSON format for easy parsing. Example log processors:
- Fluentd
- Logstash
- Vector

Example Fluentd config:
```yaml
<source>
  @type tail
  path /var/log/fleetd/*.log
  pos_file /var/log/td-agent/fleetd.log.pos
  tag fleetd
  <parse>
    @type json
  </parse>
</source>
```

## Backup & Recovery

### Database Backup

1. Create backup script (`/usr/local/bin/fleetd-backup`):
```bash
#!/bin/bash
DATE=$(date +%Y%m%d)
BACKUP_DIR=/var/backups/fleetd

mkdir -p $BACKUP_DIR
sqlite3 /var/lib/fleetd/data.db ".backup '$BACKUP_DIR/data-$DATE.db'"
tar czf $BACKUP_DIR/binaries-$DATE.tar.gz /var/lib/fleetd/binaries
find $BACKUP_DIR -mtime +30 -delete
```

2. Add cron job:
```bash
echo "0 2 * * * root /usr/local/bin/fleetd-backup" > /etc/cron.d/fleetd-backup
```

### Recovery

1. Stop service:
```bash
systemctl stop fleetd
```

2. Restore database:
```bash
sqlite3 /var/lib/fleetd/data.db ".restore '/var/backups/fleetd/data-20231201.db'"
```

3. Restore binaries:
```bash
tar xzf /var/backups/fleetd/binaries-20231201.tar.gz -C /
```

4. Start service:
```bash
systemctl start fleetd
```

## Scaling

### Load Balancing

FleetD supports running multiple server instances behind a load balancer:

1. Configure load balancer (e.g., nginx):
```nginx
upstream fleetd {
    server fleetd1:8080;
    server fleetd2:8080;
}

server {
    listen 443 ssl http2;
    server_name fleetd.example.com;

    ssl_certificate /etc/nginx/ssl/server.crt;
    ssl_certificate_key /etc/nginx/ssl/server.key;

    location / {
        grpc_pass grpc://fleetd;
    }
}
```

2. Configure each server instance with unique storage paths.

### High Availability

For high availability:
1. Use replicated storage (e.g., replicated SQLite or PostgreSQL)
2. Deploy multiple server instances
3. Use DNS-based failover or load balancing
4. Monitor instance health with Prometheus alerts

## Troubleshooting

### Common Issues

1. Agent can't connect to server:
- Check network connectivity
- Verify TLS certificates
- Check firewall rules

2. Database errors:
- Check disk space
- Verify permissions
- Check for corruption: `sqlite3 data.db "PRAGMA integrity_check;"`

3. Binary upload failures:
- Check disk space
- Verify storage permissions
- Check binary size limits

### Debug Mode

Enable debug logging:
```yaml
logging:
  level: debug
  format: text # More readable for debugging
```

Collect debug information:
```bash
fleetd-server debug-info > debug.txt
```

### Support

For additional support:
1. Check documentation: https://docs.fleetd.sh
2. GitHub issues: https://github.com/fleetd/fleetd/issues
3. Community forum: https://discuss.fleetd.sh 