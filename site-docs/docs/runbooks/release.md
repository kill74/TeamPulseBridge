# Release Runbook

## Versioning

Semantic Versioning is used: `vMAJOR.MINOR.PATCH`.

## Steps

1. Ensure the default branch head is green for `ci`, `smoke`, `docs`, and `integration`.
2. Trigger `tag-release` workflow on default branch with a valid SemVer tag.
3. Complete the remaining human checklist gates in workflow inputs for risk, rollback, and monitoring readiness.
4. Wait for `release` workflow to build, sign, verify, and publish release artifacts.
5. Confirm changelog update commit on default branch.

## Signed Artifacts

The release workflow publishes signed source and binary artifacts:

- `teampulsebridge-<version>-source.tar.gz`
- `teampulsebridge-<version>-source.zip`
- `teampulsebridge-<version>-ingestion-gateway-linux-amd64.tar.gz`
- `teampulsebridge-<version>-ingestion-gateway-linux-arm64.tar.gz`
- `teampulsebridge-<version>-ingestion-gateway-darwin-arm64.tar.gz`
- `teampulsebridge-<version>-ingestion-gateway-windows-amd64.zip`
- `SHA256SUMS`
- `release-checklist.md`
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
