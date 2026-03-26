# Infrastructure as Code - Terraform Setup

Professional-grade Terraform infrastructure for staging and production GCP environments. This setup deploys a complete, production-ready platform with GKE, Cloud SQL, monitoring, and security controls.

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                    GCP Project                              │
├─────────────────────────────────────────────────────────────┤
│                                                               │
│  ┌──────────────────────────────────────────────────────┐   │
│  │            VPC Network (10.0.0.0/16)                │   │
│  │  ┌──────────────────────────────────────────────┐   │   │
│  │  │  GKE Cluster                                 │   │   │
│  │  │  ├─ Default Node Pool (system)              │   │   │
│  │  │  │  └─ 1-5 nodes (preemptible)              │   │   │
│  │  │  └─ Workload Node Pool (app)                │   │   │
│  │  │     └─ 1-10 nodes (standard)                │   │   │
│  │  │                                              │   │   │
│  │  │  Pods CIDR: 10.1.0.0/16                     │   │   │
│  │  │  Services CIDR: 10.2.0.0/16                 │   │   │
│  │  └──────────────────────────────────────────────┘   │   │
│  │                                                      │   │
│  │  ┌──────────────────────────────────────────────┐   │   │
│  │  │  Cloud SQL (Private IP)                      │   │   │
│  │  │  ├─ PostgreSQL 15                            │   │   │
│  │  │  ├─ Regional HA (prod) / Zonal (staging)  │   │   │
│  │  │  └─ Daily automated backups                  │   │   │
│  │  └──────────────────────────────────────────────┘   │   │
│  │                                                      │   │
│  │  Cloud NAT ──→ External IP for egress              │   │
│  └──────────────────────────────────────────────────────┘   │
│                                                               │
│  ┌──────────────────────────────────────────────────────┐   │
│  │            Cloud Storage                             │   │
│  │  ├─ Artifacts bucket (versioned)                    │   │
│  │  ├─ App data bucket (with lifecycle)                │   │
│  │  ├─ Backups bucket (30-90 day retention)            │   │
│  │  └─ Logs bucket (compressed, 14-30 day retention)  │   │
│  └──────────────────────────────────────────────────────┘   │
│                                                               │
│  ┌──────────────────────────────────────────────────────┐   │
│  │            Monitoring & Alerting                     │   │
│  │  ├─ Cloud Monitoring dashboard                      │   │
│  │  ├─ Uptime checks (multi-region)                    │   │
│  │  ├─ Alert policies (CPU, memory, errors)            │   │
│  │  ├─ Email notifications                             │   │
│  │  └─ Slack integration (prod only)                   │   │
│  └──────────────────────────────────────────────────────┘   │
│                                                               │
│  ┌──────────────────────────────────────────────────────┐   │
│  │            Security & Access Control                │   │
│  │  ├─ Workload Identity binding                       │   │
│  │  ├─ IAM roles (least privilege)                     │   │
│  │  ├─ Network policies (Kubernetes)                   │   │
│  │  ├─ Pod security policies                           │   │
│  │  └─ Cloud Armor (DDoS protection)                   │   │
│  └──────────────────────────────────────────────────────┘   │
│                                                               │
└─────────────────────────────────────────────────────────────┘
```

## Features

**Multiple Environments**: Separate staging and production configurations
**High Availability**: Regional databases with automated failover (prod)
**Auto-Scaling**: Cluster and node pool auto-scaling
**Observability**: Complete monitoring, logging, and alerting stack
**Security**: Workload identity, RBAC, network policies, Cloud Armor
**Cost Optimization**: Preemptible nodes in staging, automatic cleanup
**Infrastructure as Code**: Version-controlled, reproducible deployments
**State Management**: Remote GCS backend with encryption

## Active-Active Multi-Region Topology

The root Terraform module supports optional active-active deployment in two regions.

Configuration knobs:

- `enable_multi_region = true`
- `region` and `secondary_region`
- `secondary_gke_subnet_cidr`, `secondary_pods_cidr`, `secondary_services_cidr`, `secondary_db_subnet_cidr`
- `secondary_app_domain` (optional)

Behavior when enabled:

1. Primary stack is deployed in `region`.
2. Secondary stack is deployed in `secondary_region`.
3. Additional outputs are exposed for secondary cluster/database and topology summary.

Important: Multi-region compute does not automatically guarantee globally consistent data writes.
Choose one of these patterns explicitly before production rollout:

- Single-writer database with regional failover.
- Multi-writer datastore with conflict resolution.
- Region-local write path + asynchronous replication for eventually consistent workloads.

## File Structure

```
infrastructure/
├── terraform/
│   ├── modules/
│   │   ├── gke_cluster/     # GKE cluster configuration
│   │   ├── networking/      # VPC, subnets, firewalls
│   │   ├── database/        # Cloud SQL with HA
│   │   ├── monitoring/      # Observability and alerting
│   │   ├── security/        # Service accounts and RBAC
│   │   └── storage/         # GCS buckets and lifecycle rules
│   ├── main.tf              # Root module configuration
│   ├── variables.tf         # Input variables with validation
│   ├── outputs.tf           # Output values
│   ├── providers.tf         # Provider configuration
│   ├── backend.tf           # State backend configuration
│   └── environments/
│       ├── staging/         # Staging environment config
│       │   ├── terraform.tfvars
│       │   └── backend.conf
│       └── prod/            # Production environment config
│           ├── terraform.tfvars
│           └── backend.conf
├── scripts/
│   ├── init-backend.sh      # Initialize backend and state bucket
│   ├── deploy.sh            # Deploy infrastructure
│   └── destroy.sh           # Destroy infrastructure
└── docs/
    └── README.md            # This file
```

## Prerequisites

- Terraform >= 1.0
- Google Cloud SDK (gcloud CLI)
- kubectl
- Appropriate GCP permissions (Project Owner or custom role)

## Setup Instructions

### 1. Create GCP Project

```bash
PROJECT_ID="my-teampulse-project"
gcloud projects create $PROJECT_ID --name="TeamPulse"
gcloud config set project $PROJECT_ID
```

### 2. Initialize Terraform Backend

```bash
cd infrastructure/scripts

# Create backend bucket and initialize Terraform
./init-backend.sh staging $PROJECT_ID my-terraform-state

# Repeat for production
./init-backend.sh prod $PROJECT_ID my-terraform-state
```

### 3. Configure Environment Variables

Edit the respective environment tfvars files:

**Staging** (`environments/staging/terraform.tfvars`):

```hcl
gcp_project = "my-teampulse-project"
app_domain  = "staging.api.example.com"
alert_email = "devops-staging@example.com"
```

**Production** (`environments/prod/terraform.tfvars`):

```hcl
gcp_project = "my-teampulse-project"
app_domain  = "api.example.com"
alert_email = "devops-alerts@example.com"
slack_webhook_url = "https://hooks.slack.com/services/YOUR/WEBHOOK/URL"
```

### 4. Deploy Infrastructure

```bash
# Plan staging deployment
cd infrastructure/scripts
./deploy.sh staging

# Confirm and apply

# Repeat for production
./deploy.sh prod --auto-approve  # (requires extra caution in prod)
```

### 5. Configure kubectl

```bash
# Get credentials
gcloud container clusters get-credentials teampulse-staging --region us-central1

# Verify connection
kubectl cluster-info
kubectl get nodes
```

## Key Modules

### GKE Cluster (`modules/gke_cluster`)

Provisions a production-grade Kubernetes cluster with:

- Regional master (automatic failover)
- Multiple node pools (system + workload)
- Auto-scaling (cluster and node pool level)
- Maintenance windows
- Network policies enabled
- Shielded GKE nodes
- Workload metadata service

**Staging**: 1-5 nodes, preemptible (cost-optimized)
**Production**: 3-20 nodes, standard (reliability-optimized)

### Networking (`modules/networking`)

Deploys VPC with:

- Custom subnets for GKE and databases
- Secondary IP ranges for pods and services
- Cloud NAT for outbound traffic
- Firewall rules (HTTP/HTTPS, health checks, internal)
- Cloud Armor with DDoS protection and rate limiting
- Private Service Connection for Cloud SQL

### Database (`modules/database`)

Manages Cloud SQL with:

- PostgreSQL 15
- Regional HA (prod) / Zonal (staging)
- Automated daily backups + PITR
- IAM authentication support
- Query Insights for performance monitoring

- Automated alerts (CPU, storage)

### Monitoring (`modules/monitoring`)

Sets up observability:

- Cloud Monitoring dashboard
- Uptime checks (multi-region in prod)
- Alert policies (CPU, memory, errors, restarts)

- Email and Slack notifications
- Log aggregation and retention

### Security (`modules/security`)

Implements security controls:

- Workload Identity binding (recommended over service account keys)
- IAM roles (least privilege principle)

- Kubernetes NetworkPolicies
- Pod Security Policies
- RBAC ClusterRoles and ClusterRoleBindings

### Storage (`modules/storage`)

Provisions GCS buckets:

- Artifacts bucket (versioned, 3 versions kept)
- App data bucket (lifecycle to coldline)
- Backups bucket (encrypted)

- Logs bucket (auto-cleanup)
- Public access prevention on all buckets

## Deployment Strategy

### Staging Environment

**Cost Optimization Focus**:

- Single-zone database (no failover)
- Preemptible nodes (60-90% cost savings)

- Smaller machine types (n1-standard-2)
- Shorter retention periods
- Minimal replicas

**Use for**: Development, testing, staging deployments

### Production Environment

**Reliability & Performance Focus**:

- Regional HA database (99.95% SLA)
- Standard nodes (no preemption)
- Larger machine types (n1-standard-4)
- Multi-region monitoring
- Maximum retention for compliance
- Full alerting stack (email + Slack)

**Use for**: Customer-facing, revenue-critical workloads

## Operations

### Check Deployment Status

```bash
terraform show
terraform output -json
```

### Scale Cluster

```bash
# Update max_node_count in terraform.tfvars
terraform apply -var-file=environments/staging/terraform.tfvars
```

### Add SSH Access (Debugging)

```bash
# Temporarily enable SSH in terraform.tfvars
enable_ssh_access    = true
ssh_source_ranges    = ["203.0.113.0/24"]  # Your IP

terraform apply -var-file=environments/staging/terraform.tfvars

# SSH to node
gcloud compute ssh <node-name> --zone us-central1-a

# Disable afterwards
enable_ssh_access = false
terraform apply
```

### Update Application

```bash
# Terraform manages infrastructure only.
# Application delivery is managed by Argo CD GitOps.

# 1) Bootstrap Argo CD once per cluster
cd infrastructure/scripts
./bootstrap-gitops-argocd.sh <gcp-project-id> <gke-cluster-name> <gke-region>

# 2) Promote a new image by editing the overlay image tag
# deploy/k8s/overlays/staging/kustomization.yaml
# deploy/k8s/overlays/prod/kustomization.yaml

# 3) Commit and push to main
# Argo CD reconciles staging automatically; production requires manual sync
```

### Destroy Environment

```bash

# WARNING: Deletes all resources and data
cd infrastructure/scripts
./destroy.sh staging    # or prod

# Requires manual confirmation
```

## Costs

### Staging (Monthly Estimate)

- GKE cluster: ~$50-100
- Preemptible nodes (3x n1-standard-2): ~$30
- Cloud SQL (db-g1-small): ~$20-30
- Storage & monitoring: ~$10-20
- **Total**: ~$110-180/month

### Production (Monthly Estimate)

- GKE cluster: ~$150-300
- Nodes (3x n1-standard-4): ~$200-300
- Cloud SQL HA (db-custom-4-16384): ~$150-250
- Storage & monitoring: ~$20-50
- **Total**: ~$520-900/month

_Costs vary by region and usage. Use [GCP Pricing Calculator](https://cloud.google.com/products/calculator) for accurate estimates._

## Troubleshooting

### Terraform Cloud State Lock

```bash
# If stuck during apply
terraform force-unlock <LOCK_ID>
```

### Database Connection Issues

```bash
# Test private connectivity from pod
kubectl run -it --rm debug --image=google/cloud-sdk:slim --restart=Never -- bash

# From pod: test connection to Cloud SQL
gcloud sql connect <INSTANCE_NAME> --user=appuser
```

### Cluster Access Denied

```bash
# Re-authenticate
gcloud auth application-default login
gcloud container clusters get-credentials teampulse-staging --region us-central1
```

### Out of Quota

```bash
# Check quotas
gcloud compute project-info describe --project=$PROJECT_ID | grep QUOTA

# Increase quotas via GCP Console > IAM & Admin > Quotas
```

## Security Best Practices

✅ **Implemented**:

- Workload Identity (no service account keys in pods)
- Network policies (restrict pod-to-pod traffic)
- Cloud Armor (DDoS protection)
- Private Cloud SQL (no public IP)
- Encryption at rest (GCS + SQL)

- Regular backups with PITR

⚠️ **Additional Considerations**:

- Enable Binary Authorization for image verification
- Use Google Artifact Registry instead of Docker Hub
- Implement Pod Disruption Budgets
- Set resource requests/limits in deployments
- Regular security patches (auto-upgrades enabled)
- Audit logging enabled

## Terraform State Management

State stored in GCS with:

- Encryption at rest (CSEK)

- Versioning enabled
- Uniform bucket-level access
- Access logs

**Never** commit state files to Git. State contains sensitive data.

## Team Workflows

### Senior Engineers

```bash
terraform workspace new prod
terraform apply -var-file=environments/prod/terraform.tfvars
```

### Junior Engineers (Staging Only)

```bash
terraform apply -var-file=environments/staging/terraform.tfvars
```

### Read-Only Access

```bash
terraform show
terraform output -json
```

## Further Reading

- [Terraform Google Provider Docs](https://registry.terraform.io/providers/hashicorp/google/latest/docs)
- [GKE Best Practices](https://cloud.google.com/kubernetes-engine/docs/best-practices)
- [Cloud SQL Best Practices](https://cloud.google.com/sql/docs/mysql/best-practices)
- [Workload Identity Setup](https://cloud.google.com/kubernetes-engine/docs/how-to/workload-identity)
