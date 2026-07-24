# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- 🔐 **Docs site: Turnstile keys from GitHub secrets** (GO-082) — the docs-site
  deploy workflow now injects the Turnstile **site key** into the config from a
  GitHub secret at build time (the committed test key stays when it's unset), and
  pushes the **secret key** (plus optional moderation password / IP salt) onto
  the Pages project via `wrangler pages secret put`, so no keys live in the repo.
  The comments worker README documents the pattern, including that a D1 binding
  is a project setting rather than a secret.

### Fixed
- 🐛 **`ssg --deploy=cloudflare` now ships Pages Functions** (GO-082) — when the
  output has a `functions/` tree, SSG deploys via `wrangler pages deploy`, but it
  ran `wrangler pages deploy <output-dir>` from the current directory. wrangler
  compiles the `functions/` tree relative to its **working directory**, not the
  deploy-dir argument, so it uploaded the static assets *without* the Functions:
  the API then served `/api/*` as static (GET → 200 HTML, POST → 405) and every
  worker endpoint (comments, cookie-consent geo/log, …) was silently dead. SSG
  now runs wrangler **from** the output directory and deploys `.`, so the
  Functions are compiled and shipped. (Same working-directory rule already fixed
  for `--watch` in 1.8.13.)

## [1.8.13] - 2026-07-24

### Added
- 🔑 **Comments moderation behind Cloudflare Access (SSO/JWT)** (GO-081) — an
  alternative to the shared moderation password: set `COMMENTS_ACCESS_TEAM` and
  `COMMENTS_ACCESS_AUD`, put `/comments-admin.html` and `/api/comments/admin*`
  behind a Cloudflare Access application, and the worker verifies the signed JWT
  Access forwards (signature against your team's JWKS, audience, issuer, expiry)
  instead of a password. Moderators sign in through your IdP; the panel detects
  the Access session and skips its own login. No shared secret to store or
  rotate. The password path still works when Access isn't configured.
- ⏱️ **Configurable fetch: `timeout`, `retries`, `retry_delay`, `on_error`**
  (GO-081) — a remote `include:` (and remote worker `source:`) previously had a
  fixed 30s timeout, no retry, and any failure hard-failed the build. Each remote
  include can now set its own `timeout`, `retries` (default 3), `retry_delay`
  (default 5s) and `on_error` (`fail`, the default, or `warn` to continue without
  it). A transient failure (network error, HTTP 429/5xx) is retried; a 4xx is
  not. Absent keys use the defaults, so existing configs keep working.
- 🚀 **`republish-trigger` worker** (GO-080) — one authenticated webhook,
  `POST /api/republish`, that fires a CI build on **GitHub**, **GitLab** or
  **Gitea**, so a headless-CMS "published" webhook, a cron or a `curl` redeploys
  the site without touching the repo. The caller proves itself with a shared
  `REPUBLISH_KEY` (constant-time check, header or query); the provider token
  stays server-side. GitHub uses `workflow_dispatch` (or `repository_dispatch`),
  GitLab a pipeline trigger token, Gitea a workflow dispatch — self-hosted hosts
  via `REPUBLISH_API_BASE`. GET is off unless opted in, and an optional KV
  binding debounces bursts into one build (`429` inside the window). Scaffold
  with `ssg new worker republish-trigger`.
- 🔒 **Auto-close idle comment threads** (GO-078) — a new
  `COMMENTS_CLOSE_AFTER_DAYS` var stops a thread accepting comments once that
  many days have passed since its last activity (the newest comment, or the
  post's publish date while it has none). Active discussions stay open; a post
  nobody has touched for a month locks itself. `GET /api/comments` reports
  `"closed"`, the widget then hides the form and shows a localised "Comments are
  closed" notice (existing comments stay visible), and `POST` returns `403
  comments closed` — checked before spending a Turnstile verification. The theme
  renders the post's publish date so empty old threads close too. `0`/unset
  keeps the previous always-open behaviour.
- 📥 **Bulk comment import** (GO-078) — an admin-only `POST /api/comments/import`
  takes normalised JSON (an array of `{url, author, body, email?, created_at?,
  status?}`) so a migration from Disqus, WordPress, Commento or a spreadsheet
  converts to one shape and posts once. Idempotent — each id is a content hash
  inserted with `INSERT OR IGNORE`, so re-running a file adds nothing new;
  invalid rows are skipped and counted, not fatal; up to 1000 per request.
  Imported comments default to `approved`. The moderation panel gains an
  **Import comments** box (file or paste) so it needs no curl.
- 🎨 **Mermaid diagram theme + background** (GO-079) — two new options,
  `mermaid_theme` (mermaid's built-in `default`/`neutral`/`dark`/`forest`/`base`)
  and `mermaid_background` (any CSS colour), tune diagram legibility. Diagrams
  are transparent by default, so on dark site chrome they were hard to read;
  `mermaid_background` boxes each one on a solid panel (padding + rounded
  corners), and `mermaid_theme` picks a matching palette. Both only affect pages
  that actually contain a diagram. The docs site now uses a white panel.
- 🌐 **Comments widget speaks the page's language** (GO-078) — the `comments`
  reader widget is now translated (en/pl/de/fr), picking the language from
  `<html lang>` exactly like the cookie banner, so a post in Polish gets a Polish
  form. A `comments.i18n` config block overrides any string or adds a language
  without editing the worker.
- 🧩 **Config includes: split `.ssg.yaml` across files** (GO-076) — a config can
  `include:` other YAML files from a **path or a URL**, so a project's config
  splits into focused pieces (shared defaults in a base, each worker its own
  file). Base-first merge: includes are merged in listed order, then the main
  file overlays on top and always wins. Maps merge recursively; lists of maps
  that carry a `name` merge **by name** (so each file can contribute one
  `workers:`/`content_sources:` entry without clobbering the others); other
  lists replace. Cycles are rejected, diamonds allowed. Remote includes take an
  optional `auth:` (`bearer`/`basic`/`header`) whose secret fields must
  reference environment variables.
- 🧰 **Several workers: the `workers:` list** (GO-076) — the singular `worker:`
  becomes a plural list of **independent** worker definitions, each with its own
  `routes`, `wrangler_config`, a free-form per-worker `config:` block, and an
  optional remote `source:` (a GitHub/GitLab repo or `.zip`, fetched into `dir`
  with the same `auth:` model). The singular `worker:` still works unchanged.
  Because Cloudflare Pages serves one `functions/` tree per project, the
  workers' functions merge into it and their routes combine — and two workers
  claiming the same output file is a **hard error**, never a silent overwrite.
- 🧩 **Wrangler config generator** (GO-077) — a project that uses workers needs
  a `wrangler.toml` for `wrangler pages dev`/`deploy`. SSG now writes a starter
  one when none exists — automatically on `--watch`, or on demand via
  `ssg new wrangler` — deriving `name` from the domain and
  `pages_build_output_dir` from the output dir, and appending each worker's own
  `wrangler.snippet.toml` (its bindings/vars, e.g. cookie-consent's optional
  `CONSENT_LOG` KV). An existing config is never overwritten.
- 🔧 **`--watch` serves Functions correctly for Pages** (GO-077) — a
  functions-mode worker now runs `wrangler pages dev .` **from the output
  directory** (where SSG copies the `functions/`), so pages and Functions serve
  together; the previous `wrangler dev` from the worker dir did not serve the
  static site. A prebuilt `mode: worker` is unchanged.
- 🎛️ **`toJSON` template helper + cookie-consent on the docs site** (TPL-004) —
  a `toJSON` helper emits a value as inline JSON (config blobs, JSON-LD),
  correctly once inside a `<script>` (it returns `template.JS`, so html/template
  does not double-encode it). ssgtheme renders the cookie-consent banner from a
  `variables.cookie_consent` block, and the SSG documentation site now dogfoods
  the worker. The banner's position is configurable — `bottom` (default), `top`
  or `center`.
- 💬 **`comments` worker** (GO-078) — comments for a site (blogs especially),
  stored in Cloudflare D1, scaffolded with `ssg new worker comments`. No
  accounts: a name, an optional email (avatar hash only), a body. Turnstile on
  submit, a heuristic spam score (or Akismet when a key is set), and every new
  comment held `pending` until an admin approves it in a password-protected
  panel. For compliance the row keeps a **salted hash** of the IP plus the
  user-agent — the raw IP is never stored. Ships a dependency-free reader widget
  and a moderation page; JS rendering by default, static baking documented.
- 🐛 **Scaffold shared worker modules** (GO-078) — `EmbeddedWorkers` now uses
  `//go:embed all:workers`, so a Pages Function's shared `_`-prefixed module
  (which go:embed's default rule would drop) ships with the scaffold. Without
  it, comments' `_lib.ts` was silently missing and the functions failed to
  build.
- 🍪 **`cookie-consent` worker** (GO-076) — a GDPR / ePrivacy / UK-PECR consent
  banner scaffolded with `ssg new worker cookie-consent`. Prior consent
  (non-essential `<script type="text/plain" data-consent-category>` tags stay
  inert until granted), reject as prominent as accept, edge geo-gating (shown in
  the EEA and UK by default, `GET /api/consent/geo`), granular categories,
  versioned/expiring consent, a "manage cookies" reopen hook, i18n (en/pl/de/fr),
  Google Consent Mode v2 signals, and an optional Turnstile-verified audit log
  (`POST /api/consent/log`) that stores the IP only as a salted hash. Ships a
  starter `cookie-policy.md` the user edits to list their services. The banner
  js/css live in the worker's `public/`, now served from the site root.
- 📦 **A worker's `public/` is served as static assets** (GO-076) — each worker
  can ship client-side files (a consent banner's js/css) under `public/`, copied
  to the output root at build with the same cross-worker collision guard as its
  functions.
- 🔐 **`internal/fetch`** (GO-076) — shared, hardened, authenticated fetch
  (bounded client, size caps, path-escape-guarded zip extraction, env-only
  secrets) behind config includes and remote worker sources.

### Fixed
- 🐛 **A worker without `routes_include` is no longer left unrouted** (GO-081) —
  the implicit `/api/*` default was applied only to the *combined* route list,
  so a worker that omitted `routes_include` next to one that set its own (e.g.
  `/consent/*`) never got routed and its Functions were never invoked. The
  default is now per-worker, and duplicate routes are collapsed so they don't
  count twice against the Cloudflare rule cap.
- 🐛 **A remote worker `source:` without a name is rejected** (GO-081) — two
  unnamed sources both vendored into `workers/worker`, so the second silently
  reused the first's files; a source now requires a `name` or an explicit `dir`.
- 🐛 **A failed worker fetch no longer poisons later builds** (GO-081) — a remote
  archive now extracts into a staging dir and is renamed into place only on full
  success, so a mid-extraction failure can't leave a half-populated directory
  that the next build reuses as if complete.
- 🐛 **Generated `wrangler.toml` name is always Cloudflare-valid** (GO-081) —
  `wranglerName` now prefixes a digit-leading domain (`1password.com`) so the
  name starts with a letter, and caps it at Cloudflare's 58-character limit.
- 🔒 **Comments auto-close could be bypassed with a forged `published`** (GO-081)
  — the close check took `max(lastComment, clientPublished)`, so a raw POST with
  a far-future `published` out-voted a years-old last comment and kept a closed
  thread open. The newest comment (server-side) now governs; the client-supplied
  publish date anchors only an empty thread (where forging it merely allows a
  first comment on an old-but-empty post).
- 🔒 **Comment IP hash is no longer stored unsalted** (GO-081) — `sha256(ip)`
  without a salt is reversible across the 2³² IPv4 space, defeating the
  "raw IP never recoverable" guarantee. With no `COMMENTS_IP_SALT` /
  `CONSENT_IP_SALT` set, the comments and consent-log workers now store no hash
  at all instead of a false-safe one.
- 🔒 **Open redirect via a stored comment URL** (GO-081) — `normaliseURL`
  rejected `//…` but accepted `/\evil.com`, which a browser resolves to
  `https://evil.com/`; a moderator clicking the link in the panel was sent
  off-site. Backslashes are now rejected.
- 🐛 **Bulk import wasn't idempotent for items without `created_at`** (GO-081) —
  the row id hashed the `now()` default, so re-importing such an export inserted
  duplicates; it now hashes the caller-provided timestamp (empty when absent).
- 🐛 **Consent audit log written on every pageview** (GO-081) — the banner
  re-ran the full apply (store + log) on each page load for a returning visitor,
  so the audit log grew by one entry per pageview and the cookie's expiry slid
  to "last visit". Re-applying a stored choice now only re-activates scripts and
  re-signals Consent Mode; storing and logging happen only on an actual choice.
- 🔒 **Hardening** (GO-081) — the moderation panel no longer reveals itself on a
  `503` "not configured" (only on a real sign-in); the consent-log endpoint caps
  the number/length of submitted categories; and both constant-time secret
  compares now fold the length difference in rather than returning early, so a
  configured secret's length can't leak through timing.
- 🔒 **Auth credential no longer leaks across a redirect** (GO-081) — the shared
  authed fetch (YAML `include:` URLs and remote worker `source:`) followed
  redirects while forwarding the credential: Go re-sends a custom auth header
  (the `header` auth type, e.g. `X-Api-Key`) to *any* redirect target and only
  drops `Authorization` across a different domain, so a configured server could
  `302` a private-source token to another host. The client now strips the
  credential (custom header, `Authorization`, `Cookie`) on any redirect that
  leaves the original origin or downgrades https→http. Also: `safeURL` now
  redacts URL userinfo (`https://<token>@host/…`), not just the query string, so
  a token embedded in a URL can't surface in an error message.
- 🐛 **Duplicate `name` in a merged config list no longer corrupts it** (GO-081)
  — `mergeNamedLists` dropped the first of two same-named entries and emitted the
  second twice; it now merges a repeated name in place.
- 🐛 **Bogus "imports npm package" warning on multi-line imports** (GO-080) — the
  worker npm-import scan read `import {` line by line, so a `import {\n … } from
  "./_lib"` (as in the comments worker) was mis-reported as importing a package
  literally named `"import {"`. It now inspects the `from "…"` clause across line
  breaks: relative/builtin/URL imports are silent and a genuine bare npm
  specifier is still flagged with its real name — even when the import spans
  several lines (previously such an import was missed entirely).


## [1.8.12] - 2026-07-22

### Added
- 🔗 **`strip_md_link_text`** (GO-075) — drops the `.md` from a link's visible
  text when that text is a bare filename, at publish time, so
  `[CONFIGURATION.md](CONFIGURATION.md)` reads as "CONFIGURATION". Only anchor
  text that is exactly a filename is touched — prose, inline code and code
  blocks are left alone, and the source `.md` files are never modified.
  Complements `rewrite_md_links`. The documentation site enables it.
- 📊 **Mermaid diagrams** (GO-073) — with `mermaid: true`, a ```` ```mermaid ````
  fence is rewritten to a `<pre class="mermaid">` block before rendering (so the
  diagram source passes through verbatim instead of being HTML-escaped — the
  reason such fences previously failed to parse) and the mermaid.js runtime is
  injected **only on pages that contain a diagram**, mirroring the page-scoped
  KaTeX approach. Off by default: a mermaid fence stays a plain code block.
- 🔢 **Line numbers for code highlighting** (GO-074) — `highlight_line_numbers:
  true` prefixes every Chroma-highlighted block with line numbers (requires
  `highlight: true`).

### Changed
- The documentation site (`docs-site.yaml`) now enables `highlight`,
  `highlight_line_numbers` and `mermaid`, so guide and blog code blocks are
  coloured with line numbers and their diagrams render.

## [1.8.11] - 2026-07-22

### Added
- 🖼️ **AVIF output + `imagePicture` helper** (GO-070, closes #43) — the image
  pipeline now encodes AVIF through the optional `avifenc` tool (from libavif),
  mirroring the existing `cwebp` approach: no CGO, the binary stays static, a
  missing tool is a descriptive error. The new `imagePicture` template helper
  emits a `<picture>` with format fallback — one `<source>` per format
  (avif/webp/jpeg…) in declared order, each with its own responsive `srcset`,
  and an `<img>` fallback carrying `width`/`height` for zero CLS. A format whose
  encoder is absent is **skipped with a warning, not a build failure**, so the
  same template works on a machine without `avifenc`/`cwebp`. `.HTML` returns
  ready markup; `.Sources`/`.Fallback` expose the parts. Documented in
  `docs/IMAGES.md`.
- 🧭 **`ssg init`** (GO-071) — scaffolds a ready-to-build project in the current
  directory (config, a content source tree with a sample page and post, a
  `static/` folder and a `.gitignore`) **without overwriting any existing
  file**: every file already present is kept and reported, so it is safe to run
  in a populated directory. Optional source name and `--domain`.
- 🗂️ **Per-taxonomy `paginate`** (GO-072, part of #44) — a taxonomy definition
  can set its own `paginate:` page size, overriding the global `paginate` for
  that taxonomy's term archives (0 = fall back to the global value). A site with
  400 tags and 12 categories can now paginate each differently. Documented in
  `docs/TAXONOMIES.md`.
- 🔀 **Redirects engine** (GO-063) — a `redirects:` config section now generates
  a real Cloudflare Pages / Netlify `_redirects` file (previously it was written
  empty). Rules support exact paths, `/old/*` splats with `:splat`, and status
  `301`/`302`/`307`/`308`/`410`. Frontmatter `aliases:` are added as `301`s
  automatically, and exact chains `A → B → C` are flattened to `A → C` at build
  time (with cycle detection) so visitors take one hop, not several — the
  chained-redirect SEO penalty. Validation warns on duplicate sources, wildcard
  shadowing, `:splat` without a `*`, missing targets and the Cloudflare rule
  caps, never failing the build. `alias_stubs: false` keeps only the `_redirects`
  301s and drops the meta-refresh stub pages. Empty by default — existing sites
  are unchanged.
- 📥 **`ssg import redirects`** (GO-067) — converts a Next.js `redirects()` rule
  set into a ready-to-paste `redirects:` YAML block. Reads a JSON dump
  (`--from-json`, the reliable path) or heuristically parses a
  `next.config.(js|ts|mjs)`. Next.js path syntax (`/:slug*`) is translated to
  `_redirects` syntax (`/*` → `:splat`), `permanent` maps to 301/302, and any
  entry it cannot read (conditional `has`/`missing`, template literals,
  regex-constrained params) is reported — never silently dropped.
- ⚡ **Cloudflare Pages Functions / Worker integration** (GO-065) — a `worker:`
  section wires a Functions directory (or a prebuilt `_worker.js`) into the
  build output and generates `_routes.json`, so transactional endpoints (Stripe,
  contact/job forms, dynamic pricing, server-side conversions) live beside the
  static site. Deploy is automatic: a `functions/` tree deploys via `wrangler
  pages deploy`, `mode: worker` via pure-Go Direct Upload. `--watch` defaults its
  runner to `wrangler dev` so preview and Functions run together. No JS bundler —
  Pages builds Functions from source.
- 🧰 **`ssg new worker <template>`** (GO-066) — scaffolds batteries-included
  Pages Functions templates (no npm dependencies): `contact-form` (Turnstile +
  MailChannels/Resend), `stripe-checkout` (Checkout Session + webhook signature
  verification), `dynamic-price` (KV/API price lookup + client snippet) and
  `conversions-proxy` (server-side Meta CAPI with hashed PII).
- 🧱 **Configurable `_headers`** (GO-064) — a `headers:` section overrides or
  extends the generated Cloudflare Pages header blocks per path pattern;
  `headers_defaults_off` drops the built-in security/cache blocks. Empty config
  reproduces the historical output byte-for-byte (locked by a regression test).
- 📗 **Payload CMS build-time recipe** (GO-068) — documented in
  `docs/EXTERNAL_SOURCES.md`: pull Payload's REST API into `.ExternalData` via
  the existing `http` connector, no new adapter needed.

### Fixed
- 📝 **`docs/DEPLOYMENT.md` claimed aliases became `301`s in `_redirects`** — the
  code only wrote meta-refresh stubs (GO-069). The redirects engine (GO-063)
  makes the claim true; the docs now describe the real mechanism.
- 🧩 **`layout:` in frontmatter never selected the layout** (GO-058) — the
  lookup asked for the template named `layouts/<name>.html`, but `ParseGlob`
  registers a template under its **base** filename, so `layouts/blog.html` is
  parsed as `blog.html`. Nothing matched, and the page fell back to `page.html`
  without a warning: the documented feature could not work unless the theme
  happened to write `{{ define "layouts/blog.html" }}`. Both spellings now
  resolve, path form first, so existing themes are unaffected.

## [1.8.10] - 2026-07-21

### Added
- 📚 **`content_sources`: Markdown from more than one place** (CONTENT-002) —
  a site is no longer limited to one `content/<source>/` tree. `content_sources`
  lists extra flat Markdown roots (loaded recursively), each merged as pages or
  posts and optionally filed under one category, which is created when the
  loaded metadata does not define it. Sources join the site before finalize, so
  they get the same URL, permalink, i18n, taxonomy and collision treatment as
  native content; watch mode watches them; the image pipeline resolves images
  beside them. With at least one extra source the primary `source` — and its
  `metadata.json` — becomes optional, so a site can consist of a `docs/` folder
  alone. CLI: repeatable `--content-source=DIR`. Empty by default, so
  single-source builds are unchanged.
- 🎨 **Bundled `ssgtheme` documentation theme** — cards, guide layout, archive
  and post templates, a colour-scheme switch, an optional hero photograph
  rendered through SSG's own image pipeline, and shared chrome in `partials/`.
  Design tokens mirror the [Tradik design system](https://designstyles.tradik.com/)
  1:1; all text meets WCAG 2.2 AA and body text AAA in both schemes. The
  repository's own docs build with it via `make site` / `make site-watch`.
- 🔗 **`link_rewrites`** (LINK-002) — maps an href prefix in content to a
  replacement, so documentation links to repository files the site never
  publishes (`../examples/`, a sample config) point at the repository instead
  of 404ing. Longest matching prefix wins.
- 🔤 **`auto_excerpt`** (GO-057) — derives a missing excerpt from the content's
  opening paragraph (capped at 200 characters on a word boundary, skipping
  headings, fenced code, tables, quotes, images and Liquid guards), so cards,
  feeds and meta descriptions are not blank for documents written without a
  `## Excerpt` section. Off by default: it changes those texts on an existing
  site.
- ➗ **Arithmetic template helpers `add` / `sub` / `mul` / `div`** (TPL-003) —
  Go templates have none, so a theme could not split a list into columns or
  compute "page N of M" without preprocessing in Go. Integer operands give
  integer results (`div 7 2` → `3`); a float operand gives a float. Division by
  zero and non-numeric arguments are template errors, not silent infinities.
- 🔣 **Site variables reach shortcode templates** (issue #37) — `{{$.Vars.key}}`
  / `{{.Vars.key}}` now resolve inside a shortcode template, the same spelling
  page templates use. Previously the template context was the `Shortcode`
  struct alone, so `$.Vars.anything` was a template error that silently removed
  the whole shortcode from the page while the build still exited 0.
- 🚨 **`shortcode_errors` / `--shortcode-errors=drop|keep|strict`** (issue #37)
  — chooses what a shortcode that fails to render leaves behind. `drop`
  (default) keeps today's behaviour, so existing sites build byte-identically.
  `keep` leaves the shortcode's raw source (`{{promo}}`, `[promo a="b"]`) in
  the page, making the gap visible — a page that quietly lost its payment
  widget looks fine, one showing `[stripe_form]` does not — and unlike an HTML
  comment it survives minification. `strict` additionally fails the build after
  the render step, listing every shortcode that failed.

- 🚀 **Documentation site published to Cloudflare Pages** — `ssg.tradik.com` is
  built by `.github/workflows/docs-site.yml` from `docs/` via `content_sources`,
  using the `ssg` binary from the commit being deployed. `shortcode_errors:
  strict` plus `--check-links=strict` gate the upload, so a broken shortcode or
  a dead internal link fails the run instead of publishing a hole. The workflow
  creates the Pages project and attaches the custom domain on its first run, so
  setup is two repository secrets and nothing in the dashboard.

### Removed
- 🧹 **Jekyll GitHub Pages workflow** — it built the whole repository root as a
  Jekyll site and had been failing on every push; the documentation site is now
  built by SSG itself. The `{% raw %}` guards that existed only for Jekyll are
  gone from `docs/`, where they had started leaking into rendered excerpts.

### Fixed
- 🔗 **`.md` links with an anchor were never rewritten** (GO-056) — the rewrite
  pattern required the href to *end* in `.md`, so `CONFIGURATION.md#section`
  silently shipped as a dead link to a file that does not exist in the output,
  while the same link without an anchor worked. Anchors and query strings are
  now carried across to the rewritten URL.
- 📄 **Plain Markdown files were untitled** (GO-057) — a file without
  frontmatter had no title, so it appeared blank in every listing, navigation
  menu and `<title>`. The title now falls back to the document's own first
  heading (ATX or Setext). Frontmatter still wins.
- 🧩 **`partials/` was documented but never parsed** (DOC-014) — the theme
  structure in `docs/TEMPLATES.md` has always listed `partials/`, yet only the
  theme root and `layouts/` were parsed, so defines placed there were silently
  unavailable. `partials/*.html` now joins the same template set.

### Changed
- 🩺 **A misconfigured build says what is wrong** (UX-002) — an unknown YAML key
  is reported by name and ignored instead of vanishing (a config written for a
  newer ssg no longer looks like a missing value), and missing required
  settings are named along with the config file that was read and what it
  provided, instead of printing usage alone.

### Documentation
- 📘 **Template loading and sharing** (DOC-014) — `docs/TEMPLATES.md` now states
  which directories are parsed into the template set, how a theme shares its
  chrome through `partials/` + `dict`, what `base.html` actually is, and which
  theme directories are copied to the output.
- 📘 **Extra content sources and inferred values** — `docs/CONTENT.md` documents
  `content_sources` and the title/excerpt derivation rules; `docs/CONFIGURATION.md`
  documents `content_sources`, `link_rewrites`, `auto_excerpt` and the two new
  diagnostics; `docs/TEMPLATE_HELPERS.md` documents the arithmetic helpers.
- 📘 **Shortcode template scope** (issue #37) — `docs/TEMPLATES.md` now states
  what a shortcode template can see (`.Name`…`.Tags`, `.Data`, `.Attrs`,
  `.InnerContent`, `.Vars`) and what it cannot (`.Page`, `.Site`, `.Posts` —
  one instance may render on many pages), with the failure modes table.

## [1.8.9] - 2026-07-21

### Added
- 🗂️ **Watch-runner config paths** (GO-054) — the runner's own config file no
  longer has to sit in the project root: `--wrangler-config=FILE` and
  `--workerd-config=FILE` point the emulator at a config kept anywhere (e.g.
  `deploy/wrangler.toml`) and select that runner in the process, so
  `--wrangler`/`--workerd` become optional and flag order does not matter.
  `--watch-runner-config=FILE` is the runner-agnostic spelling for use with a
  custom `--watch-runner`, and `watch_runner_config` is the config-file key.
  `wrangler` and custom runners receive it as `--config <path>`, `workerd` as
  its positional config argument. A missing file warns instead of failing, and
  the spawned command line is now echoed on start.
- 📁 **Watch-runner working directory** (issue #35) — `--wrangler-dir=DIR`,
  `--workerd-dir=DIR` and `--watch-runner-dir=DIR` (config key
  `watch_runner_dir`) start the emulator in another directory, so a monorepo
  Worker in `booking/apps/api/` no longer fails with *"Missing entry-point to
  Worker script or to assets directory"* when `ssg` runs from the repo root. A
  relative runner config is anchored to ssg's own working directory first, so
  `--wrangler-dir` and `--wrangler-config` combine; a non-existent directory
  aborts the runner without killing the build.
- 🔤 **Environment variables in `external_sources`** (GO-055, issue #35) —
  `url`, `headers` and `query` now expand `$NAME`/`${NAME}` **inline**
  (`url: "$MY_API_BASE/api/accommodations"`), so one config switches between
  production and a local Worker instead of being generated per environment.
  `$$` is a literal `$`, and a `$` not followed by a variable name stays
  literal. `dsn`/`auth` keep the stricter whole-value form.
- 🧯 **Optional sources survive unset variables** (issue #35) — a source with
  `required: false` whose config references an unset (or empty) variable is now
  **skipped with a warning** instead of aborting the build, so a shared config
  can carry env-driven sources not everyone sets up. Required sources still
  fail, naming the variable.
- 🔓 **`allow_http` / `allow_private` in `external_sources.defaults`**
  (issue #35) — previously per-source only, and silently ignored under
  `defaults`. A source can still override either. The rejection message now
  says where the key may live.

### Changed
- 🎯 **`allowed_hosts` entries may carry a port** (issue #35) — `127.0.0.1:8787`
  now matches only that port instead of being rejected outright; entries
  without a port keep matching the host on any port. The error message states
  the rule.

### Security
- 🛡️ **Image decode format allowlist** (SEC-013) — `image.Decode` dispatches on
  magic bytes, and importing `disintegration/imaging` transitively registers the
  TIFF/BMP decoders, so a crafted TIFF renamed `photo.png` could reach imaging's
  transforms — the path that panics in CVE-2023-36308 (GHSA-q7pp-wcgr-pffx, no
  fixed upstream release). Decoded formats are now checked against
  jpeg/png/gif/webp before any pixel work, in both the image processor and
  `imageInfo`. `govulncheck` reported the vulnerable symbol as uncalled; this
  removes the residual path rather than relying on that.

## [1.8.8] - 2026-07-20

### Added
- ⚡ **Watch Runner Support** — added support for spawning background watch runners (emulators) alongside the file watch loop: `--wrangler` (executes `npx wrangler dev`), `--workerd` (executes `workerd serve`), or `--watch-runner="cmd"` (runs any custom command). Automatically coordinates execution and handles process output/cleanup.

### Fixed
- 🗂️ **Enriched YAML parsing errors** (issue #31) — if a YAML data file under `data/` fails to parse, `ssg` now scans the file for space-preceded hash characters (` #`) and prints precise line-number diagnostic hints to help debug unquoted comment issues.
- 🍺 **Homebrew tap was never updated after v1.7.14** (OPS-012) — the CI step
  authenticated to `spagu/homebrew-tap` with `AUTHORIZATION: bearer <PAT>`,
  which GitHub's git-over-HTTPS endpoint rejects with 401 (it expects Basic
  auth; that an invalid header also breaks *anonymous* clones of a public repo
  is what made this look like an expired token). Now uses
  `basic base64(x-access-token:<PAT>)`, the same form `actions/checkout` uses.
- 🔊 **Silent tap failures are now loud** (OPS-012) — tap publishing moved out
  of the `release` job into `.github/workflows/homebrew.yml`, which **fails**
  on a missing token, a failed clone/push, or missing checksums, and writes the
  outcome to the job summary. Previously every failure path was
  `::warning::` + `exit 0`, so releases from v1.7.15 through v1.8.7 reported
  success while Homebrew users stayed on 1.7.14 for a week.

### Added
- 🔁 **Manually runnable tap publish** (OPS-012) — `.github/workflows/homebrew.yml`
  accepts `workflow_dispatch` with a version input, so a failed tap publish is
  repaired by re-running that one workflow instead of cutting a new tag.
  Re-running the *release* is not a fix: it rebuilds the binaries and changes
  their published SHA-256 sums.

### Changed
- 🔖 `scripts/sync-version.sh` now syncs and drift-checks the **download URLs**
  in `packaging/brew/ssg.rb`, not just its `version` field — the old check
  passed while the file claimed `version "1.8.6"` with v1.7.13 URLs.
  Checksums stay owned by the workflow; they exist only after a release builds.

## [1.8.7] - 2026-07-15

Completion of 15 unfinished-feature findings from the 2026-07-15 audit round
(GO-053…GO-062, DOC-013…DOC-016, FE-011): half-wired flags, silent
degradations, and documentation that promised more than the code delivered.

### Added
- 📦 **Embedded starter themes** (DOC-013) — `simple` and `krowy` are now
  compiled into the binary with `go:embed` and extracted (HTML **and** assets)
  on first use, so `ssg my-blog simple example.com` finally matches the README
  Quick Start without a repository checkout. Unknown themes still scaffold the
  generic starter.
- 🧹 **Image-cache garbage collection** (GO-057) — `--images-gc`
  (`images_gc: true`) prunes cache entries the finished build no longer
  references; `--images-gc-dry` reports what it would reclaim. Runs after
  generation and never fails the build.
- 🔀 **HTTP external-source pagination** (GO-062) — `pagination:` per source
  with `mode: page` (incrementing query param) or `mode: link` (`Link
  rel="next"`), `per_page`, `start_page`, and a `max_pages` guard (default 10,
  max 1000). Pages aggregate into one JSON array; hitting the cap warns.
- 💬 **Movable Type comment import** (GO-058) — `movable_type.include_comments:
  true` imports visible (`comment_visible = 1`) comments into each entry's
  `.Extra["comments"]`. Previously the option hard-failed as "deferred".

### Changed
- 🧩 **Every value flag accepts both `--flag=value` and `--flag value`**
  (GO-053) — the space form used to leak silently into positional arguments, so
  `--deploy cloudflare` quietly skipped the deploy. Both spellings now share one
  parser; unexpected positionals warn, and a value flag with no value warns.
- 🎛️ **Alt-engine helper parity** (GO-054) — pongo2 exposes the SSG FuncMap as
  real filters and Handlebars as real helpers (reflection adapter); Mustache
  reports its logic-less limitation once. Helpers an engine cannot express fail
  loudly instead of the old passthrough/ignore/`recover` silence. New support
  matrix in `docs/TEMPLATES.md`.
- 🔢 **Fenced `` ```math `` blocks render** (GO-055) — they are rewritten to
  `$$…$$` display math before conversion, so detection and KaTeX injection
  agree. Docs corrected: inline `\(…\)` is not supported.
- 🔊 **Loud TLS/HTTP-3 degradations** (GO-056) — `--http3` without TLS, and
  incomplete TLS pairs (`--tls-auto` without `--tls-domain`, cert without key),
  now warn instead of silently serving plain HTTP.
- ⚙️ **`seo_off` honoured** (GO-059) — the deprecated config key now forces SEO
  off with a deprecation warning instead of being a silent no-op.
- 🧰 **`getExternal`/`getExternalMeta` work in shortcode templates** (DOC-016).

### Fixed
- 🔒 **Generic scaffold no longer leaks to Google Fonts** (FE-011) — the
  fallback template used a system font stack; no external CDN, neutral English
  copy, `lang="en"` (was Polish text with a `fonts.googleapis.com` link,
  contradicting the project's own privacy rule).
- 🗺️ **Cloudflare deploy error names the real flag** (GO-060) —
  `--deploy-project` instead of the non-existent `--cf-project`.
- 📖 **Docs/CLI discoverability** (DOC-014/DOC-015) — `--feed`, `--toc`,
  `--highlight`, `--paginate`, `--languages`, `--outputs`, `--check-links` and
  more are now in `--help` and the man page; README deploy table fixes
  (`VERCEL_ORG_ID` optional, SFTP needs `SSH_USERNAME`); Action `version`
  output documented.

### Removed
- 🧟 **13 dead legacy transform helpers** (GO-061) — the pre-PERF-005 tree-walk
  functions (`minifyOutput`, `injectSEO`, `convertToRelativeLinks`, … and
  one-shot `contentSignature`) were reachable only from tests; removed, with
  their tests re-pointed at the live string transforms.

## [1.8.6] - 2026-07-15

Fixes for the two open WordPress-migration issues.

### Fixed
- 🔗 **Heading anchor ids derive from visible text** (#26) — a heading
  containing a Markdown link leaked the href into its auto id
  (`### [Ian Zane](/authors/ian-zane/) — Generalist` →
  `id="ian-zaneauthorsian-zane--generalist"`). Link/image-bearing headings now
  get `slugify(visible text)` (`id="ian-zane-generalist"`), de-duplicated with
  `-N` suffixes; the TOC uses the same ids. **Backward compatible:** plain
  headings keep goldmark's ids bit-for-bit, so existing anchors never change —
  only the malformed link-bearing ids do.
- 🏷️ **Numeric WordPress tag ids resolve via metadata.json** (#27) —
  `tags: [1691]` produced a raw `/tag/1691/` archive even when the export's
  `tags` collection carried the term. Numeric tag values now resolve to the
  term name (like `author:` resolves via `users`), and those id-resolved tags
  archive under the export's canonical slug. **Backward compatible:**
  hand-written tag names keep their historical derived slugs, and unknown
  ids/plain names pass through unchanged — pre-1.8.6 tag URLs never move.

## [1.8.5] - 2026-07-15

Author-archive safety, define-shell template fallback and Hugo-compatible
string helpers (GO-050/GO-051).

### Fixed
- 🛡️ **Explicit content wins over auto archives** — a page/post/alias that
  already owns `/author/<slug>/`, `/category/…`, `/tag/…` or `/series/…` used
  to be **silently overwritten** by the auto-generated archive (archives render
  last). The archive is now skipped with a build warning, and suppressed
  archives stay out of the sitemap and slug maps used for feeds.
- 🛡️ **Define-shell templates no longer render blank pages** — copying
  `category.html` to `author.html` in a `{{define}}`-based theme left the
  define name unchanged, and the whitespace-only file-level template rendered
  a **blank archive**. Shells are now treated as absent (the category.html
  fallback applies, matching pre-1.8 behaviour) and the build prints a warning
  telling the author to rename the define. Applies to every template executed
  by file name (index/post/page/category/tag/series/author/taxonomy*).

### Added
- 🖼️ **Non-destructive WebP mode** — `webp_keep_original: true`
  (`--webp-keep-original`, action input `webp-keep-original`) emits each
  `.webp` NEXT TO its original instead of replacing it, so themes with
  hardcoded `.png`/`.jpg` references (favicons, logos, `og:image`) keep
  working while rewritten `<img>` references serve WebP. The default remains
  the historical replace-in-place behaviour.
- 🎬 **GitHub Action traceability** — the resolved ssg version is logged on
  every run (a `::notice::` when `version: latest` was used) and exposed as
  the `version` output; docs now recommend pinning `version:` for production
  deploys.
- 🧩 `hasPrefix` / `hasSuffix` template helpers — Hugo-compatible aliases of
  `startsWith` / `endsWith` (also in shortcode templates).
- 📖 Author archives documented in `docs/CONTENT.md`: the `users` block in
  `metadata.json`, `author:` accepting ID/name/slug, posts-only listings, the
  `author.html` → `category.html` fallback, the reserved `author` path and the
  new collision rule. (Migrating the author archive onto the generic taxonomy
  registry remains a documented deferred item.)

## [1.8.4] - 2026-07-14

Full internationalisation (audit/i18n-feature.md), dynamic taxonomies
(audit/taxonomies-feature.md), unified external sources
(audit/ssg-external-sources-implementation-plan.md) and built-in server access
control. Everything is opt-in; builds using none of it are byte-for-byte
unchanged.

### Added
- 🔌 **External sources — one registry** (`external_sources:`) exposing every
  source as `.ExternalData.<name>` (+ `.ExternalDataMeta`, `getExternal`/
  `getExternalMeta` helpers) with deterministic ordering, bounded concurrency,
  required/optional semantics, a unified error model (source/type/stage, never
  credentials) and env-only secrets (`"$VAR"`; literals rejected). `.Data`
  unchanged. Guide: `docs/EXTERNAL_SOURCES.md` + `examples/external-sources/`.
- 🔌 **File connector** — YAML/JSON/TOML/CSV/XML with transport-independent
  parsers, template-friendly XML mapping, size caps, sha256 checksums and the
  `transform.select` dot-path unwrapper.
- 🔌 **HTTP connector** — hardened client (HTTPS default, host allowlist with
  wildcards, private/loopback IPs blocked at dial time → DNS-rebinding safe,
  5-redirect cap with re-validation, response size limits, content-type
  validation, query-free identifiers), bearer/basic/header auth, retries with
  backoff on 5xx/429; shared disk cache (`<hash>.body` + `<hash>.meta.json`,
  TTL + stale-if-error, corruption eviction), offline mode with
  `fail_on_cache_miss`. CLI: `--offline`, `--refresh-external-sources`,
  `--clear-external-cache`, `--external-source=NAME`.
- 🔌 **SQL connector** — MySQL/MariaDB (go-sql-driver), PostgreSQL (pgx),
  SQLite (pure-Go modernc.org/sqlite); queries only in config, statically
  validated read-only (single SELECT/WITH statement), per-query `max_rows`
  (exceeding errors instead of truncating), query timeouts, DSNs scrubbed from
  errors.
- 🔌 **CMS adapters** — WordPress (posts/pages/custom post types, users,
  taxonomies → dynamic-taxonomy map, custom fields → `.Extra`, media), Drupal
  8-11 (nodes, bodies, vocabularies, users, `path_alias` preserved as links,
  dynamic `node__field_*` discovery) and Movable Type (released entries/pages,
  authors, categories, tags, assets). `mode: content` merges imports into the
  site before finalize (native URL/translation/taxonomy/collision treatment);
  `mode: data` feeds only `.ExternalData`.
- 🔒 **Server access control** (config-only) — `server_auth: basic` (users as
  `login:$PASS_ENV`, constant-time compare) or `jwt` (HS256 bearer tokens,
  single-algorithm by construction, exp/nbf honoured), `ip_allowlist`/
  `ip_blocklist` (IPs/CIDRs, checked before anything else), `rate_limit`/
  `rate_burst` per-IP token bucket (429 + Retry-After). X-Forwarded-For is
  deliberately not trusted.
- 🏷️ **Dynamic taxonomies** — declare any number of classifications in
  `taxonomies:`; `category`/`tag`/`series` are auto-registered and keep their
  legacy URLs, templates and feeds. Per-taxonomy config: `label/singular/path/
  field/multiple/archive/feed/sitemap/template/term_template/sort/
  case_sensitive/slugify/generate_empty`; names validated, paths unique,
  `author`/`page`/language codes reserved.
- 🏷️ **Frontmatter sources with priority** — generic `taxonomies:` map >
  configured direct field > legacy fields; multi-value merge + dedupe,
  single-value conflicts fail the build; generic `tag`/`series` values sync
  back onto the legacy pipelines.
- 🏷️ **Term normalization** — whitespace-collapsed, Unicode case-insensitive
  identity (opt-out via `case_sensitive`), first-seen display name, slug
  collisions and archive-vs-page URL collisions fail the build.
- 🏷️ **Term metadata** — `data/taxonomies/<name>.yaml`: display name, slug,
  description, `weight` (for `sort: weight`), free-form `data`;
  `generate_empty` renders metadata-only terms.
- 🏷️ **Archives** — `/technology/` index + `/technology/go/` term pages with
  template fallback chains (`taxonomy-<name>.html` → `taxonomy.html` →
  `archive.html` → `category.html`; `-term` variants for terms), pagination
  (`/page/N/`), i18n language buckets and prefixes.
- 🏷️ **Integrations** — sitemap entries (`sitemap: true`), Atom feed per term
  (`feed: true`), `taxonomies` map in the search index and JSON output.
- 🏷️ **Template helpers** — `taxonomies`, `taxonomy`, `taxonomyTerms`,
  `pageTerms`, `termURL`, `hasTerm`, `pagesByTerm`.
- 🏷️ Example project `examples/dynamic-taxonomies/` + guide `docs/TAXONOMIES.md`.
- 🌍 **i18n core** — expanded language config (`code/locale/name/timezone`) next
  to the legacy compact list; startup validation (duplicate codes, unknown
  default, bad timezones, policy values, fallback cycles) fails the build with
  descriptive errors. `translation_key` frontmatter (or a deterministic
  path-derived key) groups content variants; duplicates fail/warn per policy;
  output-path collisions (pages + aliases) fail the build.
- 🌍 **Language-aware routing** — configurable `prefix_default_language`;
  prefix logic centralised in `internal/i18n.Prefix` and applied to pages,
  posts, aliases, home pages, pagination, feeds, search indexes and JSON output.
- 🌍 **Translation dictionaries** — YAML/JSON catalogs in `i18n/` with nested
  keys, named `{{placeholder}}` interpolation, per-language fallback chains and
  `missing_translation` policies (warn default, error/empty/fallback).
- 🌍 **Template helpers** — `t`, `hasTranslation`, `translationURL`,
  `languageURL`, `localizeDate`; context: `.Site.Language/.Languages/
  .DefaultLanguage/.LanguagePages/.LanguagePosts`, `.Page.Lang/.Locale/
  .TranslationKey/.Translations` (with `IsCurrent`).
- 🌍 **SEO** — dynamic `<html lang>`, per-translation canonical, hreflang with
  `x-default` (falling back to the default-language root when a group has no
  default variant), sitemap XHTML alternates, `og:locale`+`og:locale:alternate`,
  JSON-LD `inLanguage`.
- 🌍 **Language-aware `.md` links (§13)** — the rewriter resolves the
  active-language translation, preserves explicit `file.<lang>.md` links,
  applies the `content_fallback` chain only when enabled, warns once per
  missing translation, and is deterministic (the previous flat map picked a
  random language for translated filenames).
- 🌍 Example project `examples/multilingual-site/` + full guide `docs/I18N.md`.

### Deferred (documented, follow-up phases)
- Language-scoped LEGACY taxonomy pages (categories/tags/authors/series remain
  cross-language; custom taxonomies ARE language-scoped), language selector +
  `t` labels in the built-in themes (output `<html lang>` is corrected at
  render time), localized month names in `localizeDate`, plural rules.
- Taxonomies: hierarchical terms, term aliases/redirects, translated term
  names, custom `path`/`template` overrides for the built-in
  category/tag/series pipelines, author archive on the generic registry.
- External sources (phase 7): Ghost/Strapi/Contentful/Sanity/Notion/Airtable/
  Google Sheets/GitHub/GitLab adapters, Drupal 7, Movable Type comments,
  direct-URL helpers (`getJSON`/`getCSV`/`getXML`), file-source `watch`,
  example CMS projects with seed scripts; MDDB on the connector interface.
- Server auth: SSO and LDAP (deliberately out of scope — too heavy for the
  built-in server), RS256/JWKS token verification.

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
- 🖼️ **Image processing in templates** (`audit/images-processing-feature.md`) —
  `imageInfo`, `imageResize` (scale/fit_width/fit_height/fit/fill), `imageCrop`
  (explicit rect, 9 anchors + compass aliases, focal points), `imageFilter`
  (grayscale/invert/sepia/brightness/contrast/saturation/gamma/blur/sharpen/
  opacity), `imageProcess` (ordered pipeline) and `imageSrcSet` (responsive
  variants). Deterministic content-addressed cache (`.ssg-cache/images/`) with
  atomic publishing into `processed_images/`; EXIF orientation normalized and
  metadata stripped; path traversal/symlink escapes rejected; decompression-bomb
  limits; animated GIFs error instead of silently flattening. JPEG/PNG pure Go
  (disintegration/imaging); WebP via the optional cwebp tool. Available in theme
  AND shortcode templates. Reference: `docs/IMAGES.md`.
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
[Unreleased]: https://github.com/spagu/ssg/compare/v1.8.10...HEAD
[1.8.10]: https://github.com/spagu/ssg/compare/v1.8.9...v1.8.10
[1.8.9]: https://github.com/spagu/ssg/compare/v1.8.8...v1.8.9
[1.8.8]: https://github.com/spagu/ssg/compare/v1.8.7...v1.8.8
[1.8.7]: https://github.com/spagu/ssg/compare/v1.8.6...v1.8.7
[1.8.6]: https://github.com/spagu/ssg/compare/v1.8.5...v1.8.6
[1.8.5]: https://github.com/spagu/ssg/compare/v1.8.4...v1.8.5
[1.8.4]: https://github.com/spagu/ssg/compare/v1.8.3...v1.8.4
[1.8.3]: https://github.com/spagu/ssg/compare/v1.8.2...v1.8.3
[1.8.2]: https://github.com/spagu/ssg/compare/v1.8.1...v1.8.2
[1.8.1]: https://github.com/spagu/ssg/compare/v1.8.0...v1.8.1
[1.8.0]: https://github.com/spagu/ssg/compare/v1.7.15...v1.8.0
[1.7.15]: https://github.com/spagu/ssg/compare/v1.7.14...v1.7.15
[1.7.14]: https://github.com/spagu/ssg/compare/v1.7.13...v1.7.14
[1.7.13]: https://github.com/spagu/ssg/compare/v1.7.12...v1.7.13
[1.7.12]: https://github.com/spagu/ssg/compare/v1.7.11...v1.7.12
[1.7.11]: https://github.com/spagu/ssg/compare/v1.7.10...v1.7.11
[1.7.10]: https://github.com/spagu/ssg/compare/v1.7.9...v1.7.10
