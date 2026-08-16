# Migrating a site into SSG (`ssg migrate`)

One command takes a live site to a working SSG project:

```bash
ssg migrate wordpress https://example.com --content pages,posts,media
```

It scaffolds the project when none exists (config, `content/<source>/`,
`static/`, `.gitignore` — existing files are never overwritten), pulls the
content into SSG's native model (Markdown with frontmatter, `metadata.json`,
`media/`) and builds the site. The run ends with an honest report — what
landed, what was skipped and why — plus the next step.

## Providers

```bash
ssg migrate --list
```

| Provider | Engine | Content kinds |
|---|---|---|
| `wordpress` | [wpexporter](https://github.com/tradik/wpexporter) **≥ 1.8.11** (REST API) | `pages`, `posts`, `media`, `tags`, `users`, `menus`, `products` |

The migration asks the engine for the SSG format (`--ssg-sections`) and for a
metadata crawl (`--assisted-crawl`), so `content/<source>/metadata.json`
arrives with `marketing` (verification tokens, social profiles, og:image,
favicon, theme colour) and `analytics` (GA4, GTM, Pixel, …) alongside the
content. **wpexporter 1.8.11 is the minimum.** Every export this asks for depends on it —
the SSG section markers (1.8.1), the media scope (1.8.2), custom post types
(1.8.4), comments (1.8.5), and the fixes to ordered lists, term slugs, post-loop
pages and shortcode expansion that landed through 1.8.11. An older engine is
**refused before anything is written**, rather than producing an export that
looks complete and is not:

```text
❌ wpexporter 1.8.4 is too old — ssg migrate needs 1.8.11 or newer.
```

A banner ssg cannot read (a wrapper, a fork) is reported and the run continues:
a formatting difference is not proof of an old engine. The snap bundles a
current one, so `snap refresh static-site-generator` is the fix there.

Providers are built into the `ssg` binary — a new source type is a new
provider behind the same interface. The `wordpress` provider delegates the
data pull to **wpexporter**, the same way WebP output delegates to `cwebp`:
it is an optional external tool, discovered on `PATH`. When it is missing,
`ssg migrate` explains how to install it:

```bash
go install github.com/tradik/wpexporter/cmd/wpexporter@latest
# or
snap install wpexporter
```

The migration report names the engine that ran, including where it came from:

```text
⚙️  engine: wpexporter 1.8.7 (bundled with the snap)
```

**Snap users need none of this**: the `static-site-generator` snap bundles the
engine (since 1.8.29), because a strictly confined snap sees only its own
files — the host's `wpexporter` is invisible to it, and installing the
`wpexporter` snap does not help either, since one snap cannot execute another.
Same reason `cwebp` is bundled for WebP output. The bundled copy is built from
the exporter's latest release **at the moment the snap is built**, so it is
refreshed by every ssg release and by a weekly rebuild — if the report names an
older engine than you expect, that is why, and `snap refresh` is the fix.

## Selecting content

`--content` names what to fetch; everything else is skipped. Without the
flag the provider's full default export runs.

```bash
ssg migrate wordpress https://example.com --content pages,posts
```

**Site metadata always ships.** Tags, users and menus describe the site around
the content, so they are exported whether or not `--content` lists them — a
migration that silently drops the navigation, the category names and the post
authors is not a migration. Exclude them deliberately:

```bash
ssg migrate wordpress https://example.com --content pages,posts,no-menus
```

Kinds: `pages`, `posts`, `media`, **`custom`**, **`comments`**, `products`,
`tags`, `users`, `menus`. `custom` is every post type a theme or plugin
registered — Services, Portfolio, Team — which on a real site is often more
documents than pages and posts together. **Naming `--content` at all opts every
unlisted kind out**, so `--content pages,posts,media` leaves both the theme's
own types and the readers' comments behind. List what you want:

```bash
ssg migrate wordpress https://example.com --content pages,posts,media,custom,comments
```

An unknown kind is a hard error (a typo must not silently export the whole
site). A recognised-but-undeliverable kind is reported as skipped in the final
summary, never dropped silently — there are none today.

Passing the engine's own flags (`--no-custom-types`, `--no-comments`) to
`ssg migrate` is rejected with the `--content` equivalent, because they are one
command apart and the mistake costs a whole content type.

### Comments

Reader comments are the one content a site owner did not write and cannot
re-create. They come from `/wp/v2/comments`, which a public WordPress serves
without authentication — and serves approved comments only, which is what a
migration wants (pending and spam rows are moderation state, not content).

They land in `content/<source>/comments.json` (wpexporter 1.8.5+), addressed by
**page URL** rather than by WordPress's post ID, which means nothing after a
migration:

```json
{
  "total": 128,
  "pages": 31,
  "comments": [
    {
      "id": 4711, "post": 812, "parent": 0,
      "post_url": "/blog/wms-implementation-pitfalls/",
      "author": "Jan Kowalski",
      "date": "2024-03-01T10:00:00Z",
      "content": "<p>Świetny tekst — u nas WMS wszedł dokładnie tak.</p>",
      "status": "approved"
    }
  ]
}
```

Records are sorted by id, so a reply never precedes the comment it answers when
they are replayed into a table with a parent reference — for example the D1
schema of the [comments worker](WORKERS.md) (`ssg new worker comments`), whose
`url` column is exactly this file's `post_url`. The migration report states how
many arrived; a site whose REST route is disabled or gated migrates without
them and says so.

Comments reach each page's template as `.Comments`, threaded by parent and in
the order they were written, and the bundled theme renders them on posts:

```gotemplate
{{with .Comments}}{{range .}}<b>{{.Author}}</b> {{.Content | safeHTML}}
{{range .Replies}}<b>{{.Author}}</b> {{.Content | safeHTML}}{{end}}{{end}}{{end}}
```

### Media: only what the content uses

A migration downloads the files the content references, not the whole library.
WordPress keeps a dozen renditions of every upload and a theme demo leaves its
own behind: one real site's library was 5,255 files and 197 MB, of which 74
files (2.8 MB) were ever referenced — and ssg generates its own responsive
variants regardless. `--all-media` takes everything:

```bash
ssg migrate wordpress https://example.com --all-media
```

Files the library does not list — a page builder's own crops, the favicon, the
`og:image` — are downloaded and localised too (wpexporter 1.8.2+), so the
migrated site does not keep fetching images from the host it was migrated off.

## Live mode: `--watch --http`

```bash
ssg migrate wordpress https://example.com --content pages,posts,media --watch --http
```

Live mode migrates in front of your eyes, in this order:

1. **Server first** — the project is scaffolded (if missing) and the
   watch+HTTP server starts immediately, printing its address
   (`http://127.0.0.1:8888`).
2. **Data lands incrementally** — the engine writes pages, posts and media
   into `content/` as it fetches them; the watcher rebuilds after each batch
   and auto-reload refreshes the browser, so you watch the site fill up.
3. **Report + next step** — the summary prints and the server keeps running
   until Ctrl+C.

`--host` and `--port` address that server, exactly as they do for `ssg --http`:

```bash
ssg migrate wordpress https://example.com --watch --http --port 8889
```

A port already in use shifts forward (8889 → 8890 → …) and the address actually
served is the one announced — a migration is too long to lose to someone else's
dev server.

Without the flags, the same migration runs as a plain batch: fetch
everything, build once, report, exit.

## What the site's own wiring becomes

The crawl writes two blocks into `content/<source>/metadata.json`, and ssg
picks both up:

| Block | Contents | Where it goes |
|---|---|---|
| `marketing` | favicon, apple-touch-icon, theme colour, `og:site_name`, default `og:image`, `twitter:site`, social profile links, verification tokens | `.Site.Marketing` in templates; injected into `<head>` when `seo: true` (only what the theme did not already emit) |
| `analytics` | GA4, GTM, Pixel, Hotjar, Clarity … ids | `.Site.Analytics` in templates; rendered only with **`analytics: true`** |
| `marketing.colors` | the theme's palette by role — primary, secondary, accent, text, background, link | `.Site.Colors.<role>` in templates, `--ssg-color-<role>` on `:root`, and written into `colors:` in the config |
| `site` | the name, tagline and timezone the CMS holds in its own settings | written into `title:`, `description:` and `timezone:` in the config; `.Site.Title` / `.Site.Description` |

An exporter may write an `analytics` id as a string, as a list (the same
container found in the head and again in a plugin's footer) or as a bare
number. All three are read; the first usable id per vendor is the one the
generator emits, and a value it cannot read as an id is skipped rather than
failing the build (#131).

### The config is completed from the export

After the fetch, `migrate` reads `metadata.json` and fills in the configuration
keys the source site already answered — `title`, `description`, `timezone` and
`colors` — so the first build carries the site's own name and colours instead of
an untitled site in the starter palette:

```text
🪪 Completed .ssg.yaml from the source site:
   ✅ title: Magna Valor
   ✅ description: Supply Chain Global Advisory
   ✅ timezone: Europe/Warsaw
   ✅ colors: 6 colour(s) from the source theme
```

Only keys the config **does not have** are written, so re-running a migration
never undoes an edit of yours, and the file is edited as a YAML document — your
comments and key order survive. A timezone WordPress reports as a bare offset
(`UTC+2`) is skipped rather than written as something that will not load: it
carries no DST rules.

The palette arrives only when the engine collects it: that is wpexporter
**1.8.2**, which is **not released yet**
([tradik/wpexporter#27](https://github.com/tradik/wpexporter/issues/27)). With
1.8.1 the migration completes `title`, `description` and `timezone` as
described, and simply finds no colours. The same release will export **custom
post types** — a theme's Services, Portfolio or Team entries, silently lost
today ([#28](https://github.com/tradik/wpexporter/issues/28)) — which will land
under `pages/<type-slug>/` and keep their original URLs.

Tracking is opt-in on purpose: loading third-party JavaScript on every page is
your decision, not a side effect of moving content. Verification tokens and
icons are plain metadata — they load nothing — so they ride with `seo`.

```yaml
seo: true
analytics: true      # only when you want GTM/GA4 live again
```

## Navigation needs credentials

WordPress gates menus (and its own settings) behind `edit_theme_options`, so an
anonymous export comes back with none — and the site builds, silently, with no
navigation at all. Pass credentials and the menus travel with the content:

```bash
ssg migrate wordpress https://example.com --auth-user editor --auth-pass "$WP_APP_PASSWORD"
ssg migrate wordpress https://example.com --auth-token "$WP_TOKEN"
```

Use a WordPress **application password**, not the account's own. Credentials are
handed to the engine and **never written to `.ssg.yaml`** — that file gets
committed.

Menus reach templates as `.Site.Menus.<location>` (and `.<slug>` for a menu the
theme never assigned), each with `.Tree` giving the entries nested and in the
site's own order:

```gotemplate
{{with index .Site.Menus "primary"}}
<nav>
  {{range .Tree}}
    <a href="{{.URL}}">{{.Title}}</a>
    {{range .Children}}<a href="{{.URL}}">{{.Title}}</a>{{end}}
  {{end}}
</nav>
{{end}}
```

The bundled themes adopt this from 1.8.35 — a theme file is also read by
whatever ssg the reader has pinned, so it can only use fields the previous
release already knew. Paste the snippet above into your own theme today. When
no menus arrive, the report says why rather than leaving you to guess:

```text
⚠️  menus: not readable without authentication — WordPress gates them behind
    edit_theme_options; re-run with --auth-user/--auth-pass or --auth-token
```

### Comments and the engine's age

`comments` can be excluded from `--content` only by **wpexporter 1.8.5 or
newer**. On an older engine they are exported regardless and the report says
so, rather than failing a migration over a flag the engine does not know:

```text
⚠️  comments: exported anyway — this wpexporter cannot be asked to skip them
    (needs 1.8.5); delete content/<source>/comments.json if you do not want them
```

## The theme's own post types

Services, Portfolio, Team — a theme registers its own types, and they carry
real pages. They are exported by default; select or skip them:

```bash
ssg migrate wordpress https://example.com --custom-types cpt_services,cpt_team
ssg migrate wordpress https://example.com --no-custom-types
```

## After the migration

The site's WordPress front page (`link: "/"`) becomes the front page here too,
so the generated post listing needs a home of its own:

```yaml
posts_page: blog     # /blog/ — otherwise the listing is not generated
```

Check the content renders as markup, not as text:

```bash
ssg repair             # report anything a page builder left indented
ssg repair --fix       # rewrite those files in place
```

Exports made with wpexporter before 1.8.2 indent their builder markup, which
CommonMark reads as a code block — the page then shows `</div>` to the visitor.
Every build also reports it (`check_markup`, on by default). See
[CONFIGURATION.md](CONFIGURATION.md#validating-the-built-output).

The migrated site builds on the `simple` starter theme. To rebuild the
source site's look, hand the project to an AI agent over the
[MCP server](MCP.md). The server speaks stdio, so **the assistant launches
it** — you register it once:

```bash
claude mcp add ssg -- ssg mcp          # Claude Code, this project
```

Claude Desktop takes the same thing as JSON in `claude_desktop_config.json`:

```json
{"mcpServers": {"ssg": {"command": "ssg", "args": ["mcp"], "cwd": "/path/to/project"}}}
```

Add `--http` (`... -- ssg mcp --http`) to get a live preview on
`http://127.0.0.1:8888` that refreshes after every change the agent makes.

Then ask the agent to study the original site and recreate its template on top
of the migrated content — the content model is already in place, so the agent
only designs.

## Options

| Flag | Meaning |
|---|---|
| `--content a,b,c` | Content kinds to fetch (default: everything the provider offers): `pages`, `posts`, `media`, `custom`, `products`, `tags`, `users`, `menus` |
| `--all-media` | Download the whole media library instead of only the files the content references |
| `--watch --http` | Live mode: server first, then watch the data load |
| `--source NAME` | Content source directory name (default: the site's host, `www.` stripped) |
| `--auth-user U --auth-pass P` | Credentials for the source CMS (menus and settings need them) |
| `--auth-token T` | Bearer token instead of the user/password pair |
| `--custom-types a,b` | The theme's own post types to export |
| `--no-custom-types` | Skip the theme's own post types |
| `--no-crawl` | Skip the SEO/marketing crawl (faster; no tracking ids, social profiles or icons) |
| `--quiet`, `-q` | Suppress progress output |
| `--list` | List built-in providers with versions |

## Notes

- `migrate` is a reserved subcommand. A source directory literally named
  `migrate` still builds via `--source=migrate`.
- Old URLs are preserved: the WordPress export carries each page's original
  `link:` in frontmatter, and SSG publishes flat URLs from it, so permalinks
  keep working without a redirect map.
- Running `ssg migrate` again re-exports into the same source directory;
  the scaffold step never overwrites files it finds.
