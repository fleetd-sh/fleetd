# Security Guidelines for FleetD

## Overview

FleetD implements multiple security measures to ensure safe provisioning and management of edge devices. This document outlines the security features and best practices.

## Security Features

### 1. Input Validation

All user inputs are validated before processing:

- **Device Paths**: Validated against known patterns to prevent arbitrary device access
- **Hostnames**: RFC 1123 compliant validation
- **IP Addresses**: Proper IP validation to prevent injection
- **SSH Keys**: Format and size validation
- **WiFi Credentials**: Length and character validation
- **K3s Tokens**: Size limits and character validation

### 2. SSH Security

#### Host Key Verification
- First-time connections prompt for fingerprint verification
- Known hosts are stored in `~/.ssh/known_hosts`
- Option for strict host key checking

#### Key Management
- Private keys are never transmitted or stored in provisioned devices
- Only public keys are written to SD cards
- Support for standard SSH key formats (RSA, Ed25519, ECDSA)

#### Connection Security
- Configurable timeouts to prevent hanging connections
- No support for password authentication (key-only)
- Secure token retrieval over encrypted SSH channels

### 3. Template Security

#### Injection Prevention
- All template variables are escaped for shell safety
- Special characters are sanitized before template rendering
- No direct command execution from user input

#### Safe Functions
Templates use secure functions:
- `shellsafe`: Removes dangerous shell metacharacters
- `quote`: Properly quotes strings for shell use
- `sanitize`: Escapes special characters

### 4. File System Security

#### Path Traversal Prevention
- All file paths are validated against traversal attempts
- Absolute paths are enforced where required
- Clean path resolution to prevent `../` attacks

#### Size Limits
- SSH keys limited to 16KB
- Tokens limited to 512 bytes
- Configuration files have reasonable size limits

### 5. Network Security

#### mDNS Discovery
- Device information is validated before processing
- IP addresses are verified before connection attempts
- Timeout limits on discovery operations

#### K3s Token Handling
- Tokens are retrieved over secure SSH connections
- Optional local caching with restricted file permissions (0600)
- Tokens are never logged or displayed in plain text

### 6. Provisioning Security

#### SD Card Safety
- Only recognized device patterns are allowed
- Verification that device is removable media
- Confirmation prompts for destructive operations

#### Configuration Validation
- All configuration is validated before provisioning
- Template rendering happens in memory
- Secure defaults for all optional parameters

## Security Best Practices

### For Users

1. **SSH Key Security**
   - Use Ed25519 keys when possible (more secure, smaller)
   - Protect private keys with appropriate file permissions (0600)
   - Consider using SSH agent for key management

2. **Network Security**
   - Use WPA2/WPA3 for WiFi networks
   - Avoid provisioning over untrusted networks
   - Use static IPs in controlled environments

3. **K3s Cluster Security**
   - Rotate tokens periodically
   - Use separate tokens for different clusters
   - Secure the control plane with network policies

4. **Physical Security**
   - Provision SD cards in secure environments
   - Verify device identity before provisioning
   - Secure physical access to provisioned devices

### For Developers

1. **Input Handling**
   - Always validate user input using the `provision.Validate*` functions
   - Never pass raw user input to shell commands
   - Use parameterized queries for any database operations

2. **Error Handling**
   - Don't expose sensitive information in error messages
   - Log security events appropriately
   - Fail securely (deny by default)

3. **Cryptography**
   - Use standard crypto libraries (golang.org/x/crypto)
   - Don't implement custom crypto
   - Use secure random for token generation

4. **Code Review**
   - Review all template changes for injection vulnerabilities
   - Audit SSH operations for security issues
   - Check file operations for traversal vulnerabilities

## Security Checklist

Before provisioning:
- [ ] Verify SD card device path
- [ ] Validate all network settings
- [ ] Check SSH key permissions
- [ ] Confirm k3s token security

During provisioning:
- [ ] Monitor for unexpected errors
- [ ] Verify template rendering
- [ ] Check file write operations
- [ ] Validate configuration output

After provisioning:
- [ ] Secure token files (if saved)
- [ ] Clear sensitive data from memory
- [ ] Verify device configuration
- [ ] Test connectivity securely

## Reporting Security Issues

If you discover a security vulnerability:

1. **Do NOT** create a public GitHub issue
2. Email security details to: security@fleetd.sh
3. Include:
   - Description of the vulnerability
   - Steps to reproduce
   - Potential impact
   - Suggested fix (if any)

## Security Updates

Security updates are released as:
- **Critical**: Immediate patch release
- **High**: Within 7 days
- **Medium**: Within 30 days
- **Low**: Next regular release

Subscribe to security announcements at: https://fleetd.sh/security

## Compliance

FleetD follows security best practices from:
- OWASP Secure Coding Practices
- CIS Security Benchmarks
- NIST Cybersecurity Framework

For compliance documentation, see: https://fleetd.sh/compliance