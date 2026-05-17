# Changelog

## v1.0.1 - 2026-03-26

- ci(release): preserve release notes across checkout and commit changelog via git (89e5176)
- Update roadmap-v1.1.0.md (1f95bab)


All notable changes to this project will be documented in this file.

The format is inspired by Keep a Changelog and this project uses Semantic Versioning.

## Unreleased

- feat(queue): add configurable async publish workers, optional per-source bulkheads, aggregate/source queue snapshots, and Pub/Sub flow-control tuning
- feat(rate-limit): add shared `RateLimiter` abstraction, Redis-backed distributed counters, trusted-proxy-aware source limits, and default source limiting without explicit overrides
- feat(observability): expose per-source queue usage, depth, and failure-budget gauges when queue bulkheads are enabled
- feat(config): add runtime validation and `/admin/configz` visibility for queue worker, bulkhead, Pub/Sub flow-control, source limit, and rate-limit backend settings
- perf(schema): validate provider payload contracts using streaming top-level JSON field extraction instead of full object unmarshalling
- docs: document production scalability controls for queue workers, bulkheads, Pub/Sub flow control, Redis rate limits, and durable multi-replica stores
- feat(deploy): add `ingestion-gateway-security-rejection` AnalysisTemplate gating canary promotions on security rejection burn rate
- feat(deploy): include security-rejection analysis gate in staging (2-step) and prod (3-step) rollout overlays
- feat(policy): enforce `ingestion-gateway-security-rejection` AnalysisTemplate presence in production manifests via `check_iac.py`
- test(policy): update `test_check_iac.py` fixtures to cover security-rejection template requirements for prod
- ci: add `contracts` job to `ci.yml` — runs fixture lint and contract/schema-drift tests on every push and PR
- ci: add `policy.yml` workflow — runs Checkov (Terraform + Kubernetes) and `check_iac.py` on IaC path changes

## v1.0.0 - 2026-03-26

- Initial professional repository stack
- Ingestion gateway with signatures, queue abstraction, telemetry, and auth middleware
- Security hardening: JWT strength guardrails, admin CIDR allowlist, trusted proxies, and IP rate limiting
- Observability upgrade: SLO dashboards, security rejection metrics, and Prometheus recording/alert rules
- Infrastructure expansion: multi-region active-active Terraform topology and operational runbooks
- CI pipeline, release workflows, Docker Compose smoke/integration setup, and docs site publishing
- Contract tests for webhook payload compatibility with provider fixtures (Slack, GitHub, GitLab, Teams)
