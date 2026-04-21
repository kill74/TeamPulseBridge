# Pull Request Template

## Summary

Describe what changed and why.

## Linked Work

- Issue/ADR/Ticket:

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

## Required Labels

- [ ] One risk label is set (`risk:low`, `risk:medium`, or `risk:high`)
- [ ] One type label is set (`type:*`)

## Validation

- [ ] `go test ./...` passes
- [ ] CI checks pass
- [ ] Backward compatibility assessed
- [ ] Rollback strategy documented
- [ ] `make dev-check` passes locally

## Rollout and Rollback

- Rollout strategy:
- Blast radius:
- Rollback command/path:

## Security and Privacy

- [ ] No secrets added to repository
- [ ] Any new endpoint has authz/authn considered
- [ ] Logging does not expose sensitive data

## Observability

- [ ] Metrics/logs/traces updated where behavior changed
- [ ] Alerting impact reviewed (if applicable)

## Provider Integration Checklist (if applicable)

- [ ] Added or updated versioned fixtures in `internal/handlers/testdata/contracts/catalog-v1.json`
- [ ] Added baseline and malformed/negative provider payload variants
- [ ] Updated `services/ingestion-gateway/docs/WEBHOOK_COMPATIBILITY_MATRIX.md`
- [ ] Updated config, smoke-test, and operator documentation for the provider

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
- [ ] Monitoring/alerts verification steps included
