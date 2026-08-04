#!/usr/bin/env bash
# lib-corpora.sh — shared corpus definitions and output-manifest helpers.
#
# Sourced by scripts/taxonomy-golden.sh (does the output still match the recorded
# baseline?) and scripts/determinism.sh (does the output depend on how many
# workers built it?). Both ask a question about the same output trees, so the
# corpora, the build commands and the hashing live here once.

# sha256 of a file, portable across Linux (sha256sum) and macOS (shasum).
sha() { if command -v sha256sum >/dev/null 2>&1; then sha256sum "$@"; else shasum -a 256 "$@"; fi; }

# Some output embeds a build-time value that changes between runs and would make
# a comparison falsely red: Atom feeds and the sitemap carry <updated>/<lastmod>
# (SSG falls back to "now"), and the external-sources example renders the fetch
# date ("Products fetched YYYY-MM-DD"). These are normalised to a placeholder
# before hashing in .xml and .html files — paths, entries and all other content
# are still compared, only the volatile timestamps are neutralised.
NORMALIZE='s#<updated>[^<]*</updated>#<updated>NORM</updated>#g; s#<lastmod>[^<]*</lastmod>#<lastmod>NORM</lastmod>#g; s#fetched [0-9]{4}-[0-9]{2}-[0-9]{2}#fetched NORM#g'

# manifest <dir> — one "sha256  ./relative/path" line per file, sorted, so two
# runs of the same output compare deterministically and diffs are readable.
manifest() {
  (cd "$1" && find . -type f | LC_ALL=C sort | while read -r f; do
    case "${f##*.}" in
      xml|html)
        h="$(sed -E "$NORMALIZE" "$f" | sha | cut -d' ' -f1)"
        ;;
      *)
        h="$(sha "$f" | cut -d' ' -f1)"
        ;;
    esac
    echo "$h  $f"
  done)
}

# One build function per corpus — the exact CLI is explicit and reproducible.
# `corpus` is test/golden/corpus/ (a copy of test-content with tags/series/author
# and frontmatter aliases added, so it exercises all four built-ins plus the
# alias/_redirects path); the others are the shipped examples. Every function
# takes the output dir first and passes any further arguments straight to ssg,
# which is how the determinism check varies --workers.
build_corpus()       { out="$1"; shift; "$BIN" --source corpus --content-dir test/golden --template simple --templates-dir templates --domain ex.com --output-dir "$out" --feed "$@" >/dev/null; }
build_dynamic()      { out="$1"; shift; "$BIN" --config examples/dynamic-taxonomies/ssg.yaml --output-dir "$out" "$@" >/dev/null; }
build_multilingual() { out="$1"; shift; "$BIN" --config examples/multilingual-site/ssg.yaml --output-dir "$out" "$@" >/dev/null; }
build_external()     { out="$1"; shift; "$BIN" --config examples/external-sources/ssg.yaml --output-dir "$out" "$@" >/dev/null; }

CORPORA="corpus dynamic multilingual external"
