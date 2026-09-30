---
title: "Most Websites Are Documents. Stop Shipping Them as Applications."
slug: "most-websites-are-documents"
status: publish
type: post
date: 2026-09-30
tags: [static-sites, performance, security, build]
excerpt: "A phone downloads 486 KB of this site's home page, and 24 KB of it is what the generator wrote. The rest is choices. The case for static sites, measured."
---

Here is the claim, stated early so you can disagree with it. Most websites
answer questions whose answers were known when the page was written, such as
what the product costs or how to install it. A page like that does not need a
process per request, a database behind it, or a JavaScript runtime in the
browser to draw text that was already text.

Most of those pages are shipped as if they did. That choice is paid for in
bytes, in build minutes, in code an attacker can reach, and in errors nobody
sees. All four can be measured, so this post measures them on this site, which
ssg builds from its own repository.

## Bytes: what the generator wrote, and what we added

The median mobile home page weighed 2,311 KB in October 2024, according to the
[HTTP Archive Web Almanac](https://almanac.httparchive.org/en/2024/page-weight).
JavaScript alone was 558 KB of that.

This is what a phone downloads for this site's home page:

<svg class="post-chart" role="img" aria-labelledby="weight-t weight-d" viewBox="0 0 480 366" width="100%" style="max-width:40rem;height:auto;display:block;margin:1.5rem 0" font-size="15" fill="currentColor" xmlns="http://www.w3.org/2000/svg">
<title id="weight-t">What a phone downloads for this site's home page, against the 2024 median</title>
<desc id="weight-d">Kilobytes on the wire. HTML, CSS and JS written by ssg and the theme, gzip: 24. Two logo images: 23. Hero photo, phone size, WebP: 164. Web fonts, four files: 96. Analytics tag: 179. Whole page on a phone: 486. Median mobile home page in 2024, HTTP Archive: 2,311.</desc>
<g><title>HTML, CSS and JS from ssg (gzip): 24 KB</title><text x="0" y="20">HTML, CSS and JS from ssg (gzip)</text><rect x="0" y="28" width="3.8" height="18" rx="3" style="fill:var(--color-fg-accent,#0050a6)"/><text x="11.8" y="42" font-weight="600">24 KB</text></g>
<g><title>Two logo images: 23 KB</title><text x="0" y="72">Two logo images</text><rect x="0" y="80" width="3.6" height="18" rx="3" style="fill:var(--color-fg-muted,#64748b)"/><text x="11.6" y="94" font-weight="600">23 KB</text></g>
<g><title>Hero photo, phone size, WebP: 164 KB</title><text x="0" y="124">Hero photo, phone size, WebP</text><rect x="0" y="132" width="25.6" height="18" rx="3" style="fill:var(--color-fg-muted,#64748b)"/><text x="33.6" y="146" font-weight="600">164 KB</text></g>
<g><title>Web fonts, four files: 96 KB</title><text x="0" y="176">Web fonts, four files</text><rect x="0" y="184" width="14.9" height="18" rx="3" style="fill:var(--color-fg-muted,#64748b)"/><text x="22.9" y="198" font-weight="600">96 KB</text></g>
<g><title>Analytics tag: 179 KB</title><text x="0" y="228">Analytics tag</text><rect x="0" y="236" width="27.9" height="18" rx="3" style="fill:var(--color-fg-muted,#64748b)"/><text x="35.9" y="250" font-weight="600">179 KB</text></g>
<g><title>Whole page on a phone: 486 KB</title><text x="0" y="280">Whole page on a phone</text><rect x="0" y="288" width="75.7" height="18" rx="3" style="fill:var(--color-fg-muted,#64748b)"/><text x="83.7" y="302" font-weight="600">486 KB</text></g>
<g><title>Median mobile home page, 2024: 2,311 KB</title><text x="0" y="332">Median mobile home page, 2024</text><rect x="0" y="340" width="360.0" height="18" rx="3" style="fill:var(--color-fg-muted,#64748b)"/><text x="368.0" y="354" font-weight="600">2,311 KB</text></g>
</svg>

| What | KB on the wire |
|---|---:|
| HTML, CSS and JS written by ssg and the theme (gzip) | 24 |
| Two logo images | 23 |
| Hero photo, phone size, WebP | 164 |
| Web fonts, four files | 96 |
| Analytics tag | 179 |
| **Whole page on a phone** | **486** |
| Median mobile home page, 2024 | 2,311 |

The median is not the uncomfortable line. Ours is. The largest file on this
home page is the analytics tag: 179 KB compressed, more than seven times
everything the generator wrote. We chose that tag. We also chose the fonts, and
they outweigh the markup four to one.

A static generator does not make a page light. It makes the weight visible,
because nothing arrives that somebody did not add on purpose. A client-side
framework rendering a page of prose works the other way round. Its runtime is
the floor, paid on every page before the first word appears, whether that page
has a button on it or not.

## Time: a build nobody waits for

A build that takes minutes changes how people work. They stop previewing, and
they merge a typo fix without looking, because looking costs a coffee.

`make bench` generates a synthetic blog with a fixed random seed and times a
full build. On a 16-core Ryzen 9 7950X under WSL2, with ssg built from this
commit, best of three runs:

<svg class="post-chart" role="img" aria-labelledby="build-t build-d" viewBox="0 0 480 210" width="100%" style="max-width:40rem;height:auto;display:block;margin:1.5rem 0" font-size="15" fill="currentColor" xmlns="http://www.w3.org/2000/svg">
<title id="build-t">Build time by corpus size, make bench, best of three</title>
<desc id="build-d">Seconds for a full build on a Ryzen 9 7950X. 100 posts, 106 pages: 0.07. 500 posts, 526 pages: 0.19. 2,000 posts, 2,101 pages: 0.62. 5,000 posts, 5,251 pages: 1.46.</desc>
<g><title>100 posts (106 pages): 0.07 s</title><text x="0" y="20">100 posts (106 pages)</text><rect x="0" y="28" width="17.3" height="18" rx="3" style="fill:var(--color-fg-accent,#0050a6)"/><text x="25.3" y="42" font-weight="600">0.07 s</text></g>
<g><title>500 posts (526 pages): 0.19 s</title><text x="0" y="72">500 posts (526 pages)</text><rect x="0" y="80" width="46.8" height="18" rx="3" style="fill:var(--color-fg-accent,#0050a6)"/><text x="54.8" y="94" font-weight="600">0.19 s</text></g>
<g><title>2,000 posts (2,101 pages): 0.62 s</title><text x="0" y="124">2,000 posts (2,101 pages)</text><rect x="0" y="132" width="152.9" height="18" rx="3" style="fill:var(--color-fg-accent,#0050a6)"/><text x="160.9" y="146" font-weight="600">0.62 s</text></g>
<g><title>5,000 posts (5,251 pages): 1.46 s</title><text x="0" y="176">5,000 posts (5,251 pages)</text><rect x="0" y="184" width="360.0" height="18" rx="3" style="fill:var(--color-fg-accent,#0050a6)"/><text x="368.0" y="198" font-weight="600">1.46 s</text></g>
</svg>

| Posts | Pages written | Build | Per page |
|---:|---:|---:|---:|
| 100 | 106 | 0.07 s | 0.66 ms |
| 500 | 526 | 0.19 s | 0.35 ms |
| 2,000 | 2,101 | 0.62 s | 0.30 ms |
| 5,000 | 5,251 | 1.46 s | 0.28 ms |

The cost per page falls as the site grows. The real site does more work per
page: WebP images, a feed, a search index, a sitemap, a Markdown copy of every
page for agents, and a strict check of every internal link. With this post it
has 112 HTML pages and builds in 1.61 s on the same machine.

A CMS on a database skips the build and pays at request time instead, on every
uncached request, for as long as the site exists. Caching fixes that. A
full-page cache in front of a CMS is a static site with a database attached to
it that the visitor never needed.

## Attack surface: nothing of ours runs when a page is read

A folder of HTML on a CDN has no process of ours that runs when a visitor asks
for a page. There is no login form to brute-force and no database to inject
into. Nobody has to patch a server that does not exist.

Two caveats, because they are true. This site does run code on requests: two
small functions, for comments and for cookie consent, both opt-in, both in the
repository's `workers/` directory where anyone can read them. And a static build
still has a supply chain: the generator and everything it pulls in. We learned
that on our own package.

<svg class="post-chart" role="img" aria-labelledby="ship-t ship-d" viewBox="0 0 480 158" width="100%" style="max-width:40rem;height:auto;display:block;margin:1.5rem 0" font-size="15" fill="currentColor" xmlns="http://www.w3.org/2000/svg">
<title id="ship-t">Download size of what ships, in megabytes</title>
<desc id="ship-d">Snap 1.8.62: 82.8 MB. Snap 1.8.63, after dropping the webp package: 23.3 MB. Linux amd64 tarball, 1.8.63: 13.9 MB.</desc>
<g><title>snap, 1.8.62: 82.8 MB</title><text x="0" y="20">snap, 1.8.62</text><rect x="0" y="28" width="360.0" height="18" rx="3" style="fill:var(--color-fg-muted,#64748b)"/><text x="368.0" y="42" font-weight="600">82.8 MB</text></g>
<g><title>snap, 1.8.63: 23.3 MB</title><text x="0" y="72">snap, 1.8.63</text><rect x="0" y="80" width="101.3" height="18" rx="3" style="fill:var(--color-fg-accent,#0050a6)"/><text x="109.3" y="94" font-weight="600">23.3 MB</text></g>
<g><title>Linux amd64 tarball, 1.8.63: 13.9 MB</title><text x="0" y="124">Linux amd64 tarball, 1.8.63</text><rect x="0" y="132" width="60.4" height="18" rx="3" style="fill:var(--color-fg-muted,#64748b)"/><text x="68.4" y="146" font-weight="600">13.9 MB</text></g>
</svg>

| Artifact | MB |
|---|---:|
| snap, 1.8.62 | 82.8 |
| snap, 1.8.63 | 23.3 |
| Linux amd64 tarball, 1.8.63 | 13.9 |

In 1.8.63 the snap went from 82.8 MB to 23.3 MB. The cause was cwebp. Ubuntu's
`webp` package ships an OpenGL image viewer, and that viewer pulled 50 packages
into the snap, Mesa, LLVM and libxml2 among them, for a tool that never opens a
window and never parses XML. The Snap Store then flagged libxml2 as outdated.
cwebp is now built from libwebp 1.6.0 with only JPEG and PNG input, and the snap
workflow fails if any of that stack comes back. The same release cleared the 12
findings gosec reported and made gosec a gate: a new finding now fails CI
instead of sitting in a report.

The general rule is plain. Every dependency is attack surface you did not write.
A plugin-driven CMS asks you to install dozens of them, run them on every
request and keep all of them patched. A static site asks you to trust yours
once, at build time, on a machine you control.

## Silence costs more than slowness

The last cost is the one nobody benchmarks. A slow tool wastes minutes, and you
can see it doing so. A tool that hides errors wastes days, and you hear about
it from a visitor.

1.8.62 fixed fourteen bugs of that kind, and every one of them had a green
build. Pages were dropped without a word because a description contained an
unquoted colon. English visitors got a Romanian 404 page. On a three-language
site, 21 of 27 sitemap URLs carried language alternates, and the six without
them were the front pages and blog listings, the URLs search engines weight
most. Nothing failed. All of it was wrong.

So ssg treats silence as a bug. `--check-links=strict` fails the build on a
dead internal link. `shortcode_errors: strict` fails it on a shortcode that
could not render. An unknown key in the config file is reported by name rather
than ignored. The workflow that publishes this site runs the strict link check
on every pull request, so this post could not have merged with a broken link
in it.

Move rendering into the browser and a whole class of those errors moves with it.
The build still passes. The failure happens on a visitor's phone, and the only
log you get is an email asking why the page is blank.

## When this is the wrong tool

If the page changes per visitor, it is an application. Build an application.
Dashboards, carts, anything behind a session: a static generator cannot help
there, and forcing it leads to a site rebuilt every minute by a cron job.

If your editors need a web form and will never open a Markdown file, that is a
real constraint. Solve it without putting a database on the path every reader
takes.

Documentation, blogs, product pages and release notes are documents. Ship them
as documents.

## Check the numbers

Every figure above comes from something you can run:

```bash
make bench                                 # build times, synthetic corpus
go build -o build/ssg ./cmd/ssg
./build/ssg --config docs-site.yaml --check-links=strict   # this site, 112 pages
```

Page weights are the built files in `.site/`: gzip level 9 for text, raw bytes
for images, the phone-size hero because that is the one the stylesheet serves
below 768 px. The fonts and the analytics tag were fetched from their hosts as
a browser would request them; the 6 KB font stylesheet is left out of the
total. The 23.3 MB is the Snap Store's download size for the current snap,
82.8 MB is what the 1.8.62 snap weighed before the fix, and the tarball size is
from the GitHub release. If your machine gives different build times, post
them in the comments. A laptop will be slower, and that is worth knowing too.
