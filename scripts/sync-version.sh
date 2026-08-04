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

# SEMVER matches a version number in place, so a pattern replaces only a real
# version and never an adjacent value that merely lives under a similar key.
SEMVER='[0-9]+\.[0-9]+\.[0-9]+'

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
  # Beyond packaging: every other file that states the version. These were bumped
  # by hand each release, which is how man/ssg.1 drifted a release behind — the
  # same failure mode as the snap freeze below (OPS-013). Patterns are anchored to
  # their exact line so a bump can never wander into prose: a blog post saying
  # "SSG 1.8.16 makes that a one-liner" is a historical fact, not a version to
  # sync, and rewriting it would make the sentence false.
  apply "Dockerfile"                     "s/(-X main\.Version=)${SEMVER}/\1${VERSION}/"
  apply "Dockerfile"                     "s/^(LABEL org\.opencontainers\.image\.version=\")[^\"]*(\")/\1${VERSION}\2/"
  apply "docs/INSTALL.md"                "s/^(VERSION=)${SEMVER}$/\1${VERSION}/"
  apply "man/ssg.1"                      "s/^(\.TH SSG 1 \"[^\"]*\" \")[^\"]*(\")/\1${VERSION}\2/"
  # The docs site and the theme README both advertise the version in a variables
  # block. Requiring a semver keeps the pattern off neighbouring keys such as
  # docs-site.yaml's analytics `version: "1"`.
  apply "docs-site.yaml"                 "s/^([[:space:]]*version:[[:space:]]*\")${SEMVER}(\")/\1${VERSION}\2/"
  apply "templates/ssgtheme/README.md"   "s/^([[:space:]]*version:[[:space:]]*\")${SEMVER}(\")/\1${VERSION}\2/"
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
  grep -qE -- "-X main\.Version=${VERSION}\""       "$ROOT/Dockerfile"                     || { echo "Dockerfile build-arg drift"; rc=1; }
  grep -qE "^LABEL org\.opencontainers\.image\.version=\"${VERSION}\"$" "$ROOT/Dockerfile" || { echo "Dockerfile label drift"; rc=1; }
  grep -qE "^\.TH SSG 1 \"[^\"]*\" \"${VERSION}\""  "$ROOT/man/ssg.1"                      || { echo "man page drift";       rc=1; }
  # Every semver-shaped version: in these files must be the current one.
  for f in docs-site.yaml templates/ssgtheme/README.md docs/INSTALL.md; do
    if grep -oE "(version:[[:space:]]*\"|^VERSION=)${SEMVER}" "$ROOT/$f" |
       grep -oE "${SEMVER}\$" | grep -qv "^${VERSION}\$"; then
      echo "$f version drift"; rc=1
    fi
  done
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
