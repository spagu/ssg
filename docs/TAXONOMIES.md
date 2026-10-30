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
    outputs: [html]        # custom formats beside the index and every term (see below)
    outputs_index: []      # overrides outputs for the index only
    outputs_term: []       # overrides outputs for the term archives only
```

`paginate` sets the page size for this taxonomy's term archives, overriding the
site-wide `paginate`; leave it `0` to inherit the global value. A site with many
tags but few categories can page each differently — `paginate: 50` on `tag`,
`paginate: 12` on `category`.

Taxonomy names must match `[a-z][a-z0-9_-]*`. Every taxonomy needs a unique
`path`, and the segments `author`, `page` and configured language codes are
reserved. The built-in `category`, `tag`, `series` and `author` archives are all
driven by the single taxonomy registry, but render with byte-identical
legacy-compatible output. Overriding `category`, `tag` or `series` adjusts their
metadata (labels, feed on/off for helpers); custom `path`/`template` overrides
for the built-ins are ignored, since their output is held stable (see Deferred).

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

The index context provides `.Taxonomy` (`Name/Label/Singular/Path/URL`),
`.Terms` (each `Name/Slug/URL/Description/Count/Weight/Data`), `.Site`,
`.Lang`, `.Domain`, `.Vars`, `.Data`, `.ExternalData`, `.BuildTime`, `.SSG`,
`.Modified` (the newest lastmod of any post with a term) and, like every other
archive, `.CanonicalURL`. So that the `archive.html` → `category.html` end of
the chain renders on any theme, it also carries the archive-shaped fields
those templates read: `.Category` (`Name` = the taxonomy label, `Slug` = its
path), `.Kind` (the taxonomy name), `.Name` (the label), `.Posts` (always
empty — an index lists terms, not posts) and `.Pager` (a single page). Before
1.8.85 the index had none of them, and a theme without `taxonomy.html` failed
the build on `can't evaluate field Category` (#377). A category template still
shows an empty post list there, so a theme should ship a `taxonomy.html` (or
`taxonomy-<name>.html`) that ranges over `.Terms`; the bundled `simple` theme
ships `taxonomy.html` and `taxonomy-term.html`. The term context is the archive context described in
[TEMPLATES.md](TEMPLATES.md#category-tag-author-and-series-archives) —
`.Posts` (newest first), `.Pager`, `.Modified`, `.StructuredData` and, for
compatibility with `category.html`, `.Category`, `.Kind` and `.Name` — plus
`.Taxonomy` and `.Term`. With
`paginate` set, term archives paginate to `/technology/go/page/2/`, and
`.CanonicalURL` names the page currently being rendered.

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

Every helper above takes a **taxonomy name**. The examples use a custom one, so
it is worth stating the built-in names outright — they are `category`, `tag` and
`series`, **singular**, not the plural frontmatter field names:

```gotemplate
{{ termURL "category" "Marsaskala" }}   {{/* ✅ /category/marsaskala/ */}}
{{ termURL "categories" "Marsaskala" }} {{/* ❌ "" — no such taxonomy */}}
```

An unknown taxonomy name returns an empty string rather than an error, so the
mistake shows up as `href=""` rather than as a failed build.

**Author is not one of them.** The author archive is keyed on an author id
rather than a frontmatter field, so it is driven outside the registry and
`termURL "author"` returns `""`. Use `authorURL` instead — it takes the author id
a post carries, the post itself, an author record or a display name, and returns
`/author/<slug>/` (see
[docs/TEMPLATE_HELPERS.md](TEMPLATE_HELPERS.md)).

## Custom output formats

A format from `outputs_custom` can be written beside the taxonomy pages too,
the same way it is beside content pages (since 1.8.83, #367):

```yaml
outputs_custom:
  - name: api
    suffix: index.json
    mime: application/json
    template: formats/api.json
taxonomies:
  license:
    outputs: [html, api]   # /license/index.json and /license/<term>/index.json
```

`outputs` applies to the index and to every term; `outputs_index` or
`outputs_term` overrides it for one of the two. The default is HTML only.

The template gets the data the HTML view gets, as a map. The index has
`.Taxonomy` and `.Terms`; a term has `.Taxonomy`, `.Term` and `.Posts`; both
have `.Site`, `.Domain`, `.Vars`, `.Data` and the theme's helpers, including
`getExternal`. One template can serve both views:

```gotemplate
{{if .Term}}{"term": {{ toJSON .Term.Name }}, "count": {{ len .Posts }}}
{{else}}{"terms": [{{range $i, $t := .Terms}}{{if $i}},{{end}}{"name": {{ toJSON $t.Name }}, "count": {{ $t.Count }}}{{end}}]}{{end}}
```

The index and page 1 of each term announce every format that has a `mime` in
their `<head>`, as content pages do (since 1.8.84, #370):
`<link rel="alternate" type="application/json" href="/license/index.json">`.
A format the theme already links is not announced twice.

A paginated term writes its format once, beside page 1, and `.Posts` there is
every post of the term, not the first page: an API client wants the whole term.
Each language's taxonomy base gets its own file. Only custom formats apply —
`json`, `markdown` and `txt` describe a page's front matter and body, which a
taxonomy view does not have — so any other name is skipped with a warning. The
built-in `category`, `tag` and `series` keep their legacy output and take no
formats.

## Multilingual builds

With `i18n.enabled`, terms live in per-language buckets and custom archives are
emitted per language with the usual prefix rules: `/technology/…` for the
default language and `/en/technology/…` for others. Feeds, sitemap entries and
the helpers all follow the language of the page being rendered.

## Feeds, sitemap and search

- `feed: true` (plus global `feed: true`) writes an Atom feed per term.
- `sitemap: true` (default) adds the taxonomy index and each term archive.
  `sitemap: false` keeps them out **without** removing them: `archive` is the
  separate switch that decides whether they are written at all. It applies to the
  built-in `category` and `tag` as well as to custom taxonomies — before 1.8.58
  it parsed and validated on the built-ins and did nothing, so a site whose theme
  marked its tag archives `noindex` kept advertising them anyway. `author` is
  driven outside the registry and is not configurable this way.
- The search index and JSON output records carry a `taxonomies` map
  (`{"technology": ["Go", "Rust"], …}`) for client-side filtering.

## Deferred features

Not part of this release; tracked for later:

- Hierarchical taxonomies (nested terms with rollup counts).
- Term aliases/redirects and per-language translated term names.
- Custom `path`/`template` overrides for the built-in category/tag/series/author
  archives (they are folded onto the registry but keep byte-for-byte legacy
  output, so per-built-in `path`/`template` overrides don't apply).
