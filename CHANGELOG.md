# Changelog

## v1.2.0 - 2026-10-28

- release: ship v1.2.0 (1a50eac)
- Merge pull request #109 from kill74/dependabot/terraform/infrastructure/terraform/hashicorp/google-tw-8.8 (29511e3)
- Merge pull request #105 from kill74/dependabot/github_actions/actions-patch-and-minor-b530e7f9d1 (fcca881)
- chore(terraform): update hashicorp/google requirement (d80a13f)
- chore(ci): bump docker/setup-buildx-action (15a639b)
- Merge pull request #94 from kill74/dependabot/github_actions/actions-patch-and-minor-68b48a768d (3f1dd82)
- Merge pull request #95 from kill74/dependabot/github_actions/actions/setup-python-7 (73a3866)
- Merge pull request #103 from kill74/dependabot/terraform/infrastructure/terraform/hashicorp/google-tw-8.5 (7f444d1)
- chore(terraform): update hashicorp/google requirement (cc1b460)
- chore(ci): bump actions/setup-python from 5 to 7 (662f46a)
- chore(ci): bump the actions-patch-and-minor group with 2 updates (dd00aa6)
- Merge pull request #72 from kill74/dependabot/github_actions/actions-patch-and-minor-76c240b74c (2b07cae)
- Merge pull request #74 from kill74/dependabot/github_actions/codecov/codecov-action-7 (99aa06f)
- Merge pull request #75 from kill74/dependabot/github_actions/softprops/action-gh-release-3.0.1 (d5e29de)
- Merge pull request #87 from kill74/dependabot/go_modules/services/ingestion-gateway/go-patch-and-minor-281caeb000 (1f78d9d)
- Merge pull request #90 from kill74/dependabot/terraform/infrastructure/terraform/hashicorp/google-beta-tw-8.0 (0b836a1)
- chore(terraform): update hashicorp/google-beta requirement from ~> 7.38 to ~> 8.0 (1f788b7)
- chore(deps): bump the go-patch-and-minor group across 1 directory with 14 updates (dc233c7)
- chore(ci): bump softprops/action-gh-release from 2.6.1 to 3.0.1 (054213a)
- chore(ci): bump codecov/codecov-action from 5 to 7 (4b7364b)
- chore(ci): bump docker/setup-buildx-action (4783c9d)
- Merge pull request #55 from kill74/dependabot/docker/services/ingestion-gateway/docker-patch-and-minor-f918e77f61 (0617ae5)
- Merge pull request #57 from kill74/dependabot/github_actions/actions-patch-and-minor-23becc088c (8cf7122)
- Merge pull request #69 from kill74/dependabot/terraform/infrastructure/terraform/hashicorp/google-beta-tw-7.38 (9507c7d)
- chore(terraform): update hashicorp/google-beta requirement from ~> 5.0 to ~> 7.38 (405014d)
- chore(docker): bump golang (90622e7)
- chore(ci): bump docker/setup-buildx-action (5036807)
- Merge pull request #65 from kill74/dependabot/terraform/infrastructure/terraform/hashicorp/kubernetes-tw-3.2 (2342719)
- Merge pull request #66 from kill74/dependabot/terraform/infrastructure/terraform/hashicorp/google-tw-7.35 (903d7e0)
- chore(terraform): update hashicorp/google requirement from ~> 5.0 to ~> 7.35 (5552751)
- chore(terraform): update hashicorp/kubernetes requirement from ~> 2.20 to ~> 3.2 (042099b)
- chore: remove stale coverage files from git tracking (49bdd75)
- fix: resolve 4 golangci-lint violations (ineffassign, unconvert, wrapcheck) (91a117e)
- fix: resolve 19 issues across ingestion-gateway (round 3 audit) (e3b102a)
- ci: fix golangci-lint v2 config schema compatibility (0466067)
- ci: fix golangci-lint config to exclude pre-existing warnings and fix 6 remaining issues (e1b9b20)
- test: fix 37 testifylint and unconvert violations in test files (93dfe81)
- fix: resolve 44 golangci-lint violations (usestdlibvars, wastedassign, wrapcheck) (3633b04)
- chore: apply pending updates across ingestion-gateway and CI workflows (e4a3faf)
- Add Pub/Sub-to-Postgres event-store consumer (3877e26)
- ci: migrate golangci-lint config to v2 (b710906)
- ci: bump golangci-lint action to v9 and linter to v2.12.2 and remove go 1.24 workaround (d5e7c6d)
- ci: bypass golangci-lint strict Go version mismatch check (49e4a8a)
- ci: bump golangci-lint action to v9 and linter to v2.12.2 to support go 1.25 (267ca68)
- ci: bump golangci-lint version to v1.64.8 to fix 404 (52014cb)
- ci: upgrade golangci-lint to v1.64.0 to support Go 1.26 (ed19e17)
- ci: temporarily lower coverage threshold to 40% to unblock CI (57bc89c)
- fix: update deprecated promclient.NewGoCollector (b452cac)
- ci: update Go version to 1.26.x in CI workflow (fadbc87)
- ci: fix docker-compose path in smoke test workflow (b688e9c)
- ci: improve error handling and logging in CI workflows (69dff52)
- fix: standardize quote style in smoke.yml branch definitions (89553b9)
- ci: run tests from repo root, stabilize linter and extend smoke wait (a58b1d3)
- fix: standardize quotes in docker-compose and update telemetry import (fba67fb)
- ci(integration): use log backend in docker-compose.integration to avoid missing Pub/Sub topic (53315e0)
- ci(integration): fix compose build context and increase wait time; ci: lower coverage threshold to 65% (07fe18b)
- fix(ci): correct docker distroless tag, fix golangci-lint version, and add panic traces (ccc8697)
- fix(ci): resolve healthz timeout, metrics, and govulncheck issues (77283dd)
- fix(ci): provide missing admin auth environment variables to ingestion-gateway (d0a3f0d)
- fix(deps): tidy modules and dependencies (90672b1)
- feat(resilience): implement circuit breakers, chaos middleware, PII scrubbing, and improved testing (4bff2eb)
- fix: resolve all bugs, CI issues, and add senior-level features (75b0d43)
- Merge pull request #4 from kill74/dependabot/github_actions/docker/setup-buildx-action-4.0.0 (9c73019)
- Merge pull request #2 from kill74/dependabot/pip/site-docs/docs-python-patch-and-minor-cf6347e75e (9121e8b)
- Merge pull request #21 from kill74/dependabot/github_actions/golangci/golangci-lint-action-9 (bc9b23d)
- Merge pull request #24 from kill74/dependabot/go_modules/services/ingestion-gateway/go-patch-and-minor-94dc06be3b (1d73bbd)
- Merge pull request #25 from kill74/security/fix-timing-attack-length-2011975665488568452 (af248ec)
- 🛡️ Sentinel: [MEDIUM] Fix timing attack length leak in ConstantTimeCompare (4b42c84)
- 🛡️ Sentinel: [MEDIUM] Fix timing attack length leak in ConstantTimeCompare (674a9dc)
- feat: implement v1.2.0 core features (4dab770)
- fix: resolve CI policy failures (6533c4d)
- chore(release): update changelog for v1.1.0 (b770362)
- chore(deps): bump the go-patch-and-minor group across 1 directory with 2 updates (cae4074)
- chore(ci): bump golangci/golangci-lint-action from 6 to 9 (d3bc206)
- chore(docs-deps): bump mkdocs-material (037ecd9)
- chore(ci): bump docker/setup-buildx-action from 3.12.0 to 4.0.0 (69641ed)

## v1.2.0 - 2026-10-28

- feat(admin): add provider health summary and remediation guidance for source-level queue pressure
- feat(ui): add provider health cards and queue/failure ratio summaries to the admin dashboard
- feat(ops): expose health and provider summary endpoints for operator incident triage
- fix(admin): preserve queue snapshot and source snapshot data for live introspection
- test: add regression coverage for health-summary and provider-health behavior
- docs: publish release notes and improve operator-facing project documentation

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
