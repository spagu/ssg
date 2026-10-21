---
title: "Breadcrumbs, Honest Sitemap Dates and a Build Report"
slug: "breadcrumbs-sitemap-dates-and-the-build-report"
status: publish
type: post
date: 2026-10-21
tags: [release, seo, sitemap, structured-data, external-sources, security]
excerpt: "What changed in ssg 1.8.73 and 1.8.74: breadcrumbs that only link to pages that exist, a sitemap checked against Google's rules, dates in it that are true, the ssg version in templates, and a Go security fix."
---

Two releases since the last post. Most of what changed came from two sites
built on ssg. One is a Polish consultancy whose Search Console filled up with
errors nobody could explain. The other lists every FreeBSD port, about 35,000
pages built from data rather than Markdown. Each one ran into something ssg
did wrong, and this post goes through them.

## Breadcrumbs that pointed at nothing

ssg writes a `BreadcrumbList` into every page's structured data. It built the
trail from the URL: one step per segment. A post at `/posts/my-article/` got
Home › Posts › My article, with Posts linking to `/posts/`.

On the consultancy site there was no page at `/posts/`. Its articles lived
under `/posts/` but the listing was at `/baza-wiedzy/`. So 197 of 215 pages
carried a breadcrumb link to a 404. Link checking did not catch it, because
nobody scans JSON-LD for links. The schema check passed, because the markup was
valid. Search Console was the only thing that noticed.

The labels were wrong too. "Home" was hard-coded in English, and the other
steps were the slug with capital letters, so the site showed "Uslugi" where it
meant "Usługi".

Since 1.8.73 a step only appears if the build actually writes a page at that
address: a page, a post listing, or a taxonomy, type or date archive. It takes
that page's title. If there is no page, the step is dropped and the positions
are renumbered. You can also name the steps yourself:

```yaml
breadcrumbs:
  home: "Strona główna"
  sections:
    posts: {name: "Baza wiedzy", url: /baza-wiedzy/}
```

`breadcrumbs: false` turns the list off for a theme that writes its own.

## The 404 page was in the sitemap

A page with the slug `404` becomes `/404.html`, which is what static hosts
serve for a missing URL. ssg also listed it in `sitemap.xml`. On Cloudflare
Pages, `/404.html` redirects to `/404`, which answers 200 with "page not
found". Search Console calls that a soft 404.

The error page is no longer listed, whatever its `robots:` says. Themes get
`.Page.IsNotFound`, so they can mark it `noindex` without checking the slug.

## The sitemap, against Google's rules

The 404 bug made us read Google's [Build and submit a
sitemap](https://developers.google.com/search/docs/crawling-indexing/sitemaps/build-sitemap)
page line by line against the code. Some things were fine: UTF-8, absolute
URLs, only canonical pages, a `Sitemap:` line in `robots.txt`. Four were not:

- **Escaping.** A `<loc>` went into the file as written. A canonical URL with
  `&` in its query string made the file invalid XML, and a crawler that finds
  invalid XML rejects the whole sitemap. Addresses are now escaped, and
  characters outside ASCII are percent-encoded, so `/usługi/` becomes
  `/us%C5%82ugi/`.
- **Size.** The limit is 50,000 URLs or 50 MB per file. ssg split on the count
  only. It now splits on both. If a site would need more files than one index
  can list (50,000), the build fails instead of writing an index nobody will
  read.
- **Hub pages had no date.** Every post had a `<lastmod>`. The pages that list
  posts did not: the front page, the listings, category and tag archives,
  taxonomy pages. On the ports site that was 3,800 of 37,000 URLs, and they are
  the ones a crawler should come back to most often. Each now takes the newest
  date among the posts it lists. Templates get the same date as `.Modified`.
- **The index used the build time** as `<lastmod>` for a sitemap file whose
  pages had no dates.

That last point leads to the bigger change.

## Dates nobody wrote

Google uses `<lastmod>` only if it is "consistently and verifiably accurate".
A date that changes on every build without the page changing is the opposite
of that.

ssg had two such dates. The index used the build time, as described above.
And a page with no `date` or `modified` in its front matter took its file's
modification time. That time is correct on the machine where someone edited
the file. In a fresh clone it is the time of the clone. So on a site built in
CI, every page without a date said it changed today, on every build.

1.8.74 does what Hugo and jekyll-sitemap do. A sitemap date comes from the
file's last git commit if `lastmod_from_git` is on, otherwise from `modified`,
otherwise from `date`. If nobody wrote a date, there is no `<lastmod>`. The
file time is still used to sort pages and in feeds, because Atom needs an
`<updated>` value. It just no longer goes into the sitemap.

If you want those pages dated, add a `date:` or turn on `lastmod_from_git`.

## Pages from data, with all their fields

The ports site builds one page per port from a JSON file, using
`content_map`. The map picks the title, the body and a few dates. The page is
also about everything else in the record: the version, the maintainer, the
dependencies, the package sizes. Before 1.8.73, a template could not see any
of that. Now the whole record is available, like custom front matter:

```gotemplate
<h1>{{ .Title }} {{ .Post.Extra.version }}</h1>
<p>Maintained by {{ .Post.Extra.maintainer }}</p>
```

Numbers had a problem of their own. JSON numbers reach Go as floats. Mapping a
Unix timestamp with `modified: "{{.distTimestamp}}"` produced
`1.761638001e+09`, which is not a date. About 15,000 pages lost their
`<lastmod>` and nothing said so. Whole numbers now print as whole numbers in
mapping templates, and a `modified` that cannot be parsed now gives a warning,
the way `date` already did.

## Which ssg built this page

The ports site wanted a footer line: built with ssg 1.8.73 on 21 October, 38,037
pages in 36 seconds. Before 1.8.74 that took a Makefile macro to time the
build, `ssg --version | awk` to get the version, `find` to count the pages,
and a script on every page to read the result.

Now templates have `.SSG.Version` next to `.BuildTime`:

```gotemplate
<meta name="generator" content="ssg {{ .SSG.Version }}">
```

The page count and the build time cannot be inside a page, because the page is
written before the build ends. `build_report: build.json` (or
`--build-report`) writes them to a small file next to the pages once the last
page is done, and before your `post_build` hooks run. It holds the version,
the start and end times, the duration, the number of pages and files, and the
number of documents of each type.

Something else turned up along the way. The site was built in a FreeBSD jail
with `go install github.com/spagu/ssg/cmd/ssg@v1.8.73`, and `ssg --version`
printed `dev`. Only release builds set the version. A `go install` build now
reads it from the binary, where Go records it anyway.

## 1.8.74 includes a Go security fix

`govulncheck` found GO-2026-6809 in Go 1.27.2: `crypto/x509` trusted
certificates from intermediates that should no longer be trusted. ssg reaches
that code through every TLS connection it opens, which includes remote
sources, deploys and fetching themes. 1.8.74 is built with Go 1.27.3, and
building ssg from source now needs it.

## Upgrading

Your pages do not change, but your output files do:

- `sitemap.xml` gains dates on hub pages. It loses dates that came only from a
  file's modification time, and the index loses its build-time dates.
- Breadcrumb steps that pointed at missing pages are gone.
- `sitemap.xml` no longer lists the 404 page.

[UPGRADING](/upgrading/) explains each of these, and the
[changelog](https://github.com/spagu/ssg/blob/main/CHANGELOG.md) lists
everything else.
