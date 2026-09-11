# Typed content components

```
{{< youtube id="6hLDZ6HL0rw" ratio="16x9" >}}
```

A component is a directory with a contract, and a call that carries its
arguments. Content asks for a thing, with the details that make it *this*
thing, and the build checks the ask before the page ships.

## Why this exists

A shortcode is an entry in the site config: a fixed name that renders fixed
data. `{{gallery}}` renders the one gallery the config describes, and a second
gallery means a second config entry. There is no way to say "this one, with
these pictures, three across" from inside the content that wants it.

That is the gap. Components close it, and the schema is what makes them worth
having: a component becomes something a person can use without reading its
template, something the build can check, and something an agent can generate a
correct call for.

## The shape

```
components/
  youtube/
    component.yaml    # the props: their types, which are required, defaults
    template.html     # the markup, rendered by html/template
    assets/           # css and js, copied and linked only where it is used
      youtube.css
```

`component.yaml`:

```yaml
description: Embed a video from a privacy-friendly host.
props:
  id:
    type: string
    required: true
    description: The video id, from the share URL.
  title:
    type: string
    default: YouTube video
  ratio:
    type: string
    enum: [16x9, 4x3]
    default: 16x9
  autoplay:
    type: bool
    default: false
```

`template.html` — the props are already typed, and the site is in scope:

```gotemplate
<figure class="ssg-youtube ssg-youtube--{{ .Props.ratio }}">
  <iframe src="https://www.youtube-nocookie.com/embed/{{ .Props.id }}{{ if .Props.autoplay }}?autoplay=1{{ end }}"
          title="{{ .Props.title }}" loading="lazy" allowfullscreen></iframe>
</figure>
```

Set `components_dir` to keep them somewhere else; the default is `components/`
beside the project. A site without that directory builds exactly as it always
did.

A worked example ships with ssg in
[examples/components/youtube](https://github.com/spagu/ssg/tree/main/examples/components/youtube):
a declared schema, a template that reserves its aspect ratio so the page does
not shift as the video loads, and a stylesheet that travels with it.

## Prop types

| `type` | Written as | Arrives as |
|---|---|---|
| `string` (default) | `title="A talk"` | a string |
| `number` | `columns=3`, `ratio=1.5` | an int, or a float when it has a point |
| `bool` | `autoplay=true` | a bool |
| `array` | `tags="go, ssg"` | a list of trimmed strings |

`required: true` refuses a call that omits it. `default:` fills one that does.
`enum: [a, b]` restricts a string to a list, and refuses anything else by name.
A component with no `component.yaml` takes any argument as a string, because it
has promised nothing.

## What a mistake looks like

```
⚠️  component: youtube: "id" is required — in {{< youtube title="no id here" >}}
⚠️  component: callout: "kind" = "sideways" is not one of: note, warning — in {{< callout kind="sideways" >}}
⚠️  component: no component named "gallry" (have: callout, youtube) — in {{< gallry src="x" >}}
```

Every message names the call, which is a unique string to search the content
for. `shortcode_errors` decides what happens next — one setting for "my content
is wrong", not two:

| `shortcode_errors` | A call that does not resolve |
|---|---|
| `keep` (default) | stays in the page, visible, so an author proofreading sees it |
| `drop` | is removed from the page |
| `strict` | fails the build |

None of them invents markup.

## The syntax, and what it does not touch

`{{< name … >}}` is deliberately not the syntax the legacy shortcodes use.
`{{name}}` and `[name]` keep working exactly as they did, and neither can be
mistaken for a component call: a page that happens to contain `{{gallery}}`
renders as it always has.

One rule matters more than the grammar. **A call this build cannot resolve is
left in the page as written.** The legacy `{{name}}` form deletes what it does
not recognise — reasonable for a shortcode a config once defined, and wrong for
prose that merely looks like a call. Documentation that quotes a component call
stays quoted.

A call lives on one line and ends at the first `>}}`. A prop may contain a `>`;
it may not contain the terminator itself.

## Assets

A component's `assets/` are copied into `/components/<name>/` and linked **only
on the pages that used it**. A site with twenty components and one in use ships
one component's CSS, because the alternative is a stylesheet that grows with
the library rather than with the page.

For that to work a component's markup has to start with an element — the build
marks it, reads which components a finished page contains, and erases the mark
before the page ships. A component that renders bare text gets no assets.

## `components.json`

Every build with components publishes the contract at the site root:

```json
{
  "schema": 1,
  "components": [
    {
      "name": "youtube",
      "description": "Embed a video from a privacy-friendly host.",
      "example": "{{< youtube id=\"…\" >}}",
      "props": [
        { "name": "id", "type": "string", "required": true, "description": "The video id, from the share URL." },
        { "name": "ratio", "type": "string", "default": "16x9", "enum": ["16x9", "4x3"] }
      ]
    }
  ]
}
```

`site-graph.json` carries the other half: each page lists the components it
renders, and `ssg mcp` answers `site_components` with the inverse — which pages
use a given component. That is the question to ask before changing one.

This is what an agent reads before writing a call, and what makes generated
content something the build can check rather than something a human has to
proofread for invented attributes. The `example` is the shortest call that
would validate — something to copy, rather than a grammar to infer. See
[AI-AGENTS.md](AI-AGENTS.md).

## What is not here

- **`Page` in the template context.** A component sees `Props` and `Site`.
  Content renders through one function map built for the whole build, and the
  page a call came from is not in scope there; the honest alternatives were a
  per-page template clone or a goroutine-local current page. A component that
  needs the page is a theme partial, which has it.
- **A paired form** (`{{< x >}}…{{< /x >}}`). Self-closing in this version.
- **Nested calls.** A component's output is not scanned for more calls.
- **Remote components.** A component is a directory in your project.
- **Config shortcodes going away.** `shortcodes:` in `.ssg.yaml` works exactly
  as before, and is not deprecated by this.
