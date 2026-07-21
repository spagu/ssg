---
title: "SSG, and the case for boring websites"
slug: "what-ssg-is"
status: publish
type: post
date: 2026-07-18
tags: [introduction, static-sites, go]
excerpt: "A static site generator written in Go: what it actually does, who it is for, and the three things it refuses to do."
---

Most websites do not need a server. They need to answer a question that was
already known when the page was written — what does this product cost, what did
the release change, how do I install the thing — and then get out of the way.
SSG exists for that case. You write Markdown, run one command, and get a folder
of HTML you can put behind any CDN on earth.

That is the whole idea, and it is not new. Jekyll did it in 2008. What follows
is less about the category and more about the specific trade-offs this one
makes, because that is the part you cannot tell from a feature list.

## What a build actually looks like

```bash
ssg my-blog simple example.com --http --watch
```

Three positional arguments: where the content is, which theme renders it, and
what the canonical domain will be. The last one matters more than it looks —
it is what makes the sitemap, the feeds, the canonical tags and the Open Graph
URLs agree with each other instead of quietly disagreeing in three places.

Nothing else is required. No config file, no scaffolding step, no `node_modules`
that weighs more than the site. The `simple` and `krowy` themes are compiled
into the binary, so a fresh machine with nothing but `ssg` on it can build a
site.

Then the opt-ins start, and there are a lot of them: WebP conversion,
responsive image variants, SCSS, minification with source maps, content-hashed
asset fingerprinting, Atom feeds, a JSON search index, multilingual output with
`hreflang`, dynamic taxonomies, and native deploys to Cloudflare Pages, GitHub
Pages, Netlify, Vercel, FTP or SFTP. Every one of them is off until you ask.

That default matters. A generator that turns everything on gives you a build
you cannot explain, and eventually a bug you cannot locate.

## Where the content comes from

The obvious answer is Markdown files in a folder, and that is the common case.
Frontmatter carries the title, date, categories and the rest; a plain `.md` file
with no frontmatter is accepted too and takes its title from its first heading.

The less obvious answer is that content does not have to be *yours*, or local.
An SSG build can pull from a remote HTTP API, a read-only SQL query, or a
WordPress, Drupal or Movable Type database — the last one being how most
migrations start. All of it lands in one namespace your templates read from,
with one disk cache, one set of retry rules and one error model. The hardened
HTTP client is not a footnote: it refuses plain `http://` unless you say
otherwise, blocks private and loopback addresses *at dial time* — which is what
actually defeats DNS rebinding — caps response sizes, and keeps query strings
out of error messages so a token in a URL does not end up in your CI log.

Since 1.8.10 the local case got looser as well. A site no longer needs a
`content/<source>/` tree at all: `content_sources` points at any folders of
Markdown you already have, which is how this site is built from the
repository's own `docs/` directory. No copy step, no second source of truth
that drifts.

## The three refusals

**No client-side framework.** The output is HTML, CSS and whatever JavaScript
your theme chose to write. If a page needs interactivity, add it. The generator
will not add it for you and will not ship a runtime you did not ask for.

**No silent success.** This one is a matter of scars. A build that prints a
warning and exits 0 while a content block quietly disappeared is worse than a
build that fails, because the page still renders and looks fine. So
`--check-links=strict` fails on a dead internal link, `shortcode_errors: strict`
fails on a shortcode that could not render, and an unknown key in your config
file is reported by name rather than ignored.

**No magic version of your content.** Directories organise files; they do not
assign categories. Frontmatter does. A post's URL comes from its date and slug,
or from a permalink pattern you wrote, and never from a rule you have to
reverse-engineer.

## Who this is not for

If your site's content genuinely changes per request — a dashboard, a
marketplace, anything with a session — a static generator is the wrong shape
and no amount of build-time cleverness fixes that.

If you want a theme ecosystem with a thousand entries, Hugo is right there and
it is excellent. SSG ships three themes and a documented contract for writing
your own, which is a different bet: fewer options, less to keep compatible.

And if you are already happy with your current generator, the honest answer is
that switching buys you very little. The interesting cases are the ones where
you are fighting a plugin chain to do something a single Go binary should have
done — migrating 4,000 WordPress posts, or building one site from content that
lives in four different places.

## Where to start

`ssg --help` is exhaustive and grouped. Beyond that, the
[installation guide](/install/) covers Homebrew, Snap, Docker and the raw
binaries, and the [content guide](/content/) is the one to read second, because
almost every surprising build outcome traces back to a directory-contract rule
someone skipped.

The rest of this site is the reference. It is also, as the next post explains,
built by the tool it documents — which is either elegant or reckless, depending
on the day.
