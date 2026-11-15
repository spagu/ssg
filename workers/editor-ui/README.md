# editor-ui worker

![Cloudflare Workers](https://img.shields.io/badge/Cloudflare-Workers-F38020?logo=cloudflare&logoColor=white)
![Node tests](https://img.shields.io/badge/tests-node%3Atest-339933?logo=nodedotjs&logoColor=white)
![OpenAPI 3.1](https://img.shields.io/badge/OpenAPI-3.1-6BA539?logo=openapiinitiative&logoColor=white)
![npm dependencies: 0](https://img.shields.io/badge/npm%20dependencies-0-brightgreen)

The site's content editor: the Markdown articles and images of your site,
edited in the browser and published as one commit to the site's repository —
the site's own build then deploys them, as after any push.

> **Auth is required.** editor-ui accepts accounts of the `auth` worker only,
> on the same host: a person's session, or a service token for a script.
> There is no shared password and no Access-only mode — every publication
> needs an author and a permission per action, and a password gives neither.
> Without a complete `EDITOR_AUTH_*` set, every path answers `503`, the panel
> included.

The panel is at `EDITOR_BASE/`: the list of articles, a front matter form
and a Markdown editor with a preview, pasted images and the media library,
components and shortcodes to insert, history with restore, and the conflict
screen. Scripts and AI assistants use the same operations through the REST
API and `POST {base}/mcp` ([MCP](#mcp)).

This README is the reference. The guide for site owners — setup with the
GitHub token step by step, roles, limits per Cloudflare plan, running it
locally, troubleshooting — is [docs/EDITOR.md](../../docs/EDITOR.md),
continued in [docs/EDITOR-OPERATIONS.md](../../docs/EDITOR-OPERATIONS.md)
and [docs/EDITOR-API.md](../../docs/EDITOR-API.md).

## Install in three steps

From `workers/editor-ui`:

1. **Repository.** In `wrangler.toml`, set `EDITOR_GIT_REPO =
   "github.com/<owner>/<repo>"` and `EDITOR_GIT_BRANCH` (empty = the
   repository's default branch). GitHub only in this version.
2. **Key.** `npx wrangler secret put EDITOR_GIT_TOKEN`. Use a fine-grained
   token for that one repository with **Contents: read and write** and
   nothing else. Without the workflows permission GitHub itself refuses any
   change under `.github/workflows/` — a second, independent wall. The token
   is a secret of the Worker, never in `.ssg.yaml` or `wrangler.toml`, and
   it is sent to `api.github.com` only.
3. **Auth.** Set `EDITOR_AUTH_ISSUER`, `EDITOR_AUTH_LOGIN_URL` and
   `EDITOR_AUTH_PUBLIC_KEY` (the `AUTH_PUBLIC_KEY=` line
   `node workers/auth/scripts/auth-keys.mjs` prints). Grant the permissions
   below to roles in the accounts panel (`/auth/admin/`).

Then set `routes` to your domain and `npx wrangler deploy`. List the worker
under `modules:` in `.ssg.yaml` with its base path so `core` shows it.

## Variables

| Variable | Kind | Meaning |
|---|---|---|
| `EDITOR_BASE` | var | The path the worker owns; default `/editor`. Keep `routes` in step. |
| `EDITOR_GIT_REPO` | var | `github.com/<owner>/<repo>` — where the site's source lives. |
| `EDITOR_GIT_BRANCH` | var | The branch publications are committed to; empty = the default branch. |
| `EDITOR_GIT_TOKEN` | **secret** | The repository token (step 2). |
| `EDITOR_AUTH_ISSUER` | var | The `iss` of `auth`'s tokens, `https://<your host>/api/auth`. |
| `EDITOR_AUTH_LOGIN_URL` | var | Where a page without a session is sent, e.g. `/auth/login`. |
| `EDITOR_AUTH_PUBLIC_KEY` | var | `auth`'s public key (recommended) … |
| `EDITOR_AUTH_JWKS_URL` | var | … or `/api/auth/jwks` — exactly one of the two. |
| `EDITOR_AUTH_AUDIENCE` | var | Optional; the `aud` to accept, default the request's host. |
| `EDITOR_CONTENT_DIRS` | var | Optional override: comma-separated content folders (below). |
| `EDITOR_MEDIA_DIR` | var | Optional override: the folder pasted images go to. |
| `EDITOR_COMPONENTS_URL` | var | Optional: where the site publishes `components.json`, a path on this same site; default `/components.json` (below, "Widgets"). |
| `EDITOR_MDDB_URL` | var | Optional: the MDDB server holding articles (below). `https://` only. |
| `EDITOR_MDDB_API_KEY` | **secret** | The MDDB key; sent to `EDITOR_MDDB_URL` only. |
| `EDITOR_MDDB_COLLECTION` | var | Optional: only this collection; required without a repository. |
| `EDITOR_MDDB_LANG` | var | Optional: the build's language filter and new articles' language; default `mddb.lang`. |
| `EDITOR_MDDB_ALLOW_HTTP` | var | `"true"` allows `http://` and private or loopback hosts — a private network only. |
| `EDITOR_REBUILD_URL` | var | Optional: called after an MDDB write, on this site's origin (e.g. `/api/republish`). |
| `EDITOR_REBUILD_KEY` | **secret** | The credential sent to `EDITOR_REBUILD_URL` as `Authorization: Bearer`. |
| `RATE_LIMITER` | binding | Optional per-IP request limit (`[[ratelimits]]`). |

## Where the content is: read from the repository

The worker reads the site's configuration file from the branch — the first
of `.ssg.yaml`, `.ssg.yml`, `.ssg.toml`, `.ssg.json`, `ssg.yaml`, `ssg.yml`,
`ssg.toml`, `ssg.json`, as ssg does — plus the settings overlay
`.ssg.designer.json` beside it. It uses only `content_dir`, `source`,
`pages_path`, `posts_path`, `content_sources`, `static_dir`, `languages`,
`default_language`, `language_sections`, `shortcodes` and `mddb` (never
`mddb.url` or `mddb.api_key`). The result is cached per Worker isolate until
the branch gets a new commit.

The reader is deliberately small (no YAML library may come in). When the
file uses something it does not follow — anchors or aliases, `<<` merge keys,
`|` / `>` block values, values continued on the next line, `include:`, a
TOML file, a `$VARIABLE` for a folder — it says so and does not guess: the
summary names the problem, and every content operation answers `503` with
the file and line (`detail`) and what to set (`hint`). Then set
`EDITOR_CONTENT_DIRS` (e.g. `"content/blog, docs"`) and `EDITOR_MEDIA_DIR`;
they also override a file the reader does understand.

## MDDB as a content source

**Needs MDDB 2.18.8 or later** (document `revision` and conditional writes,
`expectedRevision`; an older server is answered `503 mddb_too_old`).

A site built from MDDB (`mddb:` in `.ssg.yaml`) is edited the same way: set
`EDITOR_MDDB_URL` and the secret `EDITOR_MDDB_API_KEY`. The collections, each
source's `type`, `section` and `filter`, and `mddb.lang` come from the site's
configuration on the branch; `EDITOR_MDDB_COLLECTION` narrows to one (or names
it when no repository is configured). The editor lists and opens only what
the build reads: a document outside a source's filter, of another type than
page/post (without a source `type`) or of another language is not found.

- **Address.** An article's `path` is `mddb:<collection>/<lang>/<key>`; every
  content route and MCP tool takes it where it takes a repository path.
  `?section=mddb:<collection>` (or `mddb:<collection>/<lang>`) lists one
  collection and is the section for `new-path`. The key is the article's
  address in MDDB; its URL on the site is its `slug` field.
- **Article = document.** The meta keys are the frontmatter keys, as ssg's
  converter reads them (`internal/mddb/converter.go`): a one-value list is a
  scalar (`tags`, `categories`, `aliases` stay lists), keys sorted, the body
  is `contentMd`. A meta key that is not a plain YAML key is kept on every
  write and listed in `fixed`. MDDB's meta is flat, so a nested value is
  refused (`fields_refused`, reason `not_flat`); comments in the block are not
  stored. `status` defaults to `publish` here — ssg's converter does so; for a
  file, frontmatter without `status` is a draft.
- **List.** Repository files first, then MDDB articles, one cursor for both;
  with both sources configured every item has `origin` (`git` / `mddb`).
  MDDB's search returns no revision, so an MDDB item's `version` is `""`:
  read the article for the version to publish over.
- **Publishing.** One MDDB article per publication, no file in the same
  publication (`400 invalid_change`), no `move`. An edit is a conditional
  write; when MDDB moved on, the answer is the same `409 conflict` as for a
  file. The answer adds `origin: "mddb"`, `rebuild` and `notice`; `commitId`
  is the new revision (`""` after a delete), `branch` the collection. MDDB
  keeps no message and no author.
- **Gaps MDDB leaves.** A new article cannot be written "only if absent", and
  a delete is not conditional: the editor reads just before, so only a write
  landing between those two requests is missed.
- **Images** go to the repository (`EDITOR_GIT_*`), committed first in a
  commit of their own; without a repository an image is refused with
  `503 media_needs_repository`.
- **Revisions** are MDDB's (`POST /v1/revisions`): newest first, pages of 30,
  named by their timestamp. They carry no author and no message (`""`).
- **The site shows a change after its next build.** With `EDITOR_REBUILD_URL`
  and `EDITOR_REBUILD_KEY` the worker starts one after each write
  (`rebuild: {triggered, status}`), e.g. through `republish-trigger`. The URL
  must be on this site's origin; the key is a dedicated secret — the caller's
  own session or token is never forwarded. Without them `notice` says
  "saved in MDDB; the site shows it after the next build".
- **The key** goes to `https://` servers that are not on a private or
  loopback address (names such as `localhost`, `*.local`, `*.internal` count
  as private), never through a redirect, and is in no answer or log.

## Permissions

| Permission | What it allows |
|---|---|
| `editor-ui:read` | List and read content files, their revisions and the media library — and open the panel and every API route. |
| `editor-ui:publish` | **Sensitive.** Commit content changes (create, edit, rename), or write them to MDDB for articles kept there. ssg renders raw HTML written in Markdown, so whoever publishes can place HTML and scripts on the site's pages: grant it with the trust you give to theme edits. Declared `"sensitive": true`: a service token carries it only when an owner allowed it (`allowSensitive`). |
| `editor-ui:media` | Add images in a publication. |
| `editor-ui:delete` | Delete a content file or an image (it stays in the repository's history), or an article from MDDB. |

Each operation checks its own permission, so the REST routes and the MCP
endpoint refuse alike; the panel only hides what a person cannot use. To
make them grantable, add them to `auth`'s `AUTH_KNOWN_PERMISSIONS`:
`node workers/auth/scripts/auth-permissions.mjs workers/*/module.json`
prints the value.

**Revocation delay.** The worker checks `auth`'s signed token itself and
never asks `auth` per request. A removed role, a blocked account or a logout
reaches it at the latest after the token lifetime plus 30 seconds — 10 min
30 s by default.

**CSRF.** A state-changing request made with the session cookie must come
from this site's own pages (`Origin`, or `Sec-Fetch-Site: same-origin`);
otherwise `403 cross_origin`. A service token in `Authorization: Bearer`
needs no such check.

## API

Paths are relative to `EDITOR_BASE`; [`openapi.yaml`](openapi.yaml) is the
reference.

| Route | Permission | Answer |
|---|---|---|
| `GET api/module` | read | The manifest, [`module.json`](module.json). |
| `GET api/health` | read | What is configured — never a value; no request to GitHub. |
| `GET api/module/summary` | read | Days since the content changed, content folders, the branch, problems. |
| `GET api/me` | read | `{kind, user, permissions, sessionExpiresAt, loginUrl, logoutUrl, refreshUrl}`. |
| `GET api/content?q=&section=&cursor=&limit=` | read | `{items: [{path, version, size, section, type, language, title, date, draft, loaded}], nextCursor, pending, truncated}` |
| `GET api/content/new-path?section=&title=` | read | `{path, slug}` — where a new article would go. |
| `GET api/content/{path}` | read | `{path, version, commit, format, bom, frontmatterRaw, fields, fixed, body}` — `{path}` URL-encoded as one segment. |
| `POST api/changes` | publish (+ delete for a `delete`, + media for an image) | `201 {commitId, url, branch, files: [{path, version}]}` |
| `GET api/media?cursor=&limit=` | read | `{dir, urlBase, limits, items: [{path, url, version, size, type}], nextCursor, truncated}` |
| `GET api/media/new-path?name=&type=&hash=` | read | `{path, url, name}` — the canonical path of an image added now. |
| `GET api/media/references?path=&cursor=` | read | `{path, url, items: [{path}], scanned, total, complete, nextCursor}` |
| `GET api/revisions/{path}?cursor=` | read | `{path, items: [{commitId, authorName, authorEmail, date, message}], nextCursor}` |
| `GET api/revisions/{path}/{commit}?cursor=` | read | `{path, commitId, …, version, text, current: {commit, version}, identical, restore: {message, change}}` |
| `GET api/revisions/{path}/diff?from=&to=&cursor=` | read | `{path, from, to, identical, tooLarge, added, removed, hunks, download}` |
| `POST mcp` | read (each tool: its operation's) | JSON-RPC 2.0 — the same operations as MCP tools, [MCP](#mcp). |

Errors are `{"error": "<snake_case>", "detail"?}`.

### Content and publishing

**What is content.** A `.md` file inside one of the site's content folders
(`content_dir`/`source` and `content_sources` of `.ssg.yaml`, or
`EDITOR_CONTENT_DIRS`). Everything else is refused with `400 invalid_path`
and a `reason` — `.ssg.yaml`, `.github/`, `workers/`, `wrangler.toml`,
dotfiles, another extension; an image sent as text gets `reason: "media"`
(images are sent as bytes — see Images below).

**The list** reads titles, dates and drafts from each file's frontmatter,
lazily: within one request it reads what the subrequest budget allows (two
requests to GitHub per file) and marks the rest `loaded: false`; asking again
fills them in from the worker's cache, which is keyed by the file's version
and so never stale. `draft` follows ssg: a file with frontmatter is published
only with `status: publish`. A file's last-change time is not listed (one
more request per file); the revisions view will show it.

**Frontmatter.** `GET api/content/{path}` splits the file as ssg does.
`fields` holds the simple top-level keys — a one-line value, a one-line
`[list]`, or `- item` lines — and `fixed` names the keys the form may not
change (nested keys, `|`/`>` block values, anchors, a key written twice).
`frontmatterRaw` + `body` are the file byte for byte. A `+++` TOML block is
kept as it is and only the body is editable (ssg itself does not read TOML
frontmatter). `bom: true` warns that the file starts with a byte-order mark,
which makes ssg read it without frontmatter.

**Publishing** is one commit with every file of the change:

```json
{"message": "Fix the intro",
 "changes": [
   {"op": "put", "path": "content/blog/posts/hello.md", "version": "<from the read>", "fields": {"title": "Hello again"}, "body": "…"},
   {"op": "put", "path": "content/blog/posts/new.md", "version": null, "text": "---\ntitle: New\n---\nText\n"},
   {"op": "move", "from": "docs/old.md", "to": "docs/new.md", "version": "<…>"},
   {"op": "delete", "path": "docs/gone.md", "version": "<…>"}]}
```

A change sends either the whole `text`, or a form edit — `fields` (only the
lines of the changed keys are rewritten; comments, order and line ends stay)
or `frontmatterRaw`, and/or `body`. At most five form edits per publication
(each reads the file first). The message must not be empty and its first line
is at most 200 characters. The author is the signed-in account's e-mail; a
service token's commit is by `service account <id>` at
`<id>@service-account.invalid` (`auth` tokens carry no display name).

**Conflicts.** `version` is the file's version as read; `null` means the file
must not exist yet. A file that changed in between is answered `409
{"error": "conflict", "files": [{path, yours, theirs, theirVersion}]}` —
both texts, nothing written; publish again over `theirVersion` once the
person has chosen. A change to another file of the branch is no conflict.
`409 branch_busy` means the branch kept moving: publish again.

**New files.** `GET api/content/new-path` turns a title into a file name the
way ssg makes slugs — lower-case `a`–`z` and `0`–`9`, everything else one
hyphen. By default ssg does not transliterate, so `Zażółć gęślą` becomes
`za-g-l`; with `slug_transliterate: true` in the site configuration (#426)
it folds Latin diacritics first (`zazolc-gesla`), and so does the editor.
The editor keeps the same rule so the file name and the URL never disagree;
both are tested against `internal/models/testdata/slug-translit.json`.

### Images

**Where they go.** The media folder is the site's
`<content_dir>/<source>/media` (ssg copies it to `/media/`); a site without
`source` uses `<static_dir>/images` (served at `/images/`). `EDITOR_MEDIA_DIR`
overrides it with a folder inside `static_dir` (served at its path below it)
or ssg's content media folder ending in `/media` (served at `/media/`);
another folder is one ssg does not publish, and image operations answer
`503 media_not_configured`. Addresses are root-relative, as an article links
them; ssg adds `base_path` at build time.

**One commit with the text.** A pasted image waits in the browser and goes
to the repository in the publication's commit, as
`{op: "put", path, version: null, base64}` next to the article. It needs
`editor-ui:media` besides `editor-ui:publish`; a publication carrying an
image without it is refused whole (`403`), text included. Deleting an image
is a `delete` like any other (`editor-ui:delete`); ask
`GET api/media/references` first — it lists the Markdown articles whose links
end in the image's path below the media folder (templates are not searched).
That search reads two requests per article and stops before the request's
budget: `complete: false` with a `nextCursor` to go on, never a silent
partial answer.

**Names.** `<mediaDir>/<yyyy>/<mm>/<slug>-<hash8>.<ext>`: the month the
image was added (UTC), the slug of its original name (`a`-`z`, `0`-`9`,
hyphens, at most 50; `image` when nothing is left), the first 8 hex
characters of the SHA-256 of the bytes, and the extension of the type the
bytes are. The panel computes it with [`src/media-name.ts`](src/media-name.ts)
(`mediaPathFor`) or asks `GET api/media/new-path`; the worker recomputes the
type and the hash from the bytes and refuses a path that does not match
(`400 invalid_image`, with `expected`). The same bytes give the same name, so
an image already on the branch is kept, not written again, and is listed in
`files` with its version.

**What is accepted.** PNG, JPEG, WebP, GIF and AVIF, recognised by their
first bytes — never by the name or a type the client sends. SVG, HTML, XML
and anything else are refused (`reason: markup` or `not_an_image`): an SVG
served from the site's host can run script there. Limits (Free plan, the
repository layer's): 10 files per publication, 2 MiB per file and per
publication; over them `413 too_large` with `limits`, before anything is
decoded, so the panel can offer to shrink the image.

**Polyglots.** A file with a valid image header and HTML appended after it is
an image to this check and is accepted — as an image. What keeps it harmless
is how it is served: ssg's output is static files, and Cloudflare serves a
`.png` with `content-type: image/png`; with `X-Content-Type-Options: nosniff`
(ssg's `_headers`, `internal/generator/headers.go`) the browser does not
second-guess that type, so the file is never rendered as a page and its
appended HTML never runs when the image is opened or embedded. What `nosniff`
does not do: it does not strip the appended bytes (anyone downloading the
file gets them), it does not help a host that serves the file with another
type or without the header (keep the generated `_headers`, or your host's
equivalent), and it does not make an image parser safe against a malformed
image — that is the browser's job.

### Widgets

`GET api/widgets` (`editor-ui:read`, MCP `widgets_list`) lists what an
article can contain beyond plain Markdown, in one list with `kind`:

- `component` — the site's typed components with their props (type,
  required, default, enum, description), from the `components.json` the
  last build published (docs/COMPONENTS.md). The published file, not the
  component folders in the repository: ssg wrote it, so the editor offers
  exactly what the build accepts, in one request. A component added on the
  branch is offered after the next deploy. The file is fetched from this
  same site (`EDITOR_COMPONENTS_URL`, default `/components.json`) with no
  credentials, no redirects, 5 seconds and 256 KiB at most; anything else
  leaves the components out with `components.status` and `components.hint`
  (`not_published`, `redirected`, `unavailable`, `too_large`, `invalid`,
  `bad_url`), and the rest of the list is still answered.
- `shortcode` — `shortcodes:` of `.ssg.yaml` (`{{name}}`).
- `block` — table, code block, footnote and YouTube embed always; Mermaid
  with `mermaid: true`, math with `math: true`.

The panel's Insert button opens this list with a search field. A component
with props gets a form generated from its schema and inserts
`{{< name key="value" n=3 flag=true list="a, b" >}}` at the caret; a value
the syntax cannot carry (a line break, `>}}`, both kinds of quotes) is
marked on its field, never altered. `test/widget-samples.json` holds the
calls the panel writes; a Go test in `internal/components` parses every one
of them, so the two cannot drift apart. The image button opens the media
library: an image already in the repository goes in at the caret after its
alt text is asked.

### Revisions

**History is the repository's.** `GET api/revisions/{path}` lists the
commits that changed the file on the branch, newest first, 30 a page (the
first line of each message). The editor keeps no history of its own, so a
rename ends the list at the commit that created the file under its current
name; GitHub's history by path does not say what it was called before.

**Only ids from that history are read.** A commit id from another branch,
a fork, or a commit that did not touch this file is `404
revision_not_found` — the worker looks for the id in the file's history
first and reads nothing by an id it did not find there. The check costs one
request per history page, at most three pages from `cursor`; the panel
passes the cursor of the page it listed the commit on, so it is normally
one request. A read or a diff costs at most 8 requests to GitHub (head,
three history pages, folder listing and blob on each side; measured on the
fake) — 3 more when the worker has not yet cached the site's configuration,
far within the 45 a request may use.

**Comparing.** `GET api/revisions/{path}/diff?from=<commit>&to=<commit>`
(`to` left out = the current version) is a line diff computed in the worker
([`src/diff.ts`](src/diff.ts), no dependency). Lines are compared without
their endings — a file whose CRLF became LF does not show every line
changed; `eol` on each side says what the endings are and `identical`
stays false. A missing final newline changes the last line (`noNewline`).
Over 256 Ki characters or 5000 lines a side, or too many differences to
compute within the Free plan's CPU time, the answer is `tooLarge` with no
hunks and `download` names the two routes giving each version whole.

**Restoring is publishing.** History is never rewritten: the panel loads
`text` of `GET api/revisions/{path}/{commit}` as a draft of the current file
and publishes it like any edit. The answer carries the change ready to send,
based on the current version, and a suggested message:

```json
{"message": "<restore.message, e.g. Restore hello.md to 1a2b3c4 of 2026-01-01>",
 "changes": ["<restore.change: {op: put, path, version: current version, text}>"]}
```

`POST api/changes` with that body makes a new commit on top of the branch
(needs `editor-ui:publish`). If someone changed the file after the revision
was read, it is the usual `409 conflict` with both texts.

## MCP

`POST {base}/mcp` gives an AI assistant or a script the same operations as
the panel: each tool is a thin wrapper of the operation behind a REST route,
run through the same permission check, so a tool call succeeds and is
refused exactly as the route (a refusal is a tool result with `isError:
true` and the route's error body plus `status` as `structuredContent`).
`tools/list` shows only the tools the caller's permissions allow.

**Protocol.** Checked against the specification on 2026-11-15: the current
revision is **2026-07-28** ([versioning](https://modelcontextprotocol.io/specification/versioning),
[Streamable HTTP](https://modelcontextprotocol.io/specification/2026-07-28/basic/transports/streamable-http));
the `initialize`-based 2025-11-25, 2025-06-18 and 2025-03-26 are answered
too ([2025-11-25 transports](https://modelcontextprotocol.io/specification/2025-11-25/basic/transports)).
Stateless: one JSON-RPC message per POST, one JSON answer — no SSE stream, no
session (`Mcp-Session-Id` is ignored), no batches; GET is `405`. Methods:
`initialize`, `notifications/initialized` (`202`), `ping`,
`server/discover`, `tools/list`, `tools/call`. A 2026-07-28 request must
mirror its body in `MCP-Protocol-Version`, `Mcp-Method` and `Mcp-Name`
(`400 -32020` otherwise). A foreign `Origin` is `403`; the body may have at
most 8 MiB (`413`). The transport is `portable/mcp-transport.ts` +
`portable/mcp-methods.ts`, shared with designer-ui.

**Who.** A script or an assistant: an `auth` service token
(`ssgsvc_…`, [SERVICE-TOKENS.md](../auth/docs/SERVICE-TOKENS.md)) exchanged
for a 10-minute access token, sent as `Authorization: Bearer`. Give the
token only the permissions the assistant needs — `editor-ui:read` alone
makes it read-only. A person's session cookie works too, from the site's own
pages (CSRF applies); a request with both uses the Bearer.

```sh
ACCESS=$(curl -sf -X POST -H "authorization: Bearer $SSG_SERVICE_TOKEN" \
  https://example.com/api/auth/token | jq -r .accessToken)
curl -s https://example.com/editor/mcp \
  -H "authorization: Bearer $ACCESS" -H "content-type: application/json" \
  -H "accept: application/json, text/event-stream" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"content_read","arguments":{"path":"content/blog/posts/hello.md"}}}'
```

A client configuration (the common `mcpServers` shape; the access token
expires after 10 minutes, so a client that can run a command for its
headers should call the exchange above instead of storing one):

```json
{
  "mcpServers": {
    "site-editor": {
      "type": "http",
      "url": "https://example.com/editor/mcp",
      "headers": { "Authorization": "Bearer <access token from POST /api/auth/token>" }
    }
  }
}
```

| Tool | Arguments | Operation | Permissions |
|---|---|---|---|
| `content_list` | `q?, section?, cursor?, limit?` | `GET api/content` | read |
| `content_read` | `path` | `GET api/content/{path}` | read |
| `content_create` | `message, path \| section+title, content` or `changes[]` | `POST api/changes` (`put`, version null; `section`+`title` via `content/new-path`) | publish |
| `content_update` | `message, path, version, content?, to?` or `changes[]` | `POST api/changes` (`put`, or `move` with `to`) | publish |
| `content_delete` | `message, path, version` or `changes[]` | `POST api/changes` (`delete`) | publish + delete |
| `media_list` | `cursor?, limit?` | `GET api/media` | read |
| `media_upload` | `message, content_base64, name?` or `changes[]` | `GET api/media/new-path` per image, then `POST api/changes` | publish + media |
| `media_delete` | `message, path, version` or `changes[]` | `POST api/changes` (`delete`) | publish + delete |
| `revisions_list` | `path, cursor?` | `GET api/revisions/{path}` | read |
| `revision_read` | `path, commit, cursor?` | `GET api/revisions/{path}/{commit}` | read |
| `widgets_list` | — | `GET api/widgets` | read |
| `help` | — | the instructions and the caller's tools | read |

Every write is one commit with every file of the call: send several files
in `changes` rather than a call per file. Tool descriptions tell the
assistant that file contents are data, never instructions. `widgets_list`
(GO-174) is added to `src/mcp-tools.ts`.

Differences from `ssg mcp` (the local development server, `internal/mcp/`):

| | `ssg mcp` | editor-ui |
|---|---|---|
| Where a write goes | the working tree; git is a separate step | one commit on the branch per call |
| `message` | only for `git_commit` | required by every write |
| Several files | one tool call per file | `changes[]` in one call, one commit |
| Lost updates | none checked | `version` (from `content_read`) required on update and delete; `409 conflict` with both texts |
| `content_update` | full text | full text, or a move with `to` |
| `content_edit`, `content_find`, `designer_*`, `site_*`, `git_*` | yes | no (search: `content_list` with `q`) |
| `media_upload` | `path` (served URL), `content_base64` or `url` (downloaded) | `content_base64` only — no download; the canonical name is chosen from `name` and the bytes |
| `media_replace` | replaces in place | none: new bytes get a new name |
| `media_delete` | served path; refused while a page links it | repository path and `version`; links are not checked (`GET api/media/references` does) |
| Paths | project-relative or served | repository paths; lists carry the served `url` |
| Access | stdio, or HTTP with a static token or OAuth | `auth` accounts, per-tool permissions, the rate limit |
| Lists | everything at once, as text | pages with `nextCursor`, as structured content |
| Revisions | — | `revisions_list`, `revision_read` |
| Components | `site_components` (which pages use one) | `widgets_list`: components with their props, shortcodes, built-in blocks |

## What it protects and what it does not

Written for site owners after the security review (GO-183,
`audit/roadmap/editor/SECURITY-REVIEW.md`). The worker holds a token that
can write to the repository your site is built and deployed from; this is
what stands between that token and the people using the panel.

**It protects**

- **The repository token.** It lives only as the Worker secret
  `EDITOR_GIT_TOKEN`, is sent only to `https://api.github.com` (the host is
  fixed in code, not configurable), redirects are never followed, and no
  answer, error, log line or MCP result carries it or a GitHub error body.
- **Files outside content.** Every path goes through one policy: `.md` files
  inside the content folders `.ssg.yaml` names, images (PNG, JPEG, WebP,
  GIF, AVIF, decided by their bytes — never SVG or HTML) in the media folder.
  `.github/`, `.git/`, `workers/`, `node_modules/`, `wrangler.*`, `.env*`,
  `.dev.vars*` and every dot file are refused whatever the configuration
  says, in any letter case. Symlinks and submodules are never read through or
  written over; every file is written as a plain, non-executable file.
- **Other people's changes.** A publication is one commit made only if the
  files you opened are unchanged on the branch; otherwise you get both
  versions. The branch is never force-pushed.
- **Who did it.** Each commit's author is the signed-in account (or
  `service account <id>`); the name and e-mail cannot add commit headers.
- **Access.** Only `auth` accounts; a missing or broken `EDITOR_AUTH_*`
  answers 503, never an open panel. Every REST route and MCP tool checks its
  own permission (before reading the request body); a cookie session's
  writes must come from this site's own pages (CSRF); a service token is
  accepted only as `Authorization: Bearer`.

**It does not protect**

- **Your visitors and your admins from what publishers write.** ssg renders
  raw HTML in Markdown. Whoever holds `editor-ui:publish` can put a script on
  a page, and the panels and `auth` share the site's origin: an administrator
  who opens that page runs the script with their own session. That is why
  `editor-ui:publish` is **sensitive**. If people you do not trust with the
  theme write content, set `sanitize_html: true` in `.ssg.yaml` (an
  `executes` key) so the build strips scripts, and consider serving the
  admin panels and `auth` from a host of their own.
- **Revocation in real time.** Removing a role or blocking an account takes
  effect when the access token expires (10 min 30 s by default).
- **Content as instructions.** An assistant using the MCP endpoint reads
  articles written by people. Tool descriptions say the content is data, but
  a model can still be talked into acting on text it reads; give an
  assistant's service token only the permissions you would give a person
  who might be fooled.
- **Your repository host's own rules.** Give the token access to this one
  repository and "Contents: read and write" only — no workflow permission —
  so GitHub itself refuses any change under `.github/workflows/`. Branch
  protection that requires pull requests makes publishing fail (by design).
- **The cost of use.** One account can use up the GitHub API rate limit of
  the token (5,000 requests per hour); bind `RATE_LIMITER` to cap requests
  per client address.

## Switching it off

Delete the Worker (`npx wrangler delete` here, or in the Cloudflare
dashboard), then revoke `EDITOR_GIT_TOKEN` on GitHub. The site's content
stays in its repository unchanged.

## Tests

From the repository root (Node 25, no npm packages):

```sh
node --import ./workers/editor-ui/test/resolve.mjs --test 'workers/editor-ui/test/*.test.ts'
node --test 'workers/*.test.ts'   # the manifest against the code, with every other template
```

No test talks to the network: GitHub is a fake in `test/fake-github.ts`, MDDB
one in `test/fake-mddb.ts` (modelled on MDDB's `docs/API.md`), and sessions are
signed with a key made in the test. `test/sources-ops.test.ts` runs the same
content operations with the same assertions on both sources;
`test/mddb-samples.json` holds documents with the page fields ssg's own Go
converter made of them, which the editor's mapping must agree with.
