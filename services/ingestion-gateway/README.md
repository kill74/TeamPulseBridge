# Ingestion Gateway (Go)

Production-oriented webhook ingress for TeamPulse Bridge.

This service validates inbound signatures/tokens, applies safety middleware,
and publishes normalized webhook envelopes to the configured queue backend.

## API Surface

- `GET /`
- `POST /webhooks/slack`
- `POST /webhooks/teams`
- `POST /webhooks/github`
- `POST /webhooks/gitlab`
- `GET /healthz`
- `GET /readyz`
- `GET /metrics`
- `GET /admin/configz`
- `POST /ui/smoke-test` (operator UI internal proxy)

### Built-in Product UI (`GET /`)

The root route serves an operator console with:

- live health/readiness checks,
- admin config visibility checks,
- optional JWT token mode for authenticated admin requests,
- guarded webhook smoke testing (explicit enable switch),
- extra-header injection for provider-specific test scenarios,
- strict browser security headers and CSP,
- versioned static UI assets (`/assets/ui.css` and `/assets/ui.js`),
- server-side smoke-test proxy with per-IP rate limiting.

## Core Guarantees

- Signature/token validation for Slack, Teams, GitHub, and GitLab
- Slack URL verification (`challenge`) and Teams validation token handshake
- Request body cap (1 MiB) and panic-safe request handling
- Request ID propagation through `X-Request-Id`
- Structured logs and metrics-ready HTTP middleware
- Queue abstraction with `log` and `pubsub` backends
- Fail-fast startup configuration validation
- Optional JWT guard for operational endpoints

## Configuration Contract

### Runtime Variables

- `PORT` (default: `8080`)
- `REQUEST_TIMEOUT_SEC` (default: `15`)
- `RATE_LIMIT_ENABLED` (default: `true`)
- `RATE_LIMIT_RPM` (default: `300`)
- `ADMIN_RATE_LIMIT_RPM` (default: `60`)
- `TRUSTED_PROXY_CIDRS` (optional comma-separated CIDRs; only these proxies are trusted for `X-Forwarded-For` and `X-Real-IP`)
- `QUEUE_BUFFER` (default: `4096`)
- `QUEUE_BACKEND` (default: `log`; options: `log|pubsub`)
- `REQUIRE_SECRETS` (default: `true`)
- `ENVIRONMENT` (default: `dev`)
- `OTEL_EXPORTER_OTLP_ENDPOINT` (optional)

### Webhook Secrets

- `SLACK_SIGNING_SECRET`
- `GITHUB_WEBHOOK_SECRET`
- `GITLAB_WEBHOOK_TOKEN`
- `TEAMS_CLIENT_STATE`

### Pub/Sub Variables (required when `QUEUE_BACKEND=pubsub`)

- `PUBSUB_PROJECT_ID`
- `PUBSUB_TOPIC_ID`
- `PUBSUB_EMULATOR_HOST` (local/integration only)

### Admin JWT (required when `ADMIN_AUTH_ENABLED=true`)

- `ADMIN_AUTH_ENABLED` (default: `false`)
- `ADMIN_JWT_ISSUER`
- `ADMIN_JWT_AUDIENCE`
- `ADMIN_JWT_SECRET`
- `ADMIN_ALLOW_CIDRS` (comma-separated CIDRs; required in production-like envs when admin auth is enabled)

Security constraints:

- `ADMIN_JWT_SECRET` must be at least 32 characters and must not be a weak default value.
- `REQUIRE_SECRETS=false` is only accepted for non-production environments (`local`, `dev`, `test`, `ci`, `staging`, `sandbox`, and `integration-test` variants).
- `/admin/*` and `/metrics` can be restricted by source IP via `ADMIN_ALLOW_CIDRS`.
- `X-Forwarded-For`/`X-Real-IP` are only trusted when the immediate source IP matches `TRUSTED_PROXY_CIDRS`.

## Local Development

### Run Service

```bash
go run ./cmd/server
```

### Run Unit + Package Tests

```bash
go test ./...
```

### Run Integration Tests With Pub/Sub Emulator

From repository root:

```bash
make integration-test
```

Targeted runs:

```bash
make integration-test-queue
make integration-test-handlers
make integration-bench
```

Integration tests skip automatically when `PUBSUB_EMULATOR_HOST` is not set.

## Operational Notes

- Prefer `REQUIRE_SECRETS=true` outside local development.
- Keep `ADMIN_AUTH_ENABLED=true` in shared environments.
- Keep `QUEUE_BACKEND=pubsub` in staging/prod for durability.
- Alert on sustained `5xx` and queue publish failures.

## Troubleshooting

### 401/403 on webhook endpoints

- Verify secret/token variables are set and current.
- Validate provider signature header names and payload integrity.

### 503 or 500 during publish

- Check queue backend connectivity.
- For Pub/Sub, validate topic existence and IAM/credentials.
- For emulator, ensure `PUBSUB_EMULATOR_HOST` points to a reachable endpoint.

### Empty metrics/admin responses

- Confirm `ADMIN_AUTH_ENABLED` and JWT claims (`iss`, `aud`) configuration.
- Check middleware ordering if custom wiring was introduced.
