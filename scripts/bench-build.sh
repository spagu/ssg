#!/usr/bin/env bash
# bench-build.sh — measure build throughput on a synthetic corpus (PERF-012/013).
#
# Publishing speed is only meaningful at scale: a 20-page site finishes before
# you can measure it, and a bottleneck that is quadratic in the number of posts
# stays invisible until a corpus is big enough to expose it. This generates
# realistic posts (frontmatter, prose, headings, a code fence, tags), builds them
# and reports wall time per page.
#
# Usage:
#   scripts/bench-build.sh                 # sizes 100 500 2000, 3 runs each
#   scripts/bench-build.sh 500 5000        # explicit sizes
#   SSG_BIN=/path/to/ssg scripts/bench-build.sh   # benchmark another binary
#   BENCH_RUNS=5 scripts/bench-build.sh    # more repetitions (best is reported)
#
# The corpus lives under a temp dir and is reused across runs, so repeated
# invocations only pay for generation once per size.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SIZES=("$@")
[[ ${#SIZES[@]} -eq 0 ]] && SIZES=(100 500 2000)
RUNS="${BENCH_RUNS:-3}"
WORK="${BENCH_DIR:-${TMPDIR:-/tmp}/ssg-bench}"

SSG_BIN="${SSG_BIN:-}"
if [[ -z "$SSG_BIN" ]]; then
  SSG_BIN="$WORK/ssg"
  mkdir -p "$WORK"
  echo "building ssg from source..."
  (cd "$ROOT" && go build -o "$SSG_BIN" ./cmd/ssg)
fi

# generate <dir> <n> — write an n-post corpus, skipped when already present.
generate() {
  local dir="$1" n="$2"
  [[ -f "$dir/.ssg.yaml" ]] && return 0
  mkdir -p "$dir/content/bench/posts" "$dir/templates/minimal"
  python3 - "$dir" "$n" <<'PY'
import os, random, sys
dir_, n = sys.argv[1], int(sys.argv[2])
random.seed(42)  # a fixed corpus keeps runs comparable across machines
words = ("publishing static site generator build cache render markdown template "
         "pipeline latency throughput incremental parallel worker pool hash "
         "content addressed deterministic output").split()
def para(k):
    return "\n\n".join(
        " ".join(random.choice(words) for _ in range(60)).capitalize() + "."
        for _ in range(k))
for i in range(n):
    tags = random.sample(["go", "perf", "cache", "build", "web", "ssg", "io", "cpu"], 3)
    open(f"{dir_}/content/bench/posts/post-{i:05d}.md", "w").write(f"""---
title: "Post number {i}"
slug: "post-{i:05d}"
date: "2026-0{(i % 9) + 1}-{(i % 28) + 1:02d}T10:00:00Z"
status: publish
type: post
tags: [{", ".join(tags)}]
excerpt: "Synthetic post {i} for benchmarking."
---

# Post number {i}

{para(12)}

## Section

{para(8)}

```go
func handler{i}(w http.ResponseWriter, r *http.Request) {{
    fmt.Fprintln(w, "post {i}")
}}
```

{para(6)}
""")
open(f"{dir_}/.ssg.yaml", "w").write("""template: minimal
domain: bench.example.com
templates_dir: templates
output_dir: out
paginate: 20
content_sources:
  - path: content/bench/posts
    type: post
    category: Bench
""")
tpl = '<!doctype html><html><head><title>%s</title></head><body>%s</body></html>'
open(f"{dir_}/templates/minimal/index.html", "w").write(
    tpl % ("Bench", "{{range .Posts}}<h2>{{.Title}}</h2>{{end}}"))
for name in ("post.html", "page.html"):
    open(f"{dir_}/templates/minimal/{name}", "w").write(tpl % ("{{.Title}}", "{{.Content}}"))
PY
}

printf '%-8s %-10s %-8s %-12s\n' "posts" "best" "pages" "per page"
printf '%-8s %-10s %-8s %-12s\n' "-----" "----" "-----" "--------"

for n in "${SIZES[@]}"; do
  dir="$WORK/corpus-$n"
  generate "$dir" "$n"
  best=""
  for _ in $(seq 1 "$RUNS"); do
    rm -rf "$dir/out"
    # Time the build itself; the corpus is already warm in the page cache, so
    # this measures the generator rather than the first read of the disk.
    start=$(python3 -c 'import time; print(time.perf_counter())')
    (cd "$dir" && "$SSG_BIN" --config .ssg.yaml --quiet >/dev/null 2>&1) || true
    end=$(python3 -c 'import time; print(time.perf_counter())')
    best=$(python3 -c "
b = '$best'; t = $end - $start
print(min(float(b), t) if b else t)")
  done
  pages=$(find "$dir/out" -name '*.html' 2>/dev/null | wc -l | tr -d ' ')
  python3 -c "
n, b, p = $n, float('$best'), $pages
print(f'{n:<8} {b:<10.2f} {p:<8} {1000*b/max(p,1):.2f} ms')"
done
