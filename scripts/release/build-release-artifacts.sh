#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

usage() {
  cat <<'EOF'
Build TeamPulse Bridge release artifacts for the provided SemVer tag.

Usage:
  bash ./scripts/release/build-release-artifacts.sh v1.2.3
EOF
}

fail() {
  printf 'Error: %s\n' "$1" >&2
  exit 1
}

[[ $# -eq 1 ]] || {
  usage
  exit 1
}

TAG="$1"
[[ "$TAG" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || fail "tag must match vMAJOR.MINOR.PATCH"

command -v git >/dev/null 2>&1 || fail "git is required"
command -v go >/dev/null 2>&1 || fail "go is required"
command -v tar >/dev/null 2>&1 || fail "tar is required"
command -v zip >/dev/null 2>&1 || fail "zip is required"
command -v sha256sum >/dev/null 2>&1 || fail "sha256sum is required"

cd "${REPO_ROOT}"

VERSION="${TAG#v}"
artifacts=()

build_binary_archive() {
  local goos="$1"
  local goarch="$2"
  local format="$3"
  local tmpdir
  tmpdir="$(mktemp -d)"
  local binary_name="ingestion-gateway"
  local ext=""

  if [[ "${goos}" == "windows" ]]; then
    ext=".exe"
  fi

  (
    cd services/ingestion-gateway
    CGO_ENABLED=0 GOOS="${goos}" GOARCH="${goarch}" go build -trimpath -ldflags="-s -w" -o "${tmpdir}/${binary_name}${ext}" ./cmd/server
  )

  local archive_base="teampulsebridge-${VERSION}-ingestion-gateway-${goos}-${goarch}"
  if [[ "${format}" == "zip" ]]; then
    (
      cd "${tmpdir}"
      zip -q "${REPO_ROOT}/${archive_base}.zip" "${binary_name}${ext}"
    )
    artifacts+=("${archive_base}.zip")
  else
    tar -C "${tmpdir}" -czf "${archive_base}.tar.gz" "${binary_name}${ext}"
    artifacts+=("${archive_base}.tar.gz")
  fi

  rm -rf "${tmpdir}"
}

git archive --format=tar.gz --output "teampulsebridge-${VERSION}-source.tar.gz" "${TAG}"
git archive --format=zip --output "teampulsebridge-${VERSION}-source.zip" "${TAG}"
artifacts+=(
  "teampulsebridge-${VERSION}-source.tar.gz"
  "teampulsebridge-${VERSION}-source.zip"
)

build_binary_archive linux amd64 tar.gz
build_binary_archive linux arm64 tar.gz
build_binary_archive darwin arm64 tar.gz
build_binary_archive windows amd64 zip

printf '%s\n' "${artifacts[@]}" > release-artifacts.txt
sha256sum "${artifacts[@]}" > SHA256SUMS

printf 'Built %d release artifacts for %s\n' "${#artifacts[@]}" "${TAG}"
