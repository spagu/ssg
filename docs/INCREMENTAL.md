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

## What it actually saves, and what it does not

On a 5 000-post corpus, editing one post, `--incremental` renders 268 pages
instead of 5 251. The wall clock moves less than that ratio suggests, and the
reason is worth knowing before you turn it on expecting more:

| Phase of an incremental build after one edit | Time | Share |
|---|---|---|
| Loading content | 800 ms | 59% |
| Generating site | 300 ms | 22% |
| Sitemap and robots | 240 ms | 17% |
| Everything else | ~30 ms | 2% |

Rendering pages is the only phase the graph narrows, and it is around a fifth
of the build. Loading content reads and parses every page whether or not that
page will be written, because an archive or a listing may show any page's body,
and no dependency graph can remove that.

So the honest summary at 5 000 posts: `--incremental` saves most of the render
phase and about a tenth overall. It is worth having in a watch loop, where the
alternative is that tenth on every save, and on sites large enough for the
render phase to grow past the reading.

Whether the load phase could be remembered instead of redone was measured under
[#270](https://github.com/spagu/ssg/issues/270). Most of it turned out not to
be conversion at all, and the answer was to stop doing unnecessary work rather
than to cache it — see `markdown_cache` in
[CONFIGURATION.md](CONFIGURATION.md), which is off by default and says why.

Measure your own site rather than trusting a number from another machine:

```bash
BENCH_INCREMENTAL=1 scripts/bench-build.sh 5000
ssg --incremental --profile=text        # where your build actually spends time
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

The graph holds pages, not aggregates. Listings, feeds and the sitemap are
computed from the whole site on every build and are never skipped, so they do
not appear in the count.

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

## Further reading

[What breaks if I change this](../blog/what-breaks-if-i-change-this.md) covers
the same feature from a maintainer's side: why the graph says "everything" so
often, the replaced-image bug it nearly shipped with, and how the two builds are
proven to agree.
