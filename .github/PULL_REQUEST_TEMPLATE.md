# Pull Request Template

## Summary

Describe what changed and why.

## Risk Level

- [ ] low
- [ ] medium
- [ ] high

## Type of Change

- [ ] feat
- [ ] fix
- [ ] refactor
- [ ] docs
- [ ] chore

## Validation

- [ ] `go test ./...` passes
- [ ] CI checks pass
- [ ] Backward compatibility assessed
- [ ] Rollback strategy documented

## Security and Privacy

- [ ] No secrets added to repository
- [ ] Any new endpoint has authz/authn considered
- [ ] Logging does not expose sensitive data

## Observability

- [ ] Metrics/logs/traces updated where behavior changed
- [ ] Alerting impact reviewed (if applicable)

## Rollout Plan

Describe deployment or migration impact.

## Type-Specific Checklist

- [ ] Service change checklist reviewed
- [ ] Infrastructure change checklist reviewed
- [ ] Deployment/GitOps checklist reviewed
- [ ] Security checklist reviewed

Reference: `docs/pr-checklists.md`

## Checklist

- [ ] Docs updated
- [ ] Changelog entry (if needed)
- [ ] Linked issue/ADR
