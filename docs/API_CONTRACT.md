# Obsyk Operator API Contract

This document defines the API contract between the Obsyk Operator and the Obsyk Platform. It specifies the exact JSON schemas for all data transmitted from the operator to the platform.

## Overview

The operator communicates with the platform via three endpoints:

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/v1/agent/snapshot` | POST | Full cluster state on startup |
| `/api/v1/agent/events` | POST | Individual resource changes |
| `/api/v1/agent/heartbeat` | POST | Periodic health check |

## Event Types

Resource change events use the following types:

```json
{
  "added": "Resource was created",
  "modified": "Resource was updated",
  "deleted": "Resource was removed"
}
```

## Resource Types

The operator collects 20 resource types:

| Resource | Kind Value | Scope |
|----------|------------|-------|
| Namespace | `Namespace` | Cluster |
| Pod | `Pod` | Namespaced |
| Service | `Service` | Namespaced |
| Node | `Node` | Cluster |
| Deployment | `Deployment` | Namespaced |
| StatefulSet | `StatefulSet` | Namespaced |
| DaemonSet | `DaemonSet` | Namespaced |
| Job | `Job` | Namespaced |
| CronJob | `CronJob` | Namespaced |
| Ingress | `Ingress` | Namespaced |
| NetworkPolicy | `NetworkPolicy` | Namespaced |
| ConfigMap | `ConfigMap` | Namespaced |
| Secret | `Secret` | Namespaced |
| PersistentVolumeClaim | `PersistentVolumeClaim` | Namespaced |
| ServiceAccount | `ServiceAccount` | Namespaced |
| Role | `Role` | Namespaced |
| ClusterRole | `ClusterRole` | Cluster |
| RoleBinding | `RoleBinding` | Namespaced |
| ClusterRoleBinding | `ClusterRoleBinding` | Cluster |
| Event | `Event` | Namespaced |

---

## Payload Schemas

### Snapshot Payload

Sent to `/api/v1/agent/snapshot` on operator startup.

```json
{
  "cluster_uid": "string (required) - kube-system namespace UID",
  "cluster_name": "string (required) - human-friendly cluster name",
  "kubernetes_version": "string (optional) - e.g., '1.28.0'",
  "platform": "string (optional) - e.g., 'eks', 'gke', 'kind'",
  "region": "string (optional) - e.g., 'us-east-1'",
  "agent_version": "string (optional) - operator version",
  "namespaces": "[NamespaceInfo]",
  "pods": "[PodInfo]",
  "services": "[ServiceInfo]",
  "nodes": "[NodeInfo]",
  "deployments": "[DeploymentInfo]",
  "statefulsets": "[StatefulSetInfo]",
  "daemonsets": "[DaemonSetInfo]",
  "jobs": "[JobInfo]",
  "cronjobs": "[CronJobInfo]",
  "ingresses": "[IngressInfo]",
  "network_policies": "[NetworkPolicyInfo]",
  "configmaps": "[ConfigMapInfo]",
  "secrets": "[SecretInfo]",
  "pvcs": "[PVCInfo]",
  "service_accounts": "[ServiceAccountInfo]",
  "roles": "[RoleInfo]",
  "cluster_roles": "[RoleInfo]",
  "role_bindings": "[RoleBindingInfo]",
  "cluster_role_bindings": "[RoleBindingInfo]",
  "events": "[EventInfo]"
}
```

### Event Payload

Sent to `/api/v1/agent/events` for each resource change.

```json
{
  "cluster_uid": "string (required)",
  "type": "string (required) - 'added' | 'modified' | 'deleted'",
  "kind": "string (required) - resource type, e.g., 'Pod'",
  "uid": "string (required) - resource UID",
  "name": "string (required) - resource name",
  "namespace": "string (optional) - empty for cluster-scoped resources",
  "object": "object (optional) - full resource data, null for deletes"
}
```

### Heartbeat Payload

Sent to `/api/v1/agent/heartbeat` periodically.

```json
{
  "cluster_uid": "string (required)",
  "agent_version": "string (optional)"
}
```

---

## Resource Schemas

### NamespaceInfo

```json
{
  "uid": "string (required)",
  "name": "string (required)",
  "labels": "object (optional) - key-value pairs",
  "annotations": "object (optional) - filtered, see note",
  "phase": "string (optional) - 'Active' | 'Terminating'",
  "k8s_created_at": "string (optional) - RFC3339 timestamp"
}
```

### PodInfo

```json
{
  "uid": "string (required)",
  "name": "string (required)",
  "namespace": "string (required)",
  "labels": "object (optional)",
  "annotations": "object (optional) - filtered",
  "node_name": "string (optional)",
  "service_account": "string (optional)",
  "phase": "string (optional) - 'Pending' | 'Running' | 'Succeeded' | 'Failed' | 'Unknown'",
  "qos_class": "string (optional) - 'Guaranteed' | 'Burstable' | 'BestEffort'",
  "priority_class_name": "string (optional)",
  "host_ip": "string (optional)",
  "pod_ip": "string (optional)",
  "conditions": "[PodCondition] (optional)",
  "containers": "[ContainerInfo] (optional)",
  "init_containers": "[ContainerInfo] (optional)",
  "volumes": "[VolumeInfo] (optional)",
  "k8s_created_at": "string (optional)"
}
```

#### PodCondition

```json
{
  "type": "string - 'Ready' | 'ContainersReady' | 'Initialized' | 'PodScheduled'",
  "status": "string - 'True' | 'False' | 'Unknown'"
}
```

#### ContainerInfo

```json
{
  "name": "string (required)",
  "image": "string (required)",
  "state": "string (optional) - 'running' | 'waiting' | 'terminated'",
  "ready": "boolean",
  "restart_count": "integer",
  "resources": "ResourceRequirements (optional)",
  "ports": "[ContainerPort] (optional)",
  "volume_mounts": "[VolumeMount] (optional)",
  "env_var_names": "[string] (optional) - names only, NOT values",
  "security_context": "ContainerSecurityContext (optional)"
}
```

#### ResourceRequirements

```json
{
  "cpu_request": "string (optional) - e.g., '100m'",
  "cpu_limit": "string (optional) - e.g., '500m'",
  "memory_request": "string (optional) - e.g., '256Mi'",
  "memory_limit": "string (optional) - e.g., '512Mi'"
}
```

#### ContainerPort

```json
{
  "container_port": "integer (required)",
  "protocol": "string (optional) - 'TCP' | 'UDP' | 'SCTP'",
  "name": "string (optional)"
}
```

#### VolumeMount

```json
{
  "name": "string (required)",
  "mount_path": "string (required)",
  "read_only": "boolean"
}
```

#### ContainerSecurityContext

```json
{
  "privileged": "boolean (optional)",
  "run_as_root": "boolean (optional) - true if runAsUser == 0",
  "capabilities": "[string] (optional) - added capabilities"
}
```

#### VolumeInfo

```json
{
  "name": "string (required)",
  "type": "string (required) - 'emptyDir' | 'configMap' | 'secret' | 'pvc' | 'hostPath' | 'projected' | 'downwardAPI' | 'csi' | 'nfs' | 'ephemeral' | 'unknown'",
  "source": "string (optional) - reference name (PVC name, ConfigMap name, etc.)"
}
```

### ServiceInfo

```json
{
  "uid": "string (required)",
  "name": "string (required)",
  "namespace": "string (required)",
  "labels": "object (optional)",
  "annotations": "object (optional) - filtered",
  "service_type": "string (optional) - 'ClusterIP' | 'NodePort' | 'LoadBalancer' | 'ExternalName'",
  "cluster_ip": "string (optional)",
  "ports": "[PortInfo] (optional)",
  "selector": "object (optional) - label selector",
  "k8s_created_at": "string (optional)"
}
```

#### PortInfo

```json
{
  "name": "string (optional)",
  "protocol": "string (required) - 'TCP' | 'UDP' | 'SCTP'",
  "port": "integer (required)",
  "targetPort": "string (optional) - can be number or named port"
}
```

### NodeInfo

```json
{
  "uid": "string (required)",
  "name": "string (required)",
  "labels": "object (optional)",
  "annotations": "object (optional) - filtered",
  "status": "string (required) - 'Ready' | 'NotReady'",
  "roles": "[string] (optional) - e.g., ['control-plane', 'worker']",
  "kubelet_version": "string (optional)",
  "container_runtime": "string (optional) - e.g., 'containerd://1.6.0'",
  "os_image": "string (optional) - e.g., 'Ubuntu 22.04 LTS'",
  "architecture": "string (optional) - 'amd64' | 'arm64'",
  "cpu_capacity": "string (optional) - e.g., '4'",
  "memory_capacity": "string (optional) - e.g., '16Gi'",
  "pod_capacity": "string (optional) - e.g., '110'",
  "k8s_created_at": "string (optional)"
}
```

### DeploymentInfo

```json
{
  "uid": "string (required)",
  "name": "string (required)",
  "namespace": "string (required)",
  "labels": "object (optional)",
  "annotations": "object (optional) - filtered",
  "replicas": "integer (required) - desired replicas",
  "ready_replicas": "integer",
  "available_replicas": "integer",
  "updated_replicas": "integer",
  "strategy": "string (optional) - 'RollingUpdate' | 'Recreate'",
  "selector": "object (optional) - matchLabels",
  "image": "string (optional) - primary container image",
  "k8s_created_at": "string (optional)"
}
```

### StatefulSetInfo

```json
{
  "uid": "string (required)",
  "name": "string (required)",
  "namespace": "string (required)",
  "labels": "object (optional)",
  "annotations": "object (optional) - filtered",
  "replicas": "integer (required)",
  "ready_replicas": "integer",
  "current_replicas": "integer",
  "update_strategy": "string (optional) - 'RollingUpdate' | 'OnDelete'",
  "service_name": "string (optional) - headless service name",
  "selector": "object (optional)",
  "image": "string (optional)",
  "k8s_created_at": "string (optional)"
}
```

### DaemonSetInfo

```json
{
  "uid": "string (required)",
  "name": "string (required)",
  "namespace": "string (required)",
  "labels": "object (optional)",
  "annotations": "object (optional) - filtered",
  "desired_number_scheduled": "integer",
  "current_number_scheduled": "integer",
  "number_ready": "integer",
  "number_available": "integer",
  "update_strategy": "string (optional) - 'RollingUpdate' | 'OnDelete'",
  "selector": "object (optional)",
  "node_selector": "object (optional)",
  "image": "string (optional)",
  "k8s_created_at": "string (optional)"
}
```

### JobInfo

```json
{
  "uid": "string (required)",
  "name": "string (required)",
  "namespace": "string (required)",
  "labels": "object (optional)",
  "annotations": "object (optional) - filtered",
  "completions": "integer - desired completions",
  "parallelism": "integer",
  "succeeded": "integer",
  "failed": "integer",
  "active": "integer",
  "start_time": "string (optional) - RFC3339",
  "completion_time": "string (optional) - RFC3339",
  "owner_ref": "string (optional) - CronJob name if owned",
  "k8s_created_at": "string (optional)"
}
```

### CronJobInfo

```json
{
  "uid": "string (required)",
  "name": "string (required)",
  "namespace": "string (required)",
  "labels": "object (optional)",
  "annotations": "object (optional) - filtered",
  "schedule": "string (required) - cron expression",
  "suspend": "boolean",
  "concurrency_policy": "string (optional) - 'Allow' | 'Forbid' | 'Replace'",
  "last_schedule_time": "string (optional) - RFC3339",
  "active_jobs": "integer",
  "k8s_created_at": "string (optional)"
}
```

### IngressInfo

```json
{
  "uid": "string (required)",
  "name": "string (required)",
  "namespace": "string (required)",
  "labels": "object (optional)",
  "annotations": "object (optional) - filtered",
  "ingress_class_name": "string (optional)",
  "rules": "[IngressRule] (optional)",
  "tls": "[IngressTLS] (optional)",
  "load_balancer_ips": "[string] (optional)",
  "k8s_created_at": "string (optional)"
}
```

#### IngressRule

```json
{
  "host": "string (optional)",
  "paths": "[IngressPath] (optional)"
}
```

#### IngressPath

```json
{
  "path": "string (optional)",
  "path_type": "string (optional) - 'Exact' | 'Prefix' | 'ImplementationSpecific'",
  "service_name": "string (optional)",
  "service_port": "integer (optional)"
}
```

#### IngressTLS

```json
{
  "hosts": "[string] (optional)",
  "secret_name": "string (optional)"
}
```

### NetworkPolicyInfo

```json
{
  "uid": "string (required)",
  "name": "string (required)",
  "namespace": "string (required)",
  "labels": "object (optional)",
  "annotations": "object (optional) - filtered",
  "pod_selector": "object (optional) - matchLabels",
  "policy_types": "[string] (optional) - 'Ingress' | 'Egress'",
  "ingress_rules": "integer - count of ingress rules",
  "egress_rules": "integer - count of egress rules",
  "k8s_created_at": "string (optional)"
}
```

### ConfigMapInfo

**SECURITY NOTE:** Only metadata and data KEYS are collected. Data VALUES are NEVER transmitted.

```json
{
  "uid": "string (required)",
  "name": "string (required)",
  "namespace": "string (required)",
  "labels": "object (optional)",
  "annotations": "object (optional) - filtered",
  "data_keys": "[string] (optional) - key names only, NO values",
  "binary_keys": "[string] (optional) - binaryData key names only",
  "immutable": "boolean",
  "k8s_created_at": "string (optional)"
}
```

### SecretInfo

**SECURITY CRITICAL:** Only metadata and data KEYS are collected. Data VALUES are NEVER transmitted. Secret data never leaves the cluster.

```json
{
  "uid": "string (required)",
  "name": "string (required)",
  "namespace": "string (required)",
  "labels": "object (optional)",
  "annotations": "object (optional) - filtered",
  "type": "string (optional) - e.g., 'kubernetes.io/tls', 'Opaque'",
  "data_keys": "[string] (optional) - key names only, NEVER values",
  "immutable": "boolean",
  "k8s_created_at": "string (optional)"
}
```

### PVCInfo

```json
{
  "uid": "string (required)",
  "name": "string (required)",
  "namespace": "string (required)",
  "labels": "object (optional)",
  "annotations": "object (optional) - filtered",
  "storage_class_name": "string (optional)",
  "access_modes": "[string] (optional) - 'ReadWriteOnce' | 'ReadOnlyMany' | 'ReadWriteMany'",
  "storage_request": "string (optional) - e.g., '10Gi'",
  "volume_name": "string (optional) - bound PV name",
  "phase": "string (optional) - 'Pending' | 'Bound' | 'Lost'",
  "volume_mode": "string (optional) - 'Filesystem' | 'Block'",
  "k8s_created_at": "string (optional)"
}
```

### ServiceAccountInfo

```json
{
  "uid": "string (required)",
  "name": "string (required)",
  "namespace": "string (required)",
  "labels": "object (optional)",
  "annotations": "object (optional) - filtered",
  "secrets": "[string] (optional) - secret names only",
  "image_pull_secrets": "[string] (optional) - secret names only",
  "automount_service_account_token": "boolean (optional)",
  "k8s_created_at": "string (optional)"
}
```

### RoleInfo

Used for both Role (namespaced) and ClusterRole (cluster-scoped).

```json
{
  "uid": "string (required)",
  "name": "string (required)",
  "namespace": "string (optional) - empty for ClusterRole",
  "labels": "object (optional)",
  "annotations": "object (optional) - filtered",
  "is_cluster": "boolean - true for ClusterRole",
  "rule_count": "integer - number of rules",
  "resources": "[string] - unique resource types covered",
  "k8s_created_at": "string (optional)"
}
```

### RoleBindingInfo

Used for both RoleBinding (namespaced) and ClusterRoleBinding (cluster-scoped).

```json
{
  "uid": "string (required)",
  "name": "string (required)",
  "namespace": "string (optional) - empty for ClusterRoleBinding",
  "labels": "object (optional)",
  "annotations": "object (optional) - filtered",
  "is_cluster": "boolean - true for ClusterRoleBinding",
  "role_ref": "RoleRef (required)",
  "subjects": "[Subject] (optional)",
  "k8s_created_at": "string (optional)"
}
```

#### RoleRef

```json
{
  "kind": "string - 'Role' | 'ClusterRole'",
  "name": "string"
}
```

#### Subject

```json
{
  "kind": "string - 'User' | 'Group' | 'ServiceAccount'",
  "name": "string",
  "namespace": "string (optional) - only for ServiceAccount"
}
```

### EventInfo

```json
{
  "uid": "string (required)",
  "name": "string (required)",
  "namespace": "string (required)",
  "type": "string - 'Normal' | 'Warning'",
  "reason": "string - e.g., 'Scheduled', 'Pulled', 'Created', 'Started', 'Failed', 'OOMKilled'",
  "message": "string",
  "involved_object": "ObjectReference (required)",
  "source": "EventSource",
  "first_timestamp": "string (optional) - RFC3339",
  "last_timestamp": "string (optional) - RFC3339",
  "count": "integer",
  "k8s_created_at": "string (optional)"
}
```

#### ObjectReference

```json
{
  "kind": "string - resource type",
  "name": "string",
  "namespace": "string (optional)",
  "uid": "string (optional)"
}
```

#### EventSource

```json
{
  "component": "string (optional) - e.g., 'kubelet', 'scheduler'",
  "host": "string (optional) - node name"
}
```

---

## Annotation Filtering

The operator filters out noisy annotations before transmitting. The following prefixes are excluded:

- `kubectl.kubernetes.io/`
- `kubernetes.io/`

This reduces payload size and removes operational noise.

---

## Security Considerations

1. **Secret Data**: Secret values are NEVER transmitted. Only key names are collected.
2. **ConfigMap Data**: ConfigMap values are NEVER transmitted. Only key names are collected.
3. **Environment Variables**: Only environment variable names are collected, NEVER values.
4. **Credentials**: OAuth2 credentials are stored in Kubernetes Secrets and never exposed in payloads.

---

## Versioning

The operator version is included in snapshot and heartbeat payloads via the `agent_version` field. This can be used for compatibility tracking and debugging.

---

## Related Documentation

- [CLAUDE.md](../CLAUDE.md) - Development guide and architecture
- [ARCHITECTURE.md](./ARCHITECTURE.md) - System architecture diagrams
