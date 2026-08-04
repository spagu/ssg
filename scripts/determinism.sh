#!/usr/bin/env bash
#
# determinism.sh — the same content must produce the same site, however many
# workers built it.
#
# Parallel rendering (BUILD-PARALLEL) promises byte-identical output regardless
# of --workers. That promise broke four times in ways no test noticed, because
# each break lived on a code path no corpus exercised: frontmatter aliases raced
# on a shared slice and both lost redirects and reordered _redirects, the
# link_rewrites prefix memo raced on its slice header, and alias stubs raced the
# output tree they were checking against. Every one of them was a scheduling
# artefact, so a single build always looked fine.
#
# This builds each corpus twice — once strictly sequential, once on a full worker
# pool — and compares the two output trees file by file, so shared render state
# that is missing a lock or an ordering shows up as a diff instead of as a bug
# report months later.
#
# It complements `go test -race` rather than replacing it, and the two catch
# different halves. Verified by re-introducing each fixed bug: dropping the
# _redirects sort and dropping the aliasRedirects mutex are both caught here (the
# output changes) but are invisible to -race in the shipped tests, because no
# corpus in the test suite drives them. Dropping the link_rewrites sync.Once is
# the reverse: -race reports it immediately, while the output usually survives —
# every worker computes the same list, so a torn read still lands on the right
# answer. Run both.
#
#   scripts/determinism.sh            # 1 worker vs 8
#   WORKERS=16 scripts/determinism.sh # compare against a different pool size
#   REPEAT=3 scripts/determinism.sh   # extra parallel builds, for flaky races
#
# Volatile timestamps are normalised exactly as the golden harness does, so a
# build straddling a second boundary is not reported as nondeterminism.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
BIN="$ROOT/build/ssg"
WORKERS="${WORKERS:-8}"
REPEAT="${REPEAT:-1}"

# shellcheck source=scripts/lib-corpora.sh
. "$ROOT/scripts/lib-corpora.sh"

echo "Building ssg…"
go build -o "$BIN" ./cmd/ssg

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
fail=0

# The shipped corpora are small — a handful of pages finish in an order the
# scheduler barely gets to vary, so on their own they missed every one of the
# races above. This generates a corpus built to vary: enough aliased posts that
# collection order actually differs run to run, link_rewrites so the prefix memo
# is exercised, and a theme that renders content through safeHTML so the whole
# markdown pipeline runs per page.
STRESS_POSTS="${STRESS_POSTS:-300}"
stress_dir="$tmp/stress-src"
generate_stress() {
  mkdir -p "$stress_dir/content/stress/posts" "$stress_dir/templates/stress"
  python3 - "$stress_dir" "$STRESS_POSTS" <<'PY'
import os, sys
root, n = sys.argv[1], int(sys.argv[2])
for i in range(n):
    open(f"{root}/content/stress/posts/post-{i:04d}.md", "w").write(f"""---
title: "Stress post {i}"
slug: "stress-{i:04d}"
date: "2026-01-{(i % 28) + 1:02d}T10:00:00Z"
status: publish
type: post
tags: [alpha, beta]
aliases:
  - "/old/stress-{i:04d}"
  - "/legacy/stress-{i:04d}"
---

Body of post {i} with a [legacy link](/legacy/docs/topic-{i % 7}) and an
[archive link](/legacy/notes/{i % 5}) so link_rewrites has work to do.

## Section

More prose for post {i}.
""")
open(f"{root}/.ssg.yaml", "w").write("""template: stress
domain: stress.example.com
templates_dir: templates
output_dir: out
paginate: 25
link_rewrites:
  /legacy/docs/: /docs/
  /legacy/: /archive/
content_sources:
  - path: content/stress/posts
    type: post
    category: Stress
""")
tpl = '<!doctype html><html><head><title>%s</title></head><body>%s</body></html>'
open(f"{root}/templates/stress/index.html", "w").write(
    tpl % ("Stress", "{{range .Posts}}<h2>{{.Title}}</h2>{{end}}"))
open(f"{root}/templates/stress/post.html", "w").write(
    tpl % ("{{.Post.Title}}", "{{.Post.Content | safeHTML}}"))
open(f"{root}/templates/stress/page.html", "w").write(
    tpl % ("{{.Page.Title}}", "{{.Page.Content | safeHTML}}"))
PY
}
generate_stress
build_stress() { out="$1"; shift; (cd "$stress_dir" && "$BIN" --config .ssg.yaml --output-dir "$out" "$@" >/dev/null); }
CORPORA="$CORPORA stress"

for name in $CORPORA; do
  # The sequential build is the reference: it is the ordering the generator had
  # before the worker pool existed.
  "build_$name" "$tmp/$name-seq" --workers=0
  manifest "$tmp/$name-seq" >"$tmp/$name-seq.manifest"
  count="$(wc -l <"$tmp/$name-seq.manifest" | tr -d ' ')"

  differs=0
  for run in $(seq 1 "$REPEAT"); do
    out="$tmp/$name-par-$run"
    "build_$name" "$out" --workers="$WORKERS"
    manifest "$out" >"$tmp/$name-par-$run.manifest"
    if ! diff -q "$tmp/$name-seq.manifest" "$tmp/$name-par-$run.manifest" >/dev/null; then
      if [ "$differs" = 0 ]; then
        echo "  ❌ $name: ${WORKERS}-worker output differs from the sequential build (run $run):"
        diff "$tmp/$name-seq.manifest" "$tmp/$name-par-$run.manifest" | sed 's/^/      /' | head -30
      fi
      differs=1
      fail=1
    fi
  done
  [ "$differs" = 0 ] && echo "  ✅ $name ($count files) — identical at 0 and ${WORKERS} workers"
done

if [ "$fail" = 0 ]; then
  echo "Deterministic: worker count does not affect the generated site."
else
  echo "NONDETERMINISM: the worker count changed the output — shared render state is missing a lock or an ordering (see diffs above)."
fi
exit "$fail"
