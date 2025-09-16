# Cloud Feature Separation Plan

## Immediate Recommendation

Based on FleetD's current state, here's what I recommend:

### ✅ Keep Open Source
- **Core agent functionality** - The heart of fleet management
- **Device discovery & registration** - Essential for any deployment
- **Basic API/gRPC endpoints** - Standard fleet operations
- **Single PostgreSQL backend** - Simple, self-hostable
- **JWT authentication** - Standard auth
- **Prometheus metrics** - Observability basics
- **Update system** - Core functionality
- **CLI tools** - Management interface

### 🔒 Move to Proprietary Cloud Package

The features we just implemented should be cloud-only:

```
/cloud/                          # Gitignored in OSS repo
├── sharding/                    # Multi-tenant DB routing
│   ├── router.go               # ShardRouter implementation
│   ├── health.go               # Health monitoring
│   └── migrations/             # Control plane schemas
├── billing/                    # Subscription management
│   ├── tier_manager.go         # Tier limits & quotas
│   ├── usage_tracker.go        # Usage metering
│   └── stripe_integration.go   # Payment processing
├── timescale/                  # Time-series optimization
│   ├── metrics_service.go      # Hypertables & aggregates
│   └── retention_policies.go   # Data lifecycle
└── multitenancy/               # Tenant isolation
    ├── rbac.go                 # Fine-grained permissions
    └── row_security.go         # RLS policies
```

## Why This Split Makes Sense

### Business Perspective
- **Open Source**: Drives adoption, builds community, establishes standard
- **Cloud**: Generates revenue from "boring" operational complexity
- **No Lock-in**: Users can self-host if they need to

### Technical Perspective
- **Clean Interfaces**: Use dependency injection and interfaces
- **No Feature Flags**: Build tags keep code paths clean
- **Clear Boundaries**: Cloud features are truly separate concerns

## Implementation Steps

### Step 1: Run Separation Script
```bash
chmod +x scripts/separate-cloud-features.sh
./scripts/separate-cloud-features.sh
```

### Step 2: Create Two Git Repos
```bash
# Public repo (current)
git remote add opensource https://github.com/fleetdsh/fleetd.git

# Private repo (new)
git remote add cloud https://github.com/fleetdsh/fleetd-cloud-private.git
```

### Step 3: Set Up Build Process
```makefile
# Open source build (no cloud features)
build-oss:
	go build -tags oss ./cmd/fleetd

# Cloud build (includes everything)
build-cloud:
	go build -tags cloud ./cmd/fleetd
```

### Step 4: Update Imports
Replace direct imports with interfaces:

```go
// Before (tight coupling)
import "fleetd.sh/internal/database/sharding"

// After (loose coupling)
import "fleetd.sh/internal/extensions"

type Server struct {
    storage extensions.StorageRouter  // Interface
}
```

## Testing Strategy

### Open Source Tests
```bash
go test -tags oss ./...  # Ensures OSS build works without cloud
```

### Cloud Tests
```bash
go test -tags cloud ./cloud/...  # Tests cloud features
```

## Documentation Strategy

### Public Docs (open source)
- Installation guide
- API reference
- Single-tenant deployment
- Contributing guide

### Private Docs (cloud)
- Multi-tenant architecture
- Sharding operations
- Billing integration
- SRE runbooks

## Revenue Model

| Edition | Target | Pricing | Features |
|---------|--------|---------|----------|
| **Open Source** | Hobbyists, Small teams | Free | Full single-tenant |
| **Cloud Starter** | Startups | $99/mo | 10 devices, shared hosting |
| **Cloud Pro** | Growing companies | $499/mo | 100 devices, dedicated shard |
| **Cloud Enterprise** | Large orgs | Custom | Unlimited, dedicated infra |
| **Enterprise Self-Hosted** | Regulated industries | $20k/yr | Support + advanced features |

## Similar Successful Models

- **Grafana**: OSS dashboards → Grafana Cloud
- **GitLab**: OSS Git → GitLab.com
- **Elastic**: OSS search → Elastic Cloud
- **Confluent**: Kafka → Confluent Cloud

## Decision Point

**Option A: Keep Everything Together** (Not Recommended)
- Use build tags in same repo
- Risk: Accidental exposure of cloud code
- Benefit: Easier development

**Option B: Separate Repositories** (Recommended)
- Clear separation of concerns
- Different access controls
- Clean licensing

## Next Actions

1. **Today**: Move cloud features to `/cloud` directory
2. **This Week**: Set up private repo for cloud code
3. **Next Week**: Update CI/CD for dual builds
4. **Month 1**: Launch open source with announcement
5. **Month 2**: Beta test cloud offering
6. **Month 3**: GA cloud launch

## Questions to Consider

1. **Licensing**: Apache 2.0, MIT, or AGPL for OSS?
2. **CLA**: Require contributor agreement?
3. **Trademark**: Protect "FleetD" name?
4. **Support**: Community forum vs paid support tiers?
5. **Hosting**: AWS/GCP/Azure or multi-cloud?

The key is making the open-source version genuinely useful while keeping the operational complexity (multi-tenancy, billing, sharding) as your proprietary value-add.