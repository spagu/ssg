# Dynamic taxonomies

Taxonomies classify posts into browsable archives. `category`, `tag` and
`series` are built in and keep their historical URLs, templates and feeds; any
number of additional taxonomies can be declared in configuration. A working
project lives in [`examples/dynamic-taxonomies/`](../examples/dynamic-taxonomies/).

## Configuration

```yaml
taxonomies:
  technology:
    label: Technologies    # plural heading (default: title-cased name)
    singular: Technology   # singular heading (default: title-cased name)
    path: technology       # URL segment (default: the taxonomy name)
    field: technology      # frontmatter field read (default: the taxonomy name)
    multiple: true         # false = exactly one value per post
    archive: true          # emit /technology/ + /technology/<term>/
    feed: false            # true = Atom feed per term (/technology/go/feed.xml)
    sitemap: true          # include archives in sitemap.xml
    template: ""           # explicit index template (see fallback chain below)
    term_template: ""      # explicit term template
    sort: name             # term ordering: name | count | weight
    paginate: 0            # posts per term-archive page; 0 = use the global paginate
    case_sensitive: false  # "Go" and "go" merge into one term by default
    slugify: true          # URL slugs derived from term names
    generate_empty: false  # true = archives for zero-post terms from data files
```

`paginate` sets the page size for this taxonomy's term archives, overriding the
site-wide `paginate`; leave it `0` to inherit the global value. A site with many
tags but few categories can page each differently — `paginate: 50` on `tag`,
`paginate: 12` on `category`.

Taxonomy names must match `[a-z][a-z0-9_-]*`. Every taxonomy needs a unique
`path`, and the segments `author`, `page` and configured language codes are
reserved. Overriding `category`, `tag` or `series` adjusts their metadata
(labels, feed on/off for helpers), but their archives stay on the legacy
pipeline — custom `path`/`template` overrides for the built-ins are ignored
there (see Deferred).

## Assigning terms in frontmatter

Three sources are merged per taxonomy, in priority order:

```yaml
---
title: Cross-compiling Go and Rust
taxonomies:            # 1. the generic map (highest priority)
  technology: [Go, Rust]
  platform: [Linux]
technology: [Go]       # 2. the configured direct field
tags: [tutorial]       # 3. legacy fields (tags/category/categories/series)
---
```

Multi-value taxonomies merge and deduplicate across sources. A single-value
taxonomy (`multiple: false`) with two distinct values after deduplication fails
the build. Values assigned to `tag`/`series` through the generic map are synced
back onto the legacy fields, so the classic `/tag/…/` archives include them.

Term identity is normalized: surrounding/inner whitespace collapses and, unless
`case_sensitive: true`, comparison is Unicode-lowercased — `Go`, `go` and
` GO ` are one term whose display name is the first spelling seen. Two distinct
terms slugifying to the same URL (e.g. `C++` and `C--` → `c`) fail the build;
set an explicit `slug` in the term metadata to resolve it. Term and index URLs
are also validated against page, post and alias URLs — collisions fail the
build instead of overwriting output.

## Term metadata

`data/taxonomies/<taxonomy>.yaml` enriches terms (keys are normalized names):

```yaml
go:
  name: Go              # display-name override
  slug: golang          # slug override
  description: The Go programming language
  weight: 10            # used by sort: weight (descending)
  data:                 # free-form, exposed as .Data on the term
    color: "#00ADD8"
```

With `generate_empty: true`, metadata-only terms get archive pages even before
any post uses them.

## Templates

Archive pages pick the first template that exists in the theme:

| Page | Fallback chain |
|---|---|
| Taxonomy index (`/technology/`) | `template:` override → `taxonomy-<name>.html` → `taxonomy.html` → `archive.html` → `category.html` |
| Term archive (`/technology/go/`) | `term_template:` override → `taxonomy-<name>-term.html` → `taxonomy-term.html` → `archive.html` → `category.html` |

The index context provides `.Taxonomy` (`Name/Label/Singular/Path/URL`) and
`.Terms` (each `Name/Slug/URL/Description/Count/Weight/Data`). The term context
provides `.Taxonomy`, `.Term`, `.Posts` (newest first), `.Pager` and — for
compatibility with `category.html` — `.Category`, `.Kind` and `.Name`. With
`paginate` set, term archives paginate to `/technology/go/page/2/`.

### Template helpers

| Helper | Example | Result |
|---|---|---|
| `taxonomies` | `{{range taxonomies}}{{.Label}}{{end}}` | every definition, stable order |
| `taxonomy` | `{{with taxonomy "technology"}}{{.URL}}{{end}}` | one definition view |
| `taxonomyTerms` | `{{range taxonomyTerms "technology"}}…{{end}}` | sorted terms (current language) |
| `pageTerms` | `{{range pageTerms "technology" .Page}}…{{end}}` | a page's terms as full views |
| `termURL` | `{{termURL "technology" "Go"}}` | `/technology/go/` |
| `hasTerm` | `{{if hasTerm "technology" "Go" .Page}}` | normalized membership test |
| `pagesByTerm` | `{{range pagesByTerm "technology" "Go"}}…{{end}}` | the term's posts, newest first |

## Multilingual builds

With `i18n.enabled`, terms live in per-language buckets and custom archives are
emitted per language with the usual prefix rules: `/technology/…` for the
default language and `/en/technology/…` for others. Feeds, sitemap entries and
the helpers all follow the language of the page being rendered.

## Feeds, sitemap and search

- `feed: true` (plus global `feed: true`) writes an Atom feed per term.
- `sitemap: true` (default) adds the taxonomy index and each term archive.
- The search index and JSON output records carry a `taxonomies` map
  (`{"technology": ["Go", "Rust"], …}`) for client-side filtering.

## Deferred features

Not part of this release; tracked for later:

- Hierarchical taxonomies (nested terms with rollup counts).
- Term aliases/redirects and per-language translated term names.
- Custom `path`/`template` overrides for the built-in category/tag/series
  pipelines (their archives intentionally stay byte-for-byte legacy).
- Migrating the author archive onto the generic registry.
