# Obsyk Operator

Kubernetes operator that connects your cluster to the [Obsyk](https://obsyk.ai) observability platform. Deploy once and automatically stream cluster metadata to gain visibility across your infrastructure.

## Features

- **Deploy & Forget** - Single Custom Resource configures everything
- **Real-time Sync** - Stream changes to Namespaces, Pods, Services, Nodes, Deployments, StatefulSets, DaemonSets, Jobs, CronJobs, Ingresses, NetworkPolicies, ConfigMaps, and Secrets as they happen
- **Secure by Design** - Read-only cluster access, OAuth2 JWT authentication. ConfigMap/Secret data values are never transmitted - only metadata and key names
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
| `agent.rateLimit.eventsPerSecond` | Max events per second to platform | `10` |
| `agent.rateLimit.burstSize` | Burst size for temporary spikes | `20` |
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
  rateLimit:                    # Optional: rate limiting for event streaming
    eventsPerSecond: 10         # Max events/sec (1-1000, default: 10)
    burstSize: 20               # Burst allowance (1-1000, default: 20)
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

| Resource | API Group | Verbs |
|----------|-----------|-------|
| Namespaces | core | list, watch |
| Pods | core | list, watch |
| Services | core | list, watch |
| Nodes | core | list, watch |
| ConfigMaps | core | list, watch |
| Secrets | core | list, watch |
| Deployments | apps | list, watch |
| StatefulSets | apps | list, watch |
| DaemonSets | apps | list, watch |
| Jobs | batch | list, watch |
| CronJobs | batch | list, watch |
| Ingresses | networking.k8s.io | list, watch |
| NetworkPolicies | networking.k8s.io | list, watch |

### Credential Security

- Private keys are **never** stored in the Custom Resource
- Keys must be provided via Kubernetes Secret reference
- Keys are read at runtime and never logged
- All API communication uses TLS
- **ConfigMap and Secret data values are NEVER transmitted** - only metadata and key names are collected

## Service Mesh Compatibility

The operator works with common service meshes. Here's how to configure them:

### Istio

The operator works out of the box with Istio. For automatic sidecar injection, ensure the namespace is labeled:

```bash
kubectl label namespace obsyk-system istio-injection=enabled
```

If you need to exclude the operator from sidecar injection (e.g., for troubleshooting), add the annotation to the Helm values:

```yaml
podAnnotations:
  sidecar.istio.io/inject: "false"
```

#### mTLS Configuration

For strict mTLS environments, create a PeerAuthentication policy:

```yaml
apiVersion: security.istio.io/v1beta1
kind: PeerAuthentication
metadata:
  name: obsyk-operator-mtls
  namespace: obsyk-system
spec:
  selector:
    matchLabels:
      app.kubernetes.io/name: obsyk-operator
  mtls:
    mode: STRICT
```

The operator's outbound connections to the Obsyk platform use TLS and work correctly with Istio's egress policies.

### Linkerd

The operator is compatible with Linkerd. To enable automatic proxy injection:

```bash
kubectl annotate namespace obsyk-system linkerd.io/inject=enabled
```

Or via Helm values:

```yaml
podAnnotations:
  linkerd.io/inject: enabled
```

For Linkerd's default-deny policies, ensure the operator can reach:
- Kubernetes API server (in-cluster)
- Obsyk platform (external HTTPS on port 443)

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
    nodes: 5
    deployments: 30
    statefulsets: 5
    daemonsets: 8
    ingresses: 15
    networkPolicies: 12
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

## Supply Chain Security

Container images are signed and attested for supply chain security compliance.

### Verify Image Signature

All container images are signed using [Sigstore Cosign](https://docs.sigstore.dev/cosign/overview/) with keyless signing:

```bash
# Install cosign: https://docs.sigstore.dev/cosign/installation/
cosign verify ghcr.io/obsyk/obsyk-operator:v0.1.0 \
  --certificate-identity-regexp="https://github.com/Obsyk/obsyk-operator" \
  --certificate-oidc-issuer="https://token.actions.githubusercontent.com"
```

### Verify SLSA Provenance

Images include [SLSA](https://slsa.dev/) build provenance attestations:

```bash
# Using GitHub CLI
gh attestation verify oci://ghcr.io/obsyk/obsyk-operator:v0.1.0 \
  --owner Obsyk
```

### Software Bill of Materials (SBOM)

Each release includes SBOMs (Software Bill of Materials) in two formats:
- **SPDX** (`sbom-spdx.json`) - Linux Foundation standard
- **CycloneDX** (`sbom-cyclonedx.json`) - OWASP standard

Download from the [Releases page](https://github.com/Obsyk/obsyk-operator/releases).

### Vulnerability Scanning

Container images are scanned with [Trivy](https://trivy.dev/) on each release. Critical and high severity vulnerabilities are reported to GitHub Security.

### Rate limiting for large clusters

For clusters with high pod churn, you may want to adjust the rate limit to balance between real-time updates and platform load:

```bash
# Increase rate limit for faster sync in large clusters
helm upgrade obsyk-operator obsyk/obsyk-operator \
  --namespace obsyk-system \
  --set agent.rateLimit.eventsPerSecond=50 \
  --set agent.rateLimit.burstSize=100
```

The default rate limit (10 events/sec, burst 20) works well for most clusters. Increase if you see events being delayed during high activity periods.

## Uninstall

```bash
helm uninstall obsyk-operator -n obsyk-system
kubectl delete namespace obsyk-system
```

## Development

- **Development Guide**: [CLAUDE.md](./CLAUDE.md) - Architecture, build commands, coding conventions
- **Contributing**: [CONTRIBUTING.md](./CONTRIBUTING.md) - How to contribute to this project
- **Changelog**: [CHANGELOG.md](./CHANGELOG.md) - Version history and release notes

## License

Apache License 2.0 - see [LICENSE](./LICENSE) for details.

## Support

- Documentation: [docs.obsyk.ai](https://docs.obsyk.ai)
- Issues: [GitHub Issues](https://github.com/Obsyk/obsyk-operator/issues)
- Email: support@obsyk.com
