# Obsyk Operator

[![Artifact Hub](https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/obsyk-operator)](https://artifacthub.io/packages/helm/obsyk-operator/obsyk-operator)

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
  rateLimit:
    eventsPerSecond: 10
    burstSize: 20
```

## Security

### RBAC Permissions

The operator requires **read-only** access to cluster resources:

| Resource | API Group | Verbs |
|----------|-----------|-------|
| Namespaces, Pods, Services, Nodes, ConfigMaps, Secrets | core | list, watch |
| Deployments, StatefulSets, DaemonSets | apps | list, watch |
| Jobs, CronJobs | batch | list, watch |
| Ingresses, NetworkPolicies | networking.k8s.io | list, watch |

### Credential Security

- Private keys are **never** stored in the Custom Resource
- Keys must be provided via Kubernetes Secret reference
- All API communication uses TLS
- **ConfigMap and Secret data values are NEVER transmitted** - only metadata and key names are collected

## Supply Chain Security

- **Signed Images** - All container images are signed with [Sigstore Cosign](https://docs.sigstore.dev/cosign/overview/)
- **SLSA Provenance** - Images include [SLSA](https://slsa.dev/) build attestations
- **SBOM** - Software Bill of Materials available in SPDX and CycloneDX formats
- **Vulnerability Scanning** - Images scanned with Trivy on each release

## Uninstall

```bash
helm uninstall obsyk-operator -n obsyk-system
kubectl delete namespace obsyk-system
```

## Links

- [Website](https://obsyk.ai)
- [Documentation](https://docs.obsyk.ai)
- [GitHub Repository](https://github.com/Obsyk/obsyk-operator)
- [Support](mailto:support@obsyk.com)

## License

Apache License 2.0
