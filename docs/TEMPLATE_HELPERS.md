# Template Collection & Conditional Helpers

{% raw %}
*Since v1.8.3.* SSG's Go template engine ships a set of generic helpers for
filtering, sorting, grouping, slicing and testing content — so a theme can build
"recently updated guides", "related posts" or "grouped archives" without any
code changes.

The **collection is always the final argument**, so helpers chain naturally in
Go template pipelines:

```gotemplate
{{ $recentGuides := .Site.Pages
    | where "Type" "guide"
    | sort "Modified" "desc"
    | first 5
}}

{{ range $recentGuides }}
  <article>
    <h2><a href="{{ .URL }}">{{ .Title }}</a></h2>
    <time>{{ formatDate .Modified }}</time>
  </article>
{{ end }}
```

Helpers work on `[]models.Page` **and generically** on slices of structs,
pointers to structs, string-keyed maps, and primitives (via reflection). They
**never mutate their input** and **never panic** — invalid usage stops template
execution with a descriptive error such as:

```text
where: field "ModifiedAt" does not exist on models.Page
sort: unsupported direction "newest"; expected "asc" or "desc"
first: count must be greater than or equal to zero
matches: invalid regular expression "["
filter: unsupported operator "newest"; expected one of eq, ne, gt, ge, lt, le, …
```

## What you do NOT need helpers for

`if`, `else if`, `with`, `range`, `eq/ne/lt/le/gt/ge`, `and/or/not` are **native
Go template features** — use them directly:

```gotemplate
{{ if .Page.HasMath }}math{{ else if eq .Page.Type "guide" }}guide{{ else }}other{{ end }}
```

Native `switch/case` does **not** exist in Go templates and SSG deliberately
does not emulate it — compose `if / else if` with the conditional helpers below.

## Supported comparison types

`where`, `filter`, `sort` and the equality-based helpers compare: **strings**,
**booleans** (`false < true`), **all integer/float kinds** (compared cross-type,
so `int(5)` equals `float64(5)`), **`time.Time`** (and convertible aliases).
Anything else (slices, maps, structs) errors in ordering contexts; equality
falls back to deep comparison.

---

## Collection helpers

### `where` — filter by field equality

```gotemplate
{{ .Site.Pages | where "Type" "guide" }}
{{ .Site.Pages | where "Status" "publish" }}
{{ .Site.Pages | where "HasMath" false }}
```

Signature `where(field, expected, collection)`. Matches struct fields,
pointer-to-struct fields and map keys exactly; preserves input order and element
type. A missing field/key is an error (no silent fallback).

### `filter` — filter with an operator

```gotemplate
{{ .Site.Pages | filter "Modified" "gt" $cutoff }}
{{ .Site.Pages | filter "Tags" "contains" "go" }}
{{ .Site.Pages | filter "Type" "in" (slice "guide" "tutorial") }}
```

Signature `filter(field, operator, expected, collection)`. Operators:
`eq` `ne` `gt` `ge` `lt` `le` `contains` `notContains` `in` `notIn`.
`contains` searches strings (substring) and slices/arrays (element);
`in`/`notIn` test the field value against a provided collection.

### `sort` — stable sort by field

```gotemplate
{{ .Site.Pages | sort "Modified" "desc" }}
{{ .Site.Pages | sort "Title" "asc" }}
```

Signature `sort(field, direction, collection)`; direction is `asc` or `desc`.
Stable, non-mutating (returns a sorted copy). Field must exist on every element
and hold a comparable type.

### `first` / `last` / `limit` / `offset` — pagination

```gotemplate
{{ .Site.Pages | first 5 }}
{{ .Site.Pages | last 3 }}
{{ .Site.Pages | sort "Modified" "desc" | limit 5 }}   {{/* limit = first */}}
{{ .Site.Pages | offset 10 | limit 10 }}               {{/* page 2 of 10 */}}
```

Negative counts error; counts past the end clamp (`offset` past the end yields
an empty collection); inputs are never mutated.

### `groupBy` — group by scalar field

```gotemplate
{{ range $category, $pages := (.Site.Pages | groupBy "Category") }}
  <h2>{{ $category }}</h2>
  {{ range $pages }}…{{ end }}
{{ end }}
```

Returns `map[key] → slice`. Item order inside each group is preserved. Go
templates iterate maps in **sorted key order**, so output is deterministic.
Keys must be scalar (string/bool/number/`time.Time` → RFC 3339); slices, maps
and other structs error.

### `uniq` / `uniqBy` — deduplicate

```gotemplate
{{ .Site.Pages | pluck "Category" | uniq }}   {{/* primitives only */}}
{{ .Site.Pages | uniqBy "Category" }}         {{/* structs, by field */}}
```

First occurrence wins. `uniq` on structs/maps errors — use `uniqBy`.

### `reverse` — reversed copy

```gotemplate
{{ .Site.Pages | reverse }}
```

### `slice` — build a list inline

```gotemplate
{{ slice "guide" "tutorial" "docs" }}
{{ if in .Page.Type (slice "guide" "tutorial") }}…{{ end }}
```

> ⚠️ Registering `slice` **overrides Go's builtin** `slice(value, i, j)`
> sub-slicing function. Bundled themes do not use the builtin; if yours does,
> switch to `printf "%.10s"` for strings or restructure the data.

### `pluck` — extract one field

```gotemplate
{{ $titles := .Site.Pages | pluck "Title" }}
```

Returns a `[]any` of field values (`nil` for nil-pointer elements).

### `indexBy` — build a lookup map

```gotemplate
{{ $bySlug := .Site.Pages | indexBy "Slug" }}
{{ $page := index $bySlug "getting-started" }}
```

Duplicate keys and empty keys are **errors** (no silent overwrites).

---

## Conditional helpers

| Helper | Signature | Example |
|--------|-----------|---------|
| `in` | `in value collection → bool` | `{{ if in .Page.Type (slice "guide" "docs") }}` |
| `notIn` | `notIn value collection → bool` | `{{ if notIn .Page.Type (slice "draft") }}` |
| `contains` | `contains container value → bool` | `{{ if contains .Page.Tags "ssg" }}` — string→substring, slice→element, map→key |
| `startsWith` | `startsWith value prefix → bool` | `{{ if startsWith .Page.Slug "guide-" }}` |
| `endsWith` | `endsWith value suffix → bool` | `{{ if endsWith .Page.SourceFile ".md" }}` |
| `hasPrefix` | Hugo-compatible alias of `startsWith` (v1.8.5) | `{{ if hasPrefix .Page.Slug "guide-" }}` |
| `hasSuffix` | Hugo-compatible alias of `endsWith` (v1.8.5) | `{{ if hasSuffix .Page.SourceFile ".md" }}` |
| `matches` | `matches pattern value → bool` | ``{{ if matches `^guide-` .Page.Slug }}`` — RE2; compiled patterns are cached; invalid patterns error |
| `isNil` | `isNil value → bool` | true for nil interfaces/pointers/maps/slices/funcs/chans; never panics |
| `isEmpty` | `isEmpty value → bool` | Go template truthiness: nil, `""`, `0`, `false`, empty slice/map ⇒ empty. Structs are never empty (zero `time.Time` included — use `.IsZero`) |
| `ternary` | `ternary cond a b → any` | `{{ ternary .Page.HasMath "math" "plain" }}` — for values, not control flow |

`in` takes the **value first, collection second** (the canonical form above).
For pipeline-style membership tests use `filter … "in" …` instead.

---

## Content helpers (wrappers)

| Helper | Equivalent to | Example |
|--------|---------------|---------|
| `latest field n c` | `sort field "desc" \| first n` | `{{ .Site.Posts \| latest "Modified" 5 }}` |
| `published c` | `where "Status" "publish"` | `{{ .Site.Pages \| published }}` |
| `byTag t c` | `filter "Tags" "contains" t` | `{{ .Site.Posts \| byTag "go" }}` |
| `byCategory name c` | *(site-aware)* | `{{ .Site.Posts \| byCategory "guides" }}` — matches frontmatter `Category` or resolved category names/slugs, case-insensitive; `[]models.Page` only |
| `byAuthor a c` | *(site-aware)* | `{{ .Site.Posts \| byAuthor "jan-kowalski" }}` — by ID, name or slug; `[]models.Page` only |
| `related page n c` | *(scored)* | `{{ .Site.Posts \| related .Page 3 }}` — ranks by shared tags (3) > shared categories (2) > same author (1), recency breaks ties, excludes the current page, only positive scores |

---

## Classic utility helpers

SSG registers several helper functions in the Go template engine for date formatting, HTML cleaning, metadata lookup, and logic controls.

### HTML and String Utilities
* **`safeHTML value`** — Returns `template.HTML` to prevent the Go template engine from auto-escaping HTML. Necessary when rendering custom templates or shortcode outputs.
  ```gotemplate
  {{ .Content | safeHTML }}
  ```
* **`decodeHTML value`** — Unescapes standard HTML entity sequences (e.g. `&amp;` becomes `&`).
  ```gotemplate
  {{ decodeHTML .Title }}
  ```
* **`stripHTML value`** — Strips all HTML tags (`<...>` pattern) from the string.
  ```gotemplate
  {{ .Content | stripHTML }}
  ```
* **`stripShortcodes value`** — Strips YouTube and embed WordPress-style bracket shortcodes (`[youtube]...[/youtube]`, `[embed]...[/embed]`) from the text.
  ```gotemplate
  {{ .Content | stripShortcodes }}
  ```

### Date Formatting
* **`formatDate value`** — Formats a date. If a string is passed, it returns it as-is.
  ```gotemplate
  {{ formatDate .Date }}
  ```
* **`formatDatePL date`** — Formats a Go `time.Time` date using Polish month names (e.g., `14 lipca 2026`).
  ```gotemplate
  {{ formatDatePL .Date }}
  ```

### Taxonomy and Metadata Lookup
* **`getCategoryName id`** — Looks up and returns the name of a category by its integer ID from `metadata.json`.
  ```gotemplate
  {{ getCategoryName .Category }}
  ```
* **`getCategorySlug id`** — Looks up and returns the slug of a category by its integer ID from `metadata.json`.
  ```gotemplate
  {{ getCategorySlug .Category }}
  ```
* **`isValidCategory id`** — Returns `true` if the category ID is not `1` (ID `1` is commonly reserved for "Bez kategorii").
  ```gotemplate
  {{ if isValidCategory .Category }}...{{ end }}
  ```
* **`getAuthorName id`** — Looks up and returns the name of an author by their integer ID from `metadata.json`.
  ```gotemplate
  {{ getAuthorName .Author }}
  ```
* **`hasValidCategories page`** — Returns `true` if the page or post has categories assigned other than ID `1`.
  ```gotemplate
  {{ if hasValidCategories . }}...{{ end }}
  ```

### Pages and URLs
* **`getURL page`** — Helper function that returns the calculated URL path for the page or post. Equivalent to calling `.GetURL`.
  ```gotemplate
  {{ getURL . }}
  ```
* **`getCanonical page`** — Helper function that returns the full canonical URL for the page or post. Equivalent to calling `.GetCanonical .Domain`.
  ```gotemplate
  {{ getCanonical . }}
  ```

### Miscellaneous Helpers
* **`thumbnailFromYoutube value`** — Extracts the YouTube video ID from a YouTube WordPress-style shortcode and returns its high-quality video thumbnail URL (`https://img.youtube.com/vi/<id>/hqdefault.jpg`).
  ```gotemplate
  {{ thumbnailFromYoutube .Content }}
  ```
* **`recentPosts n`** — Returns a list of the first `n` posts. Safely clamps the count at both ends to prevent slice panics.
  ```gotemplate
  {{ range recentPosts 5 }}...{{ end }}
  ```
* **`default defaultVal val`** — Returns `defaultVal` if `val` is empty, `nil`, `""`, or `0`. Otherwise returns `val`.
  ```gotemplate
  {{ default "No title" .Title }}
  ```
* **`dict key1 val1 key2 val2 ...`** — Creates a dictionary (map) from key-value arguments. Useful for passing complex parameters into other helpers like `imageResize`.
  ```gotemplate
  {{ $image := imageResize "photo.jpg" (dict "width" 300 "height" 200 "mode" "fill") }}
  ```
  `dict` is also how a partial receives a computed context:
  ```gotemplate
  {{ template "site-head" (dict "Title" .Page.Title "Ctx" .) }}
  ```

### Arithmetic

Go templates have no arithmetic of their own; these four cover the cases a
theme actually needs (column splits, "page N of M", index offsets).

* **`add a b`**, **`sub a b`**, **`mul a b`**, **`div a b`** — Integer operands
  give an integer result and `div` divides integrally (`div 7 2` → `3`); a
  float operand anywhere gives a float (`div 7.0 2` → `3.5`). Dividing by zero
  is a template error rather than infinity, and a non-numeric argument fails
  the render with a message naming the helper.
  ```gotemplate
  {{ $half := div (add (len .Site.Pages) 1) 2 }}
  {{ range $i, $p := .Site.Pages }}{{ if lt $i $half }}…{{ end }}{{ end }}
  ```

---

## Availability

- **Theme templates** (`base/index/post/page/category.html`, layouts, partials):
  every helper above.
- **Shortcode templates**: the safe, deterministic subset — `slice`, `in`,
  `notIn`, `contains`, `startsWith`, `endsWith`, `hasPrefix`, `hasSuffix`,
  `matches`, `isNil`, `isEmpty`, `ternary` — plus the image helpers
  (`imageResize`, `imageSrcSet`, …) and the read-only external-source helpers
  (`getExternal`, `getExternalMeta`). Collection helpers that walk site-wide
  data stay theme-only.
- **Alt engines** (pongo2/mustache/handlebars): not applicable — those engines
  ship their own filter syntax; these helpers are Go-template only.

## Limitations

- No custom predicates/lambdas, no `switch/case`, no JS expressions (by design).
- `sort`/`filter` ordering requires comparable field types (see above).
- `groupBy`/`indexBy`/`uniq` keys must be scalar.
- `uniq` across mixed numeric types dedupes by rendered value (`1` ≡ `uint8(1)`).
{% endraw %}
