// Cloudflare Pages Function: double opt-in confirmation.
//   GET  /api/newsletter/confirm?token=…  a localised page with one button
//   POST /api/newsletter/confirm?token=…  pending → confirmed
//
// The link in the confirmation mail is the GET. Confirming takes the extra
// click on purpose: corporate mail scanners and link previewers fetch every URL
// in a message, and a GET that confirmed would complete a sign-up somebody
// else made for this address — the very thing double opt-in exists to prevent.

import { Env, isToken } from "./_lib";
import { ensureSchema } from "./_schema";
import { findBy, confirm, purgeExpired } from "./_store";
import { STRINGS, pickLang } from "./_i18n";
import { action, message } from "./_pages";

// lookup resolves the token and the language its page should speak.
async function lookup(request: Request, env: Env) {
  const url = new URL(request.url);
  const token = url.searchParams.get("token");
  const row = isToken(token) ? await findBy(env, "token", token) : null;
  const lang = pickLang(request, row?.language);
  return { url, token, row, lang, t: STRINGS[lang] };
}

export const onRequestGet: PagesFunction<Env> = async ({ request, env }) => {
  if (!env.NEWSLETTER_DB) {
    const lang = pickLang(request);
    return message(lang, STRINGS[lang].confirmTitle, STRINGS[lang].not_configured, 503);
  }
  await ensureSchema(env);
  const { url, token, row, lang, t } = await lookup(request, env);
  if (row?.status === "confirmed") return message(lang, t.confirmTitle, t.confirmDone);
  // Unknown, purged, or unsubscribed since: an old confirmation link must not
  // quietly re-subscribe someone who has left.
  if (!row || row.status !== "pending") return message(lang, t.confirmTitle, t.invalid, 404);
  return action(lang, t.confirmTitle, t.confirmAsk, t.confirmButton, `${url.pathname}?token=${token}`);
};

export const onRequestPost: PagesFunction<Env> = async ({ request, env, waitUntil }) => {
  if (!env.NEWSLETTER_DB) {
    const lang = pickLang(request);
    return message(lang, STRINGS[lang].confirmTitle, STRINGS[lang].not_configured, 503);
  }
  await ensureSchema(env);
  const { row, lang, t } = await lookup(request, env);
  if (row?.status === "confirmed") return message(lang, t.confirmTitle, t.confirmDone);
  if (!row || !(await confirm(env, row.token))) return message(lang, t.confirmTitle, t.invalid, 404);
  waitUntil(purgeExpired(env).then(() => undefined));
  return message(lang, t.confirmTitle, t.confirmDone);
};
