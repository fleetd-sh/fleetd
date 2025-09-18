# Container Images

This directory contains OCI/Docker container definitions for fleetd components.

## Files

- `platform.dockerfile` - Platform API container image
- `device.dockerfile` - Device API container image

## Building Images

```bash
# Build platform API image
docker build -f container/platform.dockerfile -t fleetd/platform-api:latest .

# Build device API image
docker build -f container/device.dockerfile -t fleetd/device-api:latest .
```

## Future: Pre-built Images

The plan is to publish pre-built OCI images to a registry (Docker Hub, GitHub Container Registry, etc.) so that `fleetctl start` can pull and run images directly instead of building locally.

This would:
1. Speed up initial setup (no local build required)
2. Ensure consistency across deployments
3. Enable easier version management
4. Support multi-architecture images (amd64, arm64)

## Proposed Image Names

- `ghcr.io/fleetd-sh/platform-api:latest`
- `ghcr.io/fleetd-sh/device-api:latest`
- `ghcr.io/fleetd-sh/fleetd:latest` (device agent)

## Refactoring fleetctl start

Instead of the current approach where `fleetctl start` builds binaries locally and mounts them, it should:

1. Pull the appropriate container images
2. Run them with proper configuration
3. Use environment variables for configuration
4. Mount only configuration files, not binaries

Example future command:
```go
// Instead of building locally:
// buildCmd := exec.Command("just", "build-platform")

// Pull and run pre-built image:
docker pull ghcr.io/fleetd-sh/platform-api:v1.0.0
docker run -d \
  --name fleetd-platform-api \
  --network fleetd-network \
  -e CONFIG_FILE=/etc/fleetd/config.toml \
  -v ./config.toml:/etc/fleetd/config.toml:ro \
  ghcr.io/fleetd-sh/platform-api:v1.0.0
```