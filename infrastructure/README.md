# TeamPulse Infrastructure

Professional-grade infrastructure as code using Terraform for production GCP deployments.

See [docs/README.md](docs/README.md) for complete setup and operation guide.

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
./deploy.sh prod --auto-approve
```

## Structure

```
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
- gcloud authentication with Project Editor role

## Security Features

✅ Workload Identity (no service account keys)
✅ Private Cloud SQL (no public IP)
✅ Network policies (Kubernetes)
✅ Cloud Armor (DDoS protection)
✅ Encryption at rest (GCS + SQL)
✅ Automated daily backups
✅ IAM least-privilege roles

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
