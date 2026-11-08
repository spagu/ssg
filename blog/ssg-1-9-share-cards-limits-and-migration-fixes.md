---
title: "ssg 1.9: Share Cards, a Ceiling on Memory and Three Migration Fixes"
slug: "ssg-1-9-share-cards-limits-and-migration-fixes"
status: publish
type: post
date: 2026-11-08
tags: [release, seo, sitemap, images, performance, security]
excerpt: "ssg 1.9.0 draws a share card for every page, keeps a build inside a CPU and memory limit, splits a generated site's sitemap by source, and fixes three ways a migrated site could quietly ship the wrong thing."
---

ssg 1.9.0 is out. It is the first release under the Business Source License.
Using ssg stays free, and a commercial site keeps a one-line "Built with ssg"
notice. [A separate post](/blog/why-ssg-moved-to-the-business-source-license/)
explains what changed and why. This one covers the rest.

Some of it was asked for. Some of it went wrong on real sites and is now
fixed.

## A share card for every page

A link pasted into a chat or a social feed shows a picture if the page names
one in `og:image`. Most pages do not have a featured image, so their links
show nothing, or the site logo on every link.

`og_cards` draws one at build time. Each card is a 1200×630 PNG with the
page's title, its category and the site name. It is drawn in Go, so no
browser, no headless Chrome and no outside service is needed:

```yaml
og_cards:
  enabled: true
  text_color: "#1b1b1f"
  background: "#f4efe6"
  accent_color: "#b3261e"
```

It is off by default. The colours are required, because a card in someone
else's colours is not your site's card. If the text is hard to read against
the background, the build warns. Cards are cached, and the file name changes
when the title does, so the platforms fetch the new card instead of showing a
stale one. The WebP pass leaves the cards alone, because not every preview
service renders WebP.

## A build that stays inside its limits

A large site on a small machine could take every core and more memory than
the box has. On a 2 GB VPS, or in a container with a memory limit, that ends
with the kernel killing the build halfway through.

`resources:` sets a limit for every ssg command, not only the build:

```yaml
resources:
  cpu: 2          # cores; every worker pool and image encoder fits inside
  memory: 1.5GiB  # a soft limit: the garbage collector works harder near it
```

In a container, ssg now reads the container's own memory limit (cgroup v2) and
stays at 90% of it, even if you set nothing. `memory: none` turns that off.
The limits are printed once at startup, and `--profile` now reports the peak
memory the build used, so you can see how close it came.

`mem_limit` from earlier versions still works and means the same thing. It
used to apply only to the preview server. Now it applies everywhere.

## A sitemap per section of a generated site

One site built on ssg lists software ports from several systems: about 37,000
pages generated from data rather than written in Markdown. Its sitemap was one
long list. Search Console reports indexing per sitemap file, so the owner
could not see whether the OpenBSD pages were indexed differently from the
FreeBSD ones.

A sitemap entry can now pick pages by the data source they came from, or by
where their address starts:

```yaml
sitemaps:
  - path: /sitemap-openbsd.xml
    prefix: /openbsd/
  - path: /sitemap-ports.xml
    source: ports   # an external_sources entry, minus what /openbsd/ took
```

A prefix matches whole path segments, so `/net/` does not catch `/netbsd/`.
Feeds take the same two keys. Each page keeps its own `<lastmod>`, and the
sitemap index dates each file by its newest page. A `source:` that names
nothing now stops the build instead of writing an empty file.

## Three ways a migrated site went wrong

All three came from sites moved over from WordPress. None of them printed an
error.

**The home page went noindex.** A WordPress export keeps the old static front
page with `link: "https://example.com/"`, often still marked `robots:
noindex` from when the site stopped using it. Since a page at `/` replaces the
theme's home page, the upgraded site served that leftover as its home page,
noindexed, with another title. Now the build stops, names the file, and says
what to do. Remove its `link:` to keep the theme's home page, or remove the
`noindex` if it really is the front page.

**The iPhone icon broke.** With `webp: true`, the image pass rewrote
`<link rel="apple-touch-icon">` to point at a `.webp`. iOS only accepts a PNG
there, and by default the PNG had been deleted as well. Icons are now left
alone: touch icons, favicons, Safari's mask icon and the icons in a web app
manifest.

**`llms.txt` listed pages the site was hiding.** With `markdown_publish`, ssg
writes a Markdown copy of every page and lists them in `llms.txt` for AI
agents. That list included pages marked `noindex`, pages kept out of the
sitemap, and the 404 page. So agents were sent exactly where search engines
were told not to go. Those pages now get no Markdown copy and no line in
`llms.txt`.

## Security

- **MDDB content is sanitized by default.** A build from an MDDB server runs
  the content through the HTML sanitizer, so a compromised or shared server
  cannot put a `<script>` into your pages. If the server holds only your own
  content, `mddb.trusted: true` restores the raw HTML.
- **Online themes can be pinned and checked.** `online_theme` takes `repo@ref`
  to fetch a fixed tag or commit, and `online_theme_sha256` refuses an archive
  that is not the one you expect.
- **The GitHub Action checks the ssg binary** against the release checksums
  before running it, as the install script already did.

## Upgrading

Most sites need nothing. Six things can change a build or its output:

- Pages get the "Built with ssg" notice, unless the theme has its own.
- A `noindex` page at `/` stops the build.
- A sitemap `source:` that names nothing stops the build.
- An invalid `mem_limit` stops the command. It used to be a warning.
- Hidden pages lose their Markdown copies.
- MDDB content is sanitized unless the server is marked trusted.

[UPGRADING](/upgrading/) explains each one, and the
[changelog](https://github.com/spagu/ssg/blob/main/CHANGELOG.md) lists
everything else.
