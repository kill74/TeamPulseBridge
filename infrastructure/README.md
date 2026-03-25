# TeamPulse Infrastructure

Professional-grade infrastructure as code using Terraform for production GCP deployments.

See [docs/README.md](docs/README.md) for complete setup and operation guide.

## GitOps (Argo CD)

Application delivery is defined declaratively and reconciled by Argo CD.

Bootstrap Argo CD in a target GKE cluster:

```bash
bash scripts/bootstrap-gitops-argocd.sh <gcp-project-id> <gke-cluster-name> <gke-region>
```

GitOps manifests live in:

- `../deploy/k8s/base`
- `../deploy/k8s/overlays/staging`
- `../deploy/k8s/overlays/prod`
- `../deploy/gitops/argocd`

To validate manifests before commit:

```bash
cd ..
make gitops-validate
```

## Quick Start

### 1. Initialize Backend

```bash
cd scripts
./init-backend.sh staging your-gcp-project-id your-terraform-state-bucket
```

### 2. Deploy Staging

```bash
./deploy.sh staging
```

### 3. Deploy Production

```bash
./deploy.sh prod
```

Production should require manual confirmation in normal workflows.

## Structure

```text
terraform/
├── main.tf                  Root module definitions
├── variables.tf            Input variables with validation
├── outputs.tf              Output values
├── modules/                Reusable infrastructure modules
└── environments/           Environment-specific configurations
    ├── staging/
    └── prod/

scripts/
├── init-backend.sh        Initialize Terraform backend
├── deploy.sh              Deploy infrastructure
└── destroy.sh             Destroy infrastructure (⚠️ dangerous)

docs/
└── README.md              Complete documentation
```

## Environment Matrix

| Feature        | Staging     | Production    |
| -------------- | ----------- | ------------- |
| **Cluster**    | 1 node min  | 3 nodes min   |
| **Nodes**      | Preemptible | Standard      |
| **Database**   | Zonal       | Regional HA   |
| **Backups**    | 7 days      | 30 days       |
| **Monitoring** | Email only  | Email + Slack |
| **Cost/Month** | ~$110-180   | ~$520-900     |

## Modules

| Module                | Purpose                               |
| --------------------- | ------------------------------------- |
| `modules/gke_cluster` | GKE cluster with auto-scaling         |
| `modules/networking`  | VPC, subnets, firewalls, Cloud Armor  |
| `modules/database`    | Cloud SQL with automated backups      |
| `modules/monitoring`  | Observability, dashboards, alerts     |
| `modules/security`    | Service accounts, RBAC, policies      |
| `modules/storage`     | GCS buckets with lifecycle management |

## Key Outputs

After deployment, outputs are saved to `/tmp/outputs-{env}.json`:

```json
{
  "cluster_name": "teampulse-staging",
  "cluster_endpoint": "https://...",
  "database_connection_name": "project:region:instance",
  "app_service_account_email": "ingestion-gateway-workload@...",
  "artifacts_bucket": "my-project-artifacts-staging"
}
```

## Prerequisites

- Terraform >= 1.0
- Google Cloud SDK
- kubectl
- gcloud authentication with least-privilege deployment role

Recommended before first apply:

```bash
terraform fmt -recursive
terraform validate
```

## Security Features

✅ Workload Identity (no service account keys)
✅ Private Cloud SQL (no public IP)
✅ Network policies (Kubernetes)
✅ Cloud Armor (DDoS protection)
✅ Encryption at rest (GCS + SQL)
✅ Automated daily backups
✅ IAM least-privilege roles

## Deployment Workflow (Recommended)

Use this flow to reduce accidental drift and unsafe changes:

1. Run `./init-backend.sh <env> ...` once per environment.
2. Run `terraform fmt -recursive` and `terraform validate`.
3. Run `./deploy.sh <env>` and review plan output carefully.
4. Apply only after review/approval.
5. Capture outputs and update dependent systems.

For production, require peer review and change window before apply.

## Day-2 Operations

- Drift detection: run scheduled `terraform plan` in CI with no apply.
- State hygiene: do not edit remote state manually.
- Backup checks: periodically validate restore runbooks.
- Cost checks: compare monthly deltas before/after major applies.

## Failure Domains and Recovery

- Staging favors cost and faster iteration.
- Production favors HA and higher retention.
- Keep `destroy.sh` restricted to approved operators and non-prod by default.

## Common Commands

```bash
# Plan staging
cd terraform
terraform plan -var-file=environments/staging/terraform.tfvars

# Plan prod
terraform plan -var-file=environments/prod/terraform.tfvars
```

## Monitoring

Production deployment includes:

- Multi-region uptime checks
- Real-time dashboards
- Error rate alerts
- Pod restart monitoring
- Resource usage metrics
- Email + Slack notifications

## Documentation

Full documentation: [docs/README.md](docs/README.md)

Key topics:

- [Architecture Overview](docs/README.md#architecture-overview)
- [Setup Instructions](docs/README.md#setup-instructions)
- [Operations](docs/README.md#operations)
- [Troubleshooting](docs/README.md#troubleshooting)
- [Cost Estimates](docs/README.md#costs)
- [Security Best Practices](docs/README.md#security-best-practices)
