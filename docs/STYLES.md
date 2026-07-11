# SSG Style & Color Guidelines

This document describes the visual design tokens of the built-in themes. All text
colors meet **WCAG 2.2 AA** contrast (≥4.5:1 for normal text) against their intended
background (audit FE-002).

## Principles

- **Accessibility first** — WCAG 2.2 AA contrast for body and muted text.
- **System fonts** — themes use a native font stack (no external font CDN, no visitor
  IP leak to third parties; audit FE-003).
- **CSS custom properties** — every color/spacing token is a `--var` in `:root`, so a
  theme can be re-skinned by overriding variables.
- **Responsive & mobile-first** — layouts use relative units and flexbox/grid.

## Themes

### `krowy` — light, natural green

| Token | Value | Notes |
|-------|-------|-------|
| `--color-bg-primary` | `#f8faf5` | page background |
| `--color-text` | dark green/near-black | body text |
| `--color-text-muted` | `#4a6b4a` | meta/dates — **5.72:1** on bg-primary (AA) |
| `--font-family` | `'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif` | system stack |

### `simple` — modern dark

| Token | Value | Notes |
|-------|-------|-------|
| `--color-bg-card` | `#222222` | card background |
| `--color-text-muted` | `#9a9a9a` | meta/dates — **5.65:1** on card (AA) |
| `--color-accent` | `#6366f1` | links/buttons; paired with light text on the accent as a button background |
| `--font-family` | `'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif` | system stack |

### `imd` — editorial

| Token | Value | Notes |
|-------|-------|-------|
| `--font-main` | `'Manrope', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif` | system fallback |
| `--font-head` | `'Space Grotesk', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif` | system fallback |

## Checking contrast

When changing a text color, verify it against its background:

- Normal text (< 18.66px, or < 24px non-bold): **≥ 4.5:1**
- Large text (≥ 24px, or ≥ 18.66px bold): **≥ 3:1**

Tools: [WebAIM Contrast Checker](https://webaim.org/resources/contrastchecker/) or any
WCAG contrast calculator. Accent colors used as **links** rely on additional non-color
affordances (underline/hover); accent used as a **button background** must keep its label
text at ≥ 4.5:1.

## Fonts

Themes ship no bundled or CDN webfonts. The first family in each stack (`Inter`,
`Manrope`, `Space Grotesk`) is used only if the visitor already has it installed;
otherwise the browser falls back to the platform UI font. This keeps builds
self-contained and privacy-respecting.
