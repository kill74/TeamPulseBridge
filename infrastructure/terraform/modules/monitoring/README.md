# Monitoring Module

Creates monitoring, logging, alerting, and dashboard resources for platform operations.

## Responsibility

- Configures monitored project association.
- Optionally creates uptime checks for service health.
- Creates log sinks and log buckets with retention controls.
- Creates alert policies for pod restarts, node readiness, CPU, memory, and error rate.
- Optionally creates email and Slack notification channels.
- Creates a baseline cluster dashboard.

## Key Inputs

- `workspace_name`, `app_name`, `environment`, `gcp_project`, `region`
- `namespace`, `app_domain`, health-check settings
- Alert thresholds (`pod_restart_threshold`, `memory_threshold`, `cpu_threshold`, `error_rate_threshold`)
- Notification settings (`enable_email_notifications`, `alert_email`, `enable_slack_notifications`, `slack_webhook_url`)

See `variables.tf` for full input contract.

## Outputs

Exposes notification channel IDs and monitoring artifacts for dependent modules.

See `outputs.tf` for complete output contract.

## Operational Notes

- Route production alerts to managed on-call channels.
- Review threshold changes against historical SLO/SLI baselines.
- Treat webhook secrets for notification channels as sensitive data.
