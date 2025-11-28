# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Kubernetes operator that deploys a "Deploy & Forget" observability agent in customer clusters. The agent streams cluster metadata (Namespaces, Pods, Services) to the Obsyk SaaS platform.

**This is a public repository** - customers install this operator in their Kubernetes clusters.

## Tech Stack

- **Language**: Go 1.22+
- **Framework**: Kubebuilder v4 (controller-runtime)
- **Build**: Make, Docker
- **Installation**: Helm chart
- **CI/CD**: GitHub Actions
- **Container Registry**: GHCR (ghcr.io/obsyk/obsyk-operator)
- **Testing**: Ginkgo/Gomega (controller-runtime standard), envtest

## Repository Structure

```
/api/
  /v1/
    obsykagent_types.go      # CRD struct definitions
    groupversion_info.go     # API group registration
/cmd/
  /main.go                   # Operator entrypoint
/internal/
  /controller/
    obsykagent_controller.go # Main reconciliation logic
    suite_test.go            # Controller tests
  /transport/
    client.go                # HTTP client for platform communication
/config/
  /crd/                      # Generated CRD manifests
  /rbac/                     # RBAC manifests (ClusterRole, etc.)
  /manager/                  # Controller manager deployment
  /default/                  # Kustomize default overlay
/charts/
  /obsyk-operator/           # Helm chart
    Chart.yaml
    values.yaml
    templates/
/.github/
  /workflows/
    ci.yml                   # PR checks
    release.yml              # Release automation
/Makefile                    # Build commands
/Dockerfile                  # Multi-stage build
```

## Architecture

### Custom Resource Definition (CRD)

**ObsykAgent** - Single CRD to configure the agent:

```yaml
apiVersion: obsyk.io/v1
kind: ObsykAgent
metadata:
  name: obsyk-agent
  namespace: obsyk-system
spec:
  # Platform endpoint (Obsyk SaaS)
  platformURL: "https://api.obsyk.com"

  # Logical cluster identifier
  clusterName: "production-us-east-1"

  # API key stored in Secret (NEVER plain-text)
  apiKeySecretRef:
    name: obsyk-api-key
    key: token
status:
  conditions:
    - type: Available
      status: "True"
    - type: Syncing
      status: "True"
  lastSyncTime: "2024-01-15T10:30:00Z"
```

### Controller Logic

1. **Initialization (Snapshot)**
   - On startup or CR creation, perform full List of Namespaces, Pods, Services
   - Send bulk "Snapshot" payload to platform

2. **Event-Driven Watch (Delta)**
   - Use controller-runtime's `Watches()` (NOT manual client-go watcher loops)
   - Implement custom EventHandler to capture resource changes
   - Send "Event" payload (Added/Updated/Deleted) immediately

3. **Concurrency**
   - Handle high event volume without blocking the work queue
   - Use buffered channels or batch processing if needed

### Security Requirements

- **RBAC**: Read-only access only (list, watch) for Pods, Services, Namespaces
- **API Key**: Must be read from Secret at runtime, NEVER logged or exposed
- **Error Handling**: Exponential backoff for 5xx errors from platform

## Build Commands

```bash
# Install dependencies
make deps

# Generate CRD manifests and code
make generate
make manifests

# Run linters
make lint

# Run tests
make test

# Build binary
make build

# Build Docker image
make docker-build IMG=ghcr.io/obsyk/obsyk-operator:dev

# Push Docker image
make docker-push IMG=ghcr.io/obsyk/obsyk-operator:dev

# Install CRDs in cluster
make install

# Deploy operator to cluster
make deploy IMG=ghcr.io/obsyk/obsyk-operator:dev

# Uninstall from cluster
make uninstall
```

## Local Development

### Prerequisites
- Go 1.22+
- Docker
- kubectl configured to a test cluster
- Kind or Minikube for local testing

### Running Locally

```bash
# Start a local cluster
kind create cluster --name obsyk-dev

# Install CRDs
make install

# Run controller locally (outside cluster)
make run

# In another terminal, create test resources
kubectl apply -f config/samples/
```

### Testing

```bash
# Unit tests with envtest
make test

# Generate coverage report
make test-coverage

# Run specific test
go test ./internal/controller/... -v -run TestObsykAgentReconcile
```

## License

Apache License 2.0 - This is a public repository for customer installation.

```
// Copyright (c) Obsyk. All rights reserved.
// Licensed under the Apache License, Version 2.0.
```

## Coding Conventions

### General
- All new code must include unit tests
- Controller tests use envtest (fake K8s API server)
- No PR merges without passing CI

### Go
- Use `gofmt` and `golangci-lint`
- Table-driven tests
- Error wrapping: `fmt.Errorf("reconciling obsykagent: %w", err)`
- Structured logging with controller-runtime's `logr`
- Follow controller-runtime patterns and idioms

### Controller Patterns

**Use Watches(), not manual informers:**
```go
// GOOD - Uses controller-runtime's declarative watch
func (r *ObsykAgentReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&obsykv1.ObsykAgent{}).
        Watches(&corev1.Pod{}, handler.EnqueueRequestsFromMapFunc(r.findAgentForPod)).
        Complete(r)
}

// BAD - Manual client-go watcher loop
// Don't do this - let controller-runtime manage the informer lifecycle
```

**Read secrets at runtime:**
```go
// GOOD - Read secret in reconcile loop
secret := &corev1.Secret{}
if err := r.Get(ctx, secretRef, secret); err != nil {
    return ctrl.Result{}, err
}
apiKey := string(secret.Data[agent.Spec.APIKeySecretRef.Key])

// BAD - Store API key in memory or log it
log.Info("Using API key", "key", apiKey) // NEVER DO THIS
```

## Issue Tracking

GitHub Issues for bug/feature management.

- **Branch naming**: `feature/GH-123-description` or `fix/GH-123-description`
- **Commit messages**: `[GH-123] feat: add feature X`
- **PR closing**: Use `fixes #123` in PR description to auto-close issues

## CI/CD Pipeline

### PR Checks (ci.yml)
- Go: gofmt, go vet, golangci-lint
- Tests: Unit tests with envtest
- Build: Verify Docker image builds

### Release (release.yml) - on tag push
- Build multi-arch Docker image (amd64, arm64)
- Security scan with Trivy
- Push to GHCR with version tags
- Package and publish Helm chart to GitHub Pages

## Git Workflow

- Trunk-based development
- Short-lived feature branches → PR → squash merge to `main`
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

## Platform Communication Protocol

### Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/v1/agents/snapshot` | Full cluster state on startup |
| POST | `/api/v1/agents/events` | Individual resource changes |
| POST | `/api/v1/agents/heartbeat` | Periodic health check |

### Payload Formats

**Snapshot Payload:**
```json
{
  "clusterName": "production-us-east-1",
  "timestamp": "2024-01-15T10:30:00Z",
  "namespaces": [...],
  "pods": [...],
  "services": [...]
}
```

**Event Payload:**
```json
{
  "clusterName": "production-us-east-1",
  "timestamp": "2024-01-15T10:30:00Z",
  "eventType": "ADDED|UPDATED|DELETED",
  "resourceType": "Pod|Service|Namespace",
  "resource": {...}
}
```

### Authentication
- Header: `Authorization: Bearer <api-key>`
- API key sourced from Kubernetes Secret at runtime

### Error Handling
- 2xx: Success
- 4xx: Client error (log and don't retry)
- 5xx: Server error (exponential backoff with jitter)
