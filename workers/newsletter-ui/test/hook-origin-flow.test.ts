// #463: with NEWSLETTER_HOOK_FROM_DOMAINS set, a verified webhook event from
// another sending domain is acknowledged and changes no table; an event from
// a listed domain is applied as before; the signature is checked first; a
// malformed list answers 503. Unset, nothing changes.
import { test } from "node:test";
import assert from "node:assert/strict";
import { CAMPAIGN_TAG } from "../src/bounce-attribution.ts";
import { findSuppression } from "../src/suppress.ts";
import { HARD, campaignWithSends, counts, statusOf } from "./attribution-fixtures.ts";
import type { FakeD1 } from "./fakes.ts";
import { hook, hookEnv, mcEvent, rfc9421Headers, svixHeaders } from "./hook-fixtures.ts";
import { row, seed } from "./subscriber-seed.ts";

const OURS = "example.com";
const LIST = { NEWSLETTER_HOOK_FROM_DOMAINS: "example.com, news.example.org" };

// snapshot: every row of every table, so "no table changed" is checked whole.
function snapshot(d1: FakeD1): Record<string, unknown[]> {
  const tables = d1.sqlite.prepare("SELECT name FROM sqlite_master WHERE type = 'table' ORDER BY name").all() as { name: string }[];
  return Object.fromEntries(tables.map(({ name }) => [name, d1.sqlite.prepare(`SELECT * FROM "${name}"`).all().map((r) => ({ ...(r as object) }))]));
}

// event: a Resend event with a sender, tagged with `campaign` when given.
const event = (type: string, to: string, from: unknown, o: { bounce?: Record<string, unknown>; campaign?: string } = {}) =>
  JSON.stringify({
    type,
    created_at: "2026-11-13T00:00:00.000Z",
    data: { email_id: "e1", from, to: [to], ...(o.campaign ? { tags: { [CAMPAIGN_TAG]: o.campaign } } : {}), ...(o.bounce ? { bounce: o.bounce } : {}) },
  });

async function resend(env: unknown, body: string) {
  const res = await hook(env, "resend", body, await svixHeaders(body));
  return { status: res.status, body: await res.json() };
}

async function mailchannels(env: unknown, events: unknown[]) {
  const body = JSON.stringify(events);
  const res = await hook(env, "mailchannels", body, await rfc9421Headers(body));
  return { status: res.status, body: await res.json() };
}

// prepared: an env with a subscriber, a soft-bounce count on another address
// and a campaign still sending (so a hard bounce would feed the breaker).
async function prepared(vars: Record<string, string> = LIST) {
  const env = await hookEnv({ NEWSLETTER_BOUNCE_BREAKER_MIN: "1", NEWSLETTER_BOUNCE_BREAKER_PERCENT: "1", ...vars });
  const d1 = env.NEWSLETTER_DB;
  const [sub] = seed(d1, [{ email: "reader@example.net" }]);
  for (let i = 0; i < 2; i++) await resend(env, event("email.bounced", "full@example.net", "news@example.com", { bounce: { type: "Transient", message: "mailbox full" } }));
  const campaign = campaignWithSends(d1, "sending", 10);
  return { env, d1, sub, campaign };
}

test("Resend, list set: another domain's events are acknowledged and change no table", async () => {
  const { env, d1, campaign } = await prepared();
  const before = snapshot(d1);
  const foreign = [
    event("email.bounced", "gone@example.net", "Other Site <news@example.org>", { bounce: HARD, campaign }),
    event("email.complained", "reader@example.net", "news@sub.example.com", { campaign }),
    event("email.delivered", "full@example.net", "news@example.net"),
    event("email.bounced", "full@example.net", "news@example.net", { bounce: { type: "Transient", message: "mailbox full" } }),
    event("email.bounced", "gone@example.net", undefined, { bounce: HARD }),
    event("email.bounced", "gone@example.net", "no address", { bounce: HARD }),
    JSON.stringify({ type: "email.bounced" }),
    JSON.stringify([1, 2]),
    "null",
  ];
  for (const body of foreign) assert.deepEqual(await resend(env, body), { status: 200, body: { ok: true, applied: 0, duplicate: 0, ignored: 1 } }, body);
  assert.deepEqual(snapshot(d1), before, "no suppression, unsubscribe, soft reset, activity, campaign bounce, breaker or dedupe row");
  assert.equal(statusOf(d1, campaign), "sending");
});

test("Resend, list set: a listed domain, any display name or case, behaves as before", async () => {
  const { env, d1, sub, campaign } = await prepared();
  const hard = await resend(env, event("email.bounced", "gone@example.net", "Example Name <Newsletter@Example.COM>", { bounce: HARD, campaign }));
  assert.deepEqual(hard, { status: 200, body: { ok: true, applied: 1, duplicate: 0, ignored: 0 } });
  assert.equal((await findSuppression(env as never, "gone@example.net"))?.reason, "hard");
  assert.deepEqual(counts(d1, campaign), { "resend:hard": 1 });
  assert.equal(statusOf(d1, campaign), "paused", "the breaker saw it");
  await resend(env, event("email.complained", "reader@example.net", "news@NEWS.example.org"));
  assert.equal(row(d1, sub)?.status, "unsubscribed");
  await resend(env, event("email.delivered", "full@example.net", `news@${OURS}`));
  assert.equal(d1.sqlite.prepare("SELECT COUNT(*) AS n FROM soft_bounces").get()?.n, 0, "delivered resets the soft count");
  const kinds = d1.sqlite.prepare("SELECT kind, count FROM hook_activity ORDER BY kind").all().map((r) => ({ ...r }));
  assert.deepEqual(kinds, [{ kind: "complaint", count: 1 }, { kind: "delivered", count: 1 }, { kind: "hard", count: 1 }, { kind: "soft", count: 2 }]);
});

test("Resend, list unset: another domain's events apply exactly as before", async () => {
  const { env, d1, sub } = await prepared({});
  assert.deepEqual(await resend(env, event("email.bounced", "gone@example.net", "news@example.org", { bounce: HARD })), {
    status: 200,
    body: { ok: true, applied: 1, duplicate: 0, ignored: 0 },
  });
  await resend(env, event("email.complained", "reader@example.net", undefined));
  assert.equal(row(d1, sub)?.status, "unsubscribed");
  assert.equal((await resend(env, JSON.stringify([1]))).body.ignored, 1, "an unclassified event is ignored as before");
});

test("the signature is checked first: unsigned is 401 whatever the domain or the list", async () => {
  for (const vars of [LIST, { NEWSLETTER_HOOK_FROM_DOMAINS: "news@example.com" }]) {
    const { env, d1 } = await prepared(vars);
    const before = snapshot(d1);
    for (const from of ["news@example.com", "news@example.org"]) {
      const body = event("email.bounced", "gone@example.net", from, { bounce: HARD });
      const res = await hook(env, "resend", body, { ...(await svixHeaders(body)), "svix-signature": "v1,AAAA" });
      assert.equal(res.status, 401);
      const mc = JSON.stringify([mcEvent("hard-bounced", ["gone@example.net"], { email: from })]);
      const res2 = await hook(env, "mailchannels", mc, { ...(await rfc9421Headers(mc)), signature: "sig_1738775282=:AAAA:" });
      assert.equal(res2.status, 401);
    }
    assert.deepEqual(snapshot(d1), before);
  }
});

test("a malformed list: both hooks answer 503 and write nothing, so the provider retries", async () => {
  const { env, d1 } = await prepared({ NEWSLETTER_HOOK_FROM_DOMAINS: "example.com, https://example.org" });
  const before = snapshot(d1);
  const r = await resend(env, event("email.bounced", "gone@example.net", "news@example.com", { bounce: HARD }));
  assert.equal(r.status, 503);
  assert.equal(r.body.error, "hook_misconfigured");
  assert.match(r.body.detail, /"https:\/\/example\.org" is not a plain domain/);
  const m = await mailchannels(env, [mcEvent("hard-bounced", ["gone@example.net"])]);
  assert.deepEqual([m.status, m.body.error], [503, "hook_misconfigured"]);
  assert.deepEqual(snapshot(d1), before);
});

test("MailChannels, list set: events are filtered one by one on `email`, the sender", async () => {
  const { env, d1, sub } = await prepared();
  const before = snapshot(d1);
  const foreign = [
    mcEvent("hard-bounced", ["gone@example.net"], { email: "news@example.org", status: "550", reason: "5.1.1 unknown" }),
    mcEvent("complained", ["reader@example.net"], { email: undefined }),
    mcEvent("delivered", ["full@example.net"], { email: "news@sub.example.com" }),
  ];
  assert.deepEqual(await mailchannels(env, foreign), { status: 200, body: { ok: true, applied: 0, duplicate: 0, ignored: 3 } });
  assert.deepEqual(snapshot(d1), before, "a request of other sites' events writes nothing");
  const mixed = [...foreign, mcEvent("complained", ["reader@example.net"], { email: "News@Example.com" }), mcEvent("processed", ["x@example.net"])];
  assert.deepEqual(await mailchannels(env, mixed), { status: 200, body: { ok: true, applied: 1, duplicate: 0, ignored: 4 } });
  assert.equal(row(d1, sub)?.status, "unsubscribed");
  assert.equal(await findSuppression(env as never, "gone@example.net"), null);
  assert.equal(d1.sqlite.prepare("SELECT count FROM soft_bounces").get()?.count, 2, "the foreign delivered reset nothing");
  const kinds = d1.sqlite.prepare("SELECT kind, count FROM hook_activity ORDER BY kind").all().map((r) => ({ ...r }));
  assert.deepEqual(kinds, [{ kind: "complaint", count: 1 }, { kind: "other", count: 1 }, { kind: "soft", count: 2 }], "only our events are counted (soft: the seed)");
});

test("MailChannels, list unset: every sender applies, an empty batch still settles as before", async () => {
  const { env } = await prepared({});
  const r = await mailchannels(env, [mcEvent("hard-bounced", ["gone@example.net"], { email: "news@example.org" })]);
  assert.deepEqual(r.body, { ok: true, applied: 1, duplicate: 0, ignored: 0 });
  assert.equal((await findSuppression(env as never, "gone@example.net"))?.reason, "hard");
  assert.deepEqual((await mailchannels(env, [])).body, { ok: true, applied: 0, duplicate: 0, ignored: 0 });
  const listed = await prepared();
  assert.deepEqual((await mailchannels(listed.env, [])).body, { ok: true, applied: 0, duplicate: 0, ignored: 0 }, "an empty batch with a list set");
});
