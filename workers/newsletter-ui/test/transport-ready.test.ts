// An SMTP configuration that can never send is refused up front by every
// route that sends — confirmation mails, campaign test sends, scheduling, the
// SMTP connection test — and /api/settings says why. Nothing is sent, nothing
// is scheduled, no test-send budget is used.
import { test } from "node:test";
import assert from "node:assert/strict";
import { transportReady } from "../src/transport-ready.ts";
import { TEST_KEY } from "../src/campaign-limit.ts";
import { call, mailEnv } from "./campaign-fixtures.ts";
import { add, confirmEnv, post } from "./confirm-fixtures.ts";
import { PASSWORD, fakeCtx, req } from "./fakes.ts";
import { handleFetch } from "../src/app.ts";
import { ensureSchema } from "../src/schema.ts";

const PORT_25 = { NEWSLETTER_RESEND_API_KEY: undefined, NEWSLETTER_SMTP_HOST: "smtp.example.com", NEWSLETTER_SMTP_PORT: "25" };
const UNUSABLE = { error: "transport_unusable", detail: "smtp: port 25 is blocked by Cloudflare for outbound connections; use 465 (TLS) or 587 (STARTTLS)" };

test("transportReady: none, unusable, usable", async () => {
  const none = transportReady({} as never);
  assert.deepEqual([none?.status, await none?.json()], [503, { error: "no_transport", detail: "configure Resend, MailChannels or SMTP (see README)" }]);
  const bad = transportReady({ NEWSLETTER_SMTP_HOST: "smtp.example.com", NEWSLETTER_SMTP_PORT: "abc" } as never);
  assert.equal(bad?.status, 503);
  assert.equal((await bad?.json()).error, "transport_unusable");
  assert.equal(transportReady({ NEWSLETTER_SMTP_HOST: "smtp.example.com" } as never), null);
  assert.equal(transportReady({ NEWSLETTER_RESEND_API_KEY: "re" } as never), null);
});

test("confirmation mails: 503 transport_unusable, nothing sent or marked", async () => {
  const env = await confirmEnv(PORT_25);
  add(env.NEWSLETTER_DB, "p1");
  for (const path of ["/confirmations", "/p1/confirmation"]) {
    const r = await post(env, path);
    assert.deepEqual([r.status, r.body], [503, UNUSABLE], path);
  }
  assert.equal(env.NEWSLETTER_DB.sqlite.prepare("SELECT confirm_sent_at FROM subscribers").get()?.confirm_sent_at, null);
});

test("campaigns: test send and schedule refuse an unusable transport before anything else", async () => {
  const env = mailEnv(PORT_25);
  await ensureSchema(env as never);
  const created = await call(env, "POST", "", { subject: "Hello", body: "Hi" });
  const id = created.body.id;
  const testSend = await call(env, "POST", `/${id}/test`, { to: "editor@example.com" });
  assert.deepEqual([testSend.status, testSend.body], [503, UNUSABLE]);
  const scheduled = await call(env, "POST", `/${id}/schedule`, {});
  assert.deepEqual([scheduled.status, scheduled.body], [503, UNUSABLE]);
  assert.equal((await call(env, "GET", `/${id}`)).body.status, "draft");
  const budget = env.NEWSLETTER_DB.sqlite.prepare("SELECT COUNT(*) AS n FROM settings WHERE key = ?").get(TEST_KEY) as { n: number };
  assert.equal(budget.n, 0, "no test-send budget used");
});

test("settings: transportProblem says why; the SMTP test refuses without using the budget", async () => {
  const env = mailEnv(PORT_25);
  await ensureSchema(env as never);
  const get = async (method: string, path: string) =>
    handleFetch(req("/newsletter-admin" + path, { method, auth: PASSWORD }), env as never, fakeCtx() as never);
  const settings = await (await get("GET", "/api/settings")).json();
  assert.deepEqual([settings.transport, settings.transportProblem], ["smtp", UNUSABLE.detail]);
  const smtp = await get("POST", "/api/settings/smtp-test");
  assert.equal(smtp.status, 503);
  assert.deepEqual(await smtp.json(), { ...UNUSABLE, transport: "smtp" });
  const fine = mailEnv();
  await ensureSchema(fine as never);
  const ok = await handleFetch(req("/newsletter-admin/api/settings", { auth: PASSWORD }), fine as never, fakeCtx() as never);
  assert.equal((await ok.json()).transportProblem, null);
});
