// The frame every sign-in page shares, and the pieces forms are built from. A
// leading underscore keeps this file out of the Pages route table.
//
// The pages are rendered here, on the server, because each depends on the
// request (the language, an error code, the session, a token) and must work
// as plain HTML forms with JavaScript off. No inline script or style: the
// page links /auth/assets/ (public/auth/assets/ in this template), and the
// CSP allows only that. Every value from a request or the database goes
// through esc(). Nothing is measured: no tag manager, no analytics.

import { LANGS, LANG_NAMES, STRINGS, asLang, langCookie } from "./_i18n";
import type { Lang, Strings } from "./_i18n";
import { whoAmI } from "./_context";
import type { Ctx, Who } from "./_context";
import { redirect, secure } from "./_lib";
import type { Rules } from "./_pages-errors";
import { resetSeconds } from "./_resets";

export const esc = (s: string): string =>
  s.replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c] as string);

export const ASSETS = "/auth/assets";
export const PAGE_CSP =
  "default-src 'none'; style-src 'self'; script-src 'self'; img-src 'self'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'";

// strict-origin, not no-referrer: under no-referrer a browser sends
// "Origin: null" with a form POST (Fetch, "serializing a request origin"), and
// the CSRF defence (_request.ts) rightly refuses that — no form would work.
// strict-origin sends the origin alone, never a path or a query, so a token in
// this page's URL never travels in a Referer either; and nothing over http.
export const PAGE_REFERRER = "strict-origin";

export interface Page {
  request: Request;
  lang: Lang;
  title: string; // the h1 and the <title>
  description: string;
  body: string; // HTML, already escaped
  status?: number;
  error?: boolean; // the page shows an error: "Error: " leads the <title>
  account?: { email: string }; // signed in: the header links to the panels and the account
  cookies?: string[];
  csp?: string; // a page that needs more than PAGE_CSP (_pages-csp.ts, GO-151)
  script?: string; // one more script from another origin, allowed by `csp` (Turnstile)
}

// switcher is the language menu: plain links to this page with ?lang=, the
// rest of the query kept, so it works without JavaScript.
function switcher(request: Request, lang: Lang, t: Strings): string {
  const url = new URL(request.url);
  const items = LANGS.map((l) => {
    url.searchParams.set("lang", l);
    const current = l === lang ? ' aria-current="true"' : "";
    return `<li><a href="${esc(url.pathname + url.search)}" lang="${l}" hreflang="${l}"${current}>${esc(LANG_NAMES[l])}</a></li>`;
  });
  return `<nav class="langs" aria-label="${esc(t.language)}"><ul>${items.join("")}</ul></nav>`;
}

// navLink marks the page you are on (aria-current), for screen readers too.
const navLink = (p: Page, href: string, label: string): string =>
  `<a href="${href}"${new URL(p.request.url).pathname === href ? ' aria-current="page"' : ""}>${esc(label)}</a>`;

function header(p: Page, t: Strings): string {
  const site = esc(new URL(p.request.url).host);
  const nav = p.account
    ? `<nav class="top-nav" aria-label="${esc(t.moduleTitle)}">${navLink(p, "/auth/", t.panelsTitle)}${navLink(p, "/auth/account", t.accountTitle)}
<form method="post" action="/api/auth/logout"><input type="hidden" name="next" value="/auth/login?ok=signed_out"><button class="btn btn-quiet" type="submit">${esc(t.signOut)}</button></form></nav>`
    : "";
  return `<header class="bar"><p class="brand"><span class="brand-site">${site}</span> <span class="brand-module">${esc(t.moduleTitle)}</span></p>${nav}</header>`;
}

// page renders a complete document with the security headers: no caching, a
// Referer without path or query (tokens travel in these URLs), not indexed,
// never framed.
export function page(p: Page): Response {
  const t = STRINGS[p.lang];
  const title = p.error ? `${t.errorPrefix}: ${p.title}` : p.title;
  const doc = `<!doctype html>
<html lang="${p.lang}">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>${esc(title)}</title>
<meta name="description" content="${esc(p.description)}">
<meta name="robots" content="noindex, nofollow">
<meta name="referrer" content="strict-origin">
<meta name="color-scheme" content="light dark">
<meta name="theme-color" content="#efebe9" media="(prefers-color-scheme: light)">
<meta name="theme-color" content="#121212" media="(prefers-color-scheme: dark)">
<meta name="format-detection" content="telephone=no">
<link rel="icon" href="${ASSETS}/icon.svg" type="image/svg+xml">
<link rel="stylesheet" href="${ASSETS}/tokens.css">
<link rel="stylesheet" href="${ASSETS}/auth.css">
<script src="${ASSETS}/auth.js" defer></script>${p.script ? `\n<script src="${esc(p.script)}" async defer></script>` : ""}
</head>
<body data-show-password="${esc(t.showPassword)}" data-hide-password="${esc(t.hidePassword)}">
${header(p, t)}
<main id="main">
<h1>${esc(p.title)}</h1>
${p.body}
</main>
<footer class="foot">${switcher(p.request, p.lang, t)}<p>${esc(t.privacy)}</p></footer>
</body>
</html>`;
  const res = secure(new Response(doc, { status: p.status ?? 200, headers: { "content-type": "text/html; charset=utf-8" } }), {
    "content-security-policy": p.csp ?? PAGE_CSP,
    "referrer-policy": PAGE_REFERRER,
  });
  // A language picked with the switcher sticks for the pages under /auth.
  const picked = asLang(new URL(p.request.url).searchParams.get("lang"));
  for (const c of [...(p.cookies ?? []), ...(picked ? [langCookie(picked)] : [])]) res.headers.append("set-cookie", c);
  return res;
}

// notice is the message at the top of a form: an error (role=alert, focused
// by auth.js) or a confirmation (role=status).
export function notice(kind: "error" | "ok", heading: string, text: string): string {
  const role = kind === "error" ? 'role="alert" tabindex="-1"' : 'role="status"';
  const cls = kind === "error" ? "callout critical" : "callout ok";
  return `<div class="${cls}" id="notice" ${role}><p class="callout-title">${esc(heading)}</p><p>${esc(text)}</p></div>`;
}

export interface Field {
  id: string;
  name: string;
  label: string;
  type: "email" | "password" | "text";
  autocomplete: string;
  value?: string;
  hint?: string;
  error?: string; // shown under the label, announced with the field
  errorRef?: string; // or: the id of an error shown elsewhere (one message for two fields)
  required?: boolean;
  minlength?: number;
  maxlength?: number;
}

// field renders a labelled input; the hint and the error are tied to it with
// aria-describedby, an error also sets aria-invalid. Pasting is never blocked
// and the autocomplete value lets password managers fill it (WCAG 3.3.8).
export function field(f: Field): string {
  const ids: string[] = [];
  let extra = "";
  if (f.hint) {
    ids.push(`${f.id}-hint`);
    extra += `<p class="hint" id="${f.id}-hint">${esc(f.hint)}</p>`;
  }
  if (f.error) {
    ids.push(`${f.id}-error`);
    extra += `<p class="field-error" id="${f.id}-error">${esc(f.error)}</p>`;
  }
  if (f.errorRef) ids.push(f.errorRef);
  const attrs = [
    `id="${f.id}"`,
    `name="${f.name}"`,
    `type="${f.type}"`,
    `autocomplete="${f.autocomplete}"`,
    f.value !== undefined ? `value="${esc(f.value)}"` : "",
    f.required === false ? "" : "required",
    f.minlength ? `minlength="${f.minlength}"` : "",
    f.maxlength ? `maxlength="${f.maxlength}"` : "",
    f.type === "email" ? 'autocapitalize="none" spellcheck="false"' : "",
    ids.length ? `aria-describedby="${ids.join(" ")}"` : "",
    f.error || f.errorRef ? 'aria-invalid="true"' : "",
  ].filter(Boolean);
  return `<div class="field"><label for="${f.id}">${esc(f.label)}</label>${extra}<input ${attrs.join(" ")}></div>`;
}

export const hidden = (name: string, value: string): string => `<input type="hidden" name="${name}" value="${esc(value)}">`;

// usernameFor gives password managers the account a new password belongs to.
export const usernameFor = (email: string): string =>
  `<input type="email" name="username" autocomplete="username" value="${esc(email)}" hidden readonly>`;

// signedIn returns the session behind the page's access cookie, or the
// redirect that picks the session up through the refresh cookie (resume.ts),
// which sends the browser to the login page when there is none.
export async function signedIn(ctx: Ctx): Promise<Who | Response> {
  const who = await whoAmI(ctx, false);
  if (who) return who;
  const url = new URL(ctx.request.url);
  return redirect(`/api/auth/resume?next=${encodeURIComponent(url.pathname + url.search)}`);
}

// rules are the numbers the messages quote.
export const rules = (ctx: Ctx): Rules => ({ min: ctx.s.passwordMin, max: ctx.s.passwordMax, minutes: resetSeconds(ctx.env) / 60 });
