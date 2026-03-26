# Repository Structure Standards

This document defines the expected structure and ownership boundaries for this monorepo.

## Top-Level Domains

- `services/`: deployable application services.
- `infrastructure/`: cloud and platform provisioning.
- `deploy/`: runtime manifests, GitOps definitions, and monitoring deployment assets.
- `site-docs/`: published technical documentation (MkDocs).
- `docs/`: internal planning and communication artifacts.

## Placement Rules

- Put runnable binaries in `services/<service>/cmd`.
- Keep service internals private under `services/<service>/internal`.
- Keep Kubernetes base manifests generic and overlays environment-specific.
- Keep Terraform root composition in `infrastructure/terraform` and reusable units in `infrastructure/terraform/modules`.
- Keep public-facing runbooks in `site-docs/docs/runbooks`.

## Naming Conventions

- Folder names: lowercase kebab-case.
- Terraform modules: noun-based names (`networking`, `database`, `security`).
- Deployment overlays: environment names (`staging`, `prod`).
- Documentation files: descriptive kebab-case.

## Documentation Minimum

Any new top-level domain or service must include a `README.md` with:

- purpose and scope,
- structure map,
- local development or usage flow,
- operational and safety notes.

## Change Discipline

- Prefer small, reviewable pull requests.
- Avoid mixing infrastructure, service logic, and docs changes unless tightly related.
- Update docs in the same PR when behavior or structure changes.
