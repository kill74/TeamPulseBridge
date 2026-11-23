# Branch Protection Runbook

Use these settings on the default branch to keep quality and release safety high.

## Recommended Rules

- Require pull request before merging
- Require at least 1 approving review
- Dismiss stale approvals when new commits are pushed
- Require conversation resolution before merge
- Require status checks to pass before merge
- Require linear history
- Block force pushes and branch deletion

## Required Status Checks

Check names are `<workflow name> / <job id>` as defined in `.github/workflows/`:

- CI / ci (unit tests with `-race`, 40% coverage gate, lint)
- CI / contracts (fixture lint + webhook contract tests)
- smoke / smoke-compose (integration compose health + metrics)
- smoke / smoke-root-compose (full root-stack parity with `make ci-smoke`)
- docs / docs-build (`mkdocs build --strict`)
- Integration Tests / integration-tests (Pub/Sub emulator suite)
- Integration Tests / docker-compose-integration
- Policy / policy (checkov + IaC policy checks)
- Terraform / terraform (fmt, init, validate)
- security-scan / trivy-image-scan, trivy-fs-scan, govulncheck
- pr-governance / governance

## Admin Settings

- Include administrators in restrictions
- Restrict who can push directly to protected branch

## Merge Strategy

- Prefer squash merge for clean history
- Require conventional commit style in PR title or commit message
- Allow auto-merge only for Dependabot semver patch updates after required checks pass
