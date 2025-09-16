# Fleet CLI Consolidation Plan

## Overview

Consolidate all Fleet tooling into a single, developer-friendly CLI (`fleet`) similar to Supabase CLI, while keeping the agent binary separate and lightweight.

## Current State vs Future State

### Current State (Fragmented)
```bash
fleets server --port 8080        # Server management
fleetp -device /dev/disk2        # Device provisioning
fleetd agent --server localhost  # Agent (keep separate)
```

### Future State (Unified)
```bash
fleet start                       # Start local stack
fleet provision --device /dev/disk2  # Provision devices
fleet db migrate                  # Run migrations
fleet deploy                      # Deploy to production
fleetd agent --server localhost  # Agent (remains separate)
```

## Architecture

```
fleet (CLI)
├── cmd/
│   ├── root.go          # Main entry point
│   ├── start.go         # Local development
│   ├── stop.go
│   ├── status.go
│   ├── provision.go     # Device provisioning
│   ├── db.go            # Database operations
│   ├── deploy.go        # Deployment
│   ├── login.go         # Auth for managed service
│   ├── init.go          # Project initialization
│   └── config.go        # Configuration management
├── internal/
│   ├── docker/          # Docker compose management
│   ├── provision/       # Provisioning logic (from fleetp)
│   ├── config/          # Config file handling
│   ├── api/             # API client for remote servers
│   └── ui/              # Terminal UI components
└── config.toml          # Project configuration
```

## Command Structure

### Development Commands
```bash
# Start all services locally (like 'supabase start')
fleet start [--exclude postgres,clickhouse]

# Stop all services
fleet stop

# View status
fleet status

# View logs
fleet logs [service] [--follow]

# Reset everything
fleet reset
```

### Provisioning Commands (from fleetp)
```bash
# List available devices
fleet provision list

# Provision a device
fleet provision --device /dev/disk2 --wifi-ssid "Network" --wifi-pass "pass"

# Provision with custom image
fleet provision --device /dev/disk2 --image custom.img
```

### Database Commands
```bash
# Run migrations
fleet db migrate

# Create new migration
fleet db create-migration "add_indexes"

# Reset database
fleet db reset

# Seed data
fleet db seed
```

### Project Management
```bash
# Initialize new project
fleet init

# Link to existing project
fleet link --project-id abc123

# Generate types from proto
fleet generate types

# Run linting and formatting
fleet lint
fleet format
```

### Deployment Commands
```bash
# Login to Fleet Cloud (future)
fleet login

# Deploy to staging/production
fleet deploy --env production

# View deployments
fleet deployments list

# Rollback
fleet rollback
```

### Configuration Commands
```bash
# Set config value
fleet config set api.url https://api.fleet.example.com

# Get config value
fleet config get api.url

# List all config
fleet config list
```

## Configuration File (config.toml)

Similar to Supabase's approach:

```toml
# config.toml
[project]
id = "fleet_project_123"
name = "my-fleet"

[api]
enabled = true
port = 8080
url = "http://localhost:8080"

[db]
port = 5432
host = "localhost"
name = "fleetd"
user = "fleetd"
password = "fleetd_secret"

[stack]
# Services to start with 'fleet start'
services = [
  "postgres",
  "victoriametrics",
  "loki",
  "clickhouse",
  "valkey",
  "traefik"
]

[gateway]
port = 80
dashboard_port = 8080

[telemetry]
victoria_metrics_port = 8428
loki_port = 3100
grafana_port = 3001

[auth]
jwt_secret = "your-secret-here"
api_keys = []

[provisioning]
default_image = "raspios-lite"
default_user = "pi"

# Environment-specific overrides
[environments.staging]
api.url = "https://staging.fleet.example.com"

[environments.production]
api.url = "https://api.fleet.example.com"
```

## Implementation Plan

### Phase 1: Core CLI Structure (Week 1)
1. Create new `cmd/fleet` directory
2. Set up Cobra command structure
3. Implement config.toml parsing
4. Basic commands: start, stop, status

### Phase 2: Migrate Existing Tools (Week 2)
1. Port provisioning logic from fleetp
2. Integrate server commands from fleets
3. Maintain backward compatibility

### Phase 3: Developer Experience (Week 3)
1. Add colored output and spinners
2. Interactive prompts for complex commands
3. Progress bars for long operations
4. Helpful error messages

### Phase 4: Advanced Features (Week 4)
1. Project templates
2. Code generation
3. Remote server management
4. CI/CD integration

## Developer Experience Features

### 1. Smart Defaults
```bash
# Automatically detects project root
fleet start  # Works from any subdirectory

# Sensible defaults for common operations
fleet provision  # Interactive device selection
```

### 2. Helpful Output
```bash
$ fleet start
Starting Fleet development stack...
  ✓ PostgreSQL      (5432)
  ✓ VictoriaMetrics (8428)
  ✓ Loki           (3100)
  ✓ Valkey         (6379)
  ✓ Traefik        (80, 443)

Fleet is ready! Access at:
  • Dashboard: http://localhost:8080
  • API:       http://localhost/api
  • Metrics:   http://localhost:3001

Run 'fleet status' to check services.
```

### 3. Interactive Mode
```bash
$ fleet init
? Project name: my-fleet
? Enable authentication? Yes
? Select data backends:
  ✓ PostgreSQL
  ✓ VictoriaMetrics
  ✓ Loki
  ◯ ClickHouse

Creating project configuration...
✓ Generated config.toml
✓ Created docker-compose.yml
✓ Initialized git repository

Run 'fleet start' to begin development.
```

### 4. Context Awareness
```bash
# Automatically uses correct environment
fleet deploy  # Uses staging if on staging branch
fleet deploy --env production  # Explicit override
```

## Benefits

### For Developers
- Single tool to learn
- Consistent interface
- Better discoverability
- Reduced cognitive load

### For Maintenance
- Single codebase
- Shared utilities
- Consistent updates
- Better testing

### For Users
- Simpler installation
- Better documentation
- Unified experience
- Easier onboarding

## Migration Strategy

### Phase 1: Parallel Operation
- New `fleet` CLI alongside existing tools
- Feature parity with existing tools
- Documentation for migration

### Phase 2: Deprecation
- Mark old tools as deprecated
- Add migration warnings
- Provide migration guide

### Phase 3: Removal
- Remove old tools in major version
- Complete migration documentation
- Support scripts for transition

## Success Metrics

- **Adoption**: 80% of users on new CLI within 3 months
- **Developer Satisfaction**: Positive feedback on UX
- **Time to First Success**: <5 minutes from install to running stack
- **Support Tickets**: Reduction in CLI-related issues

## Comparison with Supabase CLI

| Feature | Supabase | Fleet (Proposed) |
|---------|----------|------------------|
| Single Binary | ✓ | ✓ |
| Local Development | `supabase start` | `fleet start` |
| Configuration | config.toml | config.toml |
| Project Init | `supabase init` | `fleet init` |
| Migrations | `supabase db migrate` | `fleet db migrate` |
| Type Generation | `supabase gen types` | `fleet generate types` |
| Multi-environment | ✓ | ✓ |
| Interactive Mode | ✓ | ✓ |
| Docker Integration | ✓ | ✓ |

## Next Steps

1. Approve consolidation plan
2. Create new CLI structure
3. Implement core commands
4. Test with users
5. Document and release