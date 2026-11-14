// GO-137: the engine is not a person. With `auth` sessions switched on (and no
// password) the cron and the queue consumer still send a whole campaign with no
// session anywhere, and the public hooks still answer without one.
import { test } from "node:test";
import assert from "node:assert/strict";
import worker from "../src/index.ts";
import { AUTH_VARS } from "./auth-fixtures.ts";
import { addCampaign, campaignRow, engineEnv, fakeMailer, pump } from "./engine-fixtures.ts";
import { fakeCtx, req } from "./fakes.ts";

test("auth mode, no session: the cron and the queue send a scheduled campaign", async () => {
  const env = await engineEnv(3, { ...AUTH_VARS, NEWSLETTER_ADMIN_PASSWORD: undefined });
  const ctx = fakeCtx();
  worker.scheduled({ cron: "* * * * *", scheduledTime: Date.now() } as never, env as never, ctx as never);
  await Promise.all(ctx.pending);
  const id = addCampaign(env.NEWSLETTER_DB);
  const mailer = fakeMailer();
  await pump(env, { open: mailer.open });
  assert.equal(campaignRow(env.NEWSLETTER_DB, id).status, "sent");
  assert.equal(mailer.sent.length, 3);
  // Through the Worker's own queue entry point too.
  await worker.queue({ queue: "ssg-newsletter-send", messages: [], ackAll() {}, retryAll() {} } as never, env as never);
});

test("auth mode, no session: the hooks answer as without auth", async () => {
  const env = await engineEnv(0, { ...AUTH_VARS, NEWSLETTER_ADMIN_PASSWORD: undefined });
  const click = await worker.fetch(req("/newsletter-admin/hooks/c/none/0"), env as never, fakeCtx() as never);
  assert.notEqual(click.status, 401);
  assert.notEqual(click.status, 302);
  const hook = await worker.fetch(req("/newsletter-admin/hooks/resend", { method: "POST", body: "{}", csrf: false }), env as never, fakeCtx() as never);
  assert.equal(hook.status, 503, "not configured (no webhook secret) — the hook's own answer, not the admin gate's");
  // The admin API next to them does need a session.
  assert.equal((await worker.fetch(req("/newsletter-admin/api/health"), env as never, fakeCtx() as never)).status, 401);
});
