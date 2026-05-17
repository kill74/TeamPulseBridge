# Changelog

## v1.1.0 - 2026-05-17

- chore(release): bump version to v1.1.0 and update changelog (7ae7506)
- feat: complete scalability architecture plan (41a9988)
- feat: introduce canary rollout policies with automatic rollback gates (6a34ee5)
- feat: add observability for circuit breaker and bulkheads (8bbb415)
- feat: add redis memory store to terraform and k8s manifests (714ac62)
- fix: resolve Go 1.22 ServeMux routing conflict and update docker-compose (2ba328c)
- feat: implement full bug/leak fixes and scalable architecture (fe96fb7)
- feat: add quick wins - health checks, retry, schema validation, rate limiting, grafana dashboard (6d702d0)
- Merge branch 'main' of https://github.com/kill74/TeamPulseBridge (6e03af2)
- Add dependency security analysis & deployment docs (57a6520)
- chore: upgrade dependencies and migrate to pubsub v2 (a049431)
- Update README.md for GCP project architecture (f87736d)
- ci: align Go toolchain and module metadata (cc7927d)
- ci: simplify working directory config and update action versions (54acf6e)
- ci: fix Go version mismatch in go.mod and workflow (fc77bf8)
- ci: re-enable linter with timeout (e7c8992)
- ci: temporarily disable linter step to debug issue (96b1054)
- ci: fix Go version to 1.23 (available on GitHub runners) (216f2bc)
- fix: resolve critical nil handler bug and goroutine leak (1c0ae8c)
- ci: add GitHub Actions workflow for CI (build, test, lint) (205e9ad)
- Add contract linting and signed release automation (f668099)
- Add policy-as-code guardrails for Terraform and Kubernetes (72e6289)
- Expand local developer workflow and policy baseline (afc866b)
- Harden replay and queue flows and refresh project documentation (38a62e7)
- Add admin failed-events & replay-audit features (ab904c2)
- Add webhook dedup, failed-event replay, and structured error handling (8e6266e)
- chore: update go.work.sum (3ef604b)
- fix: upgrade cloud.google.com/go packages to latest versions (0d4c3a3)
- fix: upgrade golang-jwt/jwt to v5.3.1 (6321f49)
- fix: upgrade OpenTelemetry dependencies to latest versions (917b489)
- fix: upgrade vulnerable dependencies (CVE-2024-45338, CVE-2024-45339) (e341a09)
- fix: comprehensive production-grade improvements to ingestion-gateway (7f81fc1)
- feat: add policy-as-code checks for Terraform and Kubernetes in CI (e8a8c1c)
- feat(docs): enhance documentation structure and add checklists for PRs and repository standards (35c016a)
- chore(release): update changelog for v1.0.1 (639b13a)


## v1.0.1 - 2026-03-26

- ci(release): preserve release notes across checkout and commit changelog via git (89e5176)
- Update roadmap-v1.1.0.md (1f95bab)


All notable changes to this project will be documented in this file.

The format is inspired by Keep a Changelog and this project uses Semantic Versioning.

## [Unreleased]

## v1.1.0 - 2026-05-17

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
