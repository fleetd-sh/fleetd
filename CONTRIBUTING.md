# Contributing to fleetd

Contributions to fleetd are appreciated. This guide explains how to contribute.

## Table of Contents
- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [Development Setup](#development-setup)
- [How to Contribute](#how-to-contribute)
- [Pull Request Process](#pull-request-process)
- [Development Workflow](#development-workflow)
- [Code Style](#code-style)
- [Testing](#testing)
- [Documentation](#documentation)
- [Release Process](#release-process)

## Code of Conduct

Contributors should follow these guidelines:
- Be respectful and inclusive
- Welcome newcomers and help them get started
- Focus on constructive criticism
- Accept feedback gracefully

## Getting Started

1. **Fork the repository** on GitHub
2. **Clone your fork** locally:
   ```bash
   git clone https://github.com/YOUR-USERNAME/fleetd.git
   cd fleetd
   ```
3. **Add upstream remote**:
   ```bash
   git remote add upstream https://github.com/fleetd-sh/fleetd.git
   ```

## Development Setup

### Prerequisites

Install required tools:
```bash
# macOS
brew install go just buf protobuf
brew install --cask docker

# Install Bun for frontend
curl -fsSL https://bun.sh/install | bash

# Linux
# ... (install Go 1.21+, Just, Buf, Docker)
curl -fsSL https://bun.sh/install | bash
```

Verify installation:
```bash
just check-tools
```

### Quick Start

1. **Install dependencies**:
   ```bash
   just install
   ```

2. **Start development environment**:
   ```bash
   # Start both backend and frontend in dev mode
   just dev

   # Or start services individually:
   just device-api-dev    # Device API on :8080
   just platform-api-dev  # Platform API on :8090
   just web-dev          # Studio UI on :3000
   ```

3. **Run tests**:
   ```bash
   just test-all
   ```

### Using fleetctl for Local Development

```bash
# Build fleetctl
just build fleetctl

# Start local platform
./bin/fleetctl start --profile development

# Check status
./bin/fleetctl status
```

## How to Contribute

### Reporting Bugs

Before creating bug reports, please check existing issues. When creating a bug report, include:

- **Clear title and description**
- **Steps to reproduce**
- **Expected vs actual behavior**
- **Environment details** (OS, Go version, etc.)
- **Logs and error messages**

### Suggesting Features

When suggesting features, provide:

- **Use case description**
- **Proposed solution**
- **Alternative solutions considered**
- **Additional context**

### First-Time Contributors

Look for issues labeled:
- `good first issue` - Simple fixes to get started
- `help wanted` - Issues that need attention
- `documentation` - Documentation improvements

## Pull Request Process

### Before Submitting

1. **Sync with upstream**:
   ```bash
   git fetch upstream
   git rebase upstream/main
   ```

2. **Create feature branch**:
   ```bash
   git checkout -b feature/your-feature-name
   ```

3. **Make changes and test**:
   ```bash
   # Format code
   just format-all

   # Lint
   just lint-all

   # Run tests
   just test-all

   # Run specific tests
   just test-go-run TestYourFeature
   ```

### Commit Guidelines

This project uses [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <subject>

<body>

<footer>
```

**Types:**
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation only
- `style`: Code style (formatting, semicolons, etc.)
- `refactor`: Code refactoring
- `perf`: Performance improvements
- `test`: Adding tests
- `chore`: Maintenance tasks
- `ci`: CI/CD changes

**Examples:**
```bash
feat(device-api): add device grouping support
fix(auth): resolve JWT expiration issue
docs(api): update OpenAPI specifications
test(telemetry): add integration tests for metrics
```

### Submitting PR

1. **Push to your fork**:
   ```bash
   git push origin feature/your-feature-name
   ```

2. **Create Pull Request** via GitHub UI

3. **PR should include**:
   - Clear title and description
   - Link to related issue (fixes #123)
   - Test results
   - Documentation updates if needed

4. **Wait for review**

### After PR is Merged

```bash
# Clean up local branch
git checkout main
git branch -d feature/your-feature-name

# Sync with upstream
git fetch upstream
git rebase upstream/main
```

## Development Workflow

### Working with Protocol Buffers

```bash
# Generate code from protos
just proto

# Format proto files
just proto-format

# Check for breaking changes
just proto-breaking
```

### Database Migrations

```bash
# Create new migration
just db-migration add_device_tags

# Run migrations
just db-migrate

# Rollback
just db-rollback
```

### Building

```bash
# Build everything
just build-all

# Build specific component
just build platform-api
just build device-api
just build fleetctl

# Build Docker images
just docker-build-all
```

### Testing Strategy

```bash
# Unit tests only
just test-go

# Integration tests
just test-integration-coverage

# E2E tests
just test-e2e

# Performance tests
just test-performance

# Full test suite with coverage
just test-full-coverage
```

## Code Style

### Go

- Follow [Effective Go](https://golang.org/doc/effective_go)
- Use `gofmt` (handled by `just format-go`)
- Meaningful variable names
- Comment exported functions
- Handle errors explicitly
- No unused imports or variables

### TypeScript/React

- Follow Biome configuration
- Functional components with hooks
- TypeScript for all new code
- Meaningful component and prop names

### General

- Keep functions small and focused
- Write self-documenting code
- Add tests for new features
- Update documentation

## Testing

### Test Requirements

- New features must include tests
- Bug fixes should include regression tests
- Maintain or improve code coverage
- All tests must pass before merging

### Running Tests

```bash
# Quick test
just test-go

# With coverage
just test-go-coverage

# Integration tests
just test-integration-coverage

# Specific test
just test-go-run TestDeviceRegistration
```

### Test Infrastructure

```bash
# Start test databases
just test-infra-up

# Stop test databases
just test-infra-down
```

## Documentation

### Code Documentation

- Document all exported functions
- Include examples for complex APIs
- Keep README.md updated
- Update OpenAPI specs when changing APIs

### Generating Docs

```bash
# Generate OpenAPI docs
just docs-generate

# Serve API documentation
just docs-serve

# Open in browser
just docs-api
```

## Release Process

### Version Numbering

This project uses [Semantic Versioning](https://semver.org/):
- MAJOR: Breaking API changes
- MINOR: New features (backward compatible)
- PATCH: Bug fixes

### Creating a Release

1. Update CHANGELOG.md
2. Create and push tag:
   ```bash
   just release 1.2.3
   ```
3. GitHub Actions will handle the rest

## Getting Help

- **Discord**: [Join our community](https://discord.gg/fleetd)
- **Issues**: [GitHub Issues](https://github.com/fleetd-sh/fleetd/issues)
- **Discussions**: [GitHub Discussions](https://github.com/fleetd-sh/fleetd/discussions)
- **Wiki**: [Documentation](https://github.com/fleetd-sh/fleetd/wiki)

## Recognition

Contributors will be recognized in:
- CHANGELOG.md
- GitHub contributors page
- Special thanks in release notes

Thank you for contributing to the fleetd platform! 🚀