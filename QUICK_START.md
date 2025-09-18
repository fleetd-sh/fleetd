# fleetd Quick Start Guide

## Option 1: Local Development

Get fleetd running locally in under 2 minutes:

```bash
# Start all services
./bin/fleetctl start

# Check health
curl http://localhost:8090/health
curl http://localhost:8081/health

# View logs
./bin/fleetctl logs platform-api

# Access services:
# - Platform API: http://localhost:8090
# - Device API: http://localhost:8081
# - Web Dashboard: http://localhost:3000
# - VictoriaMetrics: http://localhost:8428
# - Loki: http://localhost:3100
# - Traefik Dashboard: http://localhost:8080
```

## Option 2: Single Server Deployment

For small deployments on a single VPS:

```bash
# On Ubuntu 22.04 server
# 1. Install Docker and Go
curl -fsSL https://get.docker.com | sh
wget https://go.dev/dl/go1.21.5.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.21.5.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin

# 2. Clone repository
git clone https://github.com/yourusername/fleetd.git
cd fleetd

# 3. Build binaries
just build-all

# 4. Configure for production
cp config.toml config-production.toml
# Edit config-production.toml with your values

# 5. Start services
FLEETD_AUTH_MODE=production ./bin/fleetctl start

# 6. Setup nginx reverse proxy
sudo apt install nginx certbot python3-certbot-nginx
# Configure nginx and SSL
```

## Testing the Deployment

```bash
# Register a device
./bin/fleetctl device register \
  --server http://localhost:8081 \
  --name "test-device-01"

# List devices
./bin/fleetctl devices list

# Check metrics
curl http://localhost:8090/metrics

# View device API health
curl http://localhost:8081/health
```

## Next Steps

1. **Security**: Change default passwords and secrets
2. **Monitoring**: Access Grafana at http://localhost:3000
3. **Production**: Review [Production Deployment](wiki/production_deployment.md)

## Troubleshooting

### Services not starting
```bash
# Check logs
./bin/fleetctl logs postgres
./bin/fleetctl logs platform-api

# Check Docker containers
docker ps -a | grep fleetd

# Reset everything
./bin/fleetctl stop --volumes
./bin/fleetctl start --reset
```

### Database connection issues
```bash
# Check postgres is running
docker ps | grep fleetd-postgres

# Test connection
docker exec fleetd-postgres psql -U fleetd -d fleetd -c "SELECT 1"
```

### Port conflicts
```bash
# Check what's using ports
sudo lsof -i :8090
sudo lsof -i :8081
sudo lsof -i :5432

# Stop conflicting services or use different ports
# Edit config.toml to change port assignments
```

## Support

- Documentation: [Wiki](./wiki)
- Issues: [GitHub Issues](https://github.com/yourusername/fleetd/issues)
- Community: [Discord](https://discord.gg/fleetd)