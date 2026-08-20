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
| `designer_config_read` | The presentation settings that may be changed, with current values |
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

`content_create` and `content_update` are separate on purpose: creating over an
existing file and updating a missing one are both mistakes, and splitting them
turns each into an error instead of silent data loss. `content_delete` is
destructive and should follow an explicit request, never an inference.

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
