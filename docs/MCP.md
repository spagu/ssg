# MCP server — SSG for AI agents

`ssg mcp` is a Model Context Protocol server over stdio. An MCP-capable
assistant connects to it and edits a real site: templates, theme assets and
Markdown content, with the site rebuilding after every change so a broken
template comes straight back as an error rather than shipping.

It is a **development** server. It edits the working tree in front of it and, if
git is configured, can put those edits on a branch and open a pull request — but
nothing merges without a human.

## Register it

```json
{ "command": "ssg", "args": ["mcp"] }
```

Run from the site's root, or point at the config:

```bash
ssg mcp --config=.ssg.yaml
```

| Flag | Meaning |
|---|---|
| `--role=designer` | Expose the designer tools only |
| `--role=content` | Expose the content tools only |
| `--no-watch` | Do not rebuild after each change |
| `--watch` | Also watch the filesystem, so edits made outside MCP rebuild too |
| `--config=FILE` | Site config (default `.ssg.yaml`) |

Both roles are exposed by default. Narrow with `--role` when an agent should not
be able to touch the other half.

## Call `help` first

The server has a `help` tool that describes the roles, the git workflow and
every tool with its purpose. Every other tool's description states its own CAN
and CANNOT. An agent that reads those before acting will not need this page.

## The two roles

The split is deliberate: **how the site looks** and **what the site says** are
different jobs with different blast radii, and an agent asked to fix a typo has
no business rewriting a layout.

### Designer — templates and theme assets

| Tool | Purpose |
|---|---|
| `designer_list` | Every template and theme asset that may be edited |
| `designer_read` | Read one before changing it |
| `designer_write` | Create or replace one, full content (not a patch) |
| `designer_edit` | Change one piece in place: exact `old` → `new`, matched once |
| `designer_find` | Where something lives — file and line range, without reading |
| `designer_config_read` | The settings that may be changed — site name and description, theme, rendering — with current values |
| `designer_config_set` | Change one of them |

Writes are confined to the template and asset directories. The designer cannot
edit content, delete files, or write outside those directories.

`designer_config_set` is deliberately narrow — an allow-list of presentation
keys such as the theme, the Mermaid theme, the highlight style and minification.
**Secrets, deployment, server, endpoints, hooks and URL structure are neither
readable nor writable.** Call `designer_config_read` first: it returns the key
names, the current values and what each one does.

### Content manager — Markdown

| Tool | Purpose |
|---|---|
| `content_list` | Every Markdown file |
| `content_read` | Frontmatter and body of one file |
| `content_create` | A new file — full document, including frontmatter |
| `content_update` | Replace an existing file; fails if it does not exist |
| `content_delete` | Remove a file |
| `content_edit` | Change one passage in place: exact `old` → `new`, matched once |
| `content_find` | Which files mention something, and where |

`content_create` and `content_update` are separate on purpose: creating over an
existing file and updating a missing one are both mistakes, and splitting them
turns each into an error instead of silent data loss. `content_delete` is
destructive and should follow an explicit request, never an inference.

## Find, then edit — the cheap path

The tool list above has two shapes for changing a file, and the difference is
measured in tokens rather than taste.

A full write (`designer_write`, `content_update`) costs the size of the **file**.
Changing one CSS value on a real migrated site meant reading 4 812 bytes,
writing 4 812 back and reading them again to check: about 4 300 tokens of file
traffic to move one line — and that is before finding the right file at all.

An anchored edit costs the size of the **change**:

```jsonc
designer_edit {
  "path": "static/css/style.css",
  "old":  "  background: #fff;",
  "new":  "  background: #0b1220;"
}
```

- `old` is matched **byte for byte**, indentation included, and must appear
  **exactly once**. Zero matches or several is a refusal naming the count, so
  nothing fuzzy ever lands and nothing is half-applied.
- The reply carries the change with its neighbours and line numbers. That **is**
  the verification — there is no re-read.
- On a line too long to print — a minified stylesheet is one line — the window
  is measured in **characters** around the change instead, and reported as
  `line:fromCol-toCol`. Whatever is left out is stated rather than silently
  dropped. Line-based context is right for source that has lines; the reply
  should not grow to the file because the file has one.
- An empty `new` deletes the anchored text.

`designer_find` / `content_find` supply the anchor without reading anything:

```jsonc
designer_find { "query": "background" }
→ static/css/style.css:4-8
    body {
      background: #ffffff;
      color: var(--ink);
    }
```

The query is treated as a case-insensitive regular expression when it is valid
syntax and literally when it is not, so pasting a CSS fragment with an
unbalanced bracket returns an answer rather than a syntax error. Matches whose
context windows overlap are reported as one region; files over 512 KB are
skipped, because a minified bundle matches everything and helps nobody.

A match on a very long line comes back as a character window with a column
range — `static/css/site.css:1:9834-9859` — so a minified file gives a locus an
edit can anchor to rather than the whole line, which in that file is the whole
thing.

So the whole flow is **find → edit**, and the 10k-token background change
becomes a few hundred. Reserve the full writes for new files and real rewrites.

### An MDDB-backed search (optional)

A local scan matches text. It cannot answer a question phrased as a sentence —
"where is the page background set?" — because that is a search problem. Point
the find tools at an MDDB collection and it becomes answerable:

```yaml
mcp:
  search:
    mddb_url: http://localhost:11023
    mddb_collection: theme
    mddb_api_key: $MDDB_TOKEN   # optional; always $ENV, never a literal
    mddb_lang: en               # query tokenisation
    mddb_fuzzy: 1               # typo tolerance: 0 off, 1 or 2 edits
    mddb_allow_http: true       # only if the URL is http:// on a private network
```

The API key travels in `X-API-Key`, which is the header MDDB validates keys
from. Its bearer path is for JWTs — a key sent that way is parsed as a token and
refused with `401 invalid token`, which blames the key rather than the header. If
`mddb_api_key` holds a JWT instead, it is sent as `Authorization: Bearer` and
both work.

An API key is refused over plaintext `http://` to anything but loopback. On a
private container network — `http://mddb:11023`, never leaving the host — that is
the same trust boundary as loopback spelled with a service name, so
`mddb_allow_http: true` says so deliberately. Leave it off for anything routable.

Fill the collection with `ssg mddb push-theme`:

```bash
ssg mddb push-theme            # upsert every template and asset, prune what vanished
ssg mddb push-theme --dry      # show what it would do
ssg mddb push-theme --lang=pl
```

Each file becomes one document keyed by its **project-relative path**, with
`kind` (style / script / template / data / asset), `size` and a SHA-256
`checksum` in its metadata — so a search hit names the file to open, which is
what makes it actionable rather than merely relevant. The sync reconciles:
documents whose file no longer exists are deleted, and running it twice changes
nothing the second time.

When the collection carries embeddings, the query is answered by **two** calls,
and the reason is worth knowing before you tune anything:

- `/v1/hybrid-search` decides **which** documents matter. That is what the
  vectors are for, and it is what lets "how the navigation looks on phones" find
  a template that never uses either word.
- `/v1/fts` supplies the **line ranges**. Hybrid results carry none — no
  highlights, no positions — and a hit without a locus is a hit an anchored
  `designer_edit` cannot use.

A document the vectors ranked but the keywords did not match is still reported,
with its line marked unknown rather than guessed. A collection without
embeddings declines the first call once, is remembered, and behaves exactly as
it did before.

The index is consulted **first and never required**. When it errors or finds
nothing, the local scan still runs and answers. A search backend that is down
degrades the answer; it does not take the ability to edit the site down with it.

## Watch mode is the feedback loop

Unless `--no-watch` is passed, the site rebuilds after every mutation and build
errors are returned to the calling tool. A template that fails to parse, a link
that breaks, a validator that trips — the agent sees it immediately and can fix
it before doing anything else.

This is the part worth relying on: an agent editing a static site otherwise has
no way to tell whether its change was valid until someone looks at the output.

## One process for preview, MCP and watch

```bash
ssg mcp --http --watch --listen=7823
```

That one command serves the site preview, serves the MCP endpoint, and rebuilds
on **both** kinds of change: a mutation arriving through MCP, and a file edited
by anything else — a human editor open beside the agent, an `rsync`, a CMS export
refreshing content. Live reload fires either way.

The distinction between the two watch flags is worth stating once:

| Flag | What it governs |
|---|---|
| *(default on)* | Rebuild after every **MCP mutation**. Turn it off with `--no-watch` |
| `--watch` | Also poll the **filesystem** — content, templates, data, `content_sources`, and the config file |

Before 1.8.47 `--watch` was accepted here and read by nothing, so an edit that
did not arrive through MCP left the preview stale indefinitely. The workaround —
`--watch-runner="ssg mcp --listen=…"` — is one command line but still two
processes, and worse: two independent builders over one output directory with
nothing serialising them, which is a preview that intermittently serves half a
site.

**Every rebuild goes through one lock**, whatever triggered it. That also closes
a race that needed no watcher at all: the Streamable HTTP transport gives each
request its own goroutine, so two concurrent `tools/call` were already able to
rebuild the same output tree at the same time.

An edited config file is picked up here as it is under `ssg --watch`: the file is
reloaded, `endpoints:` are republished onto the running preview without dropping
the port, and the site rebuilds from the new settings. One boundary is fixed at
startup and worth knowing: the **MCP server's own surface** — which roles are
exposed, which directories its tools may write to, whether the git PR flow is
armed — is read once when the process starts. Moving `content_dir` in a live
config changes what is built, not what the assistant is allowed to edit; that
needs a restart.

## Git write-back

Optional. Without it the assistant edits files in place. With it, the changes
travel a branch → commit → PR path with a human at the end:

| Tool | Purpose |
|---|---|
| `git_status` | Branch and changed files, so both sides see what will be committed |
| `git_new_branch` | Create and switch to a working branch |
| `git_commit` | Stage and commit on that branch |
| `git_open_pr` | Push and open a pull request against the default branch |

```yaml
mcp:
  git:
    account: my-org
    token: $GITHUB_TOKEN      # an environment variable, never a literal
    repo: my-org/my-site      # optional; derived from the remote when empty
    remote: origin            # default
    default_branch: main      # PR base
    branch_prefix: mcp/       # working-branch prefix
```

The token **must** reference an environment variable. A literal in a config file
is a credential in version control.

`git_commit` never touches the base branch, and `git_open_pr` opens a pull
request — it does not merge. The approval step stays with a person.

## What this is not

It does not deploy, does not read or write secrets, and does not run arbitrary
commands. The tools it exposes are the ones needed to change a site's appearance
and its words, and nothing beyond that.

If an agent needs to build or deploy, that is the CLI's job
([DEPLOYMENT.md](DEPLOYMENT.md)) and belongs in CI, not in an editing session.

## For an agent evaluating SSG

The properties that matter when a machine is doing the editing:

- **Errors come back to the caller.** Rebuild-on-change means an invalid edit is
  reported at the point it was made, not discovered later.
- **The output is deterministic.** Two builds of the same input produce
  byte-identical output regardless of worker count, which is verified in CI —
  so a diff means the change caused it.
- **Every tool states its limits.** The CAN/CANNOT contract is in each tool's
  description, so the boundary does not have to be inferred from failures.
- **Structured data is first class.** `schema:` in frontmatter is arbitrary
  JSON-LD, so any schema.org type — `Recipe`, `Product`, `Event` — is expressible
  without SSG knowing about it ([CONFIGURATION.md](CONFIGURATION.md)).
- **Nothing is published without a person.** The write-back path ends at a pull
  request.

## See also

- [CONFIGURATION.md](CONFIGURATION.md) — the `mcp:` block and every other setting
- [TEMPLATES.md](TEMPLATES.md) — what the designer role is editing
- [CONTENT.md](CONTENT.md) — frontmatter and content structure
