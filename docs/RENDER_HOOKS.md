# Render hooks

```yaml
render_hooks:
  image: hooks/image.html
  link: hooks/link.html
```

A hook is a template that decides the markup for one kind of Markdown node. It
runs at the syntax tree, where the node still knows what it is, rather than over
the string it became.

## Why this exists

Everything this build does to rendered content, it does with regular
expressions over finished HTML: rewrite an image path, swap a `.jpg` for a
`.webp`, relativise a link. That works, and it can only ever change what is
already there.

The gap it cannot close is the one that matters most. **An image written in
Markdown comes out bare.** `![](photo.jpg)` becomes an `<img>` with a src and
an alt: no `srcset`, no width, no height, no `loading="lazy"`. The responsive
pipeline exists and is reachable only from templates, so a site migrated from
WordPress has hundreds of content images that skip it — which is exactly the
Largest Contentful Paint problem those sites have.

And there was no policy for external links at all. No `rel="noopener"`, which
is a security property and not a nicety.

## The hooks

| Hook | Node | Context |
|---|---|---|
| `image` | `![alt](src "title")` | `.Src` `.Alt` `.Title` `.IsExternal` |
| `link` | `[text](href "title")` | `.Href` `.Text` `.Title` `.IsExternal` |
| `heading` | `## Heading` | `.Level` `.ID` `.Text` |
| `code` | a fenced block | `.Lang` `.Code` `.Info` `.Rendered` |
| `table` | a table | `.Inner` |
| `blockquote` | `> quoted` | `.Inner` |

`.Text` and `.Inner` are **already rendered**, so `[**bold** link](/x)` reaches
a link hook as markup rather than as flattened text. `.Alt` is plain text,
because it is an attribute value.

`.ID` on a heading is the anchor the build already computed — the same one the
table of contents uses, so a hook cannot disagree with it.

`.Rendered` on a code block is the block as this build would otherwise have
written it, **syntax highlighting included**. A code hook wraps that: adds a
copy button, a filename, a language label. Re-implementing a highlighter in a
template is not what anyone wants from this.

## Worked examples

Responsive, lazy, captioned images for everything in the content:

```gotemplate
{{/* hooks/image.html */}}
<figure class="content-image">
  <img src="{{ .Src }}" alt="{{ .Alt }}" loading="lazy" decoding="async">
  {{- with .Title }}<figcaption>{{ . }}</figcaption>{{ end }}
</figure>
```

An external-link policy, in four lines:

```gotemplate
{{/* hooks/link.html */}}
{{- if .IsExternal -}}
  <a href="{{ .Href }}" rel="noopener noreferrer nofollow" target="_blank">{{ .Text }}</a>
{{- else -}}
  <a href="{{ .Href }}"{{ with .Title }} title="{{ . }}"{{ end }}>{{ .Text }}</a>
{{- end -}}
```

A table that scrolls on a phone instead of stretching the page:

```gotemplate
{{/* hooks/table.html */}}
<div class="table-scroll">{{ .Inner }}</div>
```

## What counts as external

A URL on another origin. A root-relative path, a fragment, a `mailto:` or a
`tel:` never is, and neither is an absolute URL on the site's own `domain` — so
content that links home the long way is still linking home. A site with no
`domain` configured treats every absolute URL as external, which is the safe
reading.

## Rules worth knowing

**Without hooks, nothing changes.** No renderer is registered and the output is
goldmark's own, byte for byte. The golden corpora check it.

**A lone image is unwrapped.** Markdown treats an image as inline, so
`![](photo.jpg)` on its own line would be `<p><figure>…</figure></p>` — markup
no browser agrees about. When an image hook is configured, a paragraph that
holds nothing but one image is unwrapped. A paragraph with prose around the
image keeps it.

**Hooks do not nest.** The content a hook receives in `.Text`, `.Inner` or
`.Rendered` is rendered by goldmark, not by other hooks. That is the price of
not recursing forever, and it means an image inside a hooked blockquote is a
plain `<img>`.

**A hook that fails is reported.** It writes nothing and the build says which
hook and why. Falling back to goldmark's markup would hide a broken template
behind output that looks almost right, which is the failure mode this feature
exists to replace.

**A hook the site names and cannot have is an error at load time** — a missing
file, a template that does not parse, a hook name that is not one of the six.

## Where this is going

The regular expressions this replaces are still there, and are meant to be
retired one at a time, each with its own golden test: image paths and the WebP
swap first, since an image hook already owns that markup. Nothing is removed in
this release.
