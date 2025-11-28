# Obsyk Operator

Kubernetes operator that connects your cluster to the [Obsyk](https://obsyk.com) observability platform. Deploy once and automatically stream cluster metadata to gain visibility across your infrastructure.

## Features

- **Deploy & Forget** - Single Custom Resource configures everything
- **Real-time Sync** - Stream Namespace, Pod, and Service changes as they happen
- **Secure by Design** - Read-only cluster access, API keys stored in Secrets
- **Lightweight** - Minimal resource footprint, efficient event-driven architecture

## Prerequisites

- Kubernetes 1.26+
- Helm 3.x
- Obsyk account with API key

## Quick Start

### 1. Add the Helm Repository

```bash
helm repo add obsyk https://obsyk.github.io/obsyk-operator
helm repo update
```

### 2. Create API Key Secret

```bash
kubectl create namespace obsyk-system

kubectl create secret generic obsyk-api-key \
  --namespace obsyk-system \
  --from-literal=token=YOUR_API_KEY
```

### 3. Install the Operator

```bash
helm install obsyk-operator obsyk/obsyk-operator \
  --namespace obsyk-system \
  --set agent.clusterName="my-cluster" \
  --set agent.platformURL="https://api.obsyk.com"
```

## Configuration

### Helm Values

| Parameter | Description | Default |
|-----------|-------------|---------|
| `agent.clusterName` | Logical name for this cluster | `""` (required) |
| `agent.platformURL` | Obsyk platform endpoint | `https://api.obsyk.com` |
| `agent.apiKeySecretRef.name` | Secret containing API key | `obsyk-api-key` |
| `agent.apiKeySecretRef.key` | Key within the Secret | `token` |
| `replicaCount` | Number of operator replicas | `1` |
| `resources.limits.cpu` | CPU limit | `200m` |
| `resources.limits.memory` | Memory limit | `256Mi` |
| `resources.requests.cpu` | CPU request | `100m` |
| `resources.requests.memory` | Memory request | `128Mi` |

### Custom Resource

The operator watches `ObsykAgent` custom resources:

```yaml
apiVersion: obsyk.io/v1
kind: ObsykAgent
metadata:
  name: obsyk-agent
  namespace: obsyk-system
spec:
  platformURL: "https://api.obsyk.com"
  clusterName: "production-us-east-1"
  apiKeySecretRef:
    name: obsyk-api-key
    key: token
```

## Security

### RBAC Permissions

The operator requires **read-only** access to cluster resources:

| Resource | Verbs |
|----------|-------|
| Namespaces | list, watch |
| Pods | list, watch |
| Services | list, watch |

### API Key Security

- API keys are **never** stored in the Custom Resource
- Keys must be provided via Kubernetes Secret reference
- Keys are read at runtime and never logged

## Observability

### Status Conditions

Check the agent status:

```bash
kubectl get obsykagent -n obsyk-system -o yaml
```

Status conditions:
- `Available` - Operator is running and connected
- `Syncing` - Actively streaming data to platform
- `Error` - Connection or configuration issue

### Logs

```bash
kubectl logs -n obsyk-system -l app.kubernetes.io/name=obsyk-operator
```

## Troubleshooting

### Agent not syncing

1. Verify the API key Secret exists:
   ```bash
   kubectl get secret obsyk-api-key -n obsyk-system
   ```

2. Check operator logs for connection errors:
   ```bash
   kubectl logs -n obsyk-system -l app.kubernetes.io/name=obsyk-operator
   ```

3. Verify network connectivity to platform URL

### High memory usage

The operator caches watched resources. For large clusters, consider:
- Adjusting memory limits in Helm values
- Contacting support for namespace filtering options

## Uninstall

```bash
helm uninstall obsyk-operator -n obsyk-system
kubectl delete namespace obsyk-system
```

## Development

See [CLAUDE.md](./CLAUDE.md) for development instructions.

## License

Apache License 2.0 - see [LICENSE](./LICENSE) for details.

## Support

- Documentation: [docs.obsyk.com](https://docs.obsyk.com)
- Issues: [GitHub Issues](https://github.com/Obsyk/obsyk-operator/issues)
- Email: support@obsyk.com
