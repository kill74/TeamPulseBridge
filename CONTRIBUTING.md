# Contributing

Thanks for your interest in improving TeamPulse Bridge.

## Engineering Principles

- Prefer small, reviewable changes over large rewrites.
- Preserve backward compatibility unless the change is explicitly breaking.
- Ship code and documentation together when behavior changes.
- Optimize for operability: observability, security, and rollback paths.

## Development Workflow

1. Create a feature branch from `main`.
2. Keep changes small and focused.
3. Run local verification before opening a PR.
4. Ensure CI checks pass.

First-time setup (repository root):

```bash
make dev-setup
```

Fast local quality gate:

```bash
make dev-check
```

Recommended pre-PR command sequence (repository root):

```bash
make verify
make integration-test
```

If your change is infrastructure-related, also run:

```bash
make infra-plan-staging
```

If your change is deployment-related, also run:

```bash
make gitops-validate
```

## Branch Naming

Use explicit branch names:

- `feat/<scope>-<short-description>`
- `fix/<scope>-<short-description>`
- `chore/<scope>-<short-description>`
- `docs/<scope>-<short-description>`

## Commit Style

Use clear, imperative commit messages.

Examples:

- `feat(ingestion): add pubsub publisher retries`
- `fix(auth): validate jwt audience`
- `chore(ci): add govulncheck`

Keep the first line concise and explain rationale in the body for non-trivial changes.

## Pull Request Checklist

- [ ] Tests added/updated
- [ ] No secrets committed
- [ ] Docs updated if behavior changed
- [ ] Backward compatibility considered
- [ ] Rollback strategy considered (when applicable)
- [ ] Risk level documented in PR description

## Branch Protection Expectations

- CI (`ci`), smoke (`smoke`), and docs (`docs`) checks must be green before merge.
- At least one approving review is required.
- Direct pushes to the default branch should be restricted.

## Definition of Done

A change is done when:

- Functional and non-functional requirements are met.
- Tests and static checks are green.
- Operational impact is documented (logs, metrics, alerts, or runbook updates).
- Documentation is updated in the same PR.

See repository structure and ownership conventions in `docs/repository-standards.md`.

## Security

If you find a security issue, avoid opening a public issue with exploit details.
Create a private report to the maintainer.

See `SECURITY.md` for responsible disclosure details.
