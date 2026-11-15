# editor-ui API and MCP for scripts

Everything the [editor panel](EDITOR.md) does goes through an API that a
script or an AI assistant can use too: REST routes under `/editor/api/` and
an MCP endpoint at `/editor/mcp`, both behind an `auth` token. Both call
the same operations with the same permission checks, so an assistant can do
exactly what its token allows and is refused exactly where a person would be.
The full contract is [`openapi.yaml`](../workers/editor-ui/openapi.yaml); the
reference with every field is the
[README's API section](../workers/editor-ui/README.md#api).
`designer-ui` has the same shape for settings and theme files
([DESIGNER.md](DESIGNER.md#scripts-and-ai-assistants)).

## Signing in as a machine

A script uses an `auth` **service token** (`ssgsvc_…`), never a person's
password. Create a service account and a token in the accounts panel
(**Service accounts**, needs `auth:admin`) or through the admin API
([SERVICE-TOKENS.md](../workers/auth/docs/SERVICE-TOKENS.md)). Give its role
only what the job needs:

- `editor-ui:read` alone makes it read-only.
- Writing needs `editor-ui:publish`, which is sensitive: the token carries it
  only when an owner ticked `allowSensitive` when creating it. Add
  `editor-ui:media` for images and `editor-ui:delete` for deletions.

The service token is exchanged for an **access token valid for 10 minutes**,
sent as `Authorization: Bearer`. Exchange again when it expires:

```sh
ACCESS=$(curl -sf -X POST -H "authorization: Bearer $SSG_SERVICE_TOKEN" \
  https://example.com/api/auth/token | jq -r .accessToken)
```

A request with a Bearer token needs no CSRF evidence. A request made with a
person's session cookie must come from the site's own pages.

## REST in four calls

```sh
H="authorization: Bearer $ACCESS"
# 1. the articles (50 a page by default, limit up to 100; nextCursor for the next page)
curl -s -H "$H" 'https://example.com/editor/api/content?q=bread'
# 2. one article, with its version; the path is URL-encoded as one segment
curl -s -H "$H" https://example.com/editor/api/content/content%2Fblog%2Fposts%2Fbread-notes.md
# 3. publish: one commit with every file in "changes"
curl -s -H "$H" -H 'content-type: application/json' \
  https://example.com/editor/api/changes -d '{
  "message": "Bread notes: add the starter",
  "changes": [
    {"op": "put", "path": "content/blog/posts/bread-notes.md",
     "version": "<version from step 2>", "fields": {"title": "Notes on bread and time"}}
  ]}'
# 4. the article's history
curl -s -H "$H" https://example.com/editor/api/revisions/content%2Fblog%2Fposts%2Fbread-notes.md
```

A change is `put` (whole `text`, or a form edit with `fields`,
`frontmatterRaw` and `body`), `move` (`from`, `to`) or `delete`. `version`
is the version you read; `null` means "this file must not exist yet". An image
is `{"op": "put", "path": …, "version": null, "base64": …}` in the same
`changes`; `GET api/media/new-path?name=&type=&hash=` gives the canonical path
it must have.

Success is `201 {commitId, url, branch, files}`. If the article changed
since you read it, nothing is written and the answer carries both texts:

```json
{"error": "conflict", "detail": "…",
 "files": [{"path": "content/blog/posts/bread-notes.md",
            "yours": "…", "theirs": "…", "theirVersion": "…"}]}
```

Publish again over `theirVersion` once you have combined them. Every error is
`{"error": "<snake_case>", "detail"?}`; the codes are in
[EDITOR-OPERATIONS.md](EDITOR-OPERATIONS.md#troubleshooting).

## MCP

`POST https://example.com/editor/mcp` speaks MCP over HTTP: the current
revision 2026-07-28 and the `initialize`-based 2025-11-25, 2025-06-18 and
2025-03-26. It is stateless: one JSON-RPC message per POST, one JSON answer,
no session. `tools/list` shows only the tools the token's permissions allow.

| Tool | What it does | Needs |
|---|---|---|
| `content_list`, `content_read` | list and read articles | read |
| `content_create`, `content_update` | new article; edit or rename (`to`) | publish |
| `content_delete` | delete an article | publish + delete |
| `media_list` | the media library | read |
| `media_upload` | add an image from `content_base64` | publish + media |
| `media_delete` | delete an image | publish + delete |
| `revisions_list`, `revision_read` | an article's history, an old version | read |
| `widgets_list` | components, shortcodes and blocks an article can hold | read |
| `help` | instructions and the caller's tools | read |

Every write takes a `message` and is one commit; several files go in one call
as `changes[]`, not one call per file. An update or delete needs the
`version` from `content_read`, and a conflict is the same `409` as on REST,
returned as a tool result with `isError: true`.

```sh
curl -s https://example.com/editor/mcp \
  -H "authorization: Bearer $ACCESS" -H 'content-type: application/json' \
  -H 'accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"content_list","arguments":{"q":"bread"}}}'
```

A client configuration in the common `mcpServers` shape:

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

The access token in such a file stops working after 10 minutes. A client that
can run a command to build its headers should run the exchange above each
time; otherwise paste a fresh token when the server starts refusing. OAuth
sign-in for MCP clients is not built in this version.

## Next to `ssg mcp`

`ssg mcp` ([MCP.md](MCP.md)) is the local development server: it works on
your checkout, writes files and leaves the commit to you. The editor's
endpoint works on the repository through GitHub and makes one commit per
call. The tool names and arguments are the same where the operation is the
same, so one prompt works in both places; the differences (versions,
`changes[]`, no `content_edit`, no download by URL in `media_upload`) are in
[one table in the README](../workers/editor-ui/README.md#mcp).

**Content is data, not instructions.** The tool descriptions tell the
assistant so, but a model can still be talked into acting on text it reads in
an article. Give an assistant's token only the permissions you would give a
person who might be fooled, and prefer `editor-ui:read` until you need more.
