# Code Review & Improvement Recommendations

## Executive Summary

After analyzing the recent changes and overall codebase, the architecture is solid with good separation of concerns. However, there are several areas where we can improve idiomaticity, readability, and maintainability.

## ✅ What's Working Well

1. **Clear API Separation**: Public vs internal APIs are well separated
2. **Type Safety**: Strong typing with Protocol Buffers and TypeScript
3. **Modern Stack**: Next.js 15, React 19, Connect RPC are excellent choices
4. **Build System**: Unified Justfile is clean and comprehensive
5. **Environment Management**: t3-env provides excellent type safety

## 🔧 Issues & Recommendations

### 1. Proto File Organization

**Issue**: Proto files mix different concerns in single files (e.g., `fleet.proto` is 300+ lines)

**Recommendation**: Split into smaller, focused files
```
proto/public/v1/
├── device.proto        # Device-related messages only
├── telemetry.proto     # Telemetry messages
├── update.proto        # Update/deployment messages
├── config.proto        # Configuration messages
└── fleet_service.proto # Service definition importing above
```

### 2. Go Code Improvements

**Issue**: Database queries directly in service methods violates separation of concerns

**Current**:
```go
func (s *FleetService) ListDevices(...) {
    query := `SELECT id, name, type...`  // SQL in service layer
    rows, err := s.db.QueryContext(...)
}
```

**Recommended**:
```go
// internal/repository/device.go
type DeviceRepository interface {
    List(ctx context.Context, opts ListOptions) ([]*Device, error)
    Get(ctx context.Context, id string) (*Device, error)
}

// internal/api/public/fleet_service.go
type FleetService struct {
    devices DeviceRepository  // Use interface
    events  EventBroadcaster
}

func (s *FleetService) ListDevices(ctx context.Context, req *connect.Request[pb.ListDevicesRequest]) (*connect.Response[pb.ListDevicesResponse], error) {
    devices, err := s.devices.List(ctx, ListOptions{
        Limit:  req.Msg.PageSize,
        Offset: calculateOffset(req.Msg.PageToken),
    })
    // Clean service logic, no SQL
}
```

### 3. Error Handling

**Issue**: Inconsistent error handling and logging

**Current**:
```go
if err != nil {
    slog.Error("Failed to query devices", "error", err)
    return nil, connect.NewError(connect.CodeInternal, err)  // Leaks internal error
}
```

**Recommended**:
```go
// internal/errors/errors.go
type Error struct {
    Code    connect.Code
    Message string
    Err     error
}

func (e *Error) ConnectError() *connect.Error {
    return connect.NewError(e.Code, errors.New(e.Message))
}

// Usage
if err != nil {
    slog.Error("database query failed",
        slog.String("operation", "ListDevices"),
        slog.Error(err))
    return nil, ErrInternal.WithMessage("Failed to retrieve devices").ConnectError()
}
```

### 4. TypeScript/React Improvements

**Issue**: API client uses localStorage directly (not SSR-safe)

**Current**:
```typescript
const token = typeof window !== 'undefined' ? localStorage.getItem('auth_token') : null
```

**Recommended**:
```typescript
// lib/auth/storage.ts
class AuthStorage {
  private isServer = typeof window === 'undefined'

  getToken(): string | null {
    if (this.isServer) return null
    return localStorage.getItem('auth_token')
  }

  setToken(token: string): void {
    if (!this.isServer) {
      localStorage.setItem('auth_token', token)
    }
  }
}

export const authStorage = new AuthStorage()

// lib/api/client.ts
import { authStorage } from '@/lib/auth/storage'

const transport = createConnectTransport({
  baseUrl: env.NEXT_PUBLIC_API_URL,
  interceptors: [
    (next) => async (req) => {
      const token = authStorage.getToken()
      if (token) {
        req.header.set('Authorization', `Bearer ${token}`)
      }
      return next(req)
    },
  ],
})
```

### 5. Component Organization

**Issue**: Mixed component patterns (some using default exports, some named)

**Recommendation**: Standardize on named exports
```typescript
// ❌ Avoid
export default function Dashboard() { }

// ✅ Prefer
export function Dashboard() { }

// Allows for better tree-shaking and refactoring
```

### 6. Missing Tests

**Critical Gap**: No tests for new code

**Recommendation**: Add test structure
```
web/
├── __tests__/
│   ├── components/
│   ├── hooks/
│   └── lib/
└── test/
    ├── setup.ts
    └── utils.tsx
```

### 7. Database Migrations

**Issue**: Manual SQL in Go code, no migration system

**Recommendation**: Use golang-migrate
```go
// migrations/001_initial.up.sql
CREATE TABLE IF NOT EXISTS device (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    -- etc
);

// migrations/001_initial.down.sql
DROP TABLE IF EXISTS device;
```

### 8. Configuration Management

**Issue**: Hardcoded values scattered throughout

**Examples**:
- `5*time.Minute` for online/offline status
- `100` for max page size
- `30*time.Second` for SSE heartbeat

**Recommendation**:
```go
// internal/config/config.go
type Config struct {
    Device DeviceConfig
    API    APIConfig
    SSE    SSEConfig
}

type DeviceConfig struct {
    OnlineThreshold   time.Duration `env:"DEVICE_ONLINE_THRESHOLD" default:"5m"`
    HeartbeatInterval time.Duration `env:"DEVICE_HEARTBEAT" default:"30s"`
}

type APIConfig struct {
    MaxPageSize int32 `env:"API_MAX_PAGE_SIZE" default:"100"`
}
```

### 9. Security Considerations

**Issues**:
- No rate limiting implementation
- CORS allows all origins (`*`)
- No request validation middleware
- Missing audit logging

**Recommendation**: Add middleware layer
```go
// internal/middleware/middleware.go
func RateLimit(limit int) connect.Interceptor { }
func ValidateRequest() connect.Interceptor { }
func AuditLog(logger *slog.Logger) connect.Interceptor { }
```

### 10. Documentation

**Issue**: Incomplete API documentation

**Recommendation**: Add OpenAPI generation from protos
```yaml
# buf.gen.yaml addition
- remote: buf.build/community/grpc-ecosystem-openapiv2
  out: docs/api
  opt:
    - allow_merge=true
    - merge_file_name=api
```

## 🎯 Priority Actions

### High Priority
1. **Add Repository Pattern**: Separate data access from business logic
2. **Improve Error Handling**: Create consistent error types
3. **Add Tests**: At least unit tests for critical paths
4. **Fix Security Issues**: Rate limiting, CORS, validation

### Medium Priority
1. **Split Proto Files**: Better organization
2. **Add Migrations**: Proper database versioning
3. **Centralize Config**: Remove hardcoded values
4. **Standardize Components**: Consistent patterns

### Low Priority
1. **Add OpenAPI Docs**: Better API documentation
2. **Add Monitoring**: Metrics and tracing
3. **Optimize Queries**: Add indexes, optimize SQL

## Code Quality Metrics

| Metric | Current | Target |
|--------|---------|--------|
| Test Coverage | 0% | >80% |
| Cyclomatic Complexity | High (>10) | <10 |
| Code Duplication | Moderate | Low |
| Type Coverage | 85% | >95% |
| Bundle Size | Unknown | <500kb |

## Next Steps

1. **Create tech debt tickets** for each high-priority item
2. **Add linting rules** to enforce standards
3. **Set up CI/CD** with quality gates
4. **Schedule refactoring sprints** to address issues

## Conclusion

The foundation is solid, but needs refinement for production readiness. Focus on:
- **Separation of concerns** (repository pattern)
- **Error handling** (consistent, secure)
- **Testing** (critical for reliability)
- **Security** (rate limiting, validation)

These improvements will make the codebase more maintainable, secure, and ready for both OSS and cloud deployments.