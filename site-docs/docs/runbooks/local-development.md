# Local Development Runbook

## Prerequisites

- Go 1.22+
- Docker and Docker Compose

## Commands

```bash
make verify
make up
```

## Quick Checks

```bash
curl -fsS http://localhost:8080/healthz
curl -fsS http://localhost:8080/metrics | head
```
