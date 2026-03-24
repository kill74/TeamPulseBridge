# Release Runbook

## Versioning

Semantic Versioning is used: `vMAJOR.MINOR.PATCH`.

## Steps

1. Run verify and smoke checks on main branch.
2. Trigger `tag-release` workflow with a valid SemVer tag.
3. Wait for `release` workflow to publish release notes.
4. Confirm changelog update commit.

## Rollback

- If release is invalid, create a patch release with fix.
- Do not rewrite published tags.
