# FleetD Production Deployment Guide

## Prerequisites

- Go 1.21+ installed
- PostgreSQL 14+ or compatible database
- Redis/Valkey for caching (optional)
- Docker & Docker Compose (for containerized deployment)

## Security Configuration

### TLS/mTLS Setup

FleetD supports three TLS modes:

1. **Auto-generated certificates** (default)
   ```yaml
   tls:
     mode: tls  # or mtls for mutual TLS
     auto_generate: true
     organization: "Your Company"
     common_name: "fleetd.yourdomain.com"
     hosts:
       - localhost
       - fleetd.yourdomain.com
       - "*.yourdomain.com"
     valid_days: 365
     cert_dir: /etc/fleetd/certs
   ```

2. **Custom certificates**
   ```yaml
   tls:
     mode: tls
     auto_generate: false
     cert_file: /path/to/server.crt
     key_file: /path/to/server.key
     ca_file: /path/to/ca.crt  # Required for mTLS
   ```

3. **No TLS** (not recommended for production)
   ```yaml
   tls:
     mode: none
   ```

### JWT Configuration

```yaml
jwt:
  signing_key: "your-256-bit-secret-key-here"  # Generate with: openssl rand -hex 32
  expiration: 3600  # 1 hour
  refresh_expiration: 2592000  # 30 days
```

## Database Setup

1. Create database:
   ```sql
   CREATE DATABASE fleetd;
   CREATE USER fleetd WITH ENCRYPTED PASSWORD 'your-password';
   GRANT ALL PRIVILEGES ON DATABASE fleetd TO fleetd;
   ```

2. Run migrations:
   ```bash
   export DATABASE_URL="postgres://fleetd:password@localhost/fleetd?sslmode=require"
   ./platform-api migrate up
   ```

## Service Deployment

### Using Docker Compose

1. Update `docker-compose.yml` with production values:
   ```yaml
   services:
     platform-api:
       environment:
         - ENVIRONMENT=production
         - DATABASE_URL=${DATABASE_URL}
         - JWT_SIGNING_KEY=${JWT_SIGNING_KEY}
         - TLS_MODE=tls
         - TLS_AUTO_GENERATE=true
   ```

2. Deploy:
   ```bash
   docker-compose up -d
   ```

### Using SystemD

1. Create service file `/etc/systemd/system/fleetd-platform.service`:
   ```ini
   [Unit]
   Description=FleetD Platform API
   After=network.target postgresql.service
   
   [Service]
   Type=simple
   User=fleetd
   Group=fleetd
   WorkingDirectory=/opt/fleetd
   Environment="ENVIRONMENT=production"
   Environment="DATABASE_URL=postgres://..."
   ExecStart=/opt/fleetd/platform-api
   Restart=always
   RestartSec=10
   
   [Install]
   WantedBy=multi-user.target
   ```

2. Enable and start:
   ```bash
   systemctl enable fleetd-platform
   systemctl start fleetd-platform
   ```

## Health Checks

- Platform API: `https://your-domain:8090/health`
- Device API: `https://your-domain:8080/health`
- Metrics: `http://your-domain:9090/metrics`

## Monitoring

### Request Tracing

All requests include `X-Request-ID` header for tracing across services.

### Logs

Structured JSON logs are output to stdout/stderr. Configure log aggregation:

```yaml
logging:
  level: info  # debug, info, warn, error
  format: json
  output: stdout
```

### Metrics

Prometheus metrics available at `/metrics` endpoint:

- `fleetd_http_requests_total`
- `fleetd_http_request_duration_seconds`
- `fleetd_device_connections`
- `fleetd_auth_attempts_total`

## Security Checklist

- [ ] TLS enabled for all services
- [ ] Strong JWT signing key configured
- [ ] Database connections use SSL
- [ ] Rate limiting enabled
- [ ] CORS properly configured
- [ ] Authentication required on all endpoints
- [ ] Secrets stored in environment variables or secret manager
- [ ] Regular security updates applied
- [ ] Audit logging enabled
- [ ] Firewall rules configured

## Backup & Recovery

1. **Database backups**:
   ```bash
   pg_dump fleetd > fleetd_backup_$(date +%Y%m%d).sql
   ```

2. **Certificate backup** (if using custom certs):
   ```bash
   tar czf certs_backup.tar.gz /etc/fleetd/certs
   ```

## Scaling

### Horizontal Scaling

1. Platform API: Stateless, scale behind load balancer
2. Device API: Use sticky sessions for WebSocket connections
3. Database: Configure read replicas for read-heavy workloads

### Load Balancer Configuration

Example HAProxy configuration:

```
frontend fleetd_https
    bind *:443 ssl crt /etc/ssl/fleetd.pem
    mode http
    default_backend platform_api

backend platform_api
    balance roundrobin
    server api1 10.0.1.10:8090 check ssl verify none
    server api2 10.0.1.11:8090 check ssl verify none
```

## Troubleshooting

### Common Issues

1. **TLS certificate errors**:
   - Check certificate expiration: `openssl x509 -in server.crt -text -noout`
   - Verify hostname matches: `openssl s_client -connect host:port`

2. **Authentication failures**:
   - Check JWT configuration
   - Verify token expiration settings
   - Review auth logs

3. **Database connection issues**:
   - Test connection: `psql $DATABASE_URL`
   - Check SSL requirements
   - Verify network connectivity

### Debug Mode

Enable debug logging:
```bash
export LOG_LEVEL=debug
export TLS_DEBUG=true
./platform-api
```

## Support

For production support:
- Documentation: https://docs.fleetd.sh
- Issues: https://github.com/fleetd/fleetd/issues
- Security: security@fleetd.sh
