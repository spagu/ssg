# Content guide

This document is the canonical reference for local Markdown content in SSG. For
configuration keys, see [CONFIGURATION.md](CONFIGURATION.md). For values exposed
to themes, see [TEMPLATES.md](TEMPLATES.md).

## Content source

A normal build selects one source directory:

```bash
ssg my-blog simple example.com
```

With the default paths, `my-blog` resolves to `content/my-blog/`. The source may
also come from `source:` in a configuration file. MDDB is an alternative remote
source and is documented in [CONFIGURATION.md](CONFIGURATION.md#mddb-content).

### Extra sources (`content_sources`)

Content that already lives elsewhere — a `docs/` folder, notes beside the code,
a package's guides in a monorepo — does not have to be copied into the source
tree. `content_sources` lists additional **flat Markdown roots** (loaded
recursively) that are merged into the site:

```yaml
content_sources:
  - path: docs                  # relative to the working directory, or absolute
    type: page                  # page (default) | post
    category: Documentation     # optional; created if metadata.json lacks it
  - path: ../shared/notes
    type: post
```

- Extra sources join the site **before** finalize, so they get the same URL,
  permalink, i18n, taxonomy and collision treatment as native content.
- `category` applies only to files whose own frontmatter names no category.
- A directory that does not exist, is a file, or has an unsupported `type`
  fails the build with a message naming the path; an empty directory warns.
- With at least one extra source, the primary `source` becomes **optional** —
  a site can consist of extra sources alone, and then no `metadata.json` is
  required either.
- Watch mode watches these directories too.

CLI: `--content-source=DIR`, repeatable, path-only (`type`/`category` need the
config file):

```bash
ssg --content-source=docs ssgtheme example.com --watch
```

An unset `source` with no `content_sources` is a build error that names the
missing settings — and the config loader warns about unknown keys, so a config
written for a newer ssg does not fail silently.

## Directory contract

```text
content/
└── my-blog/
    ├── metadata.json
    ├── pages/
    │   ├── about.md
    │   └── legal/
    │       └── privacy.md
    ├── posts/
    │   ├── general/
    │   │   └── hello.md
    │   └── 2026/
    │       └── 07/
    │           └── release.md
    └── media/
        └── hero.jpg
```

The loader follows these rules:

1. `metadata.json` is required for a local source. Unknown JSON fields are
   ignored, which allows direct use of larger export metadata files.
2. Pages are loaded recursively from `pages/`.
3. Posts must be below at least one directory inside `posts/`. A Markdown file
   placed directly in `posts/` is ignored.
4. After that first grouping directory, post directories are recursive.
5. Directories organise files only. Post categories come from frontmatter.
6. A supported non-Markdown file beside a page or post is a co-located asset.
   It is copied when that content references its filename (only files directly beside
   the Markdown file are supported; nested subdirectories next to content files are skipped).

The `pages/` and `posts/` names can be changed with `pages_path` and
`posts_path`. The source root can be changed with `content_dir`.

## metadata.json

The metadata file supplies categories, authors and exported media records:

```json
{
  "categories": [
    {
      "id": 1,
      "count": 3,
      "description": "General articles",
      "link": "/category/general/",
      "name": "General",
      "slug": "general",
      "parent": 0
    }
  ],
  "users": [
    { "id": 1, "name": "Ada Lovelace", "slug": "ada" }
  ],
  "media": []
}
```

Only the following shapes are consumed:

| Collection | Recognised fields |
|---|---|
| `categories` | `id`, `count`, `description`, `link`, `name`, `slug`, `parent` |
| `users` | `id`, `name`, `slug` |
| `tags` | `id`, `name`, `slug` — resolves numeric ids in `tags:` frontmatter and supplies canonical archive slugs (v1.8.6) |
| `media` | `id`, `slug`, `title.rendered`, `media_type`, `mime_type`, `source_url`, `media_details.width`, `media_details.height`, `media_details.file` |

Additional export fields are allowed but are not exposed as site metadata.

## Author archives

Every author referenced by at least one **post** gets an automatic archive at
`/author/<slug>/`:

- The `author` frontmatter field accepts an ID (`1`), a name (`Ada Lovelace`)
  or a slug (`ada`); all three resolve through the `users` block above. An ID
  with no `users` entry falls back to `author-<id>`.
- Archives list **posts only** — pages never join an author archive, even with
  an `author` field.
- Templates: `author.html` renders the archive (context: `.Name`, `.Posts`
  newest-first, `.Kind` = `author`); missing `author.html` falls back to
  `category.html`. Archives appear in the sitemap.
- The author archive stays on this fixed pipeline; it is not configurable via
  `taxonomies:` (migrating it onto the generic registry is a documented
  deferred item), and `author` is a reserved path custom taxonomies cannot
  claim.

**Explicit content wins (v1.8.5).** If a page, post or alias already owns an
archive URL — say a hand-written profile with `link: /author/ada/` — the
auto-generated archive for that URL is skipped with a build warning instead of
silently overwriting your page, and the suppressed archive stays out of the
sitemap and feeds. The same rule protects `/category/…`, `/tag/…` and
`/series/…` URLs.

## Markdown and frontmatter

For predictable output, authored content should use YAML frontmatter:

```markdown
---
title: Understanding WebP
slug: understanding-webp
status: publish
type: post
date: 2026-07-14
modified: 2026-07-15
categories: [Guides]
author: ada
tags: [images, performance]
excerpt: Why WebP can reduce image size.
featured_image: /media/webp-hero.jpg
---

The full Markdown article starts here.
```

Files with frontmatter are included only when `status` is exactly `publish`.
Any other value, including an omitted status, is treated as a draft.

A plain `.md` file without frontmatter is accepted and treated as published.
Frontmatter is still recommended for anything other than imported plain
content, but two values are inferred so such a file is not blank everywhere it
is listed:

| Value | Inferred from | When |
|---|---|---|
| `title` | the document's first `# heading` (or a Setext `Title` / `====`) | always, when frontmatter has no `title` |
| `excerpt` | the opening paragraph, capped at 200 characters on a word boundary | only with `auto_excerpt: true` |

The title fallback is unconditional because an untitled page is broken in every
listing, menu and `<title>`. The excerpt fallback is opt-in because it changes
card text, feed summaries and meta descriptions on an existing site. Derivation
skips headings, fenced code, tables, block quotes, images, list markers and
Liquid guards (`{%` … `%}`), so the excerpt starts at the first real sentence.

### Frontmatter fields

| Field | Type | Applies to | Behaviour |
|---|---|---|---|
| `id` | integer | both | Optional source identifier |
| `title` | string | both | Display title |
| `slug` | string | both | URL segment; defaults to the source filename |
| `status` | string | both | Only `publish` is rendered when frontmatter exists |
| `type` | string | both | Use `page` or `post`; affects URL and template behaviour |
| `date` | date | post | Publication date and default date-based URL |
| `modified` | date | both | Last modification date |
| `link` | string | both | Explicit URL path; highest URL precedence |
| `author` | integer/string | post | Author ID, numeric string, name or slug |
| `categories` | list | post | Category IDs, numeric strings, names or slugs |
| `category` | string | post | Single free-form category value exposed to templates |
| `tags` | list | post | Creates tag listings at `/tag/<slug>/` |
| `series` | string | post | Creates a series listing and previous/next navigation |
| `excerpt` | string | both | Listing, feed and metadata summary |
| `description` | string | both | SEO description; themes may fall back to `excerpt` |
| `keywords` | string | both | SEO keywords |
| `canonical` | string | both | Explicit canonical value exposed to templates |
| `aliases` | list | both | Old paths rendered as redirect stubs |
| `featured_image` | string | both | Hero/Open Graph image |
| `layout` | string | page | Theme layout; `redirect` also marks an item for sitemap exclusion |
| `template` | string | page | Specific page template filename override |
| `robots` | string | both | Robots directive; `noindex` excludes from sitemap |
| `sitemap` | string | both | `no` excludes the item from `sitemap.xml` |
| `lang` | string | both | Language used in multilingual output |

Unknown frontmatter fields are retained and flattened into the template's root
context. They never replace standard fields with the same name.

### Author and category resolution

IDs, names and slugs are supported:

```yaml
author: 1
categories: [1, 5]
```

```yaml
author: Ada Lovelace
categories: [Guides, Performance]
```

```yaml
author: ada
categories: [guides, performance]
```

Name and slug matching is case-insensitive. Numeric strings are converted to
IDs. Values that cannot be resolved through `metadata.json` are ignored.

## Excerpts and section markers

SSG supports WordPress-export-style section markers:

```markdown
## Excerpt
A short summary.

## Content
The full article.
```

The markers must match exactly and are removed from rendered content. Without
them, everything after frontmatter becomes content. Markers inside fenced code
blocks are treated as code. A top-level `# Title` line in content is discarded
as an export artifact; use the frontmatter `title` for the document title.

## Slugs and URLs

When `slug` is absent, it is derived from the filename without `.md` and
lowercased:

```text
API.md → api → /api/
```

Set `preserve_slug_case: true` to preserve filename or explicit slug casing.

Default URLs are:

| Content | Default URL |
|---|---|
| Page | `/<slug>/` |
| Post | `/<year>/<month>/<day>/<slug>/` |

URL precedence, from highest to lowest, is:

1. Frontmatter `link`
2. Configured `permalinks.post` or `permalinks.page`
3. `post_url_format` for posts
4. Default URL

Permalink patterns support `:year`, `:month`, `:day`, `:slug` and `:category`:

```yaml
permalinks:
  post: /:year/:month/:slug/
  page: /:slug/
```

Expanded paths are sanitised so they cannot escape the output directory.

`page_format` controls filesystem form:

| Value | Result for `about` |
|---|---|
| `directory` | `about/index.html` (default) |
| `flat` | `about.html` |
| `both` | Both forms |

## Aliases

Use aliases to preserve old inbound URLs:

```yaml
aliases:
  - /old/permalink/
  - /2019/legacy-path/
```

Each safe, non-conflicting alias becomes a `noindex` HTML redirect stub with a
canonical link. Aliases are excluded from the sitemap. A collision with real
content is skipped with a warning.

## Relative Markdown links

With `rewrite_md_links: true`, links to source Markdown files are rewritten to
their generated URLs:

```markdown
See [Authentication](AUTHENTICATION.md) and [API](../reference/API.md).
```

SSG resolves the source filename and final slug. Unknown `.md` links remain
unchanged. The feature is disabled by default for sites that intentionally
publish raw Markdown.

## Assets and static files

- Referenced images, media, archives and documents beside a Markdown file are
  copied as co-located assets (only files directly beside the Markdown source are supported;
  nested subdirectories next to content files are skipped). Unreferenced siblings are skipped.
- `content/<source>/media/` contains source media records/files.
- Project-level `static/` is copied recursively and verbatim to the output.
- Theme CSS, JavaScript and images are copied from the selected template.

Use `static/` for favicons, downloads, manifests and files SSG should not parse.
See [IMAGES.md](IMAGES.md) for generated image variants and template image
helpers.

## Data-driven content features

### Tags and series

`tags` generates `/tag/<slug>/` listings. `series` generates
`/series/<slug>/` and exposes previous/next series values to templates. A theme
may provide `series.html`; otherwise the category template is used.

### Multilingual content

When `languages` is configured, the default language stays at the site root and
other languages are emitted under `/<lang>/`. Set `lang` on each item to select
its language. Templates receive language, translation and `hreflang` context;
see [TEMPLATES.md](TEMPLATES.md).

### Dates

`timezone` and `language_timezones` affect content dates used by templates and
date permalink tokens. Feeds and sitemap timestamps remain UTC. With
`lastmod_from_git`, sitemap modification dates come from the source file's last
Git commit and fall back to frontmatter/file dates when unavailable.

## WordPress-compatible media shortcodes

When migrating content from a WordPress site, the content might contain legacy media shortcodes. SSG has native, built-in support for parsing and sanitizing the following bracket shortcodes:

* **`[youtube]VIDEO_ID[/youtube]`** — Renders the standard YouTube video embed responsive iframe.
* **`[embed]VIDEO_URL[/embed]`** — Renders the responsive video player iframe.

These shortcodes are automatically protected from the HTML sanitizer (`sanitize_html: true`) via a secure token bypass mechanism, preventing the iframe elements from being stripped out during the build phase while still neutralising other malicious HTML.

To strip these shortcodes from your text in lists or feeds, use the `stripShortcodes` template helper. To extract the YouTube thumbnail image from a post's content, use the `thumbnailFromYoutube` helper.

## Publication checklist

- `metadata.json` exists and contains every referenced author/category.
- Posts are not placed directly in `posts/`.
- Frontmatter content has `status: publish`.
- `type` is correct and posts have a valid `date`.
- Explicit `link` and aliases do not collide.
- `ssg ... --check-links=strict` succeeds before deployment.
