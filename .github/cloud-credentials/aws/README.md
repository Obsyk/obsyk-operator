# AWS IAM Setup for EKS E2E Testing

This Terraform configuration creates an IAM user with minimal permissions required to run EKS E2E tests for the obsyk-operator.

## Cost Estimate

Each EKS E2E test run costs approximately **$0.10-0.20**:

| Resource | Hourly Cost | 30 min Cost |
|----------|-------------|-------------|
| EKS Control Plane | $0.10/hr | $0.05 |
| 2x t3.medium nodes | $0.0832/hr | $0.04 |
| **Total** | ~$0.18/hr | **~$0.09** |

## Prerequisites

- AWS CLI configured with admin credentials
- Terraform >= 1.0
- GitHub CLI (`gh`) authenticated

## Usage

### 1. Initialize and Apply

```bash
cd .github/cloud-credentials/aws
terraform init
terraform apply
```

### 2. Add Secrets to GitHub

After Terraform apply, add the secrets:

```bash
gh secret set AWS_ACCESS_KEY_ID --repo Obsyk/obsyk-operator --body "$(terraform output -raw access_key_id)"
gh secret set AWS_SECRET_ACCESS_KEY --repo Obsyk/obsyk-operator --body "$(terraform output -raw secret_access_key)"
```

### 3. Run the E2E Test

```bash
gh workflow run cloud-e2e.yml --repo Obsyk/obsyk-operator -f provider=eks
```

## IAM Permissions

The IAM user has minimal permissions scoped to:

- **EKS**: Full access but only for clusters named `obsyk-e2e-*`
- **EC2**: Network resources (VPC, subnets, security groups) and instances
- **IAM**: Only roles/instance profiles named `eksctl-obsyk-e2e-*`
- **CloudFormation**: Only stacks named `eksctl-obsyk-e2e-*`
- **Auto Scaling**: For managed node groups
- **SSM**: Read-only for EKS AMI lookup

## Cleanup

To remove the IAM user:

```bash
terraform destroy
```

**Note**: Before destroying, ensure no E2E tests are running, as this will revoke the credentials immediately.

## Rotating Credentials

To rotate the access key:

```bash
# Taint the access key to force recreation
terraform taint aws_iam_access_key.e2e

# Apply to create new key
terraform apply

# Update GitHub secrets
gh secret set AWS_ACCESS_KEY_ID --repo Obsyk/obsyk-operator --body "$(terraform output -raw access_key_id)"
gh secret set AWS_SECRET_ACCESS_KEY --repo Obsyk/obsyk-operator --body "$(terraform output -raw secret_access_key)"
```

## Security Notes

- The IAM user has no console access
- Permissions are scoped to resources with `obsyk-e2e-*` prefix
- Access keys should be rotated periodically
- Consider using OIDC federation for production (see `oidc-federation/` for setup)
