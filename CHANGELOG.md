# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.8.3] - 2026-07-14

Template query language, SCSS, accessibility and a performance batch
(PERF-004/005/007/008). All new features are opt-in; performance changes keep
output byte-equivalent for generated pages.

### Added
- ✨ **Template collection & conditional helpers** — Go templates can now query
  content in pipelines (collection is always the last argument):
  `where` `filter` (eq/ne/gt/ge/lt/le/contains/notContains/in/notIn) `sort`
  `first` `last` `limit` `offset` `groupBy` `uniq` `uniqBy` `reverse` `slice`
  `pluck` `indexBy`; conditionals `in` `notIn` `contains` `startsWith`
  `endsWith` `matches` (cached RE2) `isNil` `isEmpty` `ternary`; content
  wrappers `latest` `published` `byTag` `byCategory` `byAuthor` `related`.
  Generic over structs/pointers/maps via reflection, never mutate input, never
  panic — invalid usage fails the render with a descriptive error. Safe subset
  also exposed to shortcode templates. Note: registering `slice` overrides Go's
  builtin sub-slicing. Full reference: `docs/TEMPLATE_HELPERS.md`.
- 🎨 **SCSS/Sass compilation (ASSET-003)** — `--scss` / `scss: true` compiles
  `*.scss` → `*.css` via the optional dart-sass CLI before bundling/minify
  (partials `_*.scss` resolve via `@use`; all `.scss` sources are removed from
  the output). Missing binary skips the step with a warning (cwebp philosophy);
  `--sass-binary=` overrides PATH lookup; paths hardened per SEC-011.
- ♿ **Skip-links (FE-004, WCAG 2.2 2.4.1)** — every theme (krowy, simple, imd,
  engine examples, ananke, embedded defaults) gains a visually-hidden
  "Skip to content" link before the navigation plus `:focus-visible` outlines.

### Performance
- ⚡ **Markdown render cache (PERF-004)** — each unique markdown body is
  converted by goldmark exactly once per build; feeds, search index, JSON
  output and both page-format paths reuse the memo (verified by a
  conversion-counter test).
- ⚡ **Single-write HTML pipeline (PERF-005)** — SEO block, KaTeX injection,
  relative links, prettify and HTML minification are applied in memory at
  render time, so each page is written once instead of being re-read/re-written
  by up to 8 tree-walks. Only genuinely global passes remain (bundling, CSS/JS
  minify, fingerprint, link check). Behaviour note: HTML copied verbatim from
  `static/` is no longer post-processed (matching its documented contract).
- ⚡ **Co-located assets only where referenced (PERF-007)** — a post's category
  directory assets are copied only into posts that actually reference them by
  filename, eliminating O(posts × assets) duplication and output-dir bloat.
- ⚡ **Watch-mode signature cache (PERF-008)** — the content signature streams
  file hashes (no whole-file loads) and caches them per path keyed by
  size+mtime, so a change event re-hashes only what changed; touch-only events
  still skip rebuilds (PLAT-006 semantics preserved).

## [1.8.2] - 2026-07-11

### Changed
- ⚠️ **SEO injection is now opt-in (`--seo` / `seo: true`)** — the generator-level
  OpenGraph/Twitter/JSON-LD partial is **off by default**, so `ssg` never rewrites your
  rendered `<head>` unless you ask. This aligns SEO with the project's opt-in philosophy
  (it *modifies* your HTML, unlike sitemap/robots which write separate files). **Behaviour
  change:** sites that relied on automatic OG tags must now pass `--seo`. The legacy
  `--seo-off` flag and `seo_off` config key are still accepted as deprecated no-ops.

### Docs
- 📚 **Greatly expanded README** for both humans and AI agents: a new "Project & Content
  Structure" section (annotated directory tree, `pages/` vs `posts/<subfolder>/` rules,
  `metadata.json` shape, minimal end-to-end example), a complete **Frontmatter Reference**
  table, richer argument/path-resolution docs, and a "Common Recipes (task → command)"
  cheat-sheet.

## [1.8.1] - 2026-07-10

Server-hardening and packaging release. The built-in server gains optional public-facing
capabilities (TLS, HTTP/2, HTTP/3, compression, limits); the build gains extra archive
formats. Every addition is opt-in; default behaviour (plain HTTP dev server, ZIP) is unchanged.

### Added
- ✨ **Optional server TLS** — `--tls-cert=`/`--tls-key=` (manual PEM) or `--tls-auto` +
  `--tls-domain=` (automatic Let's Encrypt via `autocert`). HTTP/2 is negotiated
  automatically over TLS (ALPN).
- ✨ **HTTP/3 (QUIC)** — `--http3` serves HTTP/3 alongside HTTP/2 and advertises it via
  `Alt-Svc` (requires TLS; `github.com/quic-go/quic-go/http3`).
- ✨ **Server hardening middlewares** — `--gzip` (content compression), security headers
  (`X-Content-Type-Options`, `X-Frame-Options`, `Referrer-Policy`, HSTS under TLS),
  cache-control (immutable for fingerprinted assets, `no-cache` for HTML), `--max-conns=N`
  (connection cap via `netutil.LimitListener`), `--mem-limit=SIZE` (runtime GC soft limit).
- ✨ **tar.gz / tar.xz archive output** — `--targz` and `--tarxz` alongside `--zip`
  (`archive/tar` + `compress/gzip`; `github.com/ulikunitz/xz`).
- ✨ **HTML sanitization (FE-005)** — `--sanitize-html` / `sanitize_html: true` runs raw
  HTML in markdown through the bluemonday UGC policy.
- ✨ **Timezone-aware dates (I18N-001)** — `timezone: Europe/Warsaw` / `--timezone=` renders
  content dates (permalink `:year/:month/:day` tokens, `Date`/`Modified` template context)
  in an IANA zone; `language_timezones:` overrides it per content language. The IANA db is
  embedded (`time/tzdata`) so static/Windows builds resolve zones. Empty = previous
  behaviour (no conversion).
- 🚀 **Native deploy (`--deploy=`)** — SSG publishes the output tree itself, no external
  CLI. Providers: **Cloudflare Pages** (Direct Upload API — blake3 manifest, upload only
  what changed), **GitHub Pages** (force-push to `gh-pages`), **Netlify** (digest deploy
  API), **Vercel** (files + deployments API), **FTP**, and **SFTP/SSH** (host-key verified
  against `known_hosts`). Flags `--deploy-project`/`--deploy-branch`/`--deploy-target`; all
  secrets come from the environment, never the config file. Runs after build + webp/zip.
- 🧱 **ARM improvements** — `linux/arm/v7` (GOARM=7) release binary + Docker platform;
  multi-arch cross-compile via buildx `TARGETARCH`/`TARGETVARIANT`.
- 🔤 **Template engines documented as shipping** — README/CLI now correctly list pongo2,
  mustache and handlebars as supported (they render the theme's own templates; GO-007).

### Changed
- ♻️ **Flag parsing refactor** — boolean and simple string `--flag=value` options are now
  table-driven; the value switch is split into focused helpers (resolves SonarCloud
  S1479/S3776/S1192, keeps each function under the complexity budget).
- ♻️ **`build()` split** into `runWebP` / `runArchives` / `runDeploy` helpers.

### Fixed
- 🔧 **OPS-009** — homebrew tap push uses an `http.extraheader` auth header instead of
  embedding the token in the remote URL.
- 🔧 **OPS-011** — CI/Docker workflows add a `concurrency:` group (cancel in-progress for
  branches, never for tags).
- 🔧 **OPS-013** — pinned tool versions (golangci-lint v2.12.2, govulncheck v1.3.0).
- 🔧 **FE-002** — theme muted-text colours raised to WCAG 2.2 AA (`krowy` 5.72:1,
  `simple` 5.65:1).
- 🔧 **FE-006 / FE-008** — OpenGraph/meta locale corrected to `en_US` / `en-US`; schema
  description de-hardcoded to `{{.Domain}}`.
- 🔒 **SonarCloud S5445** — the autocert cache (Let's Encrypt private keys) no longer falls
  back to the shared, world-predictable system temp dir; it uses per-user cache/home paths.
- 🔒 **SEC-014** — `--sanitize-html` now holds on every render path: alt engines
  (pongo2/mustache/handlebars), full-content feeds and raw `{{.Content}}` (plain string →
  auto-escape when the sanitizer is on). Trusted shortcode output ([youtube]/[embed],
  custom shortcodes) survives sanitization via token protection (GO-037); hostile iframes
  in content do not.
- 🔒 **SEC-015** — generator SEO meta tags HTML-escape attribute values (Go `%q` allowed
  attribute injection through titles/descriptions).
- 🔧 **GO-033** — `Alt-Svc` (HTTP/3 advertisement) is built from the configured port instead
  of quic-go's `SetQUICHeaders` (which needs a live listener); present from the first TCP
  response; `TestAltSvcMiddleware` green again.
- 🔧 **GO-012/019/020/034** — server: `--gzip` no longer corrupts Range requests;
  `--max-conns` enforced in `--tls-auto` mode too; `--tls-domain=a.com,b.com` split into a
  proper autocert whitelist; autocert `:80` bind failures logged; IPv6 `--host` handled via
  `net.JoinHostPort`.
- 🔧 **GO-013/014/015/030/031/041 (mddb)** — `--mddb-lang` actually filters (HTTP body +
  client-side; gRPC proto has no lang field → client-side); single-element
  tags/categories/aliases no longer dropped; pagination survives a missing/malformed
  `X-Total-Count` and server-clamped page sizes; gRPC string IDs normalized (`asInt`);
  `AddedAt==0` no longer becomes 1970-01-01 and dates are pinned UTC (reproducible URLs);
  checksum query URL-escaped.
- 🔧 **GO-016/017/032/038 (webp)** — uppercase extensions (`Photo.JPG`) convert correctly;
  originals deleted only when the .webp exists; reference rewriting is scoped to local
  attribute/`url()` refs with existing targets (CDN URLs and prose untouched, `.HTML`/`.CSS`
  processed); srcset includes the full-size original (RIFF-header width parser, no new
  deps); `data-src` and self-closing `<img/>` are safe.
- 🔧 **GO-021/022/023/037 (generator)** — feed summaries truncate by runes (valid UTF-8);
  `--minify-html` preserves `<pre>/<textarea>/<script>/<style>`; a post whose `link` has no
  path no longer overwrites the homepage; `--sanitize-html` no longer deletes video embeds.
- 🔧 **GO-024/025/035/036/018/046 (CLI)** — ZIP/tar output `Close` errors propagate (no more
  corrupt archives reported as success); watch mode no longer loses edits made during a
  rebuild; symlinks archive correctly as symlink entries; space-separated flag values are
  not miscounted as positional args; `--mddb-watch` (boolean form) works; vacuous
  `handleConfigSkip` removed.
- 🔧 **GO-026/027/039 (parser)** — frontmatter delimiter tolerant of trailing spaces/CRLF;
  code-fence tracking (no more eaten `# comment` lines or hijacked `## Content-…` headings);
  10 MB line buffer (base64 data-URIs parse); unclosed frontmatter is a clear error, not a
  silent empty page.
- 🔧 **GO-028/029/040 (themes)** — `.tar.gz` theme URLs rejected up-front with a clear
  message; zip prefix stripped only when truly common to all entries (no more flattened
  layouts); `main`→`master` branch fallback for GitHub/GitLab archives; extraction `Close`
  errors propagate.
- 🧹 **GO-042/043** — dead code removed: `mddb.ErrorResponse`, `models.Metadata.ExportedAt`,
  unread `generator.Config` copies (`ImageSizes*`, `Mddb.Watch*`).

### Performance
- ⚡ **PERF-001** — `--lastmod-from-git` runs one `git log --name-only` scan (path→date map)
  instead of one `git log` process per page/feed entry (minutes saved at 1k+ posts).
- ⚡ **PERF-002** — shortcode templates are parsed once per build and cached (previously
  stat+read+parse per occurrence per page).
- ⚡ **PERF-003** — fingerprint reference rewriting precompiles its regexes once per walk
  (was O(pages × assets) compiles + rescans).
- ⚡ **PERF-006** — ~25 hot-path regexes hoisted to package level; `fixMediaPaths` rewrites
  WordPress image URLs in a single pass (was a fresh regex + full-document rescan per image).
- ⚡ **PERF-009/010/011** — link-checker target memoization; mddb metadata fetched with the
  configured batch size (was hardcoded 100 → 10× fewer round trips); srcset variant stats
  and width decodes memoized per build.

### Docs
- 📚 **DOC-001** — `docs/STYLES.md` documents theme palettes with contrast ratios.
- 📚 **DOC-006** — `SECURITY.md` Supported Versions refreshed to the 1.8.x line.

### Testing
- ✅ Coverage raised on the packages below 96%: `cmd/ssg` 65→80%, `internal/webp` 92→96.5%,
  `internal/generator` 89→91.7%, `internal/theme` 94.8→95.5%. Added server, archive, mddb
  (mock-server), sanitizer and WebP responsive-variant tests.
- ✅ New `internal/deploy` package tested with mock HTTP servers (Cloudflare/Netlify/Vercel),
  a local bare-repo git push (GitHub Pages), manifest/hash and URL/credential unit tests.

## [1.8.0] - 2026-07-10

Feature release from the post-1.7.x roadmap (`audit/roadmap/`) plus audit fixes. Every new
feature is opt-in behind a config flag; default behaviour is unchanged.

### Added
- ✨ **Configurable permalinks (SEO-001)** — `permalinks:` per content type with tokens
  `:year :month :day :slug :category` (e.g. `/:year/:month/:slug/`); flags
  `--permalink-post=` / `--permalink-page=`. Empty = current date/slug behaviour.
- ✨ **Frontmatter aliases (SEO-002)** — `aliases: [/old/path/]` emits meta-refresh +
  canonical + `noindex` redirect stubs, excluded from the sitemap; collisions are skipped.
- ✨ **`--lastmod-from-git` (SEO-004)** — sitemap `<lastmod>` from each source file's last
  git commit, with graceful fallback outside git or for mddb content.
- ✨ **Reading time / word count (BLOG-006)** — `.WordCount` and `.ReadingTime` exposed to
  all engines (markup stripped; 200 wpm, rounded up).
- ✨ **Pagination (BLOG-003)** — `paginate: N` / `--paginate=N` splits the index into
  `/page/N/` and adds a `.Pager` (Current/Total/PerPage/PrevURL/NextURL). `0` = disabled.
- ✨ **Working source maps (BLOG-007 / GO-004)** — `--sourcemap` now truly emits v3
  `*.js.map` / `*.css.map` (line-preserving minification → exact mappings); the flag is no
  longer a no-op.
- ✨ **Asset fingerprinting (ASSET-001)** — `fingerprint: true` / `--fingerprint`:
  sha256 → `name.<hash8>.ext`, `assets-manifest.json`, reference rewrite in HTML and
  CSS (`url()`/`@import`), deterministic across builds. Terminal asset step.
- ✨ **Responsive images (ASSET-004)** — `image_sizes: [480,960,1600]` emits WebP variants
  (no upscaling) and `<img srcset>`/`sizes`; `--image-sizes=` / `--image-sizes-attr=`.
- ✨ **Math rendering (AX-004)** — `math: true` / `--math` detects `$$…$$` / ```` ```math ````
  and injects KaTeX only on pages that use it (`.HasMath` exposed).
- ✨ **Series (AX-005)** — `series:` frontmatter → `/series/{slug}/` landing pages
  (`series.html`, fallback `category.html`) and `.SeriesPrev*/.SeriesNext*` navigation.
- ✨ **Data files (PLAT-002)** — `data/*.yaml|*.json` loaded into `.Data.*` (nested by
  subdirectory); `data_dir:` / `--data-dir=`.
- ✨ **Build hooks (PLAT-001)** — `hooks:` `pre_build` / `post_build` / `post_page` exec
  hooks (argv-split, no shell, 60 s timeout, trusted local config only), context via env
  `SSG_OUTPUT_DIR` / `SSG_PHASE` / `SSG_PAGE_PATH`.
- ✨ **i18n / multilingual (PLAT-005)** — `languages:` + `default_language:` produce
  language-prefixed output (`/en/…`) with `.Translations`, `.Hreflang`, `.Languages`
  context and `hreflang`/`x-default` alternates.
- ✨ **Incremental watch (PLAT-006)** — `--watch` now gates rebuilds on a content
  signature, skipping touch-only (mtime-but-not-bytes) events; any real change still
  triggers a full, correct rebuild.
- ✨ **Single source of version truth (DOC-005)** — `VERSION` file + `scripts/sync-version.sh`
  (`--check`) + Makefile `-X main.Version`; the version propagates into every packaging
  manifest (FreeBSD/OpenBSD/deb/rpm/brew/install.sh).
- ✨ **Collection renderer + archives (BLOG-001/004/005)** — shared archive renderer powers
  `/tag/{slug}/` and `/author/{slug}/` listings (`tag.html`/`author.html`, fallback
  `category.html`), included in the sitemap.
- ✨ **Atom feeds (BLOG-002)** — `feed: true` writes `feed.xml` at the root and per
  category/tag; `feed_items` / `feed_full_content`. Closes the FE-010 feed gap.
- ✨ **Generator SEO partial (SEO-003)** — OpenGraph + Twitter Card + JSON-LD (Article/WebSite)
  injected into pages lacking their own OG tags, plus feed + hreflang links; `seo_off` opts out.
- ✨ **Internal link checker (SEO-005)** — `--check-links[=warn|strict]` validates internal
  href/src against the output tree (no network); strict fails the build.
- ✨ **Syntax highlighting (AX-001)** — `highlight: true` renders code blocks via Chroma;
  `highlight_style`.
- ✨ **Table of contents (AX-002)** — `toc: true` exposes `.TOC`; `[toc]` expands inline;
  `toc_depth`; anchors use goldmark auto heading IDs.
- ✨ **Footnotes (AX-003)** — goldmark footnote syntax (`[^1]`) is enabled by default.
- ✨ **Asset bundling (ASSET-002)** — `bundles:` concatenates CSS/JS groups before
  minify/fingerprint.
- ✨ **Output formats & search (PLAT-003/PLAT-004)** — `outputs: [html, json]` writes a
  per-page `index.json`; `search_index: true` writes `search-index.json` for client-side search.
- ✨ **Alternate template engines (GO-007)** — `--engine=pongo2|mustache|handlebars` now
  render for real; themes must be authored in that engine's syntax.

### Security
- 🔒 **mddb API key not sent over plaintext (SEC-007)** — the HTTP client refuses to attach
  `Authorization: Bearer` over `http://` to a non-loopback host (https:// / loopback allowed).
- 🔒 **gRPC transport security (SEC-004)** — the gRPC client selects TLS from the scheme
  (`grpcs://`/`https://` → TLS; `grpc://`/`http://` → insecure; bare host → TLS unless
  loopback) and refuses to send an API key over an insecure channel to a non-loopback host.

### Fixed
- 🐛 **No-frontmatter files no longer silently dropped (GO-009)** — a `.md` file without an
  opening `---` is treated as published content instead of yielding empty output.
- 🐛 **`datetime` attribute leading space (FE-009)** — `<time datetime>` in the krowy/imd
  themes no longer emits `datetime=" 2026-…"` (invalid machine date).
- 🐛 **Hugo theme conversion wired (GO-010)** — `--online-theme` now converts a downloaded
  Hugo theme's `layouts/`+`static/`+`assets/` into the SSG layout; dead `ToMetadata` removed.
- 🐛 **Dead/broken `base.html` removed (FE-007)** — the unused krowy/simple `base.html` (with
  invalid `{{template " description"}}` names) are gone.

### Privacy / DevOps / Docs
- 🔏 **No Google Fonts CDN (FE-003)** — first-party themes drop external font requests and
  use a system font stack (no visitor IP leak to Google).
- 🐳 **Container hardening** — `docker-compose.yml` gains log caps, healthchecks and
  resource limits/reservations via a YAML anchor (OPS-003); the Dockerfile gains a
  `HEALTHCHECK` (OPS-004); every CI job gets `timeout-minutes` (OPS-007).
- 📚 **Docs/Makefile** — README deb/rpm versions and INSTALL.md artifact links corrected and
  made version-resilient (DOC-002/DOC-004); complete `.PHONY` and demo targets on
  `test-content` (DOC-007/DOC-008); CHANGELOG compare links (DOC-011); `make security`
  target running gosec + govulncheck (DOC-012).

### Removed
- 🧹 **`LICENSE.md` duplication (DOC-010)** — `LICENSE.md` is now a pointer to the canonical
  `LICENSE` (BSD-3-Clause).

## [1.7.15] - 2026-07-09

Audit hardening round: 5 security + 3 correctness fixes from the local audit backlog.

### Security
- 🔒 **Decompression-bomb total limit (SEC-006)** — theme extraction now enforces a
  cumulative size cap (500 MB), a per-file cap (100 MB) and an entry-count cap (10 000)
  in addition to bounding the download itself, so a malicious archive can no longer
  exhaust disk/memory.
- 🔒 **Theme download timeout & redirect cap (SEC-008)** — `theme.Download` uses a bounded
  `http.Client` (30 s timeout, ≤5 redirects) instead of `http.DefaultClient`, preventing
  hangs and redirect-loop SSRF-lite.
- 🔒 **Bounded mddb response reads (SEC-009)** — every mddb HTTP body is wrapped in an
  `io.LimitReader` (64 MB payloads, 64 KB error bodies) so a hostile/broken server cannot
  exhaust memory via `io.ReadAll`/streaming decode.
- 🔒 **Archive file permissions clamped (SEC-010)** — extracted files/dirs use fixed safe
  modes (`0644`/`0755`) instead of trusting `f.Mode()` from the archive.
- 🔒 **Dev server binds loopback by default (SEC-012)** — the built-in server now listens on
  `127.0.0.1` instead of `0.0.0.0`; exposing on all interfaces requires an explicit
  `--host=0.0.0.0` (new `--host` flag / `host:` config, default `127.0.0.1`).

### Fixed
- 🐛 **`sitemap: no` honored for file content (GO-003)** — the `sitemap` frontmatter field
  is now parsed for file-based pages (previously only mddb set it), so `sitemap: no`
  correctly excludes a page from `sitemap.xml`.
- 🐛 **`--sourcemap` is no longer a silent no-op (GO-004)** — the flag now prints a clear
  "not yet implemented" notice and the help text is truthful.
- 🐛 **`recentPosts` negative-count panic fixed (GO-008)** — `{{recentPosts -1}}` no longer
  panics with slice-bounds-out-of-range; the count is clamped at both ends.

## [1.7.14] - 2026-07-08

### Security
- 🔒 **Go toolchain bumped to 1.26.5 (GO-2026-5856)** — go1.26.4's `crypto/tls`
  is affected by an Encrypted Client Hello privacy leak (reachable via the dev
  server, mddb client, and theme downloader). Pinned `GO_VERSION` and the
  Dockerfile builder image to 1.26.5, where it is fixed. `govulncheck` is clean.
- 🔒 **Path traversal / arbitrary write via slug/link hardened (SEC-001)** — output
  sub-paths derived from `slug`/`link` (fully controlled by a remote `mddb` server) are
  now sanitized (`models.SanitizeRelPath`), and every page/post/category write is verified
  to stay within the output directory (`ensureWithinOutput`). Malicious values such as
  `../../../etc/...` can no longer escape the output directory.
- 🔒 **Script injection in the GitHub composite action closed (SEC-002)** — `action.yml`
  no longer interpolates `${{ inputs.* }}` inside `run:` blocks. All inputs are passed via
  `env:` and referenced as quoted shell variables; build flags are assembled as a bash
  array; `version`/`webp-quality`/`engine` are validated. Prevents RCE on the runner.
- 🔒 **CI/CD supply-chain hardening (OpenSSF Scorecard)** — resolves the open code-scanning
  alerts:
  - **Token-Permissions** — added least-privilege top-level `permissions: contents: read`
    to every workflow that lacked one (`ci.yml`, `docker.yml`, `snap.yml`, `test-action.yml`);
    jobs that need more (release, GHCR push) elevate locally.
  - **Pinned-Dependencies** — every third-party GitHub Action is now pinned to a full commit
    SHA with a `# vX` comment (Dependabot still updates them), across all six workflows.
  - **Binary-Artifacts** — removed the 21 MB compiled `ssg` binary that was committed to the
    repository and added `/ssg`, `/ssg-*` to `.gitignore` and `.dockerignore`.
- 🔒 **Module toolchain floor raised to go1.26.5** — `go.mod`'s `go` directive is now
  `1.26.5`, so any build (not just CI/Docker) uses the toolchain where GO-2026-5856
  (`crypto/tls` ECH leak) and GO-2026-4970 (`os`) are fixed. `govulncheck ./...` is clean.
- 🔒 **cwebp argument-injection hardened (SEC-011)** — image paths passed to the `cwebp`
  binary are now prefixed with `./` when relative, so a file named like `-o.png` can no
  longer be interpreted as a `cwebp` flag.

### Added
- ✨ **`static/` passthrough directory (`--static-dir`, `static_dir:`)** — a project-level
  static directory is now copied verbatim into the output during generation.

### Fixed
- 🐛 **Panic in `fixMediaPaths` on empty media file (GO-001)** — an empty
  `MediaDetails.File` previously caused `filename[:len-4]` to panic (slice bounds out of
  range) and crash the whole build. The filename is now trimmed with `filepath.Ext` and
  empty names are skipped safely.
- 🐛 **mddb media details were dropped (GO-006)** — `extractMediaFromDoc` now populates
  `MediaDetails.file/width/height`, so mddb-sourced media has correct paths (this was the
  root cause of GO-001).
- 🐛 **`--engine` flag no longer silently ignored (GO-002)** — only the Go
  (`html/template`) engine is wired into rendering. Requesting `pongo2`/`mustache`/
  `handlebars` now fails fast with a clear "not yet implemented" error instead of silently
  rendering with Go. Help text and the action input description updated accordingly.
- 🐛 **gRPC connection leak in watch mode fixed (GO-005)** — `MddbClient` now exposes
  `Close()` (HTTP no-op, gRPC closes the connection) and `loadContentFromMddb` defers it.
  A fresh client is created on every `Generate()`, so `--mddb-watch` rebuilds no longer
  leak `*grpc.ClientConn` connections and goroutines.
- 🐛 **All `static/` files and subdirectories now reach the output (#8)** — previously only a
  fixed subset was emitted, so directories like `downloads/`, `assets/`, `scripts/`, `styles/`
  and files like `manifest.json` were silently dropped. The generator now copies the entire
  `static/` tree (configurable via `--static-dir` / `static_dir:`, default `static`) verbatim
  to the output. A missing directory is a no-op, so existing sites are unaffected.

## [1.7.13] - 2026-04-08

### Fixed
- 🐛 **Shortcode templates now have FuncMap** — `safeHTML`, `decodeHTML`, `getCategoryName`, `getAuthorName`, and other template functions are now available in shortcode templates (fixes #11)
  - `{{.InnerContent | safeHTML}}` works correctly — HTML is no longer auto-escaped
  - All standard template functions available: `formatDate`, `formatDatePL`, `stripHTML`, `default`, `dict`, etc.

## [1.7.12] - 2026-04-08

### Added
- ✨ **Bracket shortcodes with attributes and closing tags** - WordPress-style shortcode syntax (requires `shortcode_brackets: true`)
  - `[name attr="val"]` — self-closing with inline attributes, available as `{{.Attrs.key}}` in template
  - `[name]content[/name]` — closing tag with inner content, available as `{{.InnerContent}}` in template
  - `[name attr="val"]content[/name]` — combined attributes and inner content
  - Config-defined fields (Title, Text, Url, etc.) remain available alongside inline attrs
  - Unknown shortcodes are left untouched (no silent removal)

## [1.7.11] - 2026-04-06

### Added
- ✨ **Flexible author and category fields** - Frontmatter `author` and `categories` now accept both integer IDs and string values
  - `author: 3` (int ID) — works as before
  - `author: "Jan Kowalski"` (name) — resolved to ID via author name lookup
  - `author: "jan-kowalski"` (slug) — resolved to ID via author slug lookup
  - `categories: [1, 5]` (int IDs) — works as before
  - `categories: ["Humor", "Technology"]` (names) — resolved to IDs via category name/slug lookup
  - Numeric strings (e.g., `author: "42"`) are parsed as integers automatically
  - Resolution is case-insensitive
  - Same flexibility works for MDDB content source
  - Unresolved string values (no matching author/category found) are silently ignored
- ✨ **WordPress-style bracket shortcodes** - opt-in via `shortcode_brackets: true`
  - Enables `[shortcode_name]` syntax alongside existing `{{shortcode_name}}`
  - Only defined shortcodes are matched — unknown `[tags]` are left untouched
  - Disabled by default to avoid conflicts with markdown link syntax

## [1.7.10] - 2026-04-06

### Added
- ✨ **Rewrite `.md` links to final URLs** - opt-in via `rewrite_md_links: true` (closes #5)
- ✨ **Sitemap exclusion** - pages/posts with `robots: "noindex"`, `layout: "redirect"`, or `sitemap: "no"` are excluded from `sitemap.xml` (closes #7)
  - Rewrites `href="AUTHENTICATION.md"` → `href="/authentication/"` based on actual slug
  - Handles relative prefixes `./file.md`, `../dir/file.md` — only base filename is matched
  - Priority: exact source filename > lowercase > slug-derived
  - Unknown `.md` links are left untouched
  - Disabled by default to avoid breaking sites serving raw `.md` files
- ✨ **Auto-derive slug from filename** - when no `slug:` in frontmatter, derived from filename
  - `AUTHENTICATION.md` without slug → slug `authentication` → `/authentication/`
- ✨ **`preserve_slug_case` option** - control URL casing for slugs derived from filenames
  - Default (`false`): lowercased — `API.md` → `/api/`
  - `preserve_slug_case: true` — original case kept — `API.md` → `/API/`

### Fixed
- Fix sitemap: use file modification time when `date`/`modified` fields are empty instead of writing `0001-01-01`
- Fix template fallback detection for custom page layouts

## [1.7.9] - 2026-04-06

### Added
- ✨ **Configurable pages and posts paths** - Override default `pages/` and `posts/` subdirectory names via config
  - `pages_path: "docs"` — read static pages from `content/{source}/docs/` instead of `pages/`
  - `posts_path: "articles"` — read posts from `content/{source}/articles/` instead of `posts/`
  - Default behaviour (`pages/` and `posts/`) is preserved when not set

## [1.7.8] - 2026-04-06

### Added
- ✨ **Template variables** - Define custom variables in `.ssg.yaml` available in all templates as `{{.Vars.key}}`
  - Flat and nested structures supported: `{{.Vars.gtm}}`, `{{.Vars.api.endpoint}}`
  - Values starting with `$` are resolved from OS environment variables at build time (e.g. `"$GTM_CODE"`)
  - All variables automatically exported as environment variables with `SSG_` prefix (e.g. `SSG_GTM`, `SSG_API_ENDPOINT`)
  - Available in every template context: index, page, post, category

## [1.7.7] - 2026-04-01

### Added
- ✨ **Skip minification for specific elements** - Use `<!-- htmlmin:ignore -->` comments (fixes #2)
  - Wrap content with `<!-- htmlmin:ignore -->...<!-- /htmlmin:ignore -->` to preserve whitespace
  - Perfect for Mermaid.js diagrams, code blocks, and pre-formatted content
  - Multiple ignore blocks supported in a single file

## [1.7.6] - 2026-04-01

### Fixed
- 🐛 **Pages directory now supports subdirectories** - Recursive scanning of `pages/` directory (fixes #1)
  - `content/pages/docs/intro.md` → `/docs/intro/`
  - `content/pages/docs/advanced/guide.md` → `/docs/advanced/guide/`
  - Works for both pages and posts (via category subdirectories)

## [1.7.4] - 2026-04-01

### Fixed
- 🐛 **Markdown parser fallback mode** - Content without `## Excerpt` or `## Content` markers is now properly parsed
  - Previously, markdown files without explicit section markers would have empty content
  - Now all content after frontmatter is treated as content when no markers are present

## [1.7.3] - 2026-03-31

### Added
- ✨ **Dynamic MDDB metadata fields with top-level access** - Custom metadata fields are flattened to template root
  - Use `{{.dupa}}` directly instead of `{{.Extra.dupa}}` or `{{.Page.Extra.dupa}}`
  - All standard Page fields also available at root: `{{.Title}}`, `{{.Content}}`, `{{.Slug}}`, etc.
  - Backward compatible: `{{.Page.Title}}` and `{{.Post.Title}}` still work
  - URL helpers at root level: `{{.URL}}`, `{{.CanonicalURL}}`, `{{.OutputPath}}`
- ✨ **Additional SEO fields from MDDB** - Now extracts: `description`, `keywords`, `lang`, `canonical`, `robots`, `featured_image`, `tags`, `category`, `layout`, `template`

## [1.7.2] - 2026-03-31

### Added
- 🔗 **Page output format** (`--page-format` / `page_format`) - Control how HTML files are generated
  - `directory` (default): `slug/index.html` - clean URLs with trailing slash
  - `flat`: `slug.html` - direct HTML files (e.g., `/docs/introduction.html`)
  - `both`: generates both formats for maximum compatibility
  - Works for both pages and posts
  - Config file option: `page_format: "flat"`

### Documentation
- 📖 Updated README.md with complete MDDB gRPC and watch mode documentation
- 📖 Updated man page with all MDDB options (protocol, watch, batch-size)
- 📖 Updated docs/INSTALL.md to require Go 1.26

## [1.7.1] - 2026-03-30

### Added
- 📎 **Co-located content assets** - Images and media files placed alongside Markdown content files are automatically copied to the corresponding output directory
  - Place `entry-image.png` next to `entry.md` and reference it with `![](entry-image.png)`
  - Supports: PNG, JPG, JPEG, GIF, SVG, WebP, ICO, BMP, TIFF, AVIF, MP4, WebM, OGG, MP3, WAV, PDF, ZIP
  - Works for both pages and posts
- 📖 **Man page** - Comprehensive `ssg.1` man page with full documentation of all options, configuration, and examples
  - Installed automatically via `make install`, DEB, and RPM packages

### Changed
- ⬆️ **Go dependencies updated** - All modules bumped to latest versions
  - goldmark v1.7.16 → v1.8.2
  - grpc v1.79.1 → v1.79.3
  - golang.org/x/net v0.48.0 → v0.52.0
  - golang.org/x/sys v0.39.0 → v0.42.0
  - golang.org/x/text v0.32.0 → v0.35.0
- 🐳 **Docker image updated**
  - Go builder: 1.25 → 1.26
  - Alpine runtime: 3.19 → 3.23
- 🔧 **GitHub Actions updated to latest versions**
  - codecov/codecov-action v4 → v5
  - docker/setup-qemu-action v3 → v4
  - docker/setup-buildx-action v3 → v4
  - docker/login-action v3 → v4
  - docker/metadata-action v5 → v6
  - docker/build-push-action v5 → v7
  - actions/upload-artifact v4 → v7
  - actions/download-artifact v4 → v8
  - github/codeql-action v3 → v4
- 📦 **Snap package updated** - base core22 → core24, platforms syntax
- 🔒 **Security** - Added gosec `#nosec` annotations for all G703/G122 false positives

## [1.7.0] - 2026-03-05

### Added
- ✨ **MDDB gRPC Support** - Optional gRPC connection alongside HTTP
  - CLI flag: `--mddb-protocol=grpc` (default: `http`)
  - YAML config: `mddb.protocol: "grpc"`
  - gRPC port: 11024 (HTTP: 11023)
  - Uses protobuf for faster serialization
  - Full gRPC API generated from MDDB proto file
- ✨ **MDDB Watch Mode** - Auto-rebuild on content changes
  - CLI flags: `--mddb-watch`, `--mddb-watch-interval=SEC`
  - YAML config: `mddb.watch: true`, `mddb.watch_interval: 30`
  - Polls collection checksum and rebuilds when content changes
  - Works with both HTTP and gRPC protocols

### Changed
- Refactored MDDB client to use interface pattern (supports HTTP and gRPC implementations)

## [1.6.2] - 2026-03-05

### Added
- ✨ **MDDB Batch Size** - Configurable batch size for pagination
  - CLI flag: `--mddb-batch-size=N` (default: 1000)
  - YAML config: `mddb.batch_size`
  - Removed hardcoded 1000 limit in `GetByType` - now fetches all documents with pagination

## [1.6.1] - 2026-03-05

### Fixed
- 🐛 **MDDB Client** - Aligned with actual MDDB API format
  - `contentMd` instead of `content`
  - `meta` (arrays) instead of `metadata`
  - `addedAt`/`updatedAt` (unix timestamps) instead of ISO dates
  - `X-Total-Count` header for pagination
  - `/v1/get` returns document directly (no wrapper)
  - `/v1/search` returns array directly
- 🐛 **Install Script** - Fixed download URL pattern for release assets

## [1.6.0] - 2026-03-05

### Added
- ✨ **MDDB Content Source** - Fetch markdown content from [MDDB](https://github.com/tradik/mddb) server
  - Single document fetch via `/v1/get` endpoint
  - Bulk fetch via `/v1/search` endpoint with pagination
  - CLI flags: `--mddb-url`, `--mddb-collection`, `--mddb-key`, `--mddb-lang`, `--mddb-timeout`
  - YAML config support:
    ```yaml
    mddb:
      enabled: true
      url: "http://localhost:8080"
      collection: "blog"
      lang: "en_US"
    ```
  - Automatic conversion of MDDB documents to pages/posts
  - Support for categories, media, and users collections

## [1.5.4] - 2026-02-04

### Added
- ✨ **Configurable shortcodes** - Define reusable content snippets in config
  - Use `{{shortcode_name}}` syntax in markdown content
  - Each shortcode requires a template file (no built-in HTML)
  - Template variables: `{{.Name}}`, `{{.Title}}`, `{{.Text}}`, `{{.URL}}`, `{{.Logo}}`, `{{.Legal}}`, `{{.Data}}`
  - Define in `.ssg.yaml`:
    ```yaml
    shortcodes:
      - name: "promo"
        template: "shortcodes/banner.html"
        title: "Special Offer"
        text: "Get 50% off!"
        url: "https://example.com"
    ```

## [1.5.3] - 2026-02-04

### Added
- ✨ **Relative links conversion** (`--relative-links` / `relative_links: true`)
  - Converts absolute URLs with site domain to relative links
  - Supports `href`, `src`, `action` attributes and `url()` in inline styles
  - Works with https, http, and protocol-relative URLs
  - Preserves external links to other domains

## [1.5.2] - 2026-02-03

### Fixed
- 🐛 **Pretty HTML now reliably removes ALL blank lines** - Refactored algorithm for better reliability
  - Uses line-by-line processing instead of regex for more predictable results
  - Handles CRLF and mixed line endings (Windows compatibility)
  - Added tests for CRLF and mixed line ending scenarios

## [1.5.1] - 2026-02-03

### Fixed
- 🐛 **Link field always takes priority** - If a post has `link` in frontmatter, it's used regardless of `post_url_format` setting
  - `post_url_format` is now a fallback when `link` is not present

## [1.5.0] - 2026-02-03

### Added
- ✨ **Configurable post URL format** (`--post-url-format` / `post_url_format`)
  - `date` (default): `/YYYY/MM/DD/slug/` - date-based URLs
  - `slug`: `/slug/` - SEO-friendly slug-only URLs
  - `link` field from frontmatter **always** takes priority
  - Config file option: `post_url_format: "slug"`

## [1.4.9] - 2026-01-29

### Fixed
- 🐛 **Pretty HTML now removes ALL blank lines** - Improved `--pretty-html` to fully clean HTML output
  - Previously only collapsed 3+ blank lines to 1 blank line
  - Now removes ALL empty/blank lines for truly clean HTML
  - Added comprehensive tests for config file parsing (`pretty_html: true`)

## [1.4.8] - 2026-01-29

### Changed
- 🔒 **Code quality improvements** - Refactored high-complexity functions and fixed all security scanner warnings
  - Reduced cyclomatic complexity in `main()`, `parseFlags()`, `Generate()`, `loadTemplates()`, `ParseMarkdownFile()`
  - Added documented `#nosec` comments for all 41 gosec false positives (CLI tool with trusted inputs)
  - All quality checks pass: golangci-lint, gosec, gocyclo (<15)

### Added
- 🛡️ **OpenSSF Scorecard badge** - Security posture visibility in README

## [1.4.7] - 2026-01-29

### Added
- ✨ **Pretty HTML output** (`--pretty-html`) - Clean up generated HTML without minification
  - Removes excessive blank lines (collapses to max 1 between elements)
  - Removes whitespace-only lines
  - Removes trailing whitespace from lines
  - Keeps readable formatting, not aggressive like minify
  - Also available as `--pretty` shorthand
  - Config file option: `pretty_html: true`

## [1.4.6] - 2026-01-23

### Fixed
- 🐛 **Homepage overwriting prevention** - Pages with `link` field pointing to root URL no longer overwrite the main index.html
  - Generator now skips pages that would generate to root path with a warning
  - Displays hint to change the `link` field or use a different slug
  - Fixes: imd.agency frontpage showing raw content instead of designed homepage template

## [1.4.5] - 2026-01-23

### Fixed
- 🐛 **WordPress metadata parsing** - Handle `width`/`height` as string or int
  - Added `FlexInt` type for flexible JSON unmarshaling
  - Fixes: `json: cannot unmarshal string into Go struct field .media.media_details.width of type int`

## [1.4.4] - 2026-01-18

### Changed
- 📝 **Complete README overhaul** - Hugo-style comprehensive documentation
  - Added detailed Overview section
  - "What Can You Build?" guide with use cases
  - Key Capabilities table
  - Development Workflow documentation
  - Asset Processing details
  - Reorganized Features into categories

## [1.4.3] - 2026-01-18

### Fixed
- 🔧 **Example workflow moved** - `example-deploy.yml` moved to `examples/workflows/`
  - No longer runs on every push to main
  - Users copy it to their own `.github/workflows/`

### Added
- 📁 **Examples directory** - `examples/workflows/` with complete workflow templates
- 📝 Examples README with usage instructions

## [1.4.2] - 2026-01-18

### Fixed
- 🐳 **Docker build optimization** - Only builds on full semver tags (v1.4.2), not major version alias (v1)
- 📄 **Jekyll compatibility** - Escaped Liquid syntax in README.md for GitHub Pages

### Changed
- 🔧 **Code quality** - Refactored main() to reduce cyclomatic complexity (25 → 18)
- 📝 Added LICENSE.md for better Go Report Card detection

## [1.4.1] - 2026-01-18

### Added
- ✅ **Test coverage** for new packages:
  - `engine`: 61.6% coverage
  - `config`: 79.2% coverage
  - `theme`: 26.1% coverage
- 📝 **SECURITY.md** - Security policy and best practices
- 👥 **CONTRIBUTORS.md** - Contribution guidelines
- 🎨 **Template examples** for all engines (pongo2, mustache, handlebars)

### Changed
- 🔄 Updated all dependencies to latest versions
- 📦 Updated GitHub Action with `engine` and `online-theme` inputs

## [1.4.0] - 2026-01-18

### Added
- 🔧 **Multiple template engines** - choose your preferred syntax:
  - `--engine=go` (default) - Go templates
  - `--engine=pongo2` - Jinja2/Django-like templates
  - `--engine=mustache` - Mustache templates
  - `--engine=handlebars` - Handlebars templates
- 🌍 **Online theme download** (`--online-theme=URL`):
  - Download Hugo themes from GitHub/GitLab
  - Support for direct ZIP URLs
  - Auto-extraction to templates directory

### Documentation
- Added comprehensive Template Engines section
- Template syntax comparison for all engines
- Examples for using online themes

## [1.3.4] - 2026-01-17

### Changed
- 📦 **WebP tools now installed automatically** in GitHub Action
  - No need to manually install `cwebp`
  - Works on Linux and macOS runners

## [1.3.3] - 2026-01-17

### Fixed
- 🐛 **Raw binaries now included in releases** - direct download works:
  - `curl -sL .../ssg-linux-amd64 -o ssg` ✅
  - `curl -sL .../ssg-darwin-arm64 -o ssg` ✅
  - `curl -sL .../ssg-windows-amd64.exe -o ssg.exe` ✅
- Fixed CI release job to include all artifact types (archives + raw binaries)

## [1.3.2] - 2026-01-17

### Fixed
- 🔧 **Simplified release asset naming** - removed version from filenames for easier downloads
  - Archives now named `ssg-linux-amd64.tar.gz` instead of `ssg-1.3.1-linux-amd64.tar.gz`
  - Raw binaries also available: `ssg-linux-amd64` (no extension)
- 🐛 Fixed GitHub Action download URL to match new asset naming
- ✅ Added HTTP status and content validation for binary downloads

## [1.3.1] - 2026-01-17

### Added
- 🐳 **Docker support** - minimal Alpine-based image (~15MB)
  - Multi-arch builds: `linux/amd64` and `linux/arm64`
  - Published to GitHub Container Registry: `ghcr.io/spagu/ssg`
  - Docker Compose configuration included
- 🔄 Docker CI workflow for automatic image builds

### Changed
- Reverted to `cwebp` for WebP conversion to support static builds and cross-compilation (removed CGO dependency)
- Changed license to BSD 3-Clause
- ⚡ **GitHub Action now downloads pre-built binary** instead of building from source (much faster!)
  - Added `version` input to specify SSG version
  - Added `minify` and `clean` inputs

### Documentation
- Added Docker installation and usage examples
- Updated GitHub Actions versioning documentation
- Updated License badge
- Added Code of Conduct

## [1.3.0] - 2026-01-17

### Added
- 🌐 **Built-in HTTP server** (`--http` flag) - no need for external Python/Node server
- 🔌 **Custom port** (`--port=PORT`) - default: 8888
- 👀 **Watch mode** (`--watch` flag) - auto-rebuild on file changes (with error recovery)
- 📄 **Config file support** (`--config`) - load settings from YAML, TOML, or JSON
  - Auto-detects `.ssg.yaml`, `.ssg.toml`, `.ssg.json`
  - All CLI flags available in config file
- 🖼️ **WebP conversion** (`--webp`) - requires `cwebp` installed
  - `--webp-quality=N` - compression level 1-100 (default: 60)
- 📝 `stripHTML` template function for clean meta descriptions
- 🧹 **Clean build** (`--clean`) - clean output directory before build
- 🔇 **Quiet mode** (`--quiet`, `-q`) - suppress output, only exit codes
- 🗺️ **Sitemap control** (`--sitemap-off`) - disable sitemap.xml generation
- 🤖 **Robots control** (`--robots-off`) - disable robots.txt generation
- 🗜️ **Minification options**:
  - `--minify-all` - minify HTML, CSS, and JS
  - `--minify-html` - minify only HTML
  - `--minify-css` - minify only CSS
  - `--minify-js` - minify only JS
- 🗂️ **Source maps** (`--sourcemap`) - include source maps in output
- ℹ️ **Version flag** (`--version`, `-v`) - show version info
- ❓ **Help flag** (`--help`, `-h`) - show usage help
- 📦 **Multi-platform packages**:
  - Debian/Ubuntu: `.deb` packages (amd64, arm64)
  - Fedora/RHEL: `.rpm` packages (x86_64, aarch64)
  - Ubuntu Snap: `snap` package
  - macOS Homebrew: `brew install spagu/tap/ssg`
  - FreeBSD/OpenBSD: Port Makefiles
- 🔧 Quick install script (`install.sh`)
- 📖 Comprehensive installation documentation (`docs/INSTALL.md`)

### Changed
- Refactored build logic into reusable function for watch mode
- WebP conversion now uses native Go library (removed `cwebp` dependency)
- Config package for loading settings from files

### Fixed
- Page title overlapping with fixed navigation header
- Text width constrained by `max-width: 65ch` now fills container properly

## [1.2.0] - 2026-01-16

### Added
- 🎬 **GitHub Actions support** - Use SSG as a step in GitHub Actions workflows
- 📋 `action.yml` - Composite action definition with full input/output configuration
- 🔄 CI/CD workflows:
  - `ci.yml` - Test, lint, build, and release pipeline
  - `test-action.yml` - Tests for the GitHub Action itself
  - `example-deploy.yml` - Example Cloudflare Pages deployment workflow
- 📦 Automatic artifact uploads for all platforms
- 🏷️ Automatic release creation from version tags (v*)
- 🧪 Test content for CI validation
- 📂 **Custom directory paths**:
  - `--content-dir=PATH` - specify custom content directory
  - `--templates-dir=PATH` - specify custom templates directory  
  - `--output-dir=PATH` - specify custom output directory
- 😈 **FreeBSD support** - builds for FreeBSD amd64 and arm64
- 🗓️ **Flexible date parsing** - supports multiple formats:
  - RFC3339: `2025-01-01T12:00:00Z`
  - Datetime: `2025-01-01T12:00:00`
  - Date only: `2025-01-01`
  - And more formats

### Changed
- Improved cross-platform build matrix (8 targets now)
- All platforms now include arm64 builds:
  - Linux: amd64, arm64
  - FreeBSD: amd64, arm64
  - macOS: amd64, arm64
  - Windows: amd64, arm64
- Enhanced output path configuration via action inputs

### Fixed
- Date parsing now handles simple `YYYY-MM-DD` format correctly
- Fixed "same file" error in GitHub Action when testing locally with `uses: ./`
- Code cleanup: Fixed unhandled error returns (golangci-lint errcheck)

### Documentation
- Updated README with GitHub Actions usage examples
- Added workflow examples for Cloudflare Pages deployment
- Added CLI options documentation
- Added status badges for Code Quality, Coverage, and Project Stats

## [1.1.0] - 2026-01-13

### Added
- 🖼️ WebP image conversion (`--webp` flag) - reduces image sizes by ~70%
- 📦 ZIP deployment package (`--zip` flag) for Cloudflare Pages
- ☁️ Cloudflare Pages support with `_headers` and `_redirects` files
- 📊 Markdown table support (GFM extension)
- 🔗 Automatic media path fixing (relative to absolute)
- 🗺️ Sitemap.xml generation
- 🤖 robots.txt generation
- 🔐 SEO meta tags (Open Graph, Twitter Card, Schema.org JSON-LD)

### Changed
- Improved image path handling in HTML and CSS files
- Better srcset handling for responsive images

### Fixed
- Fixed relative media paths in href attributes
- Fixed srcset image extensions when using --webp

## [1.0.0] - 2026-01-13

### Added
- 🚀 Initial release of SSG (Static Site Generator)
- 📝 Markdown parser with YAML frontmatter support
- 🎨 Two templates: **simple** (dark) and **krowy** (green/farm theme)
- 📄 Page generation with SEO-friendly URLs
- 📝 Post generation with category support
- 📁 Category listing pages
- 🖼️ Media file copying
- 📱 Responsive design for both templates
- ♿ WCAG 2.2 color contrast compliance
- 🧪 Unit tests for parser and generator
- 📖 Comprehensive documentation
- 🔧 Makefile with colored output and help

### Templates
- **simple**: Modern dark theme with glassmorphism, purple gradient accents, micro-animations
- **krowy**: Light green farm theme inspired by krowy.net, natural colors, cow emoji logo

### Technical
- Go 1.25+ required
- Single binary output
- Dependencies: gopkg.in/yaml.v3, github.com/yuin/goldmark
- Cross-platform build support (Linux, macOS, Windows)

<!-- Compare links (DOC-011) -->
[1.8.0]: https://github.com/spagu/ssg/compare/v1.7.15...v1.8.0
[1.7.15]: https://github.com/spagu/ssg/compare/v1.7.14...v1.7.15
[1.7.14]: https://github.com/spagu/ssg/compare/v1.7.13...v1.7.14
[1.7.13]: https://github.com/spagu/ssg/compare/v1.7.12...v1.7.13
[1.7.12]: https://github.com/spagu/ssg/compare/v1.7.11...v1.7.12
[1.7.11]: https://github.com/spagu/ssg/compare/v1.7.10...v1.7.11
[1.7.10]: https://github.com/spagu/ssg/compare/v1.7.9...v1.7.10
