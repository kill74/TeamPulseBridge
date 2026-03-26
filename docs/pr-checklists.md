# Pull Request Checklists

Use this guide together with the PR template to keep changes safe, reviewable, and production-ready.

## Service Change Checklist

- [ ] API or handler behavior changes are documented.
- [ ] Unit tests cover success, failure, and edge cases.
- [ ] Integration tests are updated when contracts changed.
- [ ] Logs, metrics, and traces were reviewed for observability impact.
- [ ] Timeouts, retries, and rate limits were evaluated.

## Infrastructure Change Checklist

- [ ] Terraform plan reviewed by another engineer.
- [ ] Variable defaults and validations are still safe.
- [ ] Destructive operations were identified and approved.
- [ ] Security and IAM impact reviewed.
- [ ] Rollback path is documented.

## Deployment and GitOps Checklist

- [ ] Kustomize render/validation completed.
- [ ] Overlay changes are environment-scoped and minimal.
- [ ] Resource requests/limits and probes are appropriate.
- [ ] Production sync behavior and promotion steps are clear.
- [ ] Runbook updates included for operational changes.

## Documentation Change Checklist

- [ ] Documentation is accurate against current behavior.
- [ ] References and links are valid.
- [ ] Ownership and operational responsibilities are clear.
- [ ] Examples do not contain secrets or sensitive values.

## Security Review Checklist

- [ ] No credentials, keys, or tokens are committed.
- [ ] Access controls follow least privilege.
- [ ] Input validation and auth boundaries were considered.
- [ ] Sensitive fields are redacted in logs and errors.

## Release and Risk Checklist

- [ ] Risk level is declared (low, medium, high).
- [ ] Rollout strategy and blast radius are documented.
- [ ] Monitoring and alert verification steps are listed.
- [ ] Post-deploy verification criteria are explicit.
