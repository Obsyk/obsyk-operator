# Obsyk Operator

Kubernetes operator that connects your cluster to the [Obsyk](https://obsyk.ai) observability platform. Deploy once and automatically stream cluster metadata to gain visibility across your infrastructure.

## Features

- **Deploy & Forget** - Single Custom Resource configures everything
- **Real-time Sync** - Stream Namespace, Pod, and Service changes as they happen
- **Secure by Design** - Read-only cluster access, OAuth2 JWT authentication
- **Lightweight** - Minimal resource footprint, efficient event-driven architecture

## Prerequisites

- Kubernetes 1.26+
- Helm 3.x
- Obsyk account (sign up at [app.obsyk.ai](https://app.obsyk.ai))

## Quick Start

### 1. Register Your Cluster in Obsyk

1. Log in to [app.obsyk.ai](https://app.obsyk.ai)
2. Navigate to **Clusters** and click **Add cluster**
3. Enter a name for your cluster and click **Generate credentials**
4. Save the **Client ID** and **Private Key** - you'll need these for installation

### 2. Add the Helm Repository

```bash
helm repo add obsyk https://obsyk.github.io/obsyk-operator
helm repo update
```

### 3. Create Credentials Secret

Save your private key to a file (e.g., `private-key.pem`), then create the secret:

```bash
kubectl create namespace obsyk-system

kubectl create secret generic obsyk-credentials \
  --namespace obsyk-system \
  --from-literal=client_id=YOUR_CLIENT_ID \
  --from-file=private_key=private-key.pem
```

### 4. Install the Operator

```bash
helm install obsyk-operator obsyk/obsyk-operator \
  --namespace obsyk-system \
  --set agent.clusterName="my-cluster" \
  --set agent.platformURL="https://app.obsyk.ai"
```

### 5. Verify Installation

```bash
# Check the operator is running
kubectl get pods -n obsyk-system

# Check the agent status
kubectl get obsykagent -n obsyk-system

# View operator logs
kubectl logs -n obsyk-system -l app.kubernetes.io/name=obsyk-operator
```

Your cluster should now appear as **Connected** in the Obsyk dashboard!

## Configuration

### Helm Values

| Parameter | Description | Default |
|-----------|-------------|---------|
| `agent.enabled` | Create ObsykAgent CR | `true` |
| `agent.clusterName` | Display name for this cluster | `""` (required) |
| `agent.platformURL` | Obsyk platform endpoint | `https://api.obsyk.ai` |
| `agent.credentialsSecretRef.name` | Secret containing OAuth2 credentials | `obsyk-credentials` |
| `agent.syncInterval` | Heartbeat/sync interval | `5m` |
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
  platformURL: "https://app.obsyk.ai"
  clusterName: "production-us-east-1"
  credentialsSecretRef:
    name: obsyk-credentials
  syncInterval: "5m"
```

### Credentials Secret Format

The credentials secret must contain:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: obsyk-credentials
  namespace: obsyk-system
type: Opaque
data:
  client_id: <base64-encoded OAuth2 client ID>
  private_key: <base64-encoded PEM ECDSA P-256 private key>
```

## Security

### Authentication

The operator uses **OAuth2 JWT Bearer** authentication (RFC 7523):

1. When you register a cluster in Obsyk, an ECDSA P-256 key pair is generated
2. The public key is stored by Obsyk, private key is given to you
3. The operator creates JWT assertions signed with the private key
4. Tokens are exchanged for short-lived access tokens (1 hour TTL)
5. Access tokens are automatically refreshed before expiry

### RBAC Permissions

The operator requires **read-only** access to cluster resources:

| Resource | Verbs |
|----------|-------|
| Namespaces | list, watch |
| Pods | list, watch |
| Services | list, watch |

### Credential Security

- Private keys are **never** stored in the Custom Resource
- Keys must be provided via Kubernetes Secret reference
- Keys are read at runtime and never logged
- All API communication uses TLS

## Observability

### Status Conditions

Check the agent status:

```bash
kubectl get obsykagent -n obsyk-system -o yaml
```

Status conditions:
- `Available` - Operator is running and connected
- `Syncing` - Actively streaming data to platform
- `Degraded` - Connection or configuration issue

### Resource Counts

The status includes counts of watched resources:

```yaml
status:
  resourceCounts:
    namespaces: 10
    pods: 100
    services: 20
```

### Logs

```bash
kubectl logs -n obsyk-system -l app.kubernetes.io/name=obsyk-operator
```

## Troubleshooting

### Agent not connecting

1. Verify the credentials Secret exists:
   ```bash
   kubectl get secret obsyk-credentials -n obsyk-system
   ```

2. Check the secret has the required keys:
   ```bash
   kubectl get secret obsyk-credentials -n obsyk-system -o jsonpath='{.data}' | jq -r 'keys'
   ```

3. Check operator logs for authentication errors:
   ```bash
   kubectl logs -n obsyk-system -l app.kubernetes.io/name=obsyk-operator | grep -i error
   ```

4. Verify network connectivity to the platform URL

### Agent stuck in Pending

This usually means the agent hasn't sent its first heartbeat. Check:
- The operator pod is running
- No authentication errors in logs
- Network connectivity to the platform

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

- Documentation: [docs.obsyk.ai](https://docs.obsyk.ai)
- Issues: [GitHub Issues](https://github.com/Obsyk/obsyk-operator/issues)
- Email: support@obsyk.com
