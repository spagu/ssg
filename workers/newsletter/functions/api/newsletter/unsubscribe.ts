// Cloudflare Pages Function: unsubscribe.
//   GET  /api/newsletter/unsubscribe?token=…  a localised page with one button
//   POST /api/newsletter/unsubscribe?token=…  unsubscribes
//
// The POST is RFC 8058 one-click compatible. Put the link in every mail as
//   List-Unsubscribe: <https://example.com/api/newsletter/unsubscribe?token=…>
//   List-Unsubscribe-Post: List-Unsubscribe=One-Click
// and the mailbox provider POSTs `List-Unsubscribe=One-Click` to that URL when
// the reader clicks its own Unsubscribe button. That POST gets a bare 200 — no
// page, nobody is looking at it. The button on the GET page posts the same URL
// and gets a localised confirmation page back.
//
// The GET never unsubscribes by itself: mail scanners fetch every link in a
// message, and a GET that acted would unsubscribe readers who clicked nothing.

import { Env, isToken } from "./_lib";
import { ensureSchema } from "./_schema";
import { findBy, unsubscribe, purgeExpired } from "./_store";
import { STRINGS, pickLang } from "./_i18n";
import { action, message } from "./_pages";

const plain = (body: string, status: number): Response =>
  new Response(body, { status, headers: { "content-type": "text/plain; charset=utf-8", "cache-control": "no-store" } });

const notConfigured = (request: Request): Response => {
  const lang = pickLang(request);
  return message(lang, STRINGS[lang].unsubTitle, STRINGS[lang].not_configured, 503);
};

export const onRequestGet: PagesFunction<Env> = async ({ request, env }) => {
  if (!env.NEWSLETTER_DB) return notConfigured(request);
  await ensureSchema(env);
  const url = new URL(request.url);
  const token = url.searchParams.get("token");
  const row = isToken(token) ? await findBy(env, "token", token) : null;
  const lang = pickLang(request, row?.language);
  const t = STRINGS[lang];
  if (!row) return message(lang, t.unsubTitle, t.invalid, 404);
  if (row.status === "unsubscribed") return message(lang, t.unsubTitle, t.unsubDone);
  return action(lang, t.unsubTitle, t.unsubAsk, t.unsubButton, `${url.pathname}?token=${token}`);
};

export const onRequestPost: PagesFunction<Env> = async ({ request, env, waitUntil }) => {
  // RFC 8058: the body is List-Unsubscribe=One-Click, urlencoded or multipart.
  let oneClick = false;
  try {
    const form = await request.formData();
    oneClick = form.get("List-Unsubscribe") === "One-Click";
  } catch {
    /* no body (the page's button) or an unparseable one: not one-click */
  }
  if (!env.NEWSLETTER_DB) return oneClick ? plain("not configured", 503) : notConfigured(request);
  await ensureSchema(env);

  const token = new URL(request.url).searchParams.get("token");
  const row = isToken(token) ? await findBy(env, "token", token) : null;
  if (row) {
    await unsubscribe(env, row.token);
    waitUntil(purgeExpired(env).then(() => undefined));
  }

  if (oneClick) return row ? plain("unsubscribed", 200) : plain("unknown token", 404);
  const lang = pickLang(request, row?.language);
  const t = STRINGS[lang];
  return row ? message(lang, t.unsubTitle, t.unsubDone) : message(lang, t.unsubTitle, t.invalid, 404);
};
