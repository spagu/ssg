# apidoc

A theme for documentation sites: guides, references generated from code
(`api_docs` with `root:`) and REST references from OpenAPI (`api_docs` with
`openapi:`). It ships inside the ssg binary, so `template: apidoc` works
without copying anything. See [docs/API_DOCS.md](../../docs/API_DOCS.md) and
[docs/REST_API_DOCS.md](../../docs/REST_API_DOCS.md).

## Layout

- **Header**: the name (`variables.title`, else the domain), search over
  `search-index.json` (`/` focuses it, arrow keys move through results),
  links from `variables.nav`, and the colour scheme switch.
- **Sidebar**: guides, then references. The current reference opens its tree
  (modules and classes, or tags and schemas). On narrow screens it is a drawer
  behind the menu button, and Escape closes it.
- **Article**: breadcrumbs on reference pages, a label, the title, the body.
- **On this page**: the article's h2 and h3, from 75rem wide, with the
  section in view marked.

The pages work without JavaScript: the table of contents stays hidden, search
is inert, and the sidebar is reached from the front page.

## Branding

| Setting | Use |
|---|---|
| `marketing.logo` (or `variables.logo`) | the mark beside the name in the header; `variables.logo_only: true` drops the name |
| `marketing.favicon`, `marketing.og_site_name`, `marketing.og_image` | the browser icon and social cards (ssg adds them to the head) |
| `marketing.social_profiles` | `{github: url, mastodon: url, …}`: links with their marks in the footer |
| `colors.primary`, `colors.link` | brand colours in the light scheme (buttons, links, current item) |
| `colors.primary_dark`, `colors.link_dark` | the same in the dark scheme |

Check a brand colour's contrast against the backgrounds below before using it:
4.5:1 for links, and for white text on buttons.

Marks known by name, for `social_profiles`, `nav` icons and `repository_icon`:
github, gitlab, codeberg, x (or twitter), mastodon, bluesky, threads, youtube,
discord, npm, stackoverflow, telegram, reddit, matrix, facebook, instagram.
Any other name gets a plain link mark. The marks come from Simple Icons
(CC0 1.0).

## Variables

| Variable | Use |
|---|---|
| `title` | name in the header, titles and footer |
| `tagline` | the front page's lead and description (else the site description) |
| `start` | `{url, label}`: the front page's main button |
| `nav` | `[{url, label, icon}]`: your header links, in order; with `icon` the link shows that mark |
| `repository_url` | the repository: an icon in the header (GitHub, GitLab, Codeberg by host) and a Source link in the footer |
| `repository_icon` | the header mark for the repository, when the host does not tell it |
| `footer_links` | `[{url, label}]`: extra links in the footer |
| `copyright` | the footer's first line instead of "© name" |
| `docs_nav_order` | guide slugs in reading order; others follow by title |
| `version` | shown in the footer |
| `gtm_id` | Google Tag Manager container (`GTM-XXXXXXX`); nothing loads without it |
| `ssg_credit` | `false` removes "Built with SSG" from the footer |

## Type

- Text: **Public Sans** (400–700), a neutral face with clear figures and
  distinct `I`/`l`/`1`.
- Code: **JetBrains Mono** (400, 600), with ligatures turned off, so `===`
  and `=>` read as the characters they are.

Both come from Google Fonts. Delete the two font tags in `partials/head.html`
to make no external request, and the system fonts take over.

## Colour

The Google palette (Blue 700, Grey 50–900, Green/Orange/Red 800 for HTTP
methods). All tokens are in `css/tokens.css`. Light and dark follow the
system, the switch overrides it, and the choice is remembered.

| Token | Light | Dark | Used for |
|---|---|---|---|
| `--ad-bg` | `#ffffff` | `#1f1f1f` | page |
| `--ad-surface` | `#f8f9fa` | `#28292a` | sidebar, cards, table heads |
| `--ad-text` | `#202124` | `#e8eaed` | text |
| `--ad-muted` | `#5f6368` | `#bdc1c6` | labels, dates, footer |
| `--ad-link` | `#1967d2` | `#8ab4f8` | links |
| `--ad-accent` / `--ad-on-accent` | `#1967d2` / `#ffffff` | `#8ab4f8` / `#202124` | buttons, current item |
| `--ad-control` | `#80868b` | `#9aa0a6` | field and button borders |
| `--ad-focus` | `#e8710a` | `#fdd663` | focus ring (3px) |
| `--ad-code-bg` | `#f1f3f4` | `#2d2e30` | code |
| `--ad-current` | `#e8f0fe` | `#233554` | current page, hover |
| `--ad-get` / `post` / `put` / `delete` | `#137333` `#1967d2` `#9a4b00` `#c5221f` | `#81c995` `#8ab4f8` `#fcad70` `#f28b82` | method labels |

### Contrast (WCAG 2.2)

Measured ratios; AA asks 4.5:1 for text and 3:1 for control borders and focus
indicators.

| Pair | Light | Dark |
|---|---|---|
| text on page | 16.1 | 13.7 |
| text on current item | 14.1 | 10.2 |
| muted on page / surface / code | 6.1 / 5.7 / 5.4 | 9.1 / 8.1 / 7.5 |
| link on page / code | 5.4 / 4.8 | 7.8 / 6.5 |
| button text on accent | 5.4 | 7.6 |
| method label text (white / `#202124`) | 5.4–6.2 | 6.7–8.7 |
| control border on page | 3.7 | 6.2 |
| focus ring on page | 3.1 | 11.8 |

Colour never carries meaning alone. The current page also has `aria-current`
and bold text, and a method label spells out the method. Every control is at
least 44×44 px, and motion stops under `prefers-reduced-motion`.

## Files

| File | What |
|---|---|
| `index.html`, `page.html`, `post.html`, `category.html` | the roles |
| `layouts/api-*.html` | reference pages, through `partials/api.html` |
| `partials/head.html` | meta tags, fonts, stylesheets, GTM, scheme |
| `partials/chrome.html` | header, sidebar, "on this page", footer |
| `partials/icons.html` | the brand marks |
| `partials/doc.html` | one document: sidebar, article, contents |
| `css/tokens.css`, `layout.css`, `content.css` | tokens, frame, article |
| `js/main.js` | scheme switch, drawer, contents, search |

The theme uses only template functions that every ssg release knows, because
themes in the repository are also built by older releases.
