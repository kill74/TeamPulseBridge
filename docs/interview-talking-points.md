# Interview Talking Points

Use this document as a concise script for interviews and portfolio walkthroughs.

## 1. Problem Framing

Team collaboration signals are fragmented across chat and code systems. This project provides a production-style ingestion gateway that normalizes and routes those events with security and observability first.

## 2. Key Design Decisions

- Stateless webhook ingress for horizontal scaling
- Signature validation at the edge before enqueue
- Async publish path to decouple inbound latency from downstream processing
- Provider-specific verification logic isolated in internal packages
- Operational endpoints separated from webhook endpoints with optional JWT guard

## 3. Reliability Tradeoffs

- Chosen: asynchronous buffering and queue abstraction
- Benefit: low webhook response latency and resilience under bursts
- Tradeoff: eventual consistency in downstream processing path
- Mitigation: explicit metrics and alertable endpoints

## 4. Security Tradeoffs

- Chosen: strict signature checks + fail-fast startup validation
- Benefit: reduced spoofing risk and safer misconfiguration behavior
- Tradeoff: stricter config requirements in some local/dev flows
- Mitigation: `REQUIRE_SECRETS=false` in controlled local environments only

## 5. Observability Tradeoffs

- Chosen: OpenTelemetry plus Prometheus plus structured logs
- Benefit: fast root-cause analysis and measurable SLO signals
- Tradeoff: slightly higher implementation complexity and dependencies
- Mitigation: clear package boundaries and middleware composition

## 6. CI/CD Maturity Signals

- CI verifies formatting, vet, tests, race checks, lint, and vulnerability scan
- Smoke workflow validates containerized runtime and core endpoints
- Release workflow enforces SemVer tags and changelog updates
- Docs workflow builds and deploys engineering docs to GitHub Pages

## 7. What I Would Build Next

- Pub/Sub emulator integration tests
- Dead-letter replay and runbook automation
- SLO dashboards with p95/p99 service-level indicators
- Multi-service expansion (normalizer, query API, persistence worker)

## 8. 60-Second Pitch

I built this like a real backend platform service, not just a demo API. It has signed webhook ingress, async queue design, observability instrumentation, security controls, release automation, and governance docs. The repo is optimized for both engineering execution and reviewer clarity.
