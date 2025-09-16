# FleetD Open Source Strategy

## Repository Structure

### Option 1: Monorepo with Build Tags (Recommended)
Keep everything in one repo but use Go build tags to separate:

```go
// +build cloud

package billing
```

**Pros:**
- Easier development and testing
- Shared CI/CD pipeline
- Better code reuse

**Cons:**
- Need careful build processes
- Risk of accidentally including cloud code

### Option 2: Separate Repositories
```
fleetd/              # Open source core
fleetd-cloud/        # Proprietary cloud extensions
```

**Pros:**
- Clear separation
- No risk of leaking proprietary code
- Different access controls

**Cons:**
- Harder to maintain
- Code duplication
- Complex dependency management

## Implementation Approach

### 1. Move Cloud Features to Separate Package
```bash
# Create cloud-specific directory (gitignored in OSS repo)
/cloud/
  ├── billing/
  ├── sharding/
  ├── multitenancy/
  └── enterprise/
```

### 2. Use Interfaces for Extension Points
```go
// In open-source code
type StorageBackend interface {
    Store(ctx context.Context, data []byte) error
    Retrieve(ctx context.Context, id string) ([]byte, error)
}

// In cloud code
type ShardedStorageBackend struct {
    router *ShardRouter
}
```

### 3. License Structure
- **Open Source**: Apache 2.0 or MIT for core
- **Cloud**: Proprietary license for cloud extensions
- **Dual License**: Optional enterprise self-hosted license

## Feature Comparison Table

| Feature | Open Source | Cloud | Enterprise Self-Hosted |
|---------|------------|-------|------------------------|
| Core Fleet Management | ✅ | ✅ | ✅ |
| Basic Auth | ✅ | ✅ | ✅ |
| Single Tenant | ✅ | ✅ | ✅ |
| Prometheus Metrics | ✅ | ✅ | ✅ |
| Multi-Tenant | ❌ | ✅ | ❌ |
| Sharding | ❌ | ✅ | ❌ |
| Billing/Quotas | ❌ | ✅ | ❌ |
| SSO/SAML | ❌ | ✅ | ✅ |
| Advanced RBAC | ❌ | ✅ | ✅ |
| SLA Monitoring | ❌ | ✅ | ✅ |
| TimescaleDB | ❌ | ✅ | Optional |
| Audit Logs | ❌ | ✅ | ✅ |
| Support | Community | 24/7 | Business Hours |

## Migration Path

### Phase 1: Extract Cloud Features (Current)
Move the recently implemented features to a separate package:
```bash
git mv internal/database/sharding cloud/sharding
git mv internal/billing cloud/billing
git mv internal/database/timescale cloud/timescale
```

### Phase 2: Create Plugin System
```go
// pkg/plugins/registry.go
type Plugin interface {
    Name() string
    Init(config map[string]any) error
}

var registry = make(map[string]Plugin)

func Register(p Plugin) {
    registry[p.Name()] = p
}
```

### Phase 3: Build Systems
```makefile
# Open source build
build-oss:
    go build -tags oss ./cmd/fleetd

# Cloud build (includes everything)
build-cloud:
    go build -tags cloud ./cmd/fleetd
```

## Business Model Considerations

### Why This Split Works

1. **Community Adoption**: Full-featured open source drives adoption
2. **Cloud Revenue**: Multi-tenancy, managed service, SLAs
3. **Enterprise Revenue**: Support, compliance, advanced features
4. **No Lock-in**: Users can self-host if needed

### Common Pitfalls to Avoid

1. **Don't cripple open source**: It should be fully functional for single-tenant use
2. **Don't open source differentiators**: Keep cloud operations IP private
3. **Don't mix licenses**: Clear separation prevents legal issues
4. **Don't neglect community**: Open source needs active maintenance

## Recommended Next Steps

1. **Move cloud features** to `/cloud` directory
2. **Add .gitignore** for cloud directory in public repo
3. **Create interfaces** for extension points
4. **Set up build tags** for conditional compilation
5. **Document clearly** what's available in each edition
6. **Consider CLA** for contributions to maintain flexibility

## Examples from Successful Projects

- **GitLab**: Open source core, proprietary enterprise features
- **Elastic**: Open source search, proprietary security/ML
- **Grafana**: Open source dashboards, Grafana Cloud for hosting
- **Consul**: Open source service mesh, Consul Enterprise for advanced features

## Legal Considerations

- Use CLA (Contributor License Agreement) for flexibility
- Keep cloud code in separate repo or build-tagged
- Clear documentation of what's open vs closed
- Consider trademark protection for brand
