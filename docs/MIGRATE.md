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
| `wordpress` | [wpexporter](https://github.com/tradik/wpexporter) (REST API) | `pages`, `posts`, `media`, `tags`, `users`, `menus`, `products` |

Providers are built into the `ssg` binary — a new source type is a new
provider behind the same interface. The `wordpress` provider delegates the
data pull to **wpexporter**, the same way WebP output delegates to `cwebp`:
it is an optional external tool, discovered on `PATH`. When it is missing,
`ssg migrate` explains how to install it:

```bash
snap install wpexporter
# or
go install github.com/tradik/wpexporter@latest
```

## Selecting content

`--content` names what to fetch; everything else is skipped. Without the
flag the provider's full default export runs.

```bash
ssg migrate wordpress https://example.com --content pages,posts
```

An unknown kind is a hard error (a typo must not silently export the whole
site). A recognised-but-unsupported kind — `comments` today, which
wpexporter's REST export does not deliver — is reported as skipped in the
final summary, never dropped silently.

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

Without the flags, the same migration runs as a plain batch: fetch
everything, build once, report, exit.

## After the migration

The migrated site builds on the `simple` starter theme. To rebuild the
source site's look, run the [MCP server](MCP.md) and let an AI agent work
with the `designer_*` tools:

```bash
ssg mcp
```

Ask the agent to study the original site and recreate its template on top of
the migrated content — the content model is already in place, so the agent
only designs.

## Options

| Flag | Meaning |
|---|---|
| `--content a,b,c` | Content kinds to fetch (default: everything the provider offers) |
| `--watch --http` | Live mode: server first, then watch the data load |
| `--source NAME` | Content source directory name (default: the site's host, `www.` stripped) |
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
