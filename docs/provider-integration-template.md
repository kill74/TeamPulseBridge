# Provider Integration Template

Use this template when proposing or implementing a new webhook provider in TeamPulse Bridge.

The goal is to keep every provider integration reviewable, observable, and contract-tested from the first PR.

## 1. Provider Summary

- Provider:
- Owner:
- Link to issue/ADR:
- Why this provider belongs in the ingestion gateway:

## 2. Authentication Boundary

- Auth model:
- Required headers/tokens/signature inputs:
- Replay or handshake edge cases:
- Expected rejection modes:

## 3. Supported Event Families

- Event family:
  - Purpose:
  - Publish behavior:
  - Expected HTTP status:

Repeat for each family that the first release supports.

## 4. Contract Artifacts

- [ ] Add baseline fixtures to `services/ingestion-gateway/internal/handlers/testdata/contracts/`
- [ ] Register fixtures in `services/ingestion-gateway/internal/handlers/testdata/contracts/catalog-v1.json`
- [ ] Add malformed or degraded negative fixtures that reflect real provider behavior
- [ ] Update `services/ingestion-gateway/docs/WEBHOOK_COMPATIBILITY_MATRIX.md`
- [ ] Confirm publishable fixtures remain compatible with `raw-webhook-envelope-v1.schema.json`
- [ ] Run `make contract-lint`
- [ ] Run `make ci-contract`

Fixture planning table:

| event_family | variant | negative | publish | expected_status | fixture_path | notes |
| --- | --- | --- | --- | --- | --- | --- |
| example_event | baseline | false | true | 202 | provider_example_event.json | canonical payload |
| example_event | sparse_payload | true | true | 202 | provider_example_event_sparse.json | common redacted payload |

## 5. Runtime Changes

- Config additions:
- Secret management changes:
- Handler or signature validation changes:
- Replay and failed-event implications:
- Smoke-test guidance:

## 6. Observability and Operations

- Metrics/labels impacted:
- New logs or alert considerations:
- Dashboard/runbook impact:
- Operator-facing docs updated:

## 7. Delivery Checklist

- [ ] Unit tests cover success and failure paths
- [ ] Contract tests cover baseline and negative fixtures
- [ ] Docs updated in the same PR
- [ ] Rollback strategy documented
- [ ] Blast radius understood
