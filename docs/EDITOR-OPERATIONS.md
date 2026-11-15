# editor-ui limits, local preview, errors

What one publication of the browser editor may carry on each Cloudflare
plan, how to try it on your machine, what it protects, the error answers you
are likely to meet, and what is not built yet. This page continues the
[editor guide](EDITOR.md); the same limits and rules apply to `designer-ui`
([DESIGNER.md](DESIGNER.md)).

## Limits per Cloudflare plan

Cloudflare's Workers limits, as published on 2026-11-15
([Workers limits](https://developers.cloudflare.com/workers/platform/limits/)):

| | Workers Free | Workers Paid |
|---|---|---|
| CPU time per request | 10 ms | 30 s by default, up to 5 min |
| Subrequests per request | 50 | 10,000 |
| Memory per isolate | 128 MB | 128 MB |
| Requests per day | 100,000 | no daily limit |

The worker's own limits come from a measurement made on 2026-11-14 on the
repository layer's code path (Node 25, GitHub replaced by a stand-in; Workers'
own CPU time was not reported by `wrangler dev`): about 2.5 ms of CPU and
2.4 times the body in memory per MB of image, and `files + 5 + folders`
requests to GitHub per publication, plus 6 for each retry after another
commit won the race (three attempts at most). One article with a 1 MB image
took 9 requests and about 3.3 ms.

| Per publication | Applied today | Measured as safe on Paid |
|---|---|---|
| Files | 10 | 50 |
| Size of one file | 2 MiB | 10 MiB |
| Size of the whole publication | 2 MiB | 20 MiB |

**Both workers apply the Free values on every plan in this version.** The
Paid values are measured and defined in the code (`PLAN_LIMITS`,
`workers/editor-ui/portable/repo-provider.ts`), but no variable switches
them on yet. Over a limit, the answer is `413 too_large` with the limits,
before anything is decoded or sent, so the panel can say what to shrink.
One request may make at most 45 requests to GitHub; the article list loads
titles lazily within that budget and fills the rest on the next request.

Other limits to know:

- **GitHub's rate limit is per token**: 5,000 requests an hour for a
  personal access token, shared by everyone using the worker. Bind the
  optional `RATE_LIMITER` (in `wrangler.toml`, 120 requests a minute per
  client address as shipped) to keep one client from using it up.
- A request body may be at most 8 MiB, on REST and MCP alike.

## Try it on your machine

### The demo, with no account and no network

The template ships a local preview that runs the worker's real code against
an in-process stand-in for GitHub, seeded with a small invented site
(five articles in two languages, an image, two earlier publications by two
people). Nothing leaves the machine. It needs Node.js 25 or later, which runs
the TypeScript sources directly. From the site's root, after
`ssg new worker editor-ui`:

```sh
node --import ./workers/editor-ui/test/resolve.mjs workers/editor-ui/test/dev-server.mjs
```

Open `http://localhost:8789/editor/` and pick an account on the pretend
sign-in page:

| Account | Permissions |
|---|---|
| `owner` | read, publish, media, delete |
| `writer` | read, publish, media |
| `texts` | read, publish (no images) |
| `reader` | read only |

To see a conflict, open an article, change it, then publish a change to the
same article as someone else before you publish yours:

```sh
curl -X POST 'http://localhost:8789/demo/touch?path=content/blog/posts/bread-notes.md'
```

`PORT=…` changes the port. The publications land in the stand-in only and
are gone when you stop the server.

### Against a test repository with `wrangler dev`

To try the real thing before deploying, use a throw-away repository with a
copy of your site and a token for that repository only (setup step 2).
The worker trusts only tokens signed by your `auth`, and the browser session
cookie belongs to your site's host, so locally you call the API with a
service token instead of opening the panel:

1. In the accounts panel of your deployed `auth`, create a service account,
   give it a role with `editor-ui:read`, and create a token for it
   ([SERVICE-TOKENS.md](../workers/auth/docs/SERVICE-TOKENS.md#a-complete-example)).
   To publish too, the role needs `editor-ui:publish`, and the token needs an
   owner's `allowSensitive`.
2. In `workers/editor-ui/wrangler.toml`, replace `<example.com>` in
   `routes` with your host. `wrangler dev` refuses to start while the
   placeholder is there ("Cannot infer host from first route").
3. In `workers/editor-ui`, put the variables in a file named `.dev.vars`.
   Wrangler reads it for `wrangler dev` only, and its values win over the
   `[vars]` of `wrangler.toml`. Add `.dev.vars` and `.wrangler/` to the
   site's `.gitignore`: the file holds the token, and `ssg init` does not
   list either. Set `EDITOR_AUTH_AUDIENCE` to your site's host, because the
   tokens `auth` issues name that host, not `localhost`. The public key is
   the JSON object after `AUTH_PUBLIC_KEY=`, pasted without quotes:

   ```sh
   EDITOR_GIT_REPO=github.com/<owner>/<test-repo>
   EDITOR_GIT_BRANCH=main
   EDITOR_GIT_TOKEN=<the fine-grained token>
   EDITOR_AUTH_ISSUER=https://example.com/api/auth
   EDITOR_AUTH_LOGIN_URL=/auth/login
   EDITOR_AUTH_AUDIENCE=example.com
   EDITOR_AUTH_PUBLIC_KEY={"kty":"EC","crv":"P-256","x":"…","y":"…"}
   ```

4. Start it and call it:

   ```sh
   npx wrangler dev        # http://localhost:8787
   ACCESS=$(curl -sf -X POST -H "authorization: Bearer $SSG_SERVICE_TOKEN" \
     https://example.com/api/auth/token | jq -r .accessToken)
   curl -s -H "authorization: Bearer $ACCESS" http://localhost:8787/editor/api/health
   curl -s -H "authorization: Bearer $ACCESS" http://localhost:8787/editor/api/content
   ```

`api/health` says what is configured without asking GitHub (`"ok": true`,
`"auth": "auth"`, the repository and branch); `api/me` shows the
permissions the token carries; `api/content` lists the articles of the test
repository. Without a token every route answers `401` with
`"reason": "no_session"`, which shows the worker is up. More calls: [EDITOR-API.md](EDITOR-API.md).

## What it protects and what it does not

Read this before you hand out permissions. The security review wrote it for
site owners, in the worker's README:
[editor-ui: what it protects and what it does not](../workers/editor-ui/README.md#what-it-protects-and-what-it-does-not).
In short: the repository token never leaves the worker except to
`api.github.com`; only Markdown in the content folders and images in the
media folder can be written, never `.github/`, `workers/`, `wrangler.*` or
dot files; nothing overwrites someone else's change. It does not protect your
visitors and administrators from what a publisher writes (raw HTML is
rendered), it does not revoke access in real time, and it cannot stop an AI
assistant from obeying instructions hidden in an article it reads.

## Troubleshooting

| Answer | Cause | Fix |
|---|---|---|
| `503 auth_not_configured` on every path | an `EDITOR_AUTH_*` value missing, or both or neither of public key and JWKS URL | set the issuer, the login URL and exactly one of the two |
| `503 not_configured`, "the auth key or issuer is unusable" | `EDITOR_AUTH_PUBLIC_KEY` is still the placeholder or not the JSON object `auth-keys.mjs` printed | paste the value after `AUTH_PUBLIC_KEY=` |
| `401` with `"reason": "wrong_audience"` | the token names another host than the request (e.g. `wrangler dev` on `localhost`) | set `EDITOR_AUTH_AUDIENCE` to the site's host |
| `503 repo_not_configured` | `EDITOR_GIT_REPO` or the `EDITOR_GIT_TOKEN` secret missing or malformed | `detail` names the variable (never its value) |
| `503 site_config_unreadable` | the configuration file uses YAML the small reader does not follow (anchors, `<<`, `\|` blocks, `include:`, TOML) | `detail` has the file and line; set `EDITOR_CONTENT_DIRS` and `EDITOR_MEDIA_DIR` |
| `503 media_not_configured` | `EDITOR_MEDIA_DIR` is not a folder ssg publishes | a folder inside `static_dir`, or one ending in `/media` |
| `502 token_rejected` | the token expired, was revoked, or lacks Contents write on this repository | make a new token (step 2) and `wrangler secret put` it again |
| `409 branch_protected` | the branch requires pull requests or status checks | publish to a branch without those rules |
| `409 conflict` / `409 branch_busy` | someone published in between / the branch kept moving | combine and publish again / publish again |
| `429 rate_limited` | GitHub's limit for the token, or `RATE_LIMITER` | wait; `retryAt` says until when |
| `403 cross_origin` | a write with the session cookie from another site | publish from the panel on the site's own host |
| `403` naming a permission | the account's roles lack it | grant it; it applies within 10 min 30 s |

## Not built yet

- Pull requests and preview deployments: a publication is a commit on the
  configured branch.
- Repository hosts other than GitHub.
- The Paid-plan limits above (no switch yet).
- A visual (WYSIWYG) editor or live editing by two people at once; drafts
  shared between devices.
- Video and files other than images; SVG images.
- OAuth sign-in for remote MCP clients: an MCP client needs an access token
  from a service token, renewed every 10 minutes.
- Removing a front matter key from the form (edit "Frontmatter as text"
  instead); `.markdown` files; the time of the last change in the list.
- Editing `.ssg.yaml`, workflows or secrets: never, by design.
