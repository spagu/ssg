# ssgtheme

The documentation theme shipped with SSG. It renders this repository's own
`docs/` folder (`make site`) and doubles as the reference for how a theme is
put together: shared chrome in `partials/`, tokens separated from components,
no external requests.

```text
templates/ssgtheme/
├── index.html          # homepage: hero + documentation cards + latest posts
├── page.html           # a guide
├── post.html           # a post, with tags and series navigation
├── category.html       # category/tag/author/series archives
├── partials/
│   └── chrome.html     # sc-head, sc-header, sc-footer — the shared markup
├── css/
│   ├── tokens.css      # design tokens only (colour, type, space, motion)
│   └── style.css       # layout and components; imports tokens.css
└── js/
    └── main.js         # progressive enhancement: menu, colour scheme
```

## Design system

The visual language is the [Tradik design system](https://designstyles.tradik.com/).
`tokens.css` mirrors that system's published stylesheet 1:1 — token names and
values included — so a lookup there is valid here: the ink and accent ramps,
signal and surface colours, the `--color-bg-*` / `--color-fg-*` /
`--color-border-*` semantic sets, the three typefaces, the fluid type scale,
the 4 px spacing grid, radius, elevation, motion and breakpoints.

Two additions are marked in the file as theme-local: the dark scheme (upstream
ships light only) and `--color-hero-wash`.

The components follow the same system: hairline `--color-border-subtle`
borders, `--color-bg-subtle` surfaces, whole-card links that lift 2 px on
hover without a shadow, navigation that marks the current page with an accent
underline rather than a filled pill, and an **inverse** code block.

### Colour

| Role | Light | Dark | Contrast |
|---|---|---|---|
| `--color-fg-primary` | `#0F172A` | `#F8FAFC` | 17.9:1 / 16.9:1 — AAA |
| `--color-fg-secondary` | `#334155` | `#E2E8F0` | 10.4:1 / 13.4:1 — AAA |
| `--color-fg-muted` | `#64748B` | `#94A3B8` | 4.8:1 / 6.9:1 — AA |
| `--color-fg-accent` | `#0050A6` | `#99BFEB` | 7.8:1 / 9.1:1 — AA body |
| `--color-fg-danger` | `#B42318` | `#FDA29B` | 6.6:1 / 8.4:1 — AA body |

Dark mode is not a second palette: surfaces walk down the same ink ramp and
links move up the same accent ramp, because `accent-500` on a dark surface fails
contrast. Both are declared twice on purpose — once under
`@media (prefers-color-scheme: dark)` for the OS preference, once under
`:root[data-theme="dark"]` for the header toggle, so an explicit choice wins in
both directions.

### Typography

`Geist` for UI and body, `Instrument Serif` for display, `Geist Mono` for code —
the system's own faces, loaded from Google Fonts by two tags in
`partials/chrome.html`. Delete those two tags to make the theme issue **zero**
external requests: every family has a full system fallback stack
(`system-ui`, `Georgia`, `ui-monospace`), so the layout is unchanged and only
the faces differ.

Sizes are the system's nine fluid `clamp()` steps; nothing in `style.css`
hard-codes a font size.

### Spacing, radius, motion

Every length comes from a token (`--space-*`, `--radius-*`). Motion is capped at
200 ms and disabled entirely under `prefers-reduced-motion: reduce`.

## Accessibility (WCAG 2.2)

- All body text meets AAA, all other text and UI meets AA, in both schemes.
- Interactive targets are at least 40 px tall (2.5 rem) — target size AA needs
  24 px, so there is margin to spare.
- Visible focus ring on every focusable element (`:focus-visible`, 3 px), a
  skip link, and `aria-current="page"` on the active navigation entry.
- The mobile menu and colour-scheme toggle carry `aria-expanded` /
  `aria-pressed`; Escape closes the menu and returns focus to its button.
- Nothing depends on JavaScript to be readable or navigable.
- Over a hero photograph the copy keeps its ratios by construction: the scrim
  is a horizontal gradient, heaviest under the text column. Measured on the
  built page, the heading reads 16.3:1 and the lead paragraph 8.7:1 (5.8:1 over
  the brightest patch of the photo) — AA throughout, AAA for the heading.

## Analytics

`partials/chrome.html` contains a Google Tag Manager snippet with the
placeholder container `GTM-XXXXXXX`. It is inert until replaced: the loader
returns early while the ID still contains `XXXXXXX`, so no request is made and
no consent banner is required until you opt in.

## Template contract

`sc-head` takes a dict, everything else takes the page context:

```gotemplate
{{ template "sc-head" (dict
    "Title" (printf "%s — %s" .Page.Title .Domain)
    "Desc" .Page.Excerpt
    "Canonical" (printf "/%s/" .Page.Slug)
    "OgType" "article"
    "Ctx" .) }}
{{ template "sc-header" . }}
{{ template "sc-footer" . }}
```

Files in `partials/` are parsed into the same template set as the theme root,
so these define names are callable from any role template. See
[docs/TEMPLATES.md](../../docs/TEMPLATES.md#template-loading-and-sharing).

## Site configuration the theme reads

| Variable | Effect |
|---|---|
| `variables.logo` | Brand mark beside the site name, resized at build time (header 36 px, footer 48 px tall). **Must carry its own alpha** — a logo saved on a white plate shows as a white box in dark mode. Rendered as PNG so a site without `cwebp` still builds. |
| `variables.hero_image` | Optional homepage hero photograph, resized to 1920/900 WebP and laid under a scrim that keeps the copy at AA/AAA contrast. Unset ⇒ a plain hero. |
| `variables.nav` | Header navigation: a list of `{label, url}`. Fixed and hand-picked — the footer carries the full guide directory, so the bar stays one line however many guides exist. |
| `variables.github_repo` | `owner/name`; enables the header star count, fetched client-side and hidden when GitHub does not answer or the repository has no stars |
| `variables.version` | Shown as `SSG vX.Y.Z` beside the navigation, linking to that release |
| `variables.repository_url` | Footer and hero links; defaults to the SSG repository |

```yaml
variables:
  logo: logo.png          # assets/logo.png — transparent PNG
  hero_image: river.jpg   # assets/river.jpg
  github_repo: spagu/ssg
  version: "1.8.12"
  nav:
    - label: Docs
      url: /#documentation
    - label: GitHub
      url: https://github.com/spagu/ssg
```

## Using it

```bash
ssg my-content ssgtheme example.com          # any content source
make site                                    # this repository's docs/
make site-watch                              # rebuild + serve on change
```
