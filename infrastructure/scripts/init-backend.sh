#!/bin/bash

##
# Initialize Terraform backend and state bucket
# Usage: ./init-backend.sh <environment> <project-id> <state-bucket-name>
##

set -euo pipefail

ENVIRONMENT=${1:-staging}
PROJECT_ID=${2:-}
STATE_BUCKET=${3:-}

if [[ -z "$PROJECT_ID" || -z "$STATE_BUCKET" ]]; then
    echo "Usage: $0 <environment> <project-id> <state-bucket-name>"
    echo "Example: $0 staging my-gcp-project my-terraform-state"
    exit 1
fi

echo "Setting up Terraform backend for $ENVIRONMENT environment..."
echo "Project: $PROJECT_ID"
echo "State bucket: $STATE_BUCKET"

# Set GCP project
gcloud config set project "$PROJECT_ID"

# Create state bucket if it doesn't exist
if gsutil ls -b "gs://$STATE_BUCKET" &>/dev/null; then
    echo "✓ State bucket already exists"
else
    echo "Creating state bucket..."
    gsutil mb -p "$PROJECT_ID" -c STANDARD -l us-central1 "gs://$STATE_BUCKET"

    # Enable versioning
    gsutil versioning set on "gs://$STATE_BUCKET"

    # Block public access
    gsutil uniformbucketlevelaccess set on "gs://$STATE_BUCKET"

    echo "✓ State bucket created with versioning and uniform access enabled"
fi

# Enable required APIs
echo "Enabling required APIs..."
gcloud services enable \
    compute.googleapis.com \
    container.googleapis.com \
    sqladmin.googleapis.com \
    monitoring.googleapis.com \
    logging.googleapis.com \
    storage-api.googleapis.com \
    servicenetworking.googleapis.com \
    cloudresourcemanager.googleapis.com

echo "✓ APIs enabled"

# Generate encryption key if not exists
if [[ ! -f "encryption-key-$ENVIRONMENT.txt" ]]; then
    echo "Generating encryption key..."
    openssl rand -base64 32 > "encryption-key-$ENVIRONMENT.txt"
    chmod 600 "encryption-key-$ENVIRONMENT.txt"
    echo "✓ Encryption key saved to encryption-key-$ENVIRONMENT.txt"
fi

ENCRYPTION_KEY=$(cat "encryption-key-$ENVIRONMENT.txt")

# Initialize backend
echo "Initializing Terraform backend..."
cd "terraform/environments/$ENVIRONMENT"

# Generate backend config with environment-specific values
cat > "backend.conf.generated" <<EOF
bucket         = "$STATE_BUCKET"
prefix         = "teampulse/$ENVIRONMENT"
encryption_key = "$ENCRYPTION_KEY"
EOF

terraform init -backend-config="backend.conf.generated" -upgrade

rm -f "backend.conf.generated"

echo "✓ Backend initialized for $ENVIRONMENT environment"
echo ""
echo "Next steps:"
echo "1. Configure variables in terraform/environments/$ENVIRONMENT/terraform.tfvars"
echo "2. Run: terraform plan -var-file=terraform/environments/$ENVIRONMENT/terraform.tfvars"
echo "3. Run: terraform apply -var-file=terraform/environments/$ENVIRONMENT/terraform.tfvars"
