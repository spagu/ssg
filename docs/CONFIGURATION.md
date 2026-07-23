# Configuration reference

SSG can be configured with command-line flags or a YAML, TOML or JSON file. This
guide explains the configuration model and advanced features. The exhaustive,
copyable YAML template is [.ssg.yaml.example](../.ssg.yaml.example).

## Loading configuration

Select a file explicitly:

```bash
ssg --config path/to/site.yaml
```

Without `--config`, SSG checks the current directory in this order:

```text
.ssg.yaml  .ssg.yml  .ssg.toml  .ssg.json
ssg.yaml   ssg.yml   ssg.toml   ssg.json
```

Command-line flags are parsed after the file and override matching file values.
The positional values `source`, `template` and `domain` are read from the file
when all three are present. Otherwise, provide all three positionally —
`source` itself is optional once `content_sources` is configured.

Two diagnostics make a misconfigured file obvious instead of silent:

- **Unknown keys warn.** A YAML key this binary does not know is reported by
  name and ignored. A config written for a newer ssg therefore still builds,
  and the version mismatch is visible rather than looking like a missing value.
- **Missing required settings are named.** Instead of printing usage alone, ssg
  reports which of `source`/`template`/`domain` is missing, which config file
  it read and what that file provided.

### Splitting the config across files (`include:`)

A `.ssg.yaml` can pull in other YAML files — from a local path or a URL — so a
large config splits into focused pieces (each worker its own file, shared
defaults in a base):

```yaml
include:
  - shared/base.yaml                      # local, relative to this file
  - workers/comments/config.yaml
  - url: https://example.com/team.yaml    # remote
    auth:                                 # private source (optional)
      type: bearer                        # bearer | basic | header
      token: $TEAM_CONFIG_TOKEN           # secrets are env refs, never literals
```

Merge rules (YAML configs only):

- **Base-first.** Includes are merged in listed order, then the including file
  is overlaid on top, so **the main file always wins**.
- **Maps merge** recursively.
- **Lists of named maps merge by `name`** — so each worker's own file can add
  one entry to `workers:` (or `content_sources:`) without clobbering the
  others. Any other list is replaced wholesale.
- Includes may nest; a cycle is an error, and a diamond (two files pulling the
  same base) is fine.

Remote includes reuse the auth model below: `type` is `bearer`, `basic`
(`username` + `password`) or `header` (`header` name + `value`), and every
secret field must reference an environment variable.

```yaml
source: my-blog
template: simple
domain: example.com
```

```bash
ssg my-blog simple example.com
```

Most features are disabled by default. Defaults listed below come from the
current `config.DefaultConfig`; omitted strings and booleans otherwise use Go's
empty value.

## Core and paths

| Key | Default | CLI | Purpose |
|---|---:|---|---|
| `source` | required | positional | Local content collection |
| `template` | required | positional | Theme name |
| `domain` | required | positional | Canonical host without scheme |
| `content_dir` | `content` | `--content-dir` | Parent of local sources |
| `content_sources` | empty | `--content-source` (repeatable) | Extra Markdown roots merged into the site; see [CONTENT.md](CONTENT.md#extra-sources-content_sources) |
| `auto_excerpt` | `false` | `--auto-excerpt` | Derive a missing excerpt from the opening paragraph |
| `templates_dir` | `templates` | `--templates-dir` | Parent of themes |
| `output_dir` | `output` | `--output-dir` | Generated site destination |
| `static_dir` | `static` | `--static-dir` | Verbatim passthrough files |
| `data_dir` | `data` | `--data-dir` | YAML/JSON data for `.Data` |
| `pages_path` | `pages` | config only | Pages directory inside a source |
| `posts_path` | `posts` | config only | Posts directory inside a source |
| `quiet` | `false` | `--quiet`, `-q` | Suppress normal output |

`output_dir` is generated state. `clean: true` deletes its old contents before
building. See [CONTENT.md](CONTENT.md) for the source directory contract.

## Template selection

| Key | Default | CLI | Purpose |
|---|---:|---|---|
| `engine` | Go behaviour | `--engine` | `go`, `pongo2`, `mustache` or `handlebars` |
| `online_theme` | empty | `--online-theme` | GitHub, GitLab or direct ZIP theme URL |

The `template` core value names the destination/local theme directory. Engine
aliases accepted by the CLI include `jinja2`/`django` for Pongo2 and `hbs` for
Handlebars. Non-Go themes must ship their own templates in the chosen syntax.
See [TEMPLATES.md](TEMPLATES.md).

## Development server

| Key | Default | CLI | Purpose |
|---|---:|---|---|
| `http` | `false` | `--http` | Start the built-in server after building |
| `host` | `127.0.0.1` | `--host` | Bind address |
| `port` | `8888` | `--port` | TCP port |
| `watch` | `false` | `--watch` | Rebuild after local file changes |
| `watch_runner` | `""` | `--watch-runner` | Spawns a background watch runner process |
| `watch_runner_config` | `""` | `--watch-runner-config` | Config file the runner should use |
| `watch_runner_dir` | `""` | `--watch-runner-dir` | Directory the runner starts in |
| `clean` | `false` | `--clean` | Remove previous output before builds |

`watch_runner` coordinates background execution of development emulators (like `wrangler` or `workerd`). When configured, `ssg` automatically monitors files for rebuilds and spawns the runner in parallel, piping its output and terminating it on exit. Spelled `--wrangler` (for `npx wrangler dev`) or `--workerd` (for `workerd serve`) as CLI convenience flags.

`watch_runner_config` points the runner at a config file kept anywhere on disk,
so a `wrangler.toml` does not have to sit in the project root next to `.ssg`.
The path is passed as `--config <path>` to `wrangler` and to custom runners, and
as the positional config argument to `workerd serve`. A missing file is reported
as a warning; the runner is still started so its own error message is visible.

`watch_runner_dir` starts the runner in another directory — the monorepo case,
where the Worker lives in `booking/apps/api/` while content and templates stay
at the repo root. Without it `npx wrangler dev` runs where `ssg` was invoked and
fails with *"Missing entry-point to Worker script or to assets directory"*. A
relative `watch_runner_config` is resolved against **ssg's** working directory
before the runner is started, so both options can be combined safely. A
directory that does not exist aborts the runner (the build itself continues).

`--wrangler-config=FILE`, `--wrangler-dir=DIR` and the `--workerd-*` pair are
convenience spellings: each sets its value **and** selects that runner (so
`--wrangler` is implied), in any flag order. Use `--watch-runner-config=FILE` /
`--watch-runner-dir=DIR` with a custom `--watch-runner`.

```bash
# Worker in a subdirectory of the same repo (issue #35)
ssg --watch --wrangler-dir=booking/apps/api my-site simple example.com

# wrangler config kept in deploy/, not in the project root
ssg --wrangler-config=deploy/wrangler.toml my-site simple example.com

# equivalent, spelled out
ssg --watch-runner=wrangler --watch-runner-config=deploy/wrangler.toml \
    my-site simple example.com
```

```yaml
watch_runner: wrangler
watch_runner_dir: booking/apps/api
watch_runner_config: booking/apps/api/wrangler.jsonc
```

Pair it with [environment variables in `external_sources`](EXTERNAL_SOURCES.md#environment-variables-in-values)
to point the same config at the local Worker during development and at the
production API in CI.

`watch` monitors content, templates and data. Touch-only changes whose bytes are
unchanged do not trigger a rebuild; actual changes still cause a full build.

Use `host: 0.0.0.0` only when the preview must be reachable from other machines.

### Public TLS and hardening

```yaml
http: true
port: 443
tls_cert: cert.pem
tls_key: key.pem
http3: true
gzip: true
max_conns: 1024
mem_limit: 512MiB
```

| Key | Default | CLI | Purpose |
|---|---:|---|---|
| `tls_cert` | empty | `--tls-cert` | Manual PEM certificate |
| `tls_key` | empty | `--tls-key` | Manual PEM private key |
| `tls_auto` | `false` | `--tls-auto` | Obtain certificates with Let's Encrypt |
| `tls_domain` | empty | `--tls-domain` | Autocert host names, comma-separated |
| `http3` | `false` | `--http3` | Add HTTP/3/QUIC alongside HTTPS |
| `gzip` | `false` | `--gzip` | Compress accepted responses |
| `max_conns` | `0` | `--max-conns` | Connection limit; `0` is unlimited |
| `mem_limit` | empty | `--mem-limit` | Go runtime soft memory limit |

TLS enables HTTP/2 automatically through ALPN. HTTP/3 requires TLS and uses the
same UDP port. Manual certificate/key configuration takes priority over
automatic certificates. Autocert requires a public domain and access to ports
80/443.

The server automatically applies `X-Content-Type-Options`, `X-Frame-Options`,
`Referrer-Policy`, HSTS under TLS, and cache-control suitable for HTML and
fingerprinted assets.

## Output and URLs

| Key | Default | CLI | Purpose |
|---|---:|---|---|
| `sitemap_off` | `false` | `--sitemap-off` | Disable `sitemap.xml` |
| `robots_off` | `false` | `--robots-off` | Disable `robots.txt` |
| `pretty_html` | `false` | `--pretty-html` | Remove blank lines from HTML |
| `relative_links` | `false` | `--relative-links` | Convert absolute site links to relative links |
| `post_url_format` | `date` behaviour | `--post-url-format` | `date` or `slug` |
| `page_format` | `directory` behaviour | `--page-format` | `directory`, `flat` or `both` |
| `permalinks.post` | empty | `--permalink-post` | Tokenised post URL pattern |
| `permalinks.page` | empty | `--permalink-page` | Tokenised page URL pattern |
| `rewrite_md_links` | `false` | config only | Rewrite source `.md` links to final URLs (anchors and query strings are carried over) |
| `strip_md_link_text` | `false` | config only | Drop `.md` from link text that is a bare filename (`[CONFIGURATION.md]…` → "CONFIGURATION") |
| `link_rewrites` | empty | config only | Map an href prefix to a replacement, for links to repository files the site never publishes |
| `preserve_slug_case` | `false` | config only | Do not lowercase slugs |
| `outputs` | HTML only | `--outputs=html,json` | Add per-page JSON output |

The `permalinks` map contains the optional `post` and `page` patterns. Permalink
tokens are `:year`, `:month`, `:day`, `:slug` and `:category`.

`rewrite_md_links` turns in-repository links (`CONFIGURATION.md`,
`./guide.md#section`) into the built page URLs, carrying any `#anchor` or
`?query` across. `strip_md_link_text` complements it at publish time: when a
link's visible text is exactly a filename ending in `.md`, the `.md` is dropped
(`[CONFIGURATION.md](CONFIGURATION.md)` renders as "CONFIGURATION"). Only bare
filename link text is touched — prose, inline code (`` `CONFIGURATION.md` ``) and
code blocks are left alone, and the source `.md` files are never modified.
`link_rewrites` covers the other half of a documentation site:
links to repository files that the site never publishes. It maps an href prefix
to its replacement, longest match first, so one rule can cover a folder and
another override a single file:

```yaml
link_rewrites:
  "../examples/": "https://github.com/spagu/ssg/tree/main/examples/"
  "../.ssg.yaml.example": "https://github.com/spagu/ssg/blob/main/.ssg.yaml.example"
```

With both set, `check_links` on a documentation site can reach zero warnings.
Frontmatter `link` always has higher priority. Detailed URL rules are in
[CONTENT.md](CONTENT.md#slugs-and-urls).

## Minification and assets

| Key | Default | CLI | Purpose |
|---|---:|---|---|
| `minify_all` | `false` | `--minify-all` | Enable HTML, CSS and JS minification |
| `minify_html` | `false` | `--minify-html` | Minify HTML only |
| `minify_css` | `false` | `--minify-css` | Minify CSS only |
| `minify_js` | `false` | `--minify-js` | Minify JavaScript only |
| `sourcemap` | `false` | `--sourcemap` | Emit v3 maps for minified CSS/JS |
| `fingerprint` | `false` | `--fingerprint` | Hash CSS/JS names and rewrite references |
| `scss` | `false` | `--scss` | Compile SCSS with Dart Sass |
| `sass_binary` | `sass` on PATH | `--sass-binary` | Explicit Dart Sass executable |
| `bundles` | empty | config only | Concatenate named CSS/JS groups |

Example bundles:

```yaml
bundles:
  app.css: [reset.css, layout.css, theme.css]
  app.js: [vendor.js, main.js]
```

Bundles are created before minification and fingerprinting. Fingerprinting
renames CSS/JS to `name.<hash8>.ext`, emits `assets-manifest.json`, and rewrites
HTML/CSS references in dependency order. Source maps require corresponding CSS
or JavaScript minification. SCSS is removed from final output after compilation;
if Dart Sass is missing, the step is skipped with a warning.

HTML regions can opt out of minification:

```html
<!-- htmlmin:ignore -->
<pre>Whitespace is preserved here.</pre>
<!-- /htmlmin:ignore -->
```

## Images

| Key | Default | CLI | Purpose |
|---|---:|---|---|
| `webp` | `false` | `--webp` | Convert copied JPG/PNG images to WebP |
| `webp_quality` | `60` | `--webp-quality` | Quality from 1 to 100 |
| `webp_keep_original` | `false` | `--webp-keep-original` | Keep originals next to the `.webp` files |
| `reconvert_images` | `false` | `--reconvert-images` | Ignore existing conversion result |
| `image_sizes` | empty | `--image-sizes` | Responsive widths; no upscaling |
| `image_sizes_attr` | `100vw` | `--image-sizes-attr` | Generated HTML `sizes` value |

WebP encoding requires the optional `cwebp` executable. Build-time resize,
crop, filter and source-set helpers are covered by [IMAGES.md](IMAGES.md).

By default WebP conversion **replaces** each original in the output (the
historical behaviour): `logo.png` becomes `logo.webp` and `<img src>`
references are rewritten. Themes that hardcode extensions outside rewritten
attributes — favicons, logos, `og:image` — 404 in that mode. Set
`webp_keep_original: true` to emit the `.webp` next to the original: rewritten
references serve WebP, hardcoded ones keep working (v1.8.5).

## Authoring

| Key | Default | CLI | Purpose |
|---|---:|---|---|
| `sanitize_html` | `false` | `--sanitize-html` | Apply bluemonday's UGC policy to rendered content |
| `highlight` | `false` | `--highlight` | Highlight fenced code with Chroma |
| `highlight_style` | `github` | `--highlight-style` | Chroma style name |
| `highlight_line_numbers` | `false` | — | Prefix highlighted blocks with line numbers (needs `highlight`) |
| `toc` | `false` | `--toc` | Expose `.TOC`; `[toc]` also expands |
| `toc_depth` | `3` | `--toc-depth` | Maximum TOC heading level |
| `math` | `false` | `--math` | Inject KaTeX on pages containing math |
| `mermaid` | `false` | — | Render ```` ```mermaid ```` fences as diagrams |
| `mermaid_theme` | — | — | Mermaid built-in theme: `default`, `neutral`, `dark`, `forest`, `base` |
| `mermaid_background` | — | — | Solid CSS colour boxed behind each diagram |

`mermaid: true` rewrites a ```` ```mermaid ```` fence into a
`<pre class="mermaid">` block before rendering (so the diagram source is passed
through verbatim, not HTML-escaped) and injects the mermaid.js runtime **only on
pages that contain a diagram** — the same page-scoped approach as KaTeX. A
mermaid fence stays a plain code block when the option is off.

Diagrams are transparent by default, so on dark site chrome they can be hard to
read. `mermaid_background` (any CSS colour — `#ffffff`, `white`,
`hsl(0 0% 100%)`) paints a solid panel behind each diagram with padding and
rounded corners, and `mermaid_theme` picks a matching palette (`neutral` or the
light `default` read best on a dark page). Both apply only to pages that contain
a diagram. Example:

```yaml
mermaid: true
mermaid_theme: neutral
mermaid_background: "#ffffff"
```

Math detection recognises display `$$...$$` and fenced ```` ```math ````
blocks (fences are rewritten to display math before rendering, GO-055).
Inline `\(...\)` is **not** supported — CommonMark backslash-escaping would
consume the delimiters. Sanitisation is recommended for untrusted remote
content; it is off for trusted local authoring to avoid changing intentional
HTML.

### Shortcodes

Shortcodes are configured reusable snippets whose template file is required:

```yaml
shortcodes:
  - name: promo
    template: shortcodes/promo.html
    type: banner
    title: Summer offer
    text: Read the terms before continuing.
    url: https://example.com/offer
    logo: /images/offer.png
    legal: Terms apply.
    ranking: 4.5
    tags: [public, featured]
    data:
      colour: green
```

Use `{{promo}}` in Markdown. The template receives `.Name`, `.Type`, `.Title`,
`.Text`, `.Url`, `.Logo`, `.Legal`, `.Ranking`, `.Tags` and `.Data`.

Enable WordPress-style syntax with:

```yaml
shortcode_brackets: true
```

It supports attributes and paired content:

```markdown
[link url="https://example.com" label="Read more"]
[box type="warning"]Inner Markdown content[/box]
```

Templates read inline values from `.Attrs` and paired text from
`.InnerContent`. Unknown bracket tags remain unchanged.

Site-wide `variables:` are reachable as `.Vars.key` / `$.Vars.key`, the same
spelling page templates use. Page context (`.Page`, `.Site`, `.Posts`, …) is
**not** in scope — one shortcode instance may render on many pages. The full
scope table is in [TEMPLATES.md](TEMPLATES.md#what-is-in-scope-inside-a-shortcode-template).

| Key | Default | CLI | Purpose |
|---|---:|---|---|
| `shortcode_errors` | `drop` | `--shortcode-errors` | What a shortcode that fails to render leaves in the page |

- `drop` — a warning, and the shortcode is removed from the page (historical
  behaviour, so existing sites build byte-identically).
- `keep` — a warning, and the shortcode's raw source (`{{promo}}`,
  `[promo a="b"]`) stays in the page, so the failure is visible rather than
  shipping as a silently missing block.
- `strict` — as `keep`, and the build fails once rendering finishes, listing
  every shortcode that failed. Recommended in CI.

```yaml
variables:
  stripe_public_key: "pk_test_123"

shortcode_errors: strict
```

## Blog, feeds and search

| Key | Default | CLI | Purpose |
|---|---:|---|---|
| `paginate` | `0` | `--paginate` | Posts per index page; `0` disables |
| `feed` | `false` | `--feed` | Root and category/tag Atom feeds |
| `feed_items` | `20` | `--feed-items` | Maximum feed items |
| `feed_full_content` | `false` | config only | Full rendered body instead of summary |
| `search_index` | `false` | `--search-index` | Emit `search-index.json` |

Pagination writes page 1 at the site root and pages 2 onward under `/page/N/`.
Themes receive `.Pager`. The search index contains title, URL, tags, excerpt,
plain text and the per-post taxonomies map, intended for a client-side search
widget.

## Taxonomies

`category`, `tag` and `series` are built in. The config-only `taxonomies:` map
declares additional dynamic taxonomies with per-term archives, metadata files,
optional per-term feeds and template helpers — the full reference (keys,
frontmatter priority, normalization rules, template fallback chains) lives in
[TAXONOMIES.md](TAXONOMIES.md).

## External sources

The config-only `external_sources:` block feeds templates from local files
(YAML/JSON/TOML/CSV/XML), remote HTTP APIs (hardened client + shared disk
cache), read-only SQL queries (MySQL/MariaDB/PostgreSQL/SQLite) and CMS
imports (WordPress, Drupal, Movable Type — merged into the site or exposed as
data). Everything lands under `.ExternalData`; `.Data` is unchanged. Secrets
come exclusively from environment variables. CLI: `--offline`,
`--refresh-external-sources`, `--clear-external-cache`,
`--external-source=NAME`. Full reference:
[EXTERNAL_SOURCES.md](EXTERNAL_SOURCES.md).

## Server access control

| Key | Default | CLI | Purpose |
|---|---:|---|---|
| `server_auth` | empty | config only | `basic` or `jwt` (HS256); empty = open |
| `server_users` | empty | config only | Basic-auth users as `login:$PASS_ENV` |
| `jwt_secret` | empty | config only | HS256 shared secret, env reference |
| `ip_allowlist` | empty | config only | Only these IPs/CIDRs may connect |
| `ip_blocklist` | empty | config only | These IPs/CIDRs are refused first |
| `rate_limit` | `0` | config only | Requests/second per client IP |
| `rate_burst` | `0` | config only | Token-bucket size (default 2×rate) |

The chain runs blocklist → allowlist → rate limiter → auth, before the file
server. Passwords and the JWT secret must reference environment variables;
`X-Forwarded-For` is not trusted. SSO and LDAP are deliberately not
implemented.

## SEO and validation

| Key | Default | CLI | Purpose |
|---|---:|---|---|
| `seo` | `false` | `--seo` | Inject missing Open Graph, Twitter and JSON-LD metadata |
| `check_links` | empty | `--check-links[=warn|strict]` | Validate internal links |
| `lastmod_from_git` | `false` | `--lastmod-from-git` | Use Git commit dates in sitemap |

SEO injection is non-destructive and skips pages that already provide their own
Open Graph tags. The old `seo_off`/`--seo-off` setting is a deprecated no-op.
Plain `--check-links` selects warning mode; strict mode fails the build.

## Data and variables

Files below `data_dir` with `.yaml`, `.yml` or `.json` extensions are loaded by
path into `.Data`:

```text
data/authors/ada.yaml → .Data.authors.ada
```

Custom variables are exposed as `.Vars` and exported to hooks as `SSG_*`:

```yaml
variables:
  analytics_id: $ANALYTICS_ID
  api:
    endpoint: https://api.example.com
```

Values beginning with `$` resolve from the current process environment. Nested
keys are flattened for environment names, for example
`SSG_API_ENDPOINT`. Do not commit secrets to configuration files.

## Internationalisation and timezones

```yaml
languages: [pl, en]
default_language: pl
timezone: Europe/Warsaw
language_timezones:
  en: America/New_York
  pl: Europe/Warsaw
```

| Key | Default | CLI | Purpose |
|---|---:|---|---|
| `languages` | empty | `--languages=pl,en` | Enable multilingual output |
| `default_language` | empty | `--default-language` | Language kept at the root |

For the opt-in expanded multilingual system, translation dictionaries and
prefix/fallback policies, see [I18N.md](I18N.md).
| `timezone` | empty | `--timezone` | IANA zone for content dates |
| `language_timezones` | empty | config only | Per-language zone override |

Non-default languages are written below `/<lang>/`. Templates receive `.Lang`,
`.Languages`, `.DefaultLanguage`, `.Translations` and `.Hreflang`. Timezones
affect permalink calendar tokens and template dates; feeds and sitemap remain UTC.

## Build hooks

Hooks execute trusted local commands without a shell:

```yaml
hooks:
  pre_build: [./scripts/prepare.sh]
  post_build: [./scripts/report.sh]
  post_page: []
```

| Phase | Timing | Failure behaviour |
|---|---|---|
| `pre_build` | Before generation | Fails the build |
| `post_page` | After each page | Logged and non-fatal |
| `post_build` | After generation | Fails the build |

Commands are argv-split, time-limited to 60 seconds, and never loaded from
content. Hooks receive `SSG_OUTPUT_DIR`, `SSG_PHASE`, and for page hooks
`SSG_PAGE_PATH`, plus exported custom variables.

## MDDB content

MDDB replaces local Markdown with remote documents:

```yaml
template: simple
domain: example.com

mddb:
  enabled: true
  url: http://localhost:11023
  protocol: http
  collection: blog
  lang: en_US
  api_key: ""                    # optional; prefer --mddb-key from a secret env value
  timeout: 30
  batch_size: 1000
  watch: true
  watch_interval: 30
```

| Nested key | Default | CLI |
|---|---:|---|
| `mddb.enabled` | `false` | enabled by `--mddb-url` |
| `mddb.url` | empty | `--mddb-url` |
| `mddb.protocol` | HTTP behaviour | `--mddb-protocol=http|grpc` |
| `mddb.collection` | empty | `--mddb-collection` |
| `mddb.lang` | empty | `--mddb-lang` |
| `mddb.api_key` | empty | `--mddb-key` |
| `mddb.timeout` | `30` | `--mddb-timeout` |
| `mddb.batch_size` | `1000` | `--mddb-batch-size` |
| `mddb.watch` | `false` | `--mddb-watch` |
| `mddb.watch_interval` | `30` | `--mddb-watch-interval` |

HTTP commonly uses `http://localhost:11023`; gRPC commonly uses
`localhost:11024`. MDDB watch polls the collection checksum and rebuilds when it
changes. Values beginning with `$` are resolved only inside `variables`, not in
arbitrary configuration fields. In CI, pass an MDDB secret at runtime, for
example `--mddb-key="$MDDB_API_KEY"`. Use `sanitize_html` when remote content is
not fully trusted.

## Archives and deployment

| Key | Default | CLI |
|---|---:|---|
| `zip` | `false` | `--zip` |
| `targz` | `false` | `--targz` |
| `tarxz` | `false` | `--tarxz` |
| `deploy` | empty | `--deploy` |
| `deploy_project` | empty | `--deploy-project` |
| `deploy_branch` | provider default | `--deploy-branch` |
| `deploy_target` | provider-specific | `--deploy-target` |

Deployment credentials always come from environment variables. Provider details
and GitHub Action inputs are in [DEPLOYMENT.md](DEPLOYMENT.md).

## Redirects and headers (Cloudflare Pages / Netlify)

| Key | Default | Notes |
|---|---:|---|
| `redirects` | empty | list of `{from, to, status, force}` rules |
| `alias_stubs` | `true` | also write meta-refresh stub pages for `aliases:` |
| `headers` | empty | map of `path pattern → {header: value}` overrides |
| `headers_defaults_off` | `false` | drop the built-in security/cache blocks |

`redirects:` generates a real `_redirects` file: exact paths, `/old/*` splats
(`:splat` in the destination) and statuses `301`/`302`/`307`/`308`/`410`.
Frontmatter `aliases:` are added as `301`s and exact chains are flattened to a
single hop. `headers:` overrides or extends the generated `_headers` per
pattern. Full reference and the `ssg import redirects` importer:
[DEPLOYMENT.md](DEPLOYMENT.md).

```yaml
redirects:
  - from: /old-pricing
    to: /pricing        # status defaults to 301
  - from: /blog/*
    to: /articles/:splat
    status: 301
headers:
  /api/*:
    Access-Control-Allow-Origin: "*"
```

## Cloudflare Worker / Pages Functions

| Key | Default | Notes |
|---|---:|---|
| `worker.dir` | empty | Functions project (or dir with a prebuilt `_worker.js`) |
| `worker.mode` | `functions` | `functions` or `worker` |
| `worker.routes_include` | `["/api/*"]` | paths that invoke the Function |
| `worker.routes_exclude` | empty | paths carved back out to static |
| `worker.wrangler_config` | empty | wrangler config outside the project root |

Wires a Cloudflare Pages Function into the build for transactional endpoints
(payments, forms, dynamic pricing, tracking). Scaffold one with `ssg new worker
<template>`. Full guide: [WORKERS.md](WORKERS.md).

```yaml
worker:
  dir: workers/stripe-checkout
  mode: functions
  routes_include:
    - /api/*
```

## Complete example

```yaml
source: my-blog
template: simple
domain: example.com

content_dir: content
templates_dir: templates
output_dir: output
static_dir: static
data_dir: data

clean: true
minify_all: true
fingerprint: true
feed: true
search_index: true
seo: true
check_links: strict

webp: true
webp_quality: 80
image_sizes: [480, 960, 1600]

paginate: 10
outputs: [html]
```

Before relying on a key in automation, compare it with
[.ssg.yaml.example](../.ssg.yaml.example) and `ssg --help` from the installed
version.
