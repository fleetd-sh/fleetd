# Security Policy

## Reporting Security Vulnerabilities

Security vulnerabilities should be reported responsibly to help improve the project.

### Where to Report

**DO NOT** create public GitHub issues for security vulnerabilities.

Please report security vulnerabilities using [GitHub Security Advisories](https://github.com/fleetd-sh/fleetd/security/advisories/new).

### What to Include

Your report should include:

- Description of the vulnerability
- Steps to reproduce the issue
- Potential impact
- Suggested fix (if any)
- Your contact information for follow-up

### Response Timeline

- **Initial Response**: Within 48 hours
- **Status Update**: Within 7 days
- **Resolution Target**: Within 30 days for critical issues

## Security Measures

### Authentication & Authorization

- **JWT-based authentication** with short-lived tokens
- **Refresh token rotation** to prevent token replay attacks
- **API key scoping** for programmatic access
- **RBAC** with predefined roles (Admin, Operator, Viewer)
- **Session management** with configurable timeouts

### Device Security

- **mTLS support** for device-to-cloud communication
- **Device enrollment tokens** with expiration
- **Hardware ID verification** to prevent spoofing
- **Secure OTA updates** with checksum verification
- **Certificate pinning** available for critical deployments

### API Security

- **Rate limiting** per IP and per user
- **Circuit breakers** to prevent cascade failures
- **Input validation** on all endpoints
- **SQL injection prevention** via parameterized queries
- **XSS protection** headers on all responses

### Data Protection

- **Encryption at rest** for sensitive data (optional)
- **TLS 1.2+** for all communications
- **Secure credential storage** using bcrypt
- **Audit logging** for compliance requirements
- **PII handling** in compliance with GDPR

## Security Best Practices

### Deployment

1. **Change default credentials immediately**
   ```bash
   fleetctl user change-password admin
   ```

2. **Enable TLS/mTLS**
   ```yaml
   tls:
     enabled: true
     mode: auto  # or manual with your certificates
   ```

3. **Configure rate limiting**
   ```yaml
   rate_limiting:
     enabled: true
     requests_per_second: 100
     burst: 200
   ```

4. **Set up audit logging**
   ```yaml
   audit:
     enabled: true
     retention_days: 90
   ```

### Network Security

- Run services in a **private network**
- Use **firewall rules** to restrict access
- Enable **network policies** in Kubernetes
- Use **VPN or bastion hosts** for management access

### Secrets Management

- **Never commit secrets** to version control
- Use **environment variables** or secret management tools
- Rotate **JWT secrets** regularly
- Store **API keys** securely
- Use **external secret managers** (Vault, AWS Secrets Manager)

### Monitoring

- Enable **security event logging**
- Set up **alerts for suspicious activity**
- Monitor **failed authentication attempts**
- Track **API usage patterns**
- Review **audit logs regularly**

## Security Headers

The platform sets these security headers by default:

```
X-Content-Type-Options: nosniff
X-Frame-Options: DENY
X-XSS-Protection: 1; mode=block
Strict-Transport-Security: max-age=31536000; includeSubDomains
Content-Security-Policy: default-src 'self'
```

## Vulnerability Disclosure

### Scope

The following are in scope:
- fleetd Platform API
- Device API
- fleetctl CLI
- Web Dashboard (Studio UI)
- Authentication/Authorization systems
- Device agent implementations

Out of scope:
- Denial of Service attacks
- Social engineering
- Physical attacks
- Third-party services

### Safe Harbor

We support safe harbor for security researchers who:
- Make a good faith effort to avoid privacy violations
- Only exploit vulnerabilities on test instances
- Do not perform destructive actions
- Provide detailed reports

## Known Security Considerations

### Default Installation

The default installation includes:
- Demo credentials (admin@fleetd.local / admin123)
- Self-signed certificates
- Open metrics endpoints

**These MUST be changed for production use.**

### Device Enrollment

- Enrollment tokens should have **limited lifetime**
- Use **IP allowlisting** for known device networks
- Implement **device attestation** for high-security environments
- Monitor for **unusual enrollment patterns**

### Update Deployment

- Always **verify checksums** before deployment
- Use **staged rollouts** for critical updates
- Implement **rollback procedures**
- Test updates in **staging environments** first

## Compliance

The fleetd platform can be configured to support:
- **GDPR** - Data protection and privacy
- **SOC 2** - Security controls
- **HIPAA** - Healthcare data (with additional configuration)
- **ISO 27001** - Information security management

## Security Checklist

Before going to production:

- [ ] Changed all default passwords
- [ ] Configured TLS certificates
- [ ] Enabled audit logging
- [ ] Set up rate limiting
- [ ] Configured firewall rules
- [ ] Implemented backup procedures
- [ ] Tested disaster recovery
- [ ] Reviewed RBAC permissions
- [ ] Enabled monitoring and alerts
- [ ] Documented security procedures

## Security Updates

Stay informed about security updates:

1. Watch the [GitHub repository](https://github.com/fleetd-sh/fleetd)
2. Subscribe to [GitHub Security Advisories](https://github.com/fleetd-sh/fleetd/security/advisories)
3. Check the [CHANGELOG](CHANGELOG.md) for security fixes

## Resources

For security concerns:
- [GitHub Security Advisories](https://github.com/fleetd-sh/fleetd/security/advisories)

For general support:
- [GitHub Issues](https://github.com/fleetd-sh/fleetd/issues)
- [GitHub Discussions](https://github.com/fleetd-sh/fleetd/discussions)
- [Documentation](https://github.com/fleetd-sh/fleetd/wiki)

---

Security reports help improve the project for everyone.