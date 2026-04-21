# Webhook Compatibility Matrix

The ingestion gateway accepts raw webhook payloads as an ingress contract, validates the provider-specific authentication boundary, and then preserves the request body inside the versioned `raw-webhook-envelope` queue schema.

This document is the operator-facing view of the same source of truth that powers the contract suite:

- fixture catalog: `internal/handlers/testdata/contracts/catalog-v1.json`
- queue schema: `internal/queue/testdata/schemas/raw-webhook-envelope-v1.schema.json`

## Support Matrix

| provider | event_family | support_status | positive_fixture | negative_fixture | publish_behavior | notes |
| --- | --- | --- | --- | --- | --- | --- |
| slack | event_callback | supported | `slack.event_callback.baseline.v1` | `slack.event_callback.sparse_payload.v1` | publish accepted raw payload | Sparse authenticated payloads still publish so downstream parsers can make the policy decision. |
| slack | url_verification | supported | `slack.url_verification.baseline.v1` | n/a | challenge echoed, no publish | Registration handshake stays special-cased and intentionally bypasses queue publish. |
| github | pull_request | supported | `github.pull_request.baseline.v1` | `github.pull_request.redacted_nested_fields.v1` | publish accepted raw payload | Redacted nested objects remain compatible as long as the request is authenticated. |
| gitlab | merge_request | supported | `gitlab.merge_request.baseline.v1` | `gitlab.merge_request.truncated_attributes.v1` | publish accepted raw payload | Minimal `object_attributes` payloads are preserved to avoid dropping degraded upstream deliveries. |
| teams | change_notification | supported | `teams.change_notification.baseline.v1` | `teams.change_notification.sparse_entries.v1` | publish accepted raw payload | Sparse change notifications still produce a stable fallback event id from the raw body. |

## Versioning Rules

- Add a new fixture version when a provider payload shape becomes materially different or a real-world edge case is promoted into regression coverage.
- Keep old fixtures until downstream consumers no longer need compatibility guarantees for that version line.
- Update the matrix in the same change that updates `catalog-v*.json` so human-readable support status cannot drift away from the executable contract suite.

## Drift Checks

The contract suite enforces three layers:

1. `catalog-v1.json` is validated for completeness, uniqueness, and JSON readability.
2. catalog fixtures that should publish must remain compatible with `raw-webhook-envelope-v1.schema.json`.
3. every provider/event family listed in the catalog must also appear in this matrix.
