# Admin panels' look (admin_theme)

The admin panels — comments, newsletter, shop, accounts, the admin home — ship
with a neutral look. `admin_theme:` in `.ssg.yaml` gives them the site's name,
logo, colours and fonts. It takes effect through the `core` worker, which
serves the result; without `core` it does nothing. The keys are checked at
build time, and the colours are adjusted until every pair the panels use keeps
its WCAG 2.2 contrast.

## The keys

| Key | Default | Notes |
|---|---:|---|
| `admin_theme` | empty | The site's look for the admin panels, served by the `core` worker. Absent: the panels look as shipped |
| `admin_theme.name` | empty | Plain text, at most 80 characters, shown in the shared navigation instead of "Site admin" |
| `admin_theme.logo` | empty | A `.svg`, `.png` or `.webp` file on the site's own host (`/img/logo.svg`); shown through `<img>` only, with the name as its alternative text |
| `admin_theme.logo_dark` | empty | The same for dark backgrounds; the navigation bar (dark in both schemes) shows it when set |
| `admin_theme.main` | empty | The brand's main colour, `#rgb` or `#rrggbb`: the navigation bar, primary buttons, the current menu item, the first chart series |
| `admin_theme.accent` | empty | The accent: links, the focus ring, the current-page marker in the bar, the second chart series. Either colour alone derives the other |
| `admin_theme.font` | empty | Font-family list for headings (`Georgia, serif`) |
| `admin_theme.font_body` | empty | Font-family list for everything else |
| `admin_theme.font_files` | empty | `@font-face` files: `family`, `src` (a `.woff2`, `.woff`, `.ttf` or `.otf` on this host), optional `weight` (`400`, `400 700`, `bold`) and `style` (`normal`, `italic`, `oblique`); at most 8 |
| `admin_theme.tokens` | empty | Advanced: `light:` / `dark:` maps overriding single derived tokens (`primary`, `on-primary`, `primary-soft`, `on-primary-soft`, `link`, `focus`, `c1`–`c5`, `nav-bg`, `on-nav`, `nav-muted`, `nav-hover`, `nav-control`, `nav-current`) |
| `check_admin_theme` | `warn` | `off`, `warn` or `strict`: a logo or font file missing from the output, an override that breaks a contrast pair, `admin_theme` without a `core` worker |

```yaml
admin_theme:
  name: "Anna's Kitchen"
  logo: /img/logo.svg
  logo_dark: /img/logo-light-text.svg
  main: "#1b3a4b"
  accent: "#e07a5f"
  font: "Georgia, serif"
  font_body: '"Brand Sans", system-ui'
  font_files:
    - family: "Brand Sans"
      src: /fonts/brand-sans.woff2
      weight: "400 700"
  tokens:
    dark:
      focus: "#ffcc80"
```

## What is derived

Four things to set, the rest is computed. From `main` and `accent` the build
derives every colour token the panels use for the light and the dark scheme,
so that text keeps 4.5:1 on its background (links in the accent colour
included, on the page, on cards and on inset areas) and the focus ring, chart marks
and the bar's edges 3:1 (WCAG 2.2), the focus ring stays apart from the
primary colour and every two chart series stay apart. A brand colour that can
serve its role is used exactly; one that cannot keeps its hue and moves to the
nearest lightness that works, and the core worker's **Appearance** view
(`/core/#appearance`) says where and why. Neutral backgrounds and text and the
state colours (error, warning, success, info) are never themed — red always
means an error. The result is written as `/core/theme.css` (over the empty file
the `core` template ships) and as the module registry's `theme`
([WORKER-MODULES.md](WORKER-MODULES.md#the-panels-look-theme)); panels pick it
up by loading `/core/theme.css` after their own tokens. Pages visitors see
(subscription confirmations, unsubscribing) are never themed.

## What stops the build

Every value is checked at build time, and one that cannot be used stops the
build with the file and the key, whatever `check_admin_theme` says: a colour
that is not opaque hex (an alpha channel is refused — a translucent colour has
no contrast ratio of its own); a logo or font path that is not a file on this
host (no scheme, host, `//`, `..`, backslash, query, fragment or space — the
panels' content security policy allows images and fonts from their own host
only, and stays that way); a font list outside the grammar (family names,
quoted or not, and generic families, separated by commas — nothing that could
end a CSS declaration); a name with control characters or longer than 80
characters; an unknown token or scheme under `tokens`. The guidelines — how to
choose the two colours, what is derived, what cannot be set and why — are in
the [core worker's README](../workers/core/README.md#panel-look-admin_theme).

Without a `core` worker `admin_theme` has no effect (a finding); a site
without `admin_theme` builds byte for byte as before.
