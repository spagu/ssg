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

Non-Go engines receive the same data model adapted to their renderer, but do
not receive Go's FuncMap or Go template inheritance. Their theme files must be
written in the selected engine's syntax. Rendered Markdown content is provided
as HTML for these engines.

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

Bracket shortcodes additionally expose `.Attrs` and `.InnerContent`. Configuration
and supported forms are documented in
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

