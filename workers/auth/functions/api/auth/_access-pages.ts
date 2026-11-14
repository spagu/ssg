// The Cloudflare Access parts of the pages (GO-139). A leading underscore
// keeps this file out of the Pages route table.
//
//   login page    "Sign in with Cloudflare Access" — a plain LINK to
//                 /auth/access?next=…, so it works without JavaScript and needs
//                 no form-action: Access, in front of that path, signs the
//                 person in at the identity provider and lets the request
//                 through with its token.
//   account page  "Connect Cloudflare Access" — a link to /auth/access/link,
//                 which shows who Access says you are and asks to confirm (a
//                 form POST: a GET never links anything).

import { ACCESS_STRINGS } from "./_access-i18n";
import type { Who } from "./_context";
import type { Ctx } from "./_context";
import { fill, langOf } from "./_i18n";
import type { Lang } from "./_i18n";
import { esc, page } from "./_pages";
import { pageCsp } from "./_pages-csp";

export const ACCESS_ENTRY = "/auth/access";
export const ACCESS_LINK = "/auth/access/link";

// accessButton: the login page's entry (`next` is already a safe path or "").
export function accessButton(lang: Lang, next: string): string {
  const href = next ? `${ACCESS_ENTRY}?next=${encodeURIComponent(next)}` : ACCESS_ENTRY;
  return `<ul class="providers"><li><a class="btn btn-provider" href="${esc(href)}" data-provider="access">${esc(ACCESS_STRINGS[lang].signIn)}</a></li></ul>`;
}

// accessLinkButton: the account page's "Connect Cloudflare Access".
export const accessLinkButton = (lang: Lang): string =>
  `<ul class="providers"><li><a class="btn btn-provider" href="${ACCESS_LINK}" data-provider="access">${esc(ACCESS_STRINGS[lang].connectButton)}</a></li></ul>`;

// confirmPage: "Cloudflare Access says you are …; connect it to this account?"
export function confirmPage(ctx: Ctx, who: Who, accessEmail: string, formOrigins: string[]): Response {
  const lang = langOf(ctx.request);
  const a = ACCESS_STRINGS[lang];
  const site = new URL(ctx.request.url).host;
  const body = `<p class="lead">${esc(fill(a.confirmLead, { email: accessEmail || "—", account: who.user.email }))}</p>
<form class="stack" method="post" action="${ACCESS_LINK}"><button class="btn btn-primary" type="submit">${esc(a.confirmButton)}</button></form>
<p><a href="/auth/account">${esc(a.cancel)}</a></p>`;
  return page({
    request: ctx.request,
    lang,
    title: a.confirmTitle,
    description: fill(a.confirmDescription, { site }),
    body,
    account: { email: who.user.email },
    csp: pageCsp({ formAction: formOrigins }),
  });
}
