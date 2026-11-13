# The panel

`{base}/` in a browser: the administration of everything the API does,
without a terminal. Static files in `public/`, no framework, no build step, no
npm package, nothing loaded from another origin (no web font, no CDN, no
analytics or tag manager — an authenticated admin tool is not measured). It
lives behind the same authentication as the API. Back to the
[README](../README.md).

## Views

| View | Address | What it does |
|---|---|---|
| Overview | `#/` | warnings (no transport, no site URL, no sender, no queue, a campaign the engine paused, dead letters, uncertain sends, missing bounce webhook), totals, campaigns sending now with live progress and this minute's rate use, charts |
| Subscribers | `#/subscribers?status=&language=&tag=&q=` | filter, search, load more; CSV export; mail the confirmation to pending imports (loops `POST /api/subscribers/confirmations`, waits out a 429, can stop) |
| Subscriber | `#/subscribers/<id>` | record, consent text and time, sends, suppression; tags and language; unsubscribe, re-send confirmation, GDPR export, erase |
| Import | `#/import` | CSV → preview (delimiter, editable column mapping, ignored columns and why, what each row becomes, dropped dates) → asks day-first or month-first when the file does not say → run |
| Campaigns | `#/campaigns?status=` | list by status, new draft, draft from a post on the feed |
| Campaign | `#/campaigns/<id>` | a draft opens the editor (autosaves, live recipient count, preview in each language as HTML or text, test send, schedule now or at a time, duplicate, delete); otherwise progress (polled every 3 s while sending, not while the tab is hidden), results, clicks per link, timing, uncertain sends to re-send or dismiss |
| Suppressions | `#/suppressions?reason=&q=` | new suppressions by reason over 90 days, list, add, lift (a complaint asks for an explicit yes), CSV import with preview |
| Settings | `#/settings` | what the worker is configured with (`GET /api/settings`), the SMTP connection test |
| Help | `#/help` | what each part does, where configuration lives, how to read the charts |

Routing is in the hash, so a deep link or a reload works under any
`NEWSLETTER_UI_BASE` with no server rule, and every URL the panel requests is
relative (`api/…` next to `index.html`).

The API offers a few fields the panel does not show yet: the import preview's
`detected.headers`, `detected.ignored[].code` and `classCounts.toStrengthen`,
`uncertainTotal` in `/api/engine/status`, and `transportProblem` in
`/api/settings` (see the [API reference](API.md)).

## Security

The CSP has no `unsafe-inline`: no inline script, no style attribute in the
HTML, every element built with `createElement` / `textContent` (no
`innerHTML` anywhere), the strings configuration a `type="application/json"`
block — data, which the browser never executes, so it needs no CSP exception.
Every non-GET request carries `x-newsletter-admin: 1` from one fetch wrapper
(`js/core/api.js`). The mail preview shows the rendered HTML in an
`<iframe sandbox="allow-same-origin">` (no scripts, no forms, no navigation):
it is parsed inert with `DOMParser`, its CSS is lifted out and re-applied
through the CSSOM (`element.style`, a constructed stylesheet), because a
`srcdoc` frame inherits the panel's CSP and the mail's inline styles would
otherwise be blocked; the CSP was not relaxed for it. Remote images in a body
are not loaded in the preview (`img-src 'self'`); a test send shows them. Only
the language and theme are kept in `localStorage` (and the chart period in
`sessionStorage`); never a token, an address or a file.

**Errors** are words with a way out: 401 → sign in again (Access or the
password), 429 → the wait, 503 → the missing setting or "deploy the
newsletter template first", a failed fetch → check the connection; each with
Try again. Every destructive action names its object and its consequence.

## Charts

SVG drawn by `js/charts/` (no library): line (sign-ups vs unsubscribes,
30/90/365 days), horizontal and stacked bars (status, language, latest
campaigns, a campaign's results, clicks per link), stacked columns
(suppressions per day/week/month), progress. Each has a visible title, a
summary sentence computed from the data, a legend for two or more series plus
direct labels, value axes from zero, and **Show data** with the same numbers
as a `<table>`. Series differ by more than colour (dashes, markers, textures:
diagonal lines, dots). The line chart can be read by keyboard: focus it, then
← → Home End. Charts are drawn at their measured width, so text stays at its
CSS size on a phone.

## Style and colour guidelines

These are the rules a change to the panel keeps; the tokens themselves, with
the reasoning, are in `public/css/tokens.css`, and chart colours per entity in
`public/css/charts.css`.

- **Palette: Material colour tokens.** Teal is the one primary hue; the warm
  Brown family serves as neutrals (instead of the usual grey); Deep Orange is
  reserved for the focus ring; the five-hue chart set is Teal, Deep Orange,
  Deep Purple, Pink and Blue — shades 600/700/800 on light, 400/600 on dark —
  checked for colour-vision separation. Each entity (a status, a reason, a
  series) keeps its colour in every chart.
- **Type: system fonts only**, never a web font. An editorial serif stack for
  headings (Iowan Old Style, Palatino, Georgia…) — the newsletter's print
  side — and a humanist sans for everything else (Segoe UI, system-ui, Noto
  Sans with Devanagari fallbacks); tabular figures for numbers.
- **Themes.** Light and dark follow the system, with a manual choice in the
  sidebar. Every token has a value for both.
- **Contrast: WCAG 2.2 AA.** Text ≥ 4.5:1, controls and chart marks ≥ 3:1.
  `test/panel-contrast.test.ts` computes every pair from the tokens, in both
  themes; a new colour pair is added there.
- **Interaction.** A 3 px focus ring (Deep Orange), targets ≥ 40 px,
  landmarks, one `h1` per view (focused on navigation), a polite live region
  for results, modal dialogs with Escape, captioned tables with `scope`,
  reduced motion honoured, 320 px wide without horizontal page scroll (lists
  become cards).
- **Colour is never the only signal**: series differ by dash, marker or
  texture, states by a word.

## Languages

en, pl, hi, de, fr in `public/js/strings/` — from `<html lang>`, then the
browser's languages, else English; a switcher in the sidebar. Override single
strings without editing the panel:

```html
<script id="newsletter-admin-config" type="application/json">
  {"lang": "pl", "i18n": {"pl": {"nav.dashboard": "Start"}}}
</script>
```

## Local preview

Invented data (about 1 800 subscribers, campaigns in every state, one
actually sending, suppressions, a dead letter; example.com addresses only),
the real worker on an in-memory SQLite database, a pretend SMTP server that
refuses addresses containing "bounce", the cron every 15 s:

```sh
node --import ./workers/newsletter-ui/test/resolve.mjs workers/newsletter-ui/test/dev-server.mjs
# → http://localhost:8787/newsletter-admin/  (any user name, password "demo")
```

`test/panel-shots.mjs` (not part of the test run; needs Playwright installed
on the machine, never as a dependency here) takes screenshots of every view,
light and dark, at 1440×900 and 390×844, and runs axe-core when given its
file: `node workers/newsletter-ui/test/panel-shots.mjs <out-dir> [axe.min.js]`.
