# Print / PDF and Cook Mode

Two optional buttons a reader finds under an article's title, next to
**Listen**. Both are off by default. They work in every theme, including themes
written before the features existed, and need no template function and no
dependency.

| | Print / PDF (`print:`) | Cook mode (`cook_mode:`) |
|---|---|---|
| What it does | Opens the browser's print dialog, which also offers **Save as PDF** | Shows a recipe full screen, one step at a time |
| Pages | Posts by default; pages with `sections` | Pages whose structured data is a complete Recipe |
| Needs | JavaScript for the button. The print stylesheet also applies when the reader prints from the browser menu | JavaScript and `<dialog>` |
| Data | The rendered article. On a Recipe page, also the recipe card | The Recipe's `recipeIngredient` and `recipeInstructions` |

## Print / PDF

```yaml
print:
  enabled: true
  label: "Print / PDF"      # button text
  sections: [posts]         # posts (default), pages, or both
  auto: true                # place it after the first </h1> when the theme has no slot; default true
  labels:                   # optional: the other texts, for another language
    recipe: "Print recipe"
    ingredients: "Ingredients"
    steps: "Method"
    yield: "Makes"
    prep: "Prep"
    cook: "Cook"
    total: "Total"
    printed: "Printed"
```

The button calls `window.print()`. Current versions of Chrome, Edge, Firefox
and Safari, on desktop and mobile, offer "Save as PDF" in that dialog, so
nothing on the site has to generate a PDF.

Pages that have the button also get a print stylesheet (`@media print`) that:

- hides the site header, navigation, footer, sidebars (`aside`), forms,
  comments, share bars, every button (Listen, Print and Cook mode among them),
  common cookie banners, and anything the theme marks `data-ssg-noprint`.
  A `<header>` or `<footer>` inside `<main>` or `<article>` stays, so the
  article's title survives;
- prints the article full width in black on white;
- keeps images, figures, tables and code blocks in one piece
  (`break-inside: avoid`) and scales images to the page width;
- keeps headings with the text after them (`break-after: avoid`);
- prints the address after each external link in the article
  (`a[href^="http"]::after`), except links that wrap an image;
- adds a footer line with the page's address and the date it was printed.
  The script fills it in just before printing, using the page language's date
  format.

### The recipe card

On a page whose merged structured data (`schema:`, `schema_defaults:`, or
filled in from the post) is a `Recipe` with ingredients or instructions, a
second button, **Print recipe**, prints only a compact card:

- the title;
- the yield and the prep, cook and total times, when they are set. ISO 8601
  durations such as `PT1H30M` are written for people, in the page's
  language;
- the ingredients as a list;
- the steps, numbered.

The card is built when the site is built and placed at the end of `<body>` as
`<section data-ssg-recipe-card hidden>`. Readers and screen readers never see
it on screen. While that one print runs, the script adds the
`ssg-print-recipe` class to `<html>`, and the print stylesheet then shows only
the card and the footer line. The class is removed on `afterprint`, so a
later print from the browser menu prints the full article again.

`recipeInstructions` can be a list of strings, of `HowToStep` objects
(`text`, else `name`) or of `HowToSection` objects (their `itemListElement`
steps, in order). A single string is split into one step per line. Any HTML
in a value is reduced to its text.

## Cook mode

```yaml
cook_mode: true            # shorthand for enabled: true

cook_mode:
  enabled: true
  label: "Cook mode"       # button text
  auto: true               # place it after the first </h1> when the theme has no slot; default true
  labels:                  # optional: the dialog's texts
    title: "Cook mode"     # dialog heading when the recipe has no name
    ingredients: "Ingredients"
    step: "Step {n} of {total}"
    next: "Next"
    back: "Back"
    close: "Close"
    restart: "Start over"
    done: "All done"
    finished: "Enjoy your meal."
```

The button appears only on pages whose merged structured data is a `Recipe`
with both `recipeIngredient` and `recipeInstructions`. It carries the recipe
as JSON in its `data-recipe` attribute (HTML-escaped), so opening the view
makes no request.

Pressing it opens a full-screen modal dialog:

1. the ingredients as a checklist. Ticked items are struck through;
2. one screen per step, in large text;
3. a finish screen. Its **Next** button reads **Close**.

To move between screens, use **Next** and **Back**, the Left and Right arrow
keys, a horizontal swipe, or a tap on the right or left half of the screen.
**Start over** clears the saved progress. **Close** or Escape closes the
dialog and returns focus to the button.

While the dialog is open, the screen is kept awake with the Screen Wake Lock
API. The lock is released when the dialog closes and taken again when the tab
becomes visible. Browsers without the API skip this step without an error.
The current screen and the ticked ingredients are saved in `localStorage`
under `ssg-cook:<path>`, so reopening the dialog or reloading the page returns
to the same step. Where storage is blocked, the view still works but does not
save progress.

## Placement and slots

Neither feature has a template function, because a bundled theme is also
built by older ssg releases. Each block goes into an empty element that the
theme provides:

```html
<span data-ssg-listen-slot></span><span data-ssg-print-slot></span><span data-ssg-cook-slot></span>
```

The bundled themes have all three next to the post's date or reading time.
Without a slot, the block is placed right after the first `</h1>`, in the
order Listen, Print, Cook mode, unless `auto: false` is set. With
`auto: false`, a block is only placed where the theme has a slot. ssg removes
any slot that nothing goes into, so a site with the features off produces the
same bytes as before.

The scripts and styles are inlined once, before `</body>`, and only on pages
that have a button. Each button stays `hidden` until its script runs, so no
reader sees a button that does nothing.

## Theming

The buttons use the theme's text colour (`currentColor`), so their contrast
matches the theme's own text. You can override them with these custom
properties:

| Property | Default | Used for |
|---|---|---|
| `--ssg-tool-fg` | `currentColor` | Button text and icon |
| `--ssg-tool-bg` | `transparent` | Button background |
| `--ssg-tool-border` | `currentColor` | Button border |
| `--ssg-tool-radius` | `999px` | Button corners |
| `--ssg-tool-focus` | `currentColor` | Focus ring |

The Cook mode dialog uses the Material baseline palette and follows
`prefers-color-scheme`:

| Property | Light | Dark | Used for |
|---|---|---|---|
| `--ssg-cook-bg` | `#ffffff` | `#131314` | Background |
| `--ssg-cook-fg` | `#1f1f1f` | `#e3e3e3` | Text |
| `--ssg-cook-muted` | `#444746` | `#c4c7c5` | Borders, ticked ingredients |
| `--ssg-cook-accent` | `#0b57d0` | `#a8c7fa` | Next button, focus ring, checkboxes |
| `--ssg-cook-on-accent` | `#ffffff` | `#062e6f` | Text on the Next button |
| `--ssg-cook-font` | inherited | inherited | Font family |
| `--ssg-cook-step-size` | `clamp(1.5rem, 1rem + 3vw, 2.75rem)` | same | Step text size |

If you set a property, it applies in both colour schemes. In that case, pick
values that work in both, or set them inside your own
`prefers-color-scheme` rule.

To keep something out of print, add `data-ssg-noprint` to it.

## Accessibility

- The buttons are native `<button>` elements with a visible text label. The
  icons are decorative (`aria-hidden`). Every button is at least 44×44 px and
  has a 3 px `:focus-visible` outline.
- The dialog is a native modal `<dialog>` with `aria-modal="true"`, labelled
  by its heading. Tab and Shift+Tab stay inside it, Escape closes it, and
  focus returns to the button that opened it.
- The progress line ("Step 3 of 8") is a polite live region, so screen
  readers announce each move.
- The ingredient checkboxes are real checkboxes, each inside its label, and
  each row is a 44 px target.
- The default colour pairs meet WCAG 2.2 AA: at least 4.5:1 for text and
  3:1 for controls, in light and dark.
- The step-change animation is turned off under
  `prefers-reduced-motion: reduce`.
