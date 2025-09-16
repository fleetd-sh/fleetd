# FleetD Authentication Architecture

## Overview

FleetD implements a flexible, pluggable authentication system designed to support both open-source self-hosted deployments and managed cloud offerings. The architecture follows industry best practices with JWT tokens, refresh tokens, and support for multiple auth providers.

## Authentication Flow

```mermaid
sequenceDiagram
    participant User
    participant WebUI
    participant API
    participant AuthProvider
    participant Database
    participant Device

    Note over User,Device: User Authentication Flow

    User->>WebUI: Access Dashboard
    WebUI->>API: Check Auth Status
    API-->>WebUI: Unauthorized (401)
    WebUI->>User: Show Login Page

    User->>WebUI: Enter Credentials
    WebUI->>API: POST /auth/login
    API->>AuthProvider: Validate Credentials

    alt Built-in Auth
        AuthProvider->>Database: Check password hash
        Database-->>AuthProvider: User verified
    else SSO/OAuth
        AuthProvider->>AuthProvider: External IdP validation
        AuthProvider-->>API: User info + claims
    end

    API->>Database: Create session
    API->>API: Generate JWT tokens
    API-->>WebUI: Access + Refresh tokens
    WebUI->>WebUI: Store tokens (localStorage)
    WebUI->>User: Redirect to Dashboard

    Note over User,Device: Authenticated Requests

    User->>WebUI: View Devices
    WebUI->>API: GET /devices + Bearer token
    API->>API: Validate JWT
    API->>Database: Check permissions
    API->>Database: Fetch devices (filtered by org)
    API-->>WebUI: Device list
    WebUI->>User: Display devices

    Note over User,Device: Token Refresh Flow

    WebUI->>API: Request with expired token
    API-->>WebUI: Token expired (401)
    WebUI->>API: POST /auth/refresh
    API->>Database: Validate refresh token
    API->>API: Generate new access token
    API-->>WebUI: New tokens
    WebUI->>API: Retry original request

    Note over User,Device: Device Authentication

    Device->>API: Connect with API key
    API->>Database: Validate API key
    API->>Database: Check device permissions
    API-->>Device: Connection established
```

## Authentication Layers

### 1. User Authentication

```mermaid
graph TB
    subgraph "Authentication Methods"
        UP[Username/Password]
        AK[API Keys]
        SSO[SSO Providers]
        MFA[2FA/MFA]
    end

    subgraph "OSS Version"
        UP --> BA[Built-in Auth]
        AK --> BA
        BA --> LDB[(Local Database)]
    end

    subgraph "Cloud Version"
        UP --> EA[External Auth]
        SSO --> EA
        MFA --> EA
        EA --> Auth0[Auth0/Supabase]
        EA --> OIDC[OIDC]
        EA --> SAML[SAML]
    end

    BA --> JWT[JWT Generation]
    EA --> JWT
    JWT --> API[API Access]
```

### 2. Token Architecture

```mermaid
graph LR
    subgraph "Token Types"
        AT[Access Token<br/>15 min TTL]
        RT[Refresh Token<br/>7 days TTL]
        AK[API Key<br/>No expiry]
    end

    subgraph "Token Storage"
        LS[localStorage<br/>Access Token]
        HC[httpOnly Cookie<br/>Refresh Token]
        DB[(Database<br/>API Keys)]
    end

    subgraph "Token Validation"
        JWT[JWT Verify]
        RS[Rate Limit]
        IP[IP Check]
    end

    AT --> LS
    RT --> HC
    AK --> DB

    LS --> JWT
    HC --> JWT
    DB --> JWT

    JWT --> RS
    RS --> IP
```

## Authorization Model

### Role-Based Access Control (RBAC)

```mermaid
graph TB
    subgraph "Roles"
        O[Owner]
        A[Admin]
        OP[Operator]
        V[Viewer]
    end

    subgraph "Permissions"
        O --> FP[Full Permissions]
        A --> MP[Manage Devices<br/>Manage Users<br/>View Billing]
        OP --> DP[Deploy Updates<br/>Configure Devices<br/>View Telemetry]
        V --> VP[View Only]
    end

    subgraph "Resources"
        FP --> ORG[Organization]
        MP --> TEAM[Teams]
        DP --> DEV[Devices]
        VP --> TEL[Telemetry]
    end
```

### Multi-Tenancy Isolation

```mermaid
graph TB
    subgraph "Organization A"
        UA[Users A]
        TA[Teams A]
        DA[Devices A]
    end

    subgraph "Organization B"
        UB[Users B]
        TB[Teams B]
        DB[Devices B]
    end

    API[API Layer]

    UA --> API
    UB --> API

    API --> RLS[Row Level Security]

    RLS --> |org_id = A| DA
    RLS --> |org_id = B| DB

    style RLS fill:#f9f,stroke:#333,stroke-width:4px
```

## Implementation Details

### JWT Token Structure

```json
{
  "header": {
    "alg": "RS256",
    "typ": "JWT"
  },
  "payload": {
    "sub": "user_123",
    "email": "user@example.com",
    "org_id": "org_456",
    "role": "admin",
    "permissions": ["devices:read", "devices:write", "telemetry:read"],
    "iat": 1699000000,
    "exp": 1699003600,
    "iss": "fleetd.sh",
    "aud": "fleetd-api"
  },
  "signature": "..."
}
```

### API Key Format

```
fleetd_pk_live_a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6
└─────┘ └┘ └──┘ └────────────────────────────┘
  prefix │  env            random token
        type
```

## Security Features

### 1. Token Security

```mermaid
graph LR
    subgraph "Security Measures"
        ROT[Token Rotation]
        REV[Revocation List]
        AUD[Audience Validation]
        ISS[Issuer Validation]
    end

    subgraph "Attack Prevention"
        XSS[XSS Protection]
        CSRF[CSRF Protection]
        REP[Replay Prevention]
        BRU[Brute Force Protection]
    end

    ROT --> XSS
    REV --> CSRF
    AUD --> REP
    ISS --> BRU
```

### 2. Session Management

```mermaid
stateDiagram-v2
    [*] --> Unauthenticated
    Unauthenticated --> Authenticating: Login attempt
    Authenticating --> Authenticated: Success
    Authenticating --> Unauthenticated: Failure

    Authenticated --> Active: Activity
    Active --> Active: User action
    Active --> Idle: No activity (5min)
    Idle --> Active: User action
    Idle --> Warning: Idle timeout (25min)
    Warning --> Active: User action
    Warning --> Expired: Session timeout (30min)

    Authenticated --> Refreshing: Token expired
    Refreshing --> Authenticated: New token
    Refreshing --> Unauthenticated: Refresh failed

    Expired --> Unauthenticated: Must re-login
    Authenticated --> Unauthenticated: Logout
```

## Provider Configuration

### OSS Self-Hosted

```yaml
# config.yaml
auth:
  provider: built_in
  jwt:
    secret: ${JWT_SECRET}
    access_ttl: 15m
    refresh_ttl: 7d
  password:
    min_length: 12
    require_special: true
    bcrypt_cost: 12
  session:
    timeout: 30m
    max_concurrent: 5
```

### Cloud Managed

```yaml
# config.yaml
auth:
  provider: auth0  # or supabase, clerk, etc.
  auth0:
    domain: ${AUTH0_DOMAIN}
    client_id: ${AUTH0_CLIENT_ID}
    client_secret: ${AUTH0_CLIENT_SECRET}
    audience: ${AUTH0_AUDIENCE}
  sso:
    providers:
      - google
      - github
      - okta
      - azure_ad
  mfa:
    required: true
    methods:
      - totp
      - sms
      - webauthn
```

## API Endpoints

### Authentication Endpoints

| Endpoint | Method | Description | Auth Required |
|----------|--------|-------------|---------------|
| `/auth/login` | POST | User login | No |
| `/auth/logout` | POST | User logout | Yes |
| `/auth/refresh` | POST | Refresh tokens | Refresh token |
| `/auth/user` | GET | Current user info | Yes |
| `/auth/register` | POST | User registration (OSS) | No |
| `/auth/forgot-password` | POST | Password reset | No |
| `/auth/reset-password` | POST | Complete reset | Reset token |
| `/auth/verify-email` | POST | Email verification | Verify token |
| `/auth/sso/init` | POST | Start SSO flow | No |
| `/auth/sso/callback` | GET | SSO callback | No |

### API Key Management

| Endpoint | Method | Description | Auth Required |
|----------|--------|-------------|---------------|
| `/api-keys` | GET | List API keys | Yes |
| `/api-keys` | POST | Create API key | Yes |
| `/api-keys/:id` | DELETE | Revoke API key | Yes |
| `/api-keys/:id/rotate` | POST | Rotate API key | Yes |

## Migration Path (OSS → Cloud)

```mermaid
graph LR
    subgraph "Phase 1: OSS"
        BA1[Built-in Auth]
        UP1[Username/Password]
        DB1[(Local Users)]
    end

    subgraph "Phase 2: Hybrid"
        BA2[Built-in Auth]
        EA2[External Auth]
        MIG[Migration Tool]
    end

    subgraph "Phase 3: Cloud"
        EA3[External Auth Only]
        SSO3[SSO/SAML]
        MFA3[MFA Required]
    end

    BA1 --> MIG
    DB1 --> MIG
    MIG --> EA2
    EA2 --> EA3

    style MIG fill:#f96,stroke:#333,stroke-width:2px
```

## Security Best Practices

### 1. Password Requirements (OSS)
- Minimum 12 characters
- Mix of uppercase, lowercase, numbers, special chars
- No common passwords (checked against list)
- No reuse of last 5 passwords
- Forced rotation every 90 days (configurable)

### 2. Token Management
- Short-lived access tokens (15 minutes)
- Refresh token rotation on use
- Revocation on suspicious activity
- IP binding for sensitive operations
- Device fingerprinting

### 3. Rate Limiting
```
/auth/login: 5 attempts per 15 minutes
/auth/refresh: 10 per minute
/api/*: 100 per minute (authenticated)
/api/*: 10 per minute (unauthenticated)
```

### 4. Audit Logging
All authentication events are logged:
- Login attempts (success/failure)
- Token generation/refresh
- Permission changes
- API key usage
- Failed authorization attempts

## Integration Examples

### React Hook Usage

```typescript
import { useAuth } from '@/hooks/use-auth'

function MyComponent() {
  const { user, login, logout, isLoading } = useAuth()

  if (isLoading) return <Loading />
  if (!user) return <LoginForm onLogin={login} />

  return (
    <Dashboard user={user} onLogout={logout} />
  )
}
```

### API Client Usage

```typescript
import { fleetClient } from '@/lib/api/client'
import { env } from '@/env'

// Client automatically includes auth token
const devices = await fleetClient.listDevices({})

// Manual token handling
const response = await fetch(`${env.NEXT_PUBLIC_API_URL}/custom`, {
  headers: {
    Authorization: `Bearer ${getAccessToken()}`,
  },
})
```

### Server-Side Auth Check

```typescript
// app/api/route.ts
import { verifyAuth } from '@/lib/auth'
import { env } from '@/env'

export async function GET(req: Request) {
  const user = await verifyAuth(req)
  if (!user) {
    return new Response('Unauthorized', { status: 401 })
  }

  // Check permissions
  if (!user.permissions.includes('devices:read')) {
    return new Response('Forbidden', { status: 403 })
  }

  // Process request...
}
```

## Monitoring & Observability

### Key Metrics

```mermaid
graph TB
    subgraph "Auth Metrics"
        LR[Login Rate]
        LS[Login Success %]
        TL[Token Latency]
        TR[Token Refresh Rate]
    end

    subgraph "Security Metrics"
        FA[Failed Attempts]
        SA[Suspicious Activity]
        TB[Token Blacklist Size]
        SC[Session Count]
    end

    subgraph "Alerts"
        BF[Brute Force]
        TF[Token Fraud]
        AL[Anomaly Login]
    end

    LR --> BF
    FA --> BF
    SA --> TF
    LS --> AL
```

### Dashboard Panels
- Active sessions by user
- Login attempts (success/failure)
- Token refresh patterns
- API key usage
- Geographic login distribution
- Device fingerprint changes

## Troubleshooting

### Common Issues

1. **Token Expired**: Automatic refresh should handle this
2. **Invalid Signature**: Check JWT secret configuration
3. **CORS Errors**: Verify allowed origins
4. **Rate Limited**: Implement exponential backoff
5. **Session Conflicts**: Clear all tokens and re-login

### Debug Mode

Enable debug logging:
```typescript
// env.ts
DEBUG_AUTH: z.boolean().default(true)

// Logs all auth operations
if (env.DEBUG_AUTH) {
  console.log('[Auth]', operation, details)
}
```

## Future Enhancements

- [ ] WebAuthn/Passkeys support
- [ ] Hardware token support (YubiKey)
- [ ] Risk-based authentication
- [ ] Passwordless login
- [ ] Social login providers
- [ ] Enterprise SSO (SAML 2.0)
- [ ] OAuth2 authorization server
- [ ] Zero-trust architecture