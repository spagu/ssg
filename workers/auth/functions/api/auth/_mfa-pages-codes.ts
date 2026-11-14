// The backup-codes page (GO-140): the answer to the plain form that made them
// (a first factor confirmed, or "New backup codes"). A leading underscore
// keeps this file out of the Pages route table.
//
// It is the response to that POST itself, never a page to come back to: the
// codes are not stored (only their hashes), so they cannot be shown again,
// and they never travel in a URL. The page is no-store like every other.

import type { Ctx, Who } from "./_context";
import { STRINGS, fill, langOf } from "./_i18n";
import { esc, page } from "./_pages";

export function codesPage(ctx: Ctx, who: Who, codes: string[], next: string, cookies: string[] = []): Response {
  const lang = langOf(ctx.request);
  const t = STRINGS[lang];
  const site = new URL(ctx.request.url).host;
  const body = [
    `<div class="callout warning" role="status"><p class="callout-title">${esc(t.mfaCodesOnce)}</p><p>${esc(t.mfaCodesLead)}</p></div>`,
    `<section class="card" aria-labelledby="codes-title"><h2 id="codes-title">${esc(t.mfaCodesListLabel)}</h2>
<ol class="codes">${codes.map((c) => `<li><code>${esc(c)}</code></li>`).join("")}</ol><p class="hint">${esc(t.mfaCodesOld)}</p></section>`,
    `<p><a class="btn btn-primary" href="${esc(next)}">${esc(t.mfaCodesContinue)}</a></p>`,
  ].join("\n");
  return page({ request: ctx.request, lang, title: t.mfaCodesTitle, description: fill(t.mfaCodesDescription, { site }), body, account: { email: who.user.email }, cookies });
}
