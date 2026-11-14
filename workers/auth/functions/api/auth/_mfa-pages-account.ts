// "Second factor" on the account page (GO-140): the account's factors, their
// removal, the backup codes left and a new set, and the way to add one. A
// leading underscore keeps this file out of the Pages route table;
// _pages-account.ts places it. Every action is a plain form; removing and
// regenerating need a recent second factor, and the routes send a form to the
// step-up page when it is not (which comes back here).

import type { Ctx, Who } from "./_context";
import { fill, when } from "./_i18n";
import type { Lang, Strings } from "./_i18n";
import { mfaState } from "./_mfa-crypto";
import { backupCodesLeft, listFactors } from "./_mfa-factors";
import type { FactorRow } from "./_mfa-factors";
import { kindLabel } from "./_mfa-pages-parts";
import { stepUpMinutes } from "./_mfa-policy";
import { esc, hidden } from "./_pages";

const BACK = "/auth/account";

function row(f: FactorRow, lang: Lang, t: Strings): string {
  const label = f.name ? `${kindLabel(t, f.kind)} — ${f.name}` : kindLabel(t, f.kind);
  const cell = (heading: string, html: string) => `<td data-label="${esc(heading)}">${html}</td>`;
  const remove = `<form method="post" action="/api/auth/mfa/factors/${esc(f.id)}">${hidden("next", BACK)}<button class="btn" type="submit" aria-label="${esc(fill(t.mfaRemoveLabel, { name: label }))}">${esc(t.removeMethod)}</button></form>`;
  return `<tr>${cell(t.mfaColFactor, esc(label))}${cell(t.mfaColAdded, esc(when(lang, f.confirmed_at as string)))}${cell(t.colLastUsed, esc(f.last_used_at ? when(lang, f.last_used_at) : t.mfaNever))}<td>${remove}</td></tr>`;
}

export async function factorsSection(ctx: Ctx, who: Who, lang: Lang, t: Strings): Promise<string> {
  const state = mfaState(ctx.env);
  if (state === "off") return "";
  const factors = await listFactors(ctx.env, who.user.id);
  const parts = [`<section class="card" aria-labelledby="mfa-title"><h2 id="mfa-title">${esc(t.mfaAccountHeading)}</h2><p>${esc(t.mfaAccountLead)}</p>`];
  if (state === "misconfigured") parts.push(`<div class="callout warning"><p>${esc(t.errMfaUnavailable)}</p></div>`);
  if (factors.length) {
    const head = [t.mfaColFactor, t.mfaColAdded, t.colLastUsed, t.colAction].map((h) => `<th scope="col">${esc(h)}</th>`).join("");
    parts.push(`<div class="table-scroll"><table class="data"><thead><tr>${head}</tr></thead><tbody>${factors.map((f) => row(f, lang, t)).join("")}</tbody></table></div>`);
    parts.push(`<p>${esc(fill(t.mfaCodesLeft, { count: await backupCodesLeft(ctx.env, who.user.id) }))}</p>
<form class="inline-actions" method="post" action="/api/auth/mfa/backup-codes">${hidden("next", BACK)}<button class="btn" type="submit" aria-describedby="regen-hint">${esc(t.mfaRegenerate)}</button></form><p class="hint" id="regen-hint">${esc(t.mfaRegenerateHint)}</p>`);
  } else {
    parts.push(`<p>${esc(t.mfaAccountNone)}</p>`);
  }
  parts.push(`<p><a class="btn btn-primary" href="/auth/mfa/enrol?next=${encodeURIComponent(BACK)}">${esc(t.mfaAdd)}</a></p>`);
  if (factors.length) parts.push(`<p class="hint">${esc(fill(t.mfaStepUpNote, { minutes: stepUpMinutes(ctx.env) }))}</p>`);
  return parts.join("\n") + "</section>";
}
