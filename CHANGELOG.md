# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [1.8.38] - 2026-08-16

### Added
- 🔢 **`.Pager.Pages` — numbered pagination** (#156) — a pager carried only its
  neighbours, which draws "← →" and not the control most themes have: a reader
  on page six reached page two by stepping back four times. Every page now
  arrives with its number, address and whether it is current, so a theme renders
  the source site's own `.page-numbers` markup. **`.Pager.Window N`** returns the
  windowed form — first, last, N either side of the current page, with an
  ellipsis entry per gap — which is what WordPress draws. A template could not
  build these itself: `PrevURL`/`NextURL` are opaque, and deriving `/page/4/`
  from them means guessing a URL shape that `posts_page` and language prefixes
  already change. `Current`, `Total`, `PerPage`, `PrevURL` and `NextURL` are
  untouched.

### Fixed
- 📌 **A pinned post leads the listing** (#155) — WordPress lets an editor pin a
  post to the top of the blog and the export carries `sticky: true`, but the
  generator never read the field, so listings sorted by date alone and the
  pinned post landed wherever its date put it: sixth of ten on the site that
  reported it, while the source showed it first. Pinned posts now come first —
  in their own date order among themselves — on the index, the posts page and
  term archives, which is what the source CMS does. The flag also reaches
  templates as **`.Sticky`**, so a theme can mark that post the way WordPress
  marks it with a `sticky` class. A site that pins nothing keeps the order it
  has always had.


## [1.8.37] - 2026-08-16

### Fixed
- ⚓ **Heading anchors come from the heading's text, not its markup** (#153) — a
  heading wrapped in `<strong>` or a coloured `<span>` got
  `id="span-stylecolor-ffff00strongwhy-baby-swimmingstrong"`, because goldmark
  derives auto-ids from the raw source line. The anchor a reader, a table of
  contents or an inbound link expects is `#why-baby-swimming`. Headings holding
  markup are ordinary in CMS content — 15 to 37 on the front page alone of each
  of six migrated sites. Plain headings keep their existing ids bit-for-bit, so
  anchors on existing sites stay valid; the rule now covers inline HTML the way
  it already covered links and images (#26).
- 🚩 **An option ssg does not accept is named rather than ignored** (#152) —
  `--output=public` reads like it should work (the config key is `output_dir`),
  and the build wrote to `output/` and said nothing, which looks like the flag
  was honoured and the site landed elsewhere. Unknown options now warn and, when
  there is an obvious neighbour, suggest it: *Unknown option --output — ignored
  (did you mean --output-dir?)*. It warns rather than fails, so a script passing
  an option a future ssg will understand keeps working. The check reads the
  parsers' own tables, so a new flag cannot drift out of it.
- 🧱 **Structured frontmatter survives the MDDB content source** (#154,
  tradik/mddb#187) — MDDB stores meta as a flat `key → []string` by design, so a
  `faq:` list of objects or a `schema:` object can only travel as a string.
  Neither end worked: a producer that JSON-encoded correctly still handed the
  theme a JSON *string*, and one that stringified a Go map stored
  `map[answer:… question:…]`, which made **every post** fail with
  `can't evaluate field question in type interface {}` — an error pointing at
  the theme rather than at the field. ssg now decodes JSON-shaped meta values
  into the shape a template can range over, and names the document and field
  when a value is a printed Go map, which cannot be recovered. A value that is
  neither arrives exactly as before.
- 📚 **`paginate` now applies to category archives** (#149) — it paginated the
  index and left every category whole, so a migrated site's `/category/blog/`
  shipped 205 articles in one file while `/category/blog/page/2/` did not exist.
  Category archives are chunked exactly like every other term archive, each page
  carrying its own pager. A site without `paginate` still gets one file per
  archive, unchanged.
- 📰 **`posts_page` wins over a page of the same address, and says so** (#150) —
  WordPress's "Posts page" *is* a page: the admin assigns an existing one and
  WordPress renders the loop in its place. An export carries both faithfully, and
  ssg wrote the page at `/blog/` while writing the listing's *second* page to
  `/blog/page/2/` — so page one of a listing served an empty document and page
  two served the listing. Two of six sites in one batch hit it. The setting now
  wins, matching the source CMS and what the operator asked for, and the build
  names both documents instead of silently choosing.
- 📦 **The snap's bundled exporter can no longer go stale unnoticed** (#148) —
  it is built from the exporter's latest release *at build time*, so a fix
  released downstream reached nobody until ssg was rebuilt. The Snap workflow now
  also runs weekly, which puts a ceiling on how far behind the bundled engine can
  drift; the migration report already names the engine and where it came from.


## [1.8.36] - 2026-08-16

### Fixed
- 🧩 **An archive no longer vanishes over one missing field** (#145) — every
  archive view (category, tag, author, series, custom taxonomy) now arrives with
  the same set of keys, passed as a map. A template reading a field this kind
  does not fill gets nil instead of a hard error, so `{{if .Pager}}` is
  expressible and one theme file can legitimately render four kinds of archive.
  Six migrated sites had lost **every** category, tag and author archive to a
  single `.Pager` reference, with only a warning per term in a long log.
- 📄 **Archives carry their `Pager`** (#145) — it was computed per chunk and
  simply never handed to the template, so a paginated archive could only ever
  show its first page. Unpaginated archives get a truthful one-page pager rather
  than a zero value that renders as "page 0 of 0".
- 🕳️ **A missing page looks missing in the dev server** (#146) — a directory
  with no `index.html` used to answer **200** with a `<pre>` list of file names;
  posts with dated permalinks live in `/2014/05/`, so a missing date archive
  looked like a working page serving 776 bytes of filenames. It now answers
  **404**, the way every host this project deploys to does.

### Tests
- 🧪 **The approve-then-PR flow is tested end to end** — `openGitHubPR` had no
  coverage at all: it hardcoded the API host, so the request, the auth header
  and every failure path could only be exercised by opening a real pull request.
  A base-URL seam makes all of it testable against a local server. Also covered:
  the date-archive guards, `loadMetadata`'s failure paths, `ssg migrate`'s own
  flag forms and the tar writer's entry kinds.
- 📊 **Coverage now measures the code this project writes** — `codecov.yml`
  excludes `internal/mddb/proto`, 2,760 statements of protoc output that nobody
  maintains and nobody should test. Counting it put the reported number ~16
  points below the truth and made the metric move when the schema changed rather
  than when the code did. Hand-written code sits at **96.4%**.

### Added
- 📅 **Date archives** (#146) — `date_archives: true` generates `/YYYY/`,
  `/YYYY/MM/` (and `/YYYY/MM/DD/` where the permalink structure carries the day)
  from the posts already loaded, rendered by `category.html` with `Kind: "date"`
  and a label a theme can title the page with ("May 2014"). WordPress publishes
  these and links to them from every byline, widget and sitemap — on one migrated
  site they were 60 of 477 sitemap URLs. **Opt-in**: a site that never had these
  URLs does not grow them because it upgraded, and real content that already owns
  such a path keeps it. `ssg migrate` turns the key on, because a migrated site's
  own content links to them.


## [1.8.35] - 2026-08-15

### Added
- 💬 **Migrated comments are rendered** (#142) — `comments.json` reached the
  project and stopped there: counted in the report, then ignored. The generator
  now loads it and hands each page its own thread as **`.Comments`**, nested by
  parent and in the order the comments were written. Matching is a lookup on the
  URL the export recorded, not a heuristic. The bundled `simple` theme renders
  the thread on posts. They are rendered statically because that is what they
  are — historical and closed; accepting NEW comments stays the comments
  worker's job.
- 🏠 **`.IsFrontPage`** (#141) — a theme could not tell it was rendering the
  front page, so the site's most-linked document came out titled "Home - Site"
  while the source served the site's own name and tagline. The generator already
  knew; now the template data says so, instead of a theme comparing `.Link`
  against `"/"` and hoping no language prefix is in play.
- 🧭 **The bundled themes render the site's navigation** — deferred from 1.8.34
  on purpose: a theme file is read by whichever ssg the reader runs, so it may
  only use fields the *previous* release already had. 1.8.34 shipped
  `.Site.Menus`, so the themes can use it now.
- ⚙️ **The migration report names the engine that ran** (#140) — including
  `(bundled with the snap)`, the one line that explains why refreshing the
  host's `wpexporter` changed nothing.

### Fixed
- 🗂️ **Category archives honour the address the source site serves them at**
  (#143) — WordPress lets a site drop the `/category/` base, and many do. The
  export records each term's real address in `link`; ssg ignored it and built
  `/category/<term>/`, so every archive link in every post's meta line, in the
  menu and in the breadcrumbs was a 404 on the migrated site. Archives now
  render where the source serves them and **the built-in path becomes a 301**.
  A site whose export carries no `link` keeps the layout it has always had.


## [1.8.34] - 2026-08-14

### Added
- 🧭 **A migrated site keeps its navigation** (#132) — menus are the one part of
  a site that cannot be derived from its content: nothing in a page records
  which menu it belonged to, in what order, or under what label. WordPress
  gates them behind `edit_theme_options`, so `ssg migrate` now takes
  **`--auth-user`/`--auth-pass`** or **`--auth-token`** and forwards them to the
  engine — never writing them into `.ssg.yaml`, which is a file people commit.
  Menus reach templates as **`.Site.Menus.<location>`** (or `.<slug>`), with
  `.Tree` giving the entries nested and ordered as the source site rendered
  them; an item whose parent is missing is promoted rather than dropped, and a
  tangled parent chain cannot hide entries. The bundled themes will render the
  `primary` menu from **1.8.35**: a theme file on disk is also read by an OLDER
  binary — the GitHub Action downloads a released ssg — and a field that binary
  does not know is a hard template error, not a blank nav, so the theme can only
  use a field the previous release already had.
- 🗂️ **The theme's own post types are selectable** (#130) — `--custom-types
  cpt_services,cpt_team` picks them, `--no-custom-types` skips them.

### Fixed
- 🗂️ **Nested categories render where the source site served them** (#138) —
  WordPress nests categories and serves a child at
  `/category/<parent>/<child>/`; ssg flattened it to `/category/<child>/`, so
  every surviving link — a menu copied from the old theme, a bookmark, a search
  result — hit a 404 while the content sat one path away. The nesting was
  already in `metadata.json` and was being dropped at render time. Archives now
  follow the parent chain, and **the old flat path becomes a 301** so nothing
  that already points at it breaks. A taxonomy with no nesting is untouched: the
  same URLs, no redirects.

- 🚑 **`--content` no longer kills the migration on a current engine** (#137) —
  1.8.33 made `comments` a selectable kind, so any `--content` list that did not
  name it emitted `--no-comments`, a flag that arrives in **wpexporter 1.8.5**
  (unreleased; 1.8.4 is current, including the copy bundled in the snap). Every
  selective migration died with `unknown flag: --no-comments` **after** the
  project was scaffolded and, in live mode, after the server was up. The engine's
  version is now read once and the flag sent only to an engine that knows it;
  otherwise the comments come along and the report says so. An unreadable version
  banner counts as old: skipping a flag costs a line, sending an unknown one costs
  the run.

### Changed
- 🔎 **A site that arrives without navigation says why** (#132) — an export with
  no menus used to end in a clean-looking summary, leaving the operator to
  discover the missing nav in the browser. The report now names the cause and
  the fix, and distinguishes an anonymous run from one whose credentials were
  accepted but returned nothing.


## [1.8.33] - 2026-08-14

What a migration was still leaving on the old site — the readers' comments —
and the dev server giving up on a port instead of taking the next one.

### Added
- 💬 **Migrations carry the site's comments** (#134) — `--content …,comments`
  was accepted and then reported as undeliverable, so every migration left the
  one content a site owner did not write and cannot re-create behind. With
  wpexporter 1.8.5+ the kind is real: comments land in
  `content/<source>/comments.json`, addressed by **page URL** rather than by a
  WordPress post ID that means nothing after a migration, sorted so a reply
  never precedes the comment it answers — the shape the
  [comments worker](docs/WORKERS.md)'s D1 table expects. The migration report
  states how many arrived.
- 🔌 **The dev server takes the next free port instead of giving up** (#135) —
  `--port 8889` with something already on 8889 ended the run with `bind:
  address already in use` *after* the site had been generated, which for
  `ssg migrate --watch --http` meant migrating the whole site again. The port is
  now a preference: the server walks forward (8889 → 8890 → …, up to 64 ports)
  and announces where it landed. `--port=0` still means "any free port", and the
  claim happens before the address is printed, so `ssg migrate` and `ssg mcp`
  never announce a port they did not get. Failures a different port cannot fix —
  an unusable `--host`, a privileged port — still stop immediately.
- 🔧 **`ssg migrate` accepts `--host` and `--port`** (#135) — live mode *is* the
  dev server, but `migrate` parses its own flags, so `--watch --http --port
  8889` was rejected as an unknown flag after the project had already been
  scaffolded. A port that is not a number is reported rather than quietly
  falling back to 8888.

### Changed
- 🧭 **`--content` says what it costs** (#134) — the help and
  [docs/MIGRATE.md](docs/MIGRATE.md) now state that naming `--content` at all
  opts every *unlisted* kind out, which is how `--content pages,posts,media`
  quietly left a theme's own post types and the readers' comments behind. And
  passing wpexporter's own flags (`--no-custom-types`, `--no-comments`) to
  `ssg migrate` is answered with the `--content` equivalent instead of a bare
  "unknown flag".

### Security
- 🔐 **golang.org/x/image 0.45.0** (was 0.44.0) — GO-2026-6222; govulncheck
  reports it reachable from this code, which decodes user images on every build
  with `webp`/`avif` output or responsive variants. `golang.org/x/text` came
  along to 0.41.0 as its dependency.

### Tests
- 🧪 `internal/migrate` back to **100%** — the port walk's fallbacks (a listener
  whose address is not TCP, an unparseable one, an unchanged port) and the
  "recognised but undeliverable kind" path, which went dormant when comments
  gained a real export. It is exercised by declaring such a kind for the length
  of a test, so the machinery that keeps the next one from being dropped in
  silence stays honest instead of rotting until someone needs it.

## [1.8.32] - 2026-08-14

### Changed
- 🖼️ **Migrations download only the media the content points at** (#130) — a
  WordPress library keeps every crop of every image ever uploaded, plus whatever
  a since-removed plugin left behind, so a migration that took all of it copied
  gigabytes the new site never links to. Featured images and in-content media
  still arrive (with their size variants); **`--all-media`** takes the whole
  library, as before.

### Fixed
- 🧮 **An `analytics` block with unusual values no longer fails the build**
  (#131) — a migrated site did not build *at all*: wpexporter reports every id
  it finds per vendor, so `"analytics": {"google_tag_manager": ["GTM-…"]}` met a
  `map[string]string`, and since metadata.json is read before anything else the
  whole build died — no pages, no posts, no menu — over an optional block that
  renders only when `analytics: true`. An export may report a vendor's id as a
  list (the same container found in the head and again in a plugin's footer) or
  as a number, so the block is now read leniently: the first usable id per
  vendor wins, a value that cannot be read as an id is dropped, and only a
  malformed block — one that is not an object — is an error. A tracking id the
  generator does not understand is not worth a site that does not build.


## [1.8.31] - 2026-08-13

### Added
- 🩹 **`ssg repair`** — finds source Markdown whose markup is indented four
  columns or more, which CommonMark renders as a literal code block, and with
  `--fix` rewrites it in place. This is what a WordPress page builder leaves
  behind: Elementor indents its nested `<div>`s with tabs, the exporter turns
  `</p>` into a blank line, the blank line ends the HTML block — and the visitor
  reads `</div>` in monospace down the middle of the page while the build reports
  success. Dry run by default (exit 1 on findings, so CI can gate on it); front
  matter, fenced code blocks and list continuations are never touched.
- 🔎 **`check_markup`** reports the same defect on every build, naming the source
  file and line. It is the one check that is **on by default** (`warn`): the
  others weigh a judgement call, while this one reports content that provably
  does not render as written. Silent when there is nothing to report. Turn it off
  with `check_markup: ""` or `--no-check-markup`; `strict` fails the build.
- 🪪 **`title`, `description` and `colors` configuration keys**, reaching
  templates as `.Site.Title`, `.Site.Description` and `.Site.Colors.<role>`. The
  palette is also emitted as CSS custom properties (`--ssg-color-primary`, …) on
  `:root`, and stands in for `<meta name="theme-color">` when neither the theme
  nor the export declared one. Values crawled from another site are validated
  before they reach the stylesheet.
- 🚚 **`ssg migrate` completes the config from the export.** After the fetch it
  reads `metadata.json` and fills in `title`, `description`, `timezone` and
  `colors` — what the source site says about itself, which the migration already
  collected and then dropped on the floor. It only ever fills in keys the config
  does not have, so an author's decision is never overwritten, and the file is
  edited as a YAML document, so comments and key order survive. The first build
  of a migrated site now carries its own name and palette. Needs wpexporter
  1.8.2+ for the palette; earlier exports simply have no colours to read.

- 📦 **`ssg migrate` stopped hauling the whole media library (#130).** It now
  passes `--relevant-media-only`, so a migration takes the files the content
  actually references: on one real site 359 files / 12 MB instead of 5,255 /
  197 MB, of which only 74 were ever referenced — and ssg generates its own
  responsive variants anyway. `--all-media` opts back into everything.
- 🧩 **`--content custom`** selects the post types a theme or plugin registered
  (Services, Portfolio, Team — wpexporter 1.8.2+). They are content, so a
  `--content` list that does not name them turns them off like any other kind
  (#130).

### Security
- 🔐 **Go 1.26.6** (was 1.26.5) — govulncheck reports seven standard-library
  advisories reachable from this code on 1.26.5, among them GO-2026-5972
  (`encoding/asn1`, reached through `ssh.ParsePrivateKey` in the SFTP deploy)
  and GO-2026-5026 (`net/http` punycode handling, reached by every outbound
  request: MDDB, theme downloads, the proxy endpoint). All are fixed in 1.26.6.
  Building from source now needs Go 1.26.6 or newer.

### Fixed
- 🔇 **A config that could not be completed said nothing** — when `ssg migrate`
  found the site's title, description or palette but failed to write them back
  (a read-only config, a full disk), it reported no keys applied and no reason,
  leaving the operator to look for values the file never got. The failure is now
  named on stderr; the migration itself still succeeds.
- 📄 **Migrated pages that rendered their own markup as text.** The root cause is
  in the exporter (fixed in wpexporter 1.8.2), but existing content is repaired
  in place with `ssg repair --fix` rather than requiring a re-migration.


## [1.8.30] - 2026-08-12

### Fixed
- 🧭 **`--content` no longer strips a migrated site of its navigation** —
  `ssg migrate wordpress <url> --content pages,posts,media` used to disable
  menus, tags and users too (anything not listed), so the site came up without
  a menu, without category names and without authors. Site **metadata** (tags,
  users, menus) now always ships; `--content` selects *content*. Drop it
  explicitly with `no-menus` / `no-tags` / `no-users`.
- 👁️ **`ssg mcp --http` actually serves the site** — the flag was parsed into
  the config and then ignored, so an assistant reworked a theme nobody could
  look at. The preview now starts with the MCP server, announces its address on
  stderr (stdout belongs to JSON-RPC) and refreshes on every MCP rebuild —
  including the build-error overlay.
- 🔤 **HTML entities in titles, excerpts and descriptions** — a WordPress
  export carries them legitimately (the REST API serves `title.rendered` as
  HTML), so "Domowe Kino &#8211; Warszawa" reached `<title>`, meta
  descriptions, og:title, feeds and JSON-LD verbatim. Plain-text frontmatter
  fields are now entity-decoded at parse time. Body markup is untouched: there
  the entities are the author's.

### Added
- 🎯 **The site's marketing wiring now survives a migration** — `metadata.json`
  may carry `marketing` (favicon, apple-touch-icon, theme colour, `og:site_name`,
  default `og:image`, `twitter:site`, social profiles, search-console and
  business-manager verification tokens) and `analytics` (GTM, GA4, …), written
  by wpexporter's crawl. ssg loads both, exposes them to templates as
  `.Site.Marketing` / `.Site.Analytics`, and — with `seo: true` — injects the
  icons, social defaults and verification tags the theme did not already
  provide. Tracking snippets need the new **`analytics: true`** key: running
  third-party JavaScript on every page is the site owner's decision, not a side
  effect of migrating content. The build prints one line naming what it
  inherited.

### Changed
- 🎯 **Migrations now ask wpexporter (≥ 1.8.1) for the SSG format** —
  `--ssg-sections` emits the `## Excerpt` / `## Content` markers this parser
  reads and drops the leading H1 that duplicated every page's title, and
  `--assisted-crawl` fills `metadata.json`'s `marketing` and `analytics`
  blocks (GTM/GA4 ids, verification tokens, social profiles, favicon,
  og:image) — data a migration cannot reconstruct afterwards. Skip the extra
  crawl with **`--no-crawl`**.
- 🤖 **Migration and `ssg mcp` now print how to connect an assistant** — the
  MCP server speaks stdio, so the client launches it; "run `ssg mcp`" left
  people with a server nobody talked to. Both now print the registration line
  (`claude mcp add ssg -- ssg mcp`) and a Claude Desktop JSON snippet carrying
  the project directory.

## [1.8.29] - 2026-08-12

### Fixed
- 📦 **`ssg migrate wordpress` in the snap** (#114) — the snap now bundles the
  `wpexporter` engine, so migration works out of the box after
  `snap refresh static-site-generator`. Previously it always failed with
  "wpexporter not found in PATH" even when the tool was installed on the host:
  strict confinement sees only the snap's own rootfs, and installing the
  `wpexporter` snap cannot help either, because a snap may not execute another
  snap (`cannot join mount namespace of pid 1`). Same treatment `cwebp` already
  gets for WebP output.
- 📝 **Wrong install command** — the missing-engine error and
  [docs/MIGRATE.md](docs/MIGRATE.md) pointed at
  `go install github.com/tradik/wpexporter@latest`, which cannot work: the
  module root is not a main package. The correct path is
  `github.com/tradik/wpexporter/cmd/wpexporter@latest`. Inside a snap the
  message no longer suggests installing anything by hand — it says the engine
  ships with the snap and the snap needs refreshing.

## [1.8.28] - 2026-08-12

### Added
- 🚚 **`ssg migrate <provider> <url>`** (GO-100) — one command from a live site
  to a working SSG project. The built-in `wordpress` provider orchestrates
  [wpexporter](https://github.com/tradik/wpexporter) (discovered on `PATH`
  like `cwebp`; a missing binary explains how to install it), scaffolds the
  project when none exists (never overwriting files), pulls the selected
  `--content` kinds (`pages,posts,media,tags,users,menus,products`) into the
  native content model and builds. With **`--watch --http`** the migration
  runs LIVE: the server starts first and prints its address, content lands
  incrementally while auto-reload shows the site filling up in the browser,
  and the final report points at `ssg mcp` for an AI-driven template rebuild.
  Requested-but-unsupported kinds (`comments`) are reported as skipped, never
  silently dropped. See [docs/MIGRATE.md](docs/MIGRATE.md).

### Fixed
- 🛡️ **`migrate` no longer falls through to positional arguments** — before,
  `ssg migrate wordpress https://example.com` was silently parsed as
  `<source> <template> <domain>`, pretended to build and started the server
  on a nonexistent project. The verb is now fully claimed; a source directory
  literally named `migrate` still builds via `--source=migrate`.

### Tests
- 📈 Project coverage raised from 76.8% to 79.9% (owner's orders: +2pp) —
  branch-level tests across `cmd/ssg` (live-reload hub and SSE, server
  access-control and TLS listener modes, form-endpoint delivery failures,
  cache CLI error paths, archive error paths, bare `--check-*` flags),
  `internal/generator`, `internal/fetch`, `internal/config`,
  `internal/externalsource`, `internal/endpoints`, `internal/images` and
  `internal/mcp`; new `internal/migrate` lands at 98.8%.

## [1.8.27] - 2026-08-12

### Added
- 🗃️ **Unified build cache** (GO-091) — one engine (`internal/cache`) under every
  disk cache, one root (`.ssg-cache/`), and a new CLI: **`ssg cache stats`**
  (entries/size per namespace), **`ssg cache clean [--namespace=X]`** and
  **`ssg cache gc [--dry]`** (reclaims expired external-source entries; image GC
  stays with `--images-gc`, which owns the build manifest). External-source
  cache writes are now atomic (temp+rename), so a crash mid-write can no longer
  leave a torn payload. The AI cache moved from `.ai-cache/` to
  `.ssg-cache/ai/` with a read-fallback and **migrate-by-copy** — existing AI
  results are preserved byte-for-byte, never regenerated. Image cache keys are
  golden-tested and unchanged: no reconversion storm on upgrade.

## [1.8.26] - 2026-08-11

### Fixed
- 🔗 **markdown_publish flat URLs** (#116) — posts carrying a flat `link:
  /slug.html` (typical of a WordPress export) now publish a `/slug.md` twin,
  appear in `llms.txt`, and get a `<head>` alternate pointing at that file
  instead of a non-existent `index.md`. No more self-reported "broken link →
  index.md" warnings; the alternate is suppressed when no Markdown copy exists.
- ✂️ **frontmatter `excerpt:`** (#115) — a frontmatter `excerpt:` now reaches
  `page.Excerpt` (section `## Excerpt` still wins, then frontmatter, then
  `auto_excerpt`), so migrated content stops shipping empty meta descriptions,
  card summaries and feed summaries. `auto_excerpt` also derives a summary from
  content that opens with a raw-HTML block (e.g. WordPress `<p class="wp-block…">`).
- 🖼️ **root `.Content` renders** (#118) — in page/post templates `.Content` is
  now the rendered HTML, so `{{ .Content }}` no longer ships raw Markdown; and
  `safeHTML` accepts an already-rendered `template.HTML` value, so both
  `{{ .Content | safeHTML }}` and `{{ .Post.Content | safeHTML }}` work.
  Sanitisation (`--sanitize-html`) is preserved.
- 📦 **snap: WebP under strict confinement** (#114) — the snap now bundles
  `cwebp` (and `avifenc`) via `stage-packages`, so `webp: true` works without the
  host's tools, which strict confinement hides. The missing-tool error explains
  the snap case.

- 🏷️ **GitHub Action `@v1` stale** (#120) — the release workflow now re-points the
  floating major tag (`v1`) to each release's commit, so `spagu/ssg@v1` always
  ships the current `action.yml`. Inputs added later (e.g. `config`) are no longer
  silently dropped for `@v1` users.

### Documentation
- **Template helpers** (#117) — corrected `sortBy` → `sort` in the engine-support
  table; documented that `sort` needs a slice or string-keyed map and pointed
  int-keyed taxonomy maps (`.Site.Categories`) at `taxonomyTerms`; documented the
  `search-index.json` schema.

## [1.8.25] - 2026-08-11

### Added
- 🤖 **Markdown for agents** (`markdown_publish`, GO-085) — publishes a clean
  Markdown copy of every page in both agent-friendly locations (`/page/index.md`
  and the flat `/page.md`), links it from the page `<head>` as a `text/markdown`
  alternate, and writes a root `llms.txt` index. SSG is Markdown-native, so the
  published copy is the authored source, not an HTML round-trip — ideal for
  language models and agents (including ChatGPT Search) that consume Markdown.
- 🧹 **`clean_special_chars`** (GO-086) — normalises the "smart" Unicode AI tools
  emit (curly quotes, en/em dashes, ellipsis, non-breaking and zero-width
  spaces) to plain ASCII across all rendered content. Targets a fixed Western
  allowlist only: **Chinese, Japanese, Korean and every other script — and CJK's
  own full-width punctuation — pass through untouched.** Opt-in.
- 🔤 **`output_encoding` / `output_encoding_sections`** (GO-087) — choose the
  text-output encoding (`utf-8` default, `utf-16le`, `utf-16be` with BOM),
  globally or per content section (longest-prefix, like `schema_defaults`). All
  encodings are Unicode, so CJK round-trips losslessly.
- 🗂️ **`home_pages_limit` / `home_posts_limit`** (GO-088) — cap the guide and
  post cards on the home page (default 6) with a "see all" link, so the landing
  page stays uncluttered.
- 🕷️ **`robots_rules`** (GO-089) — explicit per-crawler `robots.txt` directives,
  so a site can spell out its policy for AI and search crawlers (GPTBot,
  OAI-SearchBot, Googlebot…). Empty keeps the allow-all default.

## [1.8.24] - 2026-08-09

### Added
- 🧩 **`check_schema`** (#111) — validates the JSON-LD each page emits against the
  properties search engines require, alongside the existing `check_*` validators
  and with the same `warn`/`strict` modes. Incomplete structured data is
  rejected silently from the author's side: the build succeeds, the page ships,
  the rich result never appears, and the feedback arrives weeks later in Search
  Console. Nested objects are checked too — an `Offer` missing `priceCurrency`
  invalidates the `Product` around it. An unrecognised `@type` passes silently,
  so the generality `schema:` exists for is not taken away, and only required
  properties are checked, because warning about optional ones trains people to
  ignore the warning.
- 🏷️ **`schema_defaults`** (#110) — structured-data defaults per content section,
  so a section carries its `@type` without every file repeating it. Site-wide
  `schema:` cannot: it applies to every page, so a home-page
  `SoftwareApplication` would stop each post being a `BlogPosting`. That left the
  type to be written into each file — a hundred recipes meant a hundred copies.
  Keys match the page's directory relative to the source folder by prefix,
  longest first (the same rule as `link_rewrites`), with `home` reserved for the
  site root. Precedence: `schema:` < derived < `schema_defaults` < frontmatter —
  section defaults outrank the derived type because overriding it is the point,
  while a page's own `schema:` still wins over its section.

### Changed
- 🧭 **The guide sidebar follows the reading path** — it listed pages in
  `.Site.Pages` order, which is by title, so "SSG Installation Guide" landed in
  the middle. Order now comes from `variables.docs_nav_order`; a guide missing
  from that list still appears, after the ordered ones, so a new file is never
  hidden by forgetting to add it.

### Documentation
- 🖼️ **Themes get a gallery** — `docs/TEMPLATES.md` is split into the themes you
  can use as they are and how to write one. The four bundled themes are cards
  with real screenshots, built and captured from this repository, and `imd` is
  listed at all for the first time. Downloading a theme gets its own section,
  including that an archive laid out under `layouts/` — the conventional Hugo
  shape — is converted during extraction.
- 🧭 **A sidebar on every guide** — landing on a guide from search left no way to
  reach the others except returning to the home page: the header carries a
  hand-picked few and the full set existed only in the index card grid. Built
  from `.Site.Pages`, so a new file appears with no edit, and the current page
  carries `aria-current`.
- 🏷️ **Three flags reached `--help`** — `--strict`, `--route-manifest` and
  `--notify` were accepted by the parser but absent from the help output, so the
  only way to find them was the source. Errors in this CLI point at `--help`,
  which makes an unlisted flag effectively absent. Found by auditing the parser
  against the help text; the reverse direction and all 198 config keys, 66
  template helpers and 5 subcommands came back clean.
- 📄 **`toJSON` documented** — the helper `ssgtheme` uses to hand a config block
  to a client script, and the one pattern the cookie-consent worker recommends.

### Changed
- 🧱 **Home page rewritten** — the H1 leads with the category and project name
  rather than a tagline, since it is the page's one indexable heading; the
  benefit line moves directly beneath it. A three-pillar section covers build
  speed, direct database reads and build-time AI, each with a figure that is
  checkable from this repository. `Install` joins the navigation in second
  place — it is the first thing a new visitor does.

### Documentation
- 🤖 **`docs/MCP.md`** — a reference for the `ssg mcp` server aimed at AI agents:
  both roles with every tool and its CAN/CANNOT contract, the presentation-key
  allow-list, watch mode as the feedback loop, and the branch → commit → PR
  write-back. MCP was previously described only inside the configuration
  reference and a blog post, so an agent had no page to read before deciding
  whether the server could do what it needed.

### Fixed
- 🏠 **The home page emitted no structured data** (#109) — `seo: true` injects
  JSON-LD, OpenGraph and hreflang for posts and pages, but the render transform
  applies the SEO block only when a page context exists and the index was
  rendered without one. So the page a crawler reaches first, and the only
  sensible home for site-level types, had nothing — and `derivedLD`'s `WebSite`
  branch was unreachable, since the sole page that selects it never arrived.
  Site-wide `schema:` defaults did not reach the home page either. The same
  shape was fixed for feed autodiscovery in #86; this closes it for SEO. A
  paginated `/page/2/` declares its own URL rather than being canonicalised onto
  page one.

## [1.8.23] - 2026-08-08

### Fixed
- 🧨 **`--minify-js` deleted code inside string literals** (#106) — comment
  stripping was a pair of regexes, which cannot tell a comment from the same
  characters in a string. `return "/*" + "x" + "*/"` minified to `return ""`:
  the scan started at the `/*` in the first literal and ran to the `*/` in the
  third, taking the closing quote with it. That example still parses, so the
  build reported success and the behaviour changed silently; on a vendored
  bundle whose CSS-comment parser holds `/*` in strings, the output had an
  unterminated string and the browser refused the file — with minify,
  fingerprint and every `check_*` validator passing. Replaced with a scanner
  that tracks strings, template literals (including `${}`) and regex literals,
  and that keeps a comment whenever it cannot be sure. CSS gets the same
  treatment, since `content: "/*"` is legal there.
- 🔗 **`rewrite_md_links` rewrote absolute external URLs** (#107) — any href
  ending in `.md` was matched on its basename, so a link to a page's own history
  on GitHub became a link to the page containing it. `check_links` passes, since
  the target exists, so nothing reported it. Links with a scheme or a `//`
  prefix are now left alone: an absolute URL to another host cannot be an
  in-repository link.
- 🧭 **`rewrite_md_links` and `check_orphans` ignored `pretty_urls`** (#107) —
  the same gap #103 closed for canonicals. The rewriter emitted `.html` while
  `check_redirects` correctly reported that link as one the host redirects, and
  the author could not fix it in the Markdown without abandoning the feature.
  `check_orphans` compared the pre-normalisation path, so with `pretty_urls:
  strip` a nav linking `/validator` left every page reported as an orphan while
  `check_links` resolved the very same links.

## [1.8.22] - 2026-08-08

### Fixed
- 🔗 **`pretty_urls` did not reach what a page says about itself** (#103) — it
  fed link checking only, so a site on a host that strips extensions published
  canonical tags, `og:url`, JSON-LD and a sitemap naming URLs that `308` — the
  one thing a canonical must not do. Invisible locally, because resolving
  against the output directory is not how the host answers, and easy to miss
  because `check_redirects` reported the links *between* pages while staying
  silent about each page's own declared identity. It now also decides those.
  Feed entry IDs deliberately keep the raw form: a reader keys an item on its
  id, so rewriting them would re-deliver every post already read.
- 🧭 **`pretty_urls` assumed a trailing slash** (#103) — Cloudflare Pages strips
  `.html` and adds **no** slash, so the target `check_redirects` suggested was
  itself a URL Pages would redirect. It is now a mode: `off`, `strip` (Pages) or
  `strip-slash`. **`true` and `false` keep meaning exactly what they meant** —
  `true` is `strip-slash` — so no existing config changes behaviour by being
  re-read. Building with `deploy: cloudflare` and `strip-slash` warns, because
  that pairing now publishes canonicals the host redirects.
- 📅 **`formatDate` formatted nothing** (#98) — every non-string fell through to
  `Sprintf("%v")`, and `Page.Date` is a `time.Time`, so themes rendered Go's
  debug form (`2017-05-13 20:36:46 +0000 UTC`) — including inside `datetime`
  attributes, where it is not valid HTML. `formatDatePL` took a `time.Time` and
  formatted it properly, so a Polish theme looked right while an English one did
  not, which is what kept it hidden. It now formats, takes an optional layout
  (`{{ formatDate .Date "2006-01-02" }}`), and renders a zero time as empty
  rather than `1 January 0001`. Strings are still passed through unchanged.
- 🔗 **`related` was registered twice under one name** (#99) — two different
  functions shared the key and the merge after the map literal silently won, so
  the documented three-argument form was unreachable and every post failed to
  render with an arity error, while the build still reported success and simply
  had no post pages. `related` keeps the two-argument behaviour that actually
  ran; the collection form is now `relatedIn`. Helper merges report a name that
  is already taken, so this cannot recur silently.
- 🚦 **`410` redirects are dropped by Cloudflare Pages** (#102) — `410` is a
  Netlify extension; Pages honours 301/302/303/307/308 only and ignores the rest
  without a word, so the path keeps answering `200` while the rule reads as
  handled. Building with `deploy: cloudflare` now warns. `303` was also missing
  from the accepted set, so a valid rule drew an "unsupported status" warning.
  The docs said `301/302/307/308/410` for every platform; corrected.
- 🔏 **`fingerprint` double-hashed assets when the output was not cleaned**
  (#95) — the step reads the output directory, so a rebuild treated its own
  previous output as fresh input: `style.<hash>.css` became
  `style.<hash>.<hash>.css` while the original lingered, and the HTML was
  rewritten to a name the pages no longer referenced. `--clean` hid it by
  emptying the directory first, which is why it only showed on rebuilds — and
  why `--watch`, whose rebuilds are in place, hit it on every save. The
  previous build's `assets-manifest.json` now identifies its own output, which
  is removed rather than hashed again; this also bounds a directory that
  otherwise kept every historical hash of every asset forever. A name matching
  `name.<hash8>.ext` with no manifest to confirm it is skipped but **not**
  deleted, since a theme may legitimately ship such a name.

### Added
- ⚙️ **A `config` input for the GitHub Action** (#104) — the action exposed the
  three positional arguments and a subset of flags, with no way to say "use the
  `.ssg.yaml` I already have". Everything a real site keeps there — `redirects`,
  `worker`, `variables`, the `check_*` validators — has no input equivalent, so
  the action could not build a config-driven site at all, and the mismatch only
  surfaced after writing a workflow that quietly ignored half the configuration
  and deployed anyway. With `config` set, `source`/`template`/`domain` become
  optional; flags are passed through so the CLI resolves precedence.
- ✂️ **`trimPrefix` / `trimSuffix`** (#103) — a theme could test for an affix but
  not remove one, so stripping `.html` was impossible inside a template.
- 🚧 **A generated `404.html`** (#102) — static hosts answer an unmatched path
  by falling back to `index.html` **with a `200`** unless the output has a
  `404.html`, so every dead URL read to a crawler as another live copy of the
  home page. That was the out-of-the-box behaviour for a first-class deploy
  target, and a migration trap: `next export` generates one, so a site moving to
  SSG lost proper 404s silently. A page slugged `404` still takes precedence;
  `not_found_off: true` (or `--not-found-off`) suppresses it.
- 🧩 **`append`** (#96) — add values to a list, so a theme can build a derived
  collection. `slice` only ever made a literal, which left an ordinary
  requirement — "list the sub-pages of this section" — with no direct
  expression in a Go template. The collection may be the first or the last
  argument, so both `append $kids .` and `$kids | append .` work; the input is
  never mutated. Available in shortcode templates too.
- 🔤 **`filter` gained `hasPrefix`, `hasSuffix` and `matches`** (#96) — these
  existed as standalone helpers but could not be used as filter predicates, so
  selecting pages under a path had to fall back to `contains`, which is
  substring-wise and also matches `/not-special/`. Applied to a non-string
  field they report the mistake instead of answering false.
- 🏷️ **`variables.gtm_id`** (#101) — the Tag Manager container ID was hardcoded
  in `ssgtheme`, which made it part of the theme rather than the site: lost on a
  theme update, and impossible to differ between two sites sharing a theme. It
  now mirrors `variables.gtag`. When `variables.cookie_consent` is also set the
  loader ships as `type="text/plain" data-consent-category="analytics"` so the
  consent worker starts it only after the visitor accepts — the container
  request is itself a third-party call, so a site running a banner should not be
  making it beforehand.

### Documentation
- 📚 **Four gaps that each required reading the Go source** (#100) — `.Author`
  is an `int` and `.Categories` is `[]int` (metadata IDs) while `.Tags` is
  `[]string` and `.Category` is a `string`, so the obvious template renders
  `2 · 3, 4`; the types and the `getAuthorName`/`getCategoryName` resolvers are
  now in the table. There is no `urlize`/`slugify`, and the taxonomy helpers
  were absent from the helper reference — both now cross-referenced. The
  built-in taxonomy names are `category`, `tag` and `series` (singular, not the
  frontmatter field names), and an unknown name returns `""` rather than an
  error. `variables.cookie_consent` is documented as the supported integration
  point it already was.

### Documentation
- 📈 **`docs/UPGRADING.md`** — every version-to-version step in one page, with a
  picker that narrows the list to what applies between your current version and
  today. Organised by what changed rather than by release, because 53 of the 63
  releases so far need no action and a section per version would bury the ten
  that do. The picker is progressive enhancement: without JavaScript it stays
  hidden and every step is shown.

## [1.8.21] - 2026-08-07

### Added
- 🔗 **`feed_autodiscovery: false`** — keep publishing feeds but stop injecting
  `<link rel="alternate">` into the HTML, for a theme that wants control over the
  links' order, titles or which feeds are advertised. A theme emitting its own
  feed link already suppressed injection; this is the explicit form, so the
  behaviour no longer depends on SSG noticing what the theme happened to render.
  Default `true` — existing sites are unaffected.
- 🧩 **`feed` template helper, plus `concat` and `flatten`** (#91) — a site could
  publish an aggregated feed but not *show* one. The merge, filters, dedupe,
  ordering and provenance labels were all computed, but only the feed writer
  could see them, and a template had no way to compose two collections
  (`slice a b` builds a list *of* two lists and nothing unnests it) — so an HTML
  page fell back to one section per source, the "two lists printed one after the
  other" that is not a merge. `feed "<path-or-name>"` returns a declared feed's
  items, so page and feed come from one computation and cannot drift; `concat`
  and `flatten` are the general tools, useful well beyond feeds.

### Fixed
- 📚 **Declared feeds were undocumented, and the config table was corrupted** (#92).
  `feeds:`, RSS output and the per-feed `title` appeared nowhere a reader would
  look: the table described `feed` as Atom-only and `--help` agreed, so someone
  reasonably concluded RSS was unsupported and wrote a converter instead. The
  sections added in 1.8.20 also broke the table they were inserted into — the
  `feed` row lost its description to the row below it — and **12 table rows across
  the docs carried an unescaped `|` inside inline code**, which splits a cell and
  shifts every column after it. Table repaired, pipes escaped, `--help` now points
  at `feeds:`, and two things are stated explicitly: a declared feed can be
  *named*, and SSG injects the autodiscovery tags itself so a theme must not
  hand-write them.

- 🐛 **`format: feed` rejected every feed fetched over HTTP** (#90) — the format
  was added to the parser but not to the transport's content-type allowlist, so
  `accepted[format]` was `nil` and a real feed was refused before parsing. It
  blocked the headline feature of 1.8.20 outright: SSG could not read a feed it
  had generated itself. `format: changelog` (1.8.18) had the same gap. Both now
  list their content types — `feed` deliberately spanning Atom, RSS and JSON,
  since the parser detects the format from the payload and the transport gate
  must not be narrower than the parser. The tests covered the parser but used
  `type: file`, which never touches this gate; there is now a check that **every**
  supported format has an entry, so the class of bug cannot recur.

## [1.8.20] - 2026-08-07

### Added
- ↪️ **`check_redirects` + `pretty_urls` — links that only resolve through a host
  redirect** (#87). `check_links` resolves against the output directory, which is
  not how a host answers, so a link can pass and still cost every visitor a round
  trip; one `.html` link in a shared footer puts the whole site through one.
  `pretty_urls` describes the host — it strips `.html` and appends a trailing
  slash — and makes link checking agree in **both** directions: `check_links`
  stops reporting `/docs/swagger` broken when the host serves it from
  `swagger.html`, and `check_redirects` reports the reverse with the destination
  named. Off for a plain object store, where the extensionless form is a genuine
  404 rather than a redirect, and the check then skips with a message rather than
  reporting shapes the host never rewrites.

- 🌍 **Feeds can now be read as well as written — and merged** (#89). `format: feed`
  on an external source accepts **Atom 1.0, RSS 2.0 or JSON Feed 1.1** and
  normalizes all three into one shape, with dates parsed into real timestamps.
  The format is detected from the payload rather than the declaration, since a
  `.xml` URL may be either and a redirect can change what arrives. A declared feed
  can then **aggregate** several inputs — other sites' feeds *and your own posts*
  — into one published feed: sorted newest first, deduplicated by URL, each item
  carrying a **provenance label** emitted as a category so an aggregate can be
  grouped by where things came from. Filters run **per source first, then
  feed-wide**, because what counts as noise depends on the feed it came from and
  that context disappears once everything is merged; `words` match the title and
  summary, `tags` match categories, and exclusion beats inclusion. `paginate:`
  splits a large archive into RFC 5005 `rel="next"`/`"prev"` linked pages, with
  page one keeping the declared path so a subscribed URL never moves.

- 📰 **`feeds:` — as many syndication feeds as a site needs, each with its own
  selection and format** (#86). `feed: true` publishes one Atom feed of every
  post, plus one per language and per taxonomy term; that is all-or-nothing, so a
  site with several content roots cannot offer "just the blog", and "the three
  tags that mean *release*" needs three subscriptions. A declared feed chooses
  **what goes in** (`source` folder, `categories`, `tags`, `type` — optional and
  combined with AND), **where it is written** (`path`) and **in what format**:
  **Atom 1.0**, **RSS 2.0** or **JSON Feed 1.1**, alongside per-feed `items` and
  `full_content` overrides. `feed: true` behaviour is unchanged.

### Fixed
- 📚 **`external_sources` examples were missing the `sources:` level** — wording
  introduced with `format: changelog` in 1.8.18. Sources live under
  `external_sources.sources:`, so the documented example, copied literally,
  produced `unknown configuration key` warnings and loaded nothing.

- 🗺️ **Excluding a page no longer drops every URL it emitted** (#88) — a regression
  from the 1.8.18 sitemap work. Exclusion was decided per **page** and read from a
  single output file, so a source emitting more than one URL had all of them
  judged by that one verdict. A page slugged `index` emits both `/` and `/index/`,
  so a theme marking the duplicate `noindex` — or canonicalising it away under
  `sitemap_prune_canonical` — silently removed **the site root** from
  `sitemap.xml`, which is far worse than the duplicate it was fixing. Each `<loc>`
  is now judged against the file actually served at that URL: `/` against the root
  `index.html`, `/index/` against `index/index.html`. Before 1.8.18 a theme-set
  `noindex` did not affect the sitemap at all, so this could not happen.

- 🔗 **Feed autodiscovery now covers every feed, and every page** (#86). One
  `<link rel="alternate">` per published feed, with its own MIME type and title —
  a reader offering a choice reads exactly those links, so advertising four feeds
  behind a single Atom link hid three of them. The links were also injected by
  the SEO block, which only runs for pages carrying a page context, so the **site
  homepage — the first place a reader or subscription tool looks — advertised no
  feed at all**. A theme that provides its own link is still left alone.

## [1.8.19] - 2026-08-06

### Added
- 🧩 **`raw` (alias `html`) template helper** (#83) — a plain `template.HTML` cast
  for markup that comes from data: inline SVG, a pre-rendered snippet, an embedded
  config blob. `safeHTML` is not interchangeable: in a page template it runs the
  **Markdown pipeline** (right for `.Content`), while the shortcode func map
  defines it as a bare cast — the same name meaning two things. Passing SVG
  geometry through it wrapped the fragment in a `<p>`, which is invalid inside
  `<svg>`, so the icon silently did not draw.
- 📦 **`static_sources:`** (#84) — more than one verbatim passthrough root,
  mirroring `content_sources:`. One `static_dir` forces either a committed
  duplicate that drifts or a staging script every contributor has to know about,
  which is untenable when the files a site publishes already live elsewhere in the
  repository and are read there by tests and CI. Each entry keeps its own name so
  existing public URLs keep resolving (`path: xml` serves `/xml/…`); `dest:` places
  it elsewhere and `dest: "."` spreads a directory's contents at the output root,
  the way `static_dir` behaves. Copied after `static_dir`; a later entry wins.

### Fixed
- 🔗 **`link:` no longer has `page_format` appended to it** (#81) — a page whose
  frontmatter set `link: /validator.html` was written to `validator.html.html`
  under `page_format: flat`, and to a directory literally named `validator.html/`
  under `directory`, with the sitemap advertising `/validator.html/`. `link` is
  documented as the highest-precedence URL source, so a path that already names a
  file is now final: no second extension, no trailing slash. Links without an
  extension are unchanged, and a dot that is not an extension (`/spec/v1.0`) is
  still a directory.
- 📚 **`bundles:` documentation named paths that cannot work** (#79) — the example
  used bare filenames, but bundle names and sources resolve against the **output
  root**. Following it literally reported every source missing and wrote an empty
  bundle, which reads as a broken theme rather than a config mistake.
- 📚 **The theme-vs-generator SEO precedence is now documented** (#80) — `seo` is
  not all-or-nothing. A theme that emits its own Open Graph tags (to control
  `og:image`) still gets JSON-LD generated from frontmatter. Undocumented, that
  reads as "provide OG and you are on your own", so theme authors hand-write
  structured data SSG would have produced.
- 📚 **`TEMPLATES.md` implied the field table applies under `.Page`/`.Post`** (#82)
  — wording introduced in 1.8.17. It does not: `.URL`, `.CanonicalURL`,
  `.OutputPath`, `.TOC` and `.Hreflang` are computed for the template root and have
  no struct field behind them, so `.Page.TOC` fails at render, the page is skipped
  with a warning, and on a first build it looks like the content never loaded. The
  guide now separates root-only from nested values, gives the method equivalents
  (`.Page.GetCanonical .Domain`) and documents `.Extra` as the accessor for custom
  frontmatter.

## [1.8.18] - 2026-08-04

### Fixed
- 🔗 **`simple` theme: the canonical URL ignored the permalink scheme** — `post.html`
  and `page.html` hardcoded `https://<domain>/<slug>/`, so every post on a
  date-based permalink (the default) advertised a canonical pointing at a URL that
  does not exist: `/test-post/` for a page served at `/2025/01/01/test-post/`.
  Both now use the model's own `GetCanonical`, which follows the configured
  permalink scheme, and still honour an explicit frontmatter `canonical:`. Found
  while testing #78: pruning the sitemap on canonical mismatch silently dropped
  real posts, and the theme — not the check — was wrong.

### Added
- 🔎 **Three build-time checks over the generated HTML**, in the same shape as
  `check_links` (off / `warn` / `strict`, escalated by `strict: true`). Each
  targets a failure that is invisible today because the generator did exactly what
  it was told.
  - **`--check-images` / `check_images`** (#75) reports images with **no `alt`
    attribute**. It never generates alt text: an invented description reads as
    authoritative while being wrong, which is worse for a screen-reader user than
    silence. `alt=""` is the correct treatment for a decorative image and stays
    silent; `strict-decorative` opts into reviewing those too.
  - **`--check-meta` / `check_meta`** (#76) requires a non-empty `<title>` and meta
    description on indexable pages, skipping `noindex`. Catches the whole-site
    failure where a theme interpolates a field that is always empty and every page
    ships a blank tag. Title and description **lengths** are advisory notes, never
    build failures, and the ranges are configurable via **`meta_limits`** —
    `title_min`/`title_max`/`description_min`/`description_max`, where unset means
    the built-in default and an explicit `0` disables that bound.
  - **`--check-orphans` / `check_orphans`** (#77) reports indexable pages nothing
    links to. Only `<a href>` counts — every page links to itself through
    `<link rel="canonical">`, so counting all references would make nothing an
    orphan; self-links, `noindex` pages and the site root are ignored.
- 🧹 **`content_exclude`** (#74) — globs for Markdown under `content_dir` that must
  not be loaded as a page. Matched **before** parsing, so a data file whose front
  matter is valid for its own purpose but unparseable as a page is skipped cleanly
  instead of warning and being silently dropped. `status: draft` cannot help there,
  because the failure happens while unmarshalling, before any status is read.
  `**` crosses directory separators; the full path, the content-relative path and
  the bare filename are all matched.

### Changed
- 🏷️ **`seo: true` now fills in a missing meta description** from the front-matter
  `description:` (#76). Nothing is invented — the author already wrote it, it just
  never reached the output. An existing but **empty** tag is rewritten in place
  rather than joined by a second one, since two description tags with a blank first
  is worse than the problem being fixed.
- 🗺️ **`sitemap.xml` no longer lists pages whose rendered HTML says `noindex`**
  (#78). The sitemap asks a crawler to index a URL the page itself declines, which
  search consoles report as an error. No configuration needed: the sitemap is
  written after rendering, so the answer is on disk wherever the `noindex` came
  from — front matter or theme. Pages whose canonical points at a **different** URL
  are kept by default and pruned only with **`sitemap_prune_canonical: true`**: a
  canonical disagreeing with the permalink is far more often a theme bug than a
  deliberate exclusion, and the shipped `simple` theme is itself an example, so
  pruning on it by default would silently drop real posts.

### Fixed
- 🎲 **`TestRenderParallelDeterministic` was intermittently failing** — it built
  its corpus twice, once per worker count, so the two builds read source files
  with different mtimes. A post with no explicit `modified:` takes its feed
  `<updated>` from that mtime, so whenever the two writes straddled a second
  boundary the feeds differed and the test failed on a difference it was never
  meant to measure. It blocked the 1.8.17 release tag. The corpus is now written
  once and both builds read it, leaving the worker count as the only variable;
  200 runs plus 60 under `-race` are clean (it previously failed roughly 1 run in
  30 under `-race`).

## [1.8.17] - 2026-08-04

### Changed
- ⚡ **Builds are up to 5.25x faster on large sites** (PERF-012 / PERF-013) — two
  costs that only showed up at scale. `copyColocatedAssets` listed a post's
  category directory **once per post**, so N posts sharing a directory did N
  scans of an N-entry directory: on a 2000-post corpus that single `os.ReadDir`
  was two thirds of the whole build. It is now read once per directory and shared
  (mutex-guarded for parallel render). `ComputeReadingStats` then became the
  largest cost at ~48%: it stripped markup with `regexp.ReplaceAllString` and
  called `strings.Fields`, running the regex engine over every page and
  allocating two throwaway strings per page just to count words. A single-pass
  scanner does it **7.8x faster with zero allocations** (was 47KB / 15 allocs per
  page), pinned to the original regex by a test over the awkward shapes plus 4000
  random inputs. Measured best-of-3 on an M2: 2000 posts 2.96s → 0.95s, 5000
  posts 12.07s → 2.30s, with the per-post cost now flat (~0.46ms) instead of
  climbing. Output is unchanged — golden byte-identical on all four corpora.
- 🎲 **`make determinism`** — builds every corpus sequentially and on a full
  worker pool, then compares the output trees file by file, so shared render
  state missing a lock or an ordering fails the build instead of surfacing as a
  bug report months later. It carries its own stress corpus (300 aliased posts
  with `link_rewrites`), because the shipped corpora are too small for the
  scheduler to vary — verified by re-introducing the fixed bugs: the shipped
  corpora alone caught none of them, the stress corpus catches the `_redirects`
  ordering and the lost-alias regressions. It complements `go test -race` rather
  than replacing it; the `link_rewrites` memo race is the reverse case, caught by
  `-race` while the output survives. **`make golden` and `make determinism` now
  both run in CI** — neither did before, which is how those bugs shipped.
- ⏱️ **`make bench`** — a new `scripts/bench-build.sh` generates a fixed-seed
  synthetic corpus (100/500/2000 posts by default, configurable) and reports build
  time per page, so throughput claims are reproducible on your own hardware.

### Fixed
- 📄 **`dynamic-taxonomies` and `external-sources` examples shipped broken output**
  — both post templates read `.Page.*`, but posts receive their model under
  `.Post`, so every generated post had an empty `<title>`, an empty `<h1>` and
  empty taxonomy footers. Both also printed `{{.Content}}`, which is the **raw
  Markdown source** — `safeHTML` is the helper that converts it — so readers got
  unrendered Markdown. Fixed in both examples, and the docs that let this happen
  are clearer: `docs/TEMPLATES.md` now spells out `.Page` in `page.html` vs
  `.Post` in `post.html` and warns that the mismatch fails silently, and
  `docs/TEMPLATE_HELPERS.md` describes `safeHTML` as what renders a Markdown body
  rather than merely an escaping escape hatch.
- 🔒 **Two more parallel-render races, found by auditing the whole render tree**
  after the alias bug below. `link_rewrites` memoized its sorted prefix list with
  an unguarded check-then-write, and it runs on the worker pool (the `safeHTML`
  template helper calls it during `ExecuteTemplate`), so concurrent pages raced on
  the slice header and could rewrite links against a torn or empty prefix list —
  intermittently masked depending on whether an earlier sequential render warmed
  the memo. The shared image processor had the same unguarded lazy init, kept
  correct only by a warm-up call placed before the pool. Both now build under a
  `sync.Once`. Alias stub files also moved out of the pool: writing them during
  rendering made the "alias collides with an existing page" check a race against a
  half-written output tree, so the warning appeared or not depending on timing and
  two pages claiming the same alias raced to write the same file. They are now
  recorded during rendering and written afterwards in sorted order, once every
  real page exists.
- 🐛 **Frontmatter `aliases:` silently lost redirects, and `_redirects` was not
  reproducible** — a regression from parallel rendering (1.8.15). Pages render on
  a worker pool, and each one appended its aliases to a shared slice with no
  synchronization: concurrent appends raced on the slice header and **dropped
  entries** (a 200-alias reproduction recorded only 170), so a migrated site
  could ship missing 301s without any warning. The arrival order was also
  scheduler-dependent, so identical content produced a **different `_redirects`
  file on every build** — 10 builds of the same site gave 9 distinct outputs.
  The append is now mutex-guarded and the collected aliases are sorted before
  emitting, so the file is byte-identical across builds and worker counts. The
  existing corpora never caught this because none of them use aliases; a test now
  drives the concurrent path directly and pins the ordering.
- 🔖 **`make version-sync` now covers every file that states the version** — the
  Docker image (build arg + OCI label), the man page, the install docs, the docs
  site and the theme README were bumped by hand each release, so `man/ssg.1`
  drifted a release behind and `--check` reported everything in sync. All five are
  now synced and, because CI already runs the check, drift in any of them fails
  the build. Patterns are anchored per line and require a semver, so a bump can
  never wander into neighbouring keys (`docs-site.yaml`'s analytics
  `version: "1"`) or into prose — a blog sentence naming the release that
  introduced a feature is a historical fact, not a version to sync.
- 📝 **Corrected a historical range in `snap/snapcraft.yaml`** — the note about
  the Snap Store freeze said it ran "through 1.8.16"; it was extended by each
  release's find-and-replace even though the fix landed in **1.8.13**. It now
  reads 1.8.6 → 1.8.12, with both it and the equivalent note in `homebrew.yml`
  marked as history that must not be bumped.

## [1.8.16] - 2026-08-02

### Added
- 📋 **`format: changelog` — a `CHANGELOG.md` as structured data** (#69) — a local
  file source with `format: changelog` parses the Keep-a-Changelog convention into
  `.ExternalData.<name>.versions`, `.latest` (first released version) and
  `.unreleased`, each with `version` / `date` / `released`, `sections` keyed by the
  lowercased `###` heading (`added`, `fixed`, …) and a flat `entries` list. Every
  entry is split into `title` (the leading bold run, rendered), `html` (the rest),
  `full`, `marker` (leading emoji) and `text` (raw Markdown), so a "What's New"
  panel is a `range` over the file you already edit at release time — no pre-build
  script, nothing to go stale. Both `## [1.8.16] - 2026-08-02` and
  `## 1.8.16 - 2026-08-02` headings parse; `-`/`*` bullets and wrapped entries are
  handled.
- 🔌 **`ssg mcp` — development MCP server (designer + content manager)** — a
  Model Context Protocol server over stdio that lets an AI assistant work on the
  site live during development, in two clearly-scoped roles: **designer**
  (`designer_*` — templates, partials, CSS, theme assets; cannot touch content or
  delete) and **content manager** (`content_*` — create/update/fix/delete
  Markdown; cannot touch templates or write non-Markdown). Every tool description
  states what the model can and cannot do, an always-present `help` tool restates
  the whole contract, and by default each change **rebuilds the site** — errors
  come back as the tool result so the assistant fixes its own mistakes.
  `--role=designer|content` exposes one role; `--no-watch` disables rebuilds. With
  `mcp.git` configured (account + `$ENV` token) the assistant also gets a safe
  **git write-back flow**: `git_new_branch` → edit → `git_commit` → human reviews
  → `git_open_pr` — edits never land on the base branch, only the
  content/template directories are staged, and the PR is opened only after
  explicit human approval. Without a token the `git_*` tools are not exposed.
  The designer additionally owns the **presentation keys in the config file**
  (`designer_config_read` / `designer_config_set`): a narrow allow-list — theme,
  mermaid, syntax highlighting, math, TOC, minification, fingerprinting,
  pagination, WebP — while every other key (secrets, deployment, server,
  endpoints, hooks, content and URL structure) is refused by construction. Edits
  preserve the file's comments and key order, and a change that leaves the
  configuration invalid is rolled back automatically.
- 🔗 **Related-posts template helpers** — `{{ range related . 5 }}` returns the
  posts most related to the current page by shared **tags and keywords** (ranked
  by overlap, then recency, then slug — deterministic, reads the already-loaded
  content, no network). `{{ range relatedFromMddb . 5 }}` does the same by
  querying the **mddb** corpus (a live `Search` filtered by the page's
  tags/keywords), so it can surface articles beyond the pages built into this
  site. See `examples/related-posts/` for the keyword, mddb and embeddings/vector
  approaches.
- ✉️ **Comments worker: email on a new comment** — set `COMMENTS_MAIL_URL` /
  `COMMENTS_MAIL_FROM` / `COMMENTS_MAIL_TO` (plus `COMMENTS_MAIL_KEY` for a bearer
  token) and the worker emails a moderation notice whenever a **non-spam** comment
  lands. Delivery is a JSON `POST` of `{from, to, subject, text}` — the shape
  providers like Resend accept, or point it at your own relay — sent in the
  background (`waitUntil`) so it never delays the submitter, with gateway errors
  logged rather than fatal. Spam is filtered silently and never mailed; unset ⇒
  no email.
- 📣 **Post-publish notifications** — announce each newly published (or changed)
  post to webhook destinations you define (`notifications:`), pointing them at a
  platform API, an automation service or your own endpoint; they receive the post
  as JSON `{slug, title, url, excerpt, date, tags}`. A **committed state file**
  (`notify_state`, default `.ssg-notifications.json`) dedupes on a content hash,
  so a post fires **once** — again only when its content changes — and it never
  sends unless you pass `--notify`, so dev builds stay quiet. Header secrets use
  `$ENV`; the delivery transport refuses private/loopback ranges at dial time
  (SSRF-hardened) unless `allow_private` is set; a failed destination retries next
  run.
- 🤖 **`[ai …]` content shortcode — ask an AI at build time** — a two-layer
  setup: **models** under `ai.models` are endpoints (url, `$ENV` key, model id,
  optional base system prompt, params), and **agents** under `ai.agents` are roles
  built on a model that layer a persona plus user-defined **`rules`** (constraints
  they must follow) and **`skills`** (jobs they apply). Ask from inside content
  with `[ai agent="writer" question="…" ifs="lang == en" fallback="…"]`, or invoke
  a bare model with `model="…"`. The answer is fetched **once** and
  **content-addressed cached** (`ai.cache_dir`, default `.ai-cache`) keyed by the
  effective request, so a build is deterministic and only re-queries when the
  question, model, rules, skills or params change — commit the cache and CI
  rebuilds identically with no key and no network. `ifs` is an optional guard over
  the page's fields with `AND`/`OR` and `==`/`!=`/`contains`/`>`/`<`/`>=`/`<=`;
  when it is false, or the query fails, the `fallback` text is used. Precedence is
  explicit agent → explicit model → `ai.default_agent` → `ai.default_model` → sole
  agent → sole model. Keys are read from the environment, never literals.

### Fixed
- 👀 **`--watch` now watches the configuration file** (#70) — editing `.ssg.yaml`
  during a watch session reloads the configuration and rebuilds with the new
  settings. Previously the watcher observed only content and templates and kept
  building from the configuration loaded at startup, so a changed theme or option
  silently did nothing until a restart — with no hint that it had been ignored.
  Command-line flags still take precedence over the file, exactly as at startup.
  A config edit that leaves the file unparseable is reported and the **last good
  configuration is kept running**, so a half-saved file never kills the session.
  The startup line now names every watched input, e.g. `👀 Watching for changes in
  content, templates, data, config (.ssg.yaml)...`.

## [1.8.15] - 2026-08-01

### Added
- ⚡ **Parallel page/post rendering (`--workers`)** — the HTML render loop, not
  just WebP conversion, now runs on the worker pool, so a content-heavy site
  publishes much faster (on the docs site here, `--workers=8` roughly halved the
  build). Pages are grouped by language and the shared site view is set **once**
  per language, so multilingual output stays correct; the render-time caches
  (markdown-conversion memo, shortcode templates, missing-translation warnings)
  are mutex-guarded, with the expensive markdown conversion kept outside the lock
  so it still parallelises. Output is byte-for-byte identical to a sequential
  build — verified with the race detector and the golden harness on every corpus.
  `build_workers` / `--workers=N` governs both render and WebP: unset = one per
  CPU, `N` = exactly N, `0` = off (sequential).

### Changed
- 🔗 **`rewrite_md_links` is now on by default** — a relative `.md` link in
  content is rewritten to its final output URL out of the box, since a raw `.md`
  link otherwise 404s or serves the source file. Set `rewrite_md_links: false` to
  opt out; the rewriting behaviour itself is unchanged.

## [1.8.14] - 2026-08-01

### Added
- ⚡ **Parallel builds (`--workers`)** — the build was fully sequential; WebP image
  conversion now runs on a worker pool, so an image-heavy site publishes several
  times faster on a multi-core machine. `build_workers` / `--workers=N`: unset
  uses the whole machine (one worker per CPU), `N` caps it (e.g. `--workers=2` on
  a shared box), `--workers=0` turns parallelism off (sequential). Each image is
  independent, so the output is byte-identical whatever the count — verified with
  the race detector and the golden harness. (The HTML-render loop stays sequential
  for now; it threads mutable per-language state through the template context, so
  parallelising it safely is a separate, race-audited change.)
- 🔌 **Portable server endpoints, self-hosted first** (#63, foundation) — a new
  vendor-neutral `endpoints:` block declares small server behaviours once, and the
  built-in server (`--http`) runs them natively in the single Go binary — no
  external runtime, works behind nginx/Caddy or the existing Docker image. Two
  types to start: `redirect` (a request-time 3xx, the dynamic complement to the
  static `_redirects`) and `proxy` (forward to an upstream, keeping its key
  server-side). The proxy resolves and vets the upstream IP itself and **refuses
  loopback/private ranges at dial time** — the same SSRF / DNS-rebinding guard the
  external-source client uses — with `allow_private: true` to opt in to a genuine
  self-hosted upstream. Pure-static builds are unaffected (empty `endpoints:` is a
  no-op).
  - **Platform adapters (plugin per file).** Set `endpoints_platform` and the
    build compiles the *same* `endpoints:` into a platform's functions — no
    rewrite. Three adapters: **Cloudflare** Pages Functions (`functions/<path>.js`,
    the same tree hand-written workers use, so there's no parallel mechanism),
    **Netlify** Functions v2 (each declares its own route, no `_redirects`
    wiring), and **Vercel** Edge Functions + a generated `vercel.json`. Each
    adapter is a self-contained, self-registering file, so a new target is one
    file. So an endpoint deploys unchanged to self-hosted, Cloudflare, Netlify or
    Vercel.
  - **`form` primitive.** A `form` endpoint accepts a `POST`ed submission, drops
    bots via a `honeypot` field, and delivers the collected `fields` as JSON to a
    `to` webhook (kept server-side), then `redirect`s the browser (or returns a
    small JSON ok). Works self-hosted (same SSRF-guarded delivery as `proxy`) and
    compiles to all three platforms — a contact form without a SaaS.
  - **`auth` guard.** An `auth` endpoint protects its `path` as a prefix with HTTP
    Basic auth (constant-time compare, password from an env var — never a literal),
    covering both endpoints and static files beneath it. Runs on the built-in
    server; adapters skip it (use the platform's own access control). The
    `worker:` migration is the remaining piece of this epic.
- 📋 **Content contracts: frontmatter schemas, strict mode, route manifest** (#62)
  — declare per-type frontmatter contracts in `content_schemas` (required fields
  plus `string`/`int`/`bool`/`date`/`url`/`list`/`enum` field rules) and the build
  validates every post and page against its type's schema, reporting each
  violation with file, field and reason. Violations warn by default; `strict`
  (or `--strict`) turns them — and internal link checking — into hard build
  failures, so a missing `author` or a renamed slug that orphans a link fails the
  build instead of shipping. `route_manifest` (or `--route-manifest`) writes
  `routes.json`: a sorted, deduplicated list of every generated route (posts,
  pages, and category/tag/series/author/custom-taxonomy archives) with its type,
  title, source and language — a machine-readable contract external tooling can
  diff. All opt-in; a plain build is unchanged.
- 🤖 **AI-first JSON-LD structured data** (#61) — with `seo` on, every page now
  emits richer Schema.org Linked Data derived from existing frontmatter with zero
  extra config, so AI agents and answer engines get machine-readable data without
  running JavaScript. Content types map correctly — blog posts → `BlogPosting`
  (with `headline`, `datePublished`/`dateModified`, `author`, `keywords` from
  tags, `mainEntityOfPage`), the home page → `WebSite`, other pages → `WebPage`
  (previously every non-post was mislabelled `WebSite`) — and every non-home page
  also gets a `BreadcrumbList` from its URL. Two override layers deep-merge over
  the derived data (most specific wins): site-wide `schema:` in the config (for a
  publisher/Organization on every page) and per-page `schema:` in frontmatter.
  `</script>` in any field is escaped, so untrusted titles can't break out.
- 🔀 **Per-page `alias_stubs` — 301 instead of a duplicate copy** (#65) — an
  `aliases:` entry has always emitted a `301` into `_redirects`; by default it
  *also* writes a meta-refresh stub copy (a fallback for hosts without server
  redirects). The site-wide `alias_stubs: false` that suppresses those stubs is
  now overridable **per page** in frontmatter, so a migrated legacy slug can
  consolidate to its canonical with a pure `301` (no 200-serving duplicate for
  crawlers) while the rest of the site keeps its stubs — or the reverse. The
  `301` is emitted either way.
- 📈 **ssgtheme: Google Analytics 4 (gtag.js), consent-aware** — set
  `variables.gtag: "G-XXXXXXXXXX"` and the theme injects gtag.js with **Consent
  Mode v2**: every storage type defaults to `denied`, and when the cookie-consent
  banner is present its `signal()` flips `analytics_storage` to `granted` once
  the visitor accepts. No `variables.gtag` → nothing is emitted (the theme stays
  generic; the measurement ID lives in the site config, never the shared theme).
- 💬 **Threaded replies on comments** (GO-084) — readers can reply to a
  top-level comment (one level deep). The widget shows a **Reply** button, nests
  replies under their parent, and the worker validates the parent is an approved
  top-level comment on the same page before storing it. A `parent_id` column is
  added automatically (auto-migrated on existing databases). Replies are
  moderated like any comment. New i18n strings (Reply / Replying to… / Cancel).
- 🚦 **One pending comment per person per thread** (GO-083) — a visitor who
  already has a comment awaiting review on a page can't stack up more (`429`);
  they can add another once theirs is moderated (approved, spam, or deleted).
  Keyed on the now-required email, case-insensitively, per URL. The widget also
  **hides the form after you post and keeps it hidden until your comment is
  approved** (remembered client-side per URL, with a 14-day fallback), instead of
  re-showing an empty form.
- 🗄️ **Comments worker self-initialises its D1 schema** (GO-083) — the worker
  creates its `comments` table (and indexes) on first use, so binding the D1
  database is the only setup step — no manual `wrangler d1 execute`. The
  statements are `IF NOT EXISTS` and run once per isolate, so it's a no-op
  thereafter; `schema.sql` stays for anyone who prefers to apply it by hand.
- 📥 **Import WordPress comments** (GO-083) — `workers/comments/tools/wordpress-comments.sh`
  (bash + `curl` + `jq`, no plugin) pulls comments from a WordPress site's public
  REST API and converts them to the import JSON, building each comment's URL from
  its post slug (`URL_TEMPLATE`, default `/blog/{slug}/`). It can print the JSON
  or `--post` it directly (chunked). Documented in the worker README, including
  the WXR-export fallback for pending/spam comments and emails.
- ✉️ **Comment email is now required** (GO-083) — the comment form marks email
  `required` and the worker rejects a submission without a valid email (`422`).
  Bulk import still treats email as optional (historical data). The i18n labels
  drop "optional" in all four languages.

- 🔐 **Docs site: Turnstile keys from GitHub secrets** (GO-082) — the docs-site
  deploy workflow now injects the Turnstile **site key** into the config from a
  GitHub secret at build time (the committed test key stays when it's unset), and
  pushes the **secret key** (plus optional moderation password / IP salt) onto
  the Pages project via `wrangler pages secret put`, so no keys live in the repo.
  The comments worker README documents the pattern, including that a D1 binding
  is a project setting rather than a secret.

### Changed
- ♻️ **Built-in taxonomies unified onto the registry** (#44) — the built-in
  `category`, `tag`, `series` and `author` archives are now driven by the single
  taxonomy registry (`generateTaxonomies`) instead of four separate top-level
  pipelines, so there is one place that renders every taxonomy. Output is
  **byte-for-byte identical** (verified by the `make golden` equivalence harness
  across the corpus, dynamic, multilingual and external fixtures): built-ins keep
  their legacy-compatible rendering — no taxonomy index page, no i18n path prefix,
  single-page archives, the `tag.html`→`category.html` template fallback, and
  skip-not-fail URL collisions. No user-facing change; a `Folded` definition flag
  marks a built-in migrated onto the registry.

### Fixed
- 🐛 **Social preview images follow the WebP conversion** (#64) — with `webp` in
  its default replace mode the original `.jpg`/`.png` is removed after
  conversion, but `og:image` (and the new `twitter:image` / JSON-LD `image`) kept
  pointing at the deleted file, so every share preview 404'd. The WebP reference
  pass now rewrites those social-image references to the emitted `.webp`, same as
  in-content `<img>`. SEO injection also now emits `twitter:image` and a JSON-LD
  `image` from `featured_image` (previously only `og:image`), so one field drives
  the whole preview. Absolute (`https://…`) references are still left untouched;
  use `webp_keep_original` for hardcoded ones.
- 🐛 **Moderation panel no longer double-prompts for auth** (GO-084) — the admin
  `401` carried a `WWW-Authenticate: Basic` header, so the browser popped its
  native login dialog on top of the panel's own password field. The header is
  gone (curl `-u` sends the credential proactively and doesn't need it), leaving
  just the panel's field.
- 🐛 **Docs site: widget assets no longer cached for a year** (GO-083) — a
  worker's client files (`/comments.js`, `/cookie-consent.js`, …) sit at the site
  root and are updated in place, but were served with the default one-year
  immutable asset cache, so a browser kept a stale copy (e.g. an old form label)
  long after a deploy. The docs-site config now serves them revalidating
  (etag-checked) via a `headers:` override, so updates propagate on the next
  visit. Any site with in-place-updated worker assets should do the same.
- 🐛 **Comments worker fails clean when D1 isn't bound** (GO-083) — every D1-backed
  endpoint (`GET`/`POST`/admin/import) now returns a JSON `503 comments not
  configured` instead of throwing a raw Cloudflare `500` when the `COMMENTS_DB`
  binding is missing. The widget also parses the response defensively, so a
  server error no longer shows as a misleading "network error".
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

### Documentation
- 📄 **TEMPLATES.md: theme asset output URLs** (#59) — documents that a theme's
  `css/`, `js/` and `images/` directories are copied to `output/css`, `output/js`
  and `output/images` (referenced as `/css/…`, `/js/…`, `/images/…`), that
  `media/` comes from the content source rather than the theme, and that a
  `static_dir` file overwrites a colliding theme asset (static is copied last).
  Previously a theme author had to read the generator source to learn this.

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
[Unreleased]: https://github.com/spagu/ssg/compare/v1.8.24...HEAD
[1.8.24]: https://github.com/spagu/ssg/compare/v1.8.23...v1.8.24
[1.8.23]: https://github.com/spagu/ssg/compare/v1.8.22...v1.8.23
[1.8.22]: https://github.com/spagu/ssg/compare/v1.8.21...v1.8.22
[1.8.21]: https://github.com/spagu/ssg/compare/v1.8.20...v1.8.21
[1.8.20]: https://github.com/spagu/ssg/compare/v1.8.19...v1.8.20
[1.8.19]: https://github.com/spagu/ssg/compare/v1.8.18...v1.8.19
[1.8.18]: https://github.com/spagu/ssg/compare/v1.8.17...v1.8.18
[1.8.17]: https://github.com/spagu/ssg/compare/v1.8.16...v1.8.17
[1.8.16]: https://github.com/spagu/ssg/compare/v1.8.15...v1.8.16
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
