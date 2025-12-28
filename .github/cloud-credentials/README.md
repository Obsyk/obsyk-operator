# Cloud Credentials for E2E Testing

This directory contains Terraform configurations to create the IAM roles/users needed for cloud E2E testing.

## Providers

| Provider | Directory | Status | Cost per Run |
|----------|-----------|--------|--------------|
| AWS (EKS) | [`aws/`](./aws/) | Ready | ~$0.15 |
| Azure (AKS) | `azure/` | TODO | ~$0.10 |
| GCP (GKE) | `gcp/` | TODO | ~$0.08 |

## Quick Start

### AWS (EKS)

```bash
cd aws
terraform init
terraform apply

# Add secrets to GitHub
gh secret set AWS_ACCESS_KEY_ID --repo Obsyk/obsyk-operator --body "$(terraform output -raw access_key_id)"
gh secret set AWS_SECRET_ACCESS_KEY --repo Obsyk/obsyk-operator --body "$(terraform output -raw secret_access_key)"

# Run test
gh workflow run cloud-e2e.yml --repo Obsyk/obsyk-operator -f provider=eks
```

## Required GitHub Secrets

| Secret | Provider | Description |
|--------|----------|-------------|
| `AWS_ACCESS_KEY_ID` | EKS | AWS access key |
| `AWS_SECRET_ACCESS_KEY` | EKS | AWS secret key |
| `AZURE_CREDENTIALS` | AKS | Azure service principal JSON |
| `GCP_CREDENTIALS` | GKE | GCP service account key JSON |

## Security Best Practices

1. **Minimal Permissions**: Each IAM role has only the permissions needed for E2E testing
2. **Resource Scoping**: Permissions are scoped to resources with `obsyk-e2e-*` prefix
3. **Credential Rotation**: Rotate credentials periodically using `terraform taint` + `apply`
4. **No Console Access**: Service accounts have no interactive login capability
5. **Audit Trail**: All actions are logged in CloudTrail/Azure Activity Log/GCP Audit Logs
