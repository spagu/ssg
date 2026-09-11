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

## Try it on this repository

```bash
make site-edit
```

serves this project's own documentation with the editor on. Click a heading,
change it, save: the edit lands on a branch of its own, never on the one you
have checked out.

## What this is for

The person who needs it is usually not the person who set the site up: a client
who wants the phone number in the footer corrected, an editor fixing a headline,
anyone for whom "open the repository, edit the frontmatter, commit" is three
steps too many. They get a form; you get a branch to review.

## Frontmatter

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

## Body text

A theme marks the region that holds the page's body:

```html
<div class="post-body" data-ssg-edit="body">{{ .Post.Content | safeHTML }}</div>
```

Inside it, a click on a paragraph, a heading, a list item or a quotation opens
that **block**. `ssgtheme` and `simple` mark theirs already.

**The editor shows the Markdown source, not the rendered HTML.** A paragraph
with a link and a bold run reaches you as
`The first paragraph, with a [link](/x/) and some **bold**.` That is honest:
editing a render means a lossy round trip back to Markdown, and this edits what
is in the file.

### How a click finds its block

This is the part that had to be got right, because its failure mode is silent.

The HTML on screen went through Markdown rendering, shortcode expansion, SEO
injection, link rewriting, sanitisation and minification, and none of that has
an inverse. "Take the text of the element and search the file for it" is the
shape of every editor that eventually changes the wrong paragraph.

So the search is not for the text. The source is split into the blocks Markdown
is made of; each block's plain text is compared with the clicked text after
normalising what the build is known to change — collapsed whitespace, smart
quotes, dashes, ellipses, non-breaking spaces; and the edit proceeds only when
**exactly one** block matches:

```
this text appears 2 times in the file, so an edit here could change the wrong
one — edit it in the file, or make the passages different
```

```
this text is not a block of the source — it may be generated by the theme
rather than written in the file, or changed by the build on the way to the page
```

The save then goes through `content_edit`, whose contract is the same rule one
level down: the anchor must appear exactly once, or nothing is written. Two
independent checks of one property, because the thing they prevent is a green
save that changed the wrong text.

A fenced code block is one block and is offered as it is. Editing code through
a text box in a side panel is worse than opening the file, so nothing clever
happens there.

## AI actions

When the site has a model configured (`ai:` in the config), the panel offers
four buttons over it:

| Button | Asks for |
|---|---|
| **Shorten** | this text in at most 160 characters — the same budget `--check-meta` measures against |
| **Suggest a title** | one title of at most 60 characters, specific to the text rather than a category |
| **Alt text** | alternative text for an image, for a reader using a screen reader |
| **Translate** | the same block in another language, Markdown intact |

Two rules make them safe enough to have.

**An action proposes; it never saves.** The answer lands in the field you are
editing, and the save is still the one you make, through the same endpoint and
the same validation. A model that misreads an instruction produces a suggestion
somebody rejects, not a change somebody finds next month.

**The key never reaches the browser.** The dev server holds it exactly as the
build does, calls the model itself, and hands back text. The panel has no
credential and no way to talk to a provider. Answers are cached the way the
build's `[ai …]` answers are.

Without a model configured the buttons are not shown, and the endpoint says so
rather than failing quietly.

## What is not here

- **Rich-text editing.** Editing renders back to Markdown lossily; this edits
  the source, not the render.
- **Template editing.** Use MCP, or an editor.
- **Several people at once.** One checkout, one branch, and git for the rest.
- **Remote hosting.** This is a local development server.

See also [MCP.md](MCP.md) for the server this is a client of, and
[TEMPLATES.md](TEMPLATES.md) for the attribute convention.

[The paragraph you clicked is not in the file](../blog/the-paragraph-you-clicked-is-not-in-the-file.md)
is the reasoning behind the block-matching rule: why the obvious approach fails,
what the two refusals mean, and why the AI actions propose rather than save.
