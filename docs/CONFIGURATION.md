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

A remote include can also tune its own fetch. All four are optional and fall
back to the defaults shown:

```yaml
include:
  - url: https://config.example.com/base.yaml
    auth: { type: bearer, token: $CONFIG_TOKEN }
    timeout: 30s        # per-attempt timeout (default 30s)
    retries: 3          # extra attempts on a transient failure (default 3; 0 disables)
    retry_delay: 5s     # wait between attempts (default 5s)
    on_error: fail      # fail the build (default) or warn and continue without it
```

- A transient failure — a network/transport error or an HTTP `429`/`5xx` — is
  retried up to `retries` times, `retry_delay` apart. A `4xx` (missing,
  forbidden) is **not** retried, since it will not recover.
- `on_error: warn` prints a warning and continues the build without that
  include, so an optional or occasionally-unreachable remote config doesn't
  block a publish. `on_error: fail` (the default) stops the build.
- `timeout`/`retry_delay` accept a Go duration (`30s`, `1m`) or a plain number
  of seconds. Remote worker `source:` archives use the same defaults.

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
| `domain` | required | positional | Canonical host, **without a scheme** — `example.com`, not `https://example.com`. A scheme or trailing slash is stripped and reported, since it would otherwise reach every absolute URL the site publishes |
| `title` | empty | config only | Site name → `.Site.Title` (a migration fills it in) |
| `description` | empty | config only | Site tagline → `.Site.Description` |
| `colors` | empty | config only | Palette by role → `.Site.Colors.<role>` and `--ssg-color-<role>` |
| `posts_page` | empty | config only | Where the post listing goes, e.g. `blog` → `/blog/` |
| `content_dir` | `content` | `--content-dir` | Parent of local sources |
| `content_sources` | empty | `--content-source` (repeatable) | Extra Markdown roots merged into the site; see [CONTENT.md](CONTENT.md#extra-sources-content_sources) |
| `auto_excerpt` | `false` | `--auto-excerpt` | Derive a missing excerpt from the opening paragraph |
| `templates_dir` | `templates` | `--templates-dir` | Parent of themes |
| `output_dir` | `output` | `--output-dir` | Generated site destination |
| `static_dir` | `static` | `--static-dir` | Verbatim passthrough files |
| `static_sources` | empty | config only | Extra verbatim passthrough roots, each keeping its own name |
| `data_dir` | `data` | `--data-dir` | YAML/JSON data for `.Data` |
| `pages_path` | `pages` | config only | Pages directory inside a source |
| `posts_path` | `posts` | config only | Posts directory inside a source |
| `quiet` | `false` | `--quiet`, `-q` | Suppress normal output |

### Site identity and palette (`title`, `description`, `colors`)

```yaml
title: "Magna Valor"
description: "Supply Chain Global Advisory"
colors:
  primary: "#7b2ff7"
  secondary: "#54595f"
  accent: "#61ce70"
  text: "#222733"
  background: "#f6f6f6"
  link: "#a4836d"
```

Templates read these as `.Site.Title`, `.Site.Description` and
`.Site.Colors.primary`. The palette is also emitted on `:root` as
`--ssg-color-primary`, `--ssg-color-secondary`, … so a theme can style against
the site's own colours without the values being copied into its stylesheet, and
the primary colour stands in for `<meta name="theme-color">` when nothing else
declares one. Roles beyond the six above are allowed and follow them
alphabetically; a theme that already declares `--ssg-color-*` itself wins.

`ssg migrate` fills all three in from the source site (see
[MIGRATE.md](MIGRATE.md#what-the-sites-own-wiring-becomes)) — but only where the
config has nothing, so an edit here is never overwritten.

### The front page (`posts_page`)

By default `/` is the generated post listing. A **content page** takes it
instead as soon as one resolves there — `link: "/"` in front matter, which is
what a WordPress static front page exports as:

```yaml
posts_page: blog     # the listing moves to /blog/ and /blog/page/2/
```

```text
🏠 Front page: home.md
   Post listing: /blog/
```

Without `posts_page` the listing has nowhere to go, so it is not generated —
reported, not silent:

```text
🏠 Front page: home.md
   8 post(s) are not listed anywhere — set posts_page: "blog" to publish the listing
```

`posts_page` works on its own too: a site with no front-page document can still
move its listing off the root. In multilingual builds the language prefix comes
first (`/pl/blog/`), and each language has its own front page.

### Extra feeds (`feeds`)

`feed: true` publishes one Atom feed of every post at `/feed.xml`, plus one per
language and one per taxonomy term. That is all-or-nothing: a site with several
content roots cannot offer "just the blog", and "the three tags that mean
*release*" would need three subscriptions.

`feeds:` declares any number of extra feeds, each choosing **what goes in**, **where
it is written** and **in what format**. `feed: true` keeps doing exactly what it
does today, so adding this changes nothing that already works.

```yaml
feeds:
  - path: /blog/feed.xml       # a whole content root
    title: "Blog"
    source: blog               # a content_sources path, or a content folder

  - path: /blog/rss.xml        # the same posts, a second format
    title: "Blog"
    source: blog
    format: rss

  - path: /docs/feed.json
    title: "Documentation updates"
    source: docs
    format: json

  - path: /releases.xml        # several terms in one feed
    title: "Release notes"
    format: rss
    categories: [release, changelog]
    items: 10
```

Two things worth knowing even if you only ever want Atom:

- **`feed: true` names the feed after the bare hostname.** A declared feed takes a
  `title`, so it can be called what it actually is. That alone is a reason to
  declare one rather than rely on `feed: true`.
- **SSG injects the autodiscovery `<link>` tags itself**, for every feed, into
  every page — so a theme should *not* hand-write them or the page ships
  duplicates. Turn injection off with `feed_autodiscovery: false` if the theme
  wants to own them.


| Key | Meaning |
|---|---|
| `path` | Output path — also the URL. Required |
| `title` | Feed title; defaults to the site domain |
| `format` | `atom` (default), `rss` (2.0) or `json` (JSON Feed 1.1) |
| `source` | A content root: matches that folder and everything beneath it |
| `categories` | Category names or slugs — any of |
| `tags` | Tags — any of |
| `type` | `post` (default) or `page` |
| `items` | Item cap for this feed; defaults to `feed_items` |
| `full_content` | Full body vs summary; defaults to `feed_full_content` |

Selection criteria are optional and combine with **AND** — `source: blog` plus
`tags: [release]` means release posts *from the blog folder*. A feed with no
criteria covers every post, at a path you choose.

### Aggregating feeds (a "planet")

A feed can merge several inputs — other sites' feeds and **your own posts** —
into one published feed. Read the sources with `format: feed`, then list them:

```yaml
external_sources:
  sources:
    ssg:  { type: http, url: https://ssg.tradik.com/feed.xml,  format: feed }
    mddb: { type: http, url: https://mddb.tradik.com/feed.xml, format: feed }

feeds:
  - path: /planet.xml
    title: "Planet Tradik"
    format: rss
    aggregate:
      - source: ssg
        label: "SSG"
      - source: mddb
        label: "MDDB"
        exclude:
          tags: [events]        # narrow this source only
      - site: blog              # your own content — "*" for every post
        label: "Tradik"
    exclude:
      words: [sponsored]        # applies to the whole feed
    items: 200                  # how many entries the feed carries at all
    paginate: 20                # how many per page
```

**Your own blog is an input like any other.** A planet without you is not your
planet — an aggregate that only republishes other people reads as a link dump.

| Key | Meaning |
|---|---|
| `aggregate[].source` | An `external_sources` name declared with `format: feed` |
| `aggregate[].site` | Your own content: a folder name, or `*` for every post |
| `aggregate[].label` | Provenance — attached to each item and emitted as a category |
| `aggregate[].include` / `.exclude` | Filters for **that source only** |
| `include` / `exclude` | Filters for the merged feed |
| `paginate` | Items per page; 0 (default) writes one file |

Filtering happens twice on purpose: **per source first, then feed-wide.** What
counts as noise depends on the feed it came from, and that context is gone once
everything is merged — one rule for the whole aggregate either lets noise through
or drops wanted items from the quieter sources. `words` match the title and
summary case-insensitively; `tags` match an item's categories. **Exclusion beats
inclusion**: a feed republishing other people's writing has to be able to say
"not this" with certainty.

Items are sorted newest first and **deduplicated by URL** — the same post reached
through two feeds is one item, and publishing it twice is the most visible way an
aggregate looks broken. A source that is unreachable or not declared with
`format: feed` warns and is skipped, rather than failing the build over one
site being down.

Paginated feeds are linked with RFC 5005 `rel="next"`/`"prev"`/`"first"`/`"last"`,
so a reader can walk the whole archive. **Page one keeps the declared path** —
`/planet.xml`, never `/planet-1.xml` — so the URL people already subscribed to
does not move as the archive grows.

Every published feed gets its own `<link rel="alternate">` with the correct MIME
type and title, injected into **every page including the homepage** — a reader
offering a choice reads exactly those links, so one Atom link would hide the rest.
A theme that advertises its own feed is left alone.

Set `feed_autodiscovery: false` to keep the feeds but **stop the injection into
your HTML** — for a theme that wants control over the links' order, their titles,
or which feeds are advertised at all:

```yaml
feed: true
feeds:
  - path: /rss.xml
    format: rss
feed_autodiscovery: false     # the feeds are still written; the <link> tags are yours
```

The links then have to come from the theme. A theme that already emits its own
feed link suppresses injection anyway — this is the explicit form of that, so the
behaviour does not depend on SSG noticing what the theme happened to render.

### Publishing files that live elsewhere (`static_sources`)

`static_dir` is a single root. When the files a site publishes verbatim already
live somewhere else in the repository — a specification the validator, the tests
and CI all read at the repo root — copying them into `static/` means maintaining
two copies that will drift, and staging them with a script means every
contributor has to know to run it.

```yaml
static_sources:
  - path: schema.json      # a file, served at /schema.json
  - path: xml              # a directory, served at /xml/... — the name is kept
  - path: editor
    dest: app              # placed at /app/ instead
  - path: build/assets
    dest: "."              # contents spread at the output root, like static_dir
```

Each entry keeps its own name by default, which is the point: URLs that already
exist keep resolving. Sources are copied **after** `static_dir`, so a later entry
wins a collision, and a missing path is a warning rather than a failed build.

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
| `port` | `8888` | `--port` | TCP port. Taken if free; otherwise the server walks forward (8889, 8890, …, up to 64 ports) and announces where it landed. `0` = any free port |
| `watch` | `false` | `--watch` | Rebuild after local file changes (content, templates, data and the config file) |
| `watch_runner` | `""` | `--watch-runner` | Spawns a background watch runner process |
| `watch_runner_config` | `""` | `--watch-runner-config` | Config file the runner should use |
| `watch_runner_dir` | `""` | `--watch-runner-dir` | Directory the runner starts in |
| `clean` | `false` | `--clean` | Remove previous output before builds |

`watch_runner` coordinates background execution of development emulators (like `wrangler` or `workerd`). When configured, `ssg` automatically monitors files for rebuilds and spawns the runner in parallel, piping its output and terminating it on exit. Spelled `--wrangler` (for `npx wrangler dev`) or `--workerd` (for `workerd serve`) as CLI convenience flags.

### What the watcher observes

`--watch` names its inputs at startup, e.g.
`👀 Watching for changes in content, templates, data, config (.ssg.yaml)...`

The **config file is a watched input of its own**: editing it reloads the
configuration and rebuilds with the new settings — no restart needed to change a
theme, a permalink scheme or any other option. Command-line flags still win over
the file, exactly as at startup. If an edit leaves the file unparseable, the
error is reported and the watcher keeps the **last good configuration** running
rather than exiting, so a half-saved file never kills a dev session.

A change is detected by content, not mtime: touching a file without changing its
bytes does not trigger a rebuild.

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

`--wrangler-config=FILE`, `--wrangler-dir=DIR`, `--workerd-config=FILE` and
`--workerd-dir=DIR` are
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
| `not_found_off` | `false` | `--not-found-off` | Disable the generated `404.html`. Without a 404 page, static hosts fall back to `index.html` for unmatched paths and answer `200`, so every dead URL reads to a crawler as a live copy of the home page. A page slugged `404` takes precedence |
| `pretty_html` | `false` | `--pretty-html` | Remove blank lines from HTML |
| `relative_links` | `false` | `--relative-links` | Convert absolute site links to relative links |
| `post_url_format` | `date` behaviour | `--post-url-format` | `date` or `slug` |
| `page_format` | `directory` behaviour | `--page-format` | `directory`, `flat` or `both` |
| `permalinks.post` | empty | `--permalink-post` | Tokenised post URL pattern |
| `permalinks.page` | empty | `--permalink-page` | Tokenised page URL pattern |
| `rewrite_md_links` | `true` | config only | Rewrite source `.md` links to final URLs (anchors/queries carried over); `false` opts out |
| `strip_md_link_text` | `false` | config only | Drop `.md` from link text that is a bare filename (`[CONFIGURATION.md]…` → "CONFIGURATION") |
| `link_rewrites` | empty | config only | Map an href prefix to a replacement, for links to repository files the site never publishes |
| `preserve_slug_case` | `false` | config only | Do not lowercase slugs |
| `outputs` | HTML only | `--outputs=html,json` | Add per-page JSON output |
| `markdown_publish` | `false` | config only | Publish a Markdown copy of every page (`index.md` + `page.md`), a `text/markdown` `<head>` alternate, and a root `llms.txt` — for language models and agents |
| `clean_special_chars` | `false` | config only | Normalise AI "smart" punctuation (curly quotes, en/em dashes, ellipsis, NBSP, zero-width) to ASCII across all content; CJK and other scripts untouched |
| `output_encoding` | `utf-8` | config only | Text-output encoding: `utf-8`, `utf-16le` or `utf-16be` (BOM added, `<meta charset>` kept in step) |
| `output_encoding_sections` | empty | config only | Per-section `output_encoding` overrides, keyed by content directory (longest prefix wins; `home` = root) |
| `home_pages_limit` / `home_posts_limit` | `6` | config only | Cap home-page guide/post cards before a "see all" link (`0` = default 6, negative = no limit) |
| `robots_rules` | empty | config only | Explicit per-crawler `robots.txt` directives (welcome/deny GPTBot, OAI-SearchBot, Googlebot…); empty = allow-all default |

The **Markdown-for-agents** set (`markdown_publish`, `clean_special_chars`,
`output_encoding`) serves crawlers that consume Markdown — including ChatGPT
Search and other LLMs. The published copy is the authored Markdown source, not
an HTML round-trip. Note that Google Search ignores `llms.txt` and Markdown
alternates (it reads the standard HTML), so these help third-party agents, not
Google ranking; ssg's standard SEO surface (`seo`, `schema`, canonical,
`sitemap`, `check_meta`/`check_images`, hreflang) covers Google's AI-optimization
guidance. `robots_rules` lets you state crawler policy explicitly for both.

The `permalinks` map contains the optional `post` and `page` patterns. Permalink
tokens are `:year`, `:month`, `:day`, `:slug` and `:category`.

`rewrite_md_links` turns in-repository links (`CONFIGURATION.md`,
`./guide.md#section`) into the built page URLs, carrying any `#anchor` or
`?query` across. **Only in-repository links**: an href with a scheme or a `//`
prefix is left alone, so a link to a file's history on a code host stays where
it points even though it ends in `.md`. The emitted URL follows
[`pretty_urls`](#link-checking), so the rewriter and `check_redirects` agree
about the same link. `strip_md_link_text` complements it at publish time: when a
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
| `minify_js` | `false` | `--minify-js` | Minify JavaScript only. Comments are removed by a scanner that understands strings, template literals and regex literals, so comment characters inside them are kept |
| `sourcemap` | `false` | `--sourcemap` | Emit v3 maps for minified CSS/JS |
| `fingerprint` | `false` | `--fingerprint` | Hash CSS/JS names and rewrite references |
| `scss` | `false` | `--scss` | Compile SCSS with Dart Sass |
| `sass_binary` | `sass` on PATH | `--sass-binary` | Explicit Dart Sass executable |
| `bundles` | empty | config only | Concatenate named CSS/JS groups |

**Bundle names and sources are paths relative to the output root**, not to the
theme. A theme whose assets land in `output/css/` must say so, otherwise every
source is reported missing and the bundle is written empty — which looks like a
broken theme rather than a config mistake:

```yaml
bundles:
  css/app.css:
    - css/reset.css
    - css/layout.css
    - css/theme.css
  js/app.js:
    - js/vendor.js
    - js/main.js
```

Bundling runs after assets are copied, so the paths to use are the ones you see
in `output/` after a build.

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
| `build_workers` | one per CPU | `--workers=N` | Parallel build workers; `0` = off (sequential) |

`build_workers` (`--workers=N`) sets how many **pages/posts render and images
convert to WebP in parallel**. Leave it unset to use the **whole machine** (one
worker per CPU), set an explicit `N` (e.g. `--workers=2`) to cap it on a shared
box, or `--workers=0` to turn parallelism **off** and build sequentially. The
render is grouped by language, so multilingual output stays correct; each item
writes its own file, so the output is byte-identical whatever the worker count —
only the wall-clock changes (verified with the race detector and the golden
snapshot harness).

WebP encoding requires the optional `cwebp` executable. Build-time resize,
crop, filter and source-set helpers are covered by [IMAGES.md](IMAGES.md).

**Scope.** WebP conversion runs over the **entire output tree** — content media,
copied `static/` files and theme assets alike, every `.jpg`/`.jpeg`/`.png` — not
just images under your content. There is no per-directory exclude list;
`webp_keep_original` (below) is the escape hatch when something must keep its
original extension.

By default WebP conversion **replaces** each original in the output (the
historical behaviour): `logo.png` becomes `logo.webp` and references are
rewritten to match. Rewriting covers `<img src>`/`srcset`, `href`, CSS
`url(...)`, the `og:image`/`twitter:image` social-preview metas and the JSON-LD
`image` value — so share previews follow the conversion instead of pointing at a
removed `.jpg`. Only references SSG cannot resolve to a local file stay on the
original extension: **absolute** URLs to your own images (`https://…/logo.png`,
left untouched on purpose) and — the common footgun — **paths built in
JavaScript at runtime**. SSG only rewrites HTML/CSS, so a script that fetches
`marker-icon.png` (e.g. a map library's default marker) keeps requesting the
`.png` that replace mode just deleted → a silent 404. When an asset is referenced
from JS, set `webp_keep_original: true` to emit the `.webp` next to the original —
rewritten HTML/CSS references serve WebP, the runtime `.png` still resolves
(v1.8.5) — or reference it from HTML/CSS instead so the rewrite can reach it.

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
| `date_archives` | `false` | — | Publish `/YYYY/`, `/YYYY/MM/` (and `/YYYY/MM/DD/` for dated permalinks) listings of your posts. Rendered by `category.html` with `Kind: "date"` and a label like "May 2014". Opt-in: WordPress has these URLs and links to them from every byline, a hand-authored site usually does not — `ssg migrate` turns it on. Real content that already owns such a path keeps it. |
| `type_archives` | empty | — | Which content types get a listing at `/<type>/` — the archive the source CMS renders and links to from its own menu, which is not a document and so is in no export. Keyed by type slug: `realizacje: true` builds it, `reviews: false` refuses it even when the export says the source had one. Rendered by `category.html` with `Kind: "type"`. See [Custom post type archives](#custom-post-type-archives) |
| `feed` | `false` | `--feed` | Root and category/tag **Atom** feeds at `/feed.xml` |
| `feeds` | empty | config only | Extra feeds — each with its own selection, `path`, `title` and **format** (`atom`, `rss`, `json`) |
| `feed_autodiscovery` | `true` | config only | Inject `<link rel="alternate">` for every feed into every page |
| `feed_items` | `20` | `--feed-items` | Maximum feed items |
| `feed_full_content` | `false` | config only | Full rendered body instead of summary |
| `search_index` | `false` | `--search-index` | Emit `search-index.json` |

Pagination writes page 1 at the site root and pages 2 onward under `/page/N/`.
Themes receive `.Pager`.

`search-index.json` is a JSON array of document objects, one per published page
and post, for a client-side search widget. Each object:

| Field | Type | Notes |
|---|---|---|
| `title` | string | Page title |
| `url` | string | Final page URL |
| `lang` | string | Language code (empty on single-language sites) |
| `locale` | string | BCP-47 locale (empty if unset) |
| `translation_key` | string | Groups a page's translations (empty if unset) |
| `tags` | string[] | Tag names |
| `excerpt` | string | Summary text |
| `text` | string | Full body as plain text (HTML stripped) |
| `taxonomies` | object | Present only when the page has custom taxonomies: `{ name: [terms…] }` |

On a multilingual build the array is still flat; filter by `lang` client-side.

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
| `schema` | empty | — | Site-wide JSON-LD defaults merged into every page (e.g. a publisher) |
| `schema_defaults` | empty | — | JSON-LD defaults per content section, so a section can carry an `@type` without every file repeating it |
| `check_links` | empty | `--check-links[=warn\|strict]` | Validate internal links |
| `check_images` | empty | `--check-images[=warn\|strict\|strict-decorative]` | Report images with **no** `alt` attribute |
| `check_meta` | empty | `--check-meta[=warn\|strict]` | Validate `<title>` and meta description on indexable pages |
| `check_orphans` | empty | `--check-orphans[=warn\|strict]` | Report indexable pages nothing links to |
| `check_markup` | `warn` | `--check-markup[=warn\|strict\|off]`, `--no-check-markup` | Report source markup indented into a code block (`ssg repair --fix`) |
| `check_schema` | `""` | `--check-schema[=MODE]` | Validate emitted JSON-LD against the properties search engines require: `""` (off), `warn`, `strict` |
| `check_redirects` | empty | `--check-redirects[=warn\|strict]` | Report links the host would redirect (needs `pretty_urls`) |
| `pretty_urls` | `false` | config only | The host strips `.html` and appends trailing slashes |
| `meta_limits` | see below | — | Advisory title/description length ranges for `check_meta` |
| `sitemap_prune_canonical` | `false` | — | Also drop non-self-canonical pages from `sitemap.xml` |
| `content_exclude` | empty | — | Globs for Markdown under `content_dir` that is **not** a page |
| `content_schemas` | empty | — | Per-type frontmatter contracts, validated at build |
| `strict` | `false` | `--strict` | Escalate schema violations and link checks to build failures |
| `route_manifest` | `false` | `--route-manifest` | Write `routes.json` — every route and its metadata |
| `lastmod_from_git` | `false` | `--lastmod-from-git` | Use Git commit dates in sitemap |

SEO injection is non-destructive, and it is **not** all-or-nothing. It looks at
what the page already rendered and fills only the gaps:

| The theme emitted | SSG injects |
|---|---|
| no `og:title` | Open Graph, Twitter **and** JSON-LD |
| `og:title`, no `application/ld+json` | **JSON-LD only** |
| both | nothing |

The middle row is the useful one: a theme can own its Open Graph tags — to control
`og:image`, say — and still get structured data generated from frontmatter, with
no need to hand-write JSON-LD. It also fills in a missing meta description from the
frontmatter `description:`.

The old `seo_off`/`--seo-off` setting is a deprecated no-op. Plain `--check-links`
selects warning mode; strict mode fails the build.

### Custom post type archives

A migration brings a WordPress custom post type across as a folder of documents,
each at the address the source served. What it cannot bring across is the type's
**archive**: `/realizacje/` is not a document anywhere — it is a listing
WordPress renders from `has_archive`. So the entries build, the site's own menu
links to the section, and the section is a 404.

`type_archives` says which types deserve one:

```yaml
type_archives:
  realizacje: true
  reviews: false
```

It cannot be inferred from the content, and that is not caution — a site can
register one type whose section exists and another whose section 404s on the
**source** as well. Building an index for every folder would publish pages the
original never had.

An export that records `has_archive` answers for itself: when
`content/<source>/metadata.json` carries

```json
{"custom_types": [
  {"slug": "realizacje", "name": "Realizacje", "has_archive": true},
  {"slug": "reviews",    "name": "Reviews",    "has_archive": false}
]}
```

the archive is built with no configuration at all, and a type marked
`"has_archive": false` is skipped. A `false` in `type_archives` overrules the
export — the operator has looked at the source and the export has not.

`archive_slug` moves the listing when the source did not serve it at the type's
own slug. WordPress lets `has_archive` **be** a slug, so a type called
`realizacje` can publish its archive at `/nasze-prace/`:

```json
{"slug": "realizacje", "has_archive": true, "archive_slug": "nasze-prace"}
```

`.ContentType` stays the type either way, so a theme styles the section by what
it is rather than by where it lives.

The listing is rendered by `category.html`, with the same context every other
archive gets plus two fields of its own:

| Field | Value |
|---|---|
| `.Kind` | `"type"` |
| `.ContentType` | the type slug, so a theme can style one section differently from another |
| `.Name` | the type's name from the export, or its slug made readable |
| `.Posts`, `.Pager` | as on a category archive — `paginate` applies, giving `/realizacje/page/2/` |

Real content wins: a hand-written page that already owns `/realizacje/` keeps it
and the build says so. Nothing is built for a declared type with no entries.

### Validating structured data

`check_schema` reads the JSON-LD each page actually emits and reports required
properties that are missing:

```
⚠️  structured data in recipes/pierogi.html → Recipe is missing image, recipeIngredient
⚠️  structured data in shop/laptop.html → Offer is missing priceCurrency
```

Search engines reject incomplete structured data and say nothing the author can
see: the build succeeds, the page ships, the rich result never appears, and the
feedback arrives weeks later in Search Console. Nested objects are checked too —
an `Offer` missing `priceCurrency` invalidates the `Product` containing it.

Types checked: `Recipe`, `Product`, `Offer`, `Event`, `JobPosting`,
`LocalBusiness`, `HowTo`, `VideoObject`, `Article`, `BlogPosting`,
`NewsArticle`, `FAQPage`. **An unrecognised `@type` passes silently** — that is
deliberate: schema.org has hundreds of types, and warning about the ones SSG
does not know would take away the generality `schema:` exists for. A block that
is not valid JSON is always reported, since a crawler cannot read it either and
nothing in the rendered page shows it.

Only the *required* properties are checked, not the recommended ones. Warning
about every optional field would train people to ignore the warning.

#### A type a section promised but never emitted

Missing entirely is a louder failure than present-but-incomplete, and it used to
be the one nothing reported. When `schema_defaults` declares an `@type` for a
section, every page in that section must carry it — and if none of the page's
JSON-LD does, the build says so:

```
⚠️  structured data in recipes/soup/index.html → schema_defaults promises @type "Recipe"
    and no JSON-LD on the page carries it — the theme emits 1 block(s) of its own, which
    turns auto-injection off for this page (emit the derived data yourself with
    {{ toJSON .Schema }}, or move the hand-written block into an @graph)
```

The usual cause is the SEO injection rule above: **a theme that emits any
`application/ld+json` block of its own opts the whole page out of
auto-injection.** So a theme with a hand-written `FAQPage` partial silently takes
the section's `Recipe` down with it — the page ships with complete FAQPage
markup, the check reports every required property present, and the Recipe rich
result never appears.

Two ways to have both, and the check accepts either:

```html
<!-- 1. emit the derived data beside your own block -->
<script type="application/ld+json">{{ toJSON .Schema }}</script>
<script type="application/ld+json">{"@context":"https://schema.org","@type":"FAQPage", …}</script>
```

```html
<!-- 2. or put both in one @graph -->
<script type="application/ld+json">
{"@context":"https://schema.org","@graph":[{{ toJSON .Schema }}, {"@type":"FAQPage", …}]}
</script>
```

`.Schema` is the structured data SSG would have injected, already merged in
precedence order — see [Template helpers](TEMPLATE_HELPERS.md#structured-data).
Sibling blocks are what Google's own guidance asks for when a Recipe and an
FAQPage describe the same page, so the first form is usually the right one.

A section whose `@type` is a *list* (`["Recipe", "Product"]`) promises nothing
specific and is not checked: which of them a given page must carry is the
author's business, and guessing would produce a warning nobody could act on.

### Structured data per section

`schema:` in frontmatter is arbitrary JSON-LD, so any schema.org type works
without SSG knowing it — `Recipe`, `Product`, `Event`, `Car`, nested objects and
all:

```yaml
schema:
  "@type": Recipe
  cookTime: PT20M
  recipeIngredient: ["500 g flour", "400 g potatoes"]
  nutrition: { "@type": NutritionInformation, calories: "320 kcal" }
```

What site-wide `schema:` cannot carry is `@type`: it applies to every page, so
setting `SoftwareApplication` for the home page would stop each post being a
`BlogPosting`. `schema_defaults` fills that gap — defaults keyed by section:

```yaml
schema:
  publisher: { "@type": Organization, name: Food }

schema_defaults:
  home:
    "@type": WebSite
    name: "Food — recipes and notes"
  pages/recipes:
    "@type": Recipe
    recipeCuisine: Polish
```

Keys match the page's directory **relative to the source folder**, by prefix,
longest match first — the same rule `link_rewrites` uses. `home` is reserved for
the site root, the only page that can hold a site-level type without claiming it
for everything else.

Precedence, lowest to highest:

```
schema:  <  derived (BlogPosting/WebPage/WebSite)  <  schema_defaults  <  page frontmatter
```

Section defaults sit **above** the derived data deliberately — overriding the
derived `@type` is what they exist for — while a page's own `schema:` still wins
over its section.

### Content contracts (schemas, strict mode, route manifest)

`content_schemas` declares what a page of each type must look like, so a missing
`author` or a malformed `date` fails at build time — with a precise message
(file, field, reason) — instead of silently shipping a broken page. Each schema
lists `required` fields and per-field `type`/`format`/`enum` rules:

```yaml
content_schemas:
  post:
    required: [title, date, author]
    fields:
      title:  { type: string }
      date:   { type: date }
      status: { type: enum, values: [publish, draft] }
      featured_image: { type: url }
      weight: { type: int }
```

Field types are `string`, `int`, `bool`, `date`, `url`, `list` and `enum` (with
`values`). Well-known frontmatter fields (`title`, `date`, `author`, `tags`, …)
resolve automatically; any other name is read from the page's custom frontmatter.

Violations **warn** by default so a site can adopt schemas incrementally. Turn on
`strict` (or `--strict`) to make them — and internal link checking — **hard build
failures**: a renamed slug that orphans a link, or a post missing a required
field, then fails the build instead of shipping. `strict` enables link checking
even when `check_links` is unset.

### Validating the built output

Three checks run over the generated HTML, in the same shape as `check_links`:
empty (off), `warn`, or `strict` (a finding fails the build). `strict: true`
escalates any enabled check. A fourth, `check_markup`, reads the **source**
instead — see below.

**`check_images`** reports images with **no `alt` attribute at all**. It never
generates alt text — an invented description reads as authoritative while being
wrong, which is worse for a screen-reader user than silence. `alt=""` is the
correct treatment for a decorative image (a logo next to the site name that would
otherwise be announced twice) and stays **silent**; `strict-decorative` opts into
reviewing those too.

| state | verdict |
|---|---|
| no `alt` attribute | reported — the author has to decide |
| `alt=""` | valid (decorative), silent unless `strict-decorative` |
| `alt="…"` | valid |

**`check_meta`** requires a non-empty `<title>` and meta description on every
indexable page. `noindex` pages are skipped: a 404 page legitimately has neither.
This catches a failure that is otherwise invisible — a theme interpolating a field
that happens to always be empty emits a blank tag on every page, forever, and the
generator has no reason to complain because it did exactly what the template
asked.

Lengths are reported as advisory notes, **never** as build failures, and the
ranges are yours to set. A headline that reads well at 62 characters beats one
mangled to fit, and a check that blocked the build on it would simply get
switched off.

```yaml
check_meta: warn
meta_limits:
  title_min: 30          # unset ⇒ default; explicit 0 disables the bound
  title_max: 60
  description_min: 70
  description_max: 160
```

**`check_orphans`** reports indexable pages that nothing links to. Only `<a href>`
counts: every page links to itself through `<link rel="canonical">`, so counting
all references would make nothing an orphan and the check would pass on a site
full of them. Self-links, `noindex` pages and the site root are ignored.

**`check_markup`** reports source Markdown whose markup is indented four columns
or more, which CommonMark renders as a literal code block. It is the **one check
that is on by default** (`warn`), because it does not weigh a judgement call the
way the others do: the page provably does not render as written, and the build
otherwise says nothing. It is silent when there is nothing to report.

This is what a page-builder export leaves behind — Elementor indents its nested
`<div>`s with tabs, the exporter turns `</p>` into a blank line, the blank line
ends the HTML block, and every following line is four columns deep. The visitor
reads `</div>` in monospace down the middle of the page.

```yaml
check_markup: warn      # default; "strict" fails the build, "" or "off" disables
```

Fix the content in place with **`ssg repair --fix`** (dry run without `--fix`,
which exits 1 on findings so CI can gate on it). Front matter, fenced code blocks
and list continuations are never touched. Re-exporting with wpexporter 1.8.2+
produces clean sources in the first place.

`seo: true` also fills in a missing meta description from the front-matter
`description:`. Nothing is invented — the author already wrote it, it just never
reached the output. An existing but empty tag is rewritten in place rather than
joined by a second one.

### Links the host redirects (`pretty_urls`, `check_redirects`)

`check_links` resolves a URL against the output directory. That is not how a host
answers it, so a link can pass and still cost every visitor a redirect. Most
static hosts serve **pretty URLs**: they strip a `.html` extension and append a
trailing slash, answering the un-normalised form with a 308.

```yaml
pretty_urls: true       # describe how the host serves URLs
check_redirects: warn   # "" | warn | strict
```

`pretty_urls` makes link checking agree with the host in **both** directions:

- `check_links` stops reporting `/docs/swagger` as broken when the output holds
  `docs/swagger.html` and the host serves it — without this the checker pushes you
  to restructure a page into a directory to satisfy the tool rather than the site.
- `check_redirects` reports the reverse: links that resolve *only* through a
  redirect, naming the destination so the fix is obvious.

```
⚠️  redirected link in index.html → /docs/swagger.html  →  /docs/swagger/
⚠️  redirected link in index.html → /docs/intro  →  /docs/intro/
```

Nothing here is broken, which is why `check_links` passes it — but each one is a
round trip per visitor and a hop of crawl budget per crawler, and it multiplies: a
single `.html` link in a shared footer puts every page on the site through a
redirect. It is invisible locally, because local resolution is not what the host
does.

Leave `pretty_urls` off for a plain object store, which rewrites nothing. There
`/docs/swagger` is a genuine 404 rather than a redirect, and `check_redirects`
skips with a message rather than reporting shapes the host never rewrites.

### Keeping the sitemap honest

`sitemap.xml` never lists a page whose rendered HTML says `noindex`: asking a
crawler to index a URL the page itself declines is reported as an error by search
consoles. This needs no configuration — the sitemap is written after rendering, so
the answer is already on disk, wherever the `noindex` came from.

Pages whose canonical points at a **different** URL are a separate case, and are
kept by default. A canonical that disagrees with the permalink is far more often a
theme bug than a deliberate exclusion, and quietly removing real pages from the
sitemap over one would be worse than the contradiction it fixes. Opt in with
`sitemap_prune_canonical: true`.

### Excluding Markdown that is not a page

`content_dir` is scanned recursively and every `.md` becomes a page. A file that
is *data* — a sample documenting another tool's front-matter format, say — may be
perfectly valid for its own purpose and unparseable as a page, and `status: draft`
cannot help because the failure happens while unmarshalling, before any status
field is read.

```yaml
content_exclude:
  - "docs/examples/**"    # ** crosses directory separators
  - "sample-*.md"         # bare filenames work too
```

Patterns are matched before parsing, against the full path, the content-relative
path and the filename, so each form behaves the way it reads.

`route_manifest` (or `--route-manifest`) writes `routes.json` to the output root:
a sorted, deduplicated list of every generated route — posts, pages, and category
/ tag / series / author / custom-taxonomy archives — each with its `type`,
`title`, source file and language. It is a machine-readable contract external
tooling (or generated typed clients) can diff to catch a route that moved.

A page's `featured_image` becomes the `og:image`, `twitter:image` (a
`summary_large_image` card) and the JSON-LD `image`, so one frontmatter field
drives every social preview. With `webp` on, all three follow the conversion to
`.webp` exactly like in-content images — no separate social-image setting to keep
in sync.

### AI-first JSON-LD structured data

With `seo` on, every page also gets `<script type="application/ld+json">`
Linked Data in its `<head>`, derived from existing frontmatter with **zero extra
configuration** — so AI agents and answer engines read structured, machine-
readable data without executing JavaScript. Content types map to Schema.org:

| Page | `@type` | Derived from |
|---|---|---|
| Blog post | `BlogPosting` | title, description, `date`/`modified`, author, tags → `keywords`, `featured_image` |
| Home page | `WebSite` | title, description |
| Any other page | `WebPage` | title, description |

Every non-home page additionally gets a `BreadcrumbList` built from its URL path,
placing it in the site hierarchy.

**Overrides.** Two knobs extend or replace the generated data, deep-merged in
order (most specific wins): site-wide `schema:` in the config, then per-page
`schema:` in frontmatter. Use the site-wide default for a publisher/Organization
that belongs on every page, and the per-page one to correct a `@type` or add
fields a single page needs:

```yaml
# .ssg.yaml — appears on every page
schema:
  publisher:
    "@type": Organization
    name: Acme Inc.
    logo: https://acme.example/logo.png
```

```yaml
# frontmatter — this page only
schema:
  "@type": TechArticle
  proficiencyLevel: Expert
```

The generated JSON-LD is valid Schema.org and passes Google's Rich Results Test.
`</script>` in any field is escaped, so untrusted titles cannot break out of the
block.

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

### Variables the bundled theme reads

`ssgtheme` is generic: each block below renders only when its variable is set,
so nothing here is required. They are the supported integration points, and are
listed because they were previously discoverable only by reading the theme.

| Variable | Renders |
|---|---|
| `gtag` | Google Analytics 4 (gtag.js) with Consent Mode v2 defaulting every storage type to `denied` |
| `gtm_id` | Google Tag Manager. When `cookie_consent` is also set the loader ships as `type="text/plain" data-consent-category="analytics"`, so the consent worker starts it only after the visitor accepts — the container request is itself a third-party call, so a site running a banner should not make it first |
| `cookie_consent` | The cookie banner. The value is serialised to the worker's client config; see [the worker's README](../workers/cookie-consent/README.md) for the keys |
| `marquee` | A horizontal "works with" strip: `{title, items: [{name, url, icon}]}`, where `icon` is SVG path data on a 24×24 viewBox |
| `repository_url` | The "source" link in the hero |

```yaml
variables:
  gtag: G-XXXXXXXXXX
  gtm_id: GTM-XXXXXXX
  cookie_consent:
    policyUrl: /cookie-policy/
    categories:
      - { id: necessary, required: true }
      - { id: analytics }
```

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
| `mddb.protocol` | HTTP behaviour | `--mddb-protocol=http\|grpc` |
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

### Structured frontmatter through MDDB's flat meta

MDDB stores metadata as a flat `key → list of strings` map, by design. A field
that is not flat — a `faq:` list of `{question, answer}` objects, a `schema:`
object — therefore has exactly one way through: **the producer JSON-encodes it
into a single meta string**, and ssg decodes it back when the document becomes a
page. Round-tripping is lossless and the theme ranges over the value as it would
with local frontmatter.

```json
{ "faq": "[{\"question\":\"How long?\",\"answer\":\"20 minutes\"}]" }
```

What does **not** work is stringifying the value. A loader that formats a Go map
stores `map[answer:20 minutes question:How long?]`, which cannot be recovered by
anyone. The build names the document and the field rather than letting the theme
fail on it:

```text
⚠️  document "chicken-soup": meta field faq looks like a stringified Go value
    (map[answer:20 minutes question:How long?]) — mddb stores meta as flat
    strings, so structured fields must be JSON-encoded by the producer
```

A value that is neither JSON nor a printed Go map reaches the template exactly
as it always has.

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
| `alias_stubs` | `true` | also write meta-refresh stub pages for `aliases:` (`false` = 301 only; per-page frontmatter `alias_stubs` overrides) |
| `headers` | empty | map of `path pattern → {header: value}` overrides |
| `headers_defaults_off` | `false` | drop the built-in security/cache blocks |

`redirects:` generates a real `_redirects` file: exact paths, `/old/*` splats
(`:splat` in the destination) and statuses `301`/`302`/`303`/`307`/`308`/`410`.

> ⚠️ **`410` is a Netlify extension.** Cloudflare Pages honours `301`, `302`,
> `303`, `307` and `308` only, and drops anything else without a word — so the
> path keeps answering `200` while the rule reads as handled. Building with
> `deploy: cloudflare` warns about this; serve a gone page from a Pages Function
> if you need one.
Frontmatter `aliases:` are added as `301`s and exact chains are flattened to a
single hop. By default each alias also gets a meta-refresh stub copy (a fallback
for hosts without server redirects); set `alias_stubs: false` — site-wide or per
page in frontmatter — to emit the `301` only, with no duplicate 200-serving copy.
`headers:` overrides or extends the generated `_headers` per pattern. Full
reference and the `ssg import redirects` importer: [DEPLOYMENT.md](DEPLOYMENT.md).

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

## AI content (build-time `[ai …]` shortcode)

Two layers configure build-time AI, then you ask questions from inside content
with the `[ai …]` shortcode:

- A **model** is an *endpoint* — where to reach the provider (url, key, provider
  model id) and the base generation params. It is the connection.
- An **agent** is a *role* built on a model — it runs on a model and layers a
  persona plus user-defined **rules** (constraints it must follow) and **skills**
  (jobs it applies) on top. It is the behaviour.

A shortcode invokes an **agent** (`agent="…"`, preferred) or a **bare model**
(`model="…"`). The answer is fetched **once, at build time**, and
content-addressed cached so a rebuild is deterministic and only re-queries when
the question or the effective request (model, prompt, rules, skills, params)
changes. Keys reference environment variables, never literals; the
request/response shape is OpenAI-compatible chat completions.

| Key | Notes |
|---|---|
| `ai.models.<name>.url` | Chat-completions endpoint |
| `ai.models.<name>.key` | Bearer token — use `$ENV_VAR` |
| `ai.models.<name>.model` | Provider model id |
| `ai.models.<name>.system` | Optional base system prompt |
| `ai.models.<name>.max_tokens` / `temperature` | Optional generation controls |
| `ai.agents.<name>.model` | Model this agent runs on (empty ⇒ default/sole model) |
| `ai.agents.<name>.system` | Persona, layered on the model's system prompt |
| `ai.agents.<name>.rules` | Constraints the agent must follow (folded into the prompt) |
| `ai.agents.<name>.skills` | Capabilities the agent applies (folded into the prompt) |
| `ai.agents.<name>.max_tokens` / `temperature` | Override the model when non-zero |
| `ai.default_agent` | Agent used when a shortcode names neither |
| `ai.default_model` | Model used when a shortcode names neither and no default agent |
| `ai.cache_dir` | Content-addressed answer cache (default `.ssg-cache/ai`; the pre-1.8.27 `.ai-cache` is still read and migrated by copy) |
| `ai.timeout` | Default per-query timeout (e.g. `30s`) |

```yaml
ai:
  default_agent: writer
  cache_dir: .ssg-cache/ai    # commit it for reproducible, key-free CI builds
  models:                     # endpoints — the connection
    fast:
      url: https://api.openai.com/v1/chat/completions
      key: $OPENAI_KEY
      model: gpt-4o-mini
      system: "Answer in one short paragraph."   # house style, inherited by agents
  agents:                     # roles — built on a model
    writer:
      model: fast             # runs on the "fast" model
      system: "You are the site's copy editor."
      rules:                       # constraints the agent must follow
        - "Answer in the page's language."
        - "Never invent facts or links."
      skills:                      # jobs the agent is set up for
        - "Summarise long text into one sentence."
        - "Write concise meta descriptions."
```

The effective system prompt for an agent is its model's `system`, then the
agent's `system`, then its `rules`, then its `skills` — all composed and folded
into the cache key, so editing any of them re-queries. Define an agent once and
every `[ai agent="writer" …]` inherits its role; a bare `[ai model="fast" …]`
uses only the model's own settings.

In content:

```markdown
[ai agent="writer" question="Summarise the 1.8 release line in one sentence."
   ifs="lang == en AND status == publish" timeout="20s" fallback="_summary unavailable_"]
```

- Precedence when resolving a shortcode: an explicit `agent`, then an explicit
  `model`, then `ai.default_agent`, then `ai.default_model`, then a sole agent,
  then a sole model.
- `ifs` is an optional guard evaluated against the page's fields (`lang`,
  `status`, `type`, `category`, `series`, `slug`, `title`, `tags`, any custom
  frontmatter, and site `variables`). It supports `AND`/`OR` and the operators
  `==`, `!=`, `contains`, `>`, `<`, `>=`, `<=`. When it is false — or the query
  fails, or nothing answers — the `fallback` text is used.
- Because answers are cached by the effective request, committing `cache_dir`
  lets CI rebuild the exact same content with no API key and no network.

## Notifications (announce new posts)

Send each newly published — or changed — post to webhook destinations you define:
point them at a platform API, an automation service (Zapier / Make / n8n / IFTTT)
or your own endpoint, and they receive the post as JSON. A committed state file
dedupes, so a post is announced **once**, again only when its content changes. It
never fires unless you pass `--notify`, so local dev builds stay quiet.

| Key | Notes |
|---|---|
| `notifications[].url` | Destination the post JSON is POSTed to |
| `notifications[].name` | Label used in build logs |
| `notifications[].method` | HTTP method (default `POST`) |
| `notifications[].headers` | Extra headers (auth) — use `$ENV_VAR` for secrets |
| `notifications[].allow_private` | Permit a private/loopback destination |
| `notify_state` | Dedup state file (default `.ssg-notifications.json`) |
| `notify` / `--notify` | Actually send this build (off by default) |

```yaml
notify_state: .ssg-notifications.json   # commit it — CI needs the sent-history
notifications:
  - name: zapier
    url: https://hooks.zapier.com/hooks/catch/…   # fans out to X / LinkedIn / …
    headers: { X-Token: $ZAP_TOKEN }
```

```bash
ssg --config .ssg.yaml --notify --deploy cloudflare   # announce on publish
```

The payload is `{slug, title, url, excerpt, date, tags}`. The dedup key is a hash
of the post's title, body and date, so an edit re-announces it and an untouched
post is skipped. A destination that fails is retried on the next `--notify` run.
The transport refuses private/loopback ranges at dial time unless
`allow_private` is set, so a webhook URL can't be turned into an SSRF pivot.

## Development MCP server (`ssg mcp`)

> Full reference — roles, every tool with its CAN/CANNOT contract, and the
> git write-back flow — is in [MCP.md](MCP.md). This section covers the
> configuration block.

`ssg mcp` runs a Model Context Protocol server over stdio so an AI assistant can
work on the site during development in two clearly-scoped roles:

- **Designer** (`designer_*`) — changes how the site *looks*: lists, reads and
  writes templates, partials, CSS and theme assets. It cannot touch content,
  delete files, or write outside the template/static directories. It **also owns
  the presentation settings in the config file** — see below.
- **Content manager** (`content_*`) — changes what the site *says*: lists, reads,
  creates, updates and deletes Markdown (frontmatter + body). It cannot touch
  templates or write non-Markdown files.

Every tool description tells the model exactly what it **can** and **cannot** do,
an always-present `help` tool restates the whole contract, and the same guidance
is handed to the client at connect time. By default every successful change
triggers a rebuild — a template or content error comes straight back to the
model as the tool result, so it fixes its own mistakes before moving on.

```bash
ssg mcp                    # both roles, rebuild after every change
ssg mcp --role=designer    # designer only
ssg mcp --role=content     # content manager only
ssg mcp --no-watch         # edit only, no rebuilds
```

Register it in an MCP-capable assistant as a stdio server:

```json
{ "command": "ssg", "args": ["mcp", "--config", ".ssg.yaml"] }
```

### Designer-owned configuration keys

Presentation does not live in templates alone — the theme, the syntax-highlight
style, whether diagrams render. So the designer gets `designer_config_read` and
`designer_config_set` over a **narrow allow-list** of presentation settings:

`template`, `templates_dir`, `static_dir`, `mermaid`, `mermaid_theme`,
`mermaid_background`, `highlight`, `highlight_style`, `highlight_line_numbers`,
`math`, `toc`, `toc_depth`, `minify_html`, `minify_css`, `minify_js`,
`minify_all`, `pretty_html`, `sourcemap`, `fingerprint`, `paginate`, `webp`,
`webp_quality`, `image_sizes_attr`.

Every other key is refused by construction — secrets (API keys, tokens,
`jwt_secret`, auth), deployment, server, endpoints, hooks, `sass_binary` (an
executable path) and all content/URL structure. `designer_config_read` shows only
the writable keys, so the rest of the file is never even surfaced to the model.

Three properties make this safe to hand over:

- **Comments and key order survive.** The file is edited as a YAML document, not
  re-serialised, so your annotated config stays annotated.
- **Invalid changes roll back.** After each write the config is re-loaded; if it
  no longer loads, the previous file is restored and the model is told why.
- **Changes apply immediately.** Since the watcher treats the config as a watched
  input, a `designer_config_set` in watch mode reloads and rebuilds at once.

The tools appear only when a config file is in play; without one, there is
nothing to edit and they are not exposed.

### Git write-back (optional)

With a git account and token configured, the assistant additionally gets a safe
write-back flow: `git_new_branch` → edit → `git_commit` → **human reviews** →
`git_open_pr`. Edits never land on the base branch, commits stage only the
content/template directories, and the pull request is opened only after the
person explicitly approves. The token must reference an environment variable,
never a literal.

| Key | Notes |
|---|---|
| `mcp.git.account` | Git account/owner the PR is attributed to |
| `mcp.git.token` | API token for opening PRs — use `$ENV_VAR` (e.g. `$GITHUB_TOKEN`) |
| `mcp.git.repo` | `owner/name`; empty = derived from the remote URL |
| `mcp.git.remote` | Remote to push to (default `origin`) |
| `mcp.git.default_branch` | PR base branch (default `main`) |
| `mcp.git.branch_prefix` | Working-branch prefix (default `mcp/`) |

```yaml
mcp:
  git:
    account: spagu
    token: $GITHUB_TOKEN        # never a literal
    default_branch: main
    branch_prefix: mcp/
```

Without `mcp.git.token`, the `git_*` tools are simply not exposed — the assistant
edits files in place and version control stays fully manual.

## Server endpoints (portable, no vendor lock-in)

Some sites need a little server behind the static output — a redirect that
depends on the request, or a proxy that keeps an upstream key server-side.
`endpoints:` declares those **once, in a vendor-neutral way**. The built-in
server runs them natively (`--http`), in the single Go binary, with no external
runtime — so a self-hosted deploy behind nginx/Caddy or the Docker image gets the
dynamic bits for free. Empty `endpoints:` ⇒ a pure-static build, unchanged.

| Key | Notes |
|---|---|
| `path` | Request path handled by this endpoint, e.g. `/api/quote` (exact match) |
| `type` | `redirect`, `proxy` or `form` |
| `to` / `status` | `redirect`: destination and 3xx code (default `302`) |
| `target` | `proxy`: upstream URL; the client's path is replaced by the target's |
| `methods` | `proxy`: allowed HTTP methods (empty = any) |
| `to` | `form`: the webhook the submission is POSTed to as JSON |
| `fields` | `form`: which fields to forward (empty = all submitted fields) |
| `honeypot` | `form`: a field that must stay empty — a filled one is a bot, silently dropped |
| `redirect` | `form`: where the browser goes after a successful submit (`303`); empty = a small JSON ok |
| `allow_private` | `proxy`/`form`: permit a private/loopback upstream or webhook (a self-hosted service) |
| `user` / `password` | `auth`: Basic-auth credentials; `password` should reference an env var (`$MEMBERS_PW`), never a literal |

```yaml
endpoints:
  - path: /go/latest
    type: redirect
    to: /releases/1-8-14/
    status: 302
  - path: /api/quote
    type: proxy
    target: https://api.example.com/v1/quote   # upstream key stays server-side
    methods: [GET, POST]
  - path: /api/contact
    type: form
    to: https://hooks.example.com/email        # delivery webhook stays server-side
    fields: [name, email, message]
    honeypot: company                          # bots that fill it are dropped
    redirect: /thanks/
```

A `form` endpoint accepts a `POST`ed submission, drops obvious bots via the
`honeypot` (a hidden field a human leaves empty), and delivers the collected
fields as JSON to `to` — so the delivery webhook (an email service, a chat hook)
is never exposed to the browser. On the self-hosted server the delivery uses the
same dial-time SSRF guard as `proxy`.

An `auth` endpoint guards its `path` **as a prefix** with HTTP Basic auth:

```yaml
endpoints:
  - path: /members/          # protects /members/ and everything under it
    type: auth
    user: ada
    password: $MEMBERS_PW    # from the environment, never a literal
```

The password is read from the named environment variable; the comparison is
constant-time. Auth guards run on the **built-in server only** — on a serverless
platform, protect a section with that platform's own access control — so
`endpoints_platform` compiles the other endpoint types and skips `auth`.

A `proxy` endpoint resolves and vets the upstream IP itself and **refuses
loopback/private ranges at dial time** — the same SSRF / DNS-rebinding guard the
external-source client uses — so it can't be turned into a pivot to internal
hosts. Set `allow_private: true` only when the upstream really is a private
self-hosted API. Endpoint responses are sent `Cache-Control: no-store`.

**Same declaration, any target.** The built-in server runs endpoints directly.
To run the *same* endpoints on a serverless platform instead, set
`endpoints_platform` and the build compiles them into that platform's functions —
no rewrite, no second definition:

| `endpoints_platform` | Emits |
|---|---|
| _(empty)_ | Self-hosted only — served natively by `--http` |
| `cloudflare` | `functions/<path>.js` Pages Functions (the same tree hand-written workers use) |
| `netlify` | `netlify/functions/<name>.mjs` (v2, each declares its own `path` — no `_redirects` wiring) |
| `vercel` | `api/<name>.js` Edge Functions + a `vercel.json` that rewrites each path to its function |

```yaml
endpoints_platform: cloudflare   # compile endpoints: into functions/ at build time
```

Adapters are self-contained plugins — one file per platform — so a new target
drops in without touching the format or your config. Redirect and proxy behave
the same on every target; the proxy's dial-time SSRF guard is specific to the
self-hosted server (on a platform the upstream runs at the edge).

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
