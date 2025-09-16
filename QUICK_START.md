# fleetd Quick Start Guide

##  One-Command Platform Launch

Start the entire fleetd platform with a single command (similar to Supabase):

```bash
fleetctl start
```

This command will:
1.  Generate secure secrets (if not exists)
2.  Create `config.toml` configuration
3.  Start all infrastructure services (PostgreSQL, VictoriaMetrics, Loki, Valkey, Traefik)
4.  Launch Fleet server (API)
5.  Start Web UI (Dashboard)
6.  Display all URLs and credentials

##  Prerequisites

- Docker and Docker Compose installed
- Go 1.21+ (for local development)
- Node.js 20+ (for web UI)

##  Complete Workflow for Raspberry Pi k3s Cluster

### 1. Start the Platform
```bash
# Clone the repository
git clone https://github.com/fleetd/fleetd.git
cd fleetd

# Build the CLI
go build -o fleetctl ./cmd/fleetctl

# Start everything
./fleetctl start
```

You'll see output like:
```
 fleetd Platform is running!

 Service URLs
  • Dashboard:         http://localhost:3000
  • API:               http://localhost:8080
  • API Gateway:       http://localhost:80
  • Metrics (Victoria): http://localhost:8428
  • Logs (Loki):       http://localhost:3100
  • Traefik Dashboard: http://localhost:8090

 Credentials
  • JWT Secret:    AbC3...xYz9 (first/last 4 chars)
  • API Key:       DeF4...uVw8
  • Grafana:       admin / <generated-password>

 Configuration
  • Config:        ./config.toml
  • Secrets:       ./.env
  • Database:      ./fleet.db
```

### 2. Provision SD Cards for k3s

**For k3s Server (Control Plane):**
```bash
./fleetctl provision \
  --device /dev/disk2 \
  --name "k3s-server-01" \
  --wifi-ssid "YourWiFi" \
  --wifi-pass "YourPassword" \
  --plugin k3s \
  --plugin-opt k3s.role=server \
  --fleet-server http://<your-machine-ip>:8080
```

**For k3s Worker Node:**
```bash
./fleetctl provision \
  --device /dev/disk3 \
  --name "k3s-worker-01" \
  --wifi-ssid "YourWiFi" \
  --wifi-pass "YourPassword" \
  --plugin k3s \
  --plugin-opt k3s.role=agent \
  --plugin-opt k3s.server=https://k3s-server-01.local:6443 \
  --plugin-opt k3s.token=<k3s-join-token> \
  --fleet-server http://<your-machine-ip>:8080
```

### 3. Boot Raspberry Pis

1. Insert SD cards into Raspberry Pis
2. Power on the devices
3. Wait ~2-3 minutes for boot and network connection

### 4. Discover & Manage via Dashboard

Open http://localhost:3000 in your browser:

1. Click **"Discover Devices"** - fleetd will scan the network via mDNS
2. Discovered devices auto-register and start reporting telemetry
3. View real-time metrics: CPU, Memory, Disk usage
4. Check k3s cluster status
5. Deploy applications to your cluster

### 5. Verify k3s Cluster

Once devices are registered:
```bash
# SSH into k3s server (if SSH key was configured)
ssh pi@k3s-server-01.local

# Check cluster status
sudo k3s kubectl get nodes
```

## Advanced Options

### Custom Start Options
```bash
# Start without web UI
fleetctl start --no-web

# Start without Fleet server (infrastructure only)
fleetctl start --no-server

# Exclude specific services
fleetctl start --exclude clickhouse,grafana

# Reset all data and regenerate secrets
fleetctl start --reset

# Use specific port for API
fleetctl start --expose-port 9090
```

### Manual Service Management
```bash
# Check status
fleetctl status

# View logs
fleetctl logs fleets
fleetctl logs web

# Stop everything
fleetctl stop

# Reset (careful - deletes all data!)
fleetctl reset
```

##  What's Running?

| Service | Purpose | URL |
|---------|---------|-----|
| fleets | Management API Server | http://localhost:8080 |
| Web Dashboard | Management UI | http://localhost:3000 |
| PostgreSQL | Primary database | localhost:5432 |
| VictoriaMetrics | Time-series metrics | http://localhost:8428 |
| Loki | Log aggregation | http://localhost:3100 |
| Valkey | Caching & rate limiting | localhost:6379 |
| Traefik | API Gateway | http://localhost:80 |
| Grafana | Monitoring (optional) | http://localhost:3001 |

##  Security Notes

- All secrets are auto-generated and stored in `.env`
- Never commit `.env` or `config.toml` to version control
- Default setup is for development - use proper TLS in production
- Change default passwords before deploying to production

##  Troubleshooting

### Services not starting?
```bash
# Check Docker
docker ps
docker-compose logs -f

# Check specific service
fleetctl logs postgres
fleetctl logs fleets
```

### Can't discover devices?
- Ensure all devices are on the same network
- Check firewall allows mDNS (port 5353)
- Verify Fleet server is accessible from Pi network

### Web UI not loading?
```bash
# Check if web container is running
docker ps | grep web

# Check web logs
docker logs fleetd-web

# Rebuild if needed
cd web && npm install && npm run dev
```

##  Success!

You now have:
-  Complete fleetd platform running locally
-  SD cards provisioned with fleetd agent + k3s
-  Automatic device discovery and registration
-  Real-time telemetry and monitoring
-  k3s cluster ready for deployments

## Next Steps

1. Deploy your first application to k3s
2. Set up monitoring dashboards in Grafana
3. Configure alerts for device health
4. Explore the Fleet API for automation

For more details, see the [full documentation](https://github.com/fleetd-sh/fleetd/wiki).