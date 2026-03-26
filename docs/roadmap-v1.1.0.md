# Roadmap v1.1.0

## 1. Reliability and Resilience

- Add chaos drills for regional failover and recovery time validation.
- Introduce canary rollout policies with automatic rollback gates.
- Add queue backpressure adaptive controls with failure budget-aware throttling.

## 2. Security and Governance

- Add policy-as-code checks for Terraform and Kubernetes manifests in CI.
- Enforce stronger default IAM role constraints and deny-list unsafe permissions in production.
- Add structured security event audit stream and retention policy.

## 3. Contract and Data Quality

- Expand webhook contract suite with versioned fixture catalog and schema drift checks.
- Add negative-provider contract cases for malformed but common real-world payload variants.
- Publish compatibility matrix per provider webhook event family.

## 4. Observability and Incident Response

- Add dedicated security operations dashboard with top offenders and timeline overlays.
- Add burn-rate-style alerts for security rejection anomalies.
- Add runbook automation snippets for first-response triage.

## 5. Developer Experience and Delivery

- Add pre-merge contract test target and fixture linting.
- Add release checklist automation and signed release artifacts.
- Add contribution templates for new provider integrations with contract requirements.

## Exit Criteria for v1.1.0

- Zero high-severity security findings open for runtime ingress path.
- Successful multi-region failover game day under defined RTO/RPO targets.
- Contract tests green across all supported providers on every main-branch commit.
