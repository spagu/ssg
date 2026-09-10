# Editing in the browser (`ssg serve --edit`)

```
ssg --http --watch --edit
```

Open the preview, click a marked region of a page, change the field, save. The
change is written to the Markdown file it came from, validated, rebuilt, pushed
back to the open tab by live reload, and committed to a git branch of its own.

**The source of truth stays the Markdown in your repository.** There is no
database, no draft store, no state anywhere but the files and git. Nothing here
can be edited that could not be edited in a text editor, and nothing is saved
that a `git diff` will not show you.

## What this is for

The person who needs it is usually not the person who set the site up: a client
who wants the phone number in the footer corrected, an editor fixing a headline,
anyone for whom "open the repository, edit the frontmatter, commit" is three
steps too many. They get a form; you get a branch to review.

## Phase one: frontmatter

This release edits **frontmatter** — the fields at the top of a document: title,
date, tags, description, whatever the type declares. Editing the body text is
phase two.

The form is built from `content_schemas` (see
[CONFIGURATION.md](CONFIGURATION.md)). A declared type chooses the control —
`enum` becomes a menu of exactly its values, `date` a date picker, `bool` a
true/false menu, `list` a comma-separated box — and a required field is marked
and refused when empty. A site with no schemas still gets a form: every key the
document has, as a text box.

A value the schema forbids is refused with the rule that refused it, and the
file is not touched:

```
status must be one of: publish, draft
2026-13-45 is not a date (write it as 2026-09-10)
```

## What a theme marks

A page is editable where its theme says so, with an attribute:

```html
<h1 data-ssg-edit="frontmatter:title">{{ .Post.Title }}</h1>
<time data-ssg-edit="frontmatter:date" datetime="…">…</time>
```

Everything else on the page is inert. That is a deliberate limit, not an
unfinished one: most of the text on a rendered page did not come from the body
of a file — a heading built from `title:`, a date run through a formatter, the
output of a shortcode, a label the template composed. Clicking such text and
searching for it in the Markdown either finds nothing or, worse, finds it
somewhere else and changes the wrong thing.

`ssgtheme`, `simple` and `krowy` carry the attributes already. A theme without
them still gets the **All fields** button, which opens the whole form.

**The attributes never reach a published page.** A build without `--edit`
strips them, byte for byte — the golden corpora are the proof — so a theme can
carry them without a production site shipping the scaffolding of a tool it is
not running.

## Where a save goes

The first save of a session creates a branch (`edit/<timestamp>`), and every
save after it commits there:

```
edit: hello.md: title = Hello, edited
edit: hello.md: tags = go, ssg, editing
```

**Never the checked-out branch.** An edit made by clicking is still a change to
the repository, and it should be as easy to review or throw away as any other.
With a GitHub token configured (`mcp.git.token`), the branch can be turned into
a pull request through the MCP git tools; without one, local branches and
commits still work, because asking for a forge token before a local commit
would leave most sessions with no git safety at all.

A project that is not a git repository is told so and the file is still saved.

## The security rules

This writes files and runs git from a web page, so it is treated as what it is.

- `--edit` needs `--http` **and** `--watch`, and refuses to start without them.
- Every request to `/__edit/*` carries the session token in the
  `X-SSG-Edit-Token` **header**. A header, not a cookie: a page on another
  origin cannot add one, so a stray browser tab cannot drive this.
- The token is minted at start-up and printed once. `SSG_EDIT_TOKEN` sets your
  own, which is what a scripted session should do.
- On a non-loopback address without a token, the server **does not start**.
- Editing runs in the MCP **content** role. Templates and theme assets are not
  editable from a browser; that is work for a person or an agent in the
  repository. Every write goes through the same MCP tools an assistant uses, so
  the path confinement and the refusals are decided in one place.

## What is not here yet

- **Body text** (phase two). Editing a paragraph means finding it in the source,
  and the rule for that is the one `content_edit` already applies: the passage
  must appear exactly once, or the edit is refused with the count. A wrong save
  is worse than a refused one.
- **Rich-text editing.** Editing renders back to Markdown lossily; this edits
  the source, not the render.
- **Template editing.** Use MCP, or an editor.
- **Several people at once.** One checkout, one branch, and git for the rest.
- **Remote hosting.** This is a local development server.

See also [MCP.md](MCP.md) for the server this is a client of, and
[TEMPLATES.md](TEMPLATES.md) for the attribute convention.
