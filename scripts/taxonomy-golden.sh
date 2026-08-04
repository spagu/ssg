#!/usr/bin/env bash
#
# Backward-compatibility golden harness for the taxonomy unification (#44).
#
# The refactor folds the four built-in taxonomies (category, tag, series, author)
# onto the dynamic taxonomy registry. Its contract is that, with no config
# overrides, the generated output stays IDENTICAL — same file paths, same bytes.
# This script pins that: it builds corpora that exercise the built-ins (including
# author, resolved through metadata.json users) plus the dynamic-taxonomy
# examples, and snapshots each output tree as a manifest of "sha256  path" lines.
#
#   scripts/taxonomy-golden.sh            # check the current build against the goldens
#   scripts/taxonomy-golden.sh --update   # regenerate the goldens (after an INTENDED change)
#
# The refactor must keep `--check` green. A diff means the output changed.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
BIN="$ROOT/build/ssg"
GOLDEN="$ROOT/test/golden/manifests"

UPDATE=0
[ "${1:-}" = "--update" ] && UPDATE=1

# Corpora, build commands and the output-manifest hashing are shared with
# scripts/determinism.sh — see scripts/lib-corpora.sh.
# shellcheck source=scripts/lib-corpora.sh
. "$ROOT/scripts/lib-corpora.sh"

echo "Building ssg…"
go build -o "$BIN" ./cmd/ssg
mkdir -p "$GOLDEN"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
fail=0

for name in $CORPORA; do
  out="$tmp/$name"
  "build_$name" "$out"
  cur="$tmp/$name.manifest"
  manifest "$out" >"$cur"
  gold="$GOLDEN/$name.txt"
  count="$(wc -l <"$cur" | tr -d ' ')"

  if [ "$UPDATE" = 1 ]; then
    cp "$cur" "$gold"
    echo "  updated $name ($count files)"
  elif [ ! -f "$gold" ]; then
    echo "  ❌ $name: no golden at $gold — run with --update first"
    fail=1
  elif diff -q "$gold" "$cur" >/dev/null; then
    echo "  ✅ $name ($count files) — identical"
  else
    echo "  ❌ $name: output differs from the golden:"
    diff "$gold" "$cur" | sed 's/^/      /' | head -40
    fail=1
  fi
done

if [ "$UPDATE" = 1 ]; then
  echo "Goldens updated in test/golden/manifests/. Commit them as the new baseline."
  exit 0
fi
if [ "$fail" = 0 ]; then
  echo "All corpora match — output is backward compatible."
else
  echo "REGRESSION: the build no longer matches the golden baseline (see diffs above)."
fi
exit "$fail"
