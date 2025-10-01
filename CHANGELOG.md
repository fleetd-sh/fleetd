# Changelog

All notable changes to the fleetd platform will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Initial release of the fleetd platform
- Device management with registration, monitoring, and control
- Device fleet operations for organizing devices with tags and metadata
- JWT-based authentication with refresh tokens and API keys
- Real-time telemetry collection with Prometheus integration
- OTA update deployment with multiple strategies (rolling, canary, blue-green)
- Web-based management dashboard (Studio UI)
- Command-line interface (fleetctl) for fleet operations
- Docker and Kubernetes deployment support
- mTLS support for secure device communication
- Rate limiting and circuit breakers for API protection
- Comprehensive audit logging for compliance
- WebSocket support for real-time device communication
- OpenAPI documentation for all APIs

### Infrastructure
- PostgreSQL with TimescaleDB for time-series data
- Valkey (Redis-compatible) for caching and pub/sub
- Connect-RPC for efficient API communication
- Vanguard for REST API compatibility

### Documentation
- Comprehensive README with quick start guide
- API documentation with OpenAPI specifications
- Device agent examples for Raspberry Pi
- SDK examples in Go and Node.js
- Deployment guides for Docker and Kubernetes
- Contributing guidelines

## [0.1.0] - Coming Soon

Initial public release.

### Planned Features
- Multi-region support
- Enhanced RBAC with custom roles
- Device group automation rules
- Scheduled deployments
- Advanced telemetry analytics
- Webhook integrations
- Third-party integrations (Slack, PagerDuty, etc.)

## Version History

### Development Milestones

#### Phase 1: Core Platform (Completed)
- ✅ Basic device registration and management
- ✅ JWT authentication system
- ✅ Device fleet organization capabilities
- ✅ Database schema and migrations

#### Phase 2: Telemetry & Updates (Completed)
- ✅ Real-time telemetry collection
- ✅ Metrics aggregation with TimescaleDB
- ✅ OTA update system
- ✅ Deployment strategies implementation

#### Phase 3: Security & Scale (Completed)
- ✅ mTLS for device authentication
- ✅ Rate limiting and circuit breakers
- ✅ Audit logging system
- ✅ RBAC implementation

#### Phase 4: Operations & UI (Completed)
- ✅ Web dashboard (Studio UI)
- ✅ CLI tool (fleetctl)
- ✅ Docker packaging
- ✅ Kubernetes manifests

#### Phase 5: Developer Experience (Current)
- ✅ OpenAPI documentation
- ✅ SDK examples
- ✅ Device agent examples
- ✅ Contributing guidelines
- 🚧 Automated release process
- 🚧 Integration test suite

## Upgrade Guide

### From Development to Production

1. **Database Migration**
   ```bash
   fleetctl migrate up
   ```

2. **Configuration Updates**
   - Set `JWT_SECRET` to a secure random value
   - Configure TLS certificates
   - Update database credentials
   - Set up monitoring endpoints

3. **Security Checklist**
   - Change default admin credentials
   - Enable mTLS for device communication
   - Configure rate limiting
   - Set up audit logging
   - Review RBAC permissions

## Breaking Changes

Currently no breaking changes as this is the initial release.

## Contributors

Thanks to all contributors who have helped shape the fleetd platform:

- Core Development Team
- Community Contributors
- Beta Testers
- Documentation Writers

Special thanks to the open source projects we build upon:
- Connect-RPC
- Valkey
- TimescaleDB
- shadcn/ui

---

For more details on each release, see the [GitHub Releases](https://github.com/fleetd-sh/fleetd/releases) page.