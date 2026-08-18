package generator

import (
	"fmt"
	"os"
	"path/filepath"
)

// The stylesheet and script the scaffolded templates ask for (#172).
//
// Every scaffolded template carried `<link rel="stylesheet" href="/css/style.css">`
// and `<script src="/js/main.js">`, and nothing wrote either file. The site
// built, the pages were right, the links worked, and a browser rendered unstyled
// HTML with two 404s in its console — while the build reported success. Nothing
// caught it: the link checker walks internal links, and a missing stylesheet is
// not one.
//
// This is the ordinary outcome whenever the theme step of a migration is skipped
// or fails — a site behind bot protection, a one-page site, an operator who
// chose not to rebuild the theme — so the operator's first thought is that the
// migration lost the site's design, rather than that the scaffold never had one.
//
// So the scaffold writes what it references. Deliberately plain: a readable
// column, a type scale, a nav that collapses. It should look like a decision,
// not like a stylesheet that failed to load.

// scaffoldStylesheet is written to the theme's css/style.css, which copyAssets
// publishes at /css/style.css — the address every scaffolded template names.
//
// System fonts only and no external references (FE-011): a scaffold that fetches
// a font from a CDN makes a site slower and less private than the one it
// replaced. Colours meet WCAG 2.2 AA against their backgrounds.
const scaffoldStylesheet = `/* Scaffolded by ssg. Plain on purpose — edit freely, or replace the file. */

:root {
  --ink: #1a1c1e;
  --ink-soft: #44474b;
  --paper: #ffffff;
  --rule: #d8dade;
  --accent: #0b57d0;      /* 8.6:1 on white */
  --accent-soft: #e8f0fe;
  --measure: 42rem;
  --gap: 1.5rem;
}

@media (prefers-color-scheme: dark) {
  :root {
    --ink: #e3e3e3;
    --ink-soft: #b4b7bb;
    --paper: #131416;
    --rule: #35383c;
    --accent: #a8c7fa;    /* 9.2:1 on the dark paper */
    --accent-soft: #1f2733;
  }
}

*, *::before, *::after { box-sizing: border-box; }

body {
  margin: 0;
  background: var(--paper);
  color: var(--ink);
  font: 1rem/1.65 system-ui, -apple-system, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
  -webkit-text-size-adjust: 100%;
}

.container {
  width: 100%;
  max-width: var(--measure);
  margin-inline: auto;
  padding-inline: var(--gap);
}

/* Keyboard users reach the content without walking the nav. */
.skip-link {
  position: absolute;
  left: -9999px;
  padding: 0.5rem 1rem;
  background: var(--accent);
  color: var(--paper);
}
.skip-link:focus { left: 1rem; top: 1rem; z-index: 10; }

a { color: var(--accent); text-decoration-thickness: 1px; text-underline-offset: 0.15em; }
a:hover { text-decoration-thickness: 2px; }
:focus-visible { outline: 3px solid var(--accent); outline-offset: 2px; }

.site-header { border-bottom: 1px solid var(--rule); }
.main-nav {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--gap);
  padding-block: 1rem;
}
.logo { font-weight: 600; text-decoration: none; color: var(--ink); }
.nav-links { display: flex; flex-wrap: wrap; gap: 1.25rem; }
.nav-link { text-decoration: none; color: var(--ink-soft); }
.nav-link:hover { color: var(--accent); text-decoration: underline; }

/* The toggle is hidden until the links are, so it never appears without a job. */
.menu-toggle {
  display: none;
  flex-direction: column;
  gap: 4px;
  padding: 0.5rem;
  background: none;
  border: 1px solid var(--rule);
  border-radius: 4px;
  cursor: pointer;
}
.menu-toggle span { width: 20px; height: 2px; background: var(--ink); }

@media (max-width: 40rem) {
  .menu-toggle { display: flex; }
  .nav-links { display: none; width: 100%; flex-direction: column; gap: 0.75rem; padding-bottom: 1rem; }
  .nav-links.is-open { display: flex; }
  .main-nav { flex-wrap: wrap; }
}

.main-content { padding-block: 2.5rem 3.5rem; }

h1, h2, h3 { line-height: 1.25; text-wrap: balance; margin-block: 2rem 0.75rem; }
h1 { font-size: clamp(1.8rem, 1.3rem + 2.2vw, 2.6rem); margin-block-start: 0; }
h2 { font-size: 1.5rem; }
h3 { font-size: 1.2rem; }
p, ul, ol, blockquote, pre, table { margin-block: 0 1.25rem; }
img, video { max-width: 100%; height: auto; }

blockquote {
  margin-inline: 0;
  padding: 0.25rem 0 0.25rem 1rem;
  border-left: 3px solid var(--rule);
  color: var(--ink-soft);
}

code { font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; font-size: 0.9em; }
pre {
  padding: 1rem;
  overflow-x: auto;               /* long lines scroll rather than widen the page */
  background: var(--accent-soft);
  border-radius: 6px;
}
pre code { font-size: 0.85rem; }

table { width: 100%; border-collapse: collapse; }
th, td { padding: 0.5rem 0.75rem; border-bottom: 1px solid var(--rule); text-align: left; }

.hero { padding-block: 1rem 2rem; }
.hero-title { margin-block-start: 0; }

/* Listings: index, category and tag archives share one shape. */
.post-list, .page-list { list-style: none; padding: 0; }
.post-item, .page-item { padding-block: 1.25rem; border-bottom: 1px solid var(--rule); }
.post-item h2, .page-item h2 { margin-block: 0 0.35rem; font-size: 1.25rem; }
.post-meta { color: var(--ink-soft); font-size: 0.9rem; }
.post-excerpt { margin-block: 0.5rem 0; }

.pagination { display: flex; gap: 0.5rem; flex-wrap: wrap; align-items: center; padding-block: 2rem; }
.pagination a, .pagination span { padding: 0.35rem 0.7rem; border: 1px solid var(--rule); border-radius: 4px; text-decoration: none; }
.pagination [aria-current="page"] { background: var(--accent-soft); border-color: var(--accent); font-weight: 600; }

.site-footer {
  border-top: 1px solid var(--rule);
  padding-block: 1.5rem;
  color: var(--ink-soft);
  font-size: 0.9rem;
}

@media print {
  .site-header, .site-footer, .skip-link, .pagination { display: none; }
  body { color: #000; background: #fff; }
}
`

// scaffoldScript is written to the theme's js/main.js. The templates ship a
// menu button, and a button that does nothing is worse than no button.
//
// It touches only the nav it owns, and does nothing at all if that nav is not
// there — a theme that keeps the script while replacing the markup gets silence
// rather than an error in the console.
const scaffoldScript = `// Scaffolded by ssg. The one behaviour the templates need: a nav that opens.
(function () {
  "use strict";

  var toggle = document.getElementById("menu-toggle");
  var links = document.getElementById("nav-links");
  if (!toggle || !links) return;

  toggle.setAttribute("aria-expanded", "false");
  toggle.setAttribute("aria-controls", "nav-links");

  function setOpen(open) {
    links.classList.toggle("is-open", open);
    toggle.setAttribute("aria-expanded", open ? "true" : "false");
  }

  toggle.addEventListener("click", function () {
    setOpen(!links.classList.contains("is-open"));
  });

  // Escape closes it, and focus returns to the control that opened it.
  document.addEventListener("keydown", function (e) {
    if (e.key === "Escape" && links.classList.contains("is-open")) {
      setOpen(false);
      toggle.focus();
    }
  });

  // Leaving the narrow layout must not strand the menu in its open state.
  var narrow = window.matchMedia("(max-width: 40rem)");
  var onChange = function (e) { if (!e.matches) setOpen(false); };
  if (narrow.addEventListener) narrow.addEventListener("change", onChange);
  else if (narrow.addListener) narrow.addListener(onChange);
})();
`

// scaffoldAssets are the files the scaffolded templates reference, keyed by
// their path inside the theme. copyAssets publishes css/ and js/ at the site
// root, which is where the templates look for them.
var scaffoldAssets = map[string]string{
	"css/style.css": scaffoldStylesheet,
	"js/main.js":    scaffoldScript,
}

// writeScaffoldAssets writes the stylesheet and script beside the templates
// that ask for them. An asset already on disk is left alone: a theme being
// edited must not have its stylesheet replaced by a rebuild.
func writeScaffoldAssets(themePath string) error {
	for rel, content := range scaffoldAssets {
		path := filepath.Join(themePath, filepath.FromSlash(rel))
		if _, err := os.Stat(path); err == nil {
			continue
		}
		// #nosec G301 -- theme asset directories are served, so world-traversable
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return fmt.Errorf("creating %s: %w", filepath.Dir(rel), err)
		}
		// #nosec G306 -- Web content files need to be world-readable
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return fmt.Errorf("creating %s: %w", rel, err)
		}
	}
	return nil
}
