# Ingestion Gateway

## Responsibilities

- Receive webhook requests from supported providers
- Validate signatures/tokens per provider contract
- Acknowledge quickly and enqueue asynchronously
- Emit metrics and traces for every request path

## Security Controls

- Slack HMAC verification
- GitHub `X-Hub-Signature-256` verification
- GitLab token verification
- Teams client state validation
- Optional JWT protection for operational endpoints

## Operational Endpoints

- `GET /healthz`
- `GET /readyz`
- `GET /metrics`
- `GET /admin/configz`
