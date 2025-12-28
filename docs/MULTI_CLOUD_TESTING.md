# Multi-Cloud Testing Strategy

This document describes the testing and validation strategy for ensuring the obsyk-operator works across different Kubernetes distributions and cloud providers.

## Testing Tiers

| Tier | Environment | Trigger | Cost |
|------|-------------|---------|------|
| 1 | Unit tests | Every PR | $0 |
| 2 | Kind basic E2E | Push to main | $0 |
| 3 | K8s version matrix (1.28, 1.30, 1.32) | Release only | $0 |
| 4 | Cloud providers (EKS, AKS, GKE) | Manual on-demand | ~$2-5 per provider |

## Release-Time Testing

When a release tag is pushed, the following tests run automatically before publishing:

### Kubernetes Version Matrix

Tests the operator on three Kubernetes versions using Kind clusters:

- **K8s 1.28** - Oldest supported LTS version
- **K8s 1.30** - Middle ground
- **K8s 1.32** - Current stable

If any version fails, the release is blocked.

### Pod Security Standards (Restricted)

Tests the operator under Kubernetes Pod Security Standards `restricted` mode, which approximates OpenShift/ROSA Security Context Constraints. This validates:

- `runAsNonRoot: true`
- No privileged containers
- No host networking/ports
- Read-only root filesystem
- Dropped capabilities

## Cloud Provider Testing

Cloud tests are triggered manually via GitHub Actions `workflow_dispatch` to validate on real managed Kubernetes services.

### Running Cloud Tests

1. Go to **Actions** → **Cloud E2E**
2. Click **Run workflow**
3. Select provider: `eks`, `aks`, `gke`, or `all`
4. Optionally specify a version (defaults to latest release)

### What Gets Tested

Each cloud provider test validates:

1. **Deployment Health** - Pod starts without restarts
2. **RBAC** - Operator can list/watch all 20 resource types
3. **Initial Sync** - Snapshot sent to platform successfully
4. **Resource Counts** - Status reflects actual cluster resources
5. **Platform Connectivity** - Heartbeat/sync working
6. **Cloud-Specific** - No credential errors (operator shouldn't need cloud creds)

### Cost Estimates

| Provider | Cluster Type | Approximate Cost |
|----------|--------------|------------------|
| EKS | t3.medium × 2 | ~$2/hour |
| AKS | Standard_B2s × 2 | ~$1.50/hour |
| GKE | e2-small × 2 | ~$1/hour |

Tests typically complete in 20-30 minutes per provider.

### Required Secrets

Set these in GitHub repository secrets:

```bash
# GitHub App (for pushing validation log commits)
CI_APP_ID          # GitHub App ID
CI_APP_PRIVATE_KEY # GitHub App private key (PEM format)

# AWS (for EKS)
AWS_ACCESS_KEY_ID
AWS_SECRET_ACCESS_KEY

# Azure (for AKS)
AZURE_CREDENTIALS  # Service principal JSON

# GCP (for GKE)
GCP_CREDENTIALS    # Service account key JSON

# Platform (for E2E)
E2E_PLATFORM_URL   # e.g., https://api.staging.obsyk.ai
E2E_CLIENT_ID      # OAuth client ID
E2E_PRIVATE_KEY    # Base64-encoded PEM private key
E2E_CLUSTER_NAME   # Cluster name registered in platform
```

## Validation History Log

All test results are logged to `.github/cloud-validation.json` for full transparency and audit trail.

### Log Format

```json
{
  "schema_version": 1,
  "description": "Validation history for obsyk-operator",
  "validations": [
    {
      "type": "k8s-version",
      "version": "v0.2.0",
      "k8s_version": "1.28",
      "result": "pass",
      "timestamp": "2025-01-15T10:30:00Z",
      "run_id": "12345678",
      "run_url": "https://github.com/obsyk/obsyk-operator/actions/runs/12345678"
    },
    {
      "type": "cloud-provider",
      "version": "v0.2.0",
      "provider": "eks",
      "k8s_version": "1.30",
      "result": "pass",
      "timestamp": "2025-01-15T11:00:00Z",
      "run_id": "12345679",
      "run_url": "https://github.com/obsyk/obsyk-operator/actions/runs/12345679"
    }
  ]
}
```

### Entry Types

| Type | Description |
|------|-------------|
| `k8s-version` | Kubernetes version matrix test (1.28, 1.30, 1.32) |
| `cloud-provider` | Cloud provider test (EKS, AKS, GKE) |
| `pss-restricted` | Pod Security Standards restricted mode test |

### Benefits

- **Audit Trail**: Every test is logged with timestamp and run ID
- **Verification**: Click `run_url` to see the actual GitHub Actions run
- **History**: View all past validations, not just the latest
- **Staleness Detection**: Badges show yellow if validated version doesn't match latest release

## Compatibility Badges

Badges are dynamically generated from the validation log:

- **Green**: `EKS | v0.2.0 ✓` - Latest release validated
- **Yellow**: `EKS | v0.1.0 ✓ (not latest)` - Validated, but on older release
- **Red**: `EKS | v0.2.0 ✗` - Latest release failed
- **Gray**: `EKS | untested` - No validation history

Badges are stored in `.github/badges/*.json` and rendered via shields.io endpoint URLs.

## Local Testing

Run these locally before submitting PRs:

```bash
# Basic E2E test
make e2e-kind

# Test all supported K8s versions
make e2e-kind-matrix

# Test PSS restricted (OpenShift compatibility)
make e2e-pss
```

### Prerequisites for Local Testing

- Docker (or OrbStack on macOS)
- [Kind](https://kind.sigs.k8s.io/docs/user/quick-start/#installation)
- [Helm](https://helm.sh/docs/intro/install/)
- kubectl

## Validation Checklist

### Pre-Release (Automatic)

- [ ] Unit tests pass with `-race` flag
- [ ] K8s 1.28 cluster test passes
- [ ] K8s 1.30 cluster test passes
- [ ] K8s 1.32 cluster test passes
- [ ] PSS restricted test passes

### Post-Release (Manual)

- [ ] EKS validation passes → badge updated
- [ ] AKS validation passes → badge updated
- [ ] GKE validation passes → badge updated

## Troubleshooting

### Cloud test fails to create cluster

Check:
- Quota limits in the cloud account
- Correct credentials in GitHub secrets
- Region availability

### Operator fails to start

Check:
- Image pull errors (verify GHCR access)
- RBAC permissions (ClusterRole applied correctly)
- Resource limits (increase if OOM)

### Sync timeout

Check:
- Platform URL is correct
- Credentials secret exists with correct keys
- Network egress allowed to platform

## Adding a New Cloud Provider

1. Add a new job in `.github/workflows/cloud-e2e.yml`
2. Create badge JSON in `.github/badges/<provider>.json`
3. Add badge to README.md compatibility section
4. Update this documentation
