#!/bin/bash

##
# Destroy infrastructure for specified environment
# Usage: ./destroy.sh <environment>
# Warning: This will delete all resources
##

set -euo pipefail

ENVIRONMENT=${1:-}

if [[ -z "$ENVIRONMENT" ]]; then
    echo "Usage: $0 <environment>"
    echo "Environments: staging, prod"
    exit 1
fi

if [[ ! "$ENVIRONMENT" =~ ^(staging|prod)$ ]]; then
    echo "Error: Invalid environment '$ENVIRONMENT'"
    exit 1
fi

echo "=========================================="
echo "WARNING: Destroying $ENVIRONMENT environment"
echo "=========================================="
echo ""
echo "This will delete ALL resources including:"
echo "  - GKE cluster"
echo "  - Cloud SQL database"
echo "  - VPC networks"
echo "  - Storage buckets"
echo "  - All data"
echo ""
read -p "Type 'yes, destroy $ENVIRONMENT' to confirm: " CONFIRM

if [[ "$CONFIRM" != "yes, destroy $ENVIRONMENT" ]]; then
    echo "Destruction cancelled"
    exit 0
fi

cd terraform

echo "Destroying $ENVIRONMENT infrastructure..."
terraform destroy \
    -var-file="environments/$ENVIRONMENT/terraform.tfvars" \
    -auto-approve

echo ""
echo "✓ $ENVIRONMENT environment destroyed"
