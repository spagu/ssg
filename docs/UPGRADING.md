# Upgrading

Every release is a drop-in replacement unless this page says otherwise. Replace
the binary, rebuild, and the site you had is the site you get.

Where that is not true — where a default changed, a flag moved, or a setting
started meaning something new — it is listed below, with the version it landed
in and what to do about it. Of the 66 releases so far, 53 need nothing at
all, which is why this page is organised by **what changed**, not by release
number: a section per version would be mostly empty and would bury the handful
that matter.

## The compatibility promise

A setting that stops being the right way to do something keeps working. It is
not removed in the release that replaces it — it starts warning in the build
log, naming what to use instead, and stays accepted for roughly five releases
after that. So an upgrade never fails silently: either it just works, or the
build tells you what to change while still producing your site.

Deprecation warnings are worth reading rather than filtering out. They are the
only notice you get, and they expire.

## How to upgrade

The install method decides the command; none of them need the site touched.

```bash
# One-liner (Linux/macOS) — same command installs and upgrades
curl -sSL https://raw.githubusercontent.com/spagu/ssg/main/install.sh | bash

# Debian/Ubuntu, if you added the apt repository
sudo apt update && sudo apt install --only-upgrade ssg

# macOS
brew upgrade ssg

# Docker — pull the tag you want rather than reusing a cached latest
docker pull ghcr.io/spagu/ssg:latest
```

Confirm what you actually have before and after, because a stale binary earlier
in `PATH` is the most common reason an upgrade appears to do nothing:

```bash
ssg --version
```

Full release notes for every version live in
[CHANGELOG.md](https://github.com/spagu/ssg/blob/main/CHANGELOG.md). This page
covers only the steps; the changelog covers everything else.

## What applies to you

<div class="upgrade-picker" hidden>
  <label for="upgrade-from"><strong>I am upgrading from</strong></label>
  <select id="upgrade-from">
    <option value="">— choose your current version —</option>
    <optgroup label="1.8.x">
      <option value="1.8.41">1.8.41 — 2026-08-16</option>
      <option value="1.8.40">1.8.40 — 2026-08-16</option>
      <option value="1.8.39">1.8.39 — 2026-08-16</option>
      <option value="1.8.38">1.8.38 — 2026-08-16</option>
      <option value="1.8.37">1.8.37 — 2026-08-16</option>
      <option value="1.8.36">1.8.36 — 2026-08-16</option>
      <option value="1.8.35">1.8.35 — 2026-08-15</option>
      <option value="1.8.34">1.8.34 — 2026-08-14</option>
      <option value="1.8.33">1.8.33 — 2026-08-14</option>
      <option value="1.8.32">1.8.32 — 2026-08-14</option>
      <option value="1.8.31">1.8.31 — 2026-08-13</option>
      <option value="1.8.30">1.8.30 — 2026-08-12</option>
      <option value="1.8.29">1.8.29 — 2026-08-12</option>
      <option value="1.8.28">1.8.28 — 2026-08-12</option>
      <option value="1.8.27">1.8.27 — 2026-08-12</option>
      <option value="1.8.26">1.8.26 — 2026-08-11</option>
      <option value="1.8.25">1.8.25 — 2026-08-11</option>
      <option value="1.8.24">1.8.24 — 2026-08-09</option>
      <option value="1.8.23">1.8.23 — 2026-08-08</option>
      <option value="1.8.22">1.8.22 — 2026-08-08</option>
      <option value="1.8.21">1.8.21 — 2026-08-07</option>
      <option value="1.8.20">1.8.20 — 2026-08-07</option>
      <option value="1.8.19">1.8.19 — 2026-08-06</option>
      <option value="1.8.18">1.8.18 — 2026-08-04</option>
      <option value="1.8.17">1.8.17 — 2026-08-04</option>
      <option value="1.8.16">1.8.16 — 2026-08-02</option>
      <option value="1.8.15">1.8.15 — 2026-08-01</option>
      <option value="1.8.14">1.8.14 — 2026-08-01</option>
      <option value="1.8.13">1.8.13 — 2026-07-24</option>
      <option value="1.8.12">1.8.12 — 2026-07-22</option>
      <option value="1.8.11">1.8.11 — 2026-07-22</option>
      <option value="1.8.10">1.8.10 — 2026-07-21</option>
      <option value="1.8.9">1.8.9 — 2026-07-21</option>
      <option value="1.8.8">1.8.8 — 2026-07-20</option>
      <option value="1.8.7">1.8.7 — 2026-07-15</option>
      <option value="1.8.6">1.8.6 — 2026-07-15</option>
      <option value="1.8.5">1.8.5 — 2026-07-15</option>
      <option value="1.8.4">1.8.4 — 2026-07-14</option>
      <option value="1.8.3">1.8.3 — 2026-07-14</option>
      <option value="1.8.2">1.8.2 — 2026-07-11</option>
      <option value="1.8.1">1.8.1 — 2026-07-10</option>
      <option value="1.8.0">1.8.0 — 2026-07-10</option>
    </optgroup>
    <optgroup label="1.7.x">
      <option value="1.7.15">1.7.15 — 2026-07-09</option>
      <option value="1.7.14">1.7.14 — 2026-07-08</option>
      <option value="1.7.13">1.7.13 — 2026-04-08</option>
      <option value="1.7.12">1.7.12 — 2026-04-08</option>
      <option value="1.7.11">1.7.11 — 2026-04-06</option>
      <option value="1.7.10">1.7.10 — 2026-04-06</option>
      <option value="1.7.9">1.7.9 — 2026-04-06</option>
      <option value="1.7.8">1.7.8 — 2026-04-06</option>
      <option value="1.7.7">1.7.7 — 2026-04-01</option>
      <option value="1.7.6">1.7.6 — 2026-04-01</option>
      <option value="1.7.4">1.7.4 — 2026-04-01</option>
      <option value="1.7.3">1.7.3 — 2026-03-31</option>
      <option value="1.7.2">1.7.2 — 2026-03-31</option>
      <option value="1.7.1">1.7.1 — 2026-03-30</option>
      <option value="1.7.0">1.7.0 — 2026-03-05</option>
    </optgroup>
    <optgroup label="1.6.x">
      <option value="1.6.2">1.6.2 — 2026-03-05</option>
      <option value="1.6.1">1.6.1 — 2026-03-05</option>
      <option value="1.6.0">1.6.0 — 2026-03-05</option>
    </optgroup>
    <optgroup label="1.5.x">
      <option value="1.5.4">1.5.4 — 2026-02-04</option>
      <option value="1.5.3">1.5.3 — 2026-02-04</option>
      <option value="1.5.2">1.5.2 — 2026-02-03</option>
      <option value="1.5.1">1.5.1 — 2026-02-03</option>
      <option value="1.5.0">1.5.0 — 2026-02-03</option>
    </optgroup>
    <optgroup label="1.4.x">
      <option value="1.4.9">1.4.9 — 2026-01-29</option>
      <option value="1.4.8">1.4.8 — 2026-01-29</option>
      <option value="1.4.7">1.4.7 — 2026-01-29</option>
      <option value="1.4.6">1.4.6 — 2026-01-23</option>
      <option value="1.4.5">1.4.5 — 2026-01-23</option>
      <option value="1.4.4">1.4.4 — 2026-01-18</option>
      <option value="1.4.3">1.4.3 — 2026-01-18</option>
      <option value="1.4.2">1.4.2 — 2026-01-18</option>
      <option value="1.4.1">1.4.1 — 2026-01-18</option>
      <option value="1.4.0">1.4.0 — 2026-01-18</option>
    </optgroup>
    <optgroup label="1.3.x">
      <option value="1.3.4">1.3.4 — 2026-01-17</option>
      <option value="1.3.3">1.3.3 — 2026-01-17</option>
      <option value="1.3.2">1.3.2 — 2026-01-17</option>
      <option value="1.3.1">1.3.1 — 2026-01-17</option>
      <option value="1.3.0">1.3.0 — 2026-01-17</option>
    </optgroup>
    <optgroup label="1.2.x">
      <option value="1.2.0">1.2.0 — 2026-01-16</option>
    </optgroup>
    <optgroup label="1.1.x">
      <option value="1.1.0">1.1.0 — 2026-01-13</option>
    </optgroup>
    <optgroup label="1.0.x">
      <option value="1.0.0">1.0.0 — 2026-01-13</option>
    </optgroup>
  </select>
  <p class="upgrade-count" role="status"></p>
</div>

Pick your current version and the list below narrows to the steps between it and
today. Without JavaScript the selector stays hidden and every step is shown,
which is a longer read but never a wrong one.

<div class="upgrade-steps">

<!--
  The version numbers below are historical facts: each names the release a
  change actually landed in, and the selector filters on data-since. A bulk
  find-replace during a version bump has rewritten them once already, which made
  the guide claim the wrong release and the filter show the wrong steps. Add new
  entries; never renumber existing ones.
-->


<div class="upgrade-step" data-since="1.8.42">

### 1.8.42 — `repair` and `check_markup` may report pages they used to pass

Nothing to configure. Two things can newly speak up, and in both cases what they
report has been shipping:

- **`ssg repair` now finds fenced markup**, not only indented markup. A page
  whose exported body carries a stray code fence renders as source from that line
  on, and `repair` used to walk past it. `ssg repair` (dry run) tells you which
  pages; `ssg repair --fix` removes the fence markers and leaves the markup, the
  same shape as the existing dedent. A fence you wrote on purpose — ```` ```html ````,
  or any fence in a prose document — is never touched.
- **`check_markup` names the cause**, "indented as code" or "fenced as code". On
  `check_markup: strict` a page with a swallowing fence now fails the build where
  it previously passed. Drop to `warn` if you need the build green while you fix
  the sources.

**`type_archives` is new and off**, so a custom post type's section is not built
unless you ask — with one exception worth knowing: if your
`content/<source>/metadata.json` carries `custom_types[].has_archive`, the
archives it declares are built automatically. **wpexporter 1.8.15 writes it**, so
re-exporting a migrated site gives you those sections without touching the
config. On an older export, declare them yourself:

```yaml
type_archives:
  realizacje: true
```

</div>

<div class="upgrade-step" data-since="1.8.41">

### 1.8.41 — check your `domain` if the build now warns about it

Nothing to change unless your `domain` carries a scheme or a trailing slash. If
it does, the build now says so and corrects it:

```
⚠️  domain "https://example.com" is not a bare host — using "example.com".
```

**The URLs your site publishes will change** — from `https://https://example.com/…`
to `https://example.com/…` in the canonical tag, `og:url`, the sitemap, the
JSON-LD `@id` and the feed. That is the correction, not a regression: the old
ones were never reachable addresses. Worth knowing anyway, because a sitemap
already submitted to Search Console carried them.

Fix the value in `.ssg.yaml` (or the positional argument) to silence the
warning:

```yaml
domain: "example.com"   # not https://example.com
```

A port (`example.com:8080`) and a subdirectory deploy (`example.com/blog`) are
left alone — the first is part of the host, the second is deliberate.

</div>

<div class="upgrade-step" data-since="1.8.40">

### 1.8.40 — `check_schema` may report pages it used to pass

Nothing to change unless you use `schema_defaults` **and** your theme writes
JSON-LD of its own. If it does, `check_schema` will now name every page in a
section whose declared `@type` reached the output nowhere — which is most likely
a real gap that has been shipping, not a new false positive: a theme that emits
any `application/ld+json` block turns auto-injection off for the whole page.

Emit the derived data beside your own block:

```gotemplate
<script type="application/ld+json">{{ toJSON .Schema }}</script>
{{ partial "schema-faq" . }}
```

`.Schema` is new in this release: the merged structured data SSG would have
injected, in precedence order, so the theme does not have to rebuild it.

On `check_schema: strict` this turns a previously passing build into a failing
one. If you need the build green before you can fix the theme, drop to
`check_schema: warn` for the interim — the finding is the same, it just does not
stop the build.

</div>

<div class="upgrade-step" data-since="1.8.39">

### 1.8.39 — the front page honours pinned posts, and date archives exist

Two things that were supposed to work in earlier releases now do, so output can
change **for sites that asked for them**:

- **A pinned post moves to the top of the front page** and of `.Site.Posts`. In
  1.8.38 it already led every category and tag archive; it stood wherever its
  date fell on the front page. If your theme marks the pinned entry with
  `{{if .Sticky}}`, it will now appear first there too. A site with no
  `sticky: true` in any post is byte-identical.
- **`date_archives: true` starts writing `/YYYY/` and `/YYYY/MM/`.** The key was
  accepted and nothing was generated, so a site that set it has been building
  without those pages. They appear on the next build. A page of your own that
  already owns such a URL keeps it, and the build says so. If you do not want
  them, remove the key — the default is `false`.
- **Feeds go back to chronological order.** Pinning reached `/feed/` in 1.8.38,
  which WordPress does not do. Nothing to change; a pinned older post simply
  stops appearing at the top of the feed.

Nothing to do for the engine change: `ssg migrate` now runs the newest wpexporter
it can reach. Snap users who want an engine ahead of the snap's rebuild cadence
can install one into their home directory and it will be used:

```bash
go install github.com/tradik/wpexporter/cmd/wpexporter@latest
```

`--engine /path/to/wpexporter` (or `SSG_WPEXPORTER`) picks one explicitly.

</div>

<div class="upgrade-step" data-since="1.8.38">

### 1.8.38 — `ssg migrate` needs wpexporter 1.8.11

An older engine is now refused before the migration starts, instead of producing
an export that looks complete and is not. **Snap users: nothing to do** —
`snap refresh static-site-generator` carries a current engine. Otherwise:

```bash
go install github.com/tradik/wpexporter/cmd/wpexporter@latest
```

Nothing else in this release requires action: pinned posts, the numbered pager
and the `<title>`-in-`<script>` warning are all additive.

</div>

<div class="upgrade-step" data-since="1.8.37">

### 1.8.37 — paginated category archives, and posts_page takes its address

If you set `paginate`, category archives are now paginated too: an archive with
more posts than the page size grows `/category/<slug>/page/2/` and so on, where
before it was one long file. **Nothing to do** — the archive's first page keeps
its address; a site without `paginate` is untouched.

If you set `posts_page` **and** have a page document at that same address, the
listing now takes the URL and the page is not written. That is what WordPress
does with its assigned "Posts page", and the build names both documents so you
can rename the page or change the key if you wanted the page instead.

</div>

<div class="upgrade-step" data-since="1.8.36">

### 1.8.36 — the dev server stops inventing directory indexes

A directory with no `index.html` now answers **404** instead of listing its file
names. That matches every host this project deploys to; the listing only ever
appeared locally, and it made a missing page look like a working one. Nothing to
do — unless you relied on browsing `output/` through the dev server, in which
case use a file manager or `ls`.

Date archives (`/YYYY/`, `/YYYY/MM/`) are **opt-in**: set `date_archives: true`
to publish them. Existing sites are untouched.

</div>

<div class="upgrade-step" data-since="1.8.35">

### 1.8.35 — category archives follow the source site's own address

If your `metadata.json` records a `link` for a category (every WordPress
migration does), its archive now renders at that address rather than under
`/category/`. **This changes archive URLs for migrated sites** — which is the
point: those are the addresses the content, the menu and the search results
already point at. The built-in path is emitted as a 301, so nothing that
pointed at the old form breaks. A site whose categories carry no `link` is
untouched.

The bundled themes now render `.Site.Menus.primary` when a migration brought
navigation across, and the comment thread on posts. Your own theme is
unaffected; both are opt-in by virtue of being in the theme you choose.

</div>

<div class="upgrade-step" data-since="1.8.34">

### 1.8.34 — nested categories move to their real address

A category with a `parent` now renders at `/category/<parent>/<child>/`, the
address WordPress served, instead of a flat `/category/<child>/`. **If your
`metadata.json` nests categories, those archive URLs change** — the flat path
is emitted as a 301 in `_redirects`, so existing links keep working on hosts
that honour it (Cloudflare Pages, Netlify). A site whose categories are all
top-level is untouched.

### 1.8.34 — migrated sites can keep their navigation

`ssg migrate` now accepts `--auth-user`/`--auth-pass` (or `--auth-token`) and
brings the source site's menus with the content — WordPress refuses them to an
anonymous caller, which is why migrated sites came up with no navigation at
all. **Nothing to do** for an existing site; to gain the navigation, re-run the
migration with credentials (an application password).

Themes read menus as `.Site.Menus.<location>`. The bundled themes start
rendering them in 1.8.35 — a theme file is read by whichever ssg you run, so it
may only use fields the previous release already had; your own theme can use
them now.

</div>

<div class="upgrade-step" data-since="1.8.33">

### 1.8.33 — comments migrate, and the dev server takes the next free port

`--content` now accepts **`comments`** for real: with **wpexporter 1.8.5 or
newer** they land in `content/<source>/comments.json`, addressed by page URL.
Older engines do not know `--no-comments`, so a `--content` selection that
leaves comments out will fail against them — **upgrade wpexporter** alongside
ssg (`snap refresh wpexporter`, or `go install
github.com/tradik/wpexporter/cmd/wpexporter@latest`).

While you are there: `--content` opts every **unlisted** kind out. If your
migration command says `--content pages,posts,media`, it has been leaving the
theme's own post types *and* the comments behind. The full list:

```bash
ssg migrate wordpress https://example.com --content pages,posts,media,custom,comments
```

`--http` no longer stops when the port is taken: it walks forward (8888 → 8889
→ …) and prints where it landed. **Nothing to do** — but a script that assumed
the exact port should read the announced address, or pin the port by freeing it
first.

</div>

<div class="upgrade-step" data-since="1.8.32">

### 1.8.32 — a migration takes only the media the content uses

`ssg migrate` now downloads the files your pages and posts actually reference
(featured images and in-content media, with their size variants) instead of the
source site's entire media library — which on a long-lived WordPress holds every
crop of every upload plus the leftovers of removed plugins.

**Nothing to do** for a site you have already migrated: this changes what a new
migration fetches, not what is on disk. Re-running one will pull fewer files
than before; pass **`--all-media`** to keep the old behaviour.

</div>

<div class="upgrade-step" data-since="1.8.31">

### 1.8.31 — builds now report unrenderable markup

`check_markup` is on (`warn`) by default — the only check that is. It names
source Markdown whose markup is indented four columns or more, which
CommonMark renders as a literal code block, so the visitor reads `</div>`
instead of the page. **You may see new warnings on content you have had for
months**; they describe what your site already serves. Fix them with
`ssg repair --fix`, or silence the check with `check_markup: ""` (or
`--no-check-markup`). It never fails a build unless you set `strict`.

Nothing else changes: `title`, `description` and `colors` are new optional
config keys, and `ssg migrate` filling them in only ever writes keys your
config does not already have.

</div>

<div class="upgrade-step" data-since="1.8.30">

### 1.8.30 — migrated sites keep their menus, and tracking stays opt-in

`ssg migrate --content …` no longer disables the site's metadata: tags, users
and menus now always ship (exclude them with `no-menus` / `no-tags` /
`no-users`). If a migration of yours came up without navigation, re-run it.

The tracking ids a migration records are rendered only when you set
`analytics: true` — they are never injected because content moved.

</div>

<div class="upgrade-step" data-since="1.8.29">

### 1.8.29 — the snap carries the migration engine

`ssg migrate wordpress` works in the snap out of the box: the engine
(`wpexporter`) ships inside it, because strict confinement cannot reach the
host's copy. **Do nothing** beyond `snap refresh static-site-generator`.

</div>

<div class="upgrade-step" data-since="1.8.28">

### 1.8.28 — `migrate` becomes a reserved subcommand

`ssg migrate <provider> <url>` migrates a live site into an SSG project
([docs/MIGRATE.md](MIGRATE.md)). **Do nothing** — unless a content source
directory of yours is literally named `migrate`: the bare positional form
`ssg migrate <template> <domain>` now runs the subcommand instead, so build
that source with `--source=migrate`.

</div>

<div class="upgrade-step" data-since="1.8.27">

### 1.8.27 — the AI cache moved under .ssg-cache/

All disk caches now live under one root: `.ssg-cache/images/`,
`.ssg-cache/external-sources/` and — newly — `.ssg-cache/ai/` (previously
`.ai-cache/`). **Do nothing**: existing AI answers are found in the old
location and adopted by copy, so nothing is re-generated and no generated
text changes. Image cache keys are unchanged (golden-tested) — no
reconversion happens on upgrade.

Two follow-ups, both optional:

- If you **committed `.ai-cache/`** for reproducible CI builds, keep it —
  it stays readable. New answers land in `.ssg-cache/ai/`; commit that path
  going forward (or set `ai.cache_dir` explicitly to keep one location).
- Once a build has run, `.ai-cache/` contents are duplicated and the old
  directory can be deleted: `ssg cache stats` shows it as `ai (legacy)`
  while it exists.

New CLI for all of it: `ssg cache stats`, `ssg cache clean
[--namespace=images|external-sources|ai]`, `ssg cache gc [--dry]`.

</div>

<div class="upgrade-step" data-since="1.8.26">

### 1.8.26 — `{{ .Content }}` renders, and frontmatter `excerpt:` is read

Two long-standing bugs are fixed, and each shows up as a diff rather than an error:

**Root `.Content` now renders.** In `page.html`/`post.html` the root `.Content`
was raw Markdown wrapped as HTML, so a theme printing `{{ .Content }}` shipped
literal `## headings` and `**bold**`, while `{{ .Content | safeHTML }}` raised a
type error. Now `.Content` is the rendered HTML and `safeHTML` also accepts an
already-rendered value, so both forms work. The bundled themes use
`{{ .Post.Content | safeHTML }}` and are unaffected; a theme that carried the
workaround keeps working.

**Frontmatter `excerpt:` is read.** A frontmatter `excerpt:` used to vanish into
`Extra` and never reach `page.Excerpt` — empty meta descriptions, card summaries
and feed summaries on WordPress migrations. It now fills the excerpt (a `##
Excerpt` section still wins), and `auto_excerpt` derives one from content that
opens with a raw-HTML block.

**Do nothing** — but expect summaries, meta descriptions and feed entries that
were blank to gain text. If you had worked around the empty excerpt by writing a
`## Excerpt` section, that section still takes precedence.

On the snap, `webp: true` now works under strict confinement (cwebp is bundled) —
refresh the snap.

</div>

<div class="upgrade-step" data-since="1.8.25">

### 1.8.25 — the dev server reloads the browser itself

`ssg --http --watch` now injects a tiny live-reload client into every served
page: after a successful rebuild the tab refreshes on its own, and a failed
build shows an error bar with the message instead of silently serving the stale
page. Nothing is written to `output/` for this — the script is added only to the
served HTML, never to the files on disk or to the published `.md` copies.

**Do nothing** — it is a development convenience and production output is
unchanged. Pass `--no-auto-reload` (or set `auto_reload: false`) if you would
rather refresh by hand.

The rest of 1.8.25 is opt-in and needs no action: `markdown_publish` (a Markdown
copy of every page plus `llms.txt`, for AI agents), `clean_special_chars`,
`output_encoding`, `robots_rules` and the home-page card limits are all off or
default-compatible until you enable them — see
[CONFIGURATION.md](CONFIGURATION.md) and [AI-AGENTS.md](AI-AGENTS.md).

</div>

<div class="upgrade-step" data-since="1.8.24">

### 1.8.24 — the home page now emits structured data

`seo: true` injects JSON-LD, OpenGraph and hreflang for posts and pages, but the
index was rendered without a page context, so the SEO block was skipped there
entirely — the page a crawler reaches first had none of it, and the `WebSite`
type it was supposed to carry was unreachable code.

**Do nothing**, but expect a diff: your home page gains a JSON-LD block, an
`og:url` and a canonical it did not have. If you were injecting those from your
own theme, check you are not now emitting them twice — SSG skips its own block
when the page already provides one, so a theme that emits `og:title` is left
alone.

Site-wide `schema:` defaults reach the home page for the first time too.

</div>

<div class="upgrade-step" data-since="1.8.23">

### 1.8.23 — rebuild if you minify JavaScript

`--minify-js` stripped comments with a regex, which cannot tell a comment from
the same characters inside a string. So this

```js
function f() { return "/*" + "x" + "*/"; }
```

minified to `return "";` — the scan ran from the `/*` in the first literal to
the `*/` in the third and took the closing quote with it. **The damaged output
often still parses**, so the build reported success and nothing in it warned.

**If you use `--minify-js` (or `--minify-all`), rebuild and redeploy.** Anything
already published may be silently altered, and the shapes most at risk are
vendored libraries — a CSS-comment parser holds `/*` and `*/` in strings, which
is how this was found. Grepping your deployed JS for an unterminated string is
not practical; rebuilding is.

**If you disabled JS minification to avoid this, you can turn it back on.**

CSS was affected the same way (`content: "/*"` is legal), and is fixed too.

</div>

<div class="upgrade-step" data-since="1.8.23">

### 1.8.23 — `.md` link rewriting got narrower and follows `pretty_urls`

Three things changed, all of them output:

**Absolute URLs are no longer rewritten.** Any href ending in `.md` used to be
matched on its filename, so a link to a page's own history on a code host became
a link to the page containing it. `check_links` passed, because the target
existed. **Check any external `.md` link you have** — it was pointing at the
wrong place and now points where you wrote it.

**Rewritten links follow `pretty_urls`.** With `pretty_urls: strip` a rewritten
`[CONTRIBUTING.md](./CONTRIBUTING.md)` now emits `/contributing`, not
`/contributing.html`. If `check_redirects` was reporting those, it will stop.

**`check_orphans` stops reporting false orphans.** With `pretty_urls` set and a
nav linking `/validator`, every page was reported as an orphan while
`check_links` resolved the same links. If you switched the check off because of
that noise, it is worth switching back on.

**Do nothing** for any of these — they are corrections. The first is the one to
look at, because it changed where a link goes.

</div>

<div class="upgrade-step" data-since="1.8.22">

### 1.8.22 — `related`'s three-argument form is now `relatedIn`

Two different functions were registered under the name `related`, and the
two-argument one won. So the three-argument form the reference documented never
ran: a theme using it failed with *"wrong number of args for related"*, every
post was skipped, and the build still reported success — which on a first build
reads as though the content never loaded.

**If you use `related page n`, do nothing** — that is the form that always ran,
and it is unchanged. **If you followed the old reference and passed a
collection**, the name is now `relatedIn`:

```gotemplate
{{ .Site.Posts | relatedIn .Page 3 }}
```

The two rank differently, which is why both survive rather than one being
dropped: `related` scores shared tags and keywords over the site's own posts,
`relatedIn` scores shared tags (3) > categories (2) > same author (1) over the
collection you hand it.

</div>

<div class="upgrade-step" data-since="1.8.22">

### 1.8.22 — `formatDate` actually formats

It never did: every non-string fell through to Go's `%v`, and `Page.Date` is a
`time.Time`, so themes rendered `2017-05-13 20:36:46 +0000 UTC` — including
inside `datetime` attributes, where it is not valid HTML.

**Your dates will change appearance.** The default is now `13 May 2017`, and a
layout is accepted:

```gotemplate
{{ formatDate .Date }}                {{/* 13 May 2017 */}}
{{ formatDate .Date "2006-01-02" }}   {{/* 2017-05-13 */}}
```

A zero date now renders empty instead of `1 January 0001`. Strings are still
passed through untouched, so a theme that pre-formats its dates is unaffected.

</div>

<div class="upgrade-step" data-since="1.8.22">

### 1.8.22 — a `404.html` is generated

Static hosts answer an unmatched path by falling back to `index.html` **with a
`200`** unless the output contains a `404.html`, so every dead URL read to a
crawler as another live copy of the home page.

**Do nothing** — a minimal one is generated, and a page slugged `404` still
takes precedence. If you deliberately want none:

```yaml
not_found_off: true
```

Note the new file in your output; a deploy diff will show it once.

</div>

<div class="upgrade-step" data-since="1.8.22">

### 1.8.22 — `pretty_urls` now decides what a page says about itself

It used to feed link checking only, so a site on a host that strips extensions
published canonical tags, `og:url`, JSON-LD and a sitemap naming URLs that
`308` — the one thing a canonical must not do. Those now name the URL the host
actually answers.

**Check which host you have.** `pretty_urls: true` still means what it always
meant — strip `.html` *and* add a trailing slash. Cloudflare Pages does **not**
add the slash, so on Pages the accurate setting is:

```yaml
pretty_urls: strip        # /docs/intro.html → /docs/intro
```

| Value | Host behaviour |
|---|---|
| `false` / `off` | Serves files literally |
| `strip` | Drops `.html`, no trailing slash (Cloudflare Pages) |
| `true` / `strip-slash` | Drops `.html`, adds the slash |

If you leave `true` on a host that does not add the slash, your canonical tags
will name URLs that redirect — the situation this release exists to fix. Feed
entry IDs deliberately keep the raw form, so subscribers are not re-delivered
every post.

</div>

<div class="upgrade-step" data-since="1.8.22">

### 1.8.22 — the bundled theme reads `variables.gtm_id`

`ssgtheme` had the Tag Manager container ID hardcoded, so using GTM meant
editing the theme — which put the ID in the theme rather than the site, lost it
on every theme update, and made two sites sharing the theme impossible.

**If you edited the theme to insert your container ID, move it to the config**,
or Tag Manager will stop loading:

```yaml
variables:
  gtm_id: GTM-XXXXXXX
```

When `variables.cookie_consent` is also set the loader is consent-gated rather
than live, since the container request is itself a third-party call made before
any choice.

</div>

<div class="upgrade-step" data-since="1.8.18">

### 1.8.18 — `sitemap.xml` drops pages marked `noindex`

A page whose rendered HTML carries a `noindex` robots directive is no longer
listed in the sitemap. Submitting a URL you have asked search engines to ignore
is a contradiction, so the two now agree.

**Do nothing** unless you were relying on those URLs being listed for something
other than search — a link checker or a warm-up crawler reading `sitemap.xml`
will see fewer URLs than before.

In the same release, `seo: true` fills a missing meta description from the
front-matter excerpt instead of leaving it out. If you deliberately shipped
pages with no description, they now have one.

</div>

<div class="upgrade-step" data-since="1.8.17">

### 1.8.17 — builds render in parallel by default

Rendering now uses a worker pool sized to the machine. Output is byte-identical
to the sequential build, which is enforced by `make determinism` on every
change, so this is a speed change and not an output change.

**Do nothing.** If you need the old behaviour — a constrained CI box, or
debugging — turn it off:

```bash
ssg --workers=0 …          # or build_workers: 0 in the config
```

Unset means "one worker per CPU"; `0` means sequential; any other number is
taken exactly.

</div>

<div class="upgrade-step" data-since="1.8.15">

### 1.8.15 — `rewrite_md_links` defaults to on

A relative `.md` link in your content is rewritten to the final output URL. It
had to be switched on before; now it is on unless you switch it off.

**Do nothing** in almost every case: a raw `.md` link in a built site either
404s or serves the Markdown source, so the old default was rarely what anyone
wanted. Opt out only if you were deliberately shipping `.md` links:

```yaml
rewrite_md_links: false
```

The rewriting itself did not change — only whether it runs by default.

</div>

<div class="upgrade-step" data-since="1.8.7">

### 1.8.7 — `seo_off` does what it says again

Between 1.8.2 and 1.8.6, `seo_off` and `--seo-off` were accepted but did
nothing, because SEO injection had become opt-in and there was nothing left to
turn off. From 1.8.7 the key is honoured again and forces SEO off.

**Check for a stale `seo_off: true`** left in a config from before 1.8.2. It was
harmless for five releases and is not any more: combined with `seo: true` it
wins, and your OpenGraph tags disappear.

Also in this release, every value flag accepts both spellings — `--flag=value`
and `--flag value` — which undoes the 1.8.1 restriction below.

</div>

<div class="upgrade-step" data-since="1.8.2">

### 1.8.2 — SEO injection became opt-in

**This is the one most likely to surprise you.** The generator-level OpenGraph,
Twitter and JSON-LD partial is off by default. SSG will not rewrite your
rendered `<head>` unless asked.

The reasoning is that SEO injection *modifies your HTML*, unlike the sitemap and
`robots.txt`, which write separate files and stay on. Anything that edits your
markup should be something you asked for.

**If you relied on automatic OpenGraph tags, turn them back on:**

```yaml
seo: true
```

```bash
ssg --seo …
```

Skip this and the build still succeeds — it simply stops emitting the tags, so
the symptom shows up later as bare link previews on social platforms rather than
as a failure at build time. It is worth checking one page's `<head>` after
upgrading.

</div>

<div class="upgrade-step" data-since="1.8.1">

### 1.8.1 — boolean and simple string flags wanted `--flag=value`

For six releases, boolean and simple string options had to be written joined:
`--flag=value`, not `--flag value`.

**Applies only if you are landing on 1.8.1 through 1.8.6.** Going to 1.8.7 or
later, both spellings work and there is nothing to do.

</div>

<div class="upgrade-step" data-since="1.8.0">

### 1.8.0 — the mddb API key is not sent over plaintext

The HTTP client refuses to attach `Authorization: Bearer` over `http://` to a
non-loopback host; `https://` and loopback addresses are unaffected. The gRPC
client picks transport security from the scheme — `grpcs://` and `https://` get
TLS, `grpc://` and `http://` do not, a bare host gets TLS unless it is loopback
— and likewise refuses to send a key over an insecure channel to a non-loopback
host.

**If your mddb URL is `http://` on a remote host, move it to `https://`** (or
`grpcs://`). The key is otherwise dropped, and mddb answers as if you were
unauthenticated — so the failure looks like missing content, not like an auth
error.

Local development against `127.0.0.1` or `localhost` keeps working untouched.

</div>

<div class="upgrade-step" data-since="1.7.15">

### 1.7.15 — the dev server binds loopback only

The built-in server listens on `127.0.0.1` instead of `0.0.0.0`. A development
server reachable from the whole network by default is a hazard, so exposing it
is now a deliberate act.

**If you reached the dev server from another machine** — a phone on the same
Wi-Fi, a container, a colleague — say so explicitly:

```bash
ssg --http --host=0.0.0.0 --port=8888 …
```

There is a matching `host:` config key, defaulting to `127.0.0.1`.

</div>

<div class="upgrade-step" data-since="1.7.14">

### 1.7.14 — Go 1.26.5 or newer to build from source

The module's `go` directive was raised to go1.26.5 to pick up a `crypto/tls` fix
(GO-2026-5856).

**Only affects building from source.** Released binaries, packages, the Docker
image and the GitHub Action are all built for you and carry no toolchain
requirement.

</div>

<div class="upgrade-step" data-since="1.3.1">

### 1.3.1 — WebP conversion needs the `cwebp` binary

1.3.0 briefly used a native Go library; 1.3.1 went back to `cwebp`, which is
what makes static builds and cross-compilation possible without CGO.

**Install the WebP tools** if you generate WebP images:

```bash
sudo apt install webp      # Debian/Ubuntu
brew install webp          # macOS
```

From 1.3.4 the GitHub Action installs them for you, so Action users can skip
this.

</div>

</div>

## Everything else

The releases not listed above changed nothing you have to act on: new features
you can ignore, fixes, performance work, security hardening internal to SSG, and
documentation. They are still worth skimming in the
[changelog](https://github.com/spagu/ssg/blob/main/CHANGELOG.md) — features
arrive far more often than steps do.

## If an upgrade goes wrong

Releases are not deleted. Every previous version stays downloadable from the
[releases page](https://github.com/spagu/ssg/releases), so going back is a
matter of installing the older binary. Your content and config are untouched by
an upgrade, so a rollback needs no cleanup.

If a build breaks in a way this page does not explain, that is a bug worth
reporting rather than working around —
[open an issue](https://github.com/spagu/ssg/issues) with the version you came
from, the version you went to, and what the build printed.

<style>
.upgrade-picker { margin: 2rem 0; }
.upgrade-picker select { display: block; margin-top: 0.5rem; padding: 0.5rem; max-width: 22rem; width: 100%; }
.upgrade-count { margin-top: 0.5rem; font-weight: 600; }
.upgrade-step[hidden] { display: none; }
</style>

<script>
/* Progressive enhancement: the picker is hidden until this runs, so with no
   JavaScript every step is shown and the page is still complete and correct.
   Filtering only ever hides steps that do not apply. */
(function () {
  var picker = document.querySelector('.upgrade-picker');
  var select = document.getElementById('upgrade-from');
  var count = document.querySelector('.upgrade-count');
  var steps = Array.prototype.slice.call(document.querySelectorAll('.upgrade-step'));
  if (!picker || !select || !steps.length) return;

  /* Compare dotted numeric versions of any length — 1.7.8.1 sorts after 1.7.8
     and before 1.7.9, so a shorter version is not treated as larger. */
  function cmp(a, b) {
    var x = a.split('.').map(Number), y = b.split('.').map(Number);
    for (var i = 0; i < Math.max(x.length, y.length); i++) {
      var d = (x[i] || 0) - (y[i] || 0);
      if (d) return d < 0 ? -1 : 1;
    }
    return 0;
  }

  function apply() {
    var from = select.value;
    if (!from) {
      steps.forEach(function (s) { s.hidden = false; });
      count.textContent = '';
      return;
    }
    var shown = 0;
    steps.forEach(function (s) {
      /* A step applies when it landed after the version you are on. */
      var applies = cmp(s.getAttribute('data-since'), from) > 0;
      s.hidden = !applies;
      if (applies) shown++;
    });
    count.textContent = shown === 0
      ? 'Nothing to do — upgrading from ' + from + ' is a drop-in replacement.'
      : shown + (shown === 1 ? ' step applies' : ' steps apply') + ' when upgrading from ' + from + '.';
  }

  select.addEventListener('change', apply);
  picker.hidden = false;
})();
</script>
