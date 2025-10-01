# Kubernetes Deployment Guide

This guide provides basic Kubernetes manifests for deploying the fleetd platform. Adapt these to your specific needs and infrastructure.

## Prerequisites

- Kubernetes cluster (1.20+)
- kubectl configured
- Docker images built and pushed to your registry

## Quick Start

1. **Build and push Docker images**:
```bash
# Build images
docker build -f Dockerfile.platform-api -t your-registry/fleetd/platform-api:latest .
docker build -f Dockerfile.device-api -t your-registry/fleetd/device-api:latest .

# Push to your registry
docker push your-registry/fleetd/platform-api:latest
docker push your-registry/fleetd/device-api:latest
```

2. **Create namespace**:
```bash
kubectl create namespace fleetd
```

3. **Deploy PostgreSQL** (or use your existing database):
```bash
kubectl apply -f postgres.yaml -n fleetd
```

4. **Deploy Valkey** (or use your existing Valkey/Redis-compatible cache):
```bash
kubectl apply -f valkey.yaml -n fleetd
```

5. **Configure secrets**:
```bash
# Edit secrets.yaml with your values
kubectl apply -f secrets.yaml -n fleetd
```

6. **Deploy fleetd services**:
```bash
kubectl apply -f platform-api.yaml -n fleetd
kubectl apply -f device-api.yaml -n fleetd
```

7. **Configure ingress** (optional):
```bash
kubectl apply -f ingress.yaml -n fleetd
```

## Configuration

### Environment Variables

Key environment variables to configure in the deployment manifests:

**Platform API**:
- `DATABASE_URL`: PostgreSQL connection string
- `VALKEY_URL`: Valkey connection string
- `JWT_SECRET`: Random 64-character string for JWT signing
- `TLS_MODE`: Certificate mode (auto/manual/self-signed)

**Device API**:
- `DATABASE_URL`: PostgreSQL connection string
- `VALKEY_URL`: Valkey connection string
- `MTLS_ENABLED`: Enable/disable mTLS for devices
- `PLATFORM_API_URL`: Internal platform API URL

### Resource Requirements

Recommended minimum resources:

| Component | CPU Request | Memory Request | CPU Limit | Memory Limit |
|-----------|------------|----------------|-----------|--------------|
| Platform API | 250m | 256Mi | 1000m | 1Gi |
| Device API | 500m | 512Mi | 2000m | 2Gi |
| PostgreSQL | 500m | 512Mi | 2000m | 2Gi |
| Valkey | 100m | 128Mi | 500m | 512Mi |

Adjust based on your expected load.

### Scaling

Both APIs can be horizontally scaled:

```bash
# Scale platform API
kubectl scale deployment platform-api --replicas=3 -n fleetd

# Scale device API (handles more connections)
kubectl scale deployment device-api --replicas=5 -n fleetd
```

Consider using HorizontalPodAutoscaler for automatic scaling:

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: device-api-hpa
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: device-api
  minReplicas: 3
  maxReplicas: 20
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
```

### Persistence

- **PostgreSQL**: Use PersistentVolumeClaims for database storage
- **Valkey**: Optional persistence, can run ephemeral for caching
- **Artifacts**: Consider object storage (S3, MinIO) for update artifacts

### Networking

- **Internal communication**: Services communicate via Kubernetes DNS
- **External access**: Use Ingress or LoadBalancer services
- **mTLS for devices**: Configure cert-manager or bring your own certificates

### Security Considerations

1. **Network Policies**: Restrict pod-to-pod communication
2. **RBAC**: Use service accounts with minimal permissions
3. **Secrets Management**: Use external secret managers (Vault, Sealed Secrets)
4. **Pod Security**: Run as non-root, read-only root filesystem
5. **Image Scanning**: Scan images for vulnerabilities

## Production Checklist

- [ ] Configure resource limits and requests
- [ ] Set up monitoring (Prometheus/Grafana)
- [ ] Configure log aggregation
- [ ] Implement backup strategy for PostgreSQL
- [ ] Set up TLS/SSL certificates
- [ ] Configure network policies
- [ ] Implement pod disruption budgets
- [ ] Set up health checks and readiness probes
- [ ] Configure anti-affinity rules for HA
- [ ] Test disaster recovery procedures

## Monitoring

Add Prometheus annotations to your deployments:

```yaml
metadata:
  annotations:
    prometheus.io/scrape: "true"
    prometheus.io/port: "8080"
    prometheus.io/path: "/metrics"
```

## Troubleshooting

### Check pod status
```bash
kubectl get pods -n fleetd
kubectl describe pod <pod-name> -n fleetd
kubectl logs <pod-name> -n fleetd
```

### Database connectivity
```bash
kubectl exec -it <platform-api-pod> -n fleetd -- nc -zv postgres 5432
```

### Service discovery
```bash
kubectl get svc -n fleetd
kubectl get endpoints -n fleetd
```

## Notes

- These manifests are starting points - adapt to your needs
- Consider using GitOps tools (ArgoCD, Flux) for production
- Implement proper CI/CD for image builds and deployments
- Use namespaces to isolate environments (dev, staging, prod)

For questions or issues, please refer to the main documentation or open an issue on GitHub.