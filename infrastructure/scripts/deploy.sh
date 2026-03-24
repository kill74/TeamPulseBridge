#!/bin/bash

##
# Deploy infrastructure for specified environment
# Usage: ./deploy.sh <environment> [--auto-approve]
##

set -euo pipefail

ENVIRONMENT=${1:-}
AUTO_APPROVE=${2:-}

if [[ -z "$ENVIRONMENT" ]]; then
    echo "Usage: $0 <environment> [--auto-approve]"
    echo "Environments: staging, prod"
    exit 1
fi

if [[ ! "$ENVIRONMENT" =~ ^(staging|prod)$ ]]; then
    echo "Error: Invalid environment '$ENVIRONMENT'"
    echo "Valid environments: staging, prod"
    exit 1
fi

echo "=========================================="
echo "Deploying $ENVIRONMENT environment"
echo "=========================================="

# Change to terraform directory
cd terraform

# Validate configuration
echo "Validating Terraform configuration..."
terraform validate

# Plan deployment
echo ""
echo "Planning deployment..."
PLAN_FILE="/tmp/tfplan-$ENVIRONMENT"
terraform plan \
    -var-file="environments/$ENVIRONMENT/terraform.tfvars" \
    -out="$PLAN_FILE"

echo ""
echo "Plan saved to: $PLAN_FILE"

# Apply if auto-approve or user confirms
if [[ "$AUTO_APPROVE" == "--auto-approve" ]]; then
    echo "Applying changes (auto-approved)..."
    terraform apply "$PLAN_FILE"
else
    echo ""
    read -p "Apply changes? (yes/no): " CONFIRM
    if [[ "$CONFIRM" == "yes" ]]; then
        terraform apply "$PLAN_FILE"
    else
        echo "Deployment cancelled"
        rm -f "$PLAN_FILE"
        exit 0
    fi
fi

# Save outputs
echo ""
echo "Saving outputs..."
terraform output -json > "/tmp/outputs-$ENVIRONMENT.json"

echo ""
echo "=========================================="
echo "✓ $ENVIRONMENT environment deployed"
echo "=========================================="
echo ""
echo "Outputs saved to: /tmp/outputs-$ENVIRONMENT.json"
echo ""
echo "Configure kubeconfig:"
echo "  gcloud container clusters get-credentials teampulse-$ENVIRONMENT --region us-central1"
echo ""
echo "View dashboard:"
echo "  gcloud monitoring dashboards list --project=<PROJECT_ID>"
