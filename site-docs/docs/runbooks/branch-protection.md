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

- ci / verify
- ci / race-linux
- smoke / smoke-compose
- docs / docs-build
- pr-governance / governance

Keep `ci / verify` and `ci / race-linux` separate. The first covers the broader Go verification path, while the second makes Linux race detection visible as its own required signal.

## Admin Settings

- Include administrators in restrictions
- Restrict who can push directly to protected branch

## Merge Strategy

- Prefer squash merge for clean history
- Require conventional commit style in PR title or commit message
- Allow auto-merge only for Dependabot semver patch updates after required checks pass
