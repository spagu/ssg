// GET /auth/admin/ — the accounts admin panel's shell (GO-138). A leading
// underscore keeps this file out of the Pages route table;
// functions/auth/admin/index.ts serves it.
//
// The shell is rendered here, not served as a static file, for three reasons:
// it needs a session (no session → through /api/auth/resume to the login
// page, like every page under /auth/); it refuses an account holding neither
// auth:admin nor auth:invite before any script runs; and it hands the script
// what the server already knows — the account, the token's permissions,
// which features are configured — as a JSON data block (data, never
// executed, so the CSP needs no exception). The views are ES modules under
// /auth/assets/admin/ (public/auth/assets/admin/), outside the Functions
// (routes_exclude /auth/assets/*).
//
// CSP: the pages' own (_pages.ts) plus connect-src 'self' for the API, and
// the Access team's origin in form-action when Access is on (sign-out goes on
// to its logout). Same-origin covers core's shared bar and theme.

import { accessLookup } from "./_access-config";
import { ADMIN_STRINGS } from "./_admin-i18n";
import { can } from "./_can";
import type { Ctx } from "./_context";
import { fill, langOf } from "./_i18n";
import type { AccessClaims } from "./_jwt";
import { secure } from "./_lib";
import { mailState } from "./_mail";
import { mfaState } from "./_mfa-crypto";
import { stepUpMinutes } from "./_mfa-policy";
import { STEP_UP_PAGE } from "./_mfa-signin";
import { ASSETS, PAGE_REFERRER, esc, page, signedIn } from "./_pages";
import { pageCsp } from "./_pages-csp";
import { signInMethods } from "./_provider-methods";

export const ADMIN_ASSETS = `${ASSETS}/admin`;
export const PANEL_PERMISSIONS = ["auth:admin", "auth:invite"];

// dataBlock: JSON safe inside <script type="application/json"> — no "<" that
// could close the element, no line separators.
export const dataBlock = (v: unknown): string =>
  JSON.stringify(v).replace(/[<>&\u2028\u2029]/g, (c) => `\\u${c.charCodeAt(0).toString(16).padStart(4, "0")}`);

// adminShell answers the panel, the refusal page, or the redirect to sign in.
export async function adminShell(ctx: Ctx): Promise<Response> {
  const who = await signedIn(ctx);
  if (who instanceof Response) return who;
  const lang = langOf(ctx.request);
  const w = ADMIN_STRINGS[lang];
  const perms = (who.claims as AccessClaims).perms;
  const csp = pageCsp({ formAction: signInMethods(ctx.env).flatMap((m) => (m.formOrigin ? [m.formOrigin] : [])) });
  if (!PANEL_PERMISSIONS.some((p) => can(perms, p))) {
    const body = `<p>${esc(fill(w.forbiddenText, { email: who.user.email }))}</p><p><a class="btn btn-primary" href="/auth/">${esc(w.backToPanels)}</a></p>`;
    return page({ request: ctx.request, lang, title: w.forbiddenTitle, description: w.forbiddenTitle, body, status: 403, account: { email: who.user.email }, csp });
  }
  const site = new URL(ctx.request.url).host;
  const config = {
    lang,
    user: { id: who.user.id, email: who.user.email, name: who.user.display_name },
    perms,
    mfa: mfaState(ctx.env),
    stepUpMinutes: stepUpMinutes(ctx.env),
    stepUpUrl: STEP_UP_PAGE,
    mail: mailState(ctx.env).kind,
    access: accessLookup(ctx.env).kind,
    tokenMinutes: ctx.s.tokenSeconds / 60,
    loginUrl: ctx.s.loginUrl,
  };
  const doc = `<!doctype html>
<html lang="${lang}">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>${esc(w.title)}</title>
<meta name="description" content="${esc(fill(w.description, { site }))}">
<meta name="robots" content="noindex, nofollow, noarchive">
<meta name="referrer" content="strict-origin">
<meta name="color-scheme" content="light dark">
<meta name="theme-color" content="#efebe9" media="(prefers-color-scheme: light)">
<meta name="theme-color" content="#121212" media="(prefers-color-scheme: dark)">
<meta name="format-detection" content="telephone=no">
<meta name="generator" content="ssg auth">
<link rel="icon" href="${ASSETS}/icon.svg" type="image/svg+xml">
<link rel="stylesheet" href="${ASSETS}/tokens.css">
<link rel="stylesheet" href="/core/theme.css">
<link rel="stylesheet" href="${ASSETS}/auth.css">
<link rel="stylesheet" href="${ADMIN_ASSETS}/css/admin.css">
<link rel="stylesheet" href="${ADMIN_ASSETS}/css/views.css">
<script id="auth-admin-config" type="application/json">${dataBlock(config)}</script>
<script type="module" src="${ADMIN_ASSETS}/js/main.js"></script>
</head>
<body class="admin">
<a class="skip" id="skip" href="#main">${esc(w.skip)}</a>
<script type="module" src="/core/nav.js"></script><ssg-admin-nav current="auth"></ssg-admin-nav>
<div class="shell">
<header class="sidebar" id="sidebar">
<div class="side-head"><a class="brand" href="#/"><img src="${ASSETS}/icon.svg" alt="" width="28" height="28"><span class="brand-name">${esc(w.title)}</span></a>
<button type="button" class="btn" id="menu-toggle" aria-expanded="false" aria-controls="drawer">${esc(w.menu)}</button></div>
<div class="drawer" id="drawer">
<nav id="nav" aria-label="${esc(w.sections)}"><ul id="nav-list"></ul></nav>
<section class="prefs" id="prefs" aria-label="${esc(w.preferences)}"></section>
<form class="account" method="post" action="/api/auth/logout"><p>${esc(fill(w.signedInAs, { email: who.user.email }))}</p><input type="hidden" name="next" value="/auth/login?ok=signed_out"><button class="btn btn-quiet" type="submit">${esc(w.signOut)}</button></form>
</div>
</header>
<main id="main" tabindex="-1">
<div id="notice" role="status" aria-live="polite"></div>
<div id="view"><noscript><p>${esc(w.noScript)}</p></noscript></div>
</main>
</div>
</body>
</html>`;
  return secure(new Response(doc, { headers: { "content-type": "text/html; charset=utf-8" } }), {
    "content-security-policy": `${csp}; connect-src 'self'`,
    "referrer-policy": PAGE_REFERRER,
  });
}
