# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Kubernetes operator that deploys a "Deploy & Forget" observability agent in customer clusters. The agent streams cluster metadata (Namespaces, Pods, Services, Nodes, Deployments, StatefulSets, DaemonSets, Jobs, CronJobs, Ingresses, NetworkPolicies, ConfigMaps, Secrets) to the Obsyk SaaS platform using event-driven delta streaming.

**Security Note**: ConfigMap and Secret ingestion collects only metadata and data keys - never values. This is critical for security.

**This is a public repository** - customers install this operator in their Kubernetes clusters.

## Tech Stack

- **Language**: Go 1.24+
- **Framework**: Kubebuilder v4 (controller-runtime)
- **Build**: Earthly (containerized builds)
- **Installation**: Helm chart
- **CI/CD**: GitHub Actions
- **Container Registry**: GHCR (ghcr.io/obsyk/obsyk-operator)
- **Testing**: Standard Go testing with envtest for Kubernetes integration

## Repository Structure

```
/api/
  /v1/
    obsykagent_types.go       # CRD struct definitions (ObsykAgent spec/status)
    groupversion_info.go      # API group registration
    zz_generated.deepcopy.go  # Generated deep copy methods
/cmd/
  main.go                     # Operator entrypoint
/internal/
  /auth/
    token.go                  # OAuth2 JWT Bearer token management (thread-safe)
    token_test.go             # Token manager tests
  /controller/
    obsykagent_controller.go  # Main reconciliation logic (thread-safe)
    obsykagent_controller_test.go  # Controller concurrency tests
  /ingestion/
    types.go                  # Event types, interfaces (ResourceEvent, EventSender)
    manager.go                # IngestionManager - coordinates informers (thread-safe)
    pod_ingester.go           # Pod SharedInformer event handler
    service_ingester.go       # Service SharedInformer event handler
    namespace_ingester.go     # Namespace SharedInformer event handler
    node_ingester.go          # Node SharedInformer event handler
    deployment_ingester.go    # Deployment SharedInformer event handler
    statefulset_ingester.go   # StatefulSet SharedInformer event handler
    daemonset_ingester.go     # DaemonSet SharedInformer event handler
    ingress_ingester.go       # Ingress SharedInformer event handler
    networkpolicy_ingester.go # NetworkPolicy SharedInformer event handler
    job_ingester.go           # Job SharedInformer event handler
    cronjob_ingester.go       # CronJob SharedInformer event handler
    configmap_ingester.go     # ConfigMap SharedInformer event handler (metadata only)
    secret_ingester.go        # Secret SharedInformer event handler (metadata only, NEVER values)
    *_test.go                 # Unit tests for each component
    integration_test.go       # Integration tests with envtest
  /transport/
    types.go                  # API payload types (SnapshotPayload, EventPayload)
    client.go                 # HTTP client for platform communication (thread-safe)
    client_test.go            # Client tests including concurrency tests
/config/
  /crd/                       # Generated CRD manifests
  /rbac/                      # RBAC manifests (ClusterRole, etc.)
  /manager/                   # Controller manager deployment
  /default/                   # Kustomize default overlay
  /samples/                   # Sample CR files
/charts/
  /obsyk-operator/            # Helm chart
    Chart.yaml
    values.yaml
    /crds/                    # CRD YAML for Helm
    /templates/               # Helm templates
/.github/
  /workflows/
    ci.yml                    # PR checks (lint, test, build)
    release.yml               # Release automation
    e2e.yml                   # E2E tests on Kind cluster
/docs/
  TEST_PLAN_GH11.md           # Event-driven streaming test plan
/Earthfile                    # Earthly build definitions
/Dockerfile                   # Runtime image (used by Earthly)
```

## Architecture

### High-Level Data Flow

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         Kubernetes Cluster                               │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  ┌──────────────┐    ┌──────────────────────────────────────────────┐   │
│  │  kube-system │    │           obsyk-operator (Pod)                │   │
│  │  (namespace) │    │  ┌────────────────────────────────────────┐  │   │
│  │              │    │  │        ObsykAgentReconciler            │  │   │
│  │  UID used as │───▶│  │  - Manages lifecycle                   │  │   │
│  │  ClusterUID  │    │  │  - Creates IngestionManager            │  │   │
│  └──────────────┘    │  │  - Handles initial snapshot            │  │   │
│                      │  └────────────────────────────────────────┘  │   │
│  ┌──────────────┐    │                    │                          │   │
│  │   Secrets    │    │                    ▼                          │   │
│  │              │    │  ┌────────────────────────────────────────┐  │   │
│  │ client_id    │───▶│  │          IngestionManager              │  │   │
│  │ private_key  │    │  │  ┌─────────────────────────────────┐   │  │   │
│  └──────────────┘    │  │  │  SharedInformerFactory          │   │  │   │
│                      │  │  │  (Pod, Service, Namespace,      │   │  │   │
│                      │  │  │   Node, Deploy, STS, DS,        │   │  │   │
│                      │  │  │   Ingress, NetworkPolicy)       │   │  │   │
│  ┌──────────────┐    │  │  └─────────────────────────────────┘   │  │   │
│  │  Pods        │    │  │                 │                      │  │   │
│  │  Services    │◀───│  │                 ▼                      │  │   │
│  │  Namespaces  │    │  │  ┌─────────────────────────────────┐   │  │   │
│  │  Nodes, etc. │    │  │  │  Event Channel (buffered)       │   │  │   │
│  └──────────────┘    │  │  │  ResourceEvent{Type,Kind,UID,..}│   │  │   │
│                      │  │  └─────────────────────────────────┘   │  │   │
│                      │  │                 │                      │  │   │
│                      │  │                 ▼                      │  │   │
│                      │  │  ┌─────────────────────────────────┐   │  │   │
│                      │  │  │  Event Processor (goroutine)    │   │  │   │
│                      │  │  │  - Converts to EventPayload     │   │  │   │
│                      │  │  │  - Sends via transport.Client   │   │  │   │
│                      │  │  └─────────────────────────────────┘   │  │   │
│                      │  └────────────────────────────────────────┘  │   │
│                      │                    │                          │   │
│                      │                    ▼                          │   │
│                      │  ┌────────────────────────────────────────┐  │   │
│                      │  │         transport.Client               │  │   │
│                      │  │  - HTTP client with retry              │  │   │
│                      │  │  - TokenManager for OAuth2             │  │   │
│                      │  └────────────────────────────────────────┘  │   │
│                      └──────────────────────────────────────────────┘   │
│                                           │                              │
└───────────────────────────────────────────│──────────────────────────────┘
                                            │
                                            ▼
                              ┌──────────────────────────┐
                              │    Obsyk Platform        │
                              │                          │
                              │  POST /oauth/token       │
                              │  POST /api/v1/agent/*    │
                              └──────────────────────────┘
```

### Component Details

#### 1. ObsykAgent CRD (`api/v1/obsykagent_types.go`)

The single Custom Resource that configures the agent:

```yaml
apiVersion: obsyk.io/v1
kind: ObsykAgent
metadata:
  name: obsyk-agent
  namespace: obsyk-system
spec:
  platformURL: "https://api.obsyk.ai"      # Platform endpoint
  clusterName: "production-us-east-1"      # Human-friendly name
  credentialsSecretRef:
    name: obsyk-credentials                # OAuth2 credentials secret
  syncInterval: "5m"                       # Heartbeat interval
  rateLimit:                               # Optional: rate limiting config
    eventsPerSecond: 10                    # Default: 10, range: 1-1000
    burstSize: 20                          # Default: 20, range: 1-1000
status:
  clusterUID: "abc123-def456"              # Auto-detected from kube-system
  conditions:
    - type: Available
      status: "True"
    - type: Syncing
      status: "True"
  lastSyncTime: "2024-01-15T10:30:00Z"
  lastSnapshotTime: "2024-01-15T10:25:00Z"
  resourceCounts:
    namespaces: 10
    pods: 100
    services: 20
```

**Required Secret Format:**
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

#### 2. Controller (`internal/controller/obsykagent_controller.go`)

The ObsykAgentReconciler manages the agent lifecycle:

**Thread-Safety:**
- `agentClientsMu sync.RWMutex` protects the `agentClients` map
- Uses double-checked locking in `getOrCreateAgentClient()` for optimal performance
- Credentials fetched outside the lock to avoid I/O under mutex

**Key Methods:**
- `Reconcile()` - Main reconciliation loop, handles create/update/delete
- `getOrCreateAgentClient()` - Gets or creates transport client with OAuth2
- `sendSnapshot()` - Sends full cluster state on initial sync
- `sendHeartbeat()` - Periodic health check

#### 3. Ingestion Package (`internal/ingestion/`)

Event-driven resource watching using SharedInformerFactory:

**IngestionManager** (`manager.go`):
- Creates and manages SharedInformerFactory
- Coordinates Pod, Service, Namespace ingesters
- Event channel with configurable buffer (default: 1000)
- Rate limiting for event sending (token bucket algorithm, default: 10/sec, burst: 20)
- Graceful shutdown with event draining

**Ingesters** (pod/service/namespace/node/deployment/statefulset/daemonset/ingress/networkpolicy_ingester.go):
- Implement `cache.ResourceEventHandler` interface
- Non-blocking event sends (drop with warning if channel full)
- Skip updates with same ResourceVersion (deduplication)

**Event Flow:**
```
K8s API Server ──watch──▶ SharedInformer ──▶ Ingester.OnAdd/Update/Delete
                                                        │
                                                        ▼
                                               ResourceEvent{Type, Kind, UID, ...}
                                                        │
                                                        ▼
                                               Event Channel (buffered)
                                                        │
                                                        ▼
                                               Event Processor Goroutine
                                                        │
                                                        ▼
                                               Rate Limiter (token bucket)
                                                        │
                                                        ▼
                                               transport.Client.SendEvent()
```

#### 4. Transport Package (`internal/transport/`)

HTTP client for platform communication:

**Client** (`client.go`):
- Thread-safe with `sync.RWMutex` protecting `tokenProvider`
- Retry with exponential backoff (5xx errors only)
- Methods: `SendSnapshot()`, `SendEvent()`, `SendHeartbeat()`

**Types** (`types.go`):
- `SnapshotPayload` - Full cluster state
- `EventPayload` - Single resource change (ADDED/UPDATED/DELETED)
- `HeartbeatPayload` - Periodic health check
- Helper functions: `NewPodInfo()`, `NewServiceInfo()`, `NewNamespaceInfo()`

#### 5. Auth Package (`internal/auth/`)

OAuth2 JWT Bearer authentication (RFC 7523):

**TokenManager** (`token.go`):
- Thread-safe with `sync.RWMutex`
- Creates JWT assertions signed with ECDSA P-256 private key
- Caches access tokens, refreshes before expiry
- `UpdateCredentials()` for hot-reload from secret changes

### Thread-Safety Patterns

All major components are thread-safe:

| Component | Protection | Pattern |
|-----------|------------|---------|
| `ObsykAgentReconciler.agentClients` | `sync.RWMutex` | Double-checked locking |
| `transport.Client.tokenProvider` | `sync.RWMutex` | Lock on update, RLock on read |
| `auth.TokenManager` | `sync.RWMutex` | Protects token cache and credentials |
| `ingestion.Manager` | `sync.RWMutex` | Protects started state |

### Security Requirements

- **RBAC**: Read-only access only (list, watch) for Pods, Services, Namespaces
- **Credentials**: OAuth2 credentials read from Secret at runtime, never from CR
- **Private Key**: NEVER logged or exposed, stored only in Kubernetes Secret
- **Token Management**: Access tokens cached in memory, refreshed before expiry
- **Error Handling**: Exponential backoff for 5xx errors from platform

## Build Commands

```bash
# Run all CI checks (lint, test, build)
earthly +ci

# Run linters only
earthly +lint

# Run tests only
earthly +test

# Build binary only
earthly +build

# Build Docker image
earthly +docker

# Build and push Docker image
earthly --push +docker --VERSION=v1.0.0

# Generate CRD manifests
earthly +manifests
```

## Local Development

### Prerequisites
- Go 1.24+
- Earthly
- Docker (OrbStack recommended for Mac)
- kubectl configured to a test cluster
- Kind or Minikube for local testing

### Running Locally

```bash
# Start a local cluster
kind create cluster --name obsyk-dev

# Build and run tests
earthly +ci

# Run controller locally (outside cluster)
go run ./cmd/main.go

# In another terminal, create test resources
kubectl apply -f config/samples/
```

### Testing

```bash
# All tests
go test ./... -v

# With race detection (recommended)
go test -race ./... -v

# Specific package
go test ./internal/controller/... -v

# With coverage
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

### Test Organization

| Package | Test Files | Coverage Focus |
|---------|------------|----------------|
| `internal/auth` | `token_test.go` | JWT creation, token refresh, credentials parsing |
| `internal/controller` | `obsykagent_controller_test.go` | Concurrent map access, reconcile lifecycle |
| `internal/transport` | `client_test.go` | HTTP requests, retry logic, concurrent sends |
| `internal/ingestion` | `*_test.go`, `integration_test.go` | Event handling, informer sync, graceful shutdown |

## License

Apache License 2.0 - This is a public repository for customer installation.

```
// Copyright (c) Obsyk. All rights reserved.
// Licensed under the Apache License, Version 2.0.
```

## Coding Conventions

### General
- All new code must include unit tests
- Use `-race` flag when testing concurrent code
- No PR merges without passing CI

### Go
- Use `gofmt` and `golangci-lint`
- Table-driven tests
- Error wrapping: `fmt.Errorf("reconciling obsykagent: %w", err)`
- Structured logging with controller-runtime's `logr`
- Follow controller-runtime patterns and idioms

### Thread-Safety Patterns

**Double-checked locking for map access:**
```go
// Fast path - read lock
r.mu.RLock()
if val, ok := r.cache[key]; ok {
    r.mu.RUnlock()
    return val, nil
}
r.mu.RUnlock()

// Slow path - write lock
r.mu.Lock()
defer r.mu.Unlock()

// Double-check after acquiring write lock
if val, ok := r.cache[key]; ok {
    return val, nil
}

// Create new value
val := createValue()
r.cache[key] = val
return val, nil
```

**Avoid I/O under lock:**
```go
// GOOD - fetch outside lock
data, err := fetchData(ctx)  // I/O outside lock
if err != nil {
    return err
}

r.mu.Lock()
r.cache[key] = data
r.mu.Unlock()

// BAD - I/O under lock
r.mu.Lock()
data, err := fetchData(ctx)  // Blocks other goroutines!
r.cache[key] = data
r.mu.Unlock()
```

### Controller Patterns

**Use Watches(), not manual informers for reconciler triggers:**
```go
// GOOD - Uses controller-runtime's declarative watch
func (r *ObsykAgentReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&obsykv1.ObsykAgent{}).
        Watches(&corev1.Pod{}, handler.EnqueueRequestsFromMapFunc(r.findAgentForPod)).
        Complete(r)
}
```

**Use SharedInformerFactory for event streaming:**
```go
// GOOD - Efficient shared informers
factory := informers.NewSharedInformerFactory(clientset, 0)
podInformer := factory.Core().V1().Pods().Informer()
podInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
    AddFunc:    ingester.OnAdd,
    UpdateFunc: ingester.OnUpdate,
    DeleteFunc: ingester.OnDelete,
})
```

**Read credentials at runtime:**
```go
// GOOD - Read credentials from secret and use token manager
secret := &corev1.Secret{}
if err := r.Get(ctx, secretRef, secret); err != nil {
    return ctrl.Result{}, err
}
creds, err := auth.ParseCredentials(secret.Data)
tokenManager := auth.NewTokenManager(platformURL, creds, httpClient, logger)

// BAD - Store private key in memory or log it
log.Info("Using private key", "key", privateKey) // NEVER DO THIS
```

## Issue Tracking

GitHub Issues for bug/feature management.

- **Branch naming**: `feature/GH-123-description` or `fix/GH-123-description`
- **Commit messages**: `[GH-123] feat: add feature X`
- **PR closing**: Use `fixes #123` in PR description to auto-close issues

## CI/CD Pipeline

### PR Checks (ci.yml)
- Earthly `+ci` target runs: gofmt, go vet, golangci-lint, tests, build
- Helm chart linting and template validation

### E2E Tests (e2e.yml) - on push to main
- Creates Kind cluster
- Builds operator image and loads into Kind
- Deploys operator via Helm with pre-registered credentials
- Verifies ObsykAgent syncs successfully (status.lastSyncTime set)
- Cleans up Kind cluster

**Required Secrets (pre-register cluster in platform first):**
- `E2E_PLATFORM_URL`: Platform API URL (e.g., https://api.staging.obsyk.ai)
- `E2E_CLIENT_ID`: OAuth client ID from cluster registration
- `E2E_PRIVATE_KEY`: Base64-encoded PEM private key
- `E2E_CLUSTER_NAME`: Cluster name registered in platform

**Setup Instructions:**
```bash
# 1. Generate keypair
openssl ecparam -genkey -name prime256v1 -noout -out private.pem
openssl ec -in private.pem -pubout -out public.pem

# 2. Register cluster in platform UI (upload public.pem, get client_id)

# 3. Add secrets to GitHub
gh secret set E2E_PLATFORM_URL --body "https://api.staging.obsyk.ai"
gh secret set E2E_CLIENT_ID --body "<client-id-from-platform>"
gh secret set E2E_CLUSTER_NAME --body "e2e-test-cluster"
cat private.pem | base64 | gh secret set E2E_PRIVATE_KEY
```

### Release (release.yml) - on tag push
- Earthly builds and pushes Docker image to GHCR
- Security scan with Trivy
- Package and publish Helm chart to GitHub Pages

## Git Workflow

- Trunk-based development
- Short-lived feature branches -> PR -> squash merge to `main`
- Releases via semantic versioning tags (v1.0.0)
- Require PR reviews before merge

**IMPORTANT FOR CLAUDE**: NEVER commit directly to `main`. Always:
1. Create feature branch: `git checkout -b feature/GH-<issue>-<description>`
2. Make commits on the branch
3. Push branch and create PR via `gh pr create`
4. Wait for CI checks and review before merge

## Commit Message Format

```
[GH-123] <type>: <description>

<optional body>
```

Types: `feat`, `fix`, `refactor`, `test`, `docs`, `chore`

Examples:
- `[GH-1] feat: add ObsykAgent CRD and controller scaffold`
- `[GH-5] fix: handle secret not found error gracefully`
- `[GH-12] feat: add thread-safety to transport client and controller`

## Platform Communication Protocol

### Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/oauth/token` | Exchange JWT assertion for access token |
| POST | `/api/v1/agent/snapshot` | Full cluster state on startup |
| POST | `/api/v1/agent/events` | Individual resource changes |
| POST | `/api/v1/agent/heartbeat` | Periodic health check |

### Payload Formats

**Snapshot Payload:**
```json
{
  "cluster_uid": "abc123-def456",
  "cluster_name": "production-us-east-1",
  "kubernetes_version": "1.28.0",
  "platform": "eks",
  "region": "us-east-1",
  "agent_version": "0.1.0",
  "namespaces": [
    {"uid": "...", "name": "default", "labels": {...}, "phase": "Active"}
  ],
  "pods": [
    {"uid": "...", "name": "nginx", "namespace": "default", "containers": [...]}
  ],
  "services": [
    {"uid": "...", "name": "nginx-svc", "namespace": "default", "ports": [...]}
  ]
}
```

**Event Payload:**
```json
{
  "cluster_uid": "abc123-def456",
  "type": "ADDED",
  "kind": "Pod",
  "uid": "pod-uid-123",
  "name": "nginx",
  "namespace": "default",
  "object": {"uid": "...", "name": "nginx", ...}
}
```

**Heartbeat Payload:**
```json
{
  "cluster_uid": "abc123-def456",
  "agent_version": "0.1.0"
}
```

### Authentication (OAuth2 JWT Bearer)

The operator uses OAuth2 JWT Bearer Assertion (RFC 7523) for authentication:

1. **Setup**: Customer generates ECDSA P-256 key pair locally
2. **Registration**: Customer uploads public key to platform, receives `client_id`
3. **Authentication Flow**:
   - Operator creates JWT assertion signed with private key
   - Operator exchanges assertion for access token via `/oauth/token`
   - Access token is used for all API calls (expires in 1 hour)
   - Token is automatically refreshed before expiry

**JWT Assertion Claims:**
```json
{
  "iss": "<client_id>",
  "sub": "<client_id>",
  "aud": ["https://api.obsyk.ai"],
  "iat": 1705312200,
  "exp": 1705312500,
  "jti": "<unique-id>"
}
```

**Token Request:**
```bash
curl -X POST https://api.obsyk.ai/oauth/token \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "grant_type=urn:ietf:params:oauth:grant-type:jwt-bearer" \
  -d "assertion=<signed-jwt>"
```

**API Request:**
```bash
curl -X POST https://api.obsyk.ai/api/v1/agent/snapshot \
  -H "Authorization: Bearer <access-token>" \
  -H "Content-Type: application/json" \
  -d '{"cluster_uid": "...", ...}'
```

### Error Handling

| Status | Meaning | Action |
|--------|---------|--------|
| 2xx | Success | Continue |
| 401 | Auth error | Refresh token and retry once |
| 4xx | Client error | Log and don't retry |
| 5xx | Server error | Exponential backoff with jitter (max 5 retries) |
