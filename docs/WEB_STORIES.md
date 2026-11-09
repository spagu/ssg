# Web Stories

ssg can turn posts into [Google Web Stories](https://developers.google.com/search/docs/appearance/enable-web-stories):
full-screen, tap-through AMP pages that Google shows in Search, Discover and
Images. Each story is an invitation to the article, not a copy of it: a cover
with the featured image and the title, a few full-bleed photographs from the
post with short captions, and a last page that links to the full article.

Available since 1.8.72. Off by default.

## Configuration

```yaml
web_stories:
  enabled: true
  publisher_logo: images/logo-square.png   # required: square, at least 96×96
  # sections: [posts]                      # posts (default), pages, or a content directory
  # url: /web-stories/                     # the hub; stories live under it
  # pages: 4                               # photo pages between the cover and the last page
  # min_images: 2                          # images a post needs, featured included
  # cta: Read the full article             # the last page's link text
  # narration: true                        # record each page through tts: (1.9.4)
```

`web_stories: true` is shorthand for `enabled: true`, but a story cannot be
made without `publisher_logo`, so you will normally write the mapping.

| Key | Default | Meaning |
|-----|---------|---------|
| `enabled` | `false` | Turn the feature on. |
| `publisher_logo` | — | A square image of at least 96×96 px, as a path the image pipeline finds: `assets/`, the static directory, the content source directory, the theme directory. AMP rejects a story without one, so without it the build prints a warning and makes **no** stories. |
| `sections` | `[posts]` | `posts`, `pages`, or a content directory such as `posts/recipes` (that directory and everything below it). |
| `url` | `/web-stories/` | Where the hub lives; each story is at `<url><slug>/`. |
| `pages` | `4` | The most photo pages a story has between the cover and the last page. |
| `min_images` | `2` | How many distinct images (the featured one included) a post needs to get a story. |
| `cta` | `Read the full article` | The last page's link text. Overrides the i18n catalog. |
| `narration` | `false` | Record what each page says through the site's [`tts:`](AUDIO.md) service. The page moves on when its recording ends (see [Narration](#narration)). |

## What is generated

For each selected post that has a featured image:

| Output | Path |
|--------|------|
| The story (a standalone AMP document) | `<url><slug>/index.html` |
| The hub, one per language that has stories | `<url>` (with the language prefix for other languages) |
| Derived images | `processed_images/` (the same cache as `imageResize`) |

A story has:

1. **Cover** — the featured image and the title.
2. **Photo pages** — the post's own pictures (Markdown images and `<img>`
   elements, in order, each once; a WordPress `-300x200` thumbnail is replaced
   by its original when the original exists), each with a caption. When the
   post has more captions than pictures, the pictures — the featured image
   among them — are used again in turn. Never more than `pages`.
3. **Last page** — the title, and an `amp-story-page-outlink` to the article.

Captions are hooks taken from what the post already declares, in this order:

- its first three tags (`chocolate · cake · baking`);
- for a post whose structured data is a `Recipe`: `totalTime` as
  "Ready in 1 hr 25 min" (else `cookTime` as "Cooks in 45 min") and
  `recipeYield` as "Makes 16 servings";
- its excerpt (or description), as plain text cut at 140 characters.

The method — the post's body text — is never used.

Every story supports landscape (`supports-landscape`, 1.9.4): on a desktop
screen it fills the window, not a phone-shaped column.

### From story settings

When [`story:`](STORY.md) also selects the post (1.9.4), the AMP story takes
from the post's story settings what AMP, which allows no script, can show:

| From | In the Web Story |
|------|------------------|
| `slideshow.header.logo` (`text`, `logo_height`) | the logo over every page |
| `widgets` with a `stat`, `title` or `text` | up to two per photo page, flying in. A `recipe` widget is left out: the ingredients panel shows it instead |
| `slideshow.footer` (`label`, `items` of `link`, `text`, `image`) | the promotions rotating at the bottom of every page, by CSS, and a page listing them before the last |
| `audio` | today's track as the story's background music (the same date windows) |
| the post's `Recipe` structured data | `recipeIngredient` as an "Ingredients" panel the reader pulls up. Never the method |

A post without `story:` gets none of these, so existing stories do not change.

### Narration

With `narration: true` and a working [`tts:`](AUDIO.md), each page's text is
recorded at build time:

- the cover reads the title;
- a photo page reads its caption;
- the last page reads its link text.

The recordings use the same [pronunciation rules](STORY.md#pronunciation) as
story mode's narrator. They are written as `<url><slug>/narration/<page>.mp3`
and played as the page's `background-audio`. Each page moves on 0.6 s after its
recording ends.

Recordings are cached under `.ssg-cache/story-narration/` by text and voice, so
a rebuild does not call the service again. If a recording fails, the build
reports that page, which keeps AMP's default timing. Without `tts:` the build
says so once and makes the stories silent.

AMP plays page audio and story audio at full volume over each other, so a
narrated story leaves today's music out.

### Images

| Image | Size | Use |
|-------|------|-----|
| Poster | 640×853 (3:4) | `poster-portrait-src`, the hub |
| Page background | 720×1280 (9:16) | every page |
| Publisher logo | 192×192 | `publisher-logo-src` |

Every image is cut by ssg's own pipeline: cropped to fill from the centre,
Lanczos-resampled, enlarged when the source is smaller. WebP when the site
publishes WebP (`webp: true` or `image_formats` with `webp`), JPEG otherwise;
if `cwebp` is missing the build says so and falls back to JPEG. Derived files
are cached in `.ssg-cache/images/` like every other derived image.

Images are found in the same places as `imageResize` looks: a reference such
as `/media/cake.jpg`, a path beside the post, or a URL on the site's own
domain. A featured image on another host cannot be cut, and its post is
skipped.

### Skipped posts

Posts without a story are reported once per build, grouped by reason:

```
   ⚠️  web stories: 3 skipped, no featured image: Notes, Links, Hello
   ⚠️  web stories: 1 skipped, too few images: Quick Tip
```

Reasons: no featured image; featured image is not a file of this site; too
few images; a page already renders at the story's address; an image could not
be processed.

### Where stories appear — and do not

- **Sitemap**: every story (only when `domain` is set, since a sitemap needs
  absolute URLs) and every hub.
- **Hub**: an ordinary page rendered through the theme's `web-stories` layout
  when it has one, `page.html` otherwise. Its `.Content` is a list of posters
  (`<ul class="ssg-web-stories">`); `.web_stories` is the list as data —
  `.Title`, `.URL`, `.PostURL`, `.Poster.Path`, `.Poster.Width`,
  `.Poster.Height`. It carries `hide_from_lists: true`, so themes keep it out of
  menus and card grids. A page of your own at the hub address wins.
- **Not** in post listings, feeds or the search index: stories are not posts.

## Linking a post to its story

Put a slot in the post template:

```html
<span data-ssg-story-slot></span>
```

ssg fills it with `<a class="ssg-story-link" href="/web-stories/<slug>/">Watch as a Web Story</a>`
when the post has a story, and removes it otherwise. Text inside the slot is
used as the link text: `<span data-ssg-story-slot>Zobacz relację</span>`.
The slot needs no template function, so a theme that has it still builds with
older ssg releases (the empty span is simply left in place).

## Translating

The phrases come from the i18n dictionaries (`translations_dir`, see [I18N.md](I18N.md)) under `web_stories`, with English as the
fallback:

```yaml
# <translations_dir>/pl.yaml
web_stories:
  ready_in: "Gotowe w {{time}}"
  cooks_in: "Pieczenie: {{time}}"
  makes: "Wychodzi {{yield}}"
  servings: "{{count}} porcji"
  watch: "Zobacz relację"
  cta: "Przeczytaj cały przepis"
  hub_title: "Relacje"
  hub_description: "Relacje z {{site}}"
  ingredients: "Składniki"         # the recipe panel (1.9.4)
  recommended: "Polecamy"          # the footer's page when it has no label (1.9.4)
```

Durations use the units of the `duration` template helper for the story's
language.

## Overriding the story template

A theme may define its own `story.html`:

```html
{{ define "story.html" }}<!doctype html>
<html amp lang="{{ .Lang }}">…{{ end }}
```

Its context (`StoryContext`):

| Field | Meaning |
|-------|---------|
| `.Lang`, `.Title`, `.Description` | The story's language, title and meta description. |
| `.Canonical` | The story's own URL (absolute when `domain` is set). |
| `.Publisher`, `.PublisherLogo` | The site name and the 192×192 logo. |
| `.Poster` | The 640×853 poster. |
| `.Cover` | The first page. |
| `.Pages` | The photo pages. |
| `.CTA`, `.CTALabel`, `.CTAURL` | The last page, its link text, the article's URL. |
| `.Post`, `.Site` | The post and the site, as in other templates. |
| `.Header`, `.Footer`, `.Recipe`, `.Music`, `.ExtraCSS`, `.RecommendedLabel` | From the post's `story:` settings (1.9.4); empty without them. `.Header` has `.URL`, `.Height`, `.Alt`; `.Footer` has `.Label` and `.Items` (`.URL`, `.Image`, `.Text`); `.Recipe` has `.Label` and `.Ingredients`. |

An image has `.URL` (as the story writes it), `.Path` (rooted on the site),
`.Width` and `.Height`; a page has `.ID`, `.Image`, `.Alt`, `.Caption`, and
since 1.9.4 `.Widgets` (`.Type`, `.Stat`, `.Title`, `.Text`, `.Delay`),
`.Audio` and `.Advance` (the narration and its `auto-advance-after`).

A story is written as rendered: none of the head, analytics, feed-link, listen,
component, math, diagram or WebMCP injections is added, because AMP allows no
script but its own runtime and no style but `<style amp-custom>`. The same rule
applies to any page a theme renders with `<html amp>` or `<html ⚡>`. HTML
minification and prettifying still apply.

## Validating

The built-in template passes the AMP validator. After changing a theme's
`story.html`, check a story:

```bash
npx amphtml-validator public/web-stories/<slug>/index.html
```

or paste the URL into the [AMP test](https://search.google.com/test/amp) and
the [Web Stories test](https://search.google.com/test/rich-results) once it is
published. ssg's own test suite runs the validator when it is available:

```bash
SSG_AMP_VALIDATOR="npx --yes amphtml-validator" go test ./internal/generator/ -run AMPValidator
```

## Limitations

- Stories are written as `<slug>/index.html` whatever `page_format` says.
- The story URL uses the post's slug; two posts with the same slug in one
  language get one story (the second is reported as skipped).
- A post needs a featured image that is a local file.
- `relative_links` is not applied inside stories: AMP wants absolute URLs.
- With `incremental`, the hub is always rewritten; stories are rebuilt every
  build from the image cache.
