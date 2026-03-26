# Database Module

Creates a private Cloud SQL PostgreSQL instance with backup and alerting defaults.

## Responsibility

- Provisions Cloud SQL instance with private networking and SSL requirement.
- Enables backup, point-in-time recovery, and maintenance window controls.
- Creates application database and users (IAM and optional password user).
- Configures CPU and storage alert policies.

## Key Inputs

- `instance_name`, `region`, `environment`
- `database_version`, `machine_tier`, `availability_type`
- `network_id`, `iam_service_account`
- `database_name`, `app_username`, `create_app_user`
- `cpu_threshold`, `storage_threshold`, `notification_channels`

See `variables.tf` for full input contract.

## Outputs

Exposes instance metadata and credentials references required by application runtime.

See `outputs.tf` for complete output contract.

## Operational Notes

- Keep `deletion_protection=true` in production.
- Review `availability_type` changes as potential downtime events.
- Restrict `create_app_user` usage to local or controlled scenarios when possible.
