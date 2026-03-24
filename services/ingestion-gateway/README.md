# Ingestion Gateway (Go)

Production-oriented webhook ingress for TeamPulse Bridge.

This service validates inbound signatures/tokens, applies safety middleware,
and publishes normalized webhook envelopes to the configured queue backend.

## API Surface

- `POST /webhooks/slack`
- `POST /webhooks/teams`
- `POST /webhooks/github`
- `POST /webhooks/gitlab`
- `GET /healthz`
- `GET /readyz`
- `GET /metrics`
- `GET /admin/configz`

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
