# Contributing

Thanks for your interest in improving TeamPulse Bridge.

## Development Workflow

1. Create a feature branch from `main`.
2. Keep changes small and focused.
3. Run `go test ./...` in `services/ingestion-gateway` before opening a PR.
4. Ensure CI checks pass.

## Commit Style

Use clear, imperative commit messages.

Examples:

- `feat(ingestion): add pubsub publisher retries`
- `fix(auth): validate jwt audience`
- `chore(ci): add govulncheck`

## Pull Request Checklist

- [ ] Tests added/updated
- [ ] No secrets committed
- [ ] Docs updated if behavior changed
- [ ] Backward compatibility considered

## Branch Protection Expectations

- CI (`ci`), smoke (`smoke`), and docs (`docs`) checks must be green before merge.
- At least one approving review is required.
- Direct pushes to the default branch should be restricted.

## Security

If you find a security issue, avoid opening a public issue with exploit details.
Create a private report to the maintainer.

See `SECURITY.md` for responsible disclosure details.
