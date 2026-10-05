// Cloudflare Pages Function: newsletter / waitlist sign-up.
//   POST /api/newsletter   JSON or a plain HTML form post
//
// Fields: email (required), consent + consent_text (required), language,
// source_page, tags (a list or comma-separated), and the Turnstile token as
// `cf-turnstile-response` (what the widget injects into a form) or `token`.
//
// Every accepted sign-up gets the same answer, whether the address is new,
// pending, confirmed or previously unsubscribed, so the endpoint cannot be used
// to find out who is on the list. A form post is answered with a 303 to the
// thank-you URL; a JSON call with JSON.

import { Env, json, saltedHash, turnstileSecret, verifyTurnstile, doubleOptIn } from "./_lib";
import { readBody, text, normaliseEmail, normaliseLang, normaliseTags, normalisePath, isConsent, thanksURL } from "./_input";
import { ensureSchema } from "./_schema";
import { subscribe, purgeExpired } from "./_store";
import { sendConfirmation } from "./_mail";
import { STRINGS, pickLang } from "./_i18n";
import { message } from "./_pages";

type ErrorCode = "invalid_body" | "invalid_email" | "consent_required" | "captcha_failed" | "not_configured";

const STATUS: Record<ErrorCode, number> = {
  invalid_body: 400,
  invalid_email: 422,
  consent_required: 422,
  captcha_failed: 403,
  not_configured: 503,
};

export const onRequestPost: PagesFunction<Env> = async ({ request, env, waitUntil }) => {
  const body = await readBody(request);
  const fields = body?.fields ?? {};
  const language = normaliseLang(fields.language);

  // Errors speak the caller's language: a stable code for JSON (newsletter.js
  // localises it), a small page for a form post, which has no script to do so.
  const fail = (code: ErrorCode): Response => {
    if (!body?.isForm) return json({ error: code }, STATUS[code]);
    const lang = pickLang(request, language);
    const t = STRINGS[lang];
    return message(lang, t.errorTitle, code === "invalid_body" ? t.not_configured : t[code], STATUS[code]);
  };

  if (!body) return fail("invalid_body");
  const secret = turnstileSecret(env);
  if (!env.NEWSLETTER_DB || !secret) return fail("not_configured");

  const email = normaliseEmail(fields.email);
  if (!email) return fail("invalid_email");
  // Consent is the flag AND the words it was given to: a record that says
  // "agreed" without saying to what proves nothing.
  const consentText = text(fields.consent_text, 1000);
  if (!isConsent(fields.consent) || !consentText) return fail("consent_required");

  const ip = request.headers.get("cf-connecting-ip");
  const token = text(fields["cf-turnstile-response"], 2048) || text(fields.token, 2048);
  if (!token || !(await verifyTurnstile(secret, token, ip))) return fail("captcha_failed");

  await ensureSchema(env);
  const sourcePage = normalisePath(fields.source_page);
  const doi = doubleOptIn(env);
  const { sendConfirmation: send, subscriber } = await subscribe(
    env,
    {
      email,
      language,
      sourcePage,
      tags: normaliseTags(fields.tags),
      consentText,
      ipHash: await saltedHash(env, ip),
      uaHash: await saltedHash(env, request.headers.get("user-agent")),
    },
    doi,
  );

  if (send) {
    const confirmURL = `${new URL(request.url).origin}/api/newsletter/confirm?token=${subscriber.token}`;
    waitUntil(sendConfirmation(env, email, pickLang(request, language), confirmURL));
  }
  waitUntil(purgeExpired(env).then(() => undefined));

  if (body.isForm) {
    return new Response(null, {
      status: 303,
      headers: { location: thanksURL(env.NEWSLETTER_THANKS_URL, language, sourcePage), "cache-control": "no-store" },
    });
  }
  // `confirm` reflects configuration, not this address: it tells the form to
  // say "check your inbox" rather than "you're subscribed".
  return json({ ok: true, confirm: doi });
};
