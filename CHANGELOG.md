# Changelog

All notable changes to this project will be documented in this file.

The format is inspired by Keep a Changelog and this project uses Semantic Versioning.

## Unreleased

- Next release roadmap defined for v1.1.0 (resilience, policy-as-code, and security operations)

## v1.0.0 - 2026-03-26

- Initial professional repository stack
- Ingestion gateway with signatures, queue abstraction, telemetry, and auth middleware
- Security hardening: JWT strength guardrails, admin CIDR allowlist, trusted proxies, and IP rate limiting
- Observability upgrade: SLO dashboards, security rejection metrics, and Prometheus recording/alert rules
- Infrastructure expansion: multi-region active-active Terraform topology and operational runbooks
- CI pipeline, release workflows, Docker Compose smoke/integration setup, and docs site publishing
- Contract tests for webhook payload compatibility with provider fixtures (Slack, GitHub, GitLab, Teams)
