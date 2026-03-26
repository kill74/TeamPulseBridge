# Release Runbook

## Versioning

Semantic Versioning is used: `vMAJOR.MINOR.PATCH`.

## Steps

1. Run verify and smoke checks on main branch.
2. Trigger `tag-release` workflow on default branch with a valid SemVer tag.
3. Complete all automated checklist gates in workflow inputs.
4. Wait for `release` workflow to publish release notes and signed artifacts.
5. Confirm changelog update commit on default branch.

## Signed Artifacts

The release workflow publishes source artifacts and signatures:

- `teampulsebridge-<version>-source.tar.gz`
- `teampulsebridge-<version>-source.zip`
- `SHA256SUMS`
- Cosign keyless signatures (`.sig`) and certificates (`.pem`) for each artifact

Verification example:

```bash
cosign verify-blob \
  --signature teampulsebridge-<version>-source.tar.gz.sig \
  --certificate teampulsebridge-<version>-source.tar.gz.pem \
  --certificate-identity "https://github.com/kill74/TeamPulseBridge/.github/workflows/release.yml@refs/tags/v<version>" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
  teampulsebridge-<version>-source.tar.gz
```

## Rollback

- If release is invalid, create a patch release with fix.
- Do not rewrite published tags.
