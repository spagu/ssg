# Templates and themes

This guide describes theme layout, rendering contexts and supported engines.
Go template helper signatures are in [TEMPLATE_HELPERS.md](TEMPLATE_HELPERS.md);
image functions are in [IMAGES.md](IMAGES.md).

## Selecting a theme

The second positional argument names a directory below `templates_dir`:

```bash
ssg my-blog my-theme example.com
```

With default paths, the theme is `templates/my-theme/`. The built-in `simple`
and `krowy` themes are embedded and scaffolded when first used.

| Bundled theme | For |
|---|---|
| `simple` | a minimal blog; scaffolded into an empty theme directory |
| `krowy` | the fuller blog layout used by the examples |
| `ssgtheme` | documentation sites: cards, guide layout, colour-scheme switch, shared chrome in `partials/` ([README](../templates/ssgtheme/README.md)) |

An online theme can be downloaded before generation:

```bash
ssg my-blog bearblog example.com \
  --online-theme=https://github.com/janraasch/hugo-bearblog
```

GitHub, GitLab and direct ZIP URLs are accepted. Hugo archives with a
`layouts/` structure are converted into SSG's theme layout during extraction.
Downloaded code and templates should be reviewed before use.

## Theme structure

```text
templates/my-theme/
├── base.html
├── index.html
├── page.html
├── post.html
├── category.html
├── tag.html               # optional; falls back to category.html
├── author.html            # optional; falls back to category.html
├── series.html            # optional; falls back to category.html
├── layouts/
│   └── landing.html       # optional page layout
├── partials/              # theme-owned organisation/assets
├── shortcodes/
│   └── promo.html
├── css/
├── js/
└── images/
```

Standard template roles:

| File | Renders |
|---|---|
| `index.html` | Homepage and paginated homepage pages |
| `page.html` | Normal pages |
| `post.html` | Posts |
| `category.html` | Categories and fallback for other archives |
| `tag.html` | Tag archive when present |
| `author.html` | Author archive when present |

**Define names must match file names.** Templates are selected by their
*define* name, not the file name. If your theme wraps templates in
`{{define "…"}}` blocks, copying `category.html` to `author.html` is not
enough — the copy still defines `category.html`. Rename the define to
`author.html` to activate it. Since v1.8.5 such a "shell" file falls back
gracefully (never a blank page) and the build prints a warning naming the fix.
Themes without `{{define}}` blocks are matched by file name and need no
renaming.
| `series.html` | Series archive when present |
| `layouts/<name>.html` | A page with frontmatter `layout: <name>` |
| `<name>.html` | A page with frontmatter `template: <name>` |

Custom `layout` and `template` selection currently applies to pages. Posts use
`post.html`. If a page's selected custom template is absent, SSG falls back to
`page.html`.

For the Go engine, when the theme root contains no `.html` files, SSG creates
the five standard templates. If any root HTML template exists, the theme is
treated as intentional and missing files are not individually scaffolded.
Non-Go engines never receive generated templates and must ship all templates
they need.

Theme CSS, JavaScript and images are copied to output. Build transforms such as
SCSS, bundling, minification and fingerprinting run afterward.

## Template loading and sharing

Three directories are parsed, in this order, into **one** template set:

| Parsed | Contents |
|---|---|
| `<theme>/*.html` | the role templates above |
| `<theme>/layouts/*.html` | per-page layouts selected by frontmatter `layout:` |
| `<theme>/partials/*.html` | shared `{{define}}` blocks, callable from any of the above |

Because it is one set, a `{{define "site-header"}}` written in
`partials/chrome.html` is callable from `index.html`, `post.html` or a layout —
that is how a theme keeps its `<head>`, header and footer in one place instead
of copying them into every role file:

```gotemplate
{{/* partials/chrome.html */}}
{{define "site-header"}}
  <header>…{{ .Domain }}…</header>
{{end}}

{{/* page.html */}}
{{ template "site-header" . }}
```

Pass a computed context with `dict` when a partial needs values the caller
knows:

```gotemplate
{{ template "site-head" (dict
    "Title" (printf "%s — %s" .Page.Title .Domain)
    "Canonical" (printf "/%s/" .Page.Slug)
    "Ctx" .) }}
```

Notes that save a debugging session:

- **Only `.html` files are parsed.** Other files under `partials/` are neither
  parsed nor copied to the output; public assets belong in `css/`, `js/` or
  `images/`, which are the only theme directories copied verbatim.
- **A file whose defines do not match its filename renders nothing on its own.**
  That is exactly what makes `partials/chrome.html` work — and why a
  `category.html` copied to `author.html` still needs its define renamed (see
  the warning described above).
- **`base.html`** is part of the scaffolded starter themes and is only used by
  a theme that chooses to define and call it; it is not a required file and SSG
  never invokes it implicitly.
- Templates are parsed once per build, after content is loaded, so site data is
  fully available to every helper.

The bundled `ssgtheme` is the reference implementation of this layout:
`partials/chrome.html` holds the head, header and footer; the four role
templates hold only what is unique to them. See
[`templates/ssgtheme/README.md`](../templates/ssgtheme/README.md).

## Template engines

| Engine | Configuration | Aliases | Syntax |
|---|---|---|---|
| Go | `engine: go` | default | `html/template` |
| Pongo2 | `engine: pongo2` | `jinja2`, `django` | Jinja2/Django-like |
| Mustache | `engine: mustache` | — | Logic-less Mustache |
| Handlebars | `engine: handlebars` | `hbs` | Handlebars |

CLI example:

```bash
ssg my-blog my-theme example.com --engine=pongo2
```

Syntax comparison:

```gotemplate
{{ range .Posts }}
  <h2>{{ .Title }}</h2>
{{ end }}
```

```django
{% for post in Posts %}
  <h2>{{ post.Title }}</h2>
{% endfor %}
```

```mustache
{{#Posts}}
  <h2>{{Title}}</h2>
{{/Posts}}
```

```handlebars
{{#each Posts}}
  <h2>{{Title}}</h2>
{{/each}}
```

Non-Go engines receive the same data model adapted to their renderer. Their
theme files must be written in the selected engine's syntax, and Go template
inheritance (`{{define}}`/`{{template}}`) does not apply. Rendered Markdown
content is provided as HTML for these engines.

### Helper support across engines (GO-054)

SSG hands every engine the same helper library. Pongo2 exposes helpers as
**filters** (`{{ value|helper }}`, `{{ value|helper:arg }}`); Handlebars
exposes them as **helpers** (`{{helper value}}`). Mustache is logic-less and
cannot call helpers at all. Anything an engine cannot express is reported once
at build time — never silently ignored.

| Helper group | Go | Pongo2 | Handlebars | Mustache |
|---|:--:|:--:|:--:|:--:|
| Classic (`safeHTML`, `formatDate`, `stripHTML`, `default`, `dict`, …) | ✅ | ✅¹ | ✅ | ❌ |
| Conditionals (`in`, `contains`, `startsWith`, `ternary`, `matches`, …) | ✅ | ✅ | ✅ | ❌ |
| Image (`imageResize`, `imageSrcSet`, `imageInfo`, …) | ✅ | ✅ | ✅ | ❌ |
| External sources (`getExternal`, `getExternalMeta`) | ✅ | ✅ | ✅ | ❌ |
| i18n (`t`) | ✅ | ✅ | ✅ | ❌ |
| Collection (`where`, `filter`, `sortBy`, `groupBy`, `pluck`, …) | ✅ | ⚠️² | ⚠️² | ❌ |

¹ Helpers returning HTML are marked safe automatically; pipe through pongo2's
own `|safe` only if you compose further. ² Helpers with more than two arguments
(pongo2) or more than three (Handlebars), and variadic helpers, cannot be
adapted — calling one raises a visible error (pongo2) or renders a
`[helper X error: …]` marker (Handlebars) plus a build warning, so the failure
is never silent.

### Engine limitations

- **Mustache**: logic-less by design — no helpers, filters, or expressions.
  Prepare any derived values in front matter or switch to Pongo2/Handlebars.
- **Pongo2**: helper results that are already HTML are returned as safe values;
  plain strings are auto-escaped like any pongo2 output.
- **Partials/inheritance**: alt-engine themes load each template file
  independently; use that engine's native include mechanism, not Go's
  `{{define}}`.

## Rendering contexts

SSG uses different root contexts for individual content and collection pages.
Rely on the tables below rather than assuming every value exists everywhere.

### Homepage

| Value | Type | Meaning |
|---|---|---|
| `.Site` | site data | All pages, posts, categories, media and authors |
| `.Posts` | list | Posts on the current pagination page |
| `.Pages` | list | All pages |
| `.Domain` | string | Canonical host |
| `.Vars` | map | Custom configuration variables |
| `.Data` | map | YAML/JSON data files |
| `.Pager` | pager | Pagination state |

`.Pager` contains `Current`, `Total`, `PerPage`, `PrevURL` and `NextURL`:

```gotemplate
{{ if gt .Pager.Total 1 }}
  {{ if .Pager.PrevURL }}<a href="{{ .Pager.PrevURL }}">Previous</a>{{ end }}
  <span>{{ .Pager.Current }} / {{ .Pager.Total }}</span>
  {{ if .Pager.NextURL }}<a href="{{ .Pager.NextURL }}">Next</a>{{ end }}
{{ end }}
```

### Page and post

Individual content fields are flattened at the root:

| Value | Meaning |
|---|---|
| `.Site`, `.Domain`, `.Vars`, `.Data` | Global site/configuration data |
| `.ID`, `.Title`, `.Slug`, `.Status`, `.Type` | Identity fields |
| `.Date`, `.Modified` | Dates converted to the configured content timezone |
| `.Content`, `.Excerpt`, `.Description`, `.Keywords` | Body and metadata text |
| `.URL`, `.CanonicalURL`, `.OutputPath` | Computed destinations |
| `.Link`, `.Canonical`, `.Robots`, `.Sitemap` | Explicit URL/SEO values |
| `.Author`, `.Categories`, `.Category`, `.Tags` | Taxonomy values |
| `.FeaturedImage` | Hero/social image |
| `.Layout`, `.Template` | Content template selection fields |
| `.WordCount`, `.ReadingTime` | Computed reading statistics |
| `.HasMath`, `.TOC` | Optional authoring output |
| `.Series` | Series name |
| `.SeriesPrevURL`, `.SeriesPrevTitle` | Previous series item |
| `.SeriesNextURL`, `.SeriesNextTitle` | Next series item |
| `.Lang`, `.Languages`, `.DefaultLanguage` | Language state |
| `.Translations`, `.Hreflang` | Language switching/alternate links |

For compatibility, the complete model is also available as `.Page` on pages or
`.Post` on posts. Unknown frontmatter keys are flattened into the same root but
cannot overwrite standard values.

### Category, tag, author and series archives

| Value | Meaning |
|---|---|
| `.Site` | Complete site data |
| `.Category` | Category-compatible name/slug object |
| `.Kind` | `category`, `tag`, `author` or `series` |
| `.Name` | Display name |
| `.Series` | Series name for compatibility |
| `.Posts` | Posts in the archive |
| `.Domain`, `.Vars`, `.Data` | Global values |

Category, tag and author posts are newest first. Series posts are oldest first
to preserve reading order.

## Site data

`.Site` contains:

```text
.Site.Domain
.Site.Pages
.Site.Posts
.Site.Categories   # map keyed by integer ID
.Site.Media        # map keyed by integer ID
.Site.Authors      # map keyed by integer ID
```

Examples:

```gotemplate
{{ range .Site.Pages }}
  <a href="{{ .GetURL }}">{{ .Title }}</a>
{{ end }}
```

```gotemplate
{{ with index .Site.Authors .Author }}
  <a href="/author/{{ .Slug }}/">{{ .Name }}</a>
{{ end }}
```

## Data and variables

`data/authors/ada.yaml` is exposed as `.Data.authors.ada`. Configuration:

```yaml
variables:
  analytics_id: G-XXXX
  api:
    endpoint: https://api.example.com
```

becomes:

```gotemplate
{{ .Vars.analytics_id }}
{{ .Vars.api.endpoint }}
```

See [CONFIGURATION.md](CONFIGURATION.md#data-and-variables) for environment
resolution and hook exports.

## Go template helpers

The Go engine includes standard `html/template` operations plus SSG functions.
A small example:

```gotemplate
{{ $recentGuides := .Site.Posts
    | where "Type" "post"
    | sort "Date" "desc"
    | first 5
}}

{{ range $recentGuides }}
  <article><a href="{{ .GetURL }}">{{ .Title }}</a></article>
{{ end }}
```

Helper groups include:

- collection operations: `where`, `sort`, `first`, `last`, `groupBy`, `uniq`,
  `pluck`, `reverse`, `limit` and `offset`;
- conditionals: `in`, `notIn`, `contains`, `startsWith`, `matches`, `isNil`,
  `isEmpty` and `ternary`;
- content helpers: `latest`, `published`, `byTag`, `byCategory`, `byAuthor` and
  `related`;
- Markdown, URL, date and WordPress migration helpers;
- build-time image helpers.

Invalid collection helper use fails rendering with a descriptive error. Inputs
are not mutated. Full signatures and comparison rules are documented in
[TEMPLATE_HELPERS.md](TEMPLATE_HELPERS.md).

## Image helpers

Go templates and shortcodes can inspect and process images:

```gotemplate
{{ $img := imageResize "images/hero.jpg"
    (dict "width" 1200 "height" 630 "mode" "fill" "format" "webp") }}
<img src="{{ $img.URL }}"
     width="{{ $img.Width }}"
     height="{{ $img.Height }}"
     alt="">
```

Generated variants use a deterministic content-addressed cache below
`processed_images/`. WebP output requires `cwebp`. See [IMAGES.md](IMAGES.md).

## Shortcode templates

Shortcode template paths are relative to the selected theme. A configured
`shortcodes/promo.html` can use:

```gotemplate
<aside class="promo promo--{{ .Type }}">
  {{ if .Logo }}<img src="{{ .Logo }}" alt="">{{ end }}
  <h2>{{ .Title }}</h2>
  <p>{{ .Text }}</p>
  <a href="{{ .Url }}">Learn more</a>
  {{ if .Legal }}<small>{{ .Legal }}</small>{{ end }}
</aside>
```

### What is in scope inside a shortcode template

A shortcode template is executed against the **shortcode itself**, not against
the page — `.` and `$` are the same object. In scope:

| Expression | Source |
|---|---|
| `.Name` `.Type` `.Title` `.Text` `.Url` `.Logo` `.Legal` `.Ranking` `.Tags` | the `shortcodes:` entry |
| `.Data.key` | the entry's `data:` map (values are strings) |
| `.Attrs.key`, `.InnerContent` | the invocation: `[name key="v"]inner[/name]` |
| `.Vars.key`, `$.Vars.key` | site-wide `variables:` (same map page templates see) |

**Not** in scope: `.Page`, `.Site`, `.Posts`, `.Categories` or anything else
from a page template's context. A shortcode has no page — the same instance may
render on many pages — so reaching for page data is a template error.

A template error does not stop the build by default: the shortcode is dropped
from the page and a warning is printed. Set `shortcode_errors` (or
`--shortcode-errors=`) to change that:

| Mode | Result |
|---|---|
| `drop` (default) | warning; the shortcode is removed from the page |
| `keep` | warning; the shortcode's **raw source** stays in the page, so the gap is visible |
| `strict` | as `keep`, and the build fails after rendering |

`keep` and `strict` are the ones to use in CI: a page that quietly lost its
payment widget still looks fine, whereas one showing `[stripe_form]` does not.
The raw source also survives HTML minification, which an HTML comment marker
would not.

Configuration and supported forms are documented in
[CONFIGURATION.md](CONFIGURATION.md#shortcodes).

## Creating a theme

1. Create `templates/<name>/`.
2. Add all five standard templates, even though the Go engine can scaffold an
   entirely empty theme. Explicit files make the theme portable and reviewable.
3. Start with `index.html`, `page.html`, `post.html` and `category.html` using
   only the documented context for each.
4. Put public assets below `css/`, `js/` or `images/`.
5. Test empty collections, content without optional metadata, pagination and a
   production build with minification.
6. Run strict link validation:

```bash
ssg my-blog my-theme example.com --clean --check-links=strict
```

Do not edit generated files in `output/`; change the theme or source instead.

