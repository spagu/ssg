// The scenarios of the backward-compatibility suite: every endpoint of this
// template, every answer it gives, exercised as a site and a mail provider
// would see it. SCENARIOS must answer exactly as the 1.9.7 release did
// (compat-golden.json). CHANGED holds the two deliberate changes of 1.9.8 —
// SMTP as a third transport, and mail settings that can never send answering
// `not_configured` — and the suite asserts their new behaviour separately.
import { ADMIN, withWorld } from "./compat-harness.ts";
import type { Handlers, World } from "./compat-harness.ts";

type Scenario = (h: Handlers, w: World) => Promise<void>;

const MIN = 60_000;
const DAY = 24 * 60 * MIN;
const JSONH = { "content-type": "application/json", "cf-connecting-ip": "192.0.2.10", "user-agent": "compat/1", "accept-language": "en" };
const FORMH = { "content-type": "application/x-www-form-urlencoded", "cf-connecting-ip": "192.0.2.10", "accept-language": "pl,en;q=0.5" };
const OK = { email: "Reader@Example.com", consent: true, consent_text: "Send me the newsletter", token: "tok", language: "en" };

const signup = (h: Handlers, w: World, step: string, body: unknown, env?: Record<string, unknown>) =>
  w.call(h.signup.onRequestPost, step, { method: "POST", path: "/api/newsletter", headers: JSONH, body: JSON.stringify(body), env });
const form = (h: Handlers, w: World, step: string, fields: Record<string, string>, env?: Record<string, unknown>) =>
  w.call(h.signup.onRequestPost, step, { method: "POST", path: "/api/newsletter", headers: FORMH, body: new URLSearchParams(fields).toString(), env });
const confirmGet = (h: Handlers, w: World, step: string, token: string, env?: Record<string, unknown>) =>
  w.call(h.confirm.onRequestGet, step, { path: `/api/newsletter/confirm?token=${token}`, headers: { "accept-language": "en" }, env });
const confirmPost = (h: Handlers, w: World, step: string, token: string, env?: Record<string, unknown>) =>
  w.call(h.confirm.onRequestPost, step, { method: "POST", path: `/api/newsletter/confirm?token=${token}`, headers: { "accept-language": "en" }, env });
const unsubGet = (h: Handlers, w: World, step: string, token: string, env?: Record<string, unknown>) =>
  w.call(h.unsubscribe.onRequestGet, step, { path: `/api/newsletter/unsubscribe?token=${token}`, headers: { "accept-language": "pl" }, env });
const unsubPost = (h: Handlers, w: World, step: string, token: string, oneClick: boolean, env?: Record<string, unknown>) =>
  w.call(h.unsubscribe.onRequestPost, step, {
    method: "POST",
    path: `/api/newsletter/unsubscribe?token=${token}`,
    headers: oneClick ? { "content-type": "application/x-www-form-urlencoded" } : { "accept-language": "en" },
    body: oneClick ? "List-Unsubscribe=One-Click" : undefined,
    env,
  });
const admin = (h: Handlers, w: World, step: string, query = "", auth: string | null = ADMIN, env?: Record<string, unknown>) =>
  w.call(h.admin.onRequestGet, step, { path: `/api/newsletter/admin${query}`, headers: auth ? { authorization: auth } : {}, env });
const adminPost = (h: Handlers, w: World, step: string, body: string, env?: Record<string, unknown>) =>
  w.call(h.admin.onRequestPost, step, { method: "POST", path: "/api/newsletter/admin", headers: { authorization: ADMIN, "content-type": "application/json" }, body, env });

const UNKNOWN = "A".repeat(43);
const RESEND = { NEWSLETTER_MAIL_FROM: "news@example.com", NEWSLETTER_RESEND_API_KEY: "re_test" };
const MAILCHANNELS = { NEWSLETTER_MAIL_FROM: "news@example.com", NEWSLETTER_MAILCHANNELS_API_KEY: "mc_test" };

export const SCENARIOS: Record<string, Scenario> = {
  async "sign-up errors (JSON)"(h, w) {
    await w.call(h.signup.onRequestPost, "array body", { method: "POST", path: "/api/newsletter", headers: JSONH, body: "[]" });
    await w.call(h.signup.onRequestPost, "malformed JSON", { method: "POST", path: "/api/newsletter", headers: JSONH, body: "{" });
    await w.call(h.signup.onRequestPost, "text/plain", { method: "POST", path: "/api/newsletter", headers: { "content-type": "text/plain" }, body: "x" });
    await signup(h, w, "no Turnstile secret", OK, { NEWSLETTER_TURNSTILE_SECRET: undefined });
    await signup(h, w, "no database", OK, { NEWSLETTER_DB: undefined });
    await signup(h, w, "invalid email", { ...OK, email: "not-an-address" });
    await signup(h, w, "header smuggling", { ...OK, email: "a@example.com\r\nbcc: b@example.com" });
    await signup(h, w, "no consent", { ...OK, consent: false });
    await signup(h, w, "consent without its text", { ...OK, consent_text: " " });
    await signup(h, w, "no captcha token", { ...OK, token: "" });
    await signup(h, w, "captcha refused", { ...OK, token: "bad" });
  },
  async "sign-up errors (form, localised page)"(h, w) {
    const f = { email: "reader@example.com", consent: "on", consent_text: "Yes", "cf-turnstile-response": "tok" };
    await form(h, w, "no Turnstile secret", f, { NEWSLETTER_TURNSTILE_SECRET: undefined });
    await form(h, w, "invalid email", { ...f, email: "x" });
    await form(h, w, "no consent", { ...f, consent: "" });
    await form(h, w, "captcha refused", { ...f, "cf-turnstile-response": "bad" });
    await form(h, w, "language field wins", { ...f, email: "x", language: "hi" });
  },
  async "single opt-in"(h, w) {
    await signup(h, w, "new (JSON)", { ...OK, tags: ["news", "Launch "], source_page: "/en/post/?utm=1#x" });
    await signup(h, w, "confirmed again: unchanged", { ...OK, language: "pl", tags: "other" });
    await signup(h, w, "shared TURNSTILE_SECRET", { ...OK, email: "second@example.com" }, { NEWSLETTER_TURNSTILE_SECRET: undefined, TURNSTILE_SECRET: "shared" });
    await form(h, w, "form → 303 default", { email: "form@example.com", consent: "1", consent_text: "Yes", "cf-turnstile-response": "t", tags: "a" });
    await form(h, w, "form → 303 {lang}", { email: "lang@example.com", consent: "yes", consent_text: "Tak", token: "t", language: "pl" }, { NEWSLETTER_THANKS_URL: "/{lang}/thanks/" });
    await form(h, w, "form → 303 map", { email: "map@example.com", consent: "true", consent_text: "Yes", token: "t", language: "de", source_page: "/de/" }, { NEWSLETTER_THANKS_URL: '{"pl":"/pl/dziekujemy/","default":"/thanks/"}' });
    await signup(h, w, "salted hashes", { ...OK, email: "salted@example.com" }, { NEWSLETTER_HASH_SALT: "pepper" });
  },
  async "double opt-in via Resend"(h, w) {
    Object.assign(w.env, RESEND, { NEWSLETTER_MAIL_FROM_NAME: "Example News" });
    await signup(h, w, "new → pending, mail", OK);
    await signup(h, w, "pending again within 15 min: no mail", OK);
    w.advance(16 * MIN);
    await signup(h, w, "pending after 16 min: mail again", { ...OK, language: "pl" });
    const token = w.tokenOf("reader@example.com");
    await confirmGet(h, w, "confirm page", token);
    await confirmPost(h, w, "confirm", token);
    await confirmGet(h, w, "confirm page after", token);
    await confirmPost(h, w, "confirm again", token);
    await signup(h, w, "confirmed again: nothing", OK);
    await unsubPost(h, w, "one-click", token, true);
    await confirmGet(h, w, "old confirm link after unsubscribe", token);
    await signup(h, w, "unsubscribed → pending, mail", OK);
  },
  async "double opt-in via MailChannels"(h, w) {
    Object.assign(w.env, MAILCHANNELS);
    w.providerStatus = 202;
    await signup(h, w, "no sender name", OK);
    await signup(h, w, "with sender name", { ...OK, email: "named@example.com" }, { NEWSLETTER_MAIL_FROM_NAME: "Example News" });
  },
  async "provider refuses or is unreachable"(h, w) {
    Object.assign(w.env, RESEND);
    w.providerStatus = 500;
    await signup(h, w, "Resend 500: still ok, nothing marked", OK);
    w.providerThrows = true;
    await signup(h, w, "network error", { ...OK, email: "other@example.com" });
  },
  async "confirm endpoint"(h, w) {
    Object.assign(w.env, RESEND);
    await signup(h, w, "pending", OK);
    await confirmGet(h, w, "GET no database", UNKNOWN, { NEWSLETTER_DB: undefined });
    await confirmPost(h, w, "POST no database", UNKNOWN, { NEWSLETTER_DB: undefined });
    await confirmGet(h, w, "GET malformed token", "short");
    await confirmGet(h, w, "GET unknown token", UNKNOWN);
    await confirmPost(h, w, "POST unknown token", UNKNOWN);
  },
  async "unsubscribe endpoint"(h, w) {
    await signup(h, w, "subscribed", { ...OK, language: "pl" });
    const token = w.tokenOf("reader@example.com");
    await unsubGet(h, w, "GET no database", token, { NEWSLETTER_DB: undefined });
    await unsubPost(h, w, "one-click no database", token, true, { NEWSLETTER_DB: undefined });
    await unsubPost(h, w, "button no database", token, false, { NEWSLETTER_DB: undefined });
    await unsubGet(h, w, "GET unknown", UNKNOWN);
    await unsubGet(h, w, "GET malformed token", "short");
    await unsubPost(h, w, "one-click malformed token", "short", true);
    await unsubGet(h, w, "GET subscribed", token);
    await unsubPost(h, w, "one-click unknown", UNKNOWN, true);
    await unsubPost(h, w, "button unknown", UNKNOWN, false);
    w.advance(MIN);
    await unsubPost(h, w, "button", token, false);
    w.advance(MIN);
    await unsubPost(h, w, "one-click repeat keeps the first time", token, true);
    await unsubGet(h, w, "GET unsubscribed", token);
    await w.call(h.unsubscribe.onRequestPost, "one-click multipart", {
      method: "POST",
      path: `/api/newsletter/unsubscribe?token=${token}`,
      headers: { "content-type": "multipart/form-data; boundary=b" },
      body: '--b\r\nContent-Disposition: form-data; name="List-Unsubscribe"\r\n\r\nOne-Click\r\n--b--\r\n',
    });
  },
  async "admin endpoint"(h, w) {
    await signup(h, w, "confirmed", { ...OK, tags: ["=cmd", "news"], consent_text: 'He said "yes",\nthen left' });
    await signup(h, w, "no language", { ...OK, email: "nolang@example.com", language: "", consent_text: "=1+1" });
    Object.assign(w.env, RESEND);
    await signup(h, w, "pending", { ...OK, email: "pending@example.com", language: "pl", source_page: "/pl/" });
    await unsubPost(h, w, "unsubscribed", w.tokenOf("nolang@example.com"), true);
    await admin(h, w, "no password configured", "", ADMIN, { NEWSLETTER_ADMIN_PASSWORD: undefined });
    await admin(h, w, "no credentials", "", null);
    await admin(h, w, "wrong password", "", "Basic " + btoa("admin:nope"));
    await admin(h, w, "malformed Basic", "", "Basic %%%");
    await admin(h, w, "Access without a token", "", null, { NEWSLETTER_ACCESS_TEAM: "team", NEWSLETTER_ACCESS_AUD: "aud" });
    await admin(h, w, "no database", "", ADMIN, { NEWSLETTER_DB: undefined });
    await admin(h, w, "counts");
    await admin(h, w, "CSV default", "?format=csv");
    for (const s of ["pending", "confirmed", "unsubscribed", "all"]) await admin(h, w, `CSV ${s}`, `?format=csv&status=${s}`);
    await admin(h, w, "CSV bad status", "?format=csv&status=bogus");
    await adminPost(h, w, "purge invalid JSON", "{");
    await adminPost(h, w, "purge wrong action", '{"action":"drop"}');
    await adminPost(h, w, "purge without retention", '{"action":"purge"}');
    w.advance(31 * DAY);
    await adminPost(h, w, "purge with retention", '{"action":"purge"}', { NEWSLETTER_RETENTION_DAYS: "30" });
    await admin(h, w, "counts after purge");
    await w.call(h.admin.onRequestPost, "POST no credentials", { method: "POST", path: "/api/newsletter/admin", body: "{}" });
    await adminPost(h, w, "POST no database", '{"action":"purge"}', { NEWSLETTER_DB: undefined });
  },
};

// CHANGED: the two deliberate behaviour changes. On 1.9.7 both signed the
// address up as confirmed (single opt-in: no provider key); now the first goes
// to SMTP (double opt-in) and the second is refused as not configured.
export const CHANGED: Record<string, Scenario> = {
  async "SMTP host with a sender"(h, w) {
    await signup(h, w, "sign-up", OK, { NEWSLETTER_MAIL_FROM: "news@example.com", NEWSLETTER_SMTP_HOST: "smtp.example.com" });
  },
  async "SMTP on port 25 with a sender"(h, w) {
    await signup(h, w, "sign-up", OK, { NEWSLETTER_MAIL_FROM: "news@example.com", NEWSLETTER_SMTP_HOST: "smtp.example.com", NEWSLETTER_SMTP_PORT: "25" });
  },
};

export interface Outcome {
  calls: unknown[];
  rows: Record<string, unknown>[];
  logs: string[];
}

// runScenarios runs each scenario in a fresh world and returns what it saw.
export async function runScenarios(h: Handlers, list: Record<string, Scenario>): Promise<Record<string, Outcome>> {
  const out: Record<string, Outcome> = {};
  for (const [name, s] of Object.entries(list)) {
    out[name] = await withWorld(async (w) => {
      await s(h, w);
      return { calls: w.log, rows: w.rows(), logs: w.logs };
    });
  }
  return out;
}
