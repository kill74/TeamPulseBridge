# Release Notes v1.0.0

Date: 2026-03-26

## Highlights

- Production-style ingestion service for Slack, Teams, GitHub, and GitLab webhooks.
- Security-by-default runtime hardening and admin endpoint protections.
- Multi-region active-active Terraform architecture baseline.
- SLO and security observability via Prometheus + Grafana dashboards and alerts.
- Contract test coverage for webhook payload compatibility across providers.

## Notable Capabilities

- Signature and token validation per provider.
- Queue abstraction (`log` and Pub/Sub backends) with integration tests.
- JWT-protected admin/metrics paths, CIDR allowlist, trusted proxy controls, and rate limiting.
- Security rejection telemetry (`security_rejections_total`) with dashboard and alerting hooks.
- GitOps deployment patterns and operational runbooks.

## Quality Gates

- Unit and integration test workflows in CI.
- Smoke workflow with Docker build and service health checks.
- Docs build and deploy workflows.
- SemVer tag-driven release workflow.
