# Markdown for agents & AI search

SSG is Markdown-native, so it can hand language models and AI crawlers the
*authored Markdown* of every page instead of making them parse rendered HTML.
This guide covers the flags that do it and how they map to Google and ChatGPT.

## `markdown_publish`

```yaml
markdown_publish: true
```

With this on, every page is published a second time as clean Markdown:

- **`/<page>/index.md`** — next to `index.html`, so `https://site/guide/index.md`
  serves the Markdown.
- **`/<page>.md`** — the flat sibling, for agents that append `.md` to a clean
  URL (`https://site/guide.md`).
- A **`<head>` discovery link** on every page:
  `<link rel="alternate" type="text/markdown" href="index.md">`.
- A root **`llms.txt`** index listing every page and pointing at its Markdown
  copy (the [llms.txt](https://llmstxt.org) convention).

The published copy is the Markdown you wrote — an H1 title followed by the body
— not an HTML→Markdown round-trip, so nothing is lost or re-guessed. Listing
pages (the home page, archives) carry no source Markdown and are skipped.

## `webmcp`

```yaml
webmcp: true
```

`markdown_publish` hands an agent the site's *text*. WebMCP hands it the site's
*tools*: the built pages declare callable functions through the browser's
[`navigator.modelContext`](https://github.com/webmachinelearning/webmcp) API, so
an agent that opens the site can ask it questions instead of scraping the DOM.

**This is not `ssg mcp`.** The two are easy to confuse and share nothing but a
name:

| | `ssg mcp` | `webmcp` |
|---|---|---|
| Runs | on the author's machine | in the visitor's browser |
| Serves | the author's assistant | the visitor's agent |
| Can | write files, run git | read what the site already publishes |
| Lives | while you are writing | on the deployed site, forever |

### The tools

Four, registered on every page the build writes — posts, pages, archives,
taxonomy listings, the home page and `404.html`:

| Tool | Answers |
|---|---|
| `searchPosts(query, limit)` | Matching titles, URLs and excerpts, ranked |
| `listByTag(tag)` | Every document carrying that tag |
| `getDocument(url)` | One document's title, excerpt, tags, language and text |
| `navigate(url)` | Opens one of **this site's own** documents |

`navigate` refuses any URL not in the site's own index. An agent can move a
reader around the site it is already on and nowhere else.

### What it costs a visitor without an agent

Nothing measurable. The script is a few kilobytes inline, and it does two
things: check whether `navigator.modelContext` exists, and stop if it does not.
The search index is fetched on the **first tool call**, never at page load — a
reader who has no agent never requests it.

### It brings its own data

The tools answer from `search-index.json`, so `webmcp: true` turns
`search_index` on. Four tools that all throw on first call is worse than no
tools, and that is what shipping the script alone would produce. With `i18n`
enabled each language gets its own index and each page reads the one in its own
language.

### Draft status, and what follows from it

WebMCP is a **W3C Community Group Draft**, not a Recommendation. Chrome
implements `navigator.modelContext`; other engines are engaged in the spec work
without shipped support. So:

- The feature is **off unless you ask for it**, and a site that does not ask is
  unchanged byte for byte.
- The script **feature-detects and exits** where the API is absent, which is
  most visits today. It registers nothing, fetches nothing and touches no DOM.
- A theme that ships its **own** registration keeps it — SSG detects one and
  leaves the document alone rather than registering a second set.

If the API changes shape, the switch is what keeps that from becoming your
problem: turn it off and the site is what it was.

## `components.json`

A site with typed content components publishes its contract at the root:

```json
{
  "schema": 1,
  "components": [
    { "name": "youtube",
      "description": "Embed a video from a privacy-friendly host.",
      "example": "{{< youtube id=\"…\" >}}",
      "props": [
        { "name": "id", "type": "string", "required": true, "description": "The video id." },
        { "name": "ratio", "type": "string", "default": "16x9", "enum": ["16x9", "4x3"] }
      ] }
  ]
}
```

This is the difference between an agent that can write content for a site and
one that can only write prose. Without it, a model generating a page either
avoids the site's own components or invents attributes for them, and nobody
notices until a reader does. With it, the contract is explicit — the types, what
is required, what values an enum allows — and a call that gets it wrong is
caught by the build rather than by a proofreader.

The `example` is the shortest call that would validate: something to copy, not a
grammar to infer. Read it before generating a call; see
[COMPONENTS.md](COMPONENTS.md) for the whole feature.

## `site_graph`

```yaml
site_graph: true
```

`markdown_publish` hands an agent the site's *text*. `site_graph` hands it the
site's *shape*: one JSON document describing every page, section, taxonomy,
link and redirect the build produced, stamped with the build that produced it.

```json
{
  "schema": 1,
  "build": { "version": "1.8.60", "time": "2026-09-10T12:00:00Z", "hash": "3f9c…" },
  "domain": "example.com",
  "pages":      [ { "url": "/blog/hello/", "type": "post", "title": "Hello", "canonical": "https://example.com/blog/hello/",
                    "components": ["youtube"], "relations": [{ "name": "see_also", "url": "/about/" }],
                    "tags": ["go"], "categories": ["News"], "translations": [{ "lang": "pl", "url": "/pl/blog/czesc/" }],
                    "outputs": { "html": "/blog/hello/", "markdown": "https://example.com/blog/hello/index.md" } } ],
  "sections":   [ { "path": "/tag/go/", "kind": "tag", "title": "go" } ],
  "taxonomies": [ { "name": "tag", "path": "tag", "terms": [ { "name": "go", "slug": "go", "url": "/tag/go/", "count": 4 } ] } ],
  "links":      [ { "from": "/blog/hello/", "to": "/about/", "kind": "page" } ],
  "redirects":  [ { "from": "/old", "to": "/blog/hello/", "status": 301 } ]
}
```

Why it exists: an agent working on a site otherwise learns what the site
contains by listing directories and reading files, re-deriving what the build
already knew. The build computes every one of these facts — the link checker
alone parses every output page and extracts every reference, then discards the
result once it has validated it. The graph keeps them.

**One model, several views.** `routes.json` and `llms.txt` are now generated
*from* the same in-memory graph, whether or not `site_graph` is on — so the
manifests cannot drift from each other, and their bytes did not change when
the source of truth moved (the golden corpora are the proof). `search-index.json`
stays a text index; its metadata agrees with the graph, its text is its own.

**What is in it, and what is not.** Everything in the artifact is something the
published HTML already reveals. What a page was rendered *from* — its source
file, its template — is the project's structure rather than the site's content,
and is not published here (it still appears in `routes.json`, which has always
carried it).

**`build`** is how a reader tells fresh from stale. `hash` covers the content,
not the clock: the same site built twice hashes equal, a changed site does not.

**Scale.** Above 10,000 pages the file becomes an index and the page and link
records move to JSON Lines under `site-graph/`, so a consumer can stream one
shard rather than load fifty thousand records to find one.

### Asking the graph over MCP

`ssg mcp` gains a **site** section when it knows the output directory — five
read-only tools that answer from the last build's graph and name the `build`
they answer from:

| Tool | Answers |
|---|---|
| `site_pages` | every page, filterable by `type` and `lang`, paged with `limit`/`offset` |
| `site_page` | one page in full, with every link out of it and into it |
| `site_links` | the link graph, filterable by `from`, `to` and `kind` (page, asset, external) |
| `site_taxonomies` | every taxonomy with its terms, archive URLs and counts |
| `site_redirects` | every rule the host will apply |
| `site_components` | which pages use which component — ask before changing one |

"Which pages link to `/pricing/`?" is one call rather than a grep across the
output. The tools are read-only by design: the graph is a model of the site,
not a CMS, and mutation stays with the file-shaped tools that know how.

## `clean_special_chars`

```yaml
clean_special_chars: true
```

AI tools routinely emit "smart" Unicode — curly quotes, en/em dashes, ellipsis
characters, non-breaking and zero-width spaces. This normalises them to plain
ASCII across **all** rendered content (HTML, the published Markdown, feeds and
the search index).

It targets a fixed Western-punctuation allowlist only. **Chinese, Japanese and
Korean text — and CJK's own full-width punctuation (、。（）) — pass through
untouched**, as does every other script. Off by default, because many themes use
this typography deliberately; enable it where the content is known to carry AI
artefacts.

## `output_encoding`

```yaml
output_encoding: utf-8            # utf-8 (default) | utf-16le | utf-16be
output_encoding_sections:        # optional per-section overrides
  legacy: utf-16le
```

Selects the encoding of the text output (HTML pages, published Markdown,
`llms.txt`). UTF-16 output carries a byte-order mark and the HTML `<meta charset>`
is kept in step. Overrides are keyed by content section using the same
longest-prefix rule as `schema_defaults` (`home` is the site root).

Every option is Unicode, so **Chinese/Japanese/Korean and all other scripts
round-trip losslessly** in UTF-8 and UTF-16 alike. Sitemaps, feeds and JSON stay
UTF-8 — their formats standardise on it or carry their own encoding declaration.

## `robots_rules`

```yaml
robots_rules:
  - { user_agent: "*",            allow: ["/"] }
  - { user_agent: GPTBot,         allow: ["/"] }
  - { user_agent: OAI-SearchBot,  allow: ["/"] }
  - { user_agent: Google-Extended, allow: ["/"] }
```

Replaces the default permissive `robots.txt` (`User-agent: * / Allow: /`) with
explicit per-crawler directives, so you can state your policy for AI and search
crawlers. The `Sitemap:` line is always appended. Empty keeps the allow-all
default — SSG never blocks a crawler unless you ask it to.

## Compatibility: Google vs ChatGPT

The two treat this differently, and the guidance is not the same:

| | Google Search (AI Overviews / AI Mode) | ChatGPT Search & other LLMs |
|---|---|---|
| Reads `llms.txt` / Markdown alternates | **No** — ignored; reads standard HTML | **Yes** — consumes Markdown directly |
| What earns eligibility | Standard indexing + snippet eligibility | Crawlable content it can fetch and read |
| Structured data | Helpful for rich results, **not** an AI lever | Helps disambiguation |

**For Google**, there is no special "AI SEO": ship solid, crawlable, standard
HTML. SSG already covers that surface — `seo` (OpenGraph/Twitter/JSON-LD),
`schema`/`schema_defaults`, canonical tags, `sitemap`, `check_meta`,
`check_images`, `check_orphans`, `hreflang`/i18n, `lastmod_from_git`. `llms.txt`
and Markdown alternates do **not** affect Google ranking (it ignores them).

**For ChatGPT Search and other Markdown-reading agents**, `markdown_publish`
(with `clean_special_chars`) is exactly what helps: they get the clean authored
Markdown and the `llms.txt` index. Keep `robots_rules` from blocking `GPTBot` /
`OAI-SearchBot` (the default allow-all already doesn't).

In short: **standard SEO flags for Google, `markdown_publish` for the LLMs, and
`robots_rules` to state the policy for both.**
