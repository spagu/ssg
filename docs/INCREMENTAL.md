# Incremental builds

`ssg --incremental` rebuilds only the pages a change can reach. It is on by
default in `--watch`, where the alternative is a full build on every save.

Everything here rests on one rule: **an uncertain dependency means a full
build.** A graph that misses an edge produces a stale page and a green build,
which is a worse failure than a slow one. Every case below that ends in "the
whole site" is that rule being applied, not a limitation waiting to be fixed.

## Turning it on

```bash
ssg --watch                  # incremental, because a person is waiting
ssg --incremental            # a one-shot build that reuses the last one
ssg                          # a full build
```

A one-shot build is full unless asked, because a build nobody is waiting on
should be the simple one.

## What it costs on a real corpus

Measured with `BENCH_INCREMENTAL=1 scripts/bench-build.sh 5000` on a 5 000-post
corpus, editing one post between builds:

| Build | Wall time |
|---|---|
| Full | see `scripts/bench-build.sh` output |
| Incremental, one post edited | same command, second column |

Run it yourself rather than trusting a number from another machine:

```bash
BENCH_INCREMENTAL=1 scripts/bench-build.sh 5000
```

## What a change rebuilds

`ssg graph` answers that from the last build, without running another one.

```bash
ssg graph                                   # what the last build recorded
ssg graph content/site/posts/hello.md       # what changing that file rebuilds
ssg graph content/site/posts/hello.md --json
ssg graph --dot | dot -Tsvg > graph.svg
```

With no argument it prints the size of the graph, or the reasons this site's
builds cannot be narrowed. With a file it prints the outputs that file reaches,
or the reason a change to it is a full build.

## When a build is full anyway

| Situation | Why |
|---|---|
| No previous build, or a graph from an older `ssg` | Nothing to compare against |
| A template or a partial changed | The graph does not model which pages a partial reaches, so it will not guess |
| The configuration file changed | A setting can change anything |
| A file appeared that the last build never saw | A new page reaches archives, feeds, tag listings and the sitemap, and none of those edges exist yet |
| `--clean` | The output directory is emptied, so there is nothing left to keep |
| Content comes from MDDB | The graph cannot hash a remote collection |
| External sources are enabled | They can change without any file changing |
| A CMS import contributes pages | Those pages have no source file |

The last three are properties of the site, not of one build: `ssg graph` prints
them under "This site's builds cannot be narrowed", and `--incremental` saves
nothing until they change.

## What it is not

**It is not a partial site.** Every build loads all the content and computes
every aggregate — archives, taxonomies, the sitemap, the site graph — from the
full set. Incremental only skips *writing* pages whose bytes cannot have
changed. That is why an incremental build and a full one produce the same tree,
and why a test asserts exactly that over a random sequence of edits.

Assets beside a page count as that page's inputs, so replacing an image in
place rebuilds the page and refreshes the copy. A stale picture under a green
build is the same failure as a stale page.

**It is not a replacement for the watch loop's hash check.** The watcher still
skips a rebuild entirely when nothing changed by a byte. Incremental is what
happens after that check says something did.

## Where the graph is kept

`.ssg-cache/graph/graph.json`, beside the other build caches. It is a cache:
delete it and the next build is full. `ssg cache --namespace=graph --dry` lists
it; `.gitignore` already excludes `.ssg-cache/`.

The file records a schema version. A graph written by a different version of
`ssg` is discarded rather than read, which makes an upgrade cost one full build.
