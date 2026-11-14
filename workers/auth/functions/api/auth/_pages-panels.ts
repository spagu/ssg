// GET /auth/ — "Your panels": where the signed-in account may go. A leading
// underscore keeps this file out of the Pages route table;
// functions/auth/index.ts serves it. The list comes from the access token's
// permissions and the registry (_panels.ts). The future `core` worker, which
// knows every module's panel from the build, supersedes this page.

import type { Ctx } from "./_context";
import { STRINGS, fill, langOf } from "./_i18n";
import type { Strings } from "./_i18n";
import type { AccessClaims } from "./_jwt";
import { panelsFor } from "./_panels";
import type { PanelModule } from "./_panels";
import { esc, notice, page, signedIn } from "./_pages";
import { pageCsp } from "./_pages-csp";
import { signInMethods } from "./_provider-methods";
import { policyNotice } from "./_provider-pages";
import { sessionMethod } from "./_provider-policy";

function moduleCard(m: PanelModule, t: Strings): string {
  const link = m.panel ? `<p><a class="btn btn-primary" href="${esc(m.panel)}">${esc(fill(t.panelsOpen, { title: m.title }))}</a></p>` : `<p class="muted">${esc(t.panelsNoAddress)}</p>`;
  const items = m.permissions.map((p) => `<li><code>${esc(p.name)}</code>${p.description ? ` — ${esc(p.description)}` : ""}</li>`);
  if (m.wildcard && m.wildcard !== "*") items.unshift(`<li>${esc(fill(t.panelsWildcard, { perm: m.wildcard }))}</li>`);
  // Never empty: a module is listed because the account holds something in it.
  return `<li class="card"><h2>${esc(m.title)}</h2>${link}<ul class="perms">${items.join("")}</ul></li>`;
}

export async function panelsPage(ctx: Ctx): Promise<Response> {
  const who = await signedIn(ctx);
  if (who instanceof Response) return who;
  const lang = langOf(ctx.request);
  const t = STRINGS[lang];
  const q = new URL(ctx.request.url).searchParams;
  const site = new URL(ctx.request.url).host;
  // A page reached through the access token always has its claims.
  const perms = (who.claims as AccessClaims).perms;
  const { everything, modules } = panelsFor(ctx.env, perms, lang);
  const skipped = Number.parseInt(q.get("skipped") ?? "", 10);
  const body = [
    q.get("welcome") === "1" ? notice("ok", t.doneHeading, t.welcome) : "",
    skipped > 0 ? `<div class="callout warning"><p>${esc(fill(t.skipped, { count: skipped }))}</p></div>` : "",
    // GO-151: roles whose permissions this session's sign-in method leaves out.
    await policyNotice(ctx, who, t, await sessionMethod(ctx.env, who.session.id)),
    `<p class="lead">${esc(fill(t.panelsLead, { email: who.user.email }))}</p>`,
    everything ? `<p>${esc(t.panelsEverything)}</p>` : "",
    modules.length ? `<ul class="cards">${modules.map((m) => moduleCard(m, t)).join("")}</ul>` : `<p>${esc(t.panelsNone)}</p>`,
    `<p>${esc(t.panelsAccount)} <a href="/auth/account">${esc(t.accountTitle)}</a></p>`,
  ].join("\n");
  // The header's sign-out form: an Access session is sent on to the team's
  // logout (GO-139), which form-action must allow; exactly PAGE_CSP otherwise.
  const csp = pageCsp({ formAction: signInMethods(ctx.env).flatMap((m) => (m.formOrigin ? [m.formOrigin] : [])) });
  return page({ request: ctx.request, lang, title: t.panelsTitle, description: fill(t.panelsDescription, { site }), body, account: { email: who.user.email }, csp });
}
