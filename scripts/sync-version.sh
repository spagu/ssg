#!/usr/bin/env bash
# sync-version.sh — propagate the single source of truth (./VERSION) into every
# packaging manifest so release channels never drift (audit DOC-005).
#
# Usage: scripts/sync-version.sh [--check]
#   (no args)  rewrite packaging files to match ./VERSION
#   --check    exit non-zero if any packaging file disagrees with ./VERSION
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION="$(tr -d '[:space:]' < "$ROOT/VERSION")"

if [[ -z "$VERSION" ]]; then
  echo "sync-version: ./VERSION is empty" >&2
  exit 1
fi

# Each entry: "file<TAB>sed-expression" applied in-place.
apply() {
  local file="$1" expr="$2"
  sed -i.bak -E "$expr" "$ROOT/$file"
  rm -f "$ROOT/$file.bak"
}

sync() {
  apply "packaging/freebsd/Makefile"     "s/^(DISTVERSION=[[:space:]]*).*/\1${VERSION}/"
  apply "packaging/openbsd/Makefile"     "s/^(V=[[:space:]]*).*/\1${VERSION}/"
  # The brew formula carries the version twice: the version field and the four
  # download URLs. Syncing only the field let ssg.rb claim 1.8.6 while every
  # URL still pointed at v1.7.13 (OPS-012). The sha256 lines cannot be synced
  # here — they are only knowable once the release is built, so
  # .github/workflows/homebrew.yml regenerates the published formula.
  apply "packaging/brew/ssg.rb"          "s/^([[:space:]]*version[[:space:]]+\").*(\")/\1${VERSION}\2/"
  apply "packaging/brew/ssg.rb"          "s|(releases/download/)v[^/]+/|\1v${VERSION}/|"
  apply "packaging/deb/control.template" "s/^(Version:[[:space:]]*).*/\1${VERSION}/"
  apply "packaging/rpm/ssg.spec"         "s/^(Version:[[:space:]]*).*/\1${VERSION}/"
  apply "install.sh"                     "s/^(VERSION=\"\\\$\{SSG_VERSION:-)[^}]*(\}\")/\1${VERSION}\2/"
}

check() {
  local rc=0
  grep -qE "^DISTVERSION=[[:space:]]*${VERSION}$"  "$ROOT/packaging/freebsd/Makefile"     || { echo "freebsd Makefile drift"; rc=1; }
  grep -qE "^V=[[:space:]]*${VERSION}$"             "$ROOT/packaging/openbsd/Makefile"     || { echo "openbsd Makefile drift"; rc=1; }
  grep -qE "version[[:space:]]+\"${VERSION}\""      "$ROOT/packaging/brew/ssg.rb"          || { echo "brew formula drift";   rc=1; }
  if grep -oE "releases/download/v[^/]+/" "$ROOT/packaging/brew/ssg.rb" |
     grep -qv "^releases/download/v${VERSION}/$"; then
    echo "brew formula URL drift"; rc=1
  fi
  grep -qE "^Version:[[:space:]]*${VERSION}$"       "$ROOT/packaging/deb/control.template" || { echo "deb control drift";    rc=1; }
  grep -qE "^Version:[[:space:]]*${VERSION}$"       "$ROOT/packaging/rpm/ssg.spec"         || { echo "rpm spec drift";       rc=1; }
  grep -qE "SSG_VERSION:-${VERSION}\}"              "$ROOT/install.sh"                     || { echo "install.sh drift";     rc=1; }
  # The snap deliberately has NO version to sync — it reads ./VERSION at build
  # time via adopt-info. Guard against a regression to a hardcoded version,
  # which is exactly how the store froze at 1.8.6 (OPS-013).
  if grep -qE "^version:[[:space:]]" "$ROOT/snap/snapcraft.yaml"; then
    echo "snapcraft.yaml hardcodes a version: use adopt-info + craftctl set version"; rc=1
  fi
  grep -q "adopt-info: ssg"     "$ROOT/snap/snapcraft.yaml" || { echo "snapcraft.yaml missing adopt-info"; rc=1; }
  grep -q "craftctl set version" "$ROOT/snap/snapcraft.yaml" || { echo "snapcraft.yaml missing craftctl set version"; rc=1; }
  return $rc
}

if [[ "${1:-}" == "--check" ]]; then
  if check; then echo "packaging version in sync: ${VERSION}"; else exit 1; fi
else
  sync
  echo "packaging synced to version ${VERSION}"
fi
