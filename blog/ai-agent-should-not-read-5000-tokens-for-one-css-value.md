---
title: "Your AI Agent Shouldn't Read 5,000 Tokens to Change One CSS Value"
slug: "ai-agent-should-not-read-5000-tokens-for-one-css-value"
status: publish
type: post
date: 2026-08-21
tags: [mcp, ai, agents, context, development]
excerpt: "Giving an AI agent access to your project is only half the problem. If changing one CSS value requires reading and rewriting an entire file, the agent is spending most of its context learning things it never needed to know."
mermaid: true
mermaid_theme: neutral
mermaid_background: "#ffffff"
---

A few weeks ago I gave `ssg mcp` access to the parts of a site it needed to
work on: templates, CSS and content. It could make a change and rebuild the
result. The workflow worked, but used far more context than the edit required.

For a page-background change, the only relevant information might be one line:

```css
background: #ffffff;
```

The agent did not know where that line was. It had to list files, open one or
two likely candidates, read the whole stylesheet, send the whole file back with
one value changed and often read it again to verify the edit.

On one migrated site, a one-line CSS change consumed roughly **10,000 tokens**,
mostly while locating that line. The model was not the bottleneck. The tools
made it inspect the room before letting it turn off the light.

## File access is not context access

The first MCP tools in SSG followed a fairly obvious filesystem model:

```text
list
read
write
```

That is a reasonable filesystem API, but a poor fit for many agent tasks.

If I ask a developer sitting beside me to change the page background, they do
not start by reading every stylesheet from line 1. They search for
`background`, look at a few surrounding lines, make the edit and move on.

An agent should be able to do the same thing, so SSG now has `designer_find` and
`content_find`.

```jsonc
designer_find { "query": "background" }
```

returns something closer to:

```text
static/css/style.css:4-8

body {
  background: #ffffff;
  color: var(--ink);
}
```

Those five lines are the context the task required. The navigation CSS, print
stylesheet and 300 lines of responsive rules below it add nothing to the edit.

## Finding the line was only half the problem

Search removes the expensive read, but the original write operation still
replaced the whole file.

That means changing this:

```css
background: #ffffff;
```

to this:

```css
background: #0b1220;
```

could still involve sending several kilobytes back through the model.

So the other half is an anchored edit:

```jsonc
designer_edit {
  "path": "static/css/style.css",
  "old": "  background: #ffffff;",
  "new": "  background: #0b1220;"
}
```

`old` has to appear exactly once. SSG refuses the edit if it appears zero times
or more than once.

There is no fuzzy "this is probably what you meant" step between the model and
the file.

That makes the normal path:

```mermaid
flowchart LR
    A["Find 'background'"] --> B["5 relevant lines"]
    B --> C["Anchored edit"]
    C --> D["Changed lines returned"]
    D --> E["Rebuild"]
```

The changed lines come back in the edit response, with their neighbours and line
numbers. That response is also the verification. There is no reason to read the
file again just to discover that the line now contains the value we just wrote.

## Context is a resource

Token use is usually discussed as a pricing problem. For coding agents, context
is also attention.

Every unrelated template block, CSS rule and paragraph you put into the context
window competes with the thing the agent is supposed to be working on. A larger
window lets you fit more irrelevant material in it; it does not make the
irrelevant material useful.

The difference is easy to miss because both workflows eventually produce the
same one-line diff.

One does this:

```text
list files
read stylesheet
read another stylesheet
write entire stylesheet
read stylesheet again
```

The other does this:

```text
find
edit
```

The second workflow succeeds because the tool matches the task.

## Where literal search falls short

Humans do not always know what text to search for.

You might ask:

> Where is the styling for the box around code examples?

and there may be no word `box`, `code example` or anything else from that
sentence in the stylesheet.

A local text search cannot bridge that gap.

For projects using MDDB, SSG can therefore put an MDDB full-text search behind
the same find tools. `ssg mddb push-theme` indexes the templates and assets,
including their paths, kind, size and checksum.

The index is an accelerator, not a dependency.

The find tool asks the index first. If the index is unavailable or gives no
useful answer, it falls back to the local scan.

A search outage therefore cannot prevent an agent from editing a local file.

## Why not just give the agent grep?

You can, and you can also give it a shell.

For a general coding agent working inside a disposable checkout, that may be
exactly the right answer.

`ssg mcp` has a narrower job. The designer is allowed to work on presentation.
The content role is allowed to work on Markdown. Those boundaries existed
before find/edit, and the new tools obey the same boundaries.

`designer_find` cannot quietly search the content tree.

`content_edit` cannot modify a template.

A more efficient tool must preserve the permissions of the slower one it
replaced.

That constraint matters more as agents become better. The more capable the
model, the less I want its safety model to depend on it remembering which
directories I mentioned three prompts ago.

## Small tools, smaller context

Improving an AI development tool does not always require a better prompt, a
larger context window or another reviewing agent. Often the useful change is in
the interface: do not send 4,812 bytes when the model needs 28; do not read a
file merely to locate a line; do not read it again when the edit response can
show exactly what changed.

The first version of `ssg mcp` gave the assistant access to the project. The
find and edit tools make that access economical. For many edits, five relevant
lines beat a larger context window.
