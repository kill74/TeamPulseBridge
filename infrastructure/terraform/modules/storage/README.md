# Storage Module

Provisions object storage buckets and access controls for artifacts, data, backups, and logs.

## Responsibility

- Creates dedicated GCS buckets for artifacts, application data, backups, and logs.
- Applies lifecycle and retention policies by data class.
- Enforces uniform bucket-level access and public access prevention.
- Grants bucket-scoped IAM permissions to the workload service account.

## Key Inputs

- `gcp_project`, `region`, `environment`
- `app_service_account_email`
- `data_retention_days`, `backup_retention_days`, `log_retention_days`
- `labels`

See `variables.tf` for full input contract.

## Outputs

Exposes bucket names and storage metadata for downstream integration.

See `outputs.tf` for complete output contract.

## Operational Notes

- Treat retention changes as compliance-impacting changes.
- Do not set `force_destroy=true` in production unless explicitly approved.
- Validate bucket naming constraints across projects and environments.
