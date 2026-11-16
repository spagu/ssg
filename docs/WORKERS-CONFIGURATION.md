# Worker configuration

The `.ssg.yaml` keys that bring workers into a build: `worker:` and
`workers:` (Pages Functions copied into the output), `modules:` (the module
registry the `core` worker reads) and `web_push:`. Which templates exist and
what each one does is in [WORKERS.md](WORKERS.md); their secrets, which never
go in `.ssg.yaml`, are in [WORKERS-SECRETS.md](WORKERS-SECRETS.md); the
panels' look (`admin_theme:`) is in [ADMIN-THEME.md](ADMIN-THEME.md).

## Pages Functions (`worker`, `workers`)

| Key | Default | Notes |
|---|---:|---|
| `worker.dir` | empty | Functions project (or dir with a prebuilt `_worker.js`) |
| `worker.mode` | `functions` | `functions` or `worker` |
| `worker.routes_include` | `["/api/*"]` | paths that invoke the Function |
| `worker.routes_exclude` | empty | paths carved back out to static |
| `worker.wrangler_config` | empty | wrangler config outside the project root |
| `worker.source` | empty | Fetch the worker from a repo or ZIP URL into `dir` instead of vendoring it |
| `worker.auth` | empty | Credentials for a private `source`: `type` `bearer`, `basic` or `header`; secret fields must be `$ENV` references |
| `worker.config` | empty | Free-form per-worker settings, passed to the worker phase as-is |
| `workers` | empty | A list of worker definitions (each with a `name` plus the keys above); supersedes `worker:` when set |
| `workers[].name` | — | Identifies the worker in logs, collision messages and its `config:` key |

```yaml
worker:
  dir: workers/contact-form   # a Pages Functions project (contains functions/)
  mode: functions             # "functions" (default) or "worker"
  routes_include:             # what reaches the Function; default ["/api/*"]
    - /api/*
  routes_exclude: []          # carve static paths back out if needed
  wrangler_config: ""         # optional wrangler.toml outside the project root
```

At build time SSG copies `dir` (its `functions/` tree, or a prebuilt
`_worker.js`) into the output, and writes `_routes.json` from
`routes_include`/`routes_exclude`, so every path **not** matched by `include`
is served as a static asset and never invokes the Function. `worker:` is empty
by default; without it the build is static-only, unchanged.

### Several workers: the `workers:` list

One site can need more than one worker — a cookie-consent endpoint and a
comments API, say. `workers:` is a list of **independent** definitions, each
with its own routes, config and source; the singular `worker:` stays for
back-compat.

```yaml
workers:
  - name: cookie-consent
    dir: workers/cookie-consent
    routes_include: [/api/consent]
    config:                       # free-form, surfaced to this worker
      countries: [DE, FR, PL]
  - name: comments
    source: https://github.com/example/ssg-comments   # fetched, not vendored
    auth:                                              # private repo (optional)
      type: bearer
      token: $GITHUB_TOKEN                             # env ref, never a literal
    routes_include: [/api/comments]
```

Cloudflare Pages serves a **single** `functions/` tree and one `_routes.json`
per project, so the workers' functions are copied into that shared tree and
their routes are combined. Combining **normalises**: Cloudflare rejects a
`_routes.json` whose rules overlap, so a rule already covered by a splat in the
same list is folded into it — `/api/contact` and `/api/consent/*` beside
`/api/*` are published as `/api/*` alone, and the build says which rules were
absorbed. Include and exclude are normalised separately. This matters most for
a middleware worker, which covers every route, and for a worker that names no
routes at all: it defaults to `/api/*`. **Two workers claiming the same output
file is a hard error** (never a silent overwrite). Only one worker may use
`mode: worker` (a project has one `_worker.js`).

A `source:` is fetched into `dir` (default `workers/<name>`) once and reused on
later builds, so a build is not gated on the network. Vendor the fetched worker
(commit it) for reproducible builds, or keep `source:` to track upstream.

A worker's `public/` files are copied to the output root after everything
else the build writes. Since 1.9.9 (#420) a worker file landing on a file the
build already wrote — a rendered page, a static file, a feed or the sitemap —
stops the build and names both (`output collision at /core/index.html: worker
"core" (public/core/index.html) and content/pages/core.md are both written
there …`): only you can say which the site should serve. A byte-identical
file is not a clash, nor is the worker's own copy left by an earlier build.

### `mode: functions` vs `mode: worker`

| Mode | `dir` contains | Deploy path |
|---|---|---|
| `functions` (default) | a `functions/` directory of `.ts`/`.js` handlers | `wrangler pages deploy` (Cloudflare builds them) |
| `worker` | a prebuilt, bundled `_worker.js` | pure-Go Direct Upload — no Node/wrangler needed |

Use `functions` for normal development. Use `worker` when you have already
bundled your Worker yourself (e.g. with esbuild in CI) and want the
dependency-free Direct Upload deploy.

## The module registry (`modules`)

| Key | Default | Notes |
|---|---:|---|
| `modules` | empty | Modules deployed outside this build (a standalone Worker such as `newsletter-ui`), for the `core` worker's registry |
| `modules[].id` | — | The module's id, as in its `module.json`. Required |
| `modules[].url` | — | The module's base path on the site's own host (`/newsletter-admin`). Required |
| `modules[].dir` | empty | Where its `module.json` is, so the build reads and checks it; without it `core` asks the module for its manifest at runtime |
| `check_modules` | `warn` | `off`, `warn` or `strict` (see the [checks table](CONFIGURATION.md#seo-and-validation)) |

When a worker in `workers:` has a `module.json` with `id: "core"`, the build
writes a module registry for it (`functions/_ssg-modules.json`): every bundled
worker's manifest, the `modules:` entries, and the templates this ssg ships
that the site does not use. Without such a worker nothing new is written and
the output is the same as before. Details:
[WORKER-MODULES.md](WORKER-MODULES.md#the-module-registry).

```yaml
modules:
  - id: newsletter-ui
    url: /newsletter-admin      # the module's base path on the same host
    dir: workers/newsletter-ui  # optional: read title, navigation, permissions at build time
```

`url` must be a path on this host: a scheme, a host, `//`, `..`, a backslash,
a query or a fragment stops the build with the file and the key
(`.ssg.yaml: modules[0].url "https://…" must be a path on this site's host…`),
and so does an `id` that is not a module id. Those are errors whatever
`check_modules` says; the rest are findings under `check_modules`.

## Web Push

| Key | Default | Notes |
|---|---:|---|
| `web_push` | `false` | `true`, or a mapping; a mapping turns it on unless it says `enabled: false` |
| `web_push.endpoint` | `/api/push` | where the `web-push` worker answers |

Browser notifications about new posts need a server: someone has to keep the
subscriptions and sign every message with a private key. The `web-push` worker
does that (`ssg new worker web-push`, see
[its README](../workers/web-push/README.md)) and ships `sw.js` and `push.js`.
`web_push: true` makes the site ready for it:

- writes `/manifest.webmanifest` from `.Site.Title`, `marketing.theme_color` and
  the site's icons, unless the output already has one. iOS delivers push only
  to a site added to the home screen, which needs the manifest;
- puts `<link rel="manifest">` and `<script src="/push.js" defer>` in every
  page's `<head>`, skipping either one the theme already has;
- warns when `push.js` or `sw.js` is missing from the output, because the worker
  is not configured.

The theme adds only the button. `push.js` shows it in a browser that supports
push, and asks for permission only when it is clicked:

```html
<button data-push-subscribe hidden data-label-on="Notify me about new posts"
        data-label-off="Stop notifications">Notify me about new posts</button>
```

```yaml
web_push: true
feed: true            # the worker reads /feed.xml to find new posts
worker:
  dir: workers/web-push
  routes_include:
    - /api/push/*
```
