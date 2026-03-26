# Services

This folder hosts runnable services in the monorepo.

## Current Services

- `ingestion-gateway/`: Hardened webhook ingress service built in Go.

## Service Contract

Each service should include:

- `README.md` with API and runbook details.
- `cmd/` entrypoints.
- `internal/` private application packages.
- Tests close to implementation (`*_test.go`) and integration docs if needed.

## Engineering Guidelines

- Keep service boundaries explicit.
- Avoid cross-service imports through `internal/` packages.
- Expose shared behavior via dedicated libraries only when reuse is proven.
