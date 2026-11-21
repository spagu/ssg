// The external JWT issuer's parts of the pages (#435). A leading underscore
// keeps this file out of the Pages route table.
//
//   account page  "Connect <issuer>" — a plain LINK to AUTH_JWT_LINK_URL, the
//                 issuer's own page, which signs a token for the person and
//                 posts it to /api/auth/jwt/link. Shown only when that URL is
//                 set and the account has no identity of this issuer yet.
//   /auth/jwt/link  "<issuer> says you are …; connect it to this account?" —
//                 a form POST on this origin: the token's arrival never links
//                 anything by itself (_external-link.ts).

import type { Ctx, Who } from "./_context";
import { EXTERNAL_STRINGS } from "./_external-i18n";
import { fill, langOf } from "./_i18n";
import type { Lang } from "./_i18n";
import { esc, page } from "./_pages";
import { pageCsp } from "./_pages-csp";
import type { Identity } from "./_provider";

export const JWT_LINK_PAGE = "/auth/jwt/link";
export const JWT_LINK_ARRIVAL = "/api/auth/jwt/link";

// externalLinkButton: the account page's "Connect <issuer>".
export const externalLinkButton = (lang: Lang, issuer: string, linkUrl: string): string =>
  `<ul class="providers"><li><a class="btn btn-provider" href="${esc(linkUrl)}" data-provider="jwt">${esc(fill(EXTERNAL_STRINGS[lang].connectButton, { issuer }))}</a></li></ul>`;

// externalConfirmPage asks the signed-in person to confirm the identity.
export function externalConfirmPage(ctx: Ctx, who: Who, issuer: string, id: Identity): Response {
  const lang = langOf(ctx.request);
  const x = EXTERNAL_STRINGS[lang];
  const site = new URL(ctx.request.url).host;
  const body = `<p class="lead">${esc(fill(x.confirmLead, { issuer, email: id.email || id.subject, account: who.user.email }))}</p>
<form class="stack" method="post" action="${JWT_LINK_PAGE}"><button class="btn btn-primary" type="submit">${esc(x.confirmButton)}</button></form>
<p><a href="/auth/account">${esc(x.cancel)}</a></p>`;
  return page({
    request: ctx.request,
    lang,
    title: fill(x.confirmTitle, { issuer }),
    description: fill(x.confirmDescription, { issuer, site }),
    body,
    account: { email: who.user.email },
    csp: pageCsp(),
  });
}
