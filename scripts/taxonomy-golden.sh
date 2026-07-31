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

# sha256 of a file, portable across Linux (sha256sum) and macOS (shasum).
sha() { if command -v sha256sum >/dev/null 2>&1; then sha256sum "$@"; else shasum -a 256 "$@"; fi; }

# manifest <dir> — one "sha256  ./relative/path" line per file, sorted, so two
# runs of the same output compare deterministically and diffs are readable.
#
# Atom feeds and the sitemap embed a build-time <updated>/<lastmod> (SSG sets a
# feed's <updated> to the latest page lastMod, which falls back to "now"), so
# those tags are normalised to a placeholder before hashing in .xml files —
# paths, entries and all other content are still compared, only the volatile
# timestamp is neutralised so the golden is reproducible.
manifest() {
  (cd "$1" && find . -type f | LC_ALL=C sort | while read -r f; do
    if [ "${f##*.}" = "xml" ]; then
      h="$(sed -E 's#<updated>[^<]*</updated>#<updated>NORM</updated>#g; s#<lastmod>[^<]*</lastmod>#<lastmod>NORM</lastmod>#g' "$f" | sha | cut -d' ' -f1)"
    else
      h="$(sha "$f" | cut -d' ' -f1)"
    fi
    echo "$h  $f"
  done)
}

# One build function per corpus — the exact CLI is explicit and reproducible.
# `corpus` is test/golden/corpus/ (a copy of test-content with tags/series/author
# added, so it exercises all four built-ins); the others are the shipped examples.
build_corpus()      { "$BIN" --source corpus --content-dir test/golden --template simple --templates-dir templates --domain ex.com --output-dir "$1" --feed >/dev/null; }
build_dynamic()     { "$BIN" --config examples/dynamic-taxonomies/ssg.yaml --output-dir "$1" >/dev/null; }
build_multilingual() { "$BIN" --config examples/multilingual-site/ssg.yaml --output-dir "$1" >/dev/null; }
build_external()    { "$BIN" --config examples/external-sources/ssg.yaml --output-dir "$1" >/dev/null; }

CORPORA="corpus dynamic multilingual external"

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
