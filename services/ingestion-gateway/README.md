# Ingestion Gateway (Go)

Minimal production-oriented webhook ingress service for TeamPulse Bridge.

## Endpoints

- `POST /webhooks/slack`
- `POST /webhooks/teams`
- `POST /webhooks/github`
- `POST /webhooks/gitlab`
- `GET /healthz`
- `GET /readyz`
- `GET /metrics`
- `GET /admin/configz`

## What is implemented

- Signature/token validation for Slack, Teams, GitHub, and GitLab
- Slack URL verification challenge handling
- Teams Graph validationToken handshake handling
- Async queue buffering via publisher abstraction
- Raw body size limiting (1 MiB)
- Request ID propagation (`X-Request-Id`)
- Structured JSON access logs
- Panic recovery middleware
- Startup config validation with fail-fast behavior
- Queue backend factory (`log` or `pubsub`)
- OpenTelemetry HTTP instrumentation
- Prometheus metrics endpoint
- Admin JWT protection for `/metrics` and `/admin/*` when enabled

## Environment Variables

- `PORT` (default: `8080`)
- `SLACK_SIGNING_SECRET`
- `GITHUB_WEBHOOK_SECRET`
- `GITLAB_WEBHOOK_TOKEN`
- `TEAMS_CLIENT_STATE`
- `QUEUE_BUFFER` (default: `4096`)
- `REQUEST_TIMEOUT_SEC` (default: `15`)
- `REQUIRE_SECRETS` (default: `true`)
- `QUEUE_BACKEND` (default: `log`, options: `log` | `pubsub`)
- `PUBSUB_PROJECT_ID` (required when `QUEUE_BACKEND=pubsub`)
- `PUBSUB_TOPIC_ID` (required when `QUEUE_BACKEND=pubsub`)
- `ADMIN_AUTH_ENABLED` (default: `false`)
- `ADMIN_JWT_ISSUER` (required when `ADMIN_AUTH_ENABLED=true`)
- `ADMIN_JWT_AUDIENCE` (required when `ADMIN_AUTH_ENABLED=true`)
- `ADMIN_JWT_SECRET` (required when `ADMIN_AUTH_ENABLED=true`)
- `OTEL_EXPORTER_OTLP_ENDPOINT` (optional; if set, traces are exported via OTLP HTTP)
- `ENVIRONMENT` (default: `dev`)

## Run

```bash
go run ./cmd/server
```

## Test

```bash
go test ./...
```

## Next production steps

- Add integration tests with signed webhook fixtures
- Add policy checks for privacy boundaries
- Add Google Secret Manager loader for runtime secrets
- Add OPA/Rego checks for webhook source allowlists and tenancy boundaries
