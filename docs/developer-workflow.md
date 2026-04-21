# Developer Workflow

This guide is the shortest path from clone to productive local development.

## First 5 Minutes

If you only want to run the product locally and do not need the full contributor toolchain yet, use the installer:

```bash
bash ./scripts/install.sh
```

PowerShell alternative:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\install.ps1
```

That path is ideal for evaluators, demos, and new teammates who first want to see the system working before they install lint, policy, and Terraform tooling.

If you plan to contribute code, run these commands from the repository root:

```bash
make env-init
make dev-setup
make doctor
make dev-check
```

If you work from PowerShell on Windows and do not use `make`, use:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\env-init.ps1
powershell -ExecutionPolicy Bypass -File .\scripts\ci-local.ps1 -SkipSmoke -SkipRace
```

What this gives you:

- a local `.env` file based on the repo defaults
- pre-commit and pre-push hooks
- `golangci-lint`, `govulncheck`, and `checkov`
- an early signal if Docker, ports, or required config are missing

## Daily Loop

Use these commands most often:

- `make run` to run the gateway directly
- `make up` to run the full local stack with Prometheus and Grafana
- `make dev-check` for a fast local sanity pass while iterating
- `make replay FILE=... REPLAY_ARGS='-source github -dry-run'` to test payloads safely

## Before You Push

Run the same classes of checks that run on push in GitHub Actions:

```bash
make ci-local
```

If your change adds or modifies webhook provider contracts, also run:

```bash
make ci-contract
```

PowerShell equivalent:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\ci-local.ps1
```

If your machine is still missing some local dependencies, you can run a partial pass with:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\ci-local.ps1 -SkipSmoke -SkipPolicy -SkipTerraform -SkipRace
```

That command runs:

- Go formatting, `go vet`, unit tests, lint, and vulnerability scan
- race detector tests
- fixture catalog lint plus targeted contract checks
- Terraform fmt/init/validate
- Checkov policy checks for Terraform and Kubernetes
- docker compose smoke validation against `/healthz` and `/metrics`

## Common Local Blockers

If local checks fail before your code even runs, check these first:

- Docker daemon is not running
- local ports `8080`, `9090`, `3000`, or `8085` are already occupied
- `terraform`, `checkov`, `golangci-lint`, or `govulncheck` are not on your `PATH`
- your Go patch version is outdated, so `govulncheck` is flagging standard library advisories until Go is updated
- Windows race tests need `CGO_ENABLED=1` plus a local C toolchain, otherwise use `-SkipRace` for a partial pass
- `.env` is missing or has `QUEUE_BACKEND=pubsub` without Pub/Sub identifiers
- admin auth is enabled locally but `ADMIN_JWT_SECRET` is still weak or unset

`make doctor` now surfaces these issues early so developers do not have to discover them from CI.
