# Story mode

`story:` (1.9.4) turns an article into a reading companion and a fullscreen
presentation. Its slides are the same ones the post's
[Web Story](WEB_STORIES.md) uses.

- **Reading bar:** the time left to read, at the top or the bottom of the page.
- **Photo stage:** beside the text, it shows the photograph for the paragraph
  being read. On a narrow screen it becomes a thumbnail in the corner.
- **Widgets:** tips, numbers and quotes appear while a given passage is read.
  The recipe card ticks off ingredients and counts steps.
- **Presentation:** fullscreen slides with a title card and a closing card, a
  header (logo) and a footer (promotions or an ad). It has autoplay, gestures
  like phone stories (tap left or right, hold to pause, swipe down to close)
  and five transitions.
- **Narrator:** the browser's voice reads the article and highlights the
  paragraph it is reading, with subtitles in the presentation. The reader picks
  the voice and the speed. Pronunciation rules turn "250g" into "250 grams".
- **Music:** a track chosen by date window, for example carols in December. It
  gets quieter while the narrator speaks.
- **Share:** the system share sheet on phones, a menu elsewhere. A link to the
  Web Story. `#story=3` deep links open the presentation at a slide.
- **Analytics and media keys:** the presentation sends events to `dataLayer`,
  and Media Session puts play, pause and next on the lock screen.

Everything runs in the page's own scripts, written once to
`/assets/ssg-story/`. Nothing is loaded from a third party.

## Turning it on

```yaml
story: true            # every post, default settings
```

```yaml
story:
  sections: [posts/recipes]       # posts (default), pages, or a content directory
  narrator: { rate: 1, duck: 0.3 }
  slideshow:
    header: { logo: /media/logo.png, text: My Site, link: / }
```

A mapping turns story on unless it says `enabled: false`.

## Where a page's settings come from

Each layer is laid over the one before it:

1. the site's `story:`;
2. every `story.rules` entry whose `when` matches the page, in order;
3. the file `<name>.story.yaml` beside the post's source (with or without a
   top-level `story:` key);
4. the post's own `story:` front matter.

Mappings merge key by key. `slides`, `widgets`, `figure_skip` and `hide` add up
across layers, so a post can add one slide without repeating the site's. Any
other list replaces the one before it. `story: false` in front matter keeps a
post out.

```yaml
story:
  rules:
    - when: { category: recipes }          # also: tags, section, contains
      set: { slideshow: { interval: 6s } }
    - when: { contains: "Ingredients" }
      set: { widgets: [{ type: recipe }] }
```

Keys may be written `snake_case` (ssg's style) or `camelCase`; the page script
receives camelCase. With `webp: true` (or `image_formats`), a path may name the
`.webp` the build will write. `check_links` accepts it when the source `.jpg`
or `.png` is there, because the link check runs before the conversion. The keys inside `pronounce`, `labels` and `theme` are data
and are passed as written. Paths such as `/media/logo.png` are site paths.

## Options

| Key | What it does |
|---|---|
| `content`, `column` | CSS selectors for the article text (default `article`) and the column that moves aside for the stage |
| `trigger` | the reading line, as a fraction of the viewport (default `0.45`) |
| `safe_top` | space kept free under a sticky site header (`96px`) |
| `transition`, `transition_time` | `crossfade`, `slide`, `zoom`, `blur` or `kenburns`, and its length |
| `figures`, `figure_skip`, `figure_min_width` | photos in the article join the slides. `true` (default) takes `<figure>`s and paragraphs holding only a photo; `all` also takes a photo inside a paragraph of text or on its own, as Markdown and WordPress write them, captioned by the `<em>` right after it or its alt text. Then come selectors to skip and the smallest width that counts. The Presentation button appears only on a page with at least one slide |
| `theme` | each key becomes `--story-<key>` (`accent`, `accent_text`, `ink`, `paper`, `font`, `title_font`, `radius`). `accent_text` is the accent used as text on the paper. Its default is the accent mixed half with black, which keeps a bright brand colour at 4.5:1 or better |
| `progress` | `bar`, `position` (`top` or `bottom`), `time_left`, `words_per_minute` |
| `stage` | `side`, `width`, `min_width` (below it: a thumbnail), `hide_figures` |
| `slides` | `{image, at, caption, alt, duration}`. `at` is a CSS selector or `text:…`, the first block containing that text |
| `widgets` | `{at, type, stat, title, text, until}`. Types: `card` (default), `stat`, `quote`, `recipe` |
| `audio` | one track or a list; the first whose window holds today plays (see below) |
| `music_volume` | the music's level from 0 to 1 when a track sets no `volume` (default `0.4`) |
| `narrator` | `voice`, `rate`, `pitch`, `duck`, `subtitles`, `title`, `pronounce`, `pronounce_defaults` |
| `slideshow` | `title`, `header`, `footer`, `end`, `autoplay`, `interval`, `loop`, `fit`, `gestures`, `return_to_text`. The title card's `image: auto` takes `og:image`. On another host than the site's `domain` (staging, `ssg --http`) it loads from the host the page is on, and falls back to the first slide if it fails. `byline: auto` takes the `<time>` in the theme's `.post-meta` when there is one |
| `share` | `networks` (facebook, x, whatsapp, telegram, linkedin, pinterest, email, copy), `native` |
| `analytics` | `prefix` of the `dataLayer` events (default `story_`) |
| `labels` | every text on screen: `left`, `done`, `present`, `listen`, `stop`, `share`, `music`… |

### Music by date

```yaml
story:
  audio:
    - { from: "12-01", to: "01-06", playlist: [/music/carol-1.mp3, /music/carol-2.mp3] }
    - { dates: [{ from: "2026-03-22", to: "2026-04-12" }], src: /music/easter.mp3 }   # moving feasts
    - { src: /music/ambient.mp3, volume: 0.3, loop: true }                          # the rest of the year
```

A window is `MM-DD` (and may wrap the new year) or `YYYY-MM-DD`. A playlist
plays in random order.

**Volume.** A track's `volume` (0 to 1) sets how loud it plays, and
`music_volume` is the level for tracks that set none (default 0.4). The reader
can change it with the *Music volume* slider in the settings panel (the gear
button). That level is remembered in their browser and wins over the
configured one. While the narrator speaks, the music drops to `narrator.duck`
of the current level (default 30%). Browsers start sound only after a click, so music
begins with the presentation or the reader's Sound button. ssg ships no music.
Use tracks you hold the rights to.

### Pronunciation

Text is written for the eye, and a voice reading "100g" or "180°C" reads it
badly. ssg ships a set of rules per language (Polish and British English for
now) and uses the one matching the page's language. `narrator.pronounce` adds
rules on top of it, or turns one off with `null`.
`narrator.pronounce_defaults: false` leaves out the whole built-in set.

```yaml
story:
  narrator:
    pronounce:
      "{num} blaszka|blaszki|blaszek": "{num:jedna|dwie} {word}"
      "np.": null
```

| In a pattern | Means |
|---|---|
| `{num}` | a number: `100`, `1,5`, `0.25` |
| a space | a space or none: `"{num} g"` matches `100g` and `100 g` |
| `a\|b\|c` | any of these words |

Patterns match whole words only (`"{num} l"` leaves "5 lat" alone), and the
longest pattern wins.

| In what is said | Means |
|---|---|
| `{num}` | the number as written |
| `{num:jedna\|dwie}` | the number in words, with 1 and a final 2 in this gender: 1 → jedna, 22 → dwadzieścia dwie (Polish only) |
| `{word}` | the word the pattern's `a\|b\|c` matched, as written |
| `a\|b\|c\|d` | the form that agrees with the number before it. Polish: 1, 2–4, 5+, fractions. English: 1, other |

Only the spoken text changes; the page and the subtitles keep what is
written. Abbreviations the rules know (`e.g.`, `np.`, `łyż.`) do not end a
sentence. The built-in sets live in `internal/generator/assets/story/pronounce/`.
Every set's `examples` are tests that the browser's engine (`make test-story`)
and the Go engine (`go test ./internal/generator`) must both pass.

## Accessibility

- **One named region:** everything story mode adds to the page (stage,
  widgets, reading bar, buttons, the presentation) sits in a single region named
  by `labels.tools` (default "Story").
- **Named stage:** the stage is an image named after the photograph on show,
  or `labels.stage` before the first one.
- **Widgets are notes, not landmarks.** They take the widget's ink colour
  whatever colours the site's theme gives `p` and `h3`.
- **Phone layout:** below 700px the buttons show only their icons, keep their
  names for screen readers, and wrap inside the gutters.
- **Browsers without a voice:** in a browser without a working voice (no voices
  installed), the narrator gives up after its first utterance fails or never
  starts. The presentation then keeps to its own timer and stops starting the
  narrator. The `narrator` event carries `failed: true`.

## Events

The script dispatches `ssg-story` events on `document`. `detail.type` is one
of:

- `open`
- `slide` (with `index`, `total`, `kind`, `label` and `by`, where `by` is
  `user`, `narrator`, `timer` or `open`)
- `complete`
- `close`
- `narrator`
- `sound`
- `share`

With `analytics:`, each event is also pushed to `dataLayer` as `story_<type>`,
and clicks on the footer's links as `story_ad_click`.

## With Web Stories

When `web_stories:` also makes a story for the post, the article's Share menu
links it. The AMP story then takes from the post's `story:` settings what AMP,
which allows no script, can show. See
[WEB_STORIES.md](WEB_STORIES.md#from-story-settings).

## Not included

- 9:16 share pictures of each slide.
- Music files.

The prototype made the share pictures in a headless browser; ssg does not run
one.
