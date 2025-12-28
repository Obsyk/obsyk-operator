# AWS IAM User and Role for obsyk-operator EKS E2E Testing
#
# This creates an IAM user with minimal permissions to:
# 1. Create/delete EKS clusters via eksctl
# 2. Manage EC2 resources (VPC, subnets, security groups, nodes)
# 3. Manage IAM roles required by EKS
#
# Usage:
#   cd .github/cloud-credentials/aws
#   terraform init
#   terraform apply
#
# After apply, add the secrets to GitHub:
#   gh secret set AWS_ACCESS_KEY_ID --repo Obsyk/obsyk-operator --body "$(terraform output -raw access_key_id)"
#   gh secret set AWS_SECRET_ACCESS_KEY --repo Obsyk/obsyk-operator --body "$(terraform output -raw secret_access_key)"

terraform {
  required_version = ">= 1.0"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

provider "aws" {
  region = var.aws_region
}

variable "aws_region" {
  description = "AWS region for resources"
  type        = string
  default     = "us-east-1"
}

variable "name_prefix" {
  description = "Prefix for resource names"
  type        = string
  default     = "obsyk-e2e"
}

# IAM User for GitHub Actions
resource "aws_iam_user" "e2e" {
  name = "${var.name_prefix}-github-actions"
  path = "/ci/"

  tags = {
    Purpose   = "obsyk-operator EKS E2E testing"
    ManagedBy = "terraform"
    Repo      = "Obsyk/obsyk-operator"
  }
}

# Access key for the user
resource "aws_iam_access_key" "e2e" {
  user = aws_iam_user.e2e.name
}

# Policy for EKS cluster management via eksctl
resource "aws_iam_user_policy" "eks_e2e" {
  name = "${var.name_prefix}-eks-management"
  user = aws_iam_user.e2e.name

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      # EKS permissions
      {
        Sid    = "EKSFullAccess"
        Effect = "Allow"
        Action = [
          "eks:*"
        ]
        Resource = "*"
        Condition = {
          StringLike = {
            "eks:cluster-name" = ["obsyk-e2e-*"]
          }
        }
      },
      # EKS describe/list (needed for eksctl)
      {
        Sid    = "EKSDescribe"
        Effect = "Allow"
        Action = [
          "eks:DescribeCluster",
          "eks:ListClusters",
          "eks:ListNodegroups",
          "eks:DescribeNodegroup"
        ]
        Resource = "*"
      },
      # EC2 permissions for VPC, subnets, security groups
      {
        Sid    = "EC2NetworkAccess"
        Effect = "Allow"
        Action = [
          "ec2:CreateVpc",
          "ec2:DeleteVpc",
          "ec2:DescribeVpcs",
          "ec2:ModifyVpcAttribute",
          "ec2:CreateSubnet",
          "ec2:DeleteSubnet",
          "ec2:DescribeSubnets",
          "ec2:CreateInternetGateway",
          "ec2:DeleteInternetGateway",
          "ec2:AttachInternetGateway",
          "ec2:DetachInternetGateway",
          "ec2:DescribeInternetGateways",
          "ec2:CreateNatGateway",
          "ec2:DeleteNatGateway",
          "ec2:DescribeNatGateways",
          "ec2:AllocateAddress",
          "ec2:ReleaseAddress",
          "ec2:DescribeAddresses",
          "ec2:CreateRouteTable",
          "ec2:DeleteRouteTable",
          "ec2:DescribeRouteTables",
          "ec2:CreateRoute",
          "ec2:DeleteRoute",
          "ec2:AssociateRouteTable",
          "ec2:DisassociateRouteTable",
          "ec2:CreateSecurityGroup",
          "ec2:DeleteSecurityGroup",
          "ec2:DescribeSecurityGroups",
          "ec2:AuthorizeSecurityGroupIngress",
          "ec2:AuthorizeSecurityGroupEgress",
          "ec2:RevokeSecurityGroupIngress",
          "ec2:RevokeSecurityGroupEgress",
          "ec2:CreateTags",
          "ec2:DeleteTags",
          "ec2:DescribeTags",
          "ec2:DescribeAvailabilityZones",
          "ec2:DescribeImages",
          "ec2:DescribeKeyPairs",
          "ec2:CreateLaunchTemplate",
          "ec2:DeleteLaunchTemplate",
          "ec2:DescribeLaunchTemplates",
          "ec2:DescribeLaunchTemplateVersions"
        ]
        Resource = "*"
      },
      # EC2 instances for worker nodes
      {
        Sid    = "EC2InstanceAccess"
        Effect = "Allow"
        Action = [
          "ec2:RunInstances",
          "ec2:TerminateInstances",
          "ec2:DescribeInstances",
          "ec2:DescribeInstanceTypes"
        ]
        Resource = "*"
      },
      # Auto Scaling for managed node groups
      {
        Sid    = "AutoScalingAccess"
        Effect = "Allow"
        Action = [
          "autoscaling:CreateAutoScalingGroup",
          "autoscaling:DeleteAutoScalingGroup",
          "autoscaling:DescribeAutoScalingGroups",
          "autoscaling:UpdateAutoScalingGroup",
          "autoscaling:CreateLaunchConfiguration",
          "autoscaling:DeleteLaunchConfiguration",
          "autoscaling:DescribeLaunchConfigurations"
        ]
        Resource = "*"
      },
      # IAM permissions for EKS service roles
      {
        Sid    = "IAMRoleManagement"
        Effect = "Allow"
        Action = [
          "iam:CreateRole",
          "iam:DeleteRole",
          "iam:GetRole",
          "iam:PassRole",
          "iam:AttachRolePolicy",
          "iam:DetachRolePolicy",
          "iam:ListAttachedRolePolicies",
          "iam:PutRolePolicy",
          "iam:DeleteRolePolicy",
          "iam:GetRolePolicy",
          "iam:ListRolePolicies",
          "iam:TagRole",
          "iam:UntagRole",
          "iam:CreateInstanceProfile",
          "iam:DeleteInstanceProfile",
          "iam:GetInstanceProfile",
          "iam:AddRoleToInstanceProfile",
          "iam:RemoveRoleFromInstanceProfile",
          "iam:ListInstanceProfilesForRole"
        ]
        Resource = [
          "arn:aws:iam::*:role/eksctl-obsyk-e2e-*",
          "arn:aws:iam::*:instance-profile/eksctl-obsyk-e2e-*"
        ]
      },
      # IAM permissions for OIDC provider (required by eksctl)
      {
        Sid    = "IAMOIDCProvider"
        Effect = "Allow"
        Action = [
          "iam:CreateOpenIDConnectProvider",
          "iam:DeleteOpenIDConnectProvider",
          "iam:GetOpenIDConnectProvider",
          "iam:TagOpenIDConnectProvider"
        ]
        Resource = "arn:aws:iam::*:oidc-provider/oidc.eks.*.amazonaws.com/*"
      },
      # CloudFormation for eksctl
      {
        Sid    = "CloudFormationAccess"
        Effect = "Allow"
        Action = [
          "cloudformation:CreateStack",
          "cloudformation:DeleteStack",
          "cloudformation:DescribeStacks",
          "cloudformation:DescribeStackEvents",
          "cloudformation:DescribeStackResources",
          "cloudformation:GetTemplate",
          "cloudformation:ListStacks",
          "cloudformation:UpdateStack"
        ]
        Resource = "arn:aws:cloudformation:*:*:stack/eksctl-obsyk-e2e-*/*"
      },
      # CloudFormation describe (needed for stack status)
      {
        Sid    = "CloudFormationDescribe"
        Effect = "Allow"
        Action = [
          "cloudformation:DescribeStacks",
          "cloudformation:ListStacks"
        ]
        Resource = "*"
      },
      # SSM for EKS AMI lookup
      {
        Sid    = "SSMParameterAccess"
        Effect = "Allow"
        Action = [
          "ssm:GetParameter"
        ]
        Resource = "arn:aws:ssm:*:*:parameter/aws/service/eks/optimized-ami/*"
      },
      # KMS for secrets encryption (optional but recommended)
      {
        Sid    = "KMSAccess"
        Effect = "Allow"
        Action = [
          "kms:CreateGrant",
          "kms:DescribeKey"
        ]
        Resource = "*"
        Condition = {
          StringLike = {
            "kms:RequestAlias" = "alias/eks/*"
          }
        }
      }
    ]
  })
}

# Outputs for GitHub secrets
output "access_key_id" {
  description = "AWS Access Key ID for GitHub Actions"
  value       = aws_iam_access_key.e2e.id
  sensitive   = true
}

output "secret_access_key" {
  description = "AWS Secret Access Key for GitHub Actions"
  value       = aws_iam_access_key.e2e.secret
  sensitive   = true
}

output "user_arn" {
  description = "ARN of the IAM user"
  value       = aws_iam_user.e2e.arn
}

output "setup_commands" {
  description = "Commands to add secrets to GitHub"
  value       = <<-EOT
    # Run these commands to add the secrets to GitHub:
    gh secret set AWS_ACCESS_KEY_ID --repo Obsyk/obsyk-operator --body "$(terraform output -raw access_key_id)"
    gh secret set AWS_SECRET_ACCESS_KEY --repo Obsyk/obsyk-operator --body "$(terraform output -raw secret_access_key)"
  EOT
}
