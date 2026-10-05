// The small HTML pages the worker renders itself: confirm, unsubscribe, and the
// error page a script-less form post can land on. A leading underscore keeps
// this file out of the Pages route table — it is imported, never served.

import { STRINGS } from "./_i18n";

const escapeHTML = (s: string): string =>
  s.replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c] as string);

// Colours (Google palette) meet WCAG 2.2 AA, 4.5:1 for text, in both schemes:
// #202124 on #ffffff and #e8eaed on #202124 for text; buttons are #ffffff on
// #174ea6 (light) and #202124 on #8ab4f8 (dark), both above 7:1.
const STYLE = `
:root{color-scheme:light dark;--fg:#202124;--bg:#fff;--btn:#174ea6;--btn-fg:#fff}
@media (prefers-color-scheme:dark){:root{--fg:#e8eaed;--bg:#202124;--btn:#8ab4f8;--btn-fg:#202124}}
body{margin:0;background:var(--bg);color:var(--fg);font:1.0625rem/1.6 system-ui,-apple-system,"Segoe UI","Noto Sans","Noto Sans Devanagari",sans-serif}
main{max-width:34rem;margin:12vh auto;padding:0 1rem}
h1{font-size:1.5rem;line-height:1.3}
a{color:inherit}
button{font:inherit;padding:.6rem 1.2rem;border:0;border-radius:.375rem;background:var(--btn);color:var(--btn-fg);cursor:pointer}
button:focus-visible,a:focus-visible{outline:3px solid currentColor;outline-offset:3px}`;

// page renders a complete, standalone document. The links on these pages carry
// a subscriber token in their URL, so the response forbids sending it on as a
// Referer, forbids caching, keeps the page out of search results, and allows
// no script at all.
export function page(lang: string, title: string, bodyHTML: string, status = 200): Response {
  const t = STRINGS[lang] || STRINGS.en;
  const doc = `<!doctype html>
<html lang="${escapeHTML(lang)}">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex, nofollow">
<meta name="referrer" content="no-referrer">
<meta name="description" content="${escapeHTML(title)}">
<title>${escapeHTML(title)}</title>
<style>${STYLE}</style>
</head>
<body>
<main>
<h1>${escapeHTML(title)}</h1>
${bodyHTML}
<p><a href="/">${escapeHTML(t.back)}</a></p>
</main>
</body>
</html>`;
  return new Response(doc, {
    status,
    headers: {
      "content-type": "text/html; charset=utf-8",
      "cache-control": "no-store",
      "referrer-policy": "no-referrer",
      "x-robots-tag": "noindex, nofollow",
      "content-security-policy": "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'",
    },
  });
}

// message is a page with one paragraph.
export function message(lang: string, title: string, text: string, status = 200): Response {
  return page(lang, title, `<p>${escapeHTML(text)}</p>`, status);
}

// action is a page asking for one click before a change is made. The change
// happens on the POST, never on the GET: mail scanners and link previewers
// fetch every URL in a message, and a GET that acted would unsubscribe — or
// confirm — people who never clicked anything.
export function action(lang: string, title: string, ask: string, button: string, postURL: string): Response {
  return page(
    lang,
    title,
    `<p>${escapeHTML(ask)}</p>
<form method="post" action="${escapeHTML(postURL)}"><button type="submit">${escapeHTML(button)}</button></form>`,
  );
}
