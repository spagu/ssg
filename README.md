# SSG — Static Site Generator

[![Go Version](https://img.shields.io/badge/Go-1.26.5+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![CI](https://github.com/spagu/ssg/actions/workflows/ci.yml/badge.svg)](https://github.com/spagu/ssg/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/spagu/ssg)](https://goreportcard.com/report/github.com/spagu/ssg)
[![codecov](https://codecov.io/gh/spagu/ssg/branch/main/graph/badge.svg)](https://codecov.io/gh/spagu/ssg)
[![GitHub Release](https://img.shields.io/github/v/release/spagu/ssg?style=flat&color=blue)](https://github.com/spagu/ssg/releases)
[![License](https://img.shields.io/badge/License-BSD_3--Clause-blue.svg)](LICENSE)

SSG is a fast static site generator written in Go. It turns Markdown with YAML
frontmatter into a complete website with clean URLs, templates, feeds, search,
image processing and optional native deployment.

It works especially well for blogs and WordPress migrations, but can also build
documentation, company sites, portfolios and landing pages.

[Quick start](#quick-start) · [Content model](#content-model) ·
[Configuration](#configuration) · [Templates](#templates) ·
[Deployment](#deployment) · [Documentation](#documentation)

## Why SSG?

- Fast, deterministic builds with a single Go binary
- Markdown content with YAML frontmatter
- Built-in `simple` and `krowy` themes
- Go, Pongo2, Mustache and Handlebars template engines
- Sitemap, robots.txt, Atom feeds, search index and SEO metadata
- WebP conversion, responsive images, SCSS, minification and fingerprinting
- Local server with automatic rebuilds
- Native deployment to Cloudflare Pages, GitHub Pages, Netlify, Vercel, FTP and SFTP
- GitHub Action and multi-architecture Docker images

Most advanced features are opt-in. A basic command only reads content, renders
HTML and writes the result to `output/`.

## Quick start

### 1. Install SSG

Linux and macOS:

```bash
curl -sSL https://raw.githubusercontent.com/spagu/ssg/main/install.sh | bash
```

Other supported installation methods:

| Method | Command or location |
|---|---|
| Homebrew | `brew install spagu/tap/ssg` |
| Snap | `snap install static-site-generator && sudo snap alias static-site-generator ssg` |
| Release binaries and packages | [GitHub Releases](https://github.com/spagu/ssg/releases) |
| Docker Hub | `docker pull tradik/ssg:latest` |
| GitHub Container Registry | `docker pull ghcr.io/spagu/ssg:latest` |
| Build from source | `make build` |

For platform-specific instructions, see [docs/INSTALL.md](docs/INSTALL.md).

### 2. Create the smallest useful site

```text
content/
└── my-blog/
    ├── metadata.json
    ├── pages/
    │   └── about.md
    └── posts/
        └── general/
            └── hello.md
```

`content/my-blog/metadata.json`:

```json
{
  "title": "My Blog",
  "description": "Thoughts and notes",
  "url": "https://example.com",
  "language": "en",
  "categories": [{ "id": 1, "name": "General", "slug": "general" }],
  "users": [{ "id": 1, "name": "Editor", "slug": "editor" }],
  "media": []
}
```

`content/my-blog/pages/about.md`:

```markdown
---
title: About
slug: about
status: publish
type: page
---

This is my first page.
```

`content/my-blog/posts/general/hello.md`:

```markdown
---
title: Hello World
slug: hello-world
status: publish
type: post
date: 2026-04-01
categories: [General]
author: 1
---

This is my first post.
```

### 3. Build and preview

```bash
ssg my-blog simple example.com --http --watch
```

Open <http://127.0.0.1:8888>. SSG rebuilds the site when its files change.
The generated site is written to `output/`; do not edit that directory by hand.

The `simple` and `krowy` themes are embedded in the binary and scaffolded when
first used, so this example does not require a local `templates/simple/` folder.

## Command model

```text
ssg <source> <template> <domain> [options]
```

| Argument | Meaning | Default location or use |
|---|---|---|
| `source` | Content collection name | `content/<source>/` |
| `template` | Theme name | `templates/<template>/` or an embedded theme |
| `domain` | Canonical host without a scheme | Canonical URLs, feeds, sitemap and SEO |

Example:

```bash
ssg my-blog krowy example.com --clean --minify-all
```

This reads `content/my-blog/`, uses the `krowy` theme, treats
`https://example.com` as the canonical site root and writes to `output/`.

All three values may instead be provided by a configuration file. In MDDB mode,
content is fetched remotely and the source argument is optional.

## Content model

SSG uses explicit locations and predictable output rules. These rules are useful
both when authoring a site manually and when generating one programmatically.

### Directory contract

```text
project/
├── .ssg.yaml                  # optional configuration
├── content/
│   └── <source>/
│       ├── metadata.json      # required for a local content source
│       ├── pages/             # recursively loaded pages
│       ├── posts/
│       │   └── <group>/       # at least one directory below posts/
│       │       └── post.md    # deeper nesting is allowed
│       └── media/             # optional content media
├── templates/
│   └── <template>/            # optional when using an embedded theme
├── data/                      # optional YAML/JSON template data
├── static/                    # optional files copied verbatim
└── output/                    # generated; safe to delete and rebuild
```

Important invariants:

1. Pages are loaded recursively from `pages/`.
2. Posts must be inside at least one subdirectory of `posts/`. Files directly in
   `posts/` are ignored. Below the first subdirectory, nesting is recursive.
3. A post's category comes from its `categories` frontmatter, not its directory.
4. Local builds require `metadata.json` at the root of the selected source.
5. Files with frontmatter are rendered only when `status: publish` is present.
6. A plain Markdown file without frontmatter is treated as published content.
7. `output/` contains generated artifacts and must not be used as source content.

The directory names can be changed with `pages_path`, `posts_path`,
`content_dir`, `templates_dir`, `data_dir`, `static_dir` and `output_dir`.

### Frontmatter reference

For predictable results, pages and posts should define `title`, `status` and
`type`. Posts should additionally define `date`.

| Field | Type | Meaning |
|---|---|---|
| `title` | string | Display title |
| `slug` | string | URL segment; defaults to the Markdown filename |
| `status` | string | Only `publish` is rendered when frontmatter exists |
| `type` | string | `page` or `post`; affects templates and URL generation |
| `date` | date | Post publication date in `YYYY-MM-DD` form |
| `modified` | date | Last modification date |
| `categories` | list | Category IDs, names or slugs from `metadata.json` |
| `author` | integer/string | Author ID, name or slug from `metadata.json` |
| `tags` | list | Free-form tags; generates `/tag/<slug>/` listings |
| `series` | string | Generates a `/series/<slug>/` listing and navigation |
| `excerpt` | string | Summary for listings, feeds and metadata |
| `description` | string | SEO description; falls back to the excerpt |
| `link` | string | Explicit URL path; overrides normal URL rules |
| `canonical` | string | Explicit canonical URL |
| `aliases` | list | Previous paths that should redirect to this item |
| `featured_image` | string | Hero and Open Graph image |
| `layout` | string | Page layout name; `redirect` also marks sitemap exclusion |
| `template` | string | Page template file override |
| `robots` | string | Robots directive; `noindex` also excludes from sitemap |
| `sitemap` | string | Set to `no` to exclude from `sitemap.xml` |
| `lang` | string | Content language for multilingual builds |

Unknown frontmatter keys are preserved and exposed to templates. Author and
category matching is case-insensitive; unresolved values are ignored.

### Excerpts

Markdown can use explicit export-style sections:

```markdown
## Excerpt
A short description used in listings.

## Content
The complete article starts here.
```

Without these exact markers, all Markdown after the frontmatter becomes content.

## Configuration

SSG automatically detects `.ssg.yaml`, `.ssg.toml` or `.ssg.json`:

```bash
ssg
```

An explicit file can be selected with:

```bash
ssg --config path/to/site.yaml
```

Minimal `.ssg.yaml`:

```yaml
source: my-blog
template: simple
domain: example.com

clean: true
minify_all: true
```

Common options:

| Goal | Configuration key | CLI flag |
|---|---|---|
| Development server | `http: true` | `--http` |
| Automatic rebuilds | `watch: true` | `--watch` |
| Clean output first | `clean: true` | `--clean` |
| Minify HTML/CSS/JS | `minify_all: true` | `--minify-all` |
| Convert images to WebP | `webp: true` | `--webp` |
| Responsive images | `image_sizes: [480, 960]` | `--image-sizes=480,960` |
| Fingerprint CSS/JS | `fingerprint: true` | `--fingerprint` |
| Compile SCSS | `scss: true` | `--scss` |
| Generate Atom feeds | `feed: true` | `--feed` |
| Generate search index | `search_index: true` | `--search-index` |
| Add SEO metadata | `seo: true` | `--seo` |
| Validate internal links | `check_links: strict` | `--check-links=strict` |
| Create ZIP package | `zip: true` | `--zip` |

WebP output requires the optional `cwebp` executable. SCSS compilation requires
the optional Dart Sass `sass` executable. Other native image operations use Go,
but selecting WebP as their output format also requires `cwebp`.

The canonical configuration reference is [.ssg.yaml.example](.ssg.yaml.example).
The CLI also provides an installed-version reference:

```bash
ssg --help
```

## Common recipes

| Task | Command |
|---|---|
| Preview while editing | `ssg my-blog simple example.com --http --watch` |
| Production build | `ssg my-blog simple example.com --clean --minify-all` |
| WebP and responsive images | `ssg my-blog simple example.com --webp --image-sizes=480,960,1600` |
| Feed, search and SEO | `ssg my-blog simple example.com --feed --search-index --seo` |
| Immutable asset names | `ssg my-blog simple example.com --minify-all --fingerprint` |
| Strict link validation | `ssg my-blog simple example.com --check-links=strict` |
| Create deployment archives | `ssg my-blog simple example.com --zip --targz --tarxz` |
| Use a Pongo2 theme | `ssg my-blog my-theme example.com --engine=pongo2` |
| Use only configuration | `ssg --config .ssg.yaml` |

Options are composable unless a specific option documents otherwise.

## Capability map

This is a discovery index, not a second configuration reference. Exact defaults
and accepted values live in [.ssg.yaml.example](.ssg.yaml.example).

| Area | Available capabilities |
|---|---|
| Authoring | Shortcodes, table of contents, syntax highlighting, KaTeX math, raw HTML sanitization |
| Blog | Pagination, tags, categories, series, reading time, Atom feeds, related content |
| Taxonomies | Custom dynamic taxonomies with term archives, metadata, per-term feeds and template helpers ([docs/TAXONOMIES.md](docs/TAXONOMIES.md)) |
| SEO and migration | Sitemap, robots.txt, aliases, configurable permalinks, canonical URLs, link checking, `.md` link rewriting |
| Assets | WebP, responsive variants, build-time image helpers, SCSS, bundles, minification, source maps, fingerprinting |
| Data | YAML/JSON data files, custom variables and static passthrough files |
| External sources | Unified `.ExternalData` from local files (YAML/JSON/TOML/CSV/XML), HTTP APIs with a hardened client + disk cache, read-only SQL (MySQL/MariaDB/PostgreSQL/SQLite) and CMS imports (WordPress, Drupal, Movable Type) ([docs/EXTERNAL_SOURCES.md](docs/EXTERNAL_SOURCES.md)) |
| Localisation | Full i18n: translation keys, dictionaries + `t`, language routing, `hreflang`/`x-default`, per-language feeds and search ([docs/I18N.md](docs/I18N.md)) |
| Content sources | Local Markdown or MDDB over HTTP/gRPC, including watched remote content |
| Output | Directory/flat pages, JSON output, feeds, search index, ZIP, tar.gz and tar.xz |
| Server | File watching, gzip, TLS, automatic certificates, HTTP/2, HTTP/3, resource limits, basic/JWT auth, IP allow/block lists and per-IP rate limiting |
| Automation | Lifecycle hooks, Git-derived modification dates, GitHub Action and native deployment |

## Templates

### Engines

| Engine | Value | Syntax family |
|---|---|---|
| Go templates | `go` | Go `html/template`; default and full helper support |
| Pongo2 | `pongo2` | Jinja2/Django |
| Mustache | `mustache` | Logic-less Mustache |
| Handlebars | `handlebars` | Handlebars blocks and helpers |

Select an engine with `--engine=<value>` or `engine: <value>`. Non-Go themes must
contain templates authored in their selected syntax; they do not receive the Go
template FuncMap or Go block inheritance.

### Theme files

A typical Go theme contains:

```text
templates/my-theme/
├── base.html
├── index.html
├── page.html
├── post.html
├── category.html
├── css/
├── js/
├── layouts/
└── partials/
```

Missing standard templates receive built-in fallbacks. Themes may also be
downloaded with `--online-theme=<URL>`.

Common Go template values:

| Value | Meaning |
|---|---|
| `.Title`, `.Content`, `.Excerpt` | Current page/post content |
| `.URL`, `.CanonicalURL` | Relative and canonical URLs |
| `.Date`, `.Modified` | Content dates |
| `.Site.Pages`, `.Site.Posts` | Site collections |
| `.Data` | Data loaded from `data/` |
| `.Vars` | Custom configuration variables |
| `.Pager` | Pagination state when enabled |

For collection helpers, conditionals and image functions, see
[docs/TEMPLATE_HELPERS.md](docs/TEMPLATE_HELPERS.md) and
[docs/IMAGES.md](docs/IMAGES.md).

## Generated output

Depending on enabled features, `output/` can contain:

```text
output/
├── index.html
├── <page-slug>/index.html
├── <year>/<month>/<day>/<post-slug>/index.html
├── category/<category-slug>/index.html
├── tag/<tag-slug>/index.html
├── series/<series-slug>/index.html
├── css/
├── js/
├── media/
├── sitemap.xml
├── robots.txt
├── feed.xml
└── search-index.json
```

URL layouts can be changed with `page_format`, `post_url_format`, explicit
permalink patterns or an item's `link` field.

## Deployment

SSG can deploy the generated output without provider-specific CLIs. Credentials
are read from environment variables, never from content files.

| Provider | Flag | Required environment |
|---|---|---|
| Cloudflare Pages | `--deploy=cloudflare` | `CLOUDFLARE_API_TOKEN`, `CLOUDFLARE_ACCOUNT_ID` |
| GitHub Pages | `--deploy=github-pages` | `GITHUB_TOKEN` or SSH credentials |
| Netlify | `--deploy=netlify` | `NETLIFY_AUTH_TOKEN` |
| Vercel | `--deploy=vercel` | `VERCEL_TOKEN`, `VERCEL_ORG_ID` |
| FTP | `--deploy=ftp` | `FTP_USERNAME`, `FTP_PASSWORD` |
| SFTP | `--deploy=sftp` | `SSH_PASSWORD` or `SSH_KEY_FILE` |

Example:

```bash
CLOUDFLARE_API_TOKEN=... CLOUDFLARE_ACCOUNT_ID=... \
  ssg my-blog simple example.com \
  --deploy=cloudflare --deploy-project=my-site
```

Deployment runs after generation and post-processing.

## GitHub Actions

```yaml
name: Build site

on:
  push:
    branches: [main]

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v6
      - uses: spagu/ssg@v1
        with:
          source: my-blog
          template: simple
          domain: example.com
          clean: "true"
          minify: "true"
```

All supported inputs and outputs are defined in [action.yml](action.yml). Deployment
workflow examples are available in [examples/workflows](examples/workflows/).

## Development

Building SSG itself requires Go 1.26.5 or newer. Earlier Go 1.26 releases contain
standard-library vulnerabilities relevant to this project.

```bash
git clone https://github.com/spagu/ssg.git
cd ssg
make all
```

Useful targets:

| Command | Purpose |
|---|---|
| `make build` | Build `build/ssg` |
| `make test` | Run tests |
| `make test-coverage` | Run tests and generate coverage |
| `make lint` | Run static checks |
| `make security` | Run security scanners |
| `make all` | Dependencies, lint, tests and build |
| `make install` | Install the binary and manual page |

Development workflow and review requirements are in
[CONTRIBUTING.md](CONTRIBUTING.md). Please follow
[CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md). Existing contributors are listed in
[CONTRIBUTORS.md](CONTRIBUTORS.md).

## Documentation

| Document | Scope |
|---|---|
| [.ssg.yaml.example](.ssg.yaml.example) | Complete configuration reference |
| [docs/INSTALL.md](docs/INSTALL.md) | Platform installation guide |
| [docs/CONTENT.md](docs/CONTENT.md) | Content structure, frontmatter and URL rules |
| [docs/CONFIGURATION.md](docs/CONFIGURATION.md) | Configuration and advanced feature guide |
| [docs/I18N.md](docs/I18N.md) | Internationalisation: translations, dictionaries, language routing |
| [docs/TAXONOMIES.md](docs/TAXONOMIES.md) | Dynamic taxonomies: definitions, term metadata, archives, helpers |
| [docs/EXTERNAL_SOURCES.md](docs/EXTERNAL_SOURCES.md) | External data: files, HTTP APIs, SQL, CMS imports, cache, security |
| [docs/TEMPLATES.md](docs/TEMPLATES.md) | Theme files, engines and rendering contexts |
| [docs/TEMPLATE_HELPERS.md](docs/TEMPLATE_HELPERS.md) | Go template helper reference |
| [docs/IMAGES.md](docs/IMAGES.md) | Build-time image processing |
| [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) | Native providers, archives and GitHub Actions |
| [docs/STYLES.md](docs/STYLES.md) | Built-in theme style guide |
| [examples/README.md](examples/README.md) | Example projects and workflows |
| [CONTRIBUTING.md](CONTRIBUTING.md) | Development and contribution workflow |
| [CHANGELOG.md](CHANGELOG.md) | Release history and migration notes |
| [SECURITY.md](SECURITY.md) | Vulnerability reporting policy |

## License

SSG is distributed under the [BSD 3-Clause License](LICENSE).
